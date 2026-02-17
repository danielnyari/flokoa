package builder

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// BuildService constructs a Kubernetes Service for an agent.
// This is a pure function — no I/O.
func BuildService(name, namespace string, labels map[string]string, runtime agentv1alpha1.RuntimeSpec) *corev1.Service {
	var servicePorts []corev1.ServicePort
	if runtime.Type == agentv1alpha1.RuntimeTypeStandard && runtime.Standard != nil {
		for _, cp := range runtime.Standard.Container.Ports {
			servicePorts = append(servicePorts, corev1.ServicePort{
				Name:       cp.Name,
				Port:       cp.ContainerPort,
				TargetPort: intstr.FromInt32(cp.ContainerPort),
				Protocol:   cp.Protocol,
			})
		}
	}

	// Default to port 80 -> 8080 if no ports defined
	if len(servicePorts) == 0 {
		servicePorts = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt32(8080),
				Protocol:   corev1.ProtocolTCP,
			},
		}
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}
