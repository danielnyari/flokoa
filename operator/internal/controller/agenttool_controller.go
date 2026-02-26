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
	"io"
	"net/http"
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
	tooldomain "github.com/danielnyari/flokoa/internal/domain/tool"
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
	Scheme     *runtime.Scheme
	HTTPClient *http.Client
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
		if statusErr := updateStatusWithRetry(ctx, r.Client, agentTool, func() {
			r.setCondition(agentTool, ConditionTypeValidated, metav1.ConditionFalse, ReasonValidationFailed, err.Error())
		}); statusErr != nil {
			logger.Error(statusErr, "Failed to update AgentTool status after validation failure")
		}
		return ctrl.Result{}, err
	}
	r.setCondition(agentTool, ConditionTypeValidated, metav1.ConditionTrue, ReasonValidationSuccess, "Spec is valid")

	// Resolve the spec: fetch OpenAPI from endpointPath, read from valueFrom, resolve serviceRef to URL.
	// This produces a materialized spec with openApiSchema.value always populated.
	resolvedSpec, err := r.resolveSpec(ctx, agentTool)
	if err != nil {
		logger.Error(err, "Failed to resolve spec")
		r.setCondition(agentTool, ConditionTypeStored, metav1.ConditionFalse, ReasonStorageFailed, err.Error())
		if statusErr := updateStatusWithRetry(ctx, r.Client, agentTool, func() {
			r.setCondition(agentTool, ConditionTypeStored, metav1.ConditionFalse, ReasonStorageFailed, err.Error())
		}); statusErr != nil {
			logger.Error(statusErr, "Failed to update AgentTool status after resolve failure")
		}
		return ctrl.Result{}, err
	}

	// Create or update ConfigMap with resolved spec content
	if err := r.reconcileConfigMap(ctx, agentTool, resolvedSpec); err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		r.setCondition(agentTool, ConditionTypeStored, metav1.ConditionFalse, ReasonStorageFailed, err.Error())
		if statusErr := updateStatusWithRetry(ctx, r.Client, agentTool, func() {
			r.setCondition(agentTool, ConditionTypeStored, metav1.ConditionFalse, ReasonStorageFailed, err.Error())
		}); statusErr != nil {
			logger.Error(statusErr, "Failed to update AgentTool status after ConfigMap reconciliation failure")
		}
		return ctrl.Result{}, err
	}
	r.setCondition(agentTool, ConditionTypeStored, metav1.ConditionTrue, ReasonStorageSuccess, "Spec stored in ConfigMap")

	// Update observed generation
	agentTool.Status.ObservedGeneration = agentTool.Generation

	desiredStatus := agentTool.Status.DeepCopy()
	if err := updateStatusWithRetry(ctx, r.Client, agentTool, func() {
		agentTool.Status = *desiredStatus
	}); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// validateSpec validates the AgentTool spec including the OpenAPI schema source
func (r *AgentToolReconciler) validateSpec(ctx context.Context, agentTool *agentv1alpha1.AgentTool) error {
	// Pure spec validation (no I/O)
	if err := tooldomain.ValidateSpec(agentTool); err != nil {
		return err
	}

	// I/O validation: validate ConfigMap reference if specified
	if agentTool.Spec.OpenApi != nil && agentTool.Spec.OpenApi.OpenApiSchema.ValueFrom != nil {
		if err := r.validateConfigMapRef(ctx, agentTool.Namespace, agentTool.Spec.OpenApi.OpenApiSchema.ValueFrom); err != nil {
			return fmt.Errorf("openApiSchema.valueFrom validation failed: %w", err)
		}
	}

	return nil
}

