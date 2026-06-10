package trigger

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// Condition type constants for AgentTrigger status.
const (
	ConditionTypeReady            = "Ready"
	ConditionTypeEventSourceReady = "EventSourceReady"
	ConditionTypeEventBusReady    = "EventBusReady"
	ConditionTypeAgentReady       = "AgentReady"
	ConditionTypeSensorReady      = "SensorReady"
)

// Condition reason constants for AgentTrigger status.
const (
	ReasonSensorCreated        = "SensorCreated"
	ReasonSensorCreateFailed   = "SensorCreateFailed"
	ReasonSensorReady          = "SensorReady"
	ReasonSensorNotReady       = "SensorNotReady"
	ReasonAgentResolved        = "AgentResolved"
	ReasonAgentNotFound        = "AgentNotFound"
	ReasonAgentNotReady        = "AgentNotReady"
	ReasonEventSourceResolved  = "EventSourceResolved"
	ReasonEventSourceNotFound  = "EventSourceNotFound"
	ReasonEventBusResolved     = "EventBusResolved"
	ReasonEventBusNotFound     = "EventBusNotFound"
	ReasonAllReady             = "AllReady"
	ReasonReconcileError       = "ReconcileError"
	ReasonValidationFailed     = "ValidationFailed"
	ReasonArgoEventsNotInstall = "ArgoEventsNotInstalled"
)

// SetCondition updates or adds a condition to the AgentTrigger status.
func SetCondition(trigger *agentv1alpha1.AgentTrigger, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: trigger.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&trigger.Status.Conditions, condition)
}

// CalculatePhase determines the overall phase from conditions.
func CalculatePhase(trigger *agentv1alpha1.AgentTrigger) agentv1alpha1.AgentTriggerPhase {
	readyCond := meta.FindStatusCondition(trigger.Status.Conditions, ConditionTypeReady)
	if readyCond == nil {
		return agentv1alpha1.AgentTriggerPhasePending
	}
	if readyCond.Status == metav1.ConditionTrue {
		return agentv1alpha1.AgentTriggerPhaseRunning
	}
	// If any condition has a permanent failure reason, mark as Failed
	if readyCond.Reason == ReasonValidationFailed || readyCond.Reason == ReasonArgoEventsNotInstall {
		return agentv1alpha1.AgentTriggerPhaseFailed
	}
	return agentv1alpha1.AgentTriggerPhasePending
}
