package repo

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// InstructionRepoImpl implements InstructionReader and InstructionWriter.
type InstructionRepoImpl struct {
	Client client.Client
}

func (r *InstructionRepoImpl) GetInstruction(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Instruction, error) {
	instruction := &agentv1alpha1.Instruction{}
	if err := r.Client.Get(ctx, key, instruction); err != nil {
		return nil, err
	}
	return instruction, nil
}

func (r *InstructionRepoImpl) EnsureInstruction(ctx context.Context, desired *agentv1alpha1.Instruction) error {
	existing := &agentv1alpha1.Instruction{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		if err := r.Client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create Instruction: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get Instruction: %w", err)
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if err := r.Client.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update Instruction: %w", err)
	}
	return nil
}
