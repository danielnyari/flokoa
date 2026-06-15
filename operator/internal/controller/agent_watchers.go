package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// The watch mappers below re-enqueue Agents when anything in their
// composition graph changes — this is the fleet-management story: rotate one
// Model CR and every referencing Agent recompiles, specHash changes, and the
// Deployment rolls.

func requestFor(agent *agentv1alpha1.Agent) reconcile.Request {
	return reconcile.Request{NamespacedName: types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}}
}

func refMatches(ref agentv1alpha1.NamespacedRef, agentNamespace, name, namespace string) bool {
	refNamespace := ref.Namespace
	if refNamespace == "" {
		refNamespace = agentNamespace
	}
	return ref.Name == name && refNamespace == namespace
}

// findAgentsForInstruction returns the Agents that reference a given Instruction.
func (r *AgentReconciler) findAgentsForInstruction(ctx context.Context, obj client.Object) []reconcile.Request {
	instruction, ok := obj.(*agentv1alpha1.Instruction)
	if !ok {
		log.FromContext(ctx).Error(nil, "findAgentsForInstruction received unexpected object type", "type", fmt.Sprintf("%T", obj))
		return nil
	}
	logger := log.FromContext(ctx)

	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		for _, ref := range agent.Spec.InstructionRefs {
			if refMatches(ref, agent.Namespace, instruction.Name, instruction.Namespace) {
				requests = append(requests, requestFor(&agent))
				break
			}
		}
	}

	return requests
}

// findAgentsForModel returns the Agents that reference a given Model.
func (r *AgentReconciler) findAgentsForModel(ctx context.Context, obj client.Object) []reconcile.Request {
	model, ok := obj.(*agentv1alpha1.Model)
	if !ok {
		log.FromContext(ctx).Error(nil, "findAgentsForModel received unexpected object type", "type", fmt.Sprintf("%T", obj))
		return nil
	}

	logger := log.FromContext(ctx)

	var agents agentv1alpha1.AgentList
	if err := r.List(ctx, &agents); err != nil {
		logger.Error(err, "Failed to list Agents for Model watch")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agents.Items {
		if agent.Spec.ModelRef == nil {
			continue
		}
		if refMatches(*agent.Spec.ModelRef, agent.Namespace, model.Name, model.Namespace) {
			requests = append(requests, requestFor(&agent))
		}
	}
	return requests
}

// findAgentsForModelProvider returns Agents affected by ModelProvider changes through Model -> Agent references.
func (r *AgentReconciler) findAgentsForModelProvider(ctx context.Context, obj client.Object) []reconcile.Request {
	provider, ok := obj.(*agentv1alpha1.ModelProvider)
	if !ok {
		log.FromContext(ctx).Error(nil, "findAgentsForModelProvider received unexpected object type", "type", fmt.Sprintf("%T", obj))
		return nil
	}
	logger := log.FromContext(ctx)

	modelList := &agentv1alpha1.ModelList{}
	if err := r.List(ctx, modelList); err != nil {
		logger.Error(err, "Failed to list Models for ModelProvider watch")
		return nil
	}

	affectedModels := map[types.NamespacedName]struct{}{}
	for _, model := range modelList.Items {
		providerNamespace := model.Spec.ProviderRef.Namespace
		if providerNamespace == "" {
			providerNamespace = model.Namespace
		}
		if providerNamespace == provider.Namespace && model.Spec.ProviderRef.Name == provider.Name {
			affectedModels[types.NamespacedName{Name: model.Name, Namespace: model.Namespace}] = struct{}{}
		}
	}

	if len(affectedModels) == 0 {
		return nil
	}

	return r.agentsReferencingModels(ctx, affectedModels)
}

func (r *AgentReconciler) agentsReferencingModels(ctx context.Context, models map[types.NamespacedName]struct{}) []reconcile.Request {
	logger := log.FromContext(ctx)
	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	requests := []reconcile.Request{}
	for _, agent := range agentList.Items {
		if agent.Spec.ModelRef == nil {
			continue
		}
		modelNamespace := agent.Spec.ModelRef.Namespace
		if modelNamespace == "" {
			modelNamespace = agent.Namespace
		}
		if _, ok := models[types.NamespacedName{Name: agent.Spec.ModelRef.Name, Namespace: modelNamespace}]; ok {
			requests = append(requests, requestFor(&agent))
		}
	}
	return requests
}

// findAgentsForCapability returns the Agents that attach a given Capability.
func (r *AgentReconciler) findAgentsForCapability(ctx context.Context, obj client.Object) []reconcile.Request {
	capability, ok := obj.(*agentv1alpha1.Capability)
	if !ok {
		log.FromContext(ctx).Error(nil, "findAgentsForCapability received unexpected object type", "type", fmt.Sprintf("%T", obj))
		return nil
	}
	logger := log.FromContext(ctx)

	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		for _, att := range agent.Spec.Capabilities {
			if refMatches(att.Ref, agent.Namespace, capability.Name, capability.Namespace) {
				requests = append(requests, requestFor(&agent))
				break
			}
		}
	}

	return requests
}

// findAgentsForAgentTool returns the Agents that reference a given AgentTool.
func (r *AgentReconciler) findAgentsForAgentTool(ctx context.Context, obj client.Object) []reconcile.Request {
	agentTool, ok := obj.(*agentv1alpha1.AgentTool)
	if !ok {
		log.FromContext(ctx).Error(nil, "findAgentsForAgentTool received unexpected object type", "type", fmt.Sprintf("%T", obj))
		return nil
	}
	logger := log.FromContext(ctx)

	agentList := &agentv1alpha1.AgentList{}
	if err := r.List(ctx, agentList); err != nil {
		logger.Error(err, "Failed to list Agents")
		return nil
	}

	var requests []reconcile.Request
	for _, agent := range agentList.Items {
		for _, ref := range agent.Spec.Tools {
			if refMatches(ref, agent.Namespace, agentTool.Name, agentTool.Namespace) {
				requests = append(requests, requestFor(&agent))
				break
			}
		}
	}

	return requests
}
