package agent

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
	modeldomain "github.com/danielnyari/flokoa/internal/domain/model"
	"github.com/danielnyari/flokoa/internal/infra/builder"
	"github.com/danielnyari/flokoa/internal/infra/repo"
)

// Deps holds the repository dependencies for the agent application service.
type Deps struct {
	AgentTools   repo.AgentToolReader
	AgentToolW   repo.AgentToolWriter
	Models       repo.ModelReader
	Providers    repo.ModelProviderReader
	Instructions repo.InstructionReader
	InstructionW repo.InstructionWriter
	ConfigMaps   repo.ConfigMapRepo
	Deployments  repo.DeploymentRepo
	Services     repo.ServiceRepo
	Secrets      repo.SecretReader
	OwnerSetter  repo.OwnerSetter
}

// ReconcileResult holds the result of a reconciliation.
type ReconcileResult struct {
	// Requeue indicates whether the reconciliation should be requeued.
	Requeue bool
	// Error is any error that occurred during reconciliation.
	Error error
}

// Service is the application-layer orchestrator for Agent reconciliation.
// It uses repository interfaces for all I/O, making it testable with fakes.
type Service struct {
	deps               Deps
	toolReconciler     *ToolReconciler
	modelReconciler    *ModelReconciler
	instrReconciler    *InstructionReconciler
	getProviderHandler func(agentv1alpha1.ProviderType) (modeldomain.ProviderHandler, bool)
}

// NewService creates a new agent application service.
func NewService(deps Deps) *Service {
	s := &Service{
		deps:               deps,
		getProviderHandler: modeldomain.GetProviderHandler,
	}
	s.toolReconciler = &ToolReconciler{
		agentTools: deps.AgentTools,
		agentToolW: deps.AgentToolW,
		configMaps: deps.ConfigMaps,
		owner:      deps.OwnerSetter,
	}
	s.modelReconciler = &ModelReconciler{
		models:             deps.Models,
		providers:          deps.Providers,
		configMaps:         deps.ConfigMaps,
		secrets:            deps.Secrets,
		owner:              deps.OwnerSetter,
		getProviderHandler: s.getProviderHandler,
	}
	s.instrReconciler = &InstructionReconciler{
		instructions: deps.Instructions,
		instructionW: deps.InstructionW,
		configMaps:   deps.ConfigMaps,
		owner:        deps.OwnerSetter,
	}
	return s
}

// Reconcile performs the full agent reconciliation.
// The agent is already fetched and finalizers are handled by the controller.
// This method mutates agent.Status in place.
func (s *Service) Reconcile(ctx context.Context, agent *agentv1alpha1.Agent) ReconcileResult {
	logger := log.FromContext(ctx)

	// Validate the agent spec
	if err := agentdomain.ValidateSpec(agent); err != nil {
		logger.Error(err, "Agent validation failed")
		agent.Status.Phase = agentv1alpha1.AgentPhaseFailed
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonValidationFailed, err.Error())
		return ReconcileResult{} // Don't requeue on validation errors
	}

	// Reconcile tools
	toolConfigMaps, err := s.toolReconciler.Reconcile(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile tools")
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeToolsReady, metav1.ConditionFalse, agentdomain.ReasonToolSyncFailed, err.Error())
		return ReconcileResult{Requeue: true, Error: err}
	}
	if len(agent.Spec.Tools) > 0 {
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeToolsReady, metav1.ConditionTrue, agentdomain.ReasonToolsSynced, fmt.Sprintf("Synced %d tools", len(toolConfigMaps)))
		now := metav1.Now()
		agent.Status.LastToolSync = &now
	}

	// Reconcile Instruction
	var instructionConfigMapName string
	if agent.Spec.Instruction != nil {
		instructionConfigMapName, err = s.instrReconciler.Reconcile(ctx, agent)
		if err != nil {
			logger.Error(err, "Failed to reconcile Instruction")
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeInstructionReady, metav1.ConditionFalse, agentdomain.ReasonInstructionSyncFailed, err.Error())
			return ReconcileResult{Requeue: true, Error: err}
		}
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeInstructionReady, metav1.ConditionTrue, agentdomain.ReasonInstructionResolved, "Instruction resolved")
	}

	// Reconcile AgentCard ConfigMap
	agentCardConfigMap, err := s.reconcileAgentCardConfigMap(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile AgentCard ConfigMap")
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonReconcileError, err.Error())
		return ReconcileResult{Requeue: true, Error: err}
	}

	// Reconcile Model
	var modelInfo *resolvedModelInfo
	if agent.Spec.Model != nil {
		modelInfo, err = s.modelReconciler.Reconcile(ctx, agent)
		if err != nil {
			logger.Error(err, "Failed to reconcile Model")
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeModelReady, metav1.ConditionFalse, agentdomain.ReasonModelResolveFailed, err.Error())
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeModelSecretsReady, metav1.ConditionFalse, agentdomain.ReasonModelResolveFailed, "Model resolution failed")
			return ReconcileResult{Requeue: true, Error: err}
		}
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeModelReady, metav1.ConditionTrue, agentdomain.ReasonModelResolved,
			fmt.Sprintf("Model %s/%s resolved", modelInfo.provider, modelInfo.model))
		if len(modelInfo.missingSecretRefs) > 0 {
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeModelSecretsReady, metav1.ConditionFalse, agentdomain.ReasonModelSecretsMissing,
				fmt.Sprintf("Missing model secrets: %v", modelInfo.missingSecretRefs))
		} else {
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeModelSecretsReady, metav1.ConditionTrue, agentdomain.ReasonModelSecretsResolved, "All model secrets are present")
		}
	}

	// Reconcile template config ConfigMap (if managed runtime)
	var templateConfigMapName string
	if agent.Spec.Runtime.Type == agentv1alpha1.RuntimeTypeTemplate {
		templateConfigMapName, err = s.reconcileTemplateConfigMap(ctx, agent)
		if err != nil {
			logger.Error(err, "Failed to reconcile managed config ConfigMap")
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonReconcileError, err.Error())
			return ReconcileResult{Requeue: true, Error: err}
		}
	}

	// Build and ensure Deployment
	deployment, err := s.reconcileDeployment(ctx, agent, toolConfigMaps, agentCardConfigMap, modelInfo, templateConfigMapName, instructionConfigMapName)
	if err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonReconcileError, err.Error())
		return ReconcileResult{Requeue: true, Error: err}
	}

	// Build and ensure Service
	service, err := s.reconcileService(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile Service")
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonReconcileError, err.Error())
		return ReconcileResult{Requeue: true, Error: err}
	}

	// Update status
	agent.Status.Phase = agentdomain.CalculatePhase(deployment)
	agent.Status.Backend = "core"
	agent.Status.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local", service.Name, service.Namespace)
	agent.Status.Replicas = deployment.Status.Replicas
	agent.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	agent.Status.ObservedGeneration = agent.Generation

	if deployment.Status.AvailableReplicas > 0 {
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionTrue, agentdomain.ReasonDeploymentReady, "Agent is ready")
	} else {
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonDeploymentNotReady, "Waiting for pods")
	}

	return ReconcileResult{}
}

