package agent

import (
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		agent   *agentv1alpha1.Agent
		wantErr string
	}{
		{
			name: "valid standard runtime",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type:     agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "valid template runtime",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type:     agentv1alpha1.RuntimeTypeTemplate,
						Template: &agentv1alpha1.TemplatedRuntimeSpec{},
					},
					Model:       &agentv1alpha1.AgentModelRef{Name: "gpt-4"},
					Instruction: &agentv1alpha1.InstructionEntry{Template: "Be helpful"},
				},
			},
			wantErr: "",
		},
		{
			name: "standard with managed set",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type:     agentv1alpha1.RuntimeTypeStandard,
						Template: &agentv1alpha1.TemplatedRuntimeSpec{},
					},
				},
			},
			wantErr: "runtime.managed must not be set",
		},
		{
			name: "template with standard set",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type:     agentv1alpha1.RuntimeTypeTemplate,
						Standard: &agentv1alpha1.StandardRuntimeSpec{},
					},
				},
			},
			wantErr: "runtime.standard must not be set",
		},
		{
			name: "template without managed",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeTemplate,
					},
				},
			},
			wantErr: "runtime.managed is required",
		},
		{
			name: "template without model",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type:     agentv1alpha1.RuntimeTypeTemplate,
						Template: &agentv1alpha1.TemplatedRuntimeSpec{},
					},
				},
			},
			wantErr: "spec.model is required",
		},
		{
			name: "template without instruction",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type:     agentv1alpha1.RuntimeTypeTemplate,
						Template: &agentv1alpha1.TemplatedRuntimeSpec{},
					},
					Model: &agentv1alpha1.AgentModelRef{Name: "gpt-4"},
				},
			},
			wantErr: "spec.instruction is required",
		},
		{
			name: "unsupported runtime type",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: "unknown",
					},
				},
			},
			wantErr: "unsupported runtime type",
		},
		{
			name: "instruction with both inline and ref",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type:     agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{},
					},
					Instruction: &agentv1alpha1.InstructionEntry{
						Template:       "inline",
						InstructionRef: &agentv1alpha1.NamespacedRef{Name: "ref"},
					},
				},
			},
			wantErr: "mutually exclusive",
		},
		{
			name: "instruction with neither inline nor ref",
			agent: &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Spec: agentv1alpha1.AgentSpec{
					Runtime: agentv1alpha1.RuntimeSpec{
						Type:     agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{},
					},
					Instruction: &agentv1alpha1.InstructionEntry{},
				},
			},
			wantErr: "must have either inline or instructionRef set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpec(tt.agent)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateSpec() error = %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateSpec() error = nil, want containing %q", tt.wantErr)
				} else if !containsSubstring(err.Error(), tt.wantErr) {
					t.Errorf("ValidateSpec() error = %v, want containing %q", err, tt.wantErr)
				}
			}
		})
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
