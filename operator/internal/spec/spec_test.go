package spec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadDoc(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return doc
}

func TestSupportedRunnerVersionsIncludesDefault(t *testing.T) {
	versions := SupportedRunnerVersions()
	for _, v := range versions {
		if v == DefaultRunnerVersion {
			return
		}
	}
	t.Fatalf("DefaultRunnerVersion %q not among embedded schemas %v", DefaultRunnerVersion, versions)
}

func TestLoadSchemaUnknownVersion(t *testing.T) {
	_, err := LoadSchema("99.99.99")
	if err == nil {
		t.Fatal("expected error for unknown runner version")
	}
	if !strings.Contains(err.Error(), "99.99.99") || !strings.Contains(err.Error(), DefaultRunnerVersion) {
		t.Fatalf("error should name the unknown version and list supported ones, got: %v", err)
	}
}

func TestSchemaDigestFormat(t *testing.T) {
	d, err := SchemaDigest(DefaultRunnerVersion)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d, "sha256:") || len(d) != len("sha256:")+64 {
		t.Fatalf("unexpected digest format: %q", d)
	}
}

func TestValidateGoldenDocuments(t *testing.T) {
	validFiles, err := filepath.Glob("testdata/valid/*.json")
	if err != nil || len(validFiles) == 0 {
		t.Fatalf("no valid fixtures found: %v", err)
	}
	for _, f := range validFiles {
		t.Run("valid/"+filepath.Base(f), func(t *testing.T) {
			if err := Validate(DefaultRunnerVersion, loadDoc(t, f)); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}

	invalid := []struct {
		file     string
		wantPath string
	}{
		{"testdata/invalid/unknown-field.json", "/"},
		{"testdata/invalid/bad-capability.json", "/capabilities"},
		{"testdata/invalid/bad-settings-type.json", "/model_settings/max_tokens"},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+filepath.Base(tc.file), func(t *testing.T) {
			err := Validate(DefaultRunnerVersion, loadDoc(t, tc.file))
			if err == nil {
				t.Fatal("expected validation error")
			}
			verr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected *ValidationError, got %T: %v", err, err)
			}
			if !strings.HasPrefix(verr.Path, tc.wantPath) {
				t.Fatalf("expected error path under %q, got %q (%v)", tc.wantPath, verr.Path, verr)
			}
		})
	}
}

func TestValidateRejectsHarnessStyleCapabilityNames(t *testing.T) {
	// Harness/third-party capabilities must come through Capability CRs (brief §4);
	// dotted class paths are not in the baseline schema registry.
	doc := map[string]any{
		"model": "openai:gpt-5-mini",
		"capabilities": []any{
			map[string]any{"pydantic_ai_harness.shields:Shields": map[string]any{}},
		},
	}
	if err := Validate(DefaultRunnerVersion, doc); err == nil {
		t.Fatal("expected harness-style capability entry to fail baseline schema validation")
	}
}
