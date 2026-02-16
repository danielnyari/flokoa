package agent

import (
	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// Labels returns the standard Kubernetes labels for an Agent resource.
func Labels(agent *agentv1alpha1.Agent) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       agent.Name,
		"app.kubernetes.io/instance":   agent.Name,
		"app.kubernetes.io/managed-by": "flokoa-operator",
		"flokoa.ai/agent":              agent.Name,
	}
}
