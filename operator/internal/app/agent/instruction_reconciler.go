package agent

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/infra/repo"
)

// InstructionReconciler handles instruction resolution for an agent.
type InstructionReconciler struct {
	instructions repo.InstructionReader
	instructionW repo.InstructionWriter
	configMaps   repo.ConfigMapRepo
	owner        repo.OwnerSetter
}

// Reconcile handles both inline instruction definitions and instruction references.
// For inline: creates a child Instruction CR, returns its ConfigMap name.
// For ref: looks up the existing Instruction, returns its ConfigMap name.
func (ir *InstructionReconciler) Reconcile(ctx context.Context, agent *agentv1alpha1.Agent) (string, error) {
	logger := log.FromContext(ctx)

	entry := agent.Spec.Instruction
	if entry == nil {
		return "", nil
	}

	if entry.Template != "" {
		return ir.reconcileInlineInstruction(ctx, agent, entry.Template)
	}

	if entry.InstructionRef != nil {
		namespace := entry.InstructionRef.Namespace
		if namespace == "" {
			namespace = agent.Namespace
		}

		instruction, err := ir.instructions.GetInstruction(ctx, types.NamespacedName{Name: entry.InstructionRef.Name, Namespace: namespace})
		if err != nil {
			return "", flokoaerrors.NewDependency(fmt.Errorf("failed to get referenced Instruction %s/%s: %w", namespace, entry.InstructionRef.Name, err))
		}

		if instruction.Status.ConfigMapName == "" {
			return "", flokoaerrors.NewDependencyf("instruction %s/%s has no ConfigMap yet (not reconciled)", namespace, instruction.Name)
		}

		logger.Info("Resolved instruction reference", "instruction", instruction.Name, "configMap", instruction.Status.ConfigMapName)
		return instruction.Status.ConfigMapName, nil
	}

	return "", flokoaerrors.NewPermanentf("instruction entry has neither inline nor instructionRef set")
}

// reconcileInlineInstruction creates or updates an Instruction CR for an inline instruction definition.
func (ir *InstructionReconciler) reconcileInlineInstruction(ctx context.Context, agent *agentv1alpha1.Agent, content string) (string, error) {
	instructionName := fmt.Sprintf("%s-instruction", agent.Name)

	desired := &agentv1alpha1.Instruction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instructionName,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       agent.Name,
				"app.kubernetes.io/component":  "inline-instruction",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
		},
		Spec: agentv1alpha1.InstructionSpec{
			Content: content,
		},
	}

	if err := ir.owner.SetOwner(agent, desired); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := ir.instructionW.EnsureInstruction(ctx, desired); err != nil {
		return "", err
	}

	// The Instruction controller creates a ConfigMap named "{instruction-name}-instruction"
	configMapName := fmt.Sprintf("%s-instruction", instructionName)

	// Check if the ConfigMap exists yet
	_, err := ir.configMaps.GetConfigMap(ctx, types.NamespacedName{Name: configMapName, Namespace: agent.Namespace})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return configMapName, nil
		}
		return "", fmt.Errorf("failed to check Instruction ConfigMap: %w", err)
	}

	return configMapName, nil
}
