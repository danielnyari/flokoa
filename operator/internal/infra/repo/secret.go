package repo

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretRepoImpl implements SecretReader.
type SecretRepoImpl struct {
	Client client.Client
}

func (r *SecretRepoImpl) GetSecret(ctx context.Context, key types.NamespacedName) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		return nil, err
	}
	return secret, nil
}
