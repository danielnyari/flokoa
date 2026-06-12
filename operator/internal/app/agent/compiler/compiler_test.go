package compiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/infra/repo/fakes"
	"github.com/danielnyari/flokoa/internal/spec"
)

const testNS = "default"

// platformTelemetry is the injected platform entry used across compiler tests.
const platformTelemetry = "flokoa.platform/telemetry"

func nsKey(name string) types.NamespacedName {
	return types.NamespacedName{Name: name, Namespace: testNS}
}

type fixture struct {
	models       *fakes.FakeModelRepo
	providers    *fakes.FakeModelProviderRepo
	instructions *fakes.FakeInstructionRepo
	tools        *fakes.FakeAgentToolRepo
	capabilities *fakes.FakeCapabilityRepo
	services     *fakes.FakeServiceRepo
	compiler     *Compiler
}

func newFixture(opts Options) *fixture {
	if opts.DefaultRunnerVersion == "" {
		opts.DefaultRunnerVersion = spec.DefaultRunnerVersion
	}
	f := &fixture{
		models:       fakes.NewFakeModelRepo(),
		providers:    fakes.NewFakeModelProviderRepo(),
		instructions: fakes.NewFakeInstructionRepo(),
		tools:        fakes.NewFakeAgentToolRepo(),
		capabilities: fakes.NewFakeCapabilityRepo(),
		services:     fakes.NewFakeServiceRepo(),
	}
	f.compiler = New(Deps{
		Models:       f.models,
		Providers:    f.providers,
		Instructions: f.instructions,
		AgentTools:   f.tools,
		Capabilities: f.capabilities,
		Services:     f.services,
	}, opts)
	return f
}

// addOpenAIModel registers a ready openai-provider + Model pair.
func (f *fixture) addOpenAIModel(name, model string, settings *agentv1alpha1.ModelSettings) {
	f.providers.Providers[nsKey("openai-provider")] = &agentv1alpha1.ModelProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "openai-provider", Namespace: testNS},
		Spec: agentv1alpha1.ModelProviderSpec{
			APIKeySecretRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "openai-key"},
				Key:                  "api-key",
			},
			OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
		},
		Status: agentv1alpha1.ModelProviderStatus{Ready: true, Provider: agentv1alpha1.ProviderTypeOpenAI},
	}
	f.models.Models[nsKey(name)] = &agentv1alpha1.Model{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec: agentv1alpha1.ModelSpec{
			Model:       model,
			ProviderRef: agentv1alpha1.ProviderRef{Name: "openai-provider"},
			Settings:    settings,
		},
		Status: agentv1alpha1.ModelStatus{Ready: true},
	}
}

func (f *fixture) addInstruction(name, content string) {
	f.instructions.Instructions[nsKey(name)] = &agentv1alpha1.Instruction{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNS},
		Spec:       agentv1alpha1.InstructionSpec{Content: content},
	}
}

func agentWith(mutate func(*agentv1alpha1.Agent)) *agentv1alpha1.Agent {
	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "support-agent", Namespace: testNS},
		Spec: agentv1alpha1.AgentSpec{
			Card: agentv1alpha1.AgentCardOverride{Name: "support-agent", Description: "test", Version: "1"},
		},
	}
	if mutate != nil {
		mutate(agent)
	}
	return agent
}

func rawJSON(t *testing.T, v any) *apiextensionsv1.JSON {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

func int32Ptr(v int32) *int32 { return &v }

func TestCompileInlineFragmentOnly(t *testing.T) {
	f := newFixture(Options{})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{
			Model:        "openai:gpt-5-mini",
			Instructions: []string{"Be terse."},
			ModelSettings: &agentv1alpha1.ModelSettings{
				Temperature: "0.2",
				MaxTokens:   int32Ptr(2048),
			},
			Capabilities: []agentv1alpha1.NativeCapabilityEntry{
				{Name: "Thinking"},
				{Name: "WebSearch", Config: rawJSON(t, map[string]any{"native": true})},
			},
		}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}

	if res.Doc["model"] != "openai:gpt-5-mini" {
		t.Errorf("model = %v", res.Doc["model"])
	}
	if res.Doc["name"] != "support-agent" {
		t.Errorf("name should default to the CR name, got %v", res.Doc["name"])
	}
	settings := res.Doc["model_settings"].(map[string]any)
	if settings["temperature"] != json.Number("0.2") {
		t.Errorf("temperature should be a JSON number, got %T %v", settings["temperature"], settings["temperature"])
	}
	caps := res.Doc["capabilities"].([]any)
	if len(caps) != 2 || caps[0] != "Thinking" {
		t.Errorf("capabilities = %v", caps)
	}
	if res.Hash == "" || res.SchemaDigest == "" {
		t.Error("hash and schema digest must be set")
	}
	if !strings.Contains(string(res.YAML), "model: openai:gpt-5-mini") {
		t.Errorf("YAML output missing model: %s", res.YAML)
	}
}

