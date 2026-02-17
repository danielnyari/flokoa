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

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	modelproviderdomain "github.com/danielnyari/flokoa/internal/domain/modelprovider"
)

// Condition types for ModelProvider
const (
	ModelProviderConditionTypeReady     = "Ready"
	ModelProviderConditionTypeValidated = "Validated"
)

// Condition reasons for ModelProvider
const (
	ModelProviderReasonValidated         = "Validated"
	ModelProviderReasonNoProviderSet     = "NoProviderSet"
	ModelProviderReasonMultipleProviders = "MultipleProvidersSet"
)

// ModelProviderReconciler reconciles a ModelProvider object
type ModelProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=modelproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=modelproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=modelproviders/finalizers,verbs=update

// Reconcile validates the ModelProvider and updates its status
func (r *ModelProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the ModelProvider
	var modelProvider agentv1alpha1.ModelProvider
	if err := r.Get(ctx, req.NamespacedName, &modelProvider); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("Reconciling ModelProvider", "name", modelProvider.Name)

	// Validate that exactly one provider is set
	providerType, err := modelproviderdomain.ValidateProvider(&modelProvider)
	if err != nil {
		// Set condition to indicate validation failure
		meta.SetStatusCondition(&modelProvider.Status.Conditions, metav1.Condition{
			Type:               ModelProviderConditionTypeValidated,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: modelProvider.Generation,
			Reason:             ModelProviderReasonNoProviderSet,
			Message:            err.Error(),
		})
		meta.SetStatusCondition(&modelProvider.Status.Conditions, metav1.Condition{
			Type:               ModelProviderConditionTypeReady,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: modelProvider.Generation,
			Reason:             ModelProviderReasonNoProviderSet,
			Message:            err.Error(),
		})
		modelProvider.Status.Ready = false
		modelProvider.Status.Provider = ""
		modelProvider.Status.ObservedGeneration = modelProvider.Generation

		if err := r.Status().Update(ctx, &modelProvider); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Update status with resolved provider
	modelProvider.Status.Provider = providerType
	modelProvider.Status.Ready = true
	modelProvider.Status.ObservedGeneration = modelProvider.Generation

	meta.SetStatusCondition(&modelProvider.Status.Conditions, metav1.Condition{
		Type:               ModelProviderConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: modelProvider.Generation,
		Reason:             ModelProviderReasonValidated,
		Message:            fmt.Sprintf("Provider type resolved: %s", providerType),
	})
	meta.SetStatusCondition(&modelProvider.Status.Conditions, metav1.Condition{
		Type:               ModelProviderConditionTypeReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: modelProvider.Generation,
		Reason:             ModelProviderReasonValidated,
		Message:            "ModelProvider is ready",
	})

	if err := r.Status().Update(ctx, &modelProvider); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("ModelProvider reconciled successfully", "name", modelProvider.Name, "provider", providerType)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.ModelProvider{}).
		Named("modelprovider").
		Complete(r)
}
