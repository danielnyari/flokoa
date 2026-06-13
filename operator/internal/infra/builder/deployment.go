package builder

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const (
	// DefaultRunnerImageRepository is the generic runner image; the runner
	// version selects the tag. Overridable via operator config (Helm).
	DefaultRunnerImageRepository = "ghcr.io/danielnyari/flokoa-runner"

	// The runtime contract's file interface (§2): one ConfigMap, two keys,
	// mounted as a directory under /etc/flokoa.
	AgentSpecVolumeName   = "agent-spec"
	AgentSpecConfigMapKey = "agent-spec.yaml"
	AgentSpecMountPath    = "/etc/flokoa/agent-spec.yaml"
	AgentCardConfigMapKey = "agent-card.json"
	AgentCardMountPath    = "/etc/flokoa/agent-card.json"

	// RuntimeHealthPath is the runner's lightweight A2A health endpoint
	// (served by flokoa.utils.router, mounted at the app root). It answers
	// only once the FastAPI app is up — i.e. after a successful bootstrap —
	// so the operator gates pod readiness and a slow-install startup budget on
	// it (runtime contract §2).
	RuntimeHealthPath = "/health"
)

// SpecConfigMapName names the compiled-spec ConfigMap for an agent.
func SpecConfigMapName(agentName string) string {
	return agentName + "-agent-spec"
}

// PublishedURL is the normative form of an Agent's published endpoint
// (virtual endpoint identity): callers treat it as opaque.
func PublishedURL(agentName, namespace string) string {
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/", agentName, namespace, PublishedPort)
}

// DeploymentParams captures all inputs needed to build an agent Deployment.
type DeploymentParams struct {
	AgentName      string
	AgentNamespace string
	Labels         map[string]string
	Runtime        agentv1alpha1.AgentRuntime

	// RunnerImageRepository (no tag) + RunnerVersion resolve the image
	// unless Runtime.Image overrides it entirely.
	RunnerImageRepository string
	RunnerVersion         string

	// SchemaDigest of the AgentSpec schema the spec was validated against;
	// the runner cross-checks it at bootstrap (skew detection).
	SchemaDigest string

	SpecConfigMapName string
	SpecHash          string
	SecretsHash       string

	// SecretEnv are FLOKOA_SECRET_* projections (agent secretRefs + tool
	// header secrets); ProviderEnv/ProviderSecretEnv come from the resolved
	// ModelProvider.
	SecretEnv         []corev1.EnvVar
	ProviderEnv       []corev1.EnvVar
	ProviderSecretEnv []corev1.EnvVar

	// PublishedURL is delivered as FLOKOA_PUBLIC_URL.
	PublishedURL string

	// OTLPEndpoint configures telemetry export (empty: no exporter).
	OTLPEndpoint string

	// Capabilities are the attached Capability artifacts to deliver into the
	// runner pod (roadmap 09); empty means no delivery machinery is emitted.
	Capabilities []CapabilityMount

	// CapabilityDelivery selects how Capabilities reach the pod
	// (empty: DeliveryInitContainer).
	CapabilityDelivery CapabilityDeliveryMode
}

// BuildDeployment constructs a Kubernetes Deployment for an agent.
// This is a pure function — no I/O.
func BuildDeployment(params DeploymentParams) *appsv1.Deployment {
	overrides := params.Runtime.DeploymentOverrides
	replicas := int32(1)
	if overrides.Replicas != nil {
		replicas = *overrides.Replicas
	}

	image := params.Runtime.Image
	if image == "" {
		repo := params.RunnerImageRepository
		if repo == "" {
			repo = DefaultRunnerImageRepository
		}
		image = fmt.Sprintf("%s:%s", repo, params.RunnerVersion)
	}

	container := corev1.Container{
		Name:  "agent",
		Image: image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: RuntimePort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env:             buildEnv(params),
		SecurityContext: RestrictedContainerSecurityContext(),
		StartupProbe:    runnerStartupProbe(),
		ReadinessProbe:  runnerReadinessProbe(),
	}
	if params.Runtime.Resources != nil {
		container.Resources = *params.Runtime.Resources
	}

	// One mount carries the whole contract: the compiled spec + the card.
	volumes := []corev1.Volume{{
		Name: AgentSpecVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: params.SpecConfigMapName,
				},
			},
		},
	}}
	container.VolumeMounts = []corev1.VolumeMount{
		{
			Name:      AgentSpecVolumeName,
			MountPath: AgentSpecMountPath,
			SubPath:   AgentSpecConfigMapKey,
			ReadOnly:  true,
		},
		{
			Name:      AgentSpecVolumeName,
			MountPath: AgentCardMountPath,
			SubPath:   AgentCardConfigMapKey,
			ReadOnly:  true,
		},
	}

	// Capability artifact delivery (roadmap 09): with zero capabilities every
	// helper returns nil and the Deployment is byte-identical to before.
	volumes = append(volumes, capabilityVolumes(params)...)
	container.VolumeMounts = append(container.VolumeMounts, capabilityRunnerMounts(params)...)
	initContainers := capabilityInitContainers(params)

	podAnnotations := map[string]string{
		"flokoa.ai/spec-hash": params.SpecHash,
	}
	if params.SecretsHash != "" {
		podAnnotations["flokoa.ai/secrets-hash"] = params.SecretsHash
	}
	if len(params.Capabilities) > 0 {
		podAnnotations[CapabilityDeliveryAnnotation] = string(effectiveCapabilityDelivery(params))
	}

	podSecurityContext := overrides.SecurityContext
	if podSecurityContext == nil {
		podSecurityContext = RestrictedPodSecurityContext()
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      params.AgentName,
			Namespace: params.AgentNamespace,
			Labels:    params.Labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: params.Labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      params.Labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					InitContainers:     initContainers,
					Containers:         []corev1.Container{container},
					Volumes:            volumes,
					ImagePullSecrets:   overrides.ImagePullSecrets,
					ServiceAccountName: overrides.ServiceAccountName,
					SecurityContext:    podSecurityContext,
					NodeSelector:       overrides.NodeSelector,
					Tolerations:        overrides.Tolerations,
					Affinity:           overrides.Affinity,
				},
			},
		},
	}
}

