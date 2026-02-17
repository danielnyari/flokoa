package repo

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// AgentToolRepoImpl implements AgentToolReader and AgentToolWriter.
type AgentToolRepoImpl struct {
	Client client.Client
}

func (r *AgentToolRepoImpl) GetAgentTool(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.AgentTool, error) {
	tool := &agentv1alpha1.AgentTool{}
	if err := r.Client.Get(ctx, key, tool); err != nil {
		return nil, err
	}
	return tool, nil
}

func (r *AgentToolRepoImpl) EnsureAgentTool(ctx context.Context, desired *agentv1alpha1.AgentTool) error {
	existing := &agentv1alpha1.AgentTool{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		if err := r.Client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create AgentTool: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get AgentTool: %w", err)
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	if err := r.Client.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update AgentTool: %w", err)
	}
	return nil
}
