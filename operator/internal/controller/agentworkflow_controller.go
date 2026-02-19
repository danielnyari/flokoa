/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/domain/hash"
	"github.com/danielnyari/flokoa/internal/telemetry"
)

const (
	agentWorkflowFinalizer = "agent.flokoa.ai/agentworkflow-finalizer"

	// Condition types
	ConditionTypeWorkflowCompiled  = "Compiled"
	ConditionTypeWorkflowSubmitted = "Submitted"
	ConditionTypeWorkflowReady     = "Ready"

	// Reasons
	ReasonCompiled         = "Compiled"
	ReasonCompileFailed    = "CompileFailed"
	ReasonSubmitted        = "Submitted"
	ReasonSubmitFailed     = "SubmitFailed"
	ReasonWorkflowRunning  = "WorkflowRunning"
	ReasonWorkflowComplete = "WorkflowComplete"
	ReasonWorkflowFailed   = "WorkflowFailed"
	ReasonWorkflowError    = "WorkflowError"

	// Requeue interval for monitoring running workflows
	workflowPollInterval = 15 * time.Second
)

// AgentWorkflowReconciler reconciles a AgentWorkflow object
type AgentWorkflowReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agentworkflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agentworkflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agentworkflows/finalizers,verbs=update
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=models,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=modelproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=instructions,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AgentWorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx, span := telemetry.Tracer("flokoa.operator").Start(ctx, "agentworkflow.reconcile",
		trace.WithAttributes(
			attribute.String("workflow.name", req.Name),
			attribute.String("workflow.namespace", req.Namespace),
		),
	)
	defer span.End()

	logger := log.FromContext(ctx)
	logger.Info("Reconciling AgentWorkflow", "name", req.Name, "namespace", req.Namespace)

	awf := &agentv1alpha1.AgentWorkflow{}
	if err := r.Get(ctx, req.NamespacedName, awf); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to get AgentWorkflow")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !awf.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(awf, agentWorkflowFinalizer) {
			// Clean up the Argo Workflow if it exists
			if awf.Status.ArgoWorkflowName != "" {
				argoWf := &wfv1.Workflow{}
				err := r.Get(ctx, client.ObjectKey{
					Name:      awf.Status.ArgoWorkflowName,
					Namespace: awf.Namespace,
				}, argoWf)
				if err == nil {
					logger.Info("Deleting Argo Workflow", "name", awf.Status.ArgoWorkflowName)
					if err := r.Delete(ctx, argoWf); err != nil && !apierrors.IsNotFound(err) {
						return ctrl.Result{}, err
					}
				}
			}

			controllerutil.RemoveFinalizer(awf, agentWorkflowFinalizer)
			if err := r.Update(ctx, awf); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(awf, agentWorkflowFinalizer) {
		controllerutil.AddFinalizer(awf, agentWorkflowFinalizer)
		if err := r.Update(ctx, awf); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// If we already have a submitted Argo Workflow, monitor its status
	if awf.Status.ArgoWorkflowName != "" && awf.Status.Phase != agentv1alpha1.WorkflowPhaseError {
		return r.monitorArgoWorkflow(ctx, awf)
	}

	// Compile and submit
	return r.compileAndSubmit(ctx, awf)
}

