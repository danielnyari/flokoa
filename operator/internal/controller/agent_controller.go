package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const (
	agentFinalizer = "agent.flokoa.ai/finalizer"

	// defaultManagedRuntimeImage is the default container image for managed agents.
	// This image reads configuration from mounted ConfigMaps and runs an A2A server.
	defaultManagedRuntimeImage = "ghcr.io/flokoa/agent-runtime:latest"

	// templateConfigVolumeName is the volume name for the managed agent config ConfigMap
	templateConfigVolumeName = "managed-config"
	// templateConfigConfigMapKey is the key in the ConfigMap for the managed config JSON
	templateConfigConfigMapKey = "managed-config.json"
	// templateConfigMountPath is the file path where the managed config is mounted
	templateConfigMountPath = "/etc/flokoa/managed-config.json"
)

const (
	// instructionVolumeName is the name of the volume for the instruction ConfigMap
	instructionVolumeName = "instruction"
	// instructionMountPath is the file path where the instruction text is mounted
	instructionMountPath = "/etc/flokoa/instruction.txt"
)

const (
	ConditionTypeReady            = "Ready"
	ConditionTypeToolsReady       = "ToolsReady"
	ConditionTypeModelReady       = "ModelReady"
	ConditionTypeInstructionReady = "InstructionReady"
	ReasonDeploymentReady         = "DeploymentReady"
	ReasonDeploymentNotReady      = "DeploymentNotReady"
	ReasonReconcileError          = "ReconcileError"
	ReasonToolsSynced             = "ToolsSynced"
	ReasonToolSyncFailed          = "ToolSyncFailed"
	ReasonModelResolved           = "ModelResolved"
	ReasonModelResolveFailed      = "ModelResolveFailed"
	ReasonInstructionResolved     = "InstructionResolved"
	ReasonInstructionSyncFailed   = "InstructionSyncFailed"
)

const (
	// toolsMountPath is the path where tool configurations are mounted
	toolsMountPath = "/etc/flokoa/tools"
	// agentCardVolumeName is the name of the volume for the agent card ConfigMap
	agentCardVolumeName = "agent-card"
	// agentCardConfigMapKey is the key in the ConfigMap for the agent card JSON
	agentCardConfigMapKey = "agent-card.json"
	// agentCardMountPath is the file path where the agent card is mounted
	agentCardMountPath = "/etc/flokoa/agent-card.json"
	// modelVolumeName is the name of the volume for the model ConfigMap
	modelVolumeName = "model-config"
	// modelConfigMapKey is the key in the ConfigMap for the model JSON
	modelConfigMapKey = "model.json"
	// modelMountPath is the file path where the model config is mounted
	modelMountPath = "/etc/flokoa/model.json"
)

