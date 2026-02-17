package repo

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigMapRepoImpl implements ConfigMapRepo using controller-runtime client.
type ConfigMapRepoImpl struct {
	Client client.Client
}

func (r *ConfigMapRepoImpl) GetConfigMap(ctx context.Context, key types.NamespacedName) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, key, cm); err != nil {
		return nil, err
	}
	return cm, nil
}

func (r *ConfigMapRepoImpl) EnsureConfigMap(ctx context.Context, desired *corev1.ConfigMap) error {
	existing := &corev1.ConfigMap{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if apierrors.IsNotFound(err) {
		if err := r.Client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create ConfigMap %s: %w", desired.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap %s: %w", desired.Name, err)
	}

	existing.Data = desired.Data
	existing.Labels = desired.Labels
	if err := r.Client.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update ConfigMap %s: %w", desired.Name, err)
	}
	return nil
}

func (r *ConfigMapRepoImpl) DeleteConfigMap(ctx context.Context, key types.NamespacedName) error {
	cm := &corev1.ConfigMap{}
	cm.Name = key.Name
	cm.Namespace = key.Namespace

	err := r.Client.Delete(ctx, cm)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ConfigMap %s: %w", key.Name, err)
	}
	return nil
}
