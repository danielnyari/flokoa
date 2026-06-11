package agent

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// Condition type constants for Agent status.
const (
	// ConditionTypeReady tracks deployment availability.
	ConditionTypeReady = "Ready"
	// ConditionTypeSpecValid tracks whether the composition compiled into a
	// schema-valid AgentSpec. False means the last good spec keeps running.
	ConditionTypeSpecValid = "SpecValid"
	// ConditionTypeSecretsReady tracks whether all referenced secrets exist.
	ConditionTypeSecretsReady = "SecretsReady"
)

// Condition reason constants for Agent status.
const (
	ReasonDeploymentReady    = "DeploymentReady"
	ReasonDeploymentNotReady = "DeploymentNotReady"
	ReasonReconcileError     = "ReconcileError"
	ReasonValidationFailed   = "ValidationFailed"
	ReasonSpecCompiled       = "SpecCompiled"
	ReasonSpecInvalid        = "SpecInvalid"
	ReasonDependencyMissing  = "DependencyMissing"
	ReasonSecretsResolved    = "SecretsResolved"
	ReasonSecretsMissing     = "SecretsMissing"
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
