// Package compiler turns an Agent CR's composition — inline AgentSpec
// fragment plus Model/Instruction/AgentTool references — into one resolved
// pydantic-ai AgentSpec document, validated against the runner's pinned
// AgentSpec JSON Schema (runtime contract).
//
// Merge precedence (product brief §3, deterministic): referenced CRs compose
// in declared order; inline fragment scalars win conflicts; list fields
// append (instructionRefs content before fragment instructions; tool- and
// platform-derived capability entries after fragment capabilities).
package compiler

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	capabilitydomain "github.com/danielnyari/flokoa/internal/domain/capability"
	modeldomain "github.com/danielnyari/flokoa/internal/domain/model"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/infra/repo"
	"github.com/danielnyari/flokoa/internal/spec"
)

// Deps are the read-side repositories the compiler resolves references with.
type Deps struct {
	Models             repo.ModelReader
	Providers          repo.ModelProviderReader
	Instructions       repo.InstructionReader
	AgentTools         repo.AgentToolReader
	Capabilities       repo.CapabilityReader
	Services           repo.ServiceReader
	GetProviderHandler func(agentv1alpha1.ProviderType) (modeldomain.ProviderHandler, bool)
}

// InjectedCapability is a flokoa-owned capability entry appended to every
// compiled spec after all user entries (roadmap 07). Config is the entry's
// keyword arguments; nil means the bare-name form.
type InjectedCapability struct {
	Name   string
	Config map[string]any
}

// Options carry cluster-level compiler policy.
type Options struct {
	// DefaultRunnerVersion is used when the Agent doesn't pin one.
	DefaultRunnerVersion string

	// Injected platform capabilities, appended in order.
	Injected []InjectedCapability
}

// CapabilityArtifact is the delivery input one Capability attachment
// contributes (roadmap 09 turns these into initContainer/ImageVolume mounts).
type CapabilityArtifact struct {
	// Name is the Capability CR name (the wheelhouse directory name under
	// /opt/flokoa/capabilities/).
	Name string
	// Artifact is the digest-pinned OCI reference.
	Artifact string
	// EntryName is the capability's spec-entry name in the compiled document.
	EntryName string
}

// Result is a successfully compiled and schema-validated spec.
type Result struct {
	// Doc is the resolved AgentSpec document.
	Doc map[string]any

	// YAML is the serialized agent-spec.yaml content (deterministic).
	YAML []byte

	// Hash identifies this resolved spec (status.specHash; rollout trigger).
	Hash string

	// RunnerVersion the document was validated against.
	RunnerVersion string

	// SchemaDigest of the embedded schema used (runner cross-checks it).
	SchemaDigest string

	// Injected lists the names of platform entries appended (status surfacing).
	Injected []string

	// SecretEnv projects ${secret:NAME} references as FLOKOA_SECRET_* env
	// vars (sorted by name): the Agent's secretRefs plus compiler-derived
	// AgentTool header secrets.
	SecretEnv []corev1.EnvVar

	// ProviderEnv / ProviderSecretEnv come from the resolved ModelProvider.
	ProviderEnv       []corev1.EnvVar
	ProviderSecretEnv []corev1.EnvVar

	// CapabilityArtifacts are the delivery inputs for the attached Capability
	// CRs, in declaration order (consumed by roadmap 09's builder).
	CapabilityArtifacts []CapabilityArtifact
}

// ValidationError marks a compiled document that failed schema validation:
// a permanent condition (SpecValid=False) until the composition is edited —
// running pods stay on the last good spec.
type ValidationError struct {
	RunnerVersion string
	Err           error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("compiled spec is invalid for runner %s: %v", e.RunnerVersion, e.Err)
}

func (e *ValidationError) Unwrap() error { return e.Err }

// Compiler resolves and merges an Agent's composition graph.
type Compiler struct {
	deps Deps
	opts Options
}

func New(deps Deps, opts Options) *Compiler {
	if deps.GetProviderHandler == nil {
		deps.GetProviderHandler = modeldomain.GetProviderHandler
	}
	return &Compiler{deps: deps, opts: opts}
}