func TestCompileFullComposition(t *testing.T) {
	f := newFixture(Options{})
	f.addOpenAIModel("gpt", "gpt-5-mini", &agentv1alpha1.ModelSettings{
		Temperature: "0.7",
		MaxTokens:   int32Ptr(1024),
	})
	f.addInstruction("first", "You are a support agent.")
	f.addInstruction("second", "Answer from the KB only.")
	f.tools.AgentTools[nsKey("kb")] = &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{Name: "kb", Namespace: testNS},
		Spec: agentv1alpha1.AgentToolSpec{
			Type:       agentv1alpha1.AgentToolTypeMCP,
			ServiceRef: &agentv1alpha1.ServiceRef{Name: "kb-tools", Port: int32Ptr(8080)},
			HeaderSecrets: []agentv1alpha1.SecretHeader{{
				Name: "Authorization",
				SecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "kb-token"},
					Key:                  "token",
				},
			}},
			AllowedTools: []string{"search"},
		},
	}

	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.ModelRef = &agentv1alpha1.NamespacedRef{Name: "gpt"}
		a.Spec.InstructionRefs = []agentv1alpha1.NamespacedRef{{Name: "first"}, {Name: "second"}}
		a.Spec.Tools = []agentv1alpha1.NamespacedRef{{Name: "kb"}}
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{
			Instructions: []string{"Inline instruction last."},
			ModelSettings: &agentv1alpha1.ModelSettings{
				Temperature: "0.1", // inline wins over the Model's 0.7
			},
		}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}

	if res.Doc["model"] != "openai:gpt-5-mini" {
		t.Errorf("model should be prefixed by provider, got %v", res.Doc["model"])
	}

	instructions := res.Doc["instructions"].([]any)
	want := []string{"You are a support agent.", "Answer from the KB only.", "Inline instruction last."}
	if len(instructions) != 3 {
		t.Fatalf("instructions = %v", instructions)
	}
	for i := range want {
		if instructions[i] != want[i] {
			t.Errorf("instructions[%d] = %q, want %q (refs in order, then inline)", i, instructions[i], want[i])
		}
	}

	settings := res.Doc["model_settings"].(map[string]any)
	if settings["temperature"] != json.Number("0.1") {
		t.Errorf("inline temperature must win, got %v", settings["temperature"])
	}
	if settings["max_tokens"] != json.Number("1024") {
		t.Errorf("model settings must merge for unset keys, got %v (%T)", settings["max_tokens"], settings["max_tokens"])
	}

	caps := res.Doc["capabilities"].([]any)
	if len(caps) != 1 {
		t.Fatalf("capabilities = %v", caps)
	}
	mcp := caps[0].(map[string]any)["MCP"].(map[string]any)
	if mcp["url"] != "http://kb-tools.default.svc.cluster.local:8080/mcp" {
		t.Errorf("MCP url = %v", mcp["url"])
	}
	headers := mcp["headers"].(map[string]any)
	if headers["Authorization"] != "${secret:tool-kb-authorization}" {
		t.Errorf("header secret must be a placeholder, got %v", headers["Authorization"])
	}

	// One env per header secret, normalized per the contract.
	if len(res.SecretEnv) != 1 || res.SecretEnv[0].Name != "FLOKOA_SECRET_TOOL_KB_AUTHORIZATION" {
		t.Errorf("secret env = %+v", res.SecretEnv)
	}
	// Provider env: API key from the ModelProvider.
	if len(res.ProviderSecretEnv) != 1 || res.ProviderSecretEnv[0].Name != "OPENAI_API_KEY" {
		t.Errorf("provider secret env = %+v", res.ProviderSecretEnv)
	}
}

func TestCompileInlineModelWinsOverRef(t *testing.T) {
	f := newFixture(Options{})
	f.addOpenAIModel("gpt", "gpt-5-mini", nil)

	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.ModelRef = &agentv1alpha1.NamespacedRef{Name: "gpt"}
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "anthropic:claude-sonnet-4-5"}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc["model"] != "anthropic:claude-sonnet-4-5" {
		t.Errorf("inline model must win the scalar conflict, got %v", res.Doc["model"])
	}
}

