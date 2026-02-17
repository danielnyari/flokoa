package fakes

import (
	"context"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// FakeScaledObjectRepo implements repo.ScaledObjectRepo for testing.
type FakeScaledObjectRepo struct {
	mu            sync.RWMutex
	ScaledObjects map[types.NamespacedName]*unstructured.Unstructured
	EnsureErr     error
	DeleteErr     error
}

func NewFakeScaledObjectRepo() *FakeScaledObjectRepo {
	return &FakeScaledObjectRepo{
		ScaledObjects: make(map[types.NamespacedName]*unstructured.Unstructured),
	}
}

func (f *FakeScaledObjectRepo) EnsureScaledObject(_ context.Context, desired *unstructured.Unstructured) error {
	if f.EnsureErr != nil {
		return f.EnsureErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	key := types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}
	f.ScaledObjects[key] = desired.DeepCopy()
	return nil
}

func (f *FakeScaledObjectRepo) DeleteScaledObject(_ context.Context, key types.NamespacedName) error {
	if f.DeleteErr != nil {
		return f.DeleteErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.ScaledObjects[key]; !ok {
		return apierrors.NewNotFound(schema.GroupResource{Group: "keda.sh", Resource: "scaledobjects"}, key.Name)
	}
	delete(f.ScaledObjects, key)
	return nil
}
