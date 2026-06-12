/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	capabilitydomain "github.com/danielnyari/flokoa/internal/domain/capability"
	"github.com/danielnyari/flokoa/internal/spec"
)

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-agent,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=agents,verbs=create;update,versions=v1alpha1,name=vagent-v1alpha1.kb.io,admissionReviewVersions=v1

// AgentCustomValidator validates Agent resources.
// Reader is used for cross-resource reference validation (fixes #94) and for
// the capability-aware admission checks of roadmap 08, which read the
// referenced Capability CRs.
//
// +kubebuilder:object:generate=false
type AgentCustomValidator struct {
	Reader client.Reader

	// CapabilityReader reads Capability CRs for the admission checks. It is
	// the manager's uncached APIReader so a Capability created moments before
	// an Agent cannot slip past the deny checks through a stale cache. Falls
	// back to Reader when unset (direct-call unit tests).
	CapabilityReader client.Reader

	// DefaultRunnerVersion resolves which embedded runner baseline the
	// capability checks evaluate against when the Agent doesn't pin one.
	// Empty falls back to spec.DefaultRunnerVersion.
	DefaultRunnerVersion string
}

var _ webhook.CustomValidator = &AgentCustomValidator{}

// SetupAgentWebhookWithManager registers the webhook for Agent in the manager.
func SetupAgentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&agentv1alpha1.Agent{}).
		WithValidator(&AgentCustomValidator{
			Reader:               mgr.GetClient(),
			CapabilityReader:     mgr.GetAPIReader(),
			DefaultRunnerVersion: spec.DefaultRunnerVersion,
		}).
		Complete()
}

// capabilityReader returns the reader used for Capability lookups.
func (v *AgentCustomValidator) capabilityReader() client.Reader {
	if v.CapabilityReader != nil {
		return v.CapabilityReader
	}
	return v.Reader
}

func (v *AgentCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	agent, ok := obj.(*agentv1alpha1.Agent)
	if !ok {
		return nil, fmt.Errorf("expected an Agent but got %T", obj)
	}
	return v.validate(ctx, agent)
}

func (v *AgentCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	agent, ok := newObj.(*agentv1alpha1.Agent)
	if !ok {
		return nil, fmt.Errorf("expected an Agent but got %T", newObj)
	}
	return v.validate(ctx, agent)
}

func (v *AgentCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *AgentCustomValidator) validate(ctx context.Context, agent *agentv1alpha1.Agent) (admission.Warnings, error) {
	warnings := v.validateReferences(ctx, agent)

	allErrs := validateAgent(agent)

	capWarnings, capErrs := v.validateCapabilities(ctx, agent)
	warnings = append(warnings, capWarnings...)
	allErrs = append(allErrs, capErrs...)

	return warnings, aggregateFieldErrors("Agent", agent.Name, allErrs)
}

// secretPlaceholderRe matches the runtime contract's placeholder grammar (§3).
var secretPlaceholderRe = regexp.MustCompile(`\$\{secret:([^}]*)\}`)

// validSecretRefName matches the NAME grammar of ${secret:NAME}.
var validSecretRefName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateAgent(agent *agentv1alpha1.Agent) field.ErrorList {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Runtime: the session isolation tier ships with the session router (P1).
	if agent.Spec.Runtime.Isolation == agentv1alpha1.IsolationSession {
		allErrs = append(allErrs, field.Forbidden(
			specPath.Child("runtime", "isolation"),
			"the session isolation tier is not available yet (roadmap P1: session router + pools); use \"shared\"",
		))
	}

	// Fragment capability names: baseline-native names only. Harness and
	// third-party capabilities ship exclusively through Capability CRs;
	// class/module paths are their signature. flokoa.platform/* names are
	// operator-injected and cannot be user-set.
	if agent.Spec.Spec != nil {
		fragPath := specPath.Child("spec")
		for i, capEntry := range agent.Spec.Spec.Capabilities {
			capPath := fragPath.Child("capabilities").Index(i).Child("name")
			if strings.ContainsAny(capEntry.Name, "./:") {
				allErrs = append(allErrs, field.Invalid(capPath, capEntry.Name,
					"only baseline-native capability names are allowed inline (e.g. WebSearch, MCP, Thinking); "+
						"harness and third-party capabilities ship as Capability resources",
				))
			}
		}

		// ${secret:NAME} placeholders must have matching secretRefs keys.
		allErrs = append(allErrs, validatePlaceholders(fragPath, agent.Spec.Spec, agent.Spec.SecretRefs)...)
	}

	// Placeholders in capability attachment configs follow the same grammar.
	for i, att := range agent.Spec.Capabilities {
		if att.Config == nil {
			continue
		}
		configPath := specPath.Child("capabilities").Index(i).Child("config")
		allErrs = append(allErrs, validatePlaceholders(configPath, att.Config, agent.Spec.SecretRefs)...)
	}

	// secretRefs keys must satisfy the placeholder NAME grammar.
	for name := range agent.Spec.SecretRefs {
		if !validSecretRefName.MatchString(name) {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("secretRefs").Key(name), name,
				"secret ref names must match [A-Za-z0-9._-]+"))
		}
	}

	// Unique skill IDs on the card.
	seenSkills := make(map[string]bool)
	for i, skill := range agent.Spec.Card.Skills {
		if seenSkills[skill.ID] {
			allErrs = append(allErrs, field.Duplicate(
				specPath.Child("card", "skills").Index(i).Child("id"), skill.ID))
		}
		seenSkills[skill.ID] = true
	}

	return allErrs
}

