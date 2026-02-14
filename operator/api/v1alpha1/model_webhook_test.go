/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"testing"

	"k8s.io/utils/ptr"
)

func TestValidateModel(t *testing.T) {
	tests := []struct {
		name    string
		obj     *Model
		wantErr bool
	}{
		{
			name: "valid model without parameters",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: ProviderRef{Name: "openai"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid model with openai parameters",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: ProviderRef{Name: "openai"},
					Parameters: &ModelParameters{
						Temperature: "0.7",
						OpenAI:      &OpenAIParameters{},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid model with anthropic thinking enabled",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: ProviderRef{Name: "anthropic"},
					Parameters: &ModelParameters{
						MaxTokens: ptr.To[int32](8000),
						Anthropic: &AnthropicParameters{
							Thinking: &AnthropicThinkingConfig{
								Type:         ThinkingTypeEnabled,
								BudgetTokens: ptr.To[int32](4000),
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid model with thinking disabled",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: ProviderRef{Name: "anthropic"},
					Parameters: &ModelParameters{
						Anthropic: &AnthropicParameters{
							Thinking: &AnthropicThinkingConfig{
								Type: ThinkingTypeDisabled,
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple provider-specific parameter blocks",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: ProviderRef{Name: "openai"},
					Parameters: &ModelParameters{
						OpenAI:    &OpenAIParameters{},
						Anthropic: &AnthropicParameters{},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "thinking enabled without budgetTokens",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: ProviderRef{Name: "anthropic"},
					Parameters: &ModelParameters{
						Anthropic: &AnthropicParameters{
							Thinking: &AnthropicThinkingConfig{
								Type: ThinkingTypeEnabled,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "budgetTokens >= maxTokens",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: ProviderRef{Name: "anthropic"},
					Parameters: &ModelParameters{
						MaxTokens: ptr.To[int32](4000),
						Anthropic: &AnthropicParameters{
							Thinking: &AnthropicThinkingConfig{
								Type:         ThinkingTypeEnabled,
								BudgetTokens: ptr.To[int32](4000),
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "budgetTokens > maxTokens",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: ProviderRef{Name: "anthropic"},
					Parameters: &ModelParameters{
						MaxTokens: ptr.To[int32](4000),
						Anthropic: &AnthropicParameters{
							Thinking: &AnthropicThinkingConfig{
								Type:         ThinkingTypeEnabled,
								BudgetTokens: ptr.To[int32](5000),
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "no parameters is valid",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: ProviderRef{Name: "openai"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty parameters is valid",
			obj: &Model{
				Spec: ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: ProviderRef{Name: "openai"},
					Parameters:  &ModelParameters{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModel(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateModel() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
