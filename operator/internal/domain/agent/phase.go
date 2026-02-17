package agent

import (
	appsv1 "k8s.io/api/apps/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// CalculatePhase determines the agent phase from the deployment status.
func CalculatePhase(deployment *appsv1.Deployment) agentv1alpha1.AgentPhase {
	if deployment.Status.AvailableReplicas > 0 {
		return agentv1alpha1.AgentPhaseRunning
	}
	return agentv1alpha1.AgentPhasePending
}
