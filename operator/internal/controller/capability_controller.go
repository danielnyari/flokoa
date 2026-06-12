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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// CapabilityReconciler surfaces a Capability's policy and verification state
// in status (roadmap 08). It owns no workloads: capabilities act through the
// Agents that attach them. Artifact digest/signature verification (the
// Verified condition's True/False paths) ships with capability delivery
// (roadmap 09).
type CapabilityReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=capabilities,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=capabilities/status,verbs=get;update;patch

func (r *CapabilityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	capability := &agentv1alpha1.Capability{}
	if err := r.Get(ctx, req.NamespacedName, capability); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !capability.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	err := updateStatusWithRetry(ctx, r.Client, capability, func() {
		applyCapabilityStatus(capability)
	})
	if err != nil {
		logger.Error(err, "Failed to update Capability status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// applyCapabilityStatus computes the desired status conditions in place.
func applyCapabilityStatus(capability *agentv1alpha1.Capability) {
	capability.Status.ObservedGeneration = capability.Generation

	// Permissive is the loud surfacing of schemaPolicy: permissive (product
	// brief §4): visible in status, printcolumns, and CLI output.
	permissive := metav1.Condition{
		Type:               agentv1alpha1.CapabilityConditionPermissive,
		Status:             metav1.ConditionFalse,
		Reason:             "SchemaValidated",
		Message:            "attached agent config is validated against spec.configSchema at admission",
		ObservedGeneration: capability.Generation,
	}
	if capability.Spec.SchemaPolicy == agentv1alpha1.SchemaPolicyPermissive {
		permissive.Status = metav1.ConditionTrue
		permissive.Reason = "SchemaPolicyPermissive"
		permissive.Message = "schemaPolicy is permissive: attached agent config is not validated at admission"
	}
	meta.SetStatusCondition(&capability.Status.Conditions, permissive)

	// Verified stays Unknown until controller-side artifact verification
	// lands (roadmap 09); admission already enforces the digest pin.
	if meta.FindStatusCondition(capability.Status.Conditions, agentv1alpha1.CapabilityConditionVerified) == nil {
		meta.SetStatusCondition(&capability.Status.Conditions, metav1.Condition{
			Type:               agentv1alpha1.CapabilityConditionVerified,
			Status:             metav1.ConditionUnknown,
			Reason:             "VerificationPending",
			Message:            "artifact digest/signature verification ships with capability delivery (roadmap 09)",
			ObservedGeneration: capability.Generation,
		})
	}
}

func (r *CapabilityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Capability{}).
		Named("capability").
		Complete(r)
}