type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=instructions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=models,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=modelproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Agent", "name", req.Name, "namespace", req.Namespace)

	agent := &agentv1alpha1.Agent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !agent.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
			controllerutil.RemoveFinalizer(agent, agentFinalizer)
			if err := r.Update(ctx, agent); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(agent, agentFinalizer) {
		controllerutil.AddFinalizer(agent, agentFinalizer)
		if err := r.Update(ctx, agent); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Validate the agent spec
	if err := r.validateAgent(agent); err != nil {
		logger.Error(err, "Agent validation failed")
		agent.Status.Phase = agentv1alpha1.AgentPhaseFailed
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonValidationFailed, err.Error())
		_ = r.Status().Update(ctx, agent)
		return ctrl.Result{}, nil // Don't requeue on validation errors
	}

	// Reconcile tools (inline and referenced) - creates ConfigMaps and returns volume/mount info
	toolConfigMaps, err := r.reconcileTools(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile tools")
		r.setCondition(agent, ConditionTypeToolsReady, metav1.ConditionFalse, ReasonToolSyncFailed, err.Error())
		_ = r.Status().Update(ctx, agent)
		return ctrl.Result{}, err
	}
	if len(agent.Spec.Tools) > 0 {
		r.setCondition(agent, ConditionTypeToolsReady, metav1.ConditionTrue, ReasonToolsSynced, fmt.Sprintf("Synced %d tools", len(toolConfigMaps)))
		now := metav1.Now()
		agent.Status.LastToolSync = &now
	}

	// Reconcile Instruction (if specified) - creates Instruction CR for inline, resolves ref for existing
	var instructionConfigMapName string
	if agent.Spec.Instruction != nil {
		instructionConfigMapName, err = r.reconcileInstruction(ctx, agent)
		if err != nil {
			logger.Error(err, "Failed to reconcile Instruction")
			r.setCondition(agent, ConditionTypeInstructionReady, metav1.ConditionFalse, ReasonInstructionSyncFailed, err.Error())
			_ = r.Status().Update(ctx, agent)
			return ctrl.Result{}, err
		}
		r.setCondition(agent, ConditionTypeInstructionReady, metav1.ConditionTrue, ReasonInstructionResolved, "Instruction resolved")
	}

	// Reconcile AgentCard ConfigMap
	agentCardConfigMap, err := r.reconcileAgentCardConfigMap(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile AgentCard ConfigMap")
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonReconcileError, err.Error())
		_ = r.Status().Update(ctx, agent)
		return ctrl.Result{}, err
	}

	// Reconcile Model configuration (if specified)
	var modelInfo *resolvedModelInfo
	if agent.Spec.Model != nil {
		modelInfo, err = r.reconcileModel(ctx, agent)
		if err != nil {
			logger.Error(err, "Failed to reconcile Model")
			r.setCondition(agent, ConditionTypeModelReady, metav1.ConditionFalse, ReasonModelResolveFailed, err.Error())
			_ = r.Status().Update(ctx, agent)
			return ctrl.Result{}, err
		}
		r.setCondition(agent, ConditionTypeModelReady, metav1.ConditionTrue, ReasonModelResolved,
			fmt.Sprintf("Model %s/%s resolved", modelInfo.provider, modelInfo.model))
	}

	// Reconcile managed config ConfigMap (if managed runtime)
	var templateConfigMapName string
	if agent.Spec.Runtime.Type == agentv1alpha1.RuntimeTypeManaged {
		templateConfigMapName, err = r.reconciletemplateConfigMap(ctx, agent)
		if err != nil {
			logger.Error(err, "Failed to reconcile managed config ConfigMap")
			r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonReconcileError, err.Error())
			_ = r.Status().Update(ctx, agent)
			return ctrl.Result{}, err
		}
	}

	// Reconcile Deployment
	deployment, err := r.reconcileDeployment(ctx, agent, toolConfigMaps, agentCardConfigMap, modelInfo, templateConfigMapName, instructionConfigMapName)
	if err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonReconcileError, err.Error())
		_ = r.Status().Update(ctx, agent)
		return ctrl.Result{}, err
	}

	// Reconcile Service
	service, err := r.reconcileService(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile Service")
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonReconcileError, err.Error())
		_ = r.Status().Update(ctx, agent)
		return ctrl.Result{}, err
	}

	// Update status
	agent.Status.Phase = r.calculatePhase(deployment)
	agent.Status.Backend = "core"
	agent.Status.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local", service.Name, service.Namespace)
	agent.Status.Replicas = deployment.Status.Replicas
	agent.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	agent.Status.ObservedGeneration = agent.Generation

	if deployment.Status.AvailableReplicas > 0 {
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionTrue, ReasonDeploymentReady, "Agent is ready")
	} else {
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonDeploymentNotReady, "Waiting for pods")
	}

	if err := r.Status().Update(ctx, agent); err != nil {
		logger.Error(err, "Failed to update Agent status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AgentReconciler) reconcileDeployment(ctx context.Context, agent *agentv1alpha1.Agent, toolConfigMaps []toolConfigMapInfo, agentCardConfigMap string, modelInfo *resolvedModelInfo, templateConfigMapName string, instructionConfigMapName string) (*appsv1.Deployment, error) {
	desired := r.buildDeployment(agent, toolConfigMaps, agentCardConfigMap, modelInfo, templateConfigMapName, instructionConfigMapName)

	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)

	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return nil, fmt.Errorf("failed to create Deployment: %w", err)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Deployment: %w", err)
	}

	existing.Spec = desired.Spec
	if err := r.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update Deployment: %w", err)
	}

	return existing, nil
}

// toolConfigMapInfo holds information about a tool's ConfigMap for mounting
type toolConfigMapInfo struct {
	// Name of the tool
	toolName string
	// Name of the ConfigMap containing the tool spec
	configMapName string
	// Hash of the ConfigMap data for change detection
	dataHash string
}

