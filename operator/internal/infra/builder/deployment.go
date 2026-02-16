package builder

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const (
	DefaultTemplateRuntimeImage = "ghcr.io/danielnyari/flokoa-cli:latest"

	TemplateConfigVolumeName   = "template-config"
	TemplateConfigConfigMapKey = "template-config.json"
	TemplateConfigMountPath    = "/etc/flokoa/template-config.json"

	InstructionVolumeName   = "instruction"
	InstructionMountPath    = "/etc/flokoa/instruction.txt"
	InstructionConfigMapKey = "instruction.txt"

	ToolsMountPath        = "/etc/flokoa/tools"
	AgentCardVolumeName   = "agent-card"
	AgentCardConfigMapKey = "agent-card.json"
	AgentCardMountPath    = "/etc/flokoa/agent-card.json"
	ModelVolumeName       = "model-config"
	ModelConfigMapKey     = "model.json"
	ModelMountPath        = "/etc/flokoa/model.json"
)

// ToolMount holds information about a tool's ConfigMap for mounting.
type ToolMount struct {
	ToolName      string
	ConfigMapName string
	DataHash      string
}

// ModelMount holds resolved model configuration for deployment.
type ModelMount struct {
	ConfigMapName  string
	EnvVars        []corev1.EnvVar
	SecretEnvVars  []corev1.EnvVar
	SecretRefsHash string
}

// DeploymentParams captures all inputs needed to build an agent Deployment.
type DeploymentParams struct {
	AgentName         string
	AgentNamespace    string
	Labels            map[string]string
	Runtime           agentv1alpha1.RuntimeSpec
	ToolMounts        []ToolMount
	AgentCardCM       string
	ModelInfo         *ModelMount
	TemplateCMName    string
	InstructionCMName string
}

