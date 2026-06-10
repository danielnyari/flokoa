package trigger

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestSetCondition(t *testing.T) {
	tests := []struct {
		name               string
		existingConditions []metav1.Condition
		conditionType      string
		status             metav1.ConditionStatus
		reason             string
		message            string
		generation         int64
		wantConditionLen   int
		wantStatus         metav1.ConditionStatus
		wantReason         string
		wantMessage        string
		wantGeneration     int64
	}{
		{
			name:               "add new condition to empty list",
			existingConditions: nil,
			conditionType:      ConditionTypeReady,
			status:             metav1.ConditionTrue,
			reason:             ReasonAllReady,
			message:            "all components are ready",
			generation:         1,
			wantConditionLen:   1,
			wantStatus:         metav1.ConditionTrue,
			wantReason:         ReasonAllReady,
			wantMessage:        "all components are ready",
			wantGeneration:     1,
		},
		{
			name: "update existing condition",
			existingConditions: []metav1.Condition{
				{
					Type:               ConditionTypeReady,
					Status:             metav1.ConditionFalse,
					Reason:             ReasonSensorNotReady,
					Message:            "sensor not ready",
					ObservedGeneration: 1,
					LastTransitionTime: metav1.Now(),
				},
			},
			conditionType:    ConditionTypeReady,
			status:           metav1.ConditionTrue,
			reason:           ReasonAllReady,
			message:          "all ready now",
			generation:       2,
			wantConditionLen: 1,
			wantStatus:       metav1.ConditionTrue,
			wantReason:       ReasonAllReady,
			wantMessage:      "all ready now",
			wantGeneration:   2,
		},
		{
			name: "add second condition type",
			existingConditions: []metav1.Condition{
				{
					Type:               ConditionTypeReady,
					Status:             metav1.ConditionTrue,
					Reason:             ReasonAllReady,
					Message:            "ready",
					ObservedGeneration: 1,
					LastTransitionTime: metav1.Now(),
				},
			},
			conditionType:    ConditionTypeSensorReady,
			status:           metav1.ConditionTrue,
			reason:           ReasonSensorReady,
			message:          "sensor is ready",
			generation:       1,
			wantConditionLen: 2,
			wantStatus:       metav1.ConditionTrue,
			wantReason:       ReasonSensorReady,
			wantMessage:      "sensor is ready",
			wantGeneration:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := &agentv1alpha1.AgentTrigger{}
			trigger.Generation = tt.generation
			trigger.Status.Conditions = tt.existingConditions

			SetCondition(trigger, tt.conditionType, tt.status, tt.reason, tt.message)

			if got := len(trigger.Status.Conditions); got != tt.wantConditionLen {
				t.Errorf("condition count = %d, want %d", got, tt.wantConditionLen)
			}

			// Find the condition we just set
			var found *metav1.Condition
			for i := range trigger.Status.Conditions {
				if trigger.Status.Conditions[i].Type == tt.conditionType {
					found = &trigger.Status.Conditions[i]
					break
				}
			}

			if found == nil {
				t.Fatalf("condition type %q not found", tt.conditionType)
			}

			if found.Status != tt.wantStatus {
				t.Errorf("condition status = %q, want %q", found.Status, tt.wantStatus)
			}
			if found.Reason != tt.wantReason {
				t.Errorf("condition reason = %q, want %q", found.Reason, tt.wantReason)
			}
			if found.Message != tt.wantMessage {
				t.Errorf("condition message = %q, want %q", found.Message, tt.wantMessage)
			}
			if found.ObservedGeneration != tt.wantGeneration {
				t.Errorf("condition observedGeneration = %d, want %d", found.ObservedGeneration, tt.wantGeneration)
			}
			if found.LastTransitionTime.IsZero() {
				t.Error("condition lastTransitionTime should not be zero")
			}
		})
	}
}

func TestCalculatePhase(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		wantPhase  agentv1alpha1.AgentTriggerPhase
	}{
		{
			name:       "no conditions returns Pending",
			conditions: nil,
			wantPhase:  agentv1alpha1.AgentTriggerPhasePending,
		},
		{
			name:       "empty conditions returns Pending",
			conditions: []metav1.Condition{},
			wantPhase:  agentv1alpha1.AgentTriggerPhasePending,
		},
		{
			name: "no Ready condition returns Pending",
			conditions: []metav1.Condition{
				{
					Type:   ConditionTypeSensorReady,
					Status: metav1.ConditionTrue,
					Reason: ReasonSensorReady,
				},
			},
			wantPhase: agentv1alpha1.AgentTriggerPhasePending,
		},
		{
			name: "Ready=True returns Running",
			conditions: []metav1.Condition{
				{
					Type:   ConditionTypeReady,
					Status: metav1.ConditionTrue,
					Reason: ReasonAllReady,
				},
			},
			wantPhase: agentv1alpha1.AgentTriggerPhaseRunning,
		},
		{
			name: "Ready=False with non-fatal reason returns Pending",
			conditions: []metav1.Condition{
				{
					Type:   ConditionTypeReady,
					Status: metav1.ConditionFalse,
					Reason: ReasonSensorNotReady,
				},
			},
			wantPhase: agentv1alpha1.AgentTriggerPhasePending,
		},
		{
			name: "Ready=False with ValidationFailed returns Failed",
			conditions: []metav1.Condition{
				{
					Type:   ConditionTypeReady,
					Status: metav1.ConditionFalse,
					Reason: ReasonValidationFailed,
				},
			},
			wantPhase: agentv1alpha1.AgentTriggerPhaseFailed,
		},
		{
			name: "Ready=False with ArgoEventsNotInstalled returns Failed",
			conditions: []metav1.Condition{
				{
					Type:   ConditionTypeReady,
					Status: metav1.ConditionFalse,
					Reason: ReasonArgoEventsNotInstall,
				},
			},
			wantPhase: agentv1alpha1.AgentTriggerPhaseFailed,
		},
		{
			name: "Ready=False with ReconcileError returns Pending",
			conditions: []metav1.Condition{
				{
					Type:   ConditionTypeReady,
					Status: metav1.ConditionFalse,
					Reason: ReasonReconcileError,
				},
			},
			wantPhase: agentv1alpha1.AgentTriggerPhasePending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := &agentv1alpha1.AgentTrigger{}
			trigger.Status.Conditions = tt.conditions

			got := CalculatePhase(trigger)

			if got != tt.wantPhase {
				t.Errorf("CalculatePhase() = %q, want %q", got, tt.wantPhase)
			}
		})
	}
}