// reconcileTools reconciles both inline and referenced tools, creating ConfigMaps as needed
func (r *AgentReconciler) reconcileTools(ctx context.Context, agent *agentv1alpha1.Agent) ([]toolConfigMapInfo, error) {
	logger := log.FromContext(ctx)
	var toolConfigMaps []toolConfigMapInfo

	for i, tool := range agent.Spec.Tools {
		if tool.Template != nil {
			// Handle inline tool - create an AgentTool CR for it
			toolName := tool.Name
			if toolName == "" {
				toolName = fmt.Sprintf("tool-%d", i)
			}

			cmName, err := r.reconcileInlineTool(ctx, agent, toolName, tool.Template)
			if err != nil {
				return nil, fmt.Errorf("failed to reconcile inline tool %s: %w", toolName, err)
			}

			// Get the ConfigMap to compute hash (may not exist yet if AgentTool controller hasn't run)
			cm := &corev1.ConfigMap{}
			dataHash := ""
			if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agent.Namespace}, cm); err == nil {
				dataHash = hashConfigMapData(cm.Data)
			}

			logger.Info("Reconciled inline tool", "toolName", toolName, "configMap", cmName)
			toolConfigMaps = append(toolConfigMaps, toolConfigMapInfo{
				toolName:      toolName,
				configMapName: cmName,
				dataHash:      dataHash,
			})
		} else if tool.ToolRef != nil {
			// Handle tool reference - look up the AgentTool's ConfigMap
			namespace := tool.ToolRef.Namespace
			if namespace == "" {
				namespace = agent.Namespace
			}

			agentTool := &agentv1alpha1.AgentTool{}
			if err := r.Get(ctx, types.NamespacedName{Name: tool.ToolRef.Name, Namespace: namespace}, agentTool); err != nil {
				return nil, fmt.Errorf("failed to get referenced AgentTool %s/%s: %w", namespace, tool.ToolRef.Name, err)
			}

			// The AgentTool controller creates a ConfigMap named "{agenttool-name}-spec"
			cmName := fmt.Sprintf("%s-spec", agentTool.Name)

			// Verify the ConfigMap exists and compute hash
			cm := &corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: namespace}, cm); err != nil {
				return nil, fmt.Errorf("ConfigMap for AgentTool %s not found: %w", tool.ToolRef.Name, err)
			}

			toolName := tool.Name
			if toolName == "" {
				toolName = agentTool.Name
			}
			logger.Info("Found referenced tool", "toolName", toolName, "configMap", cmName)
			toolConfigMaps = append(toolConfigMaps, toolConfigMapInfo{
				toolName:      toolName,
				configMapName: cmName,
				dataHash:      hashConfigMapData(cm.Data),
			})
		}
	}

	return toolConfigMaps, nil
}

// hashConfigMapData computes a hash of ConfigMap data for change detection
func hashConfigMapData(data map[string]string) string {
	if len(data) == 0 {
		return ""
	}

	// Sort keys for deterministic hashing
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(data[k]))
	}
	return hex.EncodeToString(h.Sum(nil))[:16] // Use first 16 chars
}

// reconcileInlineTool creates or updates an AgentTool CR for an inline tool definition
// The AgentTool controller will then handle creating the ConfigMap
func (r *AgentReconciler) reconcileInlineTool(ctx context.Context, agent *agentv1alpha1.Agent, toolName string, spec *agentv1alpha1.AgentToolSpec) (string, error) {
	agentToolName := fmt.Sprintf("%s-%s", agent.Name, toolName)

	agentTool := &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentToolName,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       toolName,
				"app.kubernetes.io/component":  "inline-tool",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
		},
		Spec: *spec,
	}

	// Set owner reference so the AgentTool is cleaned up when the Agent is deleted
	if err := controllerutil.SetControllerReference(agent, agentTool, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update AgentTool
	existing := &agentv1alpha1.AgentTool{}
	err := r.Get(ctx, types.NamespacedName{Name: agentToolName, Namespace: agent.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, agentTool); err != nil {
				return "", fmt.Errorf("failed to create AgentTool: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to get AgentTool: %w", err)
		}
	} else {
		existing.Spec = agentTool.Spec
		existing.Labels = agentTool.Labels
		if err := r.Update(ctx, existing); err != nil {
			return "", fmt.Errorf("failed to update AgentTool: %w", err)
		}
	}

	// The AgentTool controller creates a ConfigMap named "{agenttool-name}-spec"
	return fmt.Sprintf("%s-spec", agentToolName), nil
}

// reconcileAgentCardConfigMap creates or updates a ConfigMap containing the AgentCard as JSON
func (r *AgentReconciler) reconcileAgentCardConfigMap(ctx context.Context, agent *agentv1alpha1.Agent) (string, error) {
	configMapName := fmt.Sprintf("%s-agent-card", agent.Name)

	// Serialize the AgentCard to JSON
	cardJSON, err := json.Marshal(agent.Spec.CardOverride)
	if err != nil {
		return "", fmt.Errorf("failed to marshal AgentCard to JSON: %w", err)
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       agent.Name,
				"app.kubernetes.io/component":  "agent-card",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
		},
		Data: map[string]string{
			agentCardConfigMapKey: string(cardJSON),
		},
	}

	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: agent.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return "", fmt.Errorf("failed to create AgentCard ConfigMap: %w", err)
			}
			return configMapName, nil
		}
		return "", fmt.Errorf("failed to get AgentCard ConfigMap: %w", err)
	}

	existing.Data = desired.Data
	existing.Labels = desired.Labels
	if err := r.Update(ctx, existing); err != nil {
		return "", fmt.Errorf("failed to update AgentCard ConfigMap: %w", err)
	}

	return configMapName, nil
}