// Compile resolves references, merges the composition, injects platform
// capabilities, and validates against the embedded AgentSpec schema.
func (c *Compiler) Compile(ctx context.Context, agent *agentv1alpha1.Agent) (*Result, error) {
	runnerVersion := agent.Spec.Runtime.RunnerVersion
	if runnerVersion == "" {
		runnerVersion = c.opts.DefaultRunnerVersion
	}

	res := &Result{RunnerVersion: runnerVersion}
	doc := map[string]any{}
	frag := agent.Spec.Spec

	// 1. Fragment extra: the lowest-precedence layer (typed fields win).
	if frag != nil && frag.Extra != nil {
		if err := unmarshalJSONObject(frag.Extra.Raw, &doc); err != nil {
			return nil, flokoaerrors.NewPermanentf("spec.spec.extra must be a JSON object: %v", err)
		}
	}

	// 2. Fragment typed fields.
	doc["name"] = agent.Name
	if frag != nil {
		applyFragment(doc, frag)
	}

	// 3. Model reference (inline model wins; settings merge per-key, inline wins).
	if agent.Spec.ModelRef != nil {
		if err := c.applyModelRef(ctx, agent, doc, res); err != nil {
			return nil, err
		}
	}

	// 4. Instructions: refs in declared order, then fragment instructions.
	if err := c.applyInstructions(ctx, agent, doc); err != nil {
		return nil, err
	}

	// 5. AgentTool refs compile to MCP capability entries, appended after
	// the fragment's capability entries.
	if err := c.applyTools(ctx, agent, doc, res); err != nil {
		return nil, err
	}

	// 5b. Capability CR attachments: resolved and checked now (requires tuple,
	// config schema, dependency conflicts — the same domain checks admission
	// ran; re-run here because Capability edits recompile dependent Agents
	// without re-admission), but spliced into the doc only after schema
	// validation: the embedded AgentSpec schema knows baseline-native and
	// platform entries, while attachment config is governed by the
	// capability's own published schema.
	capEntries, capArtifacts, err := c.resolveCapabilities(ctx, agent, runnerVersion)
	if err != nil {
		return nil, err
	}
	res.CapabilityArtifacts = capArtifacts

	// 6. Platform capabilities, last — user entries can't shadow or reorder them.
	for _, inj := range c.opts.Injected {
		doc["capabilities"] = append(capabilityList(doc), capabilityEntry(inj.Name, inj.Config))
		res.Injected = append(res.Injected, inj.Name)
	}

	// 7. Agent secretRefs project to FLOKOA_SECRET_* env (sorted).
	names := make([]string, 0, len(agent.Spec.SecretRefs))
	for name := range agent.Spec.SecretRefs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		selector := agent.Spec.SecretRefs[name]
		res.SecretEnv = append(res.SecretEnv, corev1.EnvVar{
			Name:      spec.SecretEnvName(name),
			ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &selector},
		})
	}

	// 8. Canonicalize to JSON-generic form: the validated document is exactly
	// the document hashed and serialized — no typed-Go leftovers.
	doc, err = canonicalize(doc)
	if err != nil {
		return nil, fmt.Errorf("canonicalizing compiled spec: %w", err)
	}

	// 9. Validate against the pinned AgentSpec schema. No Deployment update
	// happens on an invalid spec (the caller keeps the last good one running).
	if err := spec.Validate(runnerVersion, doc); err != nil {
		return nil, &ValidationError{RunnerVersion: runnerVersion, Err: err}
	}

	// 10. Splice the Capability attachment entries in after the fragment/tool
	// entries and before the injected platform block (which stays last).
	if len(capEntries) > 0 {
		existing := capabilityList(doc)
		insertAt := len(existing) - len(res.Injected)
		merged := make([]any, 0, len(existing)+len(capEntries))
		merged = append(merged, existing[:insertAt]...)
		merged = append(merged, capEntries...)
		merged = append(merged, existing[insertAt:]...)
		doc["capabilities"] = merged
		if doc, err = canonicalize(doc); err != nil {
			return nil, fmt.Errorf("canonicalizing compiled spec: %w", err)
		}
	}

	digest, err := spec.SchemaDigest(runnerVersion)
	if err != nil {
		return nil, err
	}
	res.SchemaDigest = digest

	hashValue, err := hashDoc(doc)
	if err != nil {
		return nil, fmt.Errorf("hashing compiled spec: %w", err)
	}
	res.Hash = hashValue

	out, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("serializing compiled spec: %w", err)
	}
	res.Doc = doc
	res.YAML = out
	return res, nil
}