// validateConfigMapRef validates that the referenced ConfigMap and key exist
func (r *AgentToolReconciler) validateConfigMapRef(ctx context.Context, namespace string, ref *corev1.ConfigMapKeySelector) error {
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

// resolveSpec produces a materialized copy of the spec with openApiSchema.value populated.
// It handles all three schema sources:
//   - value: used as-is
//   - endpointPath: fetches the OpenAPI spec via HTTP from the resolved URL
//   - valueFrom: reads from the referenced ConfigMap
//
// It also resolves serviceRef to a url so the SDK can reach the service.
func (r *AgentToolReconciler) resolveSpec(ctx context.Context, agentTool *agentv1alpha1.AgentTool) (*agentv1alpha1.AgentToolSpec, error) {
	spec := agentTool.Spec.DeepCopy()

	if spec.OpenApi == nil {
		return spec, nil
	}

	schema := &spec.OpenApi.OpenApiSchema

	// Resolve serviceRef → url so the SDK knows where to send requests
	if spec.OpenApi.URL == "" && spec.OpenApi.ServiceRef != nil {
		spec.OpenApi.URL = resolveServiceRefURL(agentTool.Namespace, spec.OpenApi.ServiceRef)
	}

	// If value is already inline, nothing to resolve
	if schema.Value != nil {
		return spec, nil
	}

	// Resolve from endpointPath: fetch the OpenAPI spec via HTTP
	if schema.EndpointPath != "" {
		if spec.OpenApi.URL == "" {
			return nil, fmt.Errorf("endpointPath requires either url or serviceRef to be set")
		}

		fetchURL := spec.OpenApi.URL + schema.EndpointPath
		body, err := r.fetchOpenAPISpec(ctx, fetchURL, spec.OpenApi.TimeoutSeconds)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch OpenAPI spec from %s: %w", fetchURL, err)
		}

		schema.Value = &runtime.RawExtension{Raw: body}
		return spec, nil
	}

	// Resolve from valueFrom: read the referenced ConfigMap
	if schema.ValueFrom != nil {
		cm := &corev1.ConfigMap{}
		cmKey := client.ObjectKey{
			Name:      schema.ValueFrom.Name,
			Namespace: agentTool.Namespace,
		}
		if err := r.Get(ctx, cmKey, cm); err != nil {
			return nil, fmt.Errorf("failed to get ConfigMap %s for valueFrom: %w", schema.ValueFrom.Name, err)
		}

		data, ok := cm.Data[schema.ValueFrom.Key]
		if !ok {
			return nil, fmt.Errorf("key %s not found in ConfigMap %s", schema.ValueFrom.Key, schema.ValueFrom.Name)
		}

		// Validate it's valid JSON
		if !json.Valid([]byte(data)) {
			return nil, fmt.Errorf("valueFrom ConfigMap %s key %s does not contain valid JSON", schema.ValueFrom.Name, schema.ValueFrom.Key)
		}

		schema.Value = &runtime.RawExtension{Raw: []byte(data)}
		return spec, nil
	}

	return spec, nil
}

// resolveServiceRefURL builds a cluster-internal URL from a ServiceRef.
func resolveServiceRefURL(namespace string, ref *agentv1alpha1.ServiceRef) string {
	svcNamespace := ref.Namespace
	if svcNamespace == "" {
		svcNamespace = namespace
	}
	port := int32(80)
	if ref.Port != nil {
		port = *ref.Port
	}
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", ref.Name, svcNamespace, port)
}

// fetchOpenAPISpec fetches an OpenAPI spec from the given URL.
func (r *AgentToolReconciler) fetchOpenAPISpec(ctx context.Context, url string, timeoutSeconds *int32) ([]byte, error) {
	timeout := 30 * time.Second
	if timeoutSeconds != nil {
		timeout = time.Duration(*timeoutSeconds) * time.Second
	}

	httpClient := r.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if !json.Valid(body) {
		return nil, fmt.Errorf("response is not valid JSON")
	}

	return body, nil
}

// reconcileConfigMap creates or updates a ConfigMap containing the resolved tool definition.
// The ConfigMap uses key "spec.json" with ToolDefinition format: {"name": "...", "spec": {...}}.
// This is the format the Python SDK expects in /etc/flokoa/tools/*.json.
func (r *AgentToolReconciler) reconcileConfigMap(ctx context.Context, agentTool *agentv1alpha1.AgentTool, resolvedSpec *agentv1alpha1.AgentToolSpec) error {
	logger := log.FromContext(ctx)

	specJSON, err := json.Marshal(map[string]any{
		"name": agentTool.Name,
		"spec": resolvedSpec,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal tool definition: %w", err)
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
			"spec.json": string(specJSON),
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
		logger.Info("Updating ConfigMap", "name", cmName)
		existingCM.Data["spec.json"] = string(specJSON)
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
