package controller

// Re-export agent domain functions, constants, and builder helpers for use by
// existing tests and callers in the controller package. These will be cleaned
// up once all agent logic moves to the app/controller layers fully.

import (
	"context"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
	"github.com/danielnyari/flokoa/internal/domain/hash"
	"github.com/danielnyari/flokoa/internal/infra/builder"
)

// Agent condition types (re-exported from domain).
const (
	ConditionTypeReady             = agentdomain.ConditionTypeReady
	ConditionTypeToolsReady        = agentdomain.ConditionTypeToolsReady
	ConditionTypeModelReady        = agentdomain.ConditionTypeModelReady
	ConditionTypeModelSecretsReady = agentdomain.ConditionTypeModelSecretsReady
	ConditionTypeInstructionReady  = agentdomain.ConditionTypeInstructionReady
)

// Agent condition reasons (re-exported from domain).
const (
	ReasonDeploymentReady       = agentdomain.ReasonDeploymentReady
	ReasonDeploymentNotReady    = agentdomain.ReasonDeploymentNotReady
	ReasonReconcileError        = agentdomain.ReasonReconcileError
	ReasonToolsSynced           = agentdomain.ReasonToolsSynced
	ReasonToolSyncFailed        = agentdomain.ReasonToolSyncFailed
	ReasonModelResolved         = agentdomain.ReasonModelResolved
	ReasonModelSecretsResolved  = agentdomain.ReasonModelSecretsResolved
	ReasonModelSecretsMissing   = agentdomain.ReasonModelSecretsMissing
	ReasonModelResolveFailed    = agentdomain.ReasonModelResolveFailed
	ReasonInstructionResolved   = agentdomain.ReasonInstructionResolved
	ReasonInstructionSyncFailed = agentdomain.ReasonInstructionSyncFailed
)

// Builder constants (re-exported for existing tests).
const (
	defaultTemplateRuntimeImage = builder.DefaultTemplateRuntimeImage
	templateConfigVolumeName    = builder.TemplateConfigVolumeName
	templateConfigMountPath     = builder.TemplateConfigMountPath
	instructionVolumeName       = builder.InstructionVolumeName
	instructionMountPath        = builder.InstructionMountPath
)

// toolConfigMapInfo is a compatibility type for existing tests.
type toolConfigMapInfo struct {
	toolName      string
	configMapName string
	dataHash      string
}

// resolvedModelInfo is a compatibility type for existing tests.
type resolvedModelInfo struct {
	provider          agentv1alpha1.ProviderType
	model             string
	configMapName     string
	envVars           []corev1.EnvVar
	secretEnvVars     []corev1.EnvVar
	secretRefsHash    string
	missingSecretRefs []string
}

// ConfigMap key constants (re-exported for existing tests that reference them).
const (
	templateConfigConfigMapKey = builder.TemplateConfigConfigMapKey
)

// boolPtr is a compatibility wrapper for existing tests.
func boolPtr(b bool) *bool {
	return &b
}

// hashConfigMapData is a compatibility wrapper for existing tests.
func hashConfigMapData(data map[string]string) string {
	return hash.ConfigMapData(data)
}

// calculatePhase is a compatibility wrapper for existing tests that call r.calculatePhase.
func (r *AgentReconciler) calculatePhase(deployment *appsv1.Deployment) agentv1alpha1.AgentPhase {
	return agentdomain.CalculatePhase(deployment)
}

// buildDeployment is a compatibility wrapper for existing tests that call r.buildDeployment.
func (r *AgentReconciler) buildDeployment(agent *agentv1alpha1.Agent, toolConfigMaps []toolConfigMapInfo, agentCardConfigMap string, modelInfo *resolvedModelInfo, templateConfigMapName string, instructionConfigMapName string) *appsv1.Deployment {
	var toolMounts []builder.ToolMount
	for _, t := range toolConfigMaps {
		toolMounts = append(toolMounts, builder.ToolMount{
			ToolName:      t.toolName,
			ConfigMapName: t.configMapName,
			DataHash:      t.dataHash,
		})
	}

	var modelMount *builder.ModelMount
	if modelInfo != nil {
		modelMount = &builder.ModelMount{
			ConfigMapName:  modelInfo.configMapName,
			EnvVars:        modelInfo.envVars,
			SecretEnvVars:  modelInfo.secretEnvVars,
			SecretRefsHash: modelInfo.secretRefsHash,
		}
	}

	return builder.BuildDeployment(builder.DeploymentParams{
		AgentName:         agent.Name,
		AgentNamespace:    agent.Namespace,
		Labels:            agentdomain.Labels(agent),
		Runtime:           agent.Spec.Runtime,
		ToolMounts:        toolMounts,
		AgentCardCM:       agentCardConfigMap,
		ModelInfo:         modelMount,
		TemplateCMName:    templateConfigMapName,
		InstructionCMName: instructionConfigMapName,
	})
}

// computeSecretRefsHash is a compatibility wrapper for existing tests.
func (r *AgentReconciler) computeSecretRefsHash(ctx context.Context, namespace string, secretEnvVars []corev1.EnvVar) (string, []string, error) {
	if len(secretEnvVars) == 0 {
		return "", nil, nil
	}

	secretMap := map[string]string{}
	missingSecretSet := map[string]struct{}{}
	for _, envVar := range secretEnvVars {
		if envVar.ValueFrom == nil || envVar.ValueFrom.SecretKeyRef == nil {
			continue
		}

		secretName := envVar.ValueFrom.SecretKeyRef.Name
		if secretName == "" {
			continue
		}

		if _, exists := secretMap[secretName]; exists {
			continue
		}

		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				secretMap[secretName] = "missing"
				missingSecretSet[secretName] = struct{}{}
				continue
			}
			return "", nil, err
		}

		secretMap[secretName] = secret.ResourceVersion
	}

	hashValue := hash.SecretVersions(secretMap)
	if hashValue == "" {
		return "", nil, nil
	}

	missingSecrets := make([]string, 0, len(missingSecretSet))
	for name := range missingSecretSet {
		missingSecrets = append(missingSecrets, name)
	}
	sort.Strings(missingSecrets)

	return hashValue, missingSecrets, nil
}