func (c *Compiler) applyModelRef(ctx context.Context, agent *agentv1alpha1.Agent, doc map[string]any, res *Result) error {
	ref := agent.Spec.ModelRef
	key := types.NamespacedName{Name: ref.Name, Namespace: defaultNS(ref.Namespace, agent.Namespace)}

	model, err := c.deps.Models.GetModel(ctx, key)
	if err != nil {
		return flokoaerrors.NewDependencyf("referenced Model %s not found: %v", key, err)
	}
	if !model.Status.Ready {
		return flokoaerrors.NewDependencyf("referenced Model %s is not ready", key)
	}

	providerKey := types.NamespacedName{
		Name:      model.Spec.ProviderRef.Name,
		Namespace: defaultNS(model.Spec.ProviderRef.Namespace, model.Namespace),
	}
	provider, err := c.deps.Providers.GetModelProvider(ctx, providerKey)
	if err != nil {
		return flokoaerrors.NewDependencyf("ModelProvider %s (via Model %s) not found: %v", providerKey, key, err)
	}

	providerType := provider.GetProviderType()
	handler, ok := c.deps.GetProviderHandler(providerType)
	if !ok {
		return flokoaerrors.NewPermanentf("unsupported provider type %q on ModelProvider %s", providerType, providerKey)
	}
	resolved, err := handler.Resolve(provider)
	if err != nil {
		return fmt.Errorf("resolving provider %s: %w", providerKey, err)
	}

	// The provider's environment is needed regardless of whether the inline
	// model wins: the runner authenticates with provider-native env vars.
	res.ProviderEnv = resolved.EnvVars
	res.ProviderSecretEnv = resolved.SecretEnvVars

	// Inline model wins the scalar conflict.
	if s, _ := doc["model"].(string); s == "" {
		doc["model"] = modeldomain.QualifiedModelName(resolved.ModelPrefix, model.Spec.Model)
	}

	// Model settings merge under the fragment's (per-key, inline wins).
	if model.Spec.Settings != nil {
		base := settingsToMap(model.Spec.Settings)
		if len(base) > 0 {
			merged := base
			if existing, ok := doc["model_settings"].(map[string]any); ok {
				for k, v := range existing {
					merged[k] = v
				}
			}
			doc["model_settings"] = merged
		}
	}
	return nil
}

func (c *Compiler) applyInstructions(ctx context.Context, agent *agentv1alpha1.Agent, doc map[string]any) error {
	instructions := make([]string, 0, len(agent.Spec.InstructionRefs))
	for _, ref := range agent.Spec.InstructionRefs {
		key := types.NamespacedName{Name: ref.Name, Namespace: defaultNS(ref.Namespace, agent.Namespace)}
		instr, err := c.deps.Instructions.GetInstruction(ctx, key)
		if err != nil {
			return flokoaerrors.NewDependencyf("referenced Instruction %s not found: %v", key, err)
		}
		instructions = append(instructions, instr.Spec.Content)
	}

	// Fragment instructions append after the refs' content.
	instructions = append(instructions, fragmentInstructions(agent.Spec.Spec, doc)...)

	if len(instructions) > 0 {
		doc["instructions"] = instructions
	}
	return nil
}

func (c *Compiler) applyTools(ctx context.Context, agent *agentv1alpha1.Agent, doc map[string]any, res *Result) error {
	for _, ref := range agent.Spec.Tools {
		key := types.NamespacedName{Name: ref.Name, Namespace: defaultNS(ref.Namespace, agent.Namespace)}
		tool, err := c.deps.AgentTools.GetAgentTool(ctx, key)
		if err != nil {
			return flokoaerrors.NewDependencyf("referenced AgentTool %s not found: %v", key, err)
		}

		entry, secretEnv, err := c.compileTool(ctx, tool)
		if err != nil {
			return err
		}
		doc["capabilities"] = append(capabilityList(doc), entry)
		res.SecretEnv = append(res.SecretEnv, secretEnv...)

		// Per-tool timeouts compile to the agent-level tool_timeout: the
		// largest requested timeout wins; an explicit fragment value wins
		// over all tool-derived ones (inline-wins rule).
		if tool.Spec.TimeoutSeconds != nil && !fragmentSetsToolTimeout(agent.Spec.Spec) {
			t := float64(*tool.Spec.TimeoutSeconds)
			if existing, ok := doc["tool_timeout"].(float64); !ok || t > existing {
				doc["tool_timeout"] = t
			}
		}
	}
	return nil
}