// compileAndSubmit compiles the AgentWorkflow DSL to an Argo Workflow and submits it.
func (r *AgentWorkflowReconciler) compileAndSubmit(ctx context.Context, awf *agentv1alpha1.AgentWorkflow) (ctrl.Result, error) {
	ctx, span := telemetry.Tracer("flokoa.operator").Start(ctx, "agentworkflow.compile_and_submit",
		trace.WithAttributes(
			attribute.String("workflow.name", awf.Name),
			attribute.String("workflow.namespace", awf.Namespace),
			attribute.Int("workflow.task_count", len(awf.Spec.Tasks)),
		),
	)
	defer span.End()

	logger := log.FromContext(ctx)

	// Extract W3C traceparent from the current span context so it can be
	// propagated into the Argo Workflow and downstream agent processes.
	traceparent := telemetry.ExtractTraceparent(ctx)

	// Set phase to Compiling
	awf.Status.Phase = agentv1alpha1.WorkflowPhaseCompiling
	awf.Status.ObservedGeneration = awf.Generation
	if err := r.Status().Update(ctx, awf); err != nil {
		span.RecordError(err)
		return ctrl.Result{}, err
	}

	// Resolve Model/Tool/Instruction references for agentTask tasks
	resolvedTasks, err := r.resolveAgentTasks(ctx, awf)
	if err != nil {
		logger.Error(err, "Failed to resolve agent task dependencies")
		r.setCondition(awf, ConditionTypeWorkflowCompiled, metav1.ConditionFalse,
			ReasonCompileFailed, fmt.Sprintf("Failed to resolve dependencies: %v", err))
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseError
		_ = r.Status().Update(ctx, awf)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to resolve dependencies")
		return ctrl.Result{}, err
	}

	// Compile DSL to Argo Workflow, injecting trace context for downstream propagation.
	argoWf, err := compileToArgoWorkflow(awf, resolvedTasks, traceparent)
	if err != nil {
		logger.Error(err, "Failed to compile AgentWorkflow")
		r.setCondition(awf, ConditionTypeWorkflowCompiled, metav1.ConditionFalse,
			ReasonCompileFailed, err.Error())
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseError
		_ = r.Status().Update(ctx, awf)
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to compile workflow")
		return ctrl.Result{}, err
	}

	r.setCondition(awf, ConditionTypeWorkflowCompiled, metav1.ConditionTrue,
		ReasonCompiled, "AgentWorkflow compiled to Argo Workflow")

	// Set owner reference so the Argo Workflow is cleaned up with the AgentWorkflow
	if err := controllerutil.SetControllerReference(awf, argoWf, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create the Argo Workflow
	logger.Info("Creating Argo Workflow", "generateName", argoWf.GenerateName)
	if err := r.Create(ctx, argoWf); err != nil {
		logger.Error(err, "Failed to create Argo Workflow")
		r.setCondition(awf, ConditionTypeWorkflowSubmitted, metav1.ConditionFalse,
			ReasonSubmitFailed, err.Error())
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseError
		_ = r.Status().Update(ctx, awf)
		return ctrl.Result{}, err
	}

	// Update status with the created Argo Workflow name
	r.setCondition(awf, ConditionTypeWorkflowSubmitted, metav1.ConditionTrue,
		ReasonSubmitted, fmt.Sprintf("Argo Workflow %q created", argoWf.Name))
	awf.Status.ArgoWorkflowName = argoWf.Name
	awf.Status.Phase = agentv1alpha1.WorkflowPhaseRunning
	now := metav1.Now()
	awf.Status.StartTime = &now

	if err := r.Status().Update(ctx, awf); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue to monitor the workflow
	return ctrl.Result{RequeueAfter: workflowPollInterval}, nil
}

// monitorArgoWorkflow checks the status of the submitted Argo Workflow and updates the AgentWorkflow status.
func (r *AgentWorkflowReconciler) monitorArgoWorkflow(ctx context.Context, awf *agentv1alpha1.AgentWorkflow) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	argoWf := &wfv1.Workflow{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      awf.Status.ArgoWorkflowName,
		Namespace: awf.Namespace,
	}, argoWf)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Argo Workflow not found, may have been deleted", "name", awf.Status.ArgoWorkflowName)
			r.setCondition(awf, ConditionTypeWorkflowReady, metav1.ConditionFalse,
				ReasonWorkflowError, "Argo Workflow not found")
			awf.Status.Phase = agentv1alpha1.WorkflowPhaseError
			_ = r.Status().Update(ctx, awf)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Map Argo Workflow phase to AgentWorkflow phase
	previousPhase := awf.Status.Phase
	switch argoWf.Status.Phase {
	case wfv1.WorkflowSucceeded:
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseSucceeded
		now := metav1.Now()
		awf.Status.CompletionTime = &now
		r.setCondition(awf, ConditionTypeWorkflowReady, metav1.ConditionTrue,
			ReasonWorkflowComplete, "Workflow completed successfully")
	case wfv1.WorkflowFailed:
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseFailed
		now := metav1.Now()
		awf.Status.CompletionTime = &now
		msg := argoWf.Status.Message
		if msg == "" {
			msg = "Workflow failed"
		}
		r.setCondition(awf, ConditionTypeWorkflowReady, metav1.ConditionFalse,
			ReasonWorkflowFailed, msg)
	case wfv1.WorkflowError:
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseError
		now := metav1.Now()
		awf.Status.CompletionTime = &now
		msg := argoWf.Status.Message
		if msg == "" {
			msg = "Workflow encountered an error"
		}
		r.setCondition(awf, ConditionTypeWorkflowReady, metav1.ConditionFalse,
			ReasonWorkflowError, msg)
	default:
		// Still running
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseRunning
		r.setCondition(awf, ConditionTypeWorkflowReady, metav1.ConditionFalse,
			ReasonWorkflowRunning, "Workflow is running")
	}

	// Update task statuses from Argo node statuses
	awf.Status.TaskStatuses = r.extractTaskStatuses(argoWf)

	if err := r.Status().Update(ctx, awf); err != nil {
		return ctrl.Result{}, err
	}

	// If workflow is still running, continue polling
	if awf.Status.Phase == agentv1alpha1.WorkflowPhaseRunning {
		return ctrl.Result{RequeueAfter: workflowPollInterval}, nil
	}

	if previousPhase != awf.Status.Phase {
		logger.Info("Workflow completed", "phase", awf.Status.Phase)
	}

	return ctrl.Result{}, nil
}