// resolvedModelInfo holds the resolved model configuration for use in deployment
type resolvedModelInfo struct {
	// provider is the model provider type
	provider agentv1alpha1.ProviderType
	// model is the model identifier
	model string
	// configMapName is the name of the ConfigMap containing the model config
	configMapName string
	// envVars are non-secret environment variables to add to the container
	envVars []corev1.EnvVar
	// secretEnvVars are environment variables sourced from secrets
	secretEnvVars []corev1.EnvVar
}

// reconcileModel resolves the model reference and creates the model ConfigMap.
// It fetches the Model, then the ModelProvider, and builds the configuration.
func (r *AgentReconciler) reconcileModel(ctx context.Context, agent *agentv1alpha1.Agent) (*resolvedModelInfo, error) {
	logger := log.FromContext(ctx)

	if agent.Spec.Model == nil {
		return nil, nil
	}

	// Resolve the Model reference
	modelNamespace := agent.Spec.Model.Namespace
	if modelNamespace == "" {
		modelNamespace = agent.Namespace
	}

	model := &agentv1alpha1.Model{}
	if err := r.Get(ctx, types.NamespacedName{Name: agent.Spec.Model.Name, Namespace: modelNamespace}, model); err != nil {
		return nil, fmt.Errorf("failed to get Model %s/%s: %w", modelNamespace, agent.Spec.Model.Name, err)
	}

	// Check if Model is ready
	if !model.Status.Ready {
		return nil, fmt.Errorf("model %s/%s is not ready", modelNamespace, model.Name)
	}

	// Resolve the ModelProvider from Model
	providerNamespace := model.Spec.ProviderRef.Namespace
	if providerNamespace == "" {
		providerNamespace = model.Namespace
	}

	modelProvider := &agentv1alpha1.ModelProvider{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.Spec.ProviderRef.Name, Namespace: providerNamespace}, modelProvider); err != nil {
		return nil, fmt.Errorf("failed to get ModelProvider %s/%s: %w", providerNamespace, model.Spec.ProviderRef.Name, err)
	}

	// Check if ModelProvider is ready
	if !modelProvider.Status.Ready {
		return nil, fmt.Errorf("ModelProvider %s/%s is not ready", providerNamespace, modelProvider.Name)
	}

	providerType := modelProvider.GetProviderType()
	logger.Info("Resolved Model and ModelProvider", "model", model.Name, "modelName", model.Spec.Model, "provider", providerType, "modelProvider", modelProvider.Name)

	// Get the provider handler
	providerHandler, ok := GetProviderHandler(providerType)
	if !ok {
		return nil, fmt.Errorf("unsupported model provider: %s", providerType)
	}

	// Build provider-specific configuration
	resolvedConfig, err := providerHandler.BuildConfig(modelProvider, model)
	if err != nil {
		return nil, fmt.Errorf("failed to build provider config: %w", err)
	}

	// Create the model ConfigMap
	configMapName, err := r.reconcileModelConfigMap(ctx, agent, resolvedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile model ConfigMap: %w", err)
	}

	return &resolvedModelInfo{
		provider:      resolvedConfig.Provider.Type,
		model:         resolvedConfig.Model,
		configMapName: configMapName,
		envVars:       resolvedConfig.EnvVars,
		secretEnvVars: resolvedConfig.SecretEnvVars,
	}, nil
}

// reconcileModelConfigMap creates or updates the ConfigMap containing the model configuration.
func (r *AgentReconciler) reconcileModelConfigMap(ctx context.Context, agent *agentv1alpha1.Agent, config *ResolvedModelConfig) (string, error) {
	configMapName := fmt.Sprintf("%s-model", agent.Name)

	// Serialize the config to JSON
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
			modelConfigMapKey: string(configJSON),
		},
	}

	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: agent.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return "", fmt.Errorf("failed to create model ConfigMap: %w", err)
			}
			return configMapName, nil
		}
		return "", fmt.Errorf("failed to get model ConfigMap: %w", err)
	}

	existing.Data = desired.Data
	existing.Labels = desired.Labels
	if err := r.Update(ctx, existing); err != nil {
		return "", fmt.Errorf("failed to update model ConfigMap: %w", err)
	}

	return configMapName, nil
}