// resolveCapabilities fetches each attached Capability CR and re-runs the
// admission-time domain checks (requires tuple, config schema, dependency
// conflicts) against the resolved runner baseline. It returns the spec
// entries (declaration order) and the artifact delivery inputs; the caller
// splices the entries in after schema validation.
func (c *Compiler) resolveCapabilities(ctx context.Context, agent *agentv1alpha1.Agent, runnerVersion string) ([]any, []CapabilityArtifact, error) {
	if len(agent.Spec.Capabilities) == 0 {
		return nil, nil, nil
	}
	if c.deps.Capabilities == nil {
		return nil, nil, flokoaerrors.NewPermanentf("capability attachments are not supported by this operator build (no Capability reader wired)")
	}

	runner, err := spec.RunnerBaseline(runnerVersion)
	if err != nil {
		return nil, nil, &ValidationError{RunnerVersion: runnerVersion, Err: err}
	}

	// Entry names already claimed: fragment-native entries plus the platform
	// entries injected last. Attachments may not collide with any of them.
	entryOwners := map[string]string{}
	if agent.Spec.Spec != nil {
		for _, frag := range agent.Spec.Spec.Capabilities {
			entryOwners[frag.Name] = "spec.spec.capabilities"
		}
	}
	for _, inj := range c.opts.Injected {
		entryOwners[inj.Name] = "injected platform capability"
	}

	entries := make([]any, 0, len(agent.Spec.Capabilities))
	artifacts := make([]CapabilityArtifact, 0, len(agent.Spec.Capabilities))
	deps := make([]capabilitydomain.Deps, 0, len(agent.Spec.Capabilities))
	seen := map[string]bool{}
	for _, att := range agent.Spec.Capabilities {
		key := types.NamespacedName{Name: att.Ref.Name, Namespace: defaultNS(att.Ref.Namespace, agent.Namespace)}

		// Cross-namespace capability references are not supported yet (see the
		// Agent webhook for the rationale): reject without reading the foreign
		// CR so its internals cannot leak into the Agent's status.
		if key.Namespace != agent.Namespace {
			return nil, nil, flokoaerrors.NewPermanentf(
				"cross-namespace Capability reference %s is not supported yet; reference a Capability in the agent's namespace", key)
		}
		if seen[key.String()] {
			return nil, nil, flokoaerrors.NewPermanentf("Capability %s is attached more than once", key)
		}
		seen[key.String()] = true

		capCR, err := c.deps.Capabilities.GetCapability(ctx, key)
		if err != nil {
			return nil, nil, flokoaerrors.NewDependencyf("referenced Capability %s not found: %v", key, err)
		}

		req := capabilitydomain.Requires{
			Python:       capCR.Spec.Requires.Python,
			PydanticAI:   capCR.Spec.Requires.PydanticAI,
			FlokoaRunner: capCR.Spec.Requires.FlokoaRunner,
		}
		if err := capabilitydomain.CheckRequires(capCR.Name, req, runner); err != nil {
			return nil, nil, flokoaerrors.NewPermanentf("%v", err)
		}

		if capCR.Spec.SchemaPolicy != agentv1alpha1.SchemaPolicyPermissive && capCR.Spec.ConfigSchema != nil {
			configRaw := []byte("{}")
			if att.Config != nil {
				configRaw = att.Config.Raw
			}
			if err := capabilitydomain.ValidateConfig(capCR.Spec.ConfigSchema.Raw, configRaw); err != nil {
				return nil, nil, flokoaerrors.NewPermanentf("Capability %s rejected the attachment config: %v", key, err)
			}
		}

		var config map[string]any
		if att.Config != nil {
			if err := unmarshalJSONObject(att.Config.Raw, &config); err != nil {
				return nil, nil, flokoaerrors.NewPermanentf("Capability %s attachment config must be a JSON object: %v", key, err)
			}
		}

		entryName := capabilitydomain.EntryName(capCR.Spec.Entrypoint, capCR.Spec.SerializationName)
		if err := capabilitydomain.ValidateEntryName(entryName); err != nil {
			return nil, nil, flokoaerrors.NewPermanentf("Capability %s: %v", key, err)
		}
		if owner, dup := entryOwners[entryName]; dup {
			return nil, nil, flokoaerrors.NewPermanentf(
				"Capability %s compiles to spec entry %q, already claimed by %s; set spec.serializationName to disambiguate",
				key, entryName, owner)
		}
		entryOwners[entryName] = key.String()

		entries = append(entries, capabilityEntry(entryName, config))
		artifacts = append(artifacts, CapabilityArtifact{Name: capCR.Name, Artifact: capCR.Spec.Artifact, EntryName: entryName})
		deps = append(deps, capabilitydomain.Deps{Name: key.String(), Pins: capCR.Spec.Dependencies})
	}

	if msgs := capabilitydomain.DetectConflicts(deps, runner); len(msgs) > 0 {
		return nil, nil, flokoaerrors.NewPermanentf("capability dependency conflicts: %s", strings.Join(msgs, "; "))
	}

	return entries, artifacts, nil
}

func defaultNS(ns, fallback string) string {
	if ns == "" {
		return fallback
	}
	return ns
}

func unmarshalJSONObject(raw []byte, into *map[string]any) error {
	return yaml.Unmarshal(raw, into)
}

func hashDoc(doc map[string]any) (string, error) {
	return hashJSON(doc)
}