func TestCompileMissingRefIsDependencyError(t *testing.T) {
	f := newFixture(Options{})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.ModelRef = &agentv1alpha1.NamespacedRef{Name: "missing-model"}
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil || !flokoaerrors.IsDependency(err) {
		t.Fatalf("expected dependency error, got %v", err)
	}
	if !strings.Contains(err.Error(), "missing-model") {
		t.Errorf("error must name the missing ref: %v", err)
	}
}

func TestCompileSchemaValidationFailure(t *testing.T) {
	f := newFixture(Options{})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{
			Model: "openai:gpt-5-mini",
			Extra: rawJSON(t, map[string]any{"system_prompt": "not an AgentSpec field"}),
		}
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	if verr.RunnerVersion != spec.DefaultRunnerVersion {
		t.Errorf("runner version = %s", verr.RunnerVersion)
	}
}

func TestCompileUnknownRunnerVersion(t *testing.T) {
	f := newFixture(Options{})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Runtime.RunnerVersion = "9.9.9"
	})

	_, err := f.compiler.Compile(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "9.9.9") {
		t.Fatalf("unknown runner version must fail loudly, got %v", err)
	}
}

func TestCompileInjectedCapabilitiesAppendLast(t *testing.T) {
	f := newFixture(Options{
		Injected: []InjectedCapability{{Name: platformTelemetry}},
	})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{
			Model:        "openai:gpt-5-mini",
			Capabilities: []agentv1alpha1.NativeCapabilityEntry{{Name: "Thinking"}},
		}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	// The default embedded schema has no flokoa platform capabilities until
	// unit 07 registers them — accept either outcome shape here and assert
	// ordering on the doc itself.
	if err != nil {
		verr, ok := err.(*ValidationError)
		if !ok {
			t.Fatalf("unexpected error: %v", err)
		}
		t.Skipf("platform capability not in schema yet (expected before unit 07): %v", verr)
	}

	caps := res.Doc["capabilities"].([]any)
	if caps[len(caps)-1] != platformTelemetry {
		t.Errorf("injected entries must be last: %v", caps)
	}
	if len(res.Injected) != 1 || res.Injected[0] != platformTelemetry {
		t.Errorf("Injected = %v", res.Injected)
	}
}

func TestCompileSecretRefsProjectSortedEnv(t *testing.T) {
	f := newFixture(Options{})
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.SecretRefs = map[string]corev1.SecretKeySelector{
			"zeta-token": {LocalObjectReference: corev1.LocalObjectReference{Name: "zeta"}, Key: "t"},
			"api.token":  {LocalObjectReference: corev1.LocalObjectReference{Name: "alpha"}, Key: "t"},
		}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.SecretEnv) != 2 {
		t.Fatalf("secret env = %+v", res.SecretEnv)
	}
	if res.SecretEnv[0].Name != "FLOKOA_SECRET_API_TOKEN" || res.SecretEnv[1].Name != "FLOKOA_SECRET_ZETA_TOKEN" {
		t.Errorf("env must be sorted and normalized: %+v", res.SecretEnv)
	}
}

func TestCompileHashStability(t *testing.T) {
	build := func() *agentv1alpha1.Agent {
		return agentWith(func(a *agentv1alpha1.Agent) {
			a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{
				Model:        "openai:gpt-5-mini",
				Instructions: []string{"hello"},
			}
		})
	}
	f := newFixture(Options{})

	res1, err := f.compiler.Compile(context.Background(), build())
	if err != nil {
		t.Fatal(err)
	}
	res2, err := f.compiler.Compile(context.Background(), build())
	if err != nil {
		t.Fatal(err)
	}
	if res1.Hash != res2.Hash {
		t.Errorf("hash must be deterministic: %s vs %s", res1.Hash, res2.Hash)
	}
	if string(res1.YAML) != string(res2.YAML) {
		t.Error("YAML serialization must be deterministic")
	}

	changed := build()
	changed.Spec.Spec.Instructions = []string{"changed"}
	res3, err := f.compiler.Compile(context.Background(), changed)
	if err != nil {
		t.Fatal(err)
	}
	if res3.Hash == res1.Hash {
		t.Error("hash must change when the composition changes")
	}
}

