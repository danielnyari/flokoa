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
	tooldomain "github.com/danielnyari/flokoa/internal/domain/tool"
)

const (
	ConditionTypeValidated  = "Validated"
	ReasonValidationSuccess = "ValidationSuccess"
	ReasonValidationFailed  = "ValidationFailed"
)

// AgentToolReconciler reconciles a AgentTool object: a declarative MCP
// endpoint. The Agent compiler reads the spec directly — this controller only
// validates and surfaces conditions.
type AgentToolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools/status,verbs=get;update;patch

// Reconcile validates the AgentTool spec and records the result in status.
func (r *AgentToolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	agentTool := &agentv1alpha1.AgentTool{}
	if err := r.Get(ctx, req.NamespacedName, agentTool); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !agentTool.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	validationErr := tooldomain.ValidateSpec(agentTool)
	if err := updateStatusWithRetry(ctx, r.Client, agentTool, func() {
		if validationErr != nil {
			r.setCondition(agentTool, ConditionTypeValidated, metav1.ConditionFalse, ReasonValidationFailed, validationErr.Error())
		} else {
			r.setCondition(agentTool, ConditionTypeValidated, metav1.ConditionTrue, ReasonValidationSuccess, "Spec is valid")
		}
		agentTool.Status.ObservedGeneration = agentTool.Generation
	}); err != nil {
		logger.Error(err, "Failed to update AgentTool status")
		return ctrl.Result{}, err
	}

	if validationErr != nil {
		logger.Info("AgentTool spec invalid", "error", validationErr.Error())
	}
	return ctrl.Result{}, nil
}

func (r *AgentToolReconciler) setCondition(tool *agentv1alpha1.AgentTool, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&tool.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: tool.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentToolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.AgentTool{}).
		Named("agenttool").
		Complete(r)
}
