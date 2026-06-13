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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	triggerapp "github.com/danielnyari/flokoa/internal/app/trigger"
	triggerdomain "github.com/danielnyari/flokoa/internal/domain/trigger"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
)

const (
	agentTriggerFinalizer  = "agent.flokoa.ai/agenttrigger-finalizer"
	dependencyRequeueDelay = 30 * time.Second
)

// AgentTriggerReconciler reconciles an AgentTrigger object.
type AgentTriggerReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	AppService *triggerapp.Service
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttriggers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttriggers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttriggers/finalizers,verbs=update
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// Trigger secret refs (auth + push-notification tokens) are read by name
// through an uncached APIReader, never cached or watched — get alone.
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get
// +kubebuilder:rbac:groups=argoproj.io,resources=sensors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=argoproj.io,resources=eventsources,verbs=get;list;watch
// +kubebuilder:rbac:groups=argoproj.io,resources=eventbus,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop.
func (r *AgentTriggerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling AgentTrigger", "name", req.Name, "namespace", req.Namespace)

	trigger := &agentv1alpha1.AgentTrigger{}
	if err := r.Get(ctx, req.NamespacedName, trigger); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !trigger.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(trigger, agentTriggerFinalizer) {
			// Cleanup: child Sensor and ConfigMap are garbage-collected via ownerReferences.
			// Remove finalizer.
			controllerutil.RemoveFinalizer(trigger, agentTriggerFinalizer)
			if err := r.Update(ctx, trigger); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(trigger, agentTriggerFinalizer) {
		controllerutil.AddFinalizer(trigger, agentTriggerFinalizer)
		if err := r.Update(ctx, trigger); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Delegate to app service
	result, err := r.AppService.Reconcile(ctx, trigger)
	if err != nil {
		return r.handleReconcileError(ctx, trigger, err)
	}

	// Update status on success
	triggerdomain.SetCondition(trigger, triggerdomain.ConditionTypeReady, metav1.ConditionTrue, triggerdomain.ReasonAllReady, "All conditions met")
	trigger.Status.Phase = triggerdomain.CalculatePhase(trigger)
	trigger.Status.AgentEndpoint = result.AgentEndpoint
	trigger.Status.SensorName = result.SensorName
	trigger.Status.ObservedGeneration = trigger.Generation

	desiredStatus := trigger.Status.DeepCopy()
	if statusErr := updateStatusWithRetry(ctx, r.Client, trigger, func() {
		trigger.Status = *desiredStatus
	}); statusErr != nil {
		logger.Error(statusErr, "Failed to update AgentTrigger status")
		return ctrl.Result{}, statusErr
	}

	return ctrl.Result{}, nil
}

// handleReconcileError classifies errors and updates status accordingly.
func (r *AgentTriggerReconciler) handleReconcileError(ctx context.Context, trigger *agentv1alpha1.AgentTrigger, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	trigger.Status.Phase = triggerdomain.CalculatePhase(trigger)
	trigger.Status.ObservedGeneration = trigger.Generation

	desiredStatus := trigger.Status.DeepCopy()
	if statusErr := updateStatusWithRetry(ctx, r.Client, trigger, func() {
		trigger.Status = *desiredStatus
	}); statusErr != nil {
		logger.Error(statusErr, "Failed to update AgentTrigger status after error")
	}

	if flokoaerrors.IsPermanent(err) {
		logger.Error(err, "Permanent error, will not requeue")
		return ctrl.Result{}, nil
	}
	if flokoaerrors.IsDependency(err) {
		logger.Info("Dependency not ready, requeuing", "error", err.Error())
		return ctrl.Result{RequeueAfter: dependencyRequeueDelay}, nil
	}

	// Transient or unknown error: let controller-runtime handle backoff
	return ctrl.Result{}, err
}

// findTriggersForAgent maps Agent changes to AgentTrigger reconcile requests.
func (r *AgentTriggerReconciler) findTriggersForAgent(ctx context.Context, obj client.Object) []reconcile.Request {
	agent, ok := obj.(*agentv1alpha1.Agent)
	if !ok {
		return nil
	}

	triggerList := &agentv1alpha1.AgentTriggerList{}
	if err := r.List(ctx, triggerList, client.InNamespace(agent.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for i := range triggerList.Items {
		trigger := &triggerList.Items[i]
		// Re-reconcile triggers that reference this agent
		agentNS := trigger.Spec.Agent.Namespace
		if agentNS == "" {
			agentNS = trigger.Namespace
		}
		if trigger.Spec.Agent.Name == agent.Name && agentNS == agent.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      trigger.Name,
					Namespace: trigger.Namespace,
				},
			})
		}

		// Also check push notification target
		if trigger.Spec.PushNotification != nil && trigger.Spec.PushNotification.AgentRef != nil {
			pushNS := trigger.Spec.PushNotification.AgentRef.Namespace
			if pushNS == "" {
				pushNS = trigger.Namespace
			}
			if trigger.Spec.PushNotification.AgentRef.Name == agent.Name && pushNS == agent.Namespace {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      trigger.Name,
						Namespace: trigger.Namespace,
					},
				})
			}
		}
	}

	// Dedup: a trigger referencing the same agent for both spec.agent and pushNotification.agentRef
	// would otherwise be enqueued twice.
	return dedupTriggerRequests(requests)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentTriggerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.AgentTrigger{}).
		Watches(&agentv1alpha1.Agent{},
			handler.EnqueueRequestsFromMapFunc(r.findTriggersForAgent)).
		Named("agenttrigger").
		Complete(r)
}

// dedupTriggerRequests removes duplicate reconcile requests.
func dedupTriggerRequests(requests []reconcile.Request) []reconcile.Request {
	seen := make(map[types.NamespacedName]struct{})
	var result []reconcile.Request
	for _, req := range requests {
		if _, exists := seen[req.NamespacedName]; !exists {
			seen[req.NamespacedName] = struct{}{}
			result = append(result, req)
		}
	}
	return result
}