func (r *AgentReconciler) buildDeployment(agent *agentv1alpha1.Agent, toolConfigMaps []toolConfigMapInfo, agentCardConfigMap string, modelInfo *resolvedModelInfo, templateConfigMapName string, instructionConfigMapName string) *appsv1.Deployment {
	labels := r.buildLabels(agent)

	overrides := r.getDeploymentOverrides(agent)
	replicas := int32(1)
	if overrides.Replicas != nil {
		replicas = *overrides.Replicas
	}

	var container corev1.Container
	var volumes []corev1.Volume //nolint:prealloc // size depends on runtime type and optional configs

	switch agent.Spec.Runtime.Type {
	case agentv1alpha1.RuntimeTypeManaged:
		container, volumes = r.buildManagedContainerSpec(agent, templateConfigMapName)
	default:
		container, volumes = r.buildStandardContainerSpec(agent)
	}

	// Compute the agent URL for the FLOKOA_AGENT_URL env var
	agentURL := fmt.Sprintf("http://%s.%s.svc.cluster.local", agent.Name, agent.Namespace)

	// Add FLOKOA_AGENT_URL environment variable
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  "FLOKOA_AGENT_URL",
		Value: agentURL,
	})

	// Add agent card ConfigMap volume
	volumes = append(volumes, corev1.Volume{
		Name: agentCardVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: agentCardConfigMap,
				},
			},
		},
	})

	// Add agent card volume mount using SubPath to avoid overwriting /etc/flokoa
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      agentCardVolumeName,
		MountPath: agentCardMountPath,
		SubPath:   agentCardConfigMapKey,
		ReadOnly:  true,
	})

	// Add model configuration if specified
	if modelInfo != nil {
		// Add model ConfigMap volume
		volumes = append(volumes, corev1.Volume{
			Name: modelVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: modelInfo.configMapName,
					},
				},
			},
		})

		// Add model volume mount
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      modelVolumeName,
			MountPath: modelMountPath,
			SubPath:   modelConfigMapKey,
			ReadOnly:  true,
		})

		// Add non-secret environment variables from provider config
		container.Env = append(container.Env, modelInfo.envVars...)

		// Add secret-sourced environment variables
		container.Env = append(container.Env, modelInfo.secretEnvVars...)
	}

	// Add instruction ConfigMap volume mount (works for both standard and inline agents)
	if instructionConfigMapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: instructionVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: instructionConfigMapName,
					},
				},
			},
		})

		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      instructionVolumeName,
			MountPath: instructionMountPath,
			SubPath:   instructionConfigMapKey,
			ReadOnly:  true,
		})

		// Set env var so the runtime knows where to find the instruction
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "FLOKOA_INSTRUCTION_PATH",
			Value: instructionMountPath,
		})
	}

	// Add tool volume mounts to the container and compute combined hash
	var toolsHashBuilder string
	for _, toolCM := range toolConfigMaps {
		// Create a volume for each tool ConfigMap
		volumeName := fmt.Sprintf("tool-%s", toolCM.toolName)
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: toolCM.configMapName,
					},
				},
			},
		})

		// Add volume mount for this tool - each tool gets its own subdirectory
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: fmt.Sprintf("%s/%s", toolsMountPath, toolCM.toolName),
			ReadOnly:  true,
		})

		// Accumulate hashes for combined annotation
		toolsHashBuilder += toolCM.toolName + ":" + toolCM.dataHash + ";"
	}

	// Compute combined tools hash for pod annotation
	var podAnnotations map[string]string
	if len(toolConfigMaps) > 0 {
		h := sha256.Sum256([]byte(toolsHashBuilder))
		podAnnotations = map[string]string{
			"flokoa.ai/tools-hash": hex.EncodeToString(h[:])[:16],
		}
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers:         []corev1.Container{container},
					Volumes:            volumes,
					ImagePullSecrets:   overrides.ImagePullSecrets,
					ServiceAccountName: overrides.ServiceAccountName,
					SecurityContext:    overrides.SecurityContext,
					NodeSelector:       overrides.NodeSelector,
					Tolerations:        overrides.Tolerations,
					Affinity:           overrides.Affinity,
				},
			},
		},
	}
}

// getDeploymentOverrides extracts the shared DeploymentOverrides from the agent's runtime spec.
func (r *AgentReconciler) getDeploymentOverrides(agent *agentv1alpha1.Agent) agentv1alpha1.DeploymentOverrides {
	switch agent.Spec.Runtime.Type {
	case agentv1alpha1.RuntimeTypeManaged:
		if agent.Spec.Runtime.Template != nil {
			return agent.Spec.Runtime.Template.DeploymentOverrides
		}
	case agentv1alpha1.RuntimeTypeStandard:
		if agent.Spec.Runtime.Standard != nil {
			return agent.Spec.Runtime.Standard.DeploymentOverrides
		}
	}
	return agentv1alpha1.DeploymentOverrides{}
}