// validatePlaceholders scans a JSON-serializable value for ${secret:NAME}
// placeholders and checks each has a matching secretRefs key. Scanning the
// JSON form keeps this exhaustive as shapes grow fields.
func validatePlaceholders(path *field.Path, value any, secretRefs map[string]corev1.SecretKeySelector) field.ErrorList {
	var allErrs field.ErrorList
	for _, name := range jsonSecretPlaceholders(value) {
		if !validSecretRefName.MatchString(name) {
			allErrs = append(allErrs, field.Invalid(path, fmt.Sprintf("${secret:%s}", name),
				"secret placeholder names must match [A-Za-z0-9._-]+"))
			continue
		}
		if _, ok := secretRefs[name]; !ok {
			allErrs = append(allErrs, field.Invalid(path, fmt.Sprintf("${secret:%s}", name),
				fmt.Sprintf("placeholder has no matching spec.secretRefs[%q] entry", name)))
		}
	}
	return allErrs
}

// jsonSecretPlaceholders scans every string value of a JSON-serializable
// document for ${secret:NAME} placeholders.
func jsonSecretPlaceholders(value any) []string {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	matches := secretPlaceholderRe.FindAllStringSubmatch(string(raw), -1)
	names := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		if !seen[m[1]] {
			names = append(names, m[1])
			seen[m[1]] = true
		}
	}
	return names
}

// validateCapabilities runs the three capability-aware admission checks of
// roadmap 08 against each attached Capability CR: config-schema validation,
// requires-tuple compatibility, and dependency-conflict detection. Missing
// Capability CRs warn instead of denying (the ordering-tolerant reference
// pattern of #94) — the compiler re-runs the same domain checks before
// anything deploys, so an incompatible composition still cannot reach a pod.
func (v *AgentCustomValidator) validateCapabilities(ctx context.Context, agent *agentv1alpha1.Agent) (admission.Warnings, field.ErrorList) {
	if len(agent.Spec.Capabilities) == 0 || v.Reader == nil {
		return nil, nil
	}

	var warnings admission.Warnings
	var allErrs field.ErrorList
	capsPath := field.NewPath("spec", "capabilities")

	runnerVersion := agent.Spec.Runtime.RunnerVersion
	if runnerVersion == "" {
		runnerVersion = v.DefaultRunnerVersion
	}
	if runnerVersion == "" {
		runnerVersion = spec.DefaultRunnerVersion
	}
	runner, err := spec.RunnerBaseline(runnerVersion)
	if err != nil {
		return warnings, append(allErrs, field.Invalid(
			field.NewPath("spec", "runtime", "runnerVersion"), runnerVersion, err.Error()))
	}

	// Fragment-native capability names participate in entry-name uniqueness.
	entryOwners := map[string]string{}
	if agent.Spec.Spec != nil {
		for _, frag := range agent.Spec.Spec.Capabilities {
			entryOwners[frag.Name] = "spec.spec.capabilities[" + frag.Name + "]"
		}
	}

	seen := map[types.NamespacedName]bool{}
	deps := make([]capabilitydomain.Deps, 0, len(agent.Spec.Capabilities))
	for i, att := range agent.Spec.Capabilities {
		attPath := capsPath.Index(i)
		key := types.NamespacedName{Name: att.Ref.Name, Namespace: att.Ref.Namespace}
		if key.Namespace == "" {
			key.Namespace = agent.Namespace
		}

		// Cross-namespace capability references are not supported yet:
		// per-namespace allow-listing is roadmap post-P1, and echoing a
		// foreign namespace's Capability internals (requires tuple, schema,
		// dependency pins) through admission errors would be a cross-tenant
		// information-disclosure oracle. Reject with a generic message that
		// reveals nothing about the referenced resource.
		if key.Namespace != agent.Namespace {
			allErrs = append(allErrs, field.Invalid(attPath.Child("ref", "namespace"), att.Ref.Namespace,
				"cross-namespace Capability references are not supported yet; reference a Capability in the agent's namespace"))
			continue
		}

		if seen[key] {
			allErrs = append(allErrs, field.Duplicate(attPath.Child("ref"), key.String()))
			continue
		}
		seen[key] = true

		var capCR agentv1alpha1.Capability
		if err := v.capabilityReader().Get(ctx, key, &capCR); err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"referenced Capability %s not found — compatibility and config checks run at compile time once it exists", key))
			continue
		}

		// Entry-name uniqueness: two attachments (or an attachment and a
		// fragment-native entry) compiling to the same spec entry name make
		// the runner's capability registry raise at bootstrap.
		entryName := capabilitydomain.EntryName(capCR.Spec.Entrypoint, capCR.Spec.SerializationName)
		if owner, dup := entryOwners[entryName]; dup {
			allErrs = append(allErrs, field.Invalid(attPath, entryName, fmt.Sprintf(
				"capability %s compiles to spec entry %q, already claimed by %s; set spec.serializationName to disambiguate",
				key, entryName, owner)))
		} else {
			entryOwners[entryName] = key.String()
		}

		// Config must be a JSON object (matches the compiler's contract).
		if att.Config != nil && !isJSONObject(att.Config.Raw) {
			allErrs = append(allErrs, field.Invalid(attPath.Child("config"), string(att.Config.Raw),
				"capability config must be a JSON object"))
		}

		// Requires-tuple compatibility (runtime contract §5).
		req := capabilitydomain.Requires{
			Python:       capCR.Spec.Requires.Python,
			PydanticAI:   capCR.Spec.Requires.PydanticAI,
			FlokoaRunner: capCR.Spec.Requires.FlokoaRunner,
		}
		if err := capabilitydomain.CheckRequires(capCR.Name, req, runner); err != nil {
			allErrs = append(allErrs, field.Forbidden(attPath, err.Error()))
		}

		// Config-schema validation (permissive skips with a loud warning).
		if capCR.Spec.SchemaPolicy == agentv1alpha1.SchemaPolicyPermissive {
			warnings = append(warnings, fmt.Sprintf(
				"Capability %s has schemaPolicy: permissive — its config is not validated at admission", key))
		} else if capCR.Spec.ConfigSchema != nil {
			configRaw := []byte("{}")
			if att.Config != nil {
				configRaw = att.Config.Raw
			}
			if err := capabilitydomain.ValidateConfig(capCR.Spec.ConfigSchema.Raw, configRaw); err != nil {
				allErrs = append(allErrs, field.Invalid(attPath.Child("config"), string(configRaw),
					fmt.Sprintf("config rejected by Capability %s schema: %v", key, err)))
			}
		}

		// Key Deps by namespaced identity so two same-named CRs in different
		// namespaces (once cross-namespace lands) stay distinct.
		deps = append(deps, capabilitydomain.Deps{Name: key.String(), Pins: capCR.Spec.Dependencies})
	}

	// Dependency-conflict detection across attachments + the runner baseline.
	for _, msg := range capabilitydomain.DetectConflicts(deps, runner) {
		allErrs = append(allErrs, field.Forbidden(capsPath, msg))
	}

	return warnings, allErrs
}

