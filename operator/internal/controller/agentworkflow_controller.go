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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	ConditionTypeWorkflowCompiled = "Compiled"
	ConditionTypeWorkflowReady    = "Ready"

	// Reasons
	ReasonCompiled      = "Compiled"
	ReasonCompileFailed = "CompileFailed"
	ReasonApplied       = "Applied"
	ReasonApplyFailed   = "ApplyFailed"
)

// AgentWorkflowReconciler reconciles a AgentWorkflow object
type AgentWorkflowReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	CompilerOptions CompilerOptions
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agentworkflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agentworkflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agentworkflows/finalizers,verbs=update
// +kubebuilder:rbac:groups=argoproj.io,resources=workflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=workflowtemplates,verbs=get;list;watch;create;update;patch;delete
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
			// Clean up the Argo WorkflowTemplate if it exists
			if awf.Status.WorkflowTemplateName != "" {
				wft := &wfv1.WorkflowTemplate{}
				err := r.Get(ctx, client.ObjectKey{
					Name:      awf.Status.WorkflowTemplateName,
					Namespace: awf.Namespace,
				}, wft)
				if err == nil {
					logger.Info("Deleting Argo WorkflowTemplate", "name", awf.Status.WorkflowTemplateName)
					if err := r.Delete(ctx, wft); err != nil && !apierrors.IsNotFound(err) {
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

	// Compile and apply the WorkflowTemplate
	return r.compileAndApply(ctx, awf)
}

// compileAndApply compiles the AgentWorkflow DSL to an Argo WorkflowTemplate and creates or updates it.
func (r *AgentWorkflowReconciler) compileAndApply(ctx context.Context, awf *agentv1alpha1.AgentWorkflow) (ctrl.Result, error) {
	ctx, span := telemetry.Tracer("flokoa.operator").Start(ctx, "agentworkflow.compile_and_apply",
		trace.WithAttributes(
			attribute.String("workflow.name", awf.Name),
			attribute.String("workflow.namespace", awf.Namespace),
			attribute.Int("workflow.task_count", len(awf.Spec.Tasks)),
		),
	)
	defer span.End()

	logger := log.FromContext(ctx)

	// Compute spec hash for drift detection
	newSpecHash, err := hash.JSONStruct(awf.Spec)
	if err != nil {
		span.RecordError(err)
		return ctrl.Result{}, fmt.Errorf("failed to hash spec: %w", err)
	}

	// Skip recompilation if spec hasn't changed
	if awf.Status.SpecHash == newSpecHash && awf.Status.Ready {
		logger.Info("Spec unchanged, skipping recompilation", "specHash", newSpecHash)
		span.SetAttributes(attribute.Bool("workflow.skipped", true))
		return ctrl.Result{}, nil
	}

	// Mark as not ready while compiling
	awf.Status.Ready = false
	awf.Status.ObservedGeneration = awf.Generation
	if err := updateStatusWithRetry(ctx, r.Client, awf, func() {
		awf.Status.Ready = false
		awf.Status.ObservedGeneration = awf.Generation
	}); err != nil {
		span.RecordError(err)
		return ctrl.Result{}, err
	}

	// Compile DSL to Argo WorkflowTemplate
	wft, err := compileToArgoWorkflowTemplate(awf, r.CompilerOptions)
	if err != nil {
		logger.Error(err, "Failed to compile AgentWorkflow")
		r.setCondition(awf, ConditionTypeWorkflowCompiled, metav1.ConditionFalse,
			ReasonCompileFailed, err.Error())
		awf.Status.Ready = false
		awf.Status.ObservedGeneration = awf.Generation
		desiredStatus := awf.Status.DeepCopy()
		if statusErr := updateStatusWithRetry(ctx, r.Client, awf, func() {
			awf.Status = *desiredStatus
		}); statusErr != nil {
			logger.Error(statusErr, "Failed to update AgentWorkflow status after compilation failure")
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to compile workflow")
		return ctrl.Result{}, err
	}

	r.setCondition(awf, ConditionTypeWorkflowCompiled, metav1.ConditionTrue,
		ReasonCompiled, "AgentWorkflow compiled to Argo WorkflowTemplate")

	// Set owner reference so the Argo WorkflowTemplate is cleaned up with the AgentWorkflow
	if err := controllerutil.SetControllerReference(awf, wft, r.Scheme); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update the Argo WorkflowTemplate
	existing := &wfv1.WorkflowTemplate{}
	err = r.Get(ctx, client.ObjectKey{Name: wft.Name, Namespace: wft.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating Argo WorkflowTemplate", "name", wft.Name)
			if err := r.Create(ctx, wft); err != nil {
				logger.Error(err, "Failed to create Argo WorkflowTemplate")
				r.setCondition(awf, ConditionTypeWorkflowReady, metav1.ConditionFalse,
					ReasonApplyFailed, err.Error())
				awf.Status.Ready = false
				awf.Status.ObservedGeneration = awf.Generation
				desiredStatus := awf.Status.DeepCopy()
				if statusErr := updateStatusWithRetry(ctx, r.Client, awf, func() {
					awf.Status = *desiredStatus
				}); statusErr != nil {
					logger.Error(statusErr, "Failed to update AgentWorkflow status after WorkflowTemplate creation failure")
				}
				return ctrl.Result{}, err
			}
		} else {
			return ctrl.Result{}, fmt.Errorf("failed to get existing WorkflowTemplate: %w", err)
		}
	} else {
		// Update the existing WorkflowTemplate
		existing.Spec = wft.Spec
		existing.Labels = wft.Labels
		logger.Info("Updating Argo WorkflowTemplate", "name", wft.Name)
		if err := r.Update(ctx, existing); err != nil {
			logger.Error(err, "Failed to update Argo WorkflowTemplate")
			r.setCondition(awf, ConditionTypeWorkflowReady, metav1.ConditionFalse,
				ReasonApplyFailed, err.Error())
			awf.Status.Ready = false
			awf.Status.ObservedGeneration = awf.Generation
			desiredStatus := awf.Status.DeepCopy()
			if statusErr := updateStatusWithRetry(ctx, r.Client, awf, func() {
				awf.Status = *desiredStatus
			}); statusErr != nil {
				logger.Error(statusErr, "Failed to update AgentWorkflow status after WorkflowTemplate update failure")
			}
			return ctrl.Result{}, err
		}
	}

	// Update status — template is ready
	r.setCondition(awf, ConditionTypeWorkflowReady, metav1.ConditionTrue,
		ReasonApplied, fmt.Sprintf("Argo WorkflowTemplate %q applied", wft.Name))
	awf.Status.WorkflowTemplateName = wft.Name
	awf.Status.Ready = true
	awf.Status.SpecHash = newSpecHash
	awf.Status.ObservedGeneration = awf.Generation

	desiredStatus := awf.Status.DeepCopy()
	if err := updateStatusWithRetry(ctx, r.Client, awf, func() {
		awf.Status = *desiredStatus
	}); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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
		Owns(&wfv1.WorkflowTemplate{}).
		Named("agentworkflow").
		Complete(r)
}
