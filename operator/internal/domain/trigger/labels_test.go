package trigger

import (
	"testing"
)

func TestLabels(t *testing.T) {
	tests := []struct {
		name        string
		triggerName string
		wantLabels  map[string]string
	}{
		{
			name:        "standard trigger name",
			triggerName: "my-trigger",
			wantLabels: map[string]string{
				"app.kubernetes.io/name":       "my-trigger",
				"app.kubernetes.io/component":  "agenttrigger",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/trigger":            "my-trigger",
			},
		},
		{
			name:        "trigger name with dashes",
			triggerName: "stripe-payment-webhook",
			wantLabels: map[string]string{
				"app.kubernetes.io/name":       "stripe-payment-webhook",
				"app.kubernetes.io/component":  "agenttrigger",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/trigger":            "stripe-payment-webhook",
			},
		},
		{
			name:        "single character trigger name",
			triggerName: "x",
			wantLabels: map[string]string{
				"app.kubernetes.io/name":       "x",
				"app.kubernetes.io/component":  "agenttrigger",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/trigger":            "x",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Labels(tt.triggerName)

			if len(got) != len(tt.wantLabels) {
				t.Errorf("Labels() returned %d labels, want %d", len(got), len(tt.wantLabels))
			}

			for key, wantVal := range tt.wantLabels {
				gotVal, ok := got[key]
				if !ok {
					t.Errorf("Labels() missing key %q", key)
					continue
				}
				if gotVal != wantVal {
					t.Errorf("Labels()[%q] = %q, want %q", key, gotVal, wantVal)
				}
			}
		})
	}
}

func TestSensorName(t *testing.T) {
	tests := []struct {
		name        string
		triggerName string
		want        string
	}{
		{
			name:        "standard name",
			triggerName: "my-trigger",
			want:        "at-my-trigger",
		},
		{
			name:        "short name",
			triggerName: "t",
			want:        "at-t",
		},
		{
			name:        "long name with dashes",
			triggerName: "stripe-payment-webhook-handler",
			want:        "at-stripe-payment-webhook-handler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SensorName(tt.triggerName)
			if got != tt.want {
				t.Errorf("SensorName(%q) = %q, want %q", tt.triggerName, got, tt.want)
			}
		})
	}
}

func TestConfigMapName(t *testing.T) {
	tests := []struct {
		name        string
		triggerName string
		want        string
	}{
		{
			name:        "standard name",
			triggerName: "my-trigger",
			want:        "agenttrigger-my-trigger-config",
		},
		{
			name:        "short name",
			triggerName: "t",
			want:        "agenttrigger-t-config",
		},
		{
			name:        "long name with dashes",
			triggerName: "stripe-payment-webhook",
			want:        "agenttrigger-stripe-payment-webhook-config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConfigMapName(tt.triggerName)
			if got != tt.want {
				t.Errorf("ConfigMapName(%q) = %q, want %q", tt.triggerName, got, tt.want)
			}
		})
	}
}
