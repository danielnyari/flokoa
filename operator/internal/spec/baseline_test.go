package spec

import (
	"strings"
	"testing"
)

func TestRunnerBaseline(t *testing.T) {
	info, err := RunnerBaseline(DefaultRunnerVersion)
	if err != nil {
		t.Fatalf("RunnerBaseline(%s) error: %v", DefaultRunnerVersion, err)
	}

	if info.RunnerVersion != DefaultRunnerVersion {
		t.Errorf("RunnerVersion = %q, want %q", info.RunnerVersion, DefaultRunnerVersion)
	}
	if info.Python == "" {
		t.Error("Python is empty")
	}
	if info.PydanticAI == "" {
		t.Error("PydanticAI is empty")
	}
	if len(info.Baseline) == 0 {
		t.Fatal("Baseline is empty — expected the full lockfile closure")
	}

	// The headline baseline libraries of the runtime contract must be pinned.
	for _, pkg := range []string{"httpx", "starlette", "pydantic", "opentelemetry-sdk", "pydantic-ai"} {
		if info.Baseline[pkg] == "" {
			t.Errorf("Baseline[%q] missing", pkg)
		}
	}

	// pydantic-ai pin in the baseline must agree with the top-level field.
	if info.Baseline["pydantic-ai"] != info.PydanticAI {
		t.Errorf("Baseline[pydantic-ai] = %q, want %q", info.Baseline["pydantic-ai"], info.PydanticAI)
	}

	// The no-harness invariant (product brief §4) holds in the embedded copy.
	for pkg := range info.Baseline {
		if strings.Contains(pkg, "pydantic-ai-harness") {
			t.Errorf("baseline contains harness package %q", pkg)
		}
	}
}

func TestRunnerBaselineUnknownVersion(t *testing.T) {
	_, err := RunnerBaseline("9.9.9")
	if err == nil {
		t.Fatal("RunnerBaseline(9.9.9) = nil error, want unknown-version error")
	}
	if !strings.Contains(err.Error(), "9.9.9") || !strings.Contains(err.Error(), DefaultRunnerVersion) {
		t.Errorf("error %q should name the unknown version and the supported ones", err)
	}
}

func TestBuiltinCapabilityFlokoaOpenAPI(t *testing.T) {
	info, ok := BuiltinCapability(DefaultRunnerVersion, "flokoa-openapi")
	if !ok {
		t.Fatalf("BuiltinCapability(%s, flokoa-openapi) not found — run `make runner-contract`", DefaultRunnerVersion)
	}
	if info.Entrypoint != "flokoa_openapi.capability:OpenAPI" {
		t.Errorf("Entrypoint = %q, want flokoa_openapi.capability:OpenAPI", info.Entrypoint)
	}
	if info.SerializationName != "flokoa.OpenAPI" {
		t.Errorf("SerializationName = %q, want flokoa.OpenAPI", info.SerializationName)
	}
	// requires is pinned to the runner's own versions (the capability is the
	// runner): exact python minor, ==pydantic-ai pin, ==runner version.
	if info.Requires.Python == "" || info.Requires.PydanticAI == "" || info.Requires.FlokoaRunner == "" {
		t.Errorf("requires tuple incomplete: %+v", info.Requires)
	}
	if !strings.HasPrefix(info.Requires.FlokoaRunner, "==") {
		t.Errorf("Requires.FlokoaRunner = %q, want a == pin to the runner version", info.Requires.FlokoaRunner)
	}
	// Built-ins are baseline: no dependency closure (so no conflict against the
	// baseline they are part of).
	if len(info.Dependencies) != 0 {
		t.Errorf("Dependencies = %v, want empty for a built-in", info.Dependencies)
	}
	if !strings.HasPrefix(info.SchemaDigest, "sha256:") {
		t.Errorf("SchemaDigest = %q, want a sha256: digest", info.SchemaDigest)
	}
	if len(info.ConfigSchema) == 0 {
		t.Error("ConfigSchema is empty — admission needs it to validate attach-time config")
	}
}

func TestBuiltinCapabilityUnknownNameAndVersion(t *testing.T) {
	if _, ok := BuiltinCapability(DefaultRunnerVersion, "no-such-builtin"); ok {
		t.Error("BuiltinCapability returned ok for a name that is not a built-in")
	}
	if _, ok := BuiltinCapability("9.9.9", "flokoa-openapi"); ok {
		t.Error("BuiltinCapability returned ok for a runner version with no embedded baseline")
	}
}
