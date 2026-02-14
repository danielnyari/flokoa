package converter

import (
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// ObjectMetaToProto converts Kubernetes ObjectMeta to proto.
func ObjectMetaToProto(meta *metav1.ObjectMeta) *pb.ObjectMeta {
	if meta == nil {
		return nil
	}

	proto := &pb.ObjectMeta{
		Name:            meta.Name,
		Namespace:       meta.Namespace,
		Uid:             string(meta.UID),
		ResourceVersion: meta.ResourceVersion,
		Generation:      meta.Generation,
		Labels:          meta.Labels,
		Annotations:     meta.Annotations,
		Finalizers:      meta.Finalizers,
	}

	if !meta.CreationTimestamp.IsZero() {
		proto.CreationTimestamp = timestamppb.New(meta.CreationTimestamp.Time)
	}

	if meta.DeletionTimestamp != nil {
		proto.DeletionTimestamp = timestamppb.New(meta.DeletionTimestamp.Time)
	}

	for _, ref := range meta.OwnerReferences {
		ownerRef := &pb.OwnerReference{
			ApiVersion: ref.APIVersion,
			Kind:       ref.Kind,
			Name:       ref.Name,
			Uid:        string(ref.UID),
		}
		if ref.Controller != nil {
			ownerRef.Controller = *ref.Controller
		}
		if ref.BlockOwnerDeletion != nil {
			ownerRef.BlockOwnerDeletion = *ref.BlockOwnerDeletion
		}
		proto.OwnerReferences = append(proto.OwnerReferences, ownerRef)
	}

	return proto
}

// ObjectMetaFromProto converts proto ObjectMeta to Kubernetes.
func ObjectMetaFromProto(proto *pb.ObjectMeta) *metav1.ObjectMeta {
	if proto == nil {
		return nil
	}

	return &metav1.ObjectMeta{
		Name:            proto.Name,
		Namespace:       proto.Namespace,
		ResourceVersion: proto.ResourceVersion,
		Labels:          proto.Labels,
		Annotations:     proto.Annotations,
		Finalizers:      proto.Finalizers,
	}
}

// ConditionToProto converts Kubernetes Condition to proto.
func ConditionToProto(cond *metav1.Condition) *pb.Condition {
	if cond == nil {
		return nil
	}

	proto := &pb.Condition{
		Type:               cond.Type,
		Status:             string(cond.Status),
		ObservedGeneration: cond.ObservedGeneration,
		Reason:             cond.Reason,
		Message:            cond.Message,
	}

	if !cond.LastTransitionTime.IsZero() {
		proto.LastTransitionTime = timestamppb.New(cond.LastTransitionTime.Time)
	}

	return proto
}

// ConditionsToProto converts a slice of Kubernetes Conditions to proto.
func ConditionsToProto(conditions []metav1.Condition) []*pb.Condition {
	if conditions == nil {
		return nil
	}

	result := make([]*pb.Condition, len(conditions))
	for i := range conditions {
		result[i] = ConditionToProto(&conditions[i])
	}
	return result
}

// ConditionFromProto converts proto Condition to Kubernetes.
func ConditionFromProto(proto *pb.Condition) metav1.Condition {
	cond := metav1.Condition{
		Type:               proto.Type,
		Status:             metav1.ConditionStatus(proto.Status),
		ObservedGeneration: proto.ObservedGeneration,
		Reason:             proto.Reason,
		Message:            proto.Message,
	}

	if proto.LastTransitionTime != nil {
		cond.LastTransitionTime = metav1.NewTime(proto.LastTransitionTime.AsTime())
	}

	return cond
}

// ConditionsFromProto converts a slice of proto Conditions to Kubernetes.
func ConditionsFromProto(conditions []*pb.Condition) []metav1.Condition {
	if conditions == nil {
		return nil
	}

	result := make([]metav1.Condition, len(conditions))
	for i := range conditions {
		result[i] = ConditionFromProto(conditions[i])
	}
	return result
}

// ListMetaToProto converts Kubernetes ListMeta to proto.
func ListMetaToProto(meta *metav1.ListMeta) *pb.ListMeta {
	if meta == nil {
		return nil
	}

	proto := &pb.ListMeta{
		ResourceVersion: meta.ResourceVersion,
		Continue:        meta.Continue,
	}

	if meta.RemainingItemCount != nil {
		proto.RemainingItemCount = *meta.RemainingItemCount
	}

	return proto
}