// isJSONObject reports whether raw is a JSON object ({...}).
func isJSONObject(raw []byte) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil
}

// validateReferences checks that cross-resource references exist.
// These are returned as warnings (not errors) to avoid ordering issues during
// resource creation (fixes #94).
func (v *AgentCustomValidator) validateReferences(ctx context.Context, agent *agentv1alpha1.Agent) admission.Warnings {
	if v.Reader == nil {
		return nil
	}

	var warnings admission.Warnings

	refNamespace := func(ns string) string {
		if ns == "" {
			return agent.Namespace
		}
		return ns
	}

	if agent.Spec.ModelRef != nil {
		ns := refNamespace(agent.Spec.ModelRef.Namespace)
		model := &agentv1alpha1.Model{}
		if err := v.Reader.Get(ctx, types.NamespacedName{Name: agent.Spec.ModelRef.Name, Namespace: ns}, model); err != nil {
			warnings = append(warnings,
				fmt.Sprintf("referenced Model %s/%s not found — the agent will not reconcile until it exists", ns, agent.Spec.ModelRef.Name))
		}
	}

	for _, ref := range agent.Spec.InstructionRefs {
		ns := refNamespace(ref.Namespace)
		instr := &agentv1alpha1.Instruction{}
		if err := v.Reader.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, instr); err != nil {
			warnings = append(warnings,
				fmt.Sprintf("referenced Instruction %s/%s not found — the agent will not reconcile until it exists", ns, ref.Name))
		}
	}

	for _, ref := range agent.Spec.Tools {
		ns := refNamespace(ref.Namespace)
		at := &agentv1alpha1.AgentTool{}
		if err := v.Reader.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, at); err != nil {
			warnings = append(warnings,
				fmt.Sprintf("referenced AgentTool %s/%s not found — the agent will not reconcile until it exists", ns, ref.Name))
		}
	}

	return warnings
}

// aggregateFieldErrors converts a field.ErrorList into an API status error, or nil if empty.
func aggregateFieldErrors(kind, name string, allErrs field.ErrorList) error {
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: agentv1alpha1.GroupVersion.Group, Kind: kind},
		name,
		allErrs,
	)
}
