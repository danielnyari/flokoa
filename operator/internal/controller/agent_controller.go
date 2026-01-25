package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const (
	agentFinalizer = "agent.flokoa.ai/finalizer"
)

const (
	ConditionTypeReady       = "Ready"
	ReasonDeploymentReady    = "DeploymentReady"
	ReasonDeploymentNotReady = "DeploymentNotReady"
	ReasonReconcileError     = "ReconcileError"
)

type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling Agent", "name", req.Name, "namespace", req.Namespace)

	agent := &agentv1alpha1.Agent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !agent.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
			controllerutil.RemoveFinalizer(agent, agentFinalizer)
			if err := r.Update(ctx, agent); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(agent, agentFinalizer) {
		controllerutil.AddFinalizer(agent, agentFinalizer)
		if err := r.Update(ctx, agent); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Reconcile Deployment
	deployment, err := r.reconcileDeployment(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonReconcileError, err.Error())
		_ = r.Status().Update(ctx, agent)
		return ctrl.Result{}, err
	}

	// Reconcile Service
	service, err := r.reconcileService(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile Service")
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonReconcileError, err.Error())
		_ = r.Status().Update(ctx, agent)
		return ctrl.Result{}, err
	}

	// Update status
	agent.Status.Phase = r.calculatePhase(deployment)
	agent.Status.Backend = "core"
	agent.Status.URL = fmt.Sprintf("http://%s.%s.svc.cluster.local", service.Name, service.Namespace)
	agent.Status.Replicas = deployment.Status.Replicas
	agent.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	agent.Status.ObservedGeneration = agent.Generation

	if deployment.Status.AvailableReplicas > 0 {
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionTrue, ReasonDeploymentReady, "Agent is ready")
	} else {
		r.setCondition(agent, ConditionTypeReady, metav1.ConditionFalse, ReasonDeploymentNotReady, "Waiting for pods")
	}

	if err := r.Status().Update(ctx, agent); err != nil {
		logger.Error(err, "Failed to update Agent status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *AgentReconciler) reconcileDeployment(ctx context.Context, agent *agentv1alpha1.Agent) (*appsv1.Deployment, error) {
	desired := r.buildDeployment(agent)

	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)

	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return nil, fmt.Errorf("failed to create Deployment: %w", err)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Deployment: %w", err)
	}

	existing.Spec = desired.Spec
	if err := r.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update Deployment: %w", err)
	}

	return existing, nil
}

func (r *AgentReconciler) buildDeployment(agent *agentv1alpha1.Agent) *appsv1.Deployment {
	labels := r.buildLabels(agent)
	spec := agent.Spec.Runtime.Spec

	// Default spec if not provided
	if spec == nil {
		spec = &agentv1alpha1.StandardRuntimeSpec{}
	}

	replicas := int32(1)
	if spec.Replicas != nil {
		replicas = *spec.Replicas
	}

	// Use the container spec directly from the CRD
	container := spec.Container

	// Ensure the container has a name
	if container.Name == "" {
		container.Name = "agent"
	}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers:         []corev1.Container{container},
					Volumes:            spec.Volumes,
					ImagePullSecrets:   spec.ImagePullSecrets,
					ServiceAccountName: spec.ServiceAccountName,
					SecurityContext:    spec.SecurityContext,
					NodeSelector:       spec.NodeSelector,
					Tolerations:        spec.Tolerations,
					Affinity:           spec.Affinity,
				},
			},
		},
	}
}

func (r *AgentReconciler) reconcileService(ctx context.Context, agent *agentv1alpha1.Agent) (*corev1.Service, error) {
	desired := r.buildService(agent)

	if err := controllerutil.SetControllerReference(agent, desired, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)

	if apierrors.IsNotFound(err) {
		if err := r.Create(ctx, desired); err != nil {
			return nil, fmt.Errorf("failed to create Service: %w", err)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Service: %w", err)
	}

	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	if err := r.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update Service: %w", err)
	}

	return existing, nil
}

func (r *AgentReconciler) buildService(agent *agentv1alpha1.Agent) *corev1.Service {
	labels := r.buildLabels(agent)
	spec := agent.Spec.Runtime.Spec

	// Build service ports from container ports
	var servicePorts []corev1.ServicePort
	if spec != nil {
		for _, cp := range spec.Container.Ports {
			servicePorts = append(servicePorts, corev1.ServicePort{
				Name:       cp.Name,
				Port:       cp.ContainerPort,
				TargetPort: intstr.FromInt32(cp.ContainerPort),
				Protocol:   cp.Protocol,
			})
		}
	}

	// Default to port 80 -> 8080 if no ports defined
	if len(servicePorts) == 0 {
		servicePorts = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromInt32(8080),
				Protocol:   corev1.ProtocolTCP,
			},
		}
	}

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agent.Name,
			Namespace: agent.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports:    servicePorts,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func (r *AgentReconciler) buildLabels(agent *agentv1alpha1.Agent) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       agent.Name,
		"app.kubernetes.io/instance":   agent.Name,
		"app.kubernetes.io/managed-by": "flokoa-operator",
		"flokoa.ai/agent":              agent.Name,
	}
}

func (r *AgentReconciler) calculatePhase(deployment *appsv1.Deployment) agentv1alpha1.AgentPhase {
	if deployment.Status.AvailableReplicas > 0 {
		return agentv1alpha1.AgentPhaseRunning
	}
	return agentv1alpha1.AgentPhasePending
}

//nolint:unparam // conditionType is parameterized for future extensibility
func (r *AgentReconciler) setCondition(agent *agentv1alpha1.Agent, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: agent.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&agent.Status.Conditions, condition)
}

func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Agent{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Named("agent").
		Complete(r)
}
