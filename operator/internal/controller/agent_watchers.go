package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// findAgentsForInstruction returns the Agents that reference a given Instruction.
func (r *AgentReconciler) findAgentsForInstruction(ctx context.Context, obj client.Object) []reconcile.Request {
	instruction := obj.(*agentv1alpha1.Instruction)
	logger := log.FromContext(ctx)

	if agentName, ok := instruction.Labels["flokoa.ai/agent"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      agentName,
				Namespace: instruction.Namespace,
			},
		}}
	}

	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(instruction.Namespace)); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		if agent.Spec.Instruction != nil && agent.Spec.Instruction.InstructionRef != nil {
			refNamespace := agent.Spec.Instruction.InstructionRef.Namespace
			if refNamespace == "" {
				refNamespace = agent.Namespace
			}
			if agent.Spec.Instruction.InstructionRef.Name == instruction.Name && refNamespace == instruction.Namespace {
				requests = append(requests, reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      agent.Name,
						Namespace: agent.Namespace,
					},
				})
			}
		}
	}

	return requests
}

// findAgentsForModel returns the Agents that reference a given Model.
func (r *AgentReconciler) findAgentsForModel(ctx context.Context, obj client.Object) []reconcile.Request {
	model := obj.(*agentv1alpha1.Model)

	var agents agentv1alpha1.AgentList
	if err := r.List(ctx, &agents, client.InNamespace(model.Namespace)); err != nil {
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agents.Items {
		if agent.Spec.Model == nil {
			continue
		}
		modelNs := agent.Spec.Model.Namespace
		if modelNs == "" {
			modelNs = agent.Namespace
		}
		if agent.Spec.Model.Name == model.Name && modelNs == model.Namespace {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      agent.Name,
					Namespace: agent.Namespace,
				},
			})
		}
	}
	return requests
}

// findAgentsForSecret returns Agents affected by Secret changes through ModelProvider -> Model -> Agent references.
func (r *AgentReconciler) findAgentsForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)

	providerList := &agentv1alpha1.ModelProviderList{}
	if err := r.List(ctx, providerList, client.InNamespace(secret.Namespace)); err != nil {
		return nil
	}

	affectedProviders := map[string]struct{}{}
	for _, provider := range providerList.Items {
		if provider.Spec.APIKeySecretRef != nil && provider.Spec.APIKeySecretRef.Name == secret.Name {
			affectedProviders[provider.Name] = struct{}{}
		}
		if provider.Spec.Google != nil && provider.Spec.Google.ServiceAccountKeySecretRef != nil && provider.Spec.Google.ServiceAccountKeySecretRef.Name == secret.Name {
			affectedProviders[provider.Name] = struct{}{}
		}
	}

	if len(affectedProviders) == 0 {
		return nil
	}

	modelList := &agentv1alpha1.ModelList{}
	if err := r.List(ctx, modelList); err != nil {
		return nil
	}

	affectedModels := map[types.NamespacedName]struct{}{}
	for _, model := range modelList.Items {
		providerNamespace := model.Spec.ProviderRef.Namespace
		if providerNamespace == "" {
			providerNamespace = model.Namespace
		}
		if providerNamespace != secret.Namespace {
			continue
		}
		if _, ok := affectedProviders[model.Spec.ProviderRef.Name]; ok {
			affectedModels[types.NamespacedName{Name: model.Name, Namespace: model.Namespace}] = struct{}{}
		}
	}

	if len(affectedModels) == 0 {
		return nil
	}

	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList); err != nil {
		return nil
	}

	requests := []reconcile.Request{}
	for _, agent := range agentList.Items {
		if agent.Spec.Model == nil {
			continue
		}
		modelNamespace := agent.Spec.Model.Namespace
		if modelNamespace == "" {
			modelNamespace = agent.Namespace
		}
		if _, ok := affectedModels[types.NamespacedName{Name: agent.Spec.Model.Name, Namespace: modelNamespace}]; ok {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}})
		}
	}

	return requests
}

// findAgentsForAgentTool returns the Agents that reference a given AgentTool.
func (r *AgentReconciler) findAgentsForAgentTool(ctx context.Context, obj client.Object) []reconcile.Request {
	agentTool := obj.(*agentv1alpha1.AgentTool)
	logger := log.FromContext(ctx)

	if agentName, ok := agentTool.Labels["flokoa.ai/agent"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      agentName,
				Namespace: agentTool.Namespace,
			},
		}}
	}

	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList, client.InNamespace(agentTool.Namespace)); err != nil {
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

// findAgentsForConfigMap returns the Agents that use a given ConfigMap (for tool specs).
func (r *AgentReconciler) findAgentsForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	cm := obj.(*corev1.ConfigMap)

	component := cm.Labels["app.kubernetes.io/component"]
	if component != "agenttool-spec" && component != "inline-tool-spec" && component != "instruction" {
		return nil
	}

	if component == "instruction" {
		return nil
	}

	if agentName, ok := cm.Labels["flokoa.ai/agent"]; ok {
		return []reconcile.Request{{
			NamespacedName: types.NamespacedName{
				Name:      agentName,
				Namespace: cm.Namespace,
			},
		}}
	}

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
