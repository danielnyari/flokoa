package converter

import (
	"testing"
	"time"

	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestObjectMetaToProto_Nil(t *testing.T) {
	result := ObjectMetaToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestObjectMetaToProto_BasicFields(t *testing.T) {
	meta := &metav1.ObjectMeta{
		Name:            "test-agent",
		Namespace:       "default",
		UID:             types.UID("uid-123"),
		ResourceVersion: "42",
		Generation:      3,
		Labels:          map[string]string{"app": "test"},
		Annotations:     map[string]string{"note": "value"},
		Finalizers:      []string{"finalizer.flokoa.ai"},
	}

	result := ObjectMetaToProto(meta)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "test-agent" {
		t.Fatalf("expected name test-agent, got %q", result.Name)
	}
	if result.Namespace != "default" {
		t.Fatalf("expected namespace default, got %q", result.Namespace)
	}
	if result.Uid != "uid-123" {
		t.Fatalf("expected uid uid-123, got %q", result.Uid)
	}
	if result.ResourceVersion != "42" {
		t.Fatalf("expected resource version 42, got %q", result.ResourceVersion)
	}
	if result.Generation != 3 {
		t.Fatalf("expected generation 3, got %d", result.Generation)
	}
	if result.Labels["app"] != "test" {
		t.Fatal("expected label app=test")
	}
	if result.Annotations["note"] != "value" {
		t.Fatal("expected annotation note=value")
	}
	if len(result.Finalizers) != 1 || result.Finalizers[0] != "finalizer.flokoa.ai" {
		t.Fatal("expected finalizer")
	}
}

func TestObjectMetaToProto_Timestamps(t *testing.T) {
	now := time.Now()
	deletionTime := metav1.NewTime(now.Add(time.Hour))

	meta := &metav1.ObjectMeta{
		Name:              "test",
		CreationTimestamp: metav1.NewTime(now),
		DeletionTimestamp: &deletionTime,
	}

	result := ObjectMetaToProto(meta)
	if result.CreationTimestamp == nil {
		t.Fatal("expected creation timestamp to be set")
	}
	if result.DeletionTimestamp == nil {
		t.Fatal("expected deletion timestamp to be set")
	}
}

func TestObjectMetaToProto_ZeroTimestamp(t *testing.T) {
	meta := &metav1.ObjectMeta{
		Name: "test",
	}

	result := ObjectMetaToProto(meta)
	if result.CreationTimestamp != nil {
		t.Fatal("expected nil creation timestamp for zero time")
	}
	if result.DeletionTimestamp != nil {
		t.Fatal("expected nil deletion timestamp for nil input")
	}
}

func TestObjectMetaToProto_OwnerReferences(t *testing.T) {
	controller := true
	blockDeletion := true

	meta := &metav1.ObjectMeta{
		Name: "test",
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion:         "agent.flokoa.ai/v1alpha1",
				Kind:               "Agent",
				Name:               "my-agent",
				UID:                types.UID("owner-uid"),
				Controller:         &controller,
				BlockOwnerDeletion: &blockDeletion,
			},
		},
	}

	result := ObjectMetaToProto(meta)
	if len(result.OwnerReferences) != 1 {
		t.Fatalf("expected 1 owner reference, got %d", len(result.OwnerReferences))
	}

	ref := result.OwnerReferences[0]
	if ref.ApiVersion != "agent.flokoa.ai/v1alpha1" {
		t.Fatalf("expected api version, got %q", ref.ApiVersion)
	}
	if ref.Kind != "Agent" {
		t.Fatalf("expected kind Agent, got %q", ref.Kind)
	}
	if ref.Name != "my-agent" {
		t.Fatalf("expected name my-agent, got %q", ref.Name)
	}
	if ref.Uid != "owner-uid" {
		t.Fatalf("expected uid owner-uid, got %q", ref.Uid)
	}
	if !ref.Controller {
		t.Fatal("expected controller to be true")
	}
	if !ref.BlockOwnerDeletion {
		t.Fatal("expected block owner deletion to be true")
	}
}

func TestObjectMetaToProto_OwnerReferenceNilBooleans(t *testing.T) {
	meta := &metav1.ObjectMeta{
		Name: "test",
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       "cm",
				UID:        types.UID("cm-uid"),
			},
		},
	}

	result := ObjectMetaToProto(meta)
	ref := result.OwnerReferences[0]
	if ref.Controller {
		t.Fatal("expected controller to be false for nil")
	}
	if ref.BlockOwnerDeletion {
		t.Fatal("expected block owner deletion to be false for nil")
	}
}

