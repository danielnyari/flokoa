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

func TestValidateAgent(t *testing.T) {
	validStandardAgent := func() *Agent {
		return &Agent{
			Spec: AgentSpec{
				CardOverride: AgentCardOverride{
					Name:        "test-agent",
					Description: "A test agent",
					Version:     "1.0.0",
					Skills: []AgentSkill{
						{ID: "skill-1", Name: "Skill 1", Description: "First skill", Tags: []string{"test"}},
					},
				},
				Runtime: RuntimeSpec{
					Type: RuntimeTypeStandard,
					Standard: &StandardRuntimeSpec{
						Container: corev1.Container{
							Name:  "agent",
							Image: "my-agent:latest",
						},
					},
				},
			},
		}
	}

	tests := []struct {
		name    string
		obj     *Agent
		wantErr bool
	}{
		{
			name:    "valid standard agent",
			obj:     validStandardAgent(),
			wantErr: false,
		},
		{
			name: "valid template agent",
			obj: &Agent{
				Spec: AgentSpec{
					CardOverride: AgentCardOverride{
						Name:        "test-agent",
						Description: "A test agent",
						Version:     "1.0.0",
					},
					Runtime: RuntimeSpec{
						Type: RuntimeTypeTemplate,
						Template: &TemplatedRuntimeSpec{
							Config: &TemplatedAgentConfig{},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "standard type without standard config",
			obj: &Agent{
				Spec: AgentSpec{
					CardOverride: AgentCardOverride{
						Name: "test", Description: "test", Version: "1.0",
					},
					Runtime: RuntimeSpec{
						Type: RuntimeTypeStandard,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "template type without template config",
			obj: &Agent{
				Spec: AgentSpec{
					CardOverride: AgentCardOverride{
						Name: "test", Description: "test", Version: "1.0",
					},
					Runtime: RuntimeSpec{
						Type: RuntimeTypeTemplate,
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid inline instruction",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Instruction = &InstructionEntry{
					Template: "You are a helpful assistant.",
				}
				return a
			}(),
			wantErr: false,
		},
		{
			name: "valid instruction ref",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Instruction = &InstructionEntry{
					InstructionRef: &NamespacedRef{Name: "my-instruction"},
				}
				return a
			}(),
			wantErr: false,
		},
		{
			name: "instruction with both template and ref",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Instruction = &InstructionEntry{
					Template:       "inline prompt",
					InstructionRef: &NamespacedRef{Name: "my-instruction"},
				}
				return a
			}(),
			wantErr: true,
		},
		{
			name: "instruction with neither template nor ref",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Instruction = &InstructionEntry{}
				return a
			}(),
			wantErr: true,
		},
		{
			name: "valid tool with toolRef",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Tools = []ToolEntry{
					{ToolRef: &ToolRef{Name: "my-tool"}},
				}
				return a
			}(),
			wantErr: false,
		},
		{
			name: "valid tool with template and name",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Tools = []ToolEntry{
					{
						Name: "inline-tool",
						Template: &AgentToolSpec{
							Type:        AgentToolTypeOpenAPI,
							Description: "A tool",
						},
					},
				}
				return a
			}(),
			wantErr: false,
		},
		{
			name: "tool with both template and ref",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Tools = []ToolEntry{
					{
						Name:     "my-tool",
						Template: &AgentToolSpec{Type: AgentToolTypeOpenAPI, Description: "A tool"},
						ToolRef:  &ToolRef{Name: "other-tool"},
					},
				}
				return a
			}(),
			wantErr: true,
		},
		{
			name: "tool with neither template nor ref",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Tools = []ToolEntry{
					{Name: "orphan-tool"},
				}
				return a
			}(),
			wantErr: true,
		},
		{
			name: "tool template without name",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.Tools = []ToolEntry{
					{
						Template: &AgentToolSpec{
							Type:        AgentToolTypeOpenAPI,
							Description: "A tool",
						},
					},
				}
				return a
			}(),
			wantErr: true,
		},
		{
			name: "duplicate skill IDs",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.CardOverride.Skills = []AgentSkill{
					{ID: "skill-1", Name: "Skill 1", Description: "First", Tags: []string{"a"}},
					{ID: "skill-1", Name: "Skill 2", Description: "Duplicate", Tags: []string{"b"}},
				}
				return a
			}(),
			wantErr: true,
		},
		{
			name: "unique skill IDs",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.CardOverride.Skills = []AgentSkill{
					{ID: "skill-1", Name: "Skill 1", Description: "First", Tags: []string{"a"}},
					{ID: "skill-2", Name: "Skill 2", Description: "Second", Tags: []string{"b"}},
				}
				return a
			}(),
			wantErr: false,
		},
		{
			name: "no skills is valid",
			obj: func() *Agent {
				a := validStandardAgent()
				a.Spec.CardOverride.Skills = nil
				return a
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAgent(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAgent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCollectAgentWarnings(t *testing.T) {
	tests := []struct {
		name         string
		obj          *Agent
		wantWarnings int
	}{
		{
			name: "no warnings for clean standard agent",
			obj: &Agent{
				Spec: AgentSpec{
					Runtime: RuntimeSpec{
						Type: RuntimeTypeStandard,
						Standard: &StandardRuntimeSpec{
							Container: corev1.Container{Name: "agent", Image: "img:latest"},
						},
					},
				},
			},
			wantWarnings: 0,
		},
		{
			name: "warn when standard type has template set",
			obj: &Agent{
				Spec: AgentSpec{
					Runtime: RuntimeSpec{
						Type: RuntimeTypeStandard,
						Standard: &StandardRuntimeSpec{
							Container: corev1.Container{Name: "agent", Image: "img:latest"},
						},
						Template: &TemplatedRuntimeSpec{Config: &TemplatedAgentConfig{}},
					},
				},
			},
			wantWarnings: 1,
		},
		{
			name: "warn when template type has standard set",
			obj: &Agent{
				Spec: AgentSpec{
					Runtime: RuntimeSpec{
						Type:     RuntimeTypeTemplate,
						Template: &TemplatedRuntimeSpec{Config: &TemplatedAgentConfig{}},
						Standard: &StandardRuntimeSpec{
							Container: corev1.Container{Name: "agent", Image: "img:latest"},
						},
					},
				},
			},
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := collectAgentWarnings(tt.obj)
			if len(warnings) != tt.wantWarnings {
				t.Errorf("collectAgentWarnings() returned %d warnings, want %d: %v",
					len(warnings), tt.wantWarnings, warnings)
			}
		})
	}
}
