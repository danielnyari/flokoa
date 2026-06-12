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
