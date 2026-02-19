package controller

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentapp "github.com/danielnyari/flokoa/internal/app/agent"
)

const (
	agentFinalizer = "agent.flokoa.ai/finalizer"
)

// AgentReconciler reconciles Agent resources.
// It handles fetch, deletion, finalizer, and status persistence.
// All business logic is delegated to the application service.
type AgentReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	AppService *agentapp.Service
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=instructions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=models,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=modelproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Agent", "name", req.Name, "namespace", req.Namespace)

	// 1. Fetch the Agent
	agent := &agentv1alpha1.Agent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 2. Handle deletion
	if !agent.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
			controllerutil.RemoveFinalizer(agent, agentFinalizer)
			if err := r.Update(ctx, agent); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// 3. Add finalizer
	if !controllerutil.ContainsFinalizer(agent, agentFinalizer) {
		controllerutil.AddFinalizer(agent, agentFinalizer)
		if err := r.Update(ctx, agent); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// 4. Delegate to application service
	result := r.AppService.Reconcile(ctx, agent)

	// 5. Persist status (always, even on error — conditions are set by the app service)
	if err := r.Status().Update(ctx, agent); err != nil {
		logger.Error(err, "Failed to update Agent status")
		return ctrl.Result{}, err
	}

	if result.Error != nil {
		return ctrl.Result{}, result.Error
	}

	return ctrl.Result{}, nil
}

func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&agentv1alpha1.AgentTool{}).
		Owns(&agentv1alpha1.Instruction{}).
		Watches(
			&agentv1alpha1.AgentTool{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForAgentTool),
		).
		Watches(
			&agentv1alpha1.Instruction{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForInstruction),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForConfigMap),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForSecret),
		).
		Watches(
			&agentv1alpha1.Model{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForModel),
		).
		Watches(
			&agentv1alpha1.ModelProvider{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForModelProvider),
		).
		Named("agent").
		Complete(r)
}