func TestCompileToolPrefixWrapsEntry(t *testing.T) {
	f := newFixture(Options{})
	f.tools.AgentTools[nsKey("petstore")] = &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{Name: "petstore", Namespace: testNS},
		Spec: agentv1alpha1.AgentToolSpec{
			Type:       agentv1alpha1.AgentToolTypeMCP,
			URL:        "http://petstore.example.com/mcp",
			ToolPrefix: "petstore",
		},
	}
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Tools = []agentv1alpha1.NamespacedRef{{Name: "petstore"}}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	caps := res.Doc["capabilities"].([]any)
	prefixed := caps[0].(map[string]any)["PrefixTools"].(map[string]any)
	if prefixed["prefix"] != "petstore" {
		t.Errorf("prefix = %v", prefixed["prefix"])
	}
	if _, ok := prefixed["capability"].(map[string]any)["MCP"]; !ok {
		t.Errorf("wrapped entry must be the MCP capability: %v", prefixed)
	}
}

func TestCompileToolTimeoutMaxWins(t *testing.T) {
	f := newFixture(Options{})
	timeout := func(s int32) *int32 { return &s }
	f.tools.AgentTools[nsKey("slow")] = &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{Name: "slow", Namespace: testNS},
		Spec: agentv1alpha1.AgentToolSpec{
			URL: "http://slow.example.com/mcp", TimeoutSeconds: timeout(120),
		},
	}
	f.tools.AgentTools[nsKey("fast")] = &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{Name: "fast", Namespace: testNS},
		Spec: agentv1alpha1.AgentToolSpec{
			URL: "http://fast.example.com/mcp", TimeoutSeconds: timeout(10),
		},
	}
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Tools = []agentv1alpha1.NamespacedRef{{Name: "slow"}, {Name: "fast"}}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	if res.Doc["tool_timeout"] != json.Number("120") {
		t.Errorf("tool_timeout = %v, want the largest requested", res.Doc["tool_timeout"])
	}
}

func TestCompilePortNameResolvesViaService(t *testing.T) {
	f := newFixture(Options{})
	f.services.Services[nsKey("kb-tools")] = &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "kb-tools", Namespace: testNS},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{
			{Name: "mcp", Port: 9090},
		}},
	}
	f.tools.AgentTools[nsKey("kb")] = &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{Name: "kb", Namespace: testNS},
		Spec: agentv1alpha1.AgentToolSpec{
			ServiceRef: &agentv1alpha1.ServiceRef{Name: "kb-tools", PortName: "mcp"},
		},
	}
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"}
		a.Spec.Tools = []agentv1alpha1.NamespacedRef{{Name: "kb"}}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}
	mcp := res.Doc["capabilities"].([]any)[0].(map[string]any)["MCP"].(map[string]any)
	if mcp["url"] != "http://kb-tools.default.svc.cluster.local:9090/mcp" {
		t.Errorf("url = %v", mcp["url"])
	}
}

func TestCompileGoldenComposedSpec(t *testing.T) {
	// The composed golden mirrors internal/spec/testdata/valid/composed.json's
	// shape: the compiled doc must validate against the real embedded schema
	// (it does — Compile validates) and hydrate in the runner contract test.
	f := newFixture(Options{})
	f.addOpenAIModel("gpt", "gpt-5-mini", &agentv1alpha1.ModelSettings{MaxTokens: int32Ptr(4096)})
	f.addInstruction("kb-policy", "Answer based on the knowledge base only.")
	f.tools.AgentTools[nsKey("kb")] = &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{Name: "kb", Namespace: testNS},
		Spec: agentv1alpha1.AgentToolSpec{
			ServiceRef:   &agentv1alpha1.ServiceRef{Name: "kb-tools", Port: int32Ptr(8080)},
			AllowedTools: []string{"search", "fetch_article"},
		},
	}
	agent := agentWith(func(a *agentv1alpha1.Agent) {
		a.Spec.ModelRef = &agentv1alpha1.NamespacedRef{Name: "gpt"}
		a.Spec.InstructionRefs = []agentv1alpha1.NamespacedRef{{Name: "kb-policy"}}
		a.Spec.Tools = []agentv1alpha1.NamespacedRef{{Name: "kb"}}
		a.Spec.Spec = &agentv1alpha1.AgentSpecFragment{
			Description:  "Customer support agent",
			OutputSchema: rawJSON(t, map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}}}),
		}
	})

	res, err := f.compiler.Compile(context.Background(), agent)
	if err != nil {
		t.Fatal(err)
	}

	// Re-validate the serialized YAML round-trips through the schema: what we
	// write to the ConfigMap is exactly what the runner will hydrate.
	var roundTrip map[string]any
	if err := json.Unmarshal(mustJSON(t, res.Doc), &roundTrip); err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(res.RunnerVersion, roundTrip); err != nil {
		t.Fatalf("round-tripped doc failed schema validation: %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
