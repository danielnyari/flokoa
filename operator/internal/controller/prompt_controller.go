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
	"crypto/sha256"
	"encoding/hex"
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
)

// PromptReconciler reconciles a Prompt object
type PromptReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=prompts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=prompts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=prompts/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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
			// Perform any cleanup if needed
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

	// Validate that exactly one source is specified
	if err := r.validateSource(&prompt.Spec.Source); err != nil {
		logger.Error(err, "Invalid prompt source configuration")
		r.setCondition(prompt, "Ready", metav1.ConditionFalse, "InvalidSource", err.Error())
		_ = r.Status().Update(ctx, prompt)
		return ctrl.Result{}, err
	}

	// Resolve the prompt content based on the source
	content, version, err := r.resolvePromptContent(ctx, prompt)
	if err != nil {
		logger.Error(err, "Failed to resolve prompt content")
		r.setCondition(prompt, "Ready", metav1.ConditionFalse, "ResolutionFailed", err.Error())
		_ = r.Status().Update(ctx, prompt)
		return ctrl.Result{}, err
	}

	// Calculate checksum
	checksum := r.calculateChecksum(content)

	// Update status
	now := metav1.Now()
	prompt.Status.ResolvedContent = content
	prompt.Status.ResolvedAt = &now
	prompt.Status.SourceVersion = version
	prompt.Status.Checksum = checksum
	prompt.Status.ObservedGeneration = prompt.Generation
	r.setCondition(prompt, "Ready", metav1.ConditionTrue, "Resolved", "Prompt content resolved successfully")

	if err := r.Status().Update(ctx, prompt); err != nil {
		logger.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Determine requeue interval for sync
	requeueAfter := r.getRequeueInterval(prompt)
	if requeueAfter > 0 {
		logger.Info("Scheduling next sync", "interval", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
}

// validateSource ensures exactly one source is specified
func (r *PromptReconciler) validateSource(source *agentv1alpha1.PromptSource) error {
	count := 0
	if source.Langfuse != nil {
		count++
	}
	if source.Langsmith != nil {
		count++
	}
	if source.Inline != nil {
		count++
	}

	if count == 0 {
		return fmt.Errorf("exactly one of langfuse, langsmith, or inline must be specified")
	}
	if count > 1 {
		return fmt.Errorf("only one of langfuse, langsmith, or inline can be specified")
	}

	return nil
}

// resolvePromptContent resolves the prompt content based on the source type
func (r *PromptReconciler) resolvePromptContent(ctx context.Context, prompt *agentv1alpha1.Prompt) (content, version string, err error) {
	source := &prompt.Spec.Source

	if source.Inline != nil {
		// Inline source - content is directly available
		return source.Inline.Content, "inline", nil
	}

	if source.Langfuse != nil {
		// Langfuse source - fetch from Langfuse API
		return r.fetchFromLangfuse(ctx, prompt, source.Langfuse)
	}

	if source.Langsmith != nil {
		// Langsmith source - fetch from Langsmith API
		return r.fetchFromLangsmith(ctx, prompt, source.Langsmith)
	}

	return "", "", fmt.Errorf("no valid prompt source specified")
}

// fetchFromLangfuse fetches prompt content from Langfuse
func (r *PromptReconciler) fetchFromLangfuse(ctx context.Context, prompt *agentv1alpha1.Prompt, source *agentv1alpha1.LangfuseSource) (content, version string, err error) {
	logger := log.FromContext(ctx)

	// Get credentials from secret
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Name:      source.CredentialsSecretRef.Name,
		Namespace: prompt.Namespace,
	}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		return "", "", fmt.Errorf("failed to get Langfuse credentials secret: %w", err)
	}

	_, ok := secret.Data["publicKey"]
	if !ok {
		return "", "", fmt.Errorf("publicKey not found in secret %s", source.CredentialsSecretRef.Name)
	}

	_, ok = secret.Data["secretKey"]
	if !ok {
		return "", "", fmt.Errorf("secretKey not found in secret %s", source.CredentialsSecretRef.Name)
	}

	endpoint := source.Endpoint
	if endpoint == "" {
		endpoint = "https://cloud.langfuse.com"
	}

	logger.Info("Fetching prompt from Langfuse",
		"endpoint", endpoint,
		"promptName", source.PromptName,
		"version", source.Version)

	// TODO: Implement actual Langfuse API call
	// For now, return a placeholder
	return fmt.Sprintf("Langfuse prompt: %s (version: %s)", source.PromptName, source.Version), source.Version, nil
}

// fetchFromLangsmith fetches prompt content from Langsmith
func (r *PromptReconciler) fetchFromLangsmith(ctx context.Context, prompt *agentv1alpha1.Prompt, source *agentv1alpha1.LangsmithSource) (content, version string, err error) {
	logger := log.FromContext(ctx)

	// Get credentials from secret
	secret := &corev1.Secret{}
	secretKey := client.ObjectKey{
		Name:      source.CredentialsSecretRef.Name,
		Namespace: prompt.Namespace,
	}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		return "", "", fmt.Errorf("failed to get Langsmith credentials secret: %w", err)
	}

	_, ok := secret.Data["apiKey"]
	if !ok {
		return "", "", fmt.Errorf("apiKey not found in secret %s", source.CredentialsSecretRef.Name)
	}

	logger.Info("Fetching prompt from Langsmith",
		"promptName", source.PromptName,
		"commitHash", source.CommitHash)

	// TODO: Implement actual Langsmith API call
	// For now, return a placeholder
	return fmt.Sprintf("Langsmith prompt: %s (commit: %s)", source.PromptName, source.CommitHash), source.CommitHash, nil
}

// calculateChecksum calculates SHA256 checksum of the content
func (r *PromptReconciler) calculateChecksum(content string) string {
	hash := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(hash[:])
}

// getRequeueInterval determines the requeue interval based on sync configuration
func (r *PromptReconciler) getRequeueInterval(prompt *agentv1alpha1.Prompt) time.Duration {
	// Only requeue for non-inline sources with sync configured
	if prompt.Spec.Source.Inline != nil || prompt.Spec.Sync == nil {
		return 0
	}

	// Parse the interval string
	duration, err := time.ParseDuration(prompt.Spec.Sync.Interval)
	if err != nil {
		// If parsing fails, default to no requeue
		return 0
	}

	return duration
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
		Named("prompt").
		Complete(r)
}