func (s *Service) reconcileDeployment(ctx context.Context, agent *agentv1alpha1.Agent, toolConfigMaps []toolConfigMapInfo, agentCardConfigMap string, modelInfo *resolvedModelInfo, templateConfigMapName string, instructionConfigMapName string) (*appsv1.Deployment, error) {
	toolMounts := make([]builder.ToolMount, 0, len(toolConfigMaps))
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

	desired := builder.BuildDeployment(builder.DeploymentParams{
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

	if err := s.deps.OwnerSetter.SetOwner(agent, desired); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	return s.deps.Deployments.EnsureDeployment(ctx, desired)
}

func (s *Service) reconcileService(ctx context.Context, agent *agentv1alpha1.Agent) (*corev1.Service, error) {
	desired := builder.BuildService(agent.Name, agent.Namespace, agentdomain.Labels(agent), agent.Spec.Runtime)

	if err := s.deps.OwnerSetter.SetOwner(agent, desired); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	return s.deps.Services.EnsureService(ctx, desired)
}

// reconcileAgentCardConfigMap creates or updates a ConfigMap containing the AgentCard as JSON.
func (s *Service) reconcileAgentCardConfigMap(ctx context.Context, agent *agentv1alpha1.Agent) (string, error) {
	configMapName := fmt.Sprintf("%s-agent-card", agent.Name)

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
			builder.AgentCardConfigMapKey: string(cardJSON),
		},
	}

	if err := s.deps.OwnerSetter.SetOwner(agent, desired); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := s.deps.ConfigMaps.EnsureConfigMap(ctx, desired); err != nil {
		return "", err
	}

	return configMapName, nil
}

// reconcileTemplateConfigMap creates or updates the ConfigMap containing the managed agent configuration.
func (s *Service) reconcileTemplateConfigMap(ctx context.Context, agent *agentv1alpha1.Agent) (string, error) {
	configMapName := fmt.Sprintf("%s-template-config", agent.Name)

	managed := agent.Spec.Runtime.Template
	if managed == nil {
		return "", fmt.Errorf("managed spec is nil")
	}

	config := templateConfig{}
	if managed.Config != nil {
		if managed.Config.OutputSchema != nil {
			config.OutputSchema = managed.Config.OutputSchema
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
				"app.kubernetes.io/component":  "template-config",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
		},
		Data: map[string]string{
			builder.TemplateConfigConfigMapKey: string(configJSON),
		},
	}

	if err := s.deps.OwnerSetter.SetOwner(agent, desired); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := s.deps.ConfigMaps.EnsureConfigMap(ctx, desired); err != nil {
		return "", err
	}

	return configMapName, nil
}

// templateConfig represents the configuration written to the managed agent's ConfigMap.
type templateConfig struct {
	OutputSchema interface{} `json:"outputSchema,omitempty"`
	InputSchema  interface{} `json:"inputSchema,omitempty"`
}