// extractTaskStatuses maps Argo Workflow node statuses to AgentWorkflow task statuses.
func (r *AgentWorkflowReconciler) extractTaskStatuses(argoWf *wfv1.Workflow) []agentv1alpha1.WorkflowTaskStatus {
	var statuses []agentv1alpha1.WorkflowTaskStatus

	for _, node := range argoWf.Status.Nodes {
		// Skip non-task nodes (e.g., the DAG root node)
		if node.Type != wfv1.NodeTypePod && node.Type != wfv1.NodeTypePlugin {
			continue
		}

		status := agentv1alpha1.WorkflowTaskStatus{
			Name:    node.DisplayName,
			Message: node.Message,
		}

		switch node.Phase {
		case wfv1.NodeSucceeded:
			status.Phase = agentv1alpha1.WorkflowPhaseSucceeded
		case wfv1.NodeFailed:
			status.Phase = agentv1alpha1.WorkflowPhaseFailed
		case wfv1.NodeError:
			status.Phase = agentv1alpha1.WorkflowPhaseError
		case wfv1.NodeRunning:
			status.Phase = agentv1alpha1.WorkflowPhaseRunning
		default:
			status.Phase = agentv1alpha1.WorkflowPhasePending
		}

		if !node.StartedAt.IsZero() {
			t := metav1.NewTime(node.StartedAt.Time)
			status.StartTime = &t
		}
		if !node.FinishedAt.IsZero() {
			t := metav1.NewTime(node.FinishedAt.Time)
			status.CompletionTime = &t
		}

		statuses = append(statuses, status)
	}

	return statuses
}

// setCondition updates or adds a condition to the AgentWorkflow status.
func (r *AgentWorkflowReconciler) setCondition(awf *agentv1alpha1.AgentWorkflow, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: awf.Generation,
	}
	meta.SetStatusCondition(&awf.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentWorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.AgentWorkflow{}).
		Owns(&wfv1.Workflow{}).
		Named("agentworkflow").
		Complete(r)
}

