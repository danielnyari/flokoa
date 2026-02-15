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
	"fmt"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const (
	agentWorkflowFinalizer = "agent.flokoa.ai/agentworkflow-finalizer"

	// Condition types
	ConditionTypeWorkflowCompiled  = "Compiled"
	ConditionTypeWorkflowSubmitted = "Submitted"
	ConditionTypeWorkflowReady     = "Ready"

	// Reasons
	ReasonCompiled           = "Compiled"
	ReasonCompileFailed      = "CompileFailed"
	ReasonSubmitted          = "Submitted"
	ReasonSubmitFailed       = "SubmitFailed"
	ReasonWorkflowRunning    = "WorkflowRunning"
	ReasonWorkflowComplete   = "WorkflowComplete"
	ReasonWorkflowFailed     = "WorkflowFailed"
	ReasonWorkflowError      = "WorkflowError"
	ReasonEngineNotSupported = "EngineNotSupported"

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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AgentWorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling AgentWorkflow", "name", req.Name, "namespace", req.Namespace)

	awf := &agentv1alpha1.AgentWorkflow{}
	if err := r.Get(ctx, req.NamespacedName, awf); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
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

	// Check engine support
	if awf.Spec.Engine == agentv1alpha1.EngineTypeTemporal {
		r.setCondition(awf, ConditionTypeWorkflowCompiled, metav1.ConditionFalse,
			ReasonEngineNotSupported, "temporal engine is not yet supported")
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseError
		awf.Status.ObservedGeneration = awf.Generation
		if err := r.Status().Update(ctx, awf); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
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
	logger := log.FromContext(ctx)

	// Set phase to Compiling
	awf.Status.Phase = agentv1alpha1.WorkflowPhaseCompiling
	awf.Status.ObservedGeneration = awf.Generation
	if err := r.Status().Update(ctx, awf); err != nil {
		return ctrl.Result{}, err
	}

	// Compile DSL to Argo Workflow
	argoWf, err := compileToArgoWorkflow(awf)
	if err != nil {
		logger.Error(err, "Failed to compile AgentWorkflow")
		r.setCondition(awf, ConditionTypeWorkflowCompiled, metav1.ConditionFalse,
			ReasonCompileFailed, err.Error())
		awf.Status.Phase = agentv1alpha1.WorkflowPhaseError
		_ = r.Status().Update(ctx, awf)
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
