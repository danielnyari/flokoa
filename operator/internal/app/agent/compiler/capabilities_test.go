package compiler

import (
	"context"
	"errors"
	"strings"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/spec"
)

func errorsAs(err error, target **ValidationError) bool {
	return errors.As(err, target)
}

const kbConfigSchema = `{
	"type": "object",
	"required": ["endpoint"],
	"properties": {
		"endpoint": {"type": "string", "pattern": "^https://"},
		"maxResults": {"type": "integer"}
	},
	"additionalProperties": false
}`

func (f *fixture) addCapability(name string, mutate ...func(*agentv1alpha1.Capability)) {
	c := &agentv1alpha1.Capability{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec: agentv1alpha1.CapabilitySpec{
			Artifact:     "ghcr.io/danielnyari/capabilities/" + name + "@sha256:" + strings.Repeat("a", 64),
			Version:      "0.1.0",
			Entrypoint:   "flokoa_" + name + ".capability:KB",
			SchemaPolicy: agentv1alpha1.SchemaPolicyStrict,
			ConfigSchema: &apiextensionsv1.JSON{Raw: []byte(kbConfigSchema)},
			Requires: agentv1alpha1.CapabilityRequires{
				Python:       "3.13",
				PydanticAI:   ">=1.100,<2",
				FlokoaRunner: ">=0.2",
			},
		},
	}
	for _, m := range mutate {
		m(c)
	}
	f.capabilities.Capabilities[nsKey(name)] = c
}

func attachKB(t *testing.T, a *agentv1alpha1.Agent, name string) {
	t.Helper()
	a.Spec.Capabilities = append(a.Spec.Capabilities, agentv1alpha1.CapabilityAttachment{
		Ref:    agentv1alpha1.NamespacedRef{Name: name},
		Config: rawJSON(t, map[string]any{"endpoint": "https://kb.example.com"}),
	})
}

func TestCompileCapabilityAttachment(t *testing.T) {
	f := newFixture(Options{Injected: []InjectedCapability{{Name: platformTelemetry}}})
	f.addCapability("kb")
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{
			Model:        "openai:gpt-5-mini",
			Capabilities: []agentv1alpha1.NativeCapabilityEntry{{Name: "Thinking"}},
		}
		attachKB(t, a, "kb")
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}

	entries, _ := res.Doc["capabilities"].([]any)
	if len(entries) != 3 {
		t.Fatalf("capabilities = %v, want fragment + attachment + injected", entries)
	}
	// Order: fragment entries, then CR attachments, then injected last.
	if entries[0] != "Thinking" {
		t.Errorf("entries[0] = %v, want Thinking", entries[0])
	}
	kbEntry, ok := entries[1].(map[string]any)
	if !ok || kbEntry["KB"] == nil {
		t.Fatalf("entries[1] = %v, want {KB: config}", entries[1])
	}
	config := kbEntry["KB"].(map[string]any)
	if config["endpoint"] != "https://kb.example.com" {
		t.Errorf("attachment config = %v", config)
	}
	if entries[2] != platformTelemetry {
		t.Errorf("entries[2] = %v, want the injected platform entry last", entries[2])
	}

	if len(res.CapabilityArtifacts) != 1 {
		t.Fatalf("CapabilityArtifacts = %v, want one delivery input", res.CapabilityArtifacts)
	}
	art := res.CapabilityArtifacts[0]
	if art.Name != "kb" || !strings.HasSuffix(art.Artifact, "@sha256:"+strings.Repeat("a", 64)) || art.EntryName != "KB" {
		t.Errorf("artifact = %+v", art)
	}
}

