package modelprovider

import (
	"strings"
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestValidateProvider(t *testing.T) {
	tests := []struct {
		name     string
		mp       *agentv1alpha1.ModelProvider
		wantType agentv1alpha1.ProviderType
		wantErr  string
	}{
		{
			name: "valid openai",
			mp: &agentv1alpha1.ModelProvider{
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			},
			wantType: agentv1alpha1.ProviderTypeOpenAI,
			wantErr:  "",
		},
		{
			name: "valid anthropic",
			mp: &agentv1alpha1.ModelProvider{
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
				},
			},
			wantType: agentv1alpha1.ProviderTypeAnthropic,
			wantErr:  "",
		},
		{
			name: "valid google",
			mp: &agentv1alpha1.ModelProvider{
				Spec: agentv1alpha1.ModelProviderSpec{
					Google: &agentv1alpha1.GoogleProviderSpec{},
				},
			},
			wantType: agentv1alpha1.ProviderTypeGoogle,
			wantErr:  "",
		},
		{
			name: "valid bedrock",
			mp: &agentv1alpha1.ModelProvider{
				Spec: agentv1alpha1.ModelProviderSpec{
					Bedrock: &agentv1alpha1.BedrockProviderSpec{},
				},
			},
			wantType: agentv1alpha1.ProviderTypeBedrock,
			wantErr:  "",
		},
		{
			name: "no provider set",
			mp: &agentv1alpha1.ModelProvider{
				Spec: agentv1alpha1.ModelProviderSpec{},
			},
			wantType: "",
			wantErr:  "exactly one of openai, anthropic, google, or bedrock must be specified",
		},
		{
			name: "multiple providers set",
			mp: &agentv1alpha1.ModelProvider{
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI:    &agentv1alpha1.OpenAIProviderSpec{},
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
				},
			},
			wantType: "",
			wantErr:  "only one of openai, anthropic, google, or bedrock can be specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, err := ValidateProvider(tt.mp)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateProvider() error = %v, want nil", err)
				}
				if gotType != tt.wantType {
					t.Errorf("ValidateProvider() type = %v, want %v", gotType, tt.wantType)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateProvider() error = nil, want containing %q", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("ValidateProvider() error = %v, want containing %q", err, tt.wantErr)
				}
			}
		})
	}
}
