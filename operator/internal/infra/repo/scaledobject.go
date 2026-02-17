package repo

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var scaledObjectGVK = schema.GroupVersionKind{
	Group:   "keda.sh",
	Version: "v1alpha1",
	Kind:    "ScaledObject",
}

// ScaledObjectRepoImpl implements ScaledObjectRepo using controller-runtime client
// with unstructured objects to avoid a hard dependency on the KEDA Go module.
type ScaledObjectRepoImpl struct {
	Client client.Client
}

func (r *ScaledObjectRepoImpl) EnsureScaledObject(ctx context.Context, desired *unstructured.Unstructured) error {
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(scaledObjectGVK)

	err := r.Client.Get(ctx, types.NamespacedName{
		Name:      desired.GetName(),
		Namespace: desired.GetNamespace(),
	}, existing)

	if apierrors.IsNotFound(err) {
		if err := r.Client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create ScaledObject %s: %w", desired.GetName(), err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get ScaledObject %s: %w", desired.GetName(), err)
	}

	// Preserve the existing metadata (resourceVersion, etc.) and update spec + labels
	existing.Object["spec"] = desired.Object["spec"]
	existing.SetLabels(desired.GetLabels())
	if err := r.Client.Update(ctx, existing); err != nil {
		return fmt.Errorf("failed to update ScaledObject %s: %w", desired.GetName(), err)
	}
	return nil
}

func (r *ScaledObjectRepoImpl) DeleteScaledObject(ctx context.Context, key types.NamespacedName) error {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(scaledObjectGVK)
	obj.SetName(key.Name)
	obj.SetNamespace(key.Namespace)

	err := r.Client.Delete(ctx, obj)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ScaledObject %s: %w", key.Name, err)
	}
	return nil
}
