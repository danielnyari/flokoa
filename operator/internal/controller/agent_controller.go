package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const (
	agentFinalizer = "agent.flokoa.ai/finalizer"
)

const (
	ConditionTypeReady       = "Ready"
	ConditionTypeToolsReady  = "ToolsReady"
	ReasonDeploymentReady    = "DeploymentReady"
	ReasonDeploymentNotReady = "DeploymentNotReady"
	ReasonReconcileError     = "ReconcileError"
	ReasonToolsSynced        = "ToolsSynced"
	ReasonToolSyncFailed     = "ToolSyncFailed"
)

const (
	// toolsMountPath is the path where tool configurations are mounted
	toolsMountPath = "/etc/flokoa/tools"
)

type AgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agents/finalizers,verbs=update
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=agenttools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

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

	// Reconcile tools (inline and referenced) - creates ConfigMaps and returns volume/mount info
	toolConfigMaps, err := r.reconcileTools(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile tools")
		r.setCondition(agent, ConditionTypeToolsReady, metav1.ConditionFalse, ReasonToolSyncFailed, err.Error())
		_ = r.Status().Update(ctx, agent)
		return ctrl.Result{}, err
	}
	if len(agent.Spec.Tools) > 0 {
		r.setCondition(agent, ConditionTypeToolsReady, metav1.ConditionTrue, ReasonToolsSynced, fmt.Sprintf("Synced %d tools", len(toolConfigMaps)))
		now := metav1.Now()
		agent.Status.LastToolSync = &now
	}

	// Reconcile Deployment
	deployment, err := r.reconcileDeployment(ctx, agent, toolConfigMaps)
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

func (r *AgentReconciler) reconcileDeployment(ctx context.Context, agent *agentv1alpha1.Agent, toolConfigMaps []toolConfigMapInfo) (*appsv1.Deployment, error) {
	desired := r.buildDeployment(agent, toolConfigMaps)

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

// toolConfigMapInfo holds information about a tool's ConfigMap for mounting
type toolConfigMapInfo struct {
	// Name of the tool
	toolName string
	// Name of the ConfigMap containing the tool spec
	configMapName string
	// Hash of the ConfigMap data for change detection
	dataHash string
}

// reconcileTools reconciles both inline and referenced tools, creating ConfigMaps as needed
func (r *AgentReconciler) reconcileTools(ctx context.Context, agent *agentv1alpha1.Agent) ([]toolConfigMapInfo, error) {
	logger := log.FromContext(ctx)
	var toolConfigMaps []toolConfigMapInfo

	for i, tool := range agent.Spec.Tools {
		if tool.Inline != nil {
			// Handle inline tool - create an AgentTool CR for it
			toolName := tool.Name
			if toolName == "" {
				toolName = fmt.Sprintf("tool-%d", i)
			}

			cmName, err := r.reconcileInlineTool(ctx, agent, toolName, tool.Inline)
			if err != nil {
				return nil, fmt.Errorf("failed to reconcile inline tool %s: %w", toolName, err)
			}

			// Get the ConfigMap to compute hash (may not exist yet if AgentTool controller hasn't run)
			cm := &corev1.ConfigMap{}
			dataHash := ""
			if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agent.Namespace}, cm); err == nil {
				dataHash = hashConfigMapData(cm.Data)
			}

			logger.Info("Reconciled inline tool", "toolName", toolName, "configMap", cmName)
			toolConfigMaps = append(toolConfigMaps, toolConfigMapInfo{
				toolName:      toolName,
				configMapName: cmName,
				dataHash:      dataHash,
			})
		} else if tool.ToolRef != nil {
			// Handle tool reference - look up the AgentTool's ConfigMap
			namespace := tool.ToolRef.Namespace
			if namespace == "" {
				namespace = agent.Namespace
			}

			agentTool := &agentv1alpha1.AgentTool{}
			if err := r.Get(ctx, types.NamespacedName{Name: tool.ToolRef.Name, Namespace: namespace}, agentTool); err != nil {
				return nil, fmt.Errorf("failed to get referenced AgentTool %s/%s: %w", namespace, tool.ToolRef.Name, err)
			}

			// The AgentTool controller creates a ConfigMap named "{agenttool-name}-spec"
			cmName := fmt.Sprintf("%s-spec", agentTool.Name)

			// Verify the ConfigMap exists and compute hash
			cm := &corev1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: namespace}, cm); err != nil {
				return nil, fmt.Errorf("ConfigMap for AgentTool %s not found: %w", tool.ToolRef.Name, err)
			}

			toolName := tool.Name
			if toolName == "" {
				toolName = agentTool.Name
			}
			logger.Info("Found referenced tool", "toolName", toolName, "configMap", cmName)
			toolConfigMaps = append(toolConfigMaps, toolConfigMapInfo{
				toolName:      toolName,
				configMapName: cmName,
				dataHash:      hashConfigMapData(cm.Data),
			})
		}
	}

	return toolConfigMaps, nil
}

// hashConfigMapData computes a hash of ConfigMap data for change detection
func hashConfigMapData(data map[string]string) string {
	if len(data) == 0 {
		return ""
	}

	// Sort keys for deterministic hashing
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(data[k]))
	}
	return hex.EncodeToString(h.Sum(nil))[:16] // Use first 16 chars
}