// runnerReadinessProbe gates pod readiness on the A2A server actually
// serving. Until the runner finishes bootstrapping (manifest → spec →
// secrets → capability install → agent → serve) and answers RuntimeHealthPath,
// the pod is not Ready, the Deployment reports zero availableReplicas, and the
// published Service routes nothing to it. This closes the window in which the
// kubelet would otherwise mark a still-bootstrapping — or crash-looping —
// container Ready just for being "running".
func runnerReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:     healthProbeHandler(),
		PeriodSeconds:    10,
		TimeoutSeconds:   3,
		FailureThreshold: 3,
		SuccessThreshold: 1,
	}
}

// runnerStartupProbe gives bootstrap — dominated by capability wheelhouse
// installs (pip) — a generous budget before the readiness regime takes over.
// Kubernetes gates readiness behind a successful startup probe, so a
// slow-but-legitimate boot never reports Ready prematurely, while a boot that
// never completes (e.g. a failed wheelhouse integrity check) stays not-Ready
// the whole time. Budget: 5s × 60 = 300s.
func runnerStartupProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:     healthProbeHandler(),
		PeriodSeconds:    5,
		TimeoutSeconds:   3,
		FailureThreshold: 60,
		SuccessThreshold: 1,
	}
}

// healthProbeHandler is the shared HTTP GET on the runner's health endpoint.
func healthProbeHandler() corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: RuntimeHealthPath,
			Port: intstr.FromInt32(RuntimePort),
		},
	}
}

// buildEnv assembles the runner environment per the runtime contract (§2):
// serving + skew detection + telemetry identity, then secret/provider
// projections. User env (runtime.env) wins name conflicts: operator entries
// with a user-overridden name are dropped rather than duplicated.
func buildEnv(params DeploymentParams) []corev1.EnvVar {
	operatorEnv := []corev1.EnvVar{
		{Name: "FLOKOA_PUBLIC_URL", Value: params.PublishedURL},
		{Name: "FLOKOA_EXPECTED_RUNNER_VERSION", Value: params.RunnerVersion},
		{Name: "FLOKOA_EXPECTED_SCHEMA_DIGEST", Value: params.SchemaDigest},
		{Name: "OTEL_SERVICE_NAME", Value: params.AgentName},
		{Name: "OTEL_RESOURCE_ATTRIBUTES", Value: fmt.Sprintf(
			"k8s.namespace.name=%s,flokoa.agent.name=%s", params.AgentNamespace, params.AgentName)},
	}
	if params.OTLPEndpoint != "" {
		operatorEnv = append(operatorEnv, corev1.EnvVar{
			Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: params.OTLPEndpoint,
		})
	}
	operatorEnv = append(operatorEnv, params.SecretEnv...)
	operatorEnv = append(operatorEnv, params.ProviderEnv...)
	operatorEnv = append(operatorEnv, params.ProviderSecretEnv...)

	userNames := map[string]bool{}
	for _, env := range params.Runtime.Env {
		userNames[env.Name] = true
	}

	env := make([]corev1.EnvVar, 0, len(operatorEnv)+len(params.Runtime.Env))
	for _, e := range operatorEnv {
		if !userNames[e.Name] {
			env = append(env, e)
		}
	}
	env = append(env, params.Runtime.Env...)
	return env
}

// RestrictedContainerSecurityContext is the restricted-profile container
// security context applied to every operator-built container (runner,
// capability initContainers, the delivery probe pod).
func RestrictedContainerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: boolPtr(false),
		RunAsNonRoot:             boolPtr(true),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// RestrictedPodSecurityContext is the restricted-profile pod security
// context counterpart of RestrictedContainerSecurityContext.
func RestrictedPodSecurityContext() *corev1.PodSecurityContext {
	return &corev1.PodSecurityContext{
		RunAsNonRoot: boolPtr(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func boolPtr(b bool) *bool {
	return &b
}
