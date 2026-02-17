package model

import (
	"strings"
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestValidateProviderParams(t *testing.T) {
	tests := []struct {
		name         string
		params       *agentv1alpha1.ModelParameters
		providerType agentv1alpha1.ProviderType
		wantErr      string
	}{
		{
			name:         "nil params is valid",
			params:       nil,
			providerType: agentv1alpha1.ProviderTypeOpenAI,
			wantErr:      "",
		},
		{
			name:         "no provider-specific params is valid",
			params:       &agentv1alpha1.ModelParameters{},
			providerType: agentv1alpha1.ProviderTypeOpenAI,
			wantErr:      "",
		},
		{
			name: "matching provider params is valid",
			params: &agentv1alpha1.ModelParameters{
				OpenAI: &agentv1alpha1.OpenAIParameters{},
			},
			providerType: agentv1alpha1.ProviderTypeOpenAI,
			wantErr:      "",
		},
		{
			name: "mismatched provider params",
			params: &agentv1alpha1.ModelParameters{
				Anthropic: &agentv1alpha1.AnthropicParameters{},
			},
			providerType: agentv1alpha1.ProviderTypeOpenAI,
			wantErr:      "do not match the ModelProvider type",
		},
		{
			name: "multiple provider params set",
			params: &agentv1alpha1.ModelParameters{
				OpenAI:    &agentv1alpha1.OpenAIParameters{},
				Anthropic: &agentv1alpha1.AnthropicParameters{},
			},
			providerType: agentv1alpha1.ProviderTypeOpenAI,
			wantErr:      "only one provider-specific parameters block can be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderParams(tt.params, tt.providerType)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateProviderParams() error = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateProviderParams() error = nil, want containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ValidateProviderParams() error = %v, want containing %q", err, tt.wantErr)
				}
			}
		})
	}
}
