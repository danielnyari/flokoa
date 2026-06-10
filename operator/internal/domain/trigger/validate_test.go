package trigger

import (
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		trigger *agentv1alpha1.AgentTrigger
		wantErr string
	}{
		{
			name: "valid minimal spec",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "missing eventSource name",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
				},
			},
			wantErr: "eventSource.name is required",
		},
		{
			name: "missing eventSource eventName",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
				},
			},
			wantErr: "eventSource.eventName is required",
		},
		{
			name: "missing agent name",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "",
					},
				},
			},
			wantErr: "agent.name is required",
		},
		{
			name: "pushNotification with both agentRef and URL",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					PushNotification: &agentv1alpha1.PushNotificationTarget{
						AgentRef: &agentv1alpha1.AgentRef{
							Name: "target-agent",
						},
						URL: "https://example.com/webhook",
					},
				},
			},
			wantErr: "pushNotification: agentRef and url are mutually exclusive",
		},
		{
			name: "pushNotification with neither agentRef nor URL",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					PushNotification: &agentv1alpha1.PushNotificationTarget{},
				},
			},
			wantErr: "pushNotification: exactly one of agentRef or url must be specified",
		},
		{
			name: "pushNotification valid with agentRef only",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					PushNotification: &agentv1alpha1.PushNotificationTarget{
						AgentRef: &agentv1alpha1.AgentRef{
							Name: "target-agent",
						},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "pushNotification valid with URL only",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					PushNotification: &agentv1alpha1.PushNotificationTarget{
						URL: "https://example.com/webhook",
					},
				},
			},
			wantErr: "",
		},
		{
			name: "pushNotification agentRef with empty name",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					PushNotification: &agentv1alpha1.PushNotificationTarget{
						AgentRef: &agentv1alpha1.AgentRef{
							Name: "",
						},
					},
				},
			},
			wantErr: "pushNotification.agentRef.name is required",
		},
		{
			name: "filter data missing path",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					Filter: &agentv1alpha1.TriggerFilter{
						Data: []agentv1alpha1.DataFilter{
							{
								Path:  "",
								Type:  "string",
								Value: []string{"foo"},
							},
						},
					},
				},
			},
			wantErr: "filter.data[0].path is required",
		},
		{
			name: "filter data missing value",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					Filter: &agentv1alpha1.TriggerFilter{
						Data: []agentv1alpha1.DataFilter{
							{
								Path:  "body.type",
								Type:  "string",
								Value: []string{},
							},
						},
					},
				},
			},
			wantErr: "filter.data[0].value must have at least one entry",
		},
		{
			name: "filter exprs missing expr",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					Filter: &agentv1alpha1.TriggerFilter{
						Exprs: []agentv1alpha1.ExprFilter{
							{
								Expr: "",
							},
						},
					},
				},
			},
			wantErr: "filter.exprs[0].expr is required",
		},
		{
			name: "valid filter with data and exprs",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					Filter: &agentv1alpha1.TriggerFilter{
						Data: []agentv1alpha1.DataFilter{
							{
								Path:  "body.type",
								Type:  "string",
								Value: []string{"payment.created", "payment.updated"},
							},
						},
						Exprs: []agentv1alpha1.ExprFilter{
							{
								Expr: "body.amount > 1000",
							},
						},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "filter data second entry missing path",
			trigger: &agentv1alpha1.AgentTrigger{
				Spec: agentv1alpha1.AgentTriggerSpec{
					EventSource: agentv1alpha1.EventSourceRef{
						Name:      "my-source",
						EventName: "my-event",
					},
					Agent: agentv1alpha1.AgentRef{
						Name: "my-agent",
					},
					Filter: &agentv1alpha1.TriggerFilter{
						Data: []agentv1alpha1.DataFilter{
							{
								Path:  "body.type",
								Type:  "string",
								Value: []string{"foo"},
							},
							{
								Path:  "",
								Type:  "float",
								Value: []string{"100"},
							},
						},
					},
				},
			},
			wantErr: "filter.data[1].path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpec(tt.trigger)

			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("ValidateSpec() unexpected error: %v", err)
				}
				return
			}

			if err == nil {
				t.Errorf("ValidateSpec() expected error %q, got nil", tt.wantErr)
				return
			}

			if err.Error() != tt.wantErr {
				t.Errorf("ValidateSpec() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