func TestObjectMetaFromProto_Nil(t *testing.T) {
	result := ObjectMetaFromProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestObjectMetaFromProto_BasicFields(t *testing.T) {
	proto := &pb.ObjectMeta{
		Name:            "test-agent",
		Namespace:       "default",
		ResourceVersion: "42",
		Labels:          map[string]string{"app": "test"},
		Annotations:     map[string]string{"note": "value"},
		Finalizers:      []string{"finalizer.flokoa.ai"},
	}

	result := ObjectMetaFromProto(proto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "test-agent" {
		t.Fatalf("expected name test-agent, got %q", result.Name)
	}
	if result.Namespace != "default" {
		t.Fatalf("expected namespace default, got %q", result.Namespace)
	}
	if result.ResourceVersion != "42" {
		t.Fatalf("expected resource version 42, got %q", result.ResourceVersion)
	}
	if result.Labels["app"] != "test" {
		t.Fatal("expected label")
	}
	if result.Annotations["note"] != "value" {
		t.Fatal("expected annotation")
	}
	if len(result.Finalizers) != 1 || result.Finalizers[0] != "finalizer.flokoa.ai" {
		t.Fatal("expected finalizer")
	}
}

func TestObjectMetaRoundTrip(t *testing.T) {
	original := &metav1.ObjectMeta{
		Name:            "round-trip",
		Namespace:       "ns",
		ResourceVersion: "10",
		Labels:          map[string]string{"k": "v"},
		Annotations:     map[string]string{"a": "b"},
		Finalizers:      []string{"fin1", "fin2"},
	}

	proto := ObjectMetaToProto(original)
	result := ObjectMetaFromProto(proto)

	if result.Name != original.Name {
		t.Fatalf("name mismatch: %q vs %q", result.Name, original.Name)
	}
	if result.Namespace != original.Namespace {
		t.Fatalf("namespace mismatch")
	}
	if result.ResourceVersion != original.ResourceVersion {
		t.Fatalf("resource version mismatch")
	}
	if result.Labels["k"] != "v" {
		t.Fatal("labels mismatch")
	}
	if result.Annotations["a"] != "b" {
		t.Fatal("annotations mismatch")
	}
	if len(result.Finalizers) != 2 {
		t.Fatal("finalizers mismatch")
	}
}

func TestConditionToProto_Nil(t *testing.T) {
	result := ConditionToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestConditionToProto(t *testing.T) {
	now := metav1.NewTime(time.Now())
	cond := &metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 5,
		LastTransitionTime: now,
		Reason:             "AllGood",
		Message:            "Everything is fine",
	}

	result := ConditionToProto(cond)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Type != "Ready" {
		t.Fatalf("expected type Ready, got %q", result.Type)
	}
	if result.Status != "True" {
		t.Fatalf("expected status True, got %q", result.Status)
	}
	if result.ObservedGeneration != 5 {
		t.Fatalf("expected observed generation 5, got %d", result.ObservedGeneration)
	}
	if result.Reason != "AllGood" {
		t.Fatalf("expected reason AllGood, got %q", result.Reason)
	}
	if result.Message != "Everything is fine" {
		t.Fatalf("expected message, got %q", result.Message)
	}
	if result.LastTransitionTime == nil {
		t.Fatal("expected last transition time to be set")
	}
}

func TestConditionToProto_ZeroTransitionTime(t *testing.T) {
	cond := &metav1.Condition{
		Type:   "Ready",
		Status: metav1.ConditionFalse,
		Reason: "NotReady",
	}

	result := ConditionToProto(cond)
	if result.LastTransitionTime != nil {
		t.Fatal("expected nil last transition time for zero time")
	}
}

func TestConditionsToProto_Nil(t *testing.T) {
	result := ConditionsToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestConditionsToProto(t *testing.T) {
	conditions := []metav1.Condition{
		{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"},
		{Type: "Available", Status: metav1.ConditionFalse, Reason: "NotYet"},
	}

	result := ConditionsToProto(conditions)
	if len(result) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(result))
	}
	if result[0].Type != "Ready" {
		t.Fatalf("expected first condition Ready, got %q", result[0].Type)
	}
	if result[1].Type != "Available" {
		t.Fatalf("expected second condition Available, got %q", result[1].Type)
	}
}

func TestConditionsToProto_Empty(t *testing.T) {
	result := ConditionsToProto([]metav1.Condition{})
	if len(result) != 0 {
		t.Fatalf("expected 0 conditions, got %d", len(result))
	}
}

func TestListMetaToProto_Nil(t *testing.T) {
	result := ListMetaToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestListMetaToProto(t *testing.T) {
	remaining := int64(42)
	meta := &metav1.ListMeta{
		ResourceVersion:    "100",
		Continue:           "token-abc",
		RemainingItemCount: &remaining,
	}

	result := ListMetaToProto(meta)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ResourceVersion != "100" {
		t.Fatalf("expected resource version 100, got %q", result.ResourceVersion)
	}
	if result.Continue != "token-abc" {
		t.Fatalf("expected continue token-abc, got %q", result.Continue)
	}
	if result.RemainingItemCount != 42 {
		t.Fatalf("expected remaining 42, got %d", result.RemainingItemCount)
	}
}

func TestListMetaToProto_NilRemainingCount(t *testing.T) {
	meta := &metav1.ListMeta{
		ResourceVersion: "1",
	}

	result := ListMetaToProto(meta)
	if result.RemainingItemCount != 0 {
		t.Fatalf("expected 0 remaining for nil, got %d", result.RemainingItemCount)
	}
}
