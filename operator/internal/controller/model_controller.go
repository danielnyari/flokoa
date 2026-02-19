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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	modeldomain "github.com/danielnyari/flokoa/internal/domain/model"
)

// Condition types for Model
const (
	ModelConditionTypeReady            = "Ready"
	ModelConditionTypeProviderResolved = "ProviderResolved"
	ModelConditionTypeValidated        = "Validated"
)

// Condition reasons for Model
const (
	ModelReasonResolved               = "Resolved"
	ModelReasonProviderNotFound       = "ProviderNotFound"
	ModelReasonProviderNotReady       = "ProviderNotReady"
	ModelReasonValidated              = "Validated"
	ModelReasonProviderParamsMismatch = "ProviderParametersMismatch"
	ModelReasonMultipleProviderParams = "MultipleProviderParameters"
	modelRetryInterval                = 2 * time.Second
)

// ModelReconciler reconciles a Model object
type ModelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=models,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=models/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=models/finalizers,verbs=update
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=modelproviders,verbs=get;list;watch

// Reconcile validates the Model and resolves its referenced ModelProvider
func (r *ModelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the Model
	var model agentv1alpha1.Model
	if err := r.Get(ctx, req.NamespacedName, &model); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling Model", "name", model.Name, "model", model.Spec.Model)

	// Resolve the ModelProvider
	providerNamespace := model.Spec.ProviderRef.Namespace
	if providerNamespace == "" {
		providerNamespace = model.Namespace
	}

	var modelProvider agentv1alpha1.ModelProvider
	providerKey := types.NamespacedName{
		Name:      model.Spec.ProviderRef.Name,
		Namespace: providerNamespace,
	}

	if err := r.Get(ctx, providerKey, &modelProvider); err != nil {
		if errors.IsNotFound(err) {
			r.setNotReady(&model, ModelReasonProviderNotFound,
				fmt.Sprintf("ModelProvider %s/%s not found", providerNamespace, model.Spec.ProviderRef.Name))
			if err := updateStatusWithRetry(ctx, r.Client, &model, func() {
				r.setNotReady(&model, ModelReasonProviderNotFound,
					fmt.Sprintf("ModelProvider %s/%s not found", providerNamespace, model.Spec.ProviderRef.Name))
			}); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: modelRetryInterval}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if ModelProvider is ready
	if !modelProvider.Status.Ready {
		r.setNotReady(&model, ModelReasonProviderNotReady,
			fmt.Sprintf("ModelProvider %s/%s is not ready", providerNamespace, model.Spec.ProviderRef.Name))
		if err := updateStatusWithRetry(ctx, r.Client, &model, func() {
			r.setNotReady(&model, ModelReasonProviderNotReady,
				fmt.Sprintf("ModelProvider %s/%s is not ready", providerNamespace, model.Spec.ProviderRef.Name))
		}); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: modelRetryInterval}, nil
	}

	// Validate that provider-specific parameters match the provider type
	if err := modeldomain.ValidateProviderParams(model.Spec.Parameters, modelProvider.Status.Provider); err != nil {
		r.setNotReady(&model, ModelReasonProviderParamsMismatch, err.Error())
		if updateErr := updateStatusWithRetry(ctx, r.Client, &model, func() {
			r.setNotReady(&model, ModelReasonProviderParamsMismatch, err.Error())
		}); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, nil
	}

	// Update status with resolved provider info
	model.Status.ResolvedProvider = &agentv1alpha1.ResolvedProviderInfo{
		Provider:  modelProvider.Status.Provider,
		Namespace: modelProvider.Namespace,
		Name:      modelProvider.Name,
	}
	model.Status.Ready = true
	model.Status.ObservedGeneration = model.Generation

	meta.SetStatusCondition(&model.Status.Conditions, metav1.Condition{
		Type:               ModelConditionTypeProviderResolved,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: model.Generation,
		Reason:             ModelReasonResolved,
		Message:            fmt.Sprintf("Provider resolved: %s (%s/%s)", modelProvider.Status.Provider, modelProvider.Namespace, modelProvider.Name),
	})
	meta.SetStatusCondition(&model.Status.Conditions, metav1.Condition{
		Type:               ModelConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: model.Generation,
		Reason:             ModelReasonValidated,
		Message:            "Model parameters validated",
	})
	meta.SetStatusCondition(&model.Status.Conditions, metav1.Condition{
		Type:               ModelConditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: model.Generation,
		Reason:             ModelReasonResolved,
		Message:            "Model is ready",
	})

	desiredStatus := model.Status.DeepCopy()
	if err := updateStatusWithRetry(ctx, r.Client, &model, func() {
		model.Status = *desiredStatus
	}); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Model reconciled successfully", "name", model.Name, "model", model.Spec.Model, "provider", modelProvider.Status.Provider)
	return ctrl.Result{}, nil
}

// setNotReady sets the Model status to not ready with the given reason and message
func (r *ModelReconciler) setNotReady(model *agentv1alpha1.Model, reason, message string) {
	model.Status.Ready = false
	model.Status.ObservedGeneration = model.Generation

	meta.SetStatusCondition(&model.Status.Conditions, metav1.Condition{
		Type:               ModelConditionTypeReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: model.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Model{}).
		Watches(&agentv1alpha1.ModelProvider{}, handler.EnqueueRequestsFromMapFunc(
			func(ctx context.Context, obj client.Object) []reconcile.Request {
				provider := obj.(*agentv1alpha1.ModelProvider)

				// Find all Models in the same namespace that reference this provider
				var models agentv1alpha1.ModelList
				if err := r.List(ctx, &models, client.InNamespace(provider.Namespace)); err != nil {
					return nil
				}

				var requests []reconcile.Request
				for _, model := range models.Items {
					providerNs := model.Spec.ProviderRef.Namespace
					if providerNs == "" {
						providerNs = model.Namespace
					}
					if model.Spec.ProviderRef.Name == provider.Name && providerNs == provider.Namespace {
						requests = append(requests, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      model.Name,
								Namespace: model.Namespace,
							},
						})
					}
				}
				return requests
			},
		)).
		Named("model").
		Complete(r)
}
