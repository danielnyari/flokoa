package repo

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceRepoImpl implements ServiceRepo using controller-runtime client.
type ServiceRepoImpl struct {
	Client client.Client
}

func (r *ServiceRepoImpl) GetService(ctx context.Context, key types.NamespacedName) (*corev1.Service, error) {
	svc := &corev1.Service{}
	if err := r.Client.Get(ctx, key, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

func (r *ServiceRepoImpl) EnsureService(ctx context.Context, desired *corev1.Service) (*corev1.Service, error) {
	existing := &corev1.Service{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)

	if apierrors.IsNotFound(err) {
		if err := r.Client.Create(ctx, desired); err != nil {
			return nil, fmt.Errorf("failed to create Service: %w", err)
		}
		return desired, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Service: %w", err)
	}

	existing.Spec.Ports = desired.Spec.Ports
	existing.Spec.Selector = desired.Spec.Selector
	if err := r.Client.Update(ctx, existing); err != nil {
		return nil, fmt.Errorf("failed to update Service: %w", err)
	}

	return existing, nil
}
