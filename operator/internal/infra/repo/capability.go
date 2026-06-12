package repo

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// CapabilityRepoImpl implements CapabilityReader.
type CapabilityRepoImpl struct {
	Client client.Client
}

func (r *CapabilityRepoImpl) GetCapability(ctx context.Context, key types.NamespacedName) (*agentv1alpha1.Capability, error) {
	capability := &agentv1alpha1.Capability{}
	if err := r.Client.Get(ctx, key, capability); err != nil {
		return nil, err
	}
	return capability, nil
}
