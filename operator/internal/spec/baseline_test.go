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
