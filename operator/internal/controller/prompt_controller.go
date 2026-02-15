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
	promptFinalizer = "agent.flokoa.ai/prompt-finalizer"

	// promptConfigMapKey is the key in the ConfigMap for the prompt template file
	promptConfigMapKey = "prompt.txt"
)

const (
	ConditionTypePromptStored = "Stored"
	ReasonPromptStored        = "PromptStored"
	ReasonPromptStoreFailed   = "PromptStoreFailed"
	ReasonPromptSourceFailed  = "PromptSourceFailed"
)

// PromptReconciler reconciles a Prompt object
type PromptReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=prompts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=prompts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=prompts/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *PromptReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Prompt", "name", req.Name, "namespace", req.Namespace)

	prompt := &agentv1alpha1.Prompt{}
	if err := r.Get(ctx, req.NamespacedName, prompt); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !prompt.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(prompt, promptFinalizer) {
			if err := r.deleteConfigMap(ctx, prompt); err != nil {
				logger.Error(err, "Failed to delete ConfigMap")
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(prompt, promptFinalizer)
			if err := r.Update(ctx, prompt); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(prompt, promptFinalizer) {
		controllerutil.AddFinalizer(prompt, promptFinalizer)
		if err := r.Update(ctx, prompt); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Resolve the prompt template content from the source
	templateContent, err := r.resolveSource(ctx, prompt)
	if err != nil {
		logger.Error(err, "Failed to resolve prompt source")
		r.setCondition(prompt, ConditionTypePromptStored, metav1.ConditionFalse, ReasonPromptSourceFailed, err.Error())
		_ = r.Status().Update(ctx, prompt)
		return ctrl.Result{}, err
	}

	// Reconcile the ConfigMap containing the prompt template
	configMapName, err := r.reconcileConfigMap(ctx, prompt, templateContent)
	if err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		r.setCondition(prompt, ConditionTypePromptStored, metav1.ConditionFalse, ReasonPromptStoreFailed, err.Error())
		_ = r.Status().Update(ctx, prompt)
		return ctrl.Result{}, err
	}
	r.setCondition(prompt, ConditionTypePromptStored, metav1.ConditionTrue, ReasonPromptStored, "Prompt template stored in ConfigMap")

	// Update status
	prompt.Status.ConfigMapName = configMapName
	prompt.Status.ObservedGeneration = prompt.Generation

	if err := r.Status().Update(ctx, prompt); err != nil {
		logger.Error(err, "Failed to update Prompt status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// resolveSource resolves the prompt template content from the configured source.
func (r *PromptReconciler) resolveSource(ctx context.Context, prompt *agentv1alpha1.Prompt) (string, error) {
	source := prompt.Spec.Source

	if source.Value != nil {
		return *source.Value, nil
	}

	if source.ValueFrom != nil {
		return r.resolveConfigMapSource(ctx, prompt.Namespace, source.ValueFrom)
	}

	return "", fmt.Errorf("no source specified")
}

// resolveConfigMapSource fetches the prompt template content from a referenced ConfigMap.
func (r *PromptReconciler) resolveConfigMapSource(ctx context.Context, namespace string, ref *corev1.ConfigMapKeySelector) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: namespace}, cm); err != nil {
		return "", fmt.Errorf("failed to get source ConfigMap %q: %w", ref.Name, err)
	}

	key := ref.Key
	content, ok := cm.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in ConfigMap %q", key, ref.Name)
	}

	return content, nil
}

// reconcileConfigMap creates or updates a ConfigMap containing the resolved prompt template content.
func (r *PromptReconciler) reconcileConfigMap(ctx context.Context, prompt *agentv1alpha1.Prompt, content string) (string, error) {
	logger := log.FromContext(ctx)

	configMapName := fmt.Sprintf("%s-prompt", prompt.Name)

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: prompt.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       prompt.Name,
				"app.kubernetes.io/component":  "prompt",
				"app.kubernetes.io/managed-by": "flokoa-operator",
			},
		},
		Data: map[string]string{
			promptConfigMapKey: content,
		},
	}

	if err := controllerutil.SetControllerReference(prompt, desired, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: prompt.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating Prompt ConfigMap", "name", configMapName)
			if err := r.Create(ctx, desired); err != nil {
				return "", fmt.Errorf("failed to create ConfigMap: %w", err)
			}
			return configMapName, nil
		}
		return "", fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	existing.Data = desired.Data
	existing.Labels = desired.Labels
	if err := r.Update(ctx, existing); err != nil {
		return "", fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	return configMapName, nil
}

// deleteConfigMap deletes the ConfigMap associated with the Prompt
func (r *PromptReconciler) deleteConfigMap(ctx context.Context, prompt *agentv1alpha1.Prompt) error {
	configMapName := fmt.Sprintf("%s-prompt", prompt.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: prompt.Namespace,
		},
	}

	err := r.Delete(ctx, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConfigMap: %w", err)
	}

	return nil
}

// setCondition updates or adds a condition to the Prompt status
func (r *PromptReconciler) setCondition(prompt *agentv1alpha1.Prompt, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: prompt.Generation,
	}

	meta.SetStatusCondition(&prompt.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PromptReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Prompt{}).
		Owns(&corev1.ConfigMap{}).
		Named("prompt").
		Complete(r)
}
