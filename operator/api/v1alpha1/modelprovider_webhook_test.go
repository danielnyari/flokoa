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
)

func TestValidateModelProvider(t *testing.T) {
	tests := []struct {
		name    string
		obj     *ModelProvider
		wantErr bool
	}{
		{
			name: "valid openai provider",
			obj: &ModelProvider{
				Spec: ModelProviderSpec{
					OpenAI: &OpenAIProviderSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "valid anthropic provider",
			obj: &ModelProvider{
				Spec: ModelProviderSpec{
					Anthropic: &AnthropicProviderSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "valid google provider",
			obj: &ModelProvider{
				Spec: ModelProviderSpec{
					Google: &GoogleProviderSpec{
						Project:  "my-project",
						Location: "us-central1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid bedrock provider",
			obj: &ModelProvider{
				Spec: ModelProviderSpec{
					Bedrock: &BedrockProviderSpec{
						Region: "us-east-1",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "no provider block set",
			obj: &ModelProvider{
				Spec: ModelProviderSpec{},
			},
			wantErr: true,
		},
		{
			name: "multiple provider blocks set",
			obj: &ModelProvider{
				Spec: ModelProviderSpec{
					OpenAI:    &OpenAIProviderSpec{},
					Anthropic: &AnthropicProviderSpec{},
				},
			},
			wantErr: true,
		},
		{
			name: "all provider blocks set",
			obj: &ModelProvider{
				Spec: ModelProviderSpec{
					OpenAI:    &OpenAIProviderSpec{},
					Anthropic: &AnthropicProviderSpec{},
					Google:    &GoogleProviderSpec{},
					Bedrock:   &BedrockProviderSpec{},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModelProvider(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateModelProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