// resolveAgentTasks resolves Model, Tool, and Instruction references for all agentTask tasks.
// Returns a map of task name -> resolved info for use by the compiler.
func (r *AgentWorkflowReconciler) resolveAgentTasks(ctx context.Context, awf *agentv1alpha1.AgentWorkflow) (map[string]*resolvedAgentTaskInfo, error) {
	result := make(map[string]*resolvedAgentTaskInfo)

	for _, task := range awf.Spec.Tasks {
		if task.AgentTask == nil {
			continue
		}

		info := &resolvedAgentTaskInfo{}

		// Resolve task-level model
		if task.AgentTask.Model != nil {
			modelInfo, err := r.resolveModelRef(ctx, awf, task.Name, task.AgentTask.Model)
			if err != nil {
				return nil, fmt.Errorf("task %q: failed to resolve model: %w", task.Name, err)
			}
			info.modelInfo = modelInfo
		}

		// Resolve inline agent model (if different from task-level)
		if task.AgentTask.Agent != nil && task.AgentTask.Agent.Model != nil {
			agentModelInfo, err := r.resolveModelRef(ctx, awf, task.Name+"-agent", task.AgentTask.Agent.Model)
			if err != nil {
				return nil, fmt.Errorf("task %q: failed to resolve agent model: %w", task.Name, err)
			}
			info.agentModelInfo = agentModelInfo
		}

		// Resolve tools
		if len(task.AgentTask.Tools) > 0 {
			toolCMs, err := r.resolveToolRefs(ctx, awf.Namespace, task.AgentTask.Tools)
			if err != nil {
				return nil, fmt.Errorf("task %q: failed to resolve tools: %w", task.Name, err)
			}
			info.toolConfigMaps = toolCMs
		}

		// Resolve instruction ref
		if task.AgentTask.Instruction != nil && task.AgentTask.Instruction.InstructionRef != nil {
			cmName, err := r.resolveInstructionRef(ctx, awf.Namespace, task.AgentTask.Instruction.InstructionRef)
			if err != nil {
				return nil, fmt.Errorf("task %q: failed to resolve instruction ref: %w", task.Name, err)
			}
			info.instructionConfigMapName = cmName
		}

		result[task.Name] = info
	}

	return result, nil
}

// resolveModelRef resolves an AgentModelRef to a ConfigMap with the model configuration.
func (r *AgentWorkflowReconciler) resolveModelRef(ctx context.Context, awf *agentv1alpha1.AgentWorkflow, taskName string, modelRef *agentv1alpha1.AgentModelRef) (*resolvedModelInfo, error) {
	logger := log.FromContext(ctx)

	modelNamespace := modelRef.Namespace
	if modelNamespace == "" {
		modelNamespace = awf.Namespace
	}

	// Fetch the Model CR
	model := &agentv1alpha1.Model{}
	if err := r.Get(ctx, types.NamespacedName{Name: modelRef.Name, Namespace: modelNamespace}, model); err != nil {
		return nil, fmt.Errorf("failed to get Model %s/%s: %w", modelNamespace, modelRef.Name, err)
	}
	if !model.Status.Ready {
		return nil, fmt.Errorf("Model %s/%s is not ready", modelNamespace, model.Name)
	}

	// Fetch the ModelProvider CR
	providerNamespace := model.Spec.ProviderRef.Namespace
	if providerNamespace == "" {
		providerNamespace = model.Namespace
	}
	modelProvider := &agentv1alpha1.ModelProvider{}
	if err := r.Get(ctx, types.NamespacedName{Name: model.Spec.ProviderRef.Name, Namespace: providerNamespace}, modelProvider); err != nil {
		return nil, fmt.Errorf("failed to get ModelProvider %s/%s: %w", providerNamespace, model.Spec.ProviderRef.Name, err)
	}
	if !modelProvider.Status.Ready {
		return nil, fmt.Errorf("ModelProvider %s/%s is not ready", providerNamespace, modelProvider.Name)
	}

	providerType := modelProvider.GetProviderType()
	if providerType == "" {
		return nil, fmt.Errorf("ModelProvider %s/%s has no provider type configured (must set one of openai, anthropic, google, or bedrock)", providerNamespace, modelProvider.Name)
	}
	logger.Info("Resolved Model for workflow task", "task", taskName, "model", model.Spec.Model, "provider", providerType)

	// Build provider-specific configuration
	providerHandler, ok := GetProviderHandler(providerType)
	if !ok {
		return nil, fmt.Errorf("unsupported model provider: %s", providerType)
	}
	resolvedConfig, err := providerHandler.BuildConfig(modelProvider, model)
	if err != nil {
		return nil, fmt.Errorf("failed to build provider config: %w", err)
	}

	// Create or update the model ConfigMap, owned by the AgentWorkflow
	configMapName := fmt.Sprintf("%s-%s-model", awf.Name, taskName)
	configJSON, err := json.Marshal(resolvedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal model config: %w", err)
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: awf.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/component":        "model-config",
				"app.kubernetes.io/managed-by":       "flokoa-operator",
				"agent.flokoa.ai/agentworkflow-name": awf.Name,
			},
		},
		Data: map[string]string{
			"model.json": string(configJSON),
		},
	}
	if err := controllerutil.SetControllerReference(awf, desired, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference on model ConfigMap: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: configMapName, Namespace: awf.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, desired); err != nil {
				return nil, fmt.Errorf("failed to create model ConfigMap: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to get model ConfigMap: %w", err)
		}
	} else {
		existing.Data = desired.Data
		existing.Labels = desired.Labels
		if err := r.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update model ConfigMap: %w", err)
		}
	}

	return &resolvedModelInfo{
		provider:      resolvedConfig.Provider.Type,
		model:         resolvedConfig.Model,
		configMapName: configMapName,
		envVars:       resolvedConfig.EnvVars,
		secretEnvVars: resolvedConfig.SecretEnvVars,
	}, nil
}

