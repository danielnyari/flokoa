package spec

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSecretEnvNameGoldenPairs(t *testing.T) {
	raw, err := os.ReadFile("testdata/secret-env-pairs.json")
	if err != nil {
		t.Fatal(err)
	}
	var pairs map[string]string
	if err := json.Unmarshal(raw, &pairs); err != nil {
		t.Fatal(err)
	}
	if len(pairs) == 0 {
		t.Fatal("no golden pairs")
	}
	for name, want := range pairs {
		if got := SecretEnvName(name); got != want {
			t.Errorf("SecretEnvName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestPlaceholderPattern(t *testing.T) {
	matches := PlaceholderPattern.FindAllStringSubmatch(
		`{"headers": {"Authorization": "${secret:api-token}", "X": "${secret:db.dsn}"}}`, -1)
	if len(matches) != 2 || matches[0][1] != "api-token" || matches[1][1] != "db.dsn" {
		t.Fatalf("unexpected matches: %v", matches)
	}
}