// buildStandardContainerSpec builds the container and volumes for a standard runtime agent.
func (r *AgentReconciler) buildStandardContainerSpec(agent *agentv1alpha1.Agent) (corev1.Container, []corev1.Volume) {
	spec := agent.Spec.Runtime.Standard
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

// buildManagedContainerSpec builds the container and volumes for a managed runtime agent.
// The operator generates the container using a generic runtime image with config mounted from ConfigMaps.
func (r *AgentReconciler) buildManagedContainerSpec(agent *agentv1alpha1.Agent, templateConfigMapName string) (corev1.Container, []corev1.Volume) {
	managed := agent.Spec.Runtime.Template
	if managed == nil {
		managed = &agentv1alpha1.TemplatedRuntimeSpec{}
	}

	container := corev1.Container{
		Name:  "agent",
		Image: defaultManagedRuntimeImage,
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
				Value: "managed",
			},
			{
				Name:  "FLOKOA_MANAGED_CONFIG_PATH",
				Value: templateConfigMountPath,
			},
		}, managed.Env...),
	}

	if managed.Resources != nil {
		container.Resources = *managed.Resources
	}

	var volumes []corev1.Volume

	// Mount managed config ConfigMap
	if templateConfigMapName != "" {
		volumes = append(volumes, corev1.Volume{
			Name: templateConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: templateConfigMapName,
					},
				},
			},
		})

		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      templateConfigVolumeName,
			MountPath: templateConfigMountPath,
			SubPath:   templateConfigConfigMapKey,
			ReadOnly:  true,
		})
	}

	return container, volumes
}

func (r *AgentReconciler) reconcileService(ctx context.Context, agent *agentv1alpha1.Agent) (*corev1.Service, error) {
	desired := r.buildService(agent)

	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)

	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return nil, fmt.Errorf("failed to create Service: %w", err)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Service: %w", err)
	}

	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	if err := r.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update Service: %w", err)
	}

	return existing, nil
}

func (r *AgentReconciler) buildService(agent *agentv1alpha1.Agent) *corev1.Service {
	labels := r.buildLabels(agent)

	// Build service ports from container ports
	var servicePorts []corev1.ServicePort
	if agent.Spec.Runtime.Type == agentv1alpha1.RuntimeTypeStandard && agent.Spec.Runtime.Standard != nil {
		for _, cp := range agent.Spec.Runtime.Standard.Container.Ports {
			servicePorts = append(servicePorts, corev1.ServicePort{
				Name:       cp.Name,
				Port:       cp.ContainerPort,
				TargetPort: intstr.FromInt32(cp.ContainerPort),
				Protocol:   cp.Protocol,
			})
		}
	}

	// Default to port 80 -> 8080 if no ports defined (standard with no ports, or inline)
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
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func (r *AgentReconciler) buildLabels(agent *agentv1alpha1.Agent) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       agent.Name,
		"app.kubernetes.io/instance":   agent.Name,
		"app.kubernetes.io/managed-by": "flokoa-operator",
		"flokoa.ai/agent":              agent.Name,
	}
}

func (r *AgentReconciler) calculatePhase(deployment *appsv1.Deployment) agentv1alpha1.AgentPhase {
	if deployment.Status.AvailableReplicas > 0 {
		return agentv1alpha1.AgentPhaseRunning
	}
	return agentv1alpha1.AgentPhasePending
}

//nolint:unparam // conditionType is parameterized for future extensibility
func (r *AgentReconciler) setCondition(agent *agentv1alpha1.Agent, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: agent.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&agent.Status.Conditions, condition)
}

// validateAgent checks the Agent spec for consistency.
func (r *AgentReconciler) validateAgent(agent *agentv1alpha1.Agent) error {
	// Validate instruction entry if present
	if agent.Spec.Instruction != nil {
		if agent.Spec.Instruction.Template != "" && agent.Spec.Instruction.InstructionRef != nil {
			return fmt.Errorf("instruction.inline and instruction.instructionRef are mutually exclusive")
		}
		if agent.Spec.Instruction.Template == "" && agent.Spec.Instruction.InstructionRef == nil {
			return fmt.Errorf("instruction must have either inline or instructionRef set")
		}
	}

	switch agent.Spec.Runtime.Type {
	case agentv1alpha1.RuntimeTypeStandard:
		if agent.Spec.Runtime.Template != nil {
			return fmt.Errorf("runtime.managed must not be set when runtime.type is %q", agentv1alpha1.RuntimeTypeStandard)
		}
	case agentv1alpha1.RuntimeTypeManaged:
		if agent.Spec.Runtime.Standard != nil {
			return fmt.Errorf("runtime.standard must not be set when runtime.type is %q", agentv1alpha1.RuntimeTypeManaged)
		}
		if agent.Spec.Runtime.Template == nil {
			return fmt.Errorf("runtime.managed is required when runtime.type is %q", agentv1alpha1.RuntimeTypeManaged)
		}
		if agent.Spec.Model == nil {
			return fmt.Errorf("spec.model is required when runtime.type is %q", agentv1alpha1.RuntimeTypeManaged)
		}
		if agent.Spec.Instruction == nil {
			return fmt.Errorf("spec.instruction is required when runtime.type is %q", agentv1alpha1.RuntimeTypeManaged)
		}
	default:
		return fmt.Errorf("unsupported runtime type: %q", agent.Spec.Runtime.Type)
	}
	return nil
}