// resolveToolRefs resolves ToolEntry references to their ConfigMap info.
func (r *AgentWorkflowReconciler) resolveToolRefs(ctx context.Context, namespace string, tools []agentv1alpha1.ToolEntry) ([]toolConfigMapInfo, error) {
	var result []toolConfigMapInfo

	for i, tool := range tools {
		if tool.ToolRef != nil {
			// Look up the AgentTool CR and its ConfigMap
			toolNamespace := tool.ToolRef.Namespace
			if toolNamespace == "" {
				toolNamespace = namespace
			}

			agentTool := &agentv1alpha1.AgentTool{}
			if err := r.Get(ctx, types.NamespacedName{Name: tool.ToolRef.Name, Namespace: toolNamespace}, agentTool); err != nil {
				return nil, fmt.Errorf("failed to get AgentTool %s/%s: %w", toolNamespace, tool.ToolRef.Name, err)
			}

			cmName := fmt.Sprintf("%s-spec", agentTool.Name)
			cm := &corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: toolNamespace}, cm); err != nil {
				return nil, fmt.Errorf("ConfigMap for AgentTool %s not found: %w", tool.ToolRef.Name, err)
			}

			toolName := tool.Name
			if toolName == "" {
				toolName = agentTool.Name
			}

			result = append(result, toolConfigMapInfo{
				toolName:      toolName,
				configMapName: cmName,
				dataHash:      hash.ConfigMapData(cm.Data),
			})
		} else if tool.Template != nil {
			// Inline tools are not yet supported in workflows — would need to create AgentTool CRs
			toolName := tool.Name
			if toolName == "" {
				toolName = fmt.Sprintf("tool-%d", i)
			}
			return nil, fmt.Errorf("inline tool definitions are not yet supported in AgentWorkflow tasks (tool %q)", toolName)
		}
	}

	return result, nil
}

// resolveInstructionRef resolves an instruction reference to its ConfigMap name.
func (r *AgentWorkflowReconciler) resolveInstructionRef(ctx context.Context, namespace string, ref *agentv1alpha1.NamespacedRef) (string, error) {
	instructionNamespace := ref.Namespace
	if instructionNamespace == "" {
		instructionNamespace = namespace
	}

	instruction := &agentv1alpha1.Instruction{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: instructionNamespace}, instruction); err != nil {
		return "", fmt.Errorf("failed to get Instruction %s/%s: %w", instructionNamespace, ref.Name, err)
	}

	// The Instruction controller creates a ConfigMap named "{instruction-name}-instruction"
	cmName := instruction.Status.ConfigMapName
	if cmName == "" {
		cmName = fmt.Sprintf("%s-instruction", instruction.Name)
	}

	// Verify it exists
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: instructionNamespace}, cm); err != nil {
		return "", fmt.Errorf("ConfigMap for Instruction %s not found: %w", ref.Name, err)
	}

	return cmName, nil
}
