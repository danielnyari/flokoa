package builder

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// PublishedPort is the port of the published agent endpoint.
	PublishedPort int32 = 80
	// RuntimePort is the port the runner serves on inside the pod.
	RuntimePort int32 = 8080

	// RuntimeServiceSuffix marks the internal workload Service. It is NOT
	// part of the public contract — callers use the published Service only.
	RuntimeServiceSuffix = "-runtime"
)

// RuntimeServiceName names the internal workload Service for an agent.
func RuntimeServiceName(agentName string) string {
	return agentName + RuntimeServiceSuffix
}

// BuildPublishedService constructs the published endpoint Service — the
// flokoa-owned virtual endpoint behind status.url. In v1 it selects the
// runner pods directly (zero added hops); for session-tier agents the
// operator later flips this selector to the session router. Callers see
// nothing: the identity is the contract, the backend is flokoa's.
func BuildPublishedService(name, namespace string, labels map[string]string) *corev1.Service {
	svc := buildService(name, namespace, labels)
	svc.Labels["flokoa.ai/endpoint"] = "published"
	return svc
}

// BuildRuntimeService constructs the internal workload Service
// ({agent}-runtime): the stable name for runner pods, used by flokoa
// internals only.
func BuildRuntimeService(agentName, namespace string, labels map[string]string) *corev1.Service {
	svc := buildService(RuntimeServiceName(agentName), namespace, labels)
	svc.Labels["flokoa.ai/endpoint"] = "runtime"
	return svc
}

func buildService(name, namespace string, labels map[string]string) *corev1.Service {
	serviceLabels := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		serviceLabels[k] = v
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    serviceLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       PublishedPort,
					TargetPort: intstr.FromInt32(RuntimePort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}
