package repo

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ModelRepoImpl implements ModelReader.
type ModelRepoImpl struct {
	Client client.Client
}

func (r *ModelRepoImpl) GetModel(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Model, error) {
	model := &agentv1alpha1.Model{}
	if err := r.Client.Get(ctx, key, model); err != nil {
		return nil, err
	}
	return model, nil
}
