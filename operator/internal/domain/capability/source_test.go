package capability

import (
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestSourceAllowed(t *testing.T) {
	all := []agentv1alpha1.CapabilitySource{
		agentv1alpha1.CapabilitySourceBuiltin,
		agentv1alpha1.CapabilitySourceImage,
		agentv1alpha1.CapabilitySourceGit,
	}
	tests := []struct {
		name    string
		source  agentv1alpha1.CapabilitySource
		allowed []agentv1alpha1.CapabilitySource
		want    bool
	}{
		{"empty allowed admits everything", agentv1alpha1.CapabilitySourcePypi, nil, true},
		{"empty allowed admits empty source", "", nil, true},
		{"source in set", agentv1alpha1.CapabilitySourceImage, all, true},
		{"source not in set", agentv1alpha1.CapabilitySourcePypi, all, false},
		{"builtin in set", agentv1alpha1.CapabilitySourceBuiltin, all, true},
		{"git in set", agentv1alpha1.CapabilitySourceGit, all, true},
		{"empty source treated as image (in set)", "", all, true},
		{
			"empty source treated as image (not in set)",
			"",
			[]agentv1alpha1.CapabilitySource{agentv1alpha1.CapabilitySourceBuiltin, agentv1alpha1.CapabilitySourceGit},
			false,
		},
		{
			"single-element set matches",
			agentv1alpha1.CapabilitySourcePypi,
			[]agentv1alpha1.CapabilitySource{agentv1alpha1.CapabilitySourcePypi},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SourceAllowed(tt.source, tt.allowed); got != tt.want {
				t.Fatalf("SourceAllowed(%q, %v) = %v, want %v", tt.source, tt.allowed, got, tt.want)
			}
		})
	}
}
