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
)

// BuildService constructs the Service for an agent, selecting the runner
// pods by the agent labels.
func BuildService(name, namespace string, labels map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
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
