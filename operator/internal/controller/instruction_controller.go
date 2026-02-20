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
	instructionFinalizer = "agent.flokoa.ai/instruction-finalizer"

	// instructionConfigMapKey is the key in the ConfigMap for the instruction text file
	instructionConfigMapKey = "instruction.txt"
)

const (
	ConditionTypeInstructionStored = "Stored"
	ReasonInstructionStored        = "InstructionStored"
	ReasonInstructionStoreFailed   = "InstructionStoreFailed"
)

// InstructionReconciler reconciles an Instruction object
type InstructionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=instructions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=instructions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=instructions/finalizers,verbs=update

func (r *InstructionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Instruction", "name", req.Name, "namespace", req.Namespace)

	instruction := &agentv1alpha1.Instruction{}
	if err := r.Get(ctx, req.NamespacedName, instruction); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !instruction.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(instruction, instructionFinalizer) {
			if err := r.deleteConfigMap(ctx, instruction); err != nil {
				logger.Error(err, "Failed to delete ConfigMap")
				return ctrl.Result{}, err
			}

			controllerutil.RemoveFinalizer(instruction, instructionFinalizer)
			if err := r.Update(ctx, instruction); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(instruction, instructionFinalizer) {
		controllerutil.AddFinalizer(instruction, instructionFinalizer)
		if err := r.Update(ctx, instruction); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Reconcile the ConfigMap containing the instruction text
	configMapName, err := r.reconcileConfigMap(ctx, instruction)
	if err != nil {
		logger.Error(err, "Failed to reconcile ConfigMap")
		r.setCondition(instruction, ConditionTypeInstructionStored, metav1.ConditionFalse, ReasonInstructionStoreFailed, err.Error())
		if statusErr := updateStatusWithRetry(ctx, r.Client, instruction, func() {
			r.setCondition(instruction, ConditionTypeInstructionStored, metav1.ConditionFalse, ReasonInstructionStoreFailed, err.Error())
		}); statusErr != nil {
			logger.Error(statusErr, "Failed to update Instruction status after ConfigMap reconciliation failure")
		}
		return ctrl.Result{}, err
	}
	r.setCondition(instruction, ConditionTypeInstructionStored, metav1.ConditionTrue, ReasonInstructionStored, "Instruction stored in ConfigMap")

	// Update status
	instruction.Status.ConfigMapName = configMapName
	instruction.Status.ObservedGeneration = instruction.Generation

	desiredStatus := instruction.Status.DeepCopy()
	if err := updateStatusWithRetry(ctx, r.Client, instruction, func() {
		instruction.Status = *desiredStatus
	}); err != nil {
		logger.Error(err, "Failed to update Instruction status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileConfigMap creates or updates a ConfigMap containing the instruction content as a .txt file
func (r *InstructionReconciler) reconcileConfigMap(ctx context.Context, instruction *agentv1alpha1.Instruction) (string, error) {
	logger := log.FromContext(ctx)

	configMapName := fmt.Sprintf("%s-instruction", instruction.Name)

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instruction.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       instruction.Name,
				"app.kubernetes.io/component":  "instruction",
				"app.kubernetes.io/managed-by": "flokoa-operator",
			},
		},
		Data: map[string]string{
			instructionConfigMapKey: instruction.Spec.Content,
		},
	}

	if err := controllerutil.SetControllerReference(instruction, desired, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: configMapName, Namespace: instruction.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("Creating Instruction ConfigMap", "name", configMapName)
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

// deleteConfigMap deletes the ConfigMap associated with the Instruction
func (r *InstructionReconciler) deleteConfigMap(ctx context.Context, instruction *agentv1alpha1.Instruction) error {
	configMapName := fmt.Sprintf("%s-instruction", instruction.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: instruction.Namespace,
		},
	}

	err := r.Delete(ctx, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConfigMap: %w", err)
	}

	return nil
}

// setCondition updates or adds a condition to the Instruction status
func (r *InstructionReconciler) setCondition(instruction *agentv1alpha1.Instruction, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: instruction.Generation,
	}

	meta.SetStatusCondition(&instruction.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *InstructionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Instruction{}).
		Owns(&corev1.ConfigMap{}).
		Named("instruction").
		Complete(r)
}
