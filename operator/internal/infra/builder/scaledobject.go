package builder

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ScaledObjectGVK is the GroupVersionKind for KEDA ScaledObjects.
var ScaledObjectGVK = schema.GroupVersionKind{
	Group:   "keda.sh",
	Version: "v1alpha1",
	Kind:    "ScaledObject",
}

// ScaledObjectParams captures all inputs needed to build a KEDA ScaledObject.
type ScaledObjectParams struct {
	AgentName      string
	AgentNamespace string
	Labels         map[string]string
	DeploymentName string
	Scaling        agentv1alpha1.ScalingSpec
}

// BuildScaledObject constructs an unstructured KEDA ScaledObject for an agent's Deployment.
// This is a pure function — no I/O.
func BuildScaledObject(params ScaledObjectParams) *unstructured.Unstructured {
	// Convert triggers to unstructured format
	triggers := make([]interface{}, len(params.Scaling.Triggers))
	for i, t := range params.Scaling.Triggers {
		trigger := map[string]interface{}{
			"type":     t.Type,
			"metadata": toStringInterfaceMap(t.Metadata),
		}
		if t.Name != "" {
			trigger["name"] = t.Name
		}
		if t.AuthenticationRef != nil {
			authRef := map[string]interface{}{
				"name": t.AuthenticationRef.Name,
			}
			if t.AuthenticationRef.Kind != "" {
				authRef["kind"] = t.AuthenticationRef.Kind
			}
			trigger["authenticationRef"] = authRef
		}
		if t.MetricType != "" {
			trigger["metricType"] = t.MetricType
		}
		triggers[i] = trigger
	}

	spec := map[string]interface{}{
		"scaleTargetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       params.DeploymentName,
		},
		"triggers": triggers,
	}

	if params.Scaling.MinReplicaCount != nil {
		spec["minReplicaCount"] = int64(*params.Scaling.MinReplicaCount)
	}
	if params.Scaling.MaxReplicaCount != nil {
		spec["maxReplicaCount"] = int64(*params.Scaling.MaxReplicaCount)
	}
	if params.Scaling.CooldownPeriod != nil {
		spec["cooldownPeriod"] = int64(*params.Scaling.CooldownPeriod)
	}
	if params.Scaling.PollingInterval != nil {
		spec["pollingInterval"] = int64(*params.Scaling.PollingInterval)
	}

	// Convert labels to map[string]interface{} for unstructured
	labels := toStringInterfaceMap(params.Labels)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "keda.sh/v1alpha1",
			"kind":       "ScaledObject",
			"metadata": map[string]interface{}{
				"name":      ScaledObjectName(params.AgentName),
				"namespace": params.AgentNamespace,
				"labels":    labels,
			},
			"spec": spec,
		},
	}

	return obj
}

// ScaledObjectName returns the name of the KEDA ScaledObject for a given agent.
func ScaledObjectName(agentName string) string {
	return agentName + "-scaler"
}

// toStringInterfaceMap converts map[string]string to map[string]interface{}
// for unstructured object compatibility.
func toStringInterfaceMap(m map[string]string) map[string]interface{} {
	if m == nil {
		return nil
	}
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
