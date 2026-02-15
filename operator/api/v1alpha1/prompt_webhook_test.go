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

	corev1 "k8s.io/api/core/v1"
)

func TestValidatePrompt(t *testing.T) {
	strPtr := func(s string) *string { return &s }

	tests := []struct {
		name    string
		obj     *Prompt
		wantErr bool
	}{
		{
			name: "valid prompt with inline value",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{
						Value: strPtr("Hello {{ name }}, welcome to {{ company }}."),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid prompt with valueFrom",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{
						ValueFrom: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "my-prompts",
							},
							Key: "greeting.txt",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid prompt with variables",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{
						Value: strPtr("Hello {{ name }}"),
					},
					Variables: []PromptVariable{
						{Name: "name", Description: "User name", Required: true},
						{Name: "company", Default: "Acme"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid: no source specified",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid: both value and valueFrom specified",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{
						Value: strPtr("inline template"),
						ValueFrom: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "my-prompts",
							},
							Key: "greeting.txt",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid: empty value string",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{
						Value: strPtr(""),
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid: valueFrom missing name",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{
						ValueFrom: &corev1.ConfigMapKeySelector{
							Key: "greeting.txt",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid: valueFrom missing key",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{
						ValueFrom: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "my-prompts",
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid: duplicate variable names",
			obj: &Prompt{
				Spec: PromptSpec{
					Source: PromptSource{
						Value: strPtr("Hello {{ name }}"),
					},
					Variables: []PromptVariable{
						{Name: "name", Required: true},
						{Name: "name", Required: false},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePrompt(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePrompt() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