// templateConfig represents the configuration written to the managed agent's ConfigMap.
// The generic runtime image reads this config to configure the agent.
// Instructions are stored separately in the Instruction CR's ConfigMap.
type templateConfig struct {
	OutputSchema interface{} `json:"outputSchema,omitempty"`
	InputSchema  interface{} `json:"inputSchema,omitempty"`
}

// reconciletemplateConfigMap creates or updates the ConfigMap containing the managed agent configuration.
func (r *AgentReconciler) reconciletemplateConfigMap(ctx context.Context, agent *agentv1alpha1.Agent) (string, error) {
	configMapName := fmt.Sprintf("%s-managed-config", agent.Name)

	managed := agent.Spec.Runtime.Template
	if managed == nil {
		return "", fmt.Errorf("managed spec is nil")
	}

	config := templateConfig{}
	if managed.Config != nil {
		if managed.Config.OutputSchema != nil {
			config.OutputSchema = managed.Config.OutputSchema
		}
		if managed.Config.InputSchema != nil {
			config.InputSchema = managed.Config.InputSchema
		}
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal managed config to JSON: %w", err)
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       agent.Name,
				"app.kubernetes.io/component":  "managed-config",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
		},
		Data: map[string]string{
			templateConfigConfigMapKey: string(configJSON),
		},
	}

	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: agent.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return "", fmt.Errorf("failed to create managed config ConfigMap: %w", err)
			}
			return configMapName, nil
		}
		return "", fmt.Errorf("failed to get managed config ConfigMap: %w", err)
	}

	existing.Data = desired.Data
	existing.Labels = desired.Labels
	if err := r.Update(ctx, existing); err != nil {
		return "", fmt.Errorf("failed to update managed config ConfigMap: %w", err)
	}

	return configMapName, nil
}

// reconcileInstruction handles both inline instruction definitions and instruction references.
// For inline: creates a child Instruction CR, returns its ConfigMap name.
// For ref: looks up the existing Instruction, returns its ConfigMap name.
func (r *AgentReconciler) reconcileInstruction(ctx context.Context, agent *agentv1alpha1.Agent) (string, error) {
	logger := log.FromContext(ctx)

	entry := agent.Spec.Instruction
	if entry == nil {
		return "", nil
	}

	if entry.Template != "" {
		// Inline instruction — create a child Instruction CR
		return r.reconcileInlineInstruction(ctx, agent, entry.Template)
	}

	if entry.InstructionRef != nil {
		// Reference — look up the existing Instruction CR and its ConfigMap
		namespace := entry.InstructionRef.Namespace
		if namespace == "" {
			namespace = agent.Namespace
		}

		instruction := &agentv1alpha1.Instruction{}
		if err := r.Get(ctx, types.NamespacedName{Name: entry.InstructionRef.Name, Namespace: namespace}, instruction); err != nil {
			return "", fmt.Errorf("failed to get referenced Instruction %s/%s: %w", namespace, entry.InstructionRef.Name, err)
		}

		if instruction.Status.ConfigMapName == "" {
			return "", fmt.Errorf("instruction %s/%s has no ConfigMap yet (not reconciled)", namespace, instruction.Name)
		}

		logger.Info("Resolved instruction reference", "instruction", instruction.Name, "configMap", instruction.Status.ConfigMapName)
		return instruction.Status.ConfigMapName, nil
	}

	return "", fmt.Errorf("instruction entry has neither inline nor instructionRef set")
}

// reconcileInlineInstruction creates or updates an Instruction CR for an inline instruction definition.
// The Instruction controller handles creating the ConfigMap.
func (r *AgentReconciler) reconcileInlineInstruction(ctx context.Context, agent *agentv1alpha1.Agent, content string) (string, error) {
	instructionName := fmt.Sprintf("%s-instruction", agent.Name)

	desired := &agentv1alpha1.Instruction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instructionName,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       agent.Name,
				"app.kubernetes.io/component":  "inline-instruction",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
		},
		Spec: agentv1alpha1.InstructionSpec{
			Content: content,
		},
	}

	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &agentv1alpha1.Instruction{}
	err := r.Get(ctx, types.NamespacedName{Name: instructionName, Namespace: agent.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return "", fmt.Errorf("failed to create Instruction: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to get Instruction: %w", err)
		}
	} else {
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		if err := r.Update(ctx, existing); err != nil {
			return "", fmt.Errorf("failed to update Instruction: %w", err)
		}
	}

	// The Instruction controller creates a ConfigMap named "{instruction-name}-instruction"
	configMapName := fmt.Sprintf("%s-instruction", instructionName)

	// Check if the ConfigMap exists yet (the Instruction controller may not have run yet)
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: agent.Namespace}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			// ConfigMap not yet created by Instruction controller — return the expected name
			// The agent will be re-reconciled when the Instruction controller creates it
			return configMapName, nil
		}
		return "", fmt.Errorf("failed to check Instruction ConfigMap: %w", err)
	}

	return configMapName, nil
}

