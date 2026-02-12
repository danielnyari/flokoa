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

	corev1 "k8s.io/api/core/v1"
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
	agentToolFinalizer = "agent.flokoa.ai/agenttool-finalizer"
)

const (
	ConditionTypeValidated  = "Validated"
	ConditionTypeStored     = "Stored"
	ReasonValidationSuccess = "ValidationSuccess"
	ReasonValidationFailed  = "ValidationFailed"
	ReasonStorageSuccess    = "StorageSuccess"
	ReasonStorageFailed     = "StorageFailed"
)

// AgentToolReconciler reconciles a AgentTool object
type AgentToolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AgentToolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling AgentTool", "name", req.Name, "namespace", req.Namespace)

	agentTool := &agentv1alpha1.AgentTool{}
	if err := r.Get(ctx, req.NamespacedName, agentTool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !agentTool.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(agentTool, agentToolFinalizer) {
			// Delete ConfigMap
			if err := r.deleteConfigMap(ctx, agentTool); err != nil {
				logger.Error(err, "Failed to delete ConfigMap")
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(agentTool, agentToolFinalizer)
			if err := r.Update(ctx, agentTool); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(agentTool, agentToolFinalizer) {
		controllerutil.AddFinalizer(agentTool, agentToolFinalizer)
		if err := r.Update(ctx, agentTool); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Validate the OpenAPI tool spec
	if err := r.validateSpec(ctx, agentTool); err != nil {
		logger.Error(err, "Spec validation failed")
		r.setCondition(agentTool, ConditionTypeValidated, metav1.ConditionFalse, ReasonValidationFailed, err.Error())
		_ = r.Status().Update(ctx, agentTool)
		return ctrl.Result{}, err
	}
	r.setCondition(agentTool, ConditionTypeValidated, metav1.ConditionTrue, ReasonValidationSuccess, "Spec is valid")

	// Create or update ConfigMap with spec content
	if err := r.reconcileConfigMap(ctx, agentTool); err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		r.setCondition(agentTool, ConditionTypeStored, metav1.ConditionFalse, ReasonStorageFailed, err.Error())
		_ = r.Status().Update(ctx, agentTool)
		return ctrl.Result{}, err
	}
	r.setCondition(agentTool, ConditionTypeStored, metav1.ConditionTrue, ReasonStorageSuccess, "Spec stored in ConfigMap")

	// Update observed generation
	agentTool.Status.ObservedGeneration = agentTool.Generation

	if err := r.Status().Update(ctx, agentTool); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// validateSpec validates the AgentTool spec including the OpenAPI schema source
func (r *AgentToolReconciler) validateSpec(ctx context.Context, agentTool *agentv1alpha1.AgentTool) error {
	if agentTool.Spec.OpenApi == nil {
		return fmt.Errorf("openApi is required when type is %q", agentTool.Spec.Type)
	}

	// Validate that exactly one of URL or ServiceRef is specified
	openApi := agentTool.Spec.OpenApi
	if openApi.URL == "" && openApi.ServiceRef == nil {
		return fmt.Errorf("either url or serviceRef must be specified")
	}
	if openApi.URL != "" && openApi.ServiceRef != nil {
		return fmt.Errorf("url and serviceRef are mutually exclusive")
	}

	// Validate the OpenAPI schema source
	schema := &openApi.OpenApiSchema
	sources := 0
	if schema.Value != nil && schema.Value.Raw != nil {
		sources++
	}
	if schema.ValueFrom != nil {
		sources++
	}
	if schema.EndpointPath != "" {
		sources++
	}

	if sources == 0 {
		return fmt.Errorf("openApiSchema is required: exactly one of value, valueFrom, or endpointPath must be specified")
	}
	if sources > 1 {
		return fmt.Errorf("openApiSchema: only one of value, valueFrom, or endpointPath may be specified")
	}

	// Validate valueFrom ConfigMap reference if specified
	if schema.ValueFrom != nil {
		if err := r.validateConfigMapRef(ctx, agentTool.Namespace, schema.ValueFrom); err != nil {
			return fmt.Errorf("openApiSchema.valueFrom validation failed: %w", err)
		}
	}

	return nil
}

// validateConfigMapRef validates that the referenced ConfigMap and key exist
func (r *AgentToolReconciler) validateConfigMapRef(ctx context.Context, namespace string, ref *agentv1alpha1.ConfigMapKeyRef) error {
	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{
		Name:      ref.Name,
		Namespace: namespace,
	}
	if err := r.Get(ctx, key, cm); err != nil {
		return fmt.Errorf("ConfigMap %s not found: %w", ref.Name, err)
	}
	if _, ok := cm.Data[ref.Key]; !ok {
		return fmt.Errorf("key %s not found in ConfigMap %s", ref.Key, ref.Name)
	}
	return nil
}

// reconcileConfigMap creates or updates a ConfigMap containing the AgentTool spec as JSON
func (r *AgentToolReconciler) reconcileConfigMap(ctx context.Context, agentTool *agentv1alpha1.AgentTool) error {
	logger := log.FromContext(ctx)

	// Serialize the spec to JSON
	specJSON, err := json.Marshal(agentTool.Spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}

	cmName := fmt.Sprintf("%s-spec", agentTool.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: agentTool.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       agentTool.Name,
				"app.kubernetes.io/component":  "agenttool-spec",
				"app.kubernetes.io/managed-by": "flokoa-operator",
			},
		},
		Data: map[string]string{
			fmt.Sprintf("%s-spec.json", agentTool.Name): string(specJSON),
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(agentTool, cm, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update ConfigMap
	existingCM := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: cmName, Namespace: agentTool.Namespace}, existingCM)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating ConfigMap", "name", cmName)
			if err := r.Create(ctx, cm); err != nil {
				return fmt.Errorf("failed to create ConfigMap: %w", err)
			}
		} else {
			return fmt.Errorf("failed to get ConfigMap: %w", err)
		}
	} else {
		// Update data with new spec JSON
		logger.Info("Updating ConfigMap", "name", cmName)
		existingCM.Data[fmt.Sprintf("%s-spec.json", agentTool.Name)] = string(specJSON)
		if err := r.Update(ctx, existingCM); err != nil {
			return fmt.Errorf("failed to update ConfigMap: %w", err)
		}
	}

	return nil
}

// deleteConfigMap deletes the ConfigMap associated with the AgentTool
func (r *AgentToolReconciler) deleteConfigMap(ctx context.Context, agentTool *agentv1alpha1.AgentTool) error {
	cmName := fmt.Sprintf("%s-spec", agentTool.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: agentTool.Namespace,
		},
	}

	err := r.Delete(ctx, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConfigMap: %w", err)
	}

	return nil
}

// setCondition updates or adds a condition to the AgentTool status
func (r *AgentToolReconciler) setCondition(agentTool *agentv1alpha1.AgentTool, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: agentTool.Generation,
	}

	meta.SetStatusCondition(&agentTool.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentToolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.AgentTool{}).
		Owns(&corev1.ConfigMap{}).
		Named("agenttool").
		Complete(r)
}
