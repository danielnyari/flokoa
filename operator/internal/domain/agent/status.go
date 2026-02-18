package agent

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// Condition type constants for Agent status.
const (
	ConditionTypeReady             = "Ready"
	ConditionTypeToolsReady        = "ToolsReady"
	ConditionTypeModelReady        = "ModelReady"
	ConditionTypeModelSecretsReady = "ModelSecretsReady"
	ConditionTypeInstructionReady  = "InstructionReady"
)

// Condition reason constants for Agent status.
const (
	ReasonDeploymentReady       = "DeploymentReady"
	ReasonDeploymentNotReady    = "DeploymentNotReady"
	ReasonReconcileError        = "ReconcileError"
	ReasonValidationFailed      = "ValidationFailed"
	ReasonToolsSynced           = "ToolsSynced"
	ReasonToolSyncFailed        = "ToolSyncFailed"
	ReasonModelResolved         = "ModelResolved"
	ReasonModelSecretsResolved  = "ModelSecretsResolved"
	ReasonModelSecretsMissing   = "ModelSecretsMissing"
	ReasonModelResolveFailed    = "ModelResolveFailed"
	ReasonInstructionResolved   = "InstructionResolved"
	ReasonInstructionSyncFailed = "InstructionSyncFailed"
)

// SetCondition updates or adds a condition to the Agent status.
func SetCondition(agent *agentv1alpha1.Agent, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: agent.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&agent.Status.Conditions, condition)
}
