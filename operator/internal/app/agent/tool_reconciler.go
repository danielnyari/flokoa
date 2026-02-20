package agent

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/domain/hash"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/infra/repo"
)

// toolConfigMapInfo holds information about a tool's ConfigMap for mounting.
type toolConfigMapInfo struct {
	toolName      string
	configMapName string
	dataHash      string
}

// resolvedModelInfo holds the resolved model configuration for use in deployment.
type resolvedModelInfo struct {
	provider          agentv1alpha1.ProviderType
	model             string
	configMapName     string
	envVars           []corev1.EnvVar
	secretEnvVars     []corev1.EnvVar
	secretRefsHash    string
	missingSecretRefs []string
}

// ToolReconciler handles tool resolution for an agent.
type ToolReconciler struct {
	agentTools repo.AgentToolReader
	agentToolW repo.AgentToolWriter
	configMaps repo.ConfigMapRepo
	owner      repo.OwnerSetter
}

// Reconcile resolves both inline and referenced tools, creating AgentTool CRs and returning ConfigMap info.
func (t *ToolReconciler) Reconcile(ctx context.Context, agent *agentv1alpha1.Agent) ([]toolConfigMapInfo, error) {
	logger := log.FromContext(ctx)
	var toolConfigMaps []toolConfigMapInfo

	for i, tool := range agent.Spec.Tools {
		if tool.Template != nil {
			toolName := tool.Name
			if toolName == "" {
				toolName = fmt.Sprintf("tool-%d", i)
			}

			cmName, err := t.reconcileInlineTool(ctx, agent, toolName, tool.Template)
			if err != nil {
				return nil, fmt.Errorf("failed to reconcile inline tool %s: %w", toolName, err)
			}

			// Get the ConfigMap to compute hash.
			// The ConfigMap is created by the AgentTool controller, so it may not exist yet.
			cm, err := t.configMaps.GetConfigMap(ctx, types.NamespacedName{Name: cmName, Namespace: agent.Namespace})
			if err != nil {
				return nil, flokoaerrors.NewDependency(fmt.Errorf("ConfigMap for inline tool %s not ready: %w", toolName, err))
			}
			dataHash := hash.ConfigMapData(cm.Data)

			logger.Info("Reconciled inline tool", "toolName", toolName, "configMap", cmName)
			toolConfigMaps = append(toolConfigMaps, toolConfigMapInfo{
				toolName:      toolName,
				configMapName: cmName,
				dataHash:      dataHash,
			})
		} else if tool.ToolRef != nil {
			namespace := tool.ToolRef.Namespace
			if namespace == "" {
				namespace = agent.Namespace
			}

			agentTool, err := t.agentTools.GetAgentTool(ctx, types.NamespacedName{Name: tool.ToolRef.Name, Namespace: namespace})
			if err != nil {
				return nil, flokoaerrors.NewDependency(fmt.Errorf("failed to get referenced AgentTool %s/%s: %w", namespace, tool.ToolRef.Name, err))
			}

			cmName := fmt.Sprintf("%s-spec", agentTool.Name)

			cm, err := t.configMaps.GetConfigMap(ctx, types.NamespacedName{Name: cmName, Namespace: namespace})
			if err != nil {
				return nil, flokoaerrors.NewDependency(fmt.Errorf("ConfigMap for AgentTool %s not found: %w", tool.ToolRef.Name, err))
			}

			toolName := tool.Name
			if toolName == "" {
				toolName = agentTool.Name
			}
			logger.Info("Found referenced tool", "toolName", toolName, "configMap", cmName)
			toolConfigMaps = append(toolConfigMaps, toolConfigMapInfo{
				toolName:      toolName,
				configMapName: cmName,
				dataHash:      hash.ConfigMapData(cm.Data),
			})
		}
	}

	return toolConfigMaps, nil
}

// reconcileInlineTool creates or updates an AgentTool CR for an inline tool definition.
func (t *ToolReconciler) reconcileInlineTool(ctx context.Context, agent *agentv1alpha1.Agent, toolName string, spec *agentv1alpha1.AgentToolSpec) (string, error) {
	agentToolName := fmt.Sprintf("%s-%s", agent.Name, toolName)

	desired := &agentv1alpha1.AgentTool{
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

	if err := t.owner.SetOwner(agent, desired); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := t.agentToolW.EnsureAgentTool(ctx, desired); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-spec", agentToolName), nil
}