// reconcileInlineTool creates or updates an AgentTool CR for an inline tool definition
// The AgentTool controller will then handle creating the ConfigMap
func (r *AgentReconciler) reconcileInlineTool(ctx context.Context, agent *agentv1alpha1.Agent, toolName string, spec *agentv1alpha1.AgentToolSpec) (string, error) {
	agentToolName := fmt.Sprintf("%s-%s", agent.Name, toolName)

	agentTool := &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentToolName,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       toolName,
				"app.kubernetes.io/component":  "inline-tool",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
		},
		Spec: *spec,
	}

	// Set owner reference so the AgentTool is cleaned up when the Agent is deleted
	if err := controllerutil.SetControllerReference(agent, agentTool, r.Scheme); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update AgentTool
	existing := &agentv1alpha1.AgentTool{}
	err := r.Get(ctx, types.NamespacedName{Name: agentToolName, Namespace: agent.Namespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Create(ctx, agentTool); err != nil {
				return "", fmt.Errorf("failed to create AgentTool: %w", err)
			}
		} else {
			return "", fmt.Errorf("failed to get AgentTool: %w", err)
		}
	} else {
		existing.Spec = agentTool.Spec
		existing.Labels = agentTool.Labels
		if err := r.Update(ctx, existing); err != nil {
			return "", fmt.Errorf("failed to update AgentTool: %w", err)
		}
	}

	// The AgentTool controller creates a ConfigMap named "{agenttool-name}-spec"
	return fmt.Sprintf("%s-spec", agentToolName), nil
}

func (r *AgentReconciler) buildDeployment(agent *agentv1alpha1.Agent, toolConfigMaps []toolConfigMapInfo) *appsv1.Deployment {
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

	// Build volumes from spec and tools
	volumes := append([]corev1.Volume{}, spec.Volumes...)

	// Add tool volume mounts to the container and compute combined hash
	var toolsHashBuilder string
	for _, toolCM := range toolConfigMaps {
		// Create a volume for each tool ConfigMap
		volumeName := fmt.Sprintf("tool-%s", toolCM.toolName)
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: toolCM.configMapName,
					},
				},
			},
		})

		// Add volume mount for this tool - each tool gets its own subdirectory
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: fmt.Sprintf("%s/%s", toolsMountPath, toolCM.toolName),
			ReadOnly:  true,
		})

		// Accumulate hashes for combined annotation
		toolsHashBuilder += toolCM.toolName + ":" + toolCM.dataHash + ";"
	}

	// Compute combined tools hash for pod annotation
	var podAnnotations map[string]string
	if len(toolConfigMaps) > 0 {
		h := sha256.Sum256([]byte(toolsHashBuilder))
		podAnnotations = map[string]string{
			"flokoa.ai/tools-hash": hex.EncodeToString(h[:])[:16],
		}
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
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers:         []corev1.Container{container},
					Volumes:            volumes,
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
		Owns(&agentv1alpha1.AgentTool{}).
		Watches(
			&agentv1alpha1.AgentTool{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForAgentTool),
		).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentsForConfigMap),
		).
		Named("agent").
		Complete(r)
}

// findAgentsForAgentTool returns the Agents that reference a given AgentTool
func (r *AgentReconciler) findAgentsForAgentTool(ctx context.Context, obj client.Object) []reconcile.Request {
	agentTool := obj.(*agentv1alpha1.AgentTool)
	logger := log.FromContext(ctx)

	// Check if this is an inline tool (owned by an Agent)
	if agentName, ok := agentTool.Labels["flokoa.ai/agent"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      agentName,
				Namespace: agentTool.Namespace,
			},
		}}
	}

	// List all agents in the same namespace to find those referencing this AgentTool
	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(agentTool.Namespace)); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		// Check if this agent references the updated AgentTool
		for _, tool := range agent.Spec.Tools {
			if tool.ToolRef != nil {
				refNamespace := tool.ToolRef.Namespace
				if refNamespace == "" {
					refNamespace = agent.Namespace
				}
				if tool.ToolRef.Name == agentTool.Name && refNamespace == agentTool.Namespace {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      agent.Name,
							Namespace: agent.Namespace,
						},
					})
					break
				}
			}
		}
	}

	return requests
}

// findAgentsForConfigMap returns the Agents that use a given ConfigMap (for tool specs)
func (r *AgentReconciler) findAgentsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	cm := obj.(*corev1.ConfigMap)

	// Only care about tool spec ConfigMaps
	component := cm.Labels["app.kubernetes.io/component"]
	if component != "agenttool-spec" && component != "inline-tool-spec" {
		return nil
	}

	// If this is an inline tool ConfigMap, it has the agent label
	if agentName, ok := cm.Labels["flokoa.ai/agent"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      agentName,
				Namespace: cm.Namespace,
			},
		}}
	}

	// For standalone AgentTool ConfigMaps, find agents referencing the AgentTool
	// ConfigMap name is "{agenttool-name}-spec", so extract the AgentTool name
	agentToolName := cm.Labels["app.kubernetes.io/name"]
	if agentToolName == "" {
		return nil
	}

	logger := log.FromContext(ctx)
	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(cm.Namespace)); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		for _, tool := range agent.Spec.Tools {
			if tool.ToolRef != nil {
				refNamespace := tool.ToolRef.Namespace
				if refNamespace == "" {
					refNamespace = agent.Namespace
				}
				if tool.ToolRef.Name == agentToolName && refNamespace == cm.Namespace {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      agent.Name,
							Namespace: agent.Namespace,
						},
					})
					break
				}
			}
		}
	}

	return requests
}
