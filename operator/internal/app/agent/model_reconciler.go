package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/domain/hash"
	modeldomain "github.com/danielnyari/flokoa/internal/domain/model"
	"github.com/danielnyari/flokoa/internal/infra/builder"
	"github.com/danielnyari/flokoa/internal/infra/repo"
)

// ModelReconciler handles model resolution for an agent.
type ModelReconciler struct {
	models             repo.ModelReader
	providers          repo.ModelProviderReader
	configMaps         repo.ConfigMapRepo
	secrets            repo.SecretReader
	owner              repo.OwnerSetter
	getProviderHandler func(agentv1alpha1.ProviderType) (modeldomain.ProviderHandler, bool)
}

// Reconcile resolves the model reference and creates the model ConfigMap.
func (m *ModelReconciler) Reconcile(ctx context.Context, agent *agentv1alpha1.Agent) (*resolvedModelInfo, error) {
	logger := log.FromContext(ctx)

	if agent.Spec.Model == nil {
		return nil, nil
	}

	// Resolve the Model reference
	modelNamespace := agent.Spec.Model.Namespace
	if modelNamespace == "" {
		modelNamespace = agent.Namespace
	}

	model, err := m.models.GetModel(ctx, types.NamespacedName{Name: agent.Spec.Model.Name, Namespace: modelNamespace})
	if err != nil {
		return nil, fmt.Errorf("failed to get Model %s/%s: %w", modelNamespace, agent.Spec.Model.Name, err)
	}

	if !model.Status.Ready {
		return nil, fmt.Errorf("model %s/%s is not ready", modelNamespace, model.Name)
	}

	// Resolve the ModelProvider
	providerNamespace := model.Spec.ProviderRef.Namespace
	if providerNamespace == "" {
		providerNamespace = model.Namespace
	}

	modelProvider, err := m.providers.GetModelProvider(ctx, types.NamespacedName{Name: model.Spec.ProviderRef.Name, Namespace: providerNamespace})
	if err != nil {
		return nil, fmt.Errorf("failed to get ModelProvider %s/%s: %w", providerNamespace, model.Spec.ProviderRef.Name, err)
	}

	if !modelProvider.Status.Ready {
		return nil, fmt.Errorf("ModelProvider %s/%s is not ready", providerNamespace, modelProvider.Name)
	}

	providerType := modelProvider.GetProviderType()
	if providerType == "" {
		return nil, fmt.Errorf("ModelProvider %s/%s has no provider type configured (must set one of openai, anthropic, google, or bedrock)", providerNamespace, modelProvider.Name)
	}
	logger.Info("Resolved Model and ModelProvider", "model", model.Name, "modelName", model.Spec.Model, "provider", providerType, "modelProvider", modelProvider.Name)

	providerHandler, ok := m.getProviderHandler(providerType)
	if !ok {
		return nil, fmt.Errorf("unsupported model provider: %s", providerType)
	}

	resolvedConfig, err := providerHandler.BuildConfig(modelProvider, model)
	if err != nil {
		return nil, fmt.Errorf("failed to build provider config: %w", err)
	}

	configMapName, err := m.reconcileModelConfigMap(ctx, agent, resolvedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile model ConfigMap: %w", err)
	}

	secretRefsHash, missingSecrets, err := m.computeSecretRefsHash(ctx, agent.Namespace, resolvedConfig.SecretEnvVars)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve model secrets: %w", err)
	}

	return &resolvedModelInfo{
		provider:          resolvedConfig.Provider.Type,
		model:             resolvedConfig.Model,
		configMapName:     configMapName,
		envVars:           resolvedConfig.EnvVars,
		secretEnvVars:     resolvedConfig.SecretEnvVars,
		secretRefsHash:    secretRefsHash,
		missingSecretRefs: missingSecrets,
	}, nil
}

// reconcileModelConfigMap creates or updates the ConfigMap containing the model configuration.
func (m *ModelReconciler) reconcileModelConfigMap(ctx context.Context, agent *agentv1alpha1.Agent, config *modeldomain.ResolvedModelConfig) (string, error) {
	configMapName := fmt.Sprintf("%s-model", agent.Name)

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal model config to JSON: %w", err)
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       agent.Name,
				"app.kubernetes.io/component":  "model-config",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
		},
		Data: map[string]string{
			builder.ModelConfigMapKey: string(configJSON),
		},
	}

	if err := m.owner.SetOwner(agent, desired); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := m.configMaps.EnsureConfigMap(ctx, desired); err != nil {
		return "", err
	}

	return configMapName, nil
}

// computeSecretRefsHash fetches secret resource versions and computes a combined hash.
func (m *ModelReconciler) computeSecretRefsHash(ctx context.Context, namespace string, secretEnvVars []corev1.EnvVar) (string, []string, error) {
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

		secret, err := m.secrets.GetSecret(ctx, types.NamespacedName{Name: secretName, Namespace: namespace})
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
