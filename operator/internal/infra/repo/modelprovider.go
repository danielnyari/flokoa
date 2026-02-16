package repo

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ModelProviderRepoImpl implements ModelProviderReader.
type ModelProviderRepoImpl struct {
	Client client.Client
}

func (r *ModelProviderRepoImpl) GetModelProvider(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.ModelProvider, error) {
	provider := &agentv1alpha1.ModelProvider{}
	if err := r.Client.Get(ctx, key, provider); err != nil {
		return nil, err
	}
	return provider, nil
}
