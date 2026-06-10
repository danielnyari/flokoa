package repo

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// AgentRepoImpl implements AgentReader using controller-runtime client.
type AgentRepoImpl struct {
	Client client.Client
}

func (r *AgentRepoImpl) GetAgent(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Agent, error) {
	agent := &agentv1alpha1.Agent{}
	if err := r.Client.Get(ctx, key, agent); err != nil {
		return nil, err
	}
	return agent, nil
}