// BuildDeployment constructs a Kubernetes Deployment for an agent.
// This is a pure function — no I/O.
func BuildDeployment(params DeploymentParams) *appsv1.Deployment {
	overrides := getDeploymentOverrides(params.Runtime)
	replicas := int32(1)
	if overrides.Replicas != nil {
		replicas = *overrides.Replicas
	}

	var container corev1.Container
	var volumes []corev1.Volume //nolint:prealloc // size depends on runtime type and optional configs

	switch params.Runtime.Type {
	case agentv1alpha1.RuntimeTypeTemplate:
		container, volumes = buildManagedContainerSpec(params.Runtime, params.TemplateCMName)
	default:
		container, volumes = buildStandardContainerSpec(params.Runtime)
	}

	// Compute the agent URL for the FLOKOA_AGENT_URL env var
	agentURL := fmt.Sprintf("http://%s.%s.svc.cluster.local", params.AgentName, params.AgentNamespace)

	// Add FLOKOA_AGENT_URL environment variable
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  "FLOKOA_AGENT_URL",
		Value: agentURL,
	})

	// Defensive fallback: template runtime must always have a valid runtime image.
	if params.Runtime.Type == agentv1alpha1.RuntimeTypeTemplate && container.Image == "" {
		container.Image = DefaultTemplateRuntimeImage
	}

	// Add agent card ConfigMap volume
	volumes = append(volumes, corev1.Volume{
		Name: AgentCardVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: params.AgentCardCM,
				},
			},
		},
	})
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      AgentCardVolumeName,
		MountPath: AgentCardMountPath,
		SubPath:   AgentCardConfigMapKey,
		ReadOnly:  true,
	})

	// Add model configuration if specified
	if params.ModelInfo != nil {
		volumes = append(volumes, corev1.Volume{
			Name: ModelVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: params.ModelInfo.ConfigMapName,
					},
				},
			},
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      ModelVolumeName,
			MountPath: ModelMountPath,
			SubPath:   ModelConfigMapKey,
			ReadOnly:  true,
		})
		container.Env = append(container.Env, params.ModelInfo.EnvVars...)
		container.Env = append(container.Env, params.ModelInfo.SecretEnvVars...)
	}

	// Add instruction ConfigMap volume mount
	if params.InstructionCMName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: InstructionVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: params.InstructionCMName,
					},
				},
			},
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      InstructionVolumeName,
			MountPath: InstructionMountPath,
			SubPath:   InstructionConfigMapKey,
			ReadOnly:  true,
		})
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "FLOKOA_INSTRUCTION_PATH",
			Value: InstructionMountPath,
		})
	}

	// Add tool volume mounts and compute combined hash
	var toolsHashBuilder string
	for _, toolCM := range params.ToolMounts {
		volumeName := fmt.Sprintf("tool-%s", toolCM.ToolName)
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: toolCM.ConfigMapName,
					},
				},
			},
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: fmt.Sprintf("%s/%s", ToolsMountPath, toolCM.ToolName),
			ReadOnly:  true,
		})
		toolsHashBuilder += toolCM.ToolName + ":" + toolCM.DataHash + ";"
	}

	// Compute combined tools hash for pod annotation
	var podAnnotations map[string]string
	if len(params.ToolMounts) > 0 {
		h := sha256.Sum256([]byte(toolsHashBuilder))
		podAnnotations = map[string]string{
			"flokoa.ai/tools-hash": hex.EncodeToString(h[:])[:16],
		}
	}

	if params.ModelInfo != nil && params.ModelInfo.SecretRefsHash != "" {
		if podAnnotations == nil {
			podAnnotations = map[string]string{}
		}
		podAnnotations["flokoa.ai/model-secrets-hash"] = params.ModelInfo.SecretRefsHash
	}

	// Default to restricted PSS-compliant pod security context if none provided
	podSecurityContext := overrides.SecurityContext
	if podSecurityContext == nil {
		podSecurityContext = restrictedPodSecurityContext()
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

func getDeploymentOverrides(runtime agentv1alpha1.RuntimeSpec) agentv1alpha1.DeploymentOverrides {
	switch runtime.Type {
	case agentv1alpha1.RuntimeTypeTemplate:
		if runtime.Template != nil {
			return runtime.Template.DeploymentOverrides
		}
	case agentv1alpha1.RuntimeTypeStandard:
		if runtime.Standard != nil {
			return runtime.Standard.DeploymentOverrides
		}
	}
	return agentv1alpha1.DeploymentOverrides{}
}

func buildStandardContainerSpec(runtime agentv1alpha1.RuntimeSpec) (corev1.Container, []corev1.Volume) {
	spec := runtime.Standard
	if spec == nil {
		spec = &agentv1alpha1.StandardRuntimeSpec{}
	}

	container := spec.Container
	if container.Name == "" {
		container.Name = "agent"
	}

	volumes := append([]corev1.Volume{}, spec.Volumes...)
	return container, volumes
}

func buildManagedContainerSpec(runtime agentv1alpha1.RuntimeSpec, templateConfigMapName string) (corev1.Container, []corev1.Volume) {
	template := runtime.Template
	if template == nil {
		template = &agentv1alpha1.TemplatedRuntimeSpec{}
	}

	container := corev1.Container{
		Name:  "agent",
		Image: DefaultTemplateRuntimeImage,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: 8080,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: append([]corev1.EnvVar{
			{
				Name:  "FLOKOA_RUNTIME_MODE",
				Value: "template",
			},
			{
				Name:  "FLOKOA_TEMPLATE_CONFIG_PATH",
				Value: TemplateConfigMountPath,
			},
		}, template.Env...),
		SecurityContext: restrictedContainerSecurityContext(),
	}

	if template.Resources != nil {
		container.Resources = *template.Resources
	}

	var volumes []corev1.Volume

	if templateConfigMapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: TemplateConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: templateConfigMapName,
					},
				},
			},
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      TemplateConfigVolumeName,
			MountPath: TemplateConfigMountPath,
			SubPath:   TemplateConfigConfigMapKey,
			ReadOnly:  true,
		})
	}

	return container, volumes
}

func restrictedContainerSecurityContext() *corev1.SecurityContext {
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

func restrictedPodSecurityContext() *corev1.PodSecurityContext {
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
