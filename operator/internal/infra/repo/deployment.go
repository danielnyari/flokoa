package repo

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DeploymentRepoImpl implements DeploymentRepo using controller-runtime client.
type DeploymentRepoImpl struct {
	Client client.Client
}

func (r *DeploymentRepoImpl) EnsureDeployment(ctx context.Context, desired *appsv1.Deployment) (*appsv1.Deployment, error) {
	existing := &appsv1.Deployment{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)

	if apierrors.IsNotFound(err) {
		if err := r.Client.Create(ctx, desired); err != nil {
			return nil, fmt.Errorf("failed to create Deployment: %w", err)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Deployment: %w", err)
	}

	existing.Spec = desired.Spec
	if err := r.Client.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update Deployment: %w", err)
	}

	return existing, nil
}