// findAgentsForInstruction returns the Agents that reference a given Instruction
func (r *AgentReconciler) findAgentsForInstruction(ctx context.Context, obj client.Object) []reconcile.Request {
	instruction := obj.(*agentv1alpha1.Instruction)
	logger := log.FromContext(ctx)

	// Check if this is an inline instruction (owned by an Agent)
	if agentName, ok := instruction.Labels["flokoa.ai/agent"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      agentName,
				Namespace: instruction.Namespace,
			},
		}}
	}

	// List all agents in the same namespace to find those referencing this Instruction
	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(instruction.Namespace)); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		if agent.Spec.Instruction != nil && agent.Spec.Instruction.InstructionRef != nil {
			refNamespace := agent.Spec.Instruction.InstructionRef.Namespace
			if refNamespace == "" {
				refNamespace = agent.Namespace
			}
			if agent.Spec.Instruction.InstructionRef.Name == instruction.Name && refNamespace == instruction.Namespace {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      agent.Name,
						Namespace: agent.Namespace,
					},
				})
			}
		}
	}

	return requests
}

func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&agentv1alpha1.AgentTool{}).
		Owns(&agentv1alpha1.Instruction{}).
		Watches(
			&agentv1alpha1.AgentTool{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForAgentTool),
		).
		Watches(
			&agentv1alpha1.Instruction{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForInstruction),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForConfigMap),
		).
		Named("agent").
		Complete(r)
}

// findAgentsForAgentTool returns the Agents that reference a given AgentTool
func (r *AgentReconciler) findAgentsForAgentTool(ctx context.Context, obj client.Object) []reconcile.Request {
	agentTool := obj.(*agentv1alpha1.AgentTool)
	logger := log.FromContext(ctx)

	// Check if this is an inline tool (owned by an Agent)
	if agentName, ok := agentTool.Labels["flokoa.ai/agent"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      agentName,
				Namespace: agentTool.Namespace,
			},
		}}
	}

	// List all agents in the same namespace to find those referencing this AgentTool
	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(agentTool.Namespace)); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		// Check if this agent references the updated AgentTool
		for _, tool := range agent.Spec.Tools {
			if tool.ToolRef != nil {
				refNamespace := tool.ToolRef.Namespace
				if refNamespace == "" {
					refNamespace = agent.Namespace
				}
				if tool.ToolRef.Name == agentTool.Name && refNamespace == agentTool.Namespace {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      agent.Name,
							Namespace: agent.Namespace,
						},
					})
					break
				}
			}
		}
	}

	return requests
}

// findAgentsForConfigMap returns the Agents that use a given ConfigMap (for tool specs)
func (r *AgentReconciler) findAgentsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	cm := obj.(*corev1.ConfigMap)

	// Only care about tool spec or instruction ConfigMaps
	component := cm.Labels["app.kubernetes.io/component"]
	if component != "agenttool-spec" && component != "inline-tool-spec" && component != "instruction" {
		return nil
	}

	// Instruction ConfigMaps are handled via Instruction watcher
	if component == "instruction" {
		return nil
	}

	// If this is an inline tool ConfigMap, it has the agent label
	if agentName, ok := cm.Labels["flokoa.ai/agent"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      agentName,
				Namespace: cm.Namespace,
			},
		}}
	}

	// For standalone AgentTool ConfigMaps, find agents referencing the AgentTool
	// ConfigMap name is "{agenttool-name}-spec", so extract the AgentTool name
	agentToolName := cm.Labels["app.kubernetes.io/name"]
	if agentToolName == "" {
		return nil
	}

	logger := log.FromContext(ctx)
	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(cm.Namespace)); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		for _, tool := range agent.Spec.Tools {
			if tool.ToolRef != nil {
				refNamespace := tool.ToolRef.Namespace
				if refNamespace == "" {
					refNamespace = agent.Namespace
				}
				if tool.ToolRef.Name == agentToolName && refNamespace == cm.Namespace {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      agent.Name,
							Namespace: agent.Namespace,
						},
					})
					break
				}
			}
		}
	}

	return requests
}