func TestCompileCapabilityWithoutConfigUsesBareName(t *testing.T) {
	f := newFixture(Options{})
	f.addCapability("kb", func(c *agentv1alpha1.Capability) {
		c.Spec.SchemaPolicy = agentv1alpha1.SchemaPolicyPermissive
		c.Spec.ConfigSchema = nil
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{{Ref: agentv1alpha1.NamespacedRef{Name: "kb"}}}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := res.Doc["capabilities"].([]any)
	if len(entries) != 1 || entries[0] != "KB" {
		t.Fatalf("capabilities = %v, want the bare entry name", entries)
	}
}

func TestCompileCapabilitySerializationNameOverride(t *testing.T) {
	f := newFixture(Options{})
	f.addCapability("kb", func(c *agentv1alpha1.Capability) {
		c.Spec.SerializationName = "FlokoaKB"
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := res.Doc["capabilities"].([]any)
	entry, ok := entries[0].(map[string]any)
	if !ok || entry["FlokoaKB"] == nil {
		t.Fatalf("capabilities = %v, want the serializationName override as the entry key", entries)
	}
}

func TestCompileEntryNameCollisionIsPermanent(t *testing.T) {
	f := newFixture(Options{})
	// Both default to entry name "KB" (fixture entrypoint flokoa_<name>:KB).
	f.addCapability("kb-a")
	f.addCapability("kb-b")
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb-a")
		attachKB(t, a, "kb-b")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("colliding entry names must be permanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "spec entry \"KB\"") {
		t.Errorf("error %q should name the colliding entry", err)
	}
}

func TestCompileCapabilityUnknownRunnerVersion(t *testing.T) {
	f := newFixture(Options{})
	f.addCapability("kb")
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Runtime.RunnerVersion = "9.9.9"
		attachKB(t, a, "kb")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	var verr *ValidationError
	if !errorsAs(err, &verr) {
		t.Fatalf("expected *ValidationError for an unknown runner version, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "9.9.9") {
		t.Errorf("error %q should name the unknown runner version", err)
	}
}

func TestCompileCrossNamespaceCapabilityRejected(t *testing.T) {
	f := newFixture(Options{})
	f.addCapability("kb")
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{{
			Ref: agentv1alpha1.NamespacedRef{Name: "kb", Namespace: "other"},
		}}
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("cross-namespace capability ref must be permanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "cross-namespace") {
		t.Errorf("error %q should explain the cross-namespace restriction", err)
	}
}

func TestCompileMissingCapabilityIsDependencyError(t *testing.T) {
	f := newFixture(Options{})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "ghost")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if !flokoaerrors.IsDependency(err) {
		t.Fatalf("missing Capability must be a dependency error (requeue), got %v", err)
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error %q should name the missing Capability", err)
	}
}

func TestCompileIncompatibleCapabilityIsPermanent(t *testing.T) {
	f := newFixture(Options{})
	f.addCapability("kb", func(c *agentv1alpha1.Capability) {
		c.Spec.Requires.PydanticAI = ">=2,<3"
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("incompatible requires must be permanent (SpecValid=False), got %v", err)
	}
	for _, want := range []string{">=2,<3", "pydantic-ai"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name both tuples (%q)", err, want)
		}
	}
}

func TestCompileConflictingCapabilitiesArePermanent(t *testing.T) {
	f := newFixture(Options{})
	f.addCapability("shields", func(c *agentv1alpha1.Capability) {
		c.Spec.SerializationName = "Shields"
		c.Spec.Dependencies = []string{"pydantic-ai-harness==0.2.1"}
	})
	f.addCapability("planning", func(c *agentv1alpha1.Capability) {
		c.Spec.SerializationName = "Planning"
		c.Spec.Dependencies = []string{"pydantic-ai-harness==0.3.0"}
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "shields")
		attachKB(t, a, "planning")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("conflicting pins must be permanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "conflicting versions of pydantic-ai-harness") {
		t.Errorf("error %q should name the conflict", err)
	}
}

func TestCompileCapabilityConfigViolationIsPermanent(t *testing.T) {
	f := newFixture(Options{})
	f.addCapability("kb")
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{{
			Ref:    agentv1alpha1.NamespacedRef{Name: "kb"},
			Config: rawJSON(t, map[string]any{"endpoint": "https://kb.example.com", "maxResults": "five"}),
		}}
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("schema-violating config must be permanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "maxResults") {
		t.Errorf("error %q should point at the offending property", err)
	}
}

func TestCompileStrictCapabilityWithoutSchemaIsPermanent(t *testing.T) {
	// Defense in depth: a strict Capability missing its configSchema (possible
	// when webhooks were disabled) must fail compile, not silently skip
	// validation.
	f := newFixture(Options{})
	f.addCapability("kb", func(c *agentv1alpha1.Capability) {
		c.Spec.ConfigSchema = nil // strict (default) + no schema
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("strict capability without configSchema must be permanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "configSchema") {
		t.Errorf("error %q should name the missing configSchema", err)
	}
}

func TestCompileTwoCompatibleCapabilities(t *testing.T) {
	f := newFixture(Options{})
	f.addCapability("kb")
	f.addCapability("search", func(c *agentv1alpha1.Capability) {
		c.Spec.Entrypoint = "flokoa_search.capability:Search"
		c.Spec.Dependencies = []string{"left-pad==1.0.0"}
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
		attachKB(t, a, "search")
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := res.Doc["capabilities"].([]any)
	if len(entries) != 2 {
		t.Fatalf("capabilities = %v, want both attachments in declaration order", entries)
	}
	if len(res.CapabilityArtifacts) != 2 {
		t.Fatalf("CapabilityArtifacts = %v, want both delivery inputs", res.CapabilityArtifacts)
	}
	// Deterministic hash: compiling again yields the same hash.
	res2, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	if res.Hash != res2.Hash {
		t.Errorf("hash not deterministic: %s vs %s", res.Hash, res2.Hash)
	}
}

// --- requireVerified compile-time policy (roadmap 09) ---

// stampVerified mutates a Capability with a Verified condition.
func stampVerified(status metav1.ConditionStatus, reason, message string) func(*agentv1alpha1.Capability) {
	return func(c *agentv1alpha1.Capability) {
		c.Status.Conditions = []metav1.Condition{{
			Type:               agentv1alpha1.CapabilityConditionVerified,
			Status:             status,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
		}}
	}
}

func TestCompileRequireVerifiedAdmitsVerifiedCapability(t *testing.T) {
	f := newFixture(Options{RequireVerified: true})
	f.addCapability("kb", stampVerified(metav1.ConditionTrue,
		agentv1alpha1.CapabilityVerifiedReasonVerified, "cosign signature verified"))
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	if _, err := f.compiler.Compile(context.Background(), agent); err != nil {
		t.Fatalf("a verified capability must compile under requireVerified, got %v", err)
	}
}

func TestCompileRequireVerifiedBlocksUnverifiedCapability(t *testing.T) {
	f := newFixture(Options{RequireVerified: true})
	f.addCapability("kb", stampVerified(metav1.ConditionFalse,
		agentv1alpha1.CapabilityVerifiedReasonInvalid, "signature did not verify"))
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil {
		t.Fatal("an unverified capability must not compile under requireVerified")
	}
	// Dependency (requeue) error, not permanent: a signature published later
	// flips the condition without any Agent edit.
	if !flokoaerrors.IsDependency(err) {
		t.Fatalf("requireVerified failures must be dependency errors, got %v", err)
	}
	for _, want := range []string{"is not verified", "Verified=False", "reason SignatureInvalid", "requireVerified"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
}

func TestCompileRequireVerifiedInFlightIsRetryable(t *testing.T) {
	// Transient nuance (§4.6): Unknown/VerifyError must read as in-flight,
	// never as invalid, and must requeue.
	f := newFixture(Options{RequireVerified: true})
	f.addCapability("kb", stampVerified(metav1.ConditionUnknown,
		agentv1alpha1.CapabilityVerifiedReasonError, "registry unavailable"))
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil || !flokoaerrors.IsDependency(err) {
		t.Fatalf("in-flight verification must be a dependency error, got %v", err)
	}
	if !strings.Contains(err.Error(), "verification is in flight") {
		t.Fatalf("error %q should read as in-flight", err.Error())
	}
	if strings.Contains(err.Error(), "is not verified") {
		t.Fatalf("an in-flight error must not read as a failed verification: %q", err.Error())
	}
}

func TestCompileRequireVerifiedNoConditionIsRetryable(t *testing.T) {
	f := newFixture(Options{RequireVerified: true})
	f.addCapability("kb") // controller has not stamped Verified yet
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil || !flokoaerrors.IsDependency(err) {
		t.Fatalf("a missing Verified condition must be a dependency error, got %v", err)
	}
	if !strings.Contains(err.Error(), "verification is in flight") {
		t.Fatalf("error %q should read as in-flight", err.Error())
	}
}

func TestCompileWithoutRequireVerifiedIgnoresCondition(t *testing.T) {
	// Policy off (the default): the Verified condition does not gate
	// compilation.
	f := newFixture(Options{})
	f.addCapability("kb", stampVerified(metav1.ConditionFalse,
		agentv1alpha1.CapabilityVerifiedReasonMissing, "no cosign signature found"))
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	if _, err := f.compiler.Compile(context.Background(), agent); err != nil {
		t.Fatalf("without requireVerified the condition must not gate compilation, got %v", err)
	}
}

// --- allowedSources compile-time policy (source tiers PR1) ---

// stampSource mutates a Capability's source tier.
func stampSource(source agentv1alpha1.CapabilitySource) func(*agentv1alpha1.Capability) {
	return func(c *agentv1alpha1.Capability) { c.Spec.Source = source }
}

func TestCompileAllowedSourcesBlocksDisallowedTier(t *testing.T) {
	// A disallowed source is a settled policy decision: a permanent error
	// (SpecValid=False), not a dependency requeue.
	f := newFixture(Options{AllowedSources: []agentv1alpha1.CapabilitySource{
		agentv1alpha1.CapabilitySourceBuiltin,
		agentv1alpha1.CapabilitySourceImage,
		agentv1alpha1.CapabilitySourceGit,
	}})
	f.addCapability("kb", stampSource(agentv1alpha1.CapabilitySourcePypi))
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil {
		t.Fatal("a disallowed source must not compile")
	}
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("allowedSources failures must be permanent errors, got %v", err)
	}
	for _, want := range []string{`has source "pypi"`, "this cluster does not allow", "allowedSources="} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
}

func TestCompileAllowedSourcesAdmitsAllowedTier(t *testing.T) {
	f := newFixture(Options{AllowedSources: []agentv1alpha1.CapabilitySource{
		agentv1alpha1.CapabilitySourceImage,
	}})
	f.addCapability("kb", stampSource(agentv1alpha1.CapabilitySourceImage))
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	if _, err := f.compiler.Compile(context.Background(), agent); err != nil {
		t.Fatalf("an allowed source must compile, got %v", err)
	}
}

func TestCompileEmptyAllowedSourcesAdmitsAll(t *testing.T) {
	// Empty allowedSources = no restriction: a pypi capability compiles.
	f := newFixture(Options{})
	f.addCapability("kb", stampSource(agentv1alpha1.CapabilitySourcePypi))
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	if _, err := f.compiler.Compile(context.Background(), agent); err != nil {
		t.Fatalf("empty allowedSources must admit every tier, got %v", err)
	}
}

// --- built-in tier (source tiers PR2) -------------------------------------

// builtinOpenAPIName is the name of the flokoa-openapi built-in capability the
// runner baseline embeds; the compiler validates a source: builtin CR against
// that embedded metadata.
const builtinOpenAPIName = "flokoa-openapi"

// addBuiltinOpenAPI registers a source: builtin Capability CR that matches the
// operator's embedded built-in metadata for the default runner version (the
// shape the chart ships). Optional mutators can corrupt it to test mismatch
// rejection.
func (f *fixture) addBuiltinOpenAPI(mutate ...func(*agentv1alpha1.Capability)) {
	info, ok := spec.BuiltinCapability(spec.DefaultRunnerVersion, builtinOpenAPIName)
	if !ok {
		panic("embedded baseline is missing the flokoa-openapi built-in; run `make runner-contract`")
	}
	c := &agentv1alpha1.Capability{
		ObjectMeta: metav1.ObjectMeta{Name: builtinOpenAPIName, Namespace: testNS},
		Spec: agentv1alpha1.CapabilitySpec{
			Source:            agentv1alpha1.CapabilitySourceBuiltin,
			Version:           spec.DefaultRunnerVersion,
			Entrypoint:        info.Entrypoint,
			SerializationName: info.SerializationName,
			SchemaPolicy:      agentv1alpha1.SchemaPolicyStrict,
			ConfigSchema:      &apiextensionsv1.JSON{Raw: info.ConfigSchema},
			Requires: agentv1alpha1.CapabilityRequires{
				Python:       info.Requires.Python,
				PydanticAI:   info.Requires.PydanticAI,
				FlokoaRunner: info.Requires.FlokoaRunner,
			},
		},
	}
	for _, m := range mutate {
		m(c)
	}
	f.capabilities.Capabilities[nsKey(builtinOpenAPIName)] = c
}

func TestCompileBuiltinAttachmentEmitsEntryButNoArtifact(t *testing.T) {
	f := newFixture(Options{})
	f.addBuiltinOpenAPI()
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{
			{Ref: agentv1alpha1.NamespacedRef{Name: builtinOpenAPIName}},
		}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}

	// The built-in appears in the compiled spec by its serialization name so
	// the runner hydrates it — same spec shape as a delivered capability.
	entries, _ := res.Doc["capabilities"].([]any)
	if len(entries) != 1 || entries[0] != "flokoa.OpenAPI" {
		t.Fatalf("capabilities = %v, want the built-in serialization entry", entries)
	}

	// The crux of the built-in tier: nothing is delivered. Zero artifacts means
	// the builder emits zero initContainers, zero volumes, zero mounts.
	if len(res.CapabilityArtifacts) != 0 {
		t.Fatalf("CapabilityArtifacts = %v, want zero (built-ins are baked into the runner image)", res.CapabilityArtifacts)
	}
}

func TestCompileBuiltinWithConfigEmitsConfiguredEntry(t *testing.T) {
	f := newFixture(Options{})
	f.addBuiltinOpenAPI()
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{{
			Ref:    agentv1alpha1.NamespacedRef{Name: builtinOpenAPIName},
			Config: rawJSON(t, map[string]any{"base_url": "https://api.example.com"}),
		}}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := res.Doc["capabilities"].([]any)
	entry, ok := entries[0].(map[string]any)
	if !ok || entry["flokoa.OpenAPI"] == nil {
		t.Fatalf("entries[0] = %v, want {flokoa.OpenAPI: config}", entries[0])
	}
	cfg := entry["flokoa.OpenAPI"].(map[string]any)
	if cfg["base_url"] != "https://api.example.com" {
		t.Errorf("built-in attachment config = %v", cfg)
	}
	if len(res.CapabilityArtifacts) != 0 {
		t.Fatalf("CapabilityArtifacts = %v, want zero", res.CapabilityArtifacts)
	}
}

func TestCompileBuiltinUnknownNameIsPermanent(t *testing.T) {
	// A source: builtin CR whose name is not a real built-in for the runner is
	// a permanent error (SpecValid=False) — there is no "trust the CR" path.
	f := newFixture(Options{})
	f.addCapability("not-a-builtin", func(c *agentv1alpha1.Capability) {
		c.Spec.Source = agentv1alpha1.CapabilitySourceBuiltin
		c.Spec.Artifact = ""
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "not-a-builtin")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil {
		t.Fatal("a fake built-in must not compile")
	}
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("unknown built-in must be a permanent error, got %v", err)
	}
	if !strings.Contains(err.Error(), "not a built-in capability") {
		t.Errorf("error %q should explain the name is not a built-in", err.Error())
	}
}

func TestCompileBuiltinEntrypointMismatchIsPermanent(t *testing.T) {
	// A real built-in name but a tampered entrypoint must be rejected: the CR
	// must match the embedded metadata exactly.
	f := newFixture(Options{})
	f.addBuiltinOpenAPI(func(c *agentv1alpha1.Capability) {
		c.Spec.Entrypoint = "evil_module.capability:Backdoor"
		c.Spec.SerializationName = "flokoa.OpenAPI"
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{
			{Ref: agentv1alpha1.NamespacedRef{Name: builtinOpenAPIName}},
		}
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil {
		t.Fatal("a built-in CR with a mismatched entrypoint must not compile")
	}
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("built-in entrypoint mismatch must be permanent, got %v", err)
	}
	if !strings.Contains(err.Error(), "entrypoint") {
		t.Errorf("error %q should name the entrypoint mismatch", err.Error())
	}
}

func TestCompileBuiltinOmittedSchemaIsPermanent(t *testing.T) {
	// A real built-in name + entrypoint, but the CR omits configSchema while the
	// embedded metadata HAS one. The compiler must reject it (not skip the
	// schema match) — the same dual-gate the webhook enforces, so a schema-less
	// CR can't dodge the comparison after admission (or with webhooks disabled).
	f := newFixture(Options{})
	f.addBuiltinOpenAPI(func(c *agentv1alpha1.Capability) {
		c.Spec.ConfigSchema = nil
		c.Spec.SchemaPolicy = agentv1alpha1.SchemaPolicyPermissive
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{
			{Ref: agentv1alpha1.NamespacedRef{Name: builtinOpenAPIName}},
		}
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil {
		t.Fatal("a built-in CR omitting configSchema (while the built-in has one) must not compile")
	}
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("built-in omitted-schema must be a permanent error, got %v", err)
	}
	if !strings.Contains(err.Error(), "the CR omits it") {
		t.Errorf("error %q should explain the CR omitted the configSchema", err.Error())
	}
}

// --- built-in tier × requireVerified (source tiers PR2 + PR1 interaction) ---

func TestCompileBuiltinWithVerifiedTrueBuiltInPassesRequireVerified(t *testing.T) {
	// Architecture §8.3: built-ins are the most trusted tier and must work on
	// requireVerified clusters. The controller stamps Verified=True/BuiltIn on
	// a builtin CR; the compiler's requireVerified gate checks the condition
	// value (True), not the reason, so a BuiltIn-reason condition must compile.
	f := newFixture(Options{RequireVerified: true})
	f.addBuiltinOpenAPI(stampVerified(
		metav1.ConditionTrue,
		agentv1alpha1.CapabilityVerifiedReasonBuiltIn,
		"built into the runner image; integrity bound by the runner image digest",
	))
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{
			{Ref: agentv1alpha1.NamespacedRef{Name: builtinOpenAPIName}},
		}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatalf("builtin capability with Verified=True/BuiltIn must compile under requireVerified, got %v", err)
	}
	if len(res.CapabilityArtifacts) != 0 {
		t.Fatalf("built-in must still produce zero artifacts even under requireVerified, got %v", res.CapabilityArtifacts)
	}
}

func TestCompileBuiltinWithNoVerifiedConditionIsRetryableUnderRequireVerified(t *testing.T) {
	// Before the CapabilityReconciler has run (initial deployment window), a
	// builtin CR has no Verified condition yet. Under requireVerified this is a
	// dependency error — the same timing window as any other capability — so the
	// Agent requeues until the controller stamps Verified=True/BuiltIn.
	f := newFixture(Options{RequireVerified: true})
	f.addBuiltinOpenAPI() // no Verified condition set
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{
			{Ref: agentv1alpha1.NamespacedRef{Name: builtinOpenAPIName}},
		}
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil {
		t.Fatal("builtin with no Verified condition must be a dependency error under requireVerified")
	}
	if !flokoaerrors.IsDependency(err) {
		t.Fatalf("initial-window builtin requireVerified failure must be dependency (requeue), got %v", err)
	}
	// The message must read as "in flight", never as "not verified" — the
	// builtin isn't unverified, the controller just hasn't run yet.
	if !strings.Contains(err.Error(), "verification is in flight") {
		t.Errorf("error %q should read as in-flight, not as a permanent failure", err.Error())
	}
}

// --- allowedSources: legacy CR with empty source (pre-source-field) ---

func TestCompileAllowedSourcesLegacyCREmptySourceTreatedAsImage(t *testing.T) {
	// A Capability CR written before the source field was added has an empty
	// spec.source. SourceAllowed treats empty as image (the CRD default). If
	// the cluster's allowedSources excludes image, such a legacy CR is denied —
	// the same behavior as an explicit source: image CR.
	f := newFixture(Options{AllowedSources: []agentv1alpha1.CapabilitySource{
		agentv1alpha1.CapabilitySourceBuiltin,
		agentv1alpha1.CapabilitySourceGit,
		// image deliberately excluded
	}})
	f.addCapability("kb") // addCapability does not set source; it stays the zero value ""
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		attachKB(t, a, "kb")
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil {
		t.Fatal("a legacy CR with no source (treated as image) must be blocked when image is not in allowedSources")
	}
	if !flokoaerrors.IsPermanent(err) {
		t.Fatalf("allowedSources denial for a legacy CR must be permanent, got %v", err)
	}
	// The error names the empty source as-is (the caller sees the raw value)
	// so operators can correlate with the SourceAllowed rule.
	if !strings.Contains(err.Error(), "this cluster does not allow") {
		t.Errorf("error %q should explain the allowedSources denial", err.Error())
	}
}
