package repo

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretRepoImpl implements SecretReader. Client is a client.Reader (not a
// full client.Client) so it can be wired with the manager's uncached
// APIReader: the operator reads Secret values on demand by name and never
// caches or watches them, which keeps its Secret RBAC to a cluster-wide get
// instead of the list/watch a cache-backed read would require.
type SecretRepoImpl struct {
	Client client.Reader
}

func (r *SecretRepoImpl) GetSecret(ctx context.Context, key types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		return nil, err
	}
	return secret, nil
}
