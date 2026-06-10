package converter

import (
	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// AgentTriggerToProto converts a Kubernetes AgentTrigger to proto.
func AgentTriggerToProto(trigger *agentv1alpha1.AgentTrigger) *pb.AgentTrigger {
	if trigger == nil {
		return nil
	}

	return &pb.AgentTrigger{
		Metadata: ObjectMetaToProto(&trigger.ObjectMeta),
		Spec:     AgentTriggerSpecToProto(&trigger.Spec),
		Status:   AgentTriggerStatusToProto(&trigger.Status),
	}
}

// AgentTriggerSpecToProto converts AgentTriggerSpec to proto.
func AgentTriggerSpecToProto(spec *agentv1alpha1.AgentTriggerSpec) *pb.AgentTriggerSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.AgentTriggerSpec{
		EventSource: &pb.EventSourceRef{
			Name:      spec.EventSource.Name,
			EventName: spec.EventSource.EventName,
		},
		Agent: &pb.AgentRef{
			Name:      spec.Agent.Name,
			Namespace: spec.Agent.Namespace,
		},
	}

	if spec.EventBus != nil {
		pbSpec.EventBus = &pb.EventBusRef{
			Name: spec.EventBus.Name,
		}
	}

	if spec.Filter != nil {
		pbSpec.Filter = TriggerFilterToProto(spec.Filter)
	}

	if spec.Task != nil {
		pbSpec.Task = &pb.TriggerTaskConfig{
			SessionKeyFrom: spec.Task.SessionKeyFrom,
			Metadata:       spec.Task.Metadata,
		}
	}

	if spec.PushNotification != nil {
		pbSpec.PushNotification = PushNotificationTargetToProto(spec.PushNotification)
	}

	if spec.Limits != nil {
		pbSpec.Limits = TriggerLimitsToProto(spec.Limits)
	}

	return pbSpec
}

// TriggerFilterToProto converts TriggerFilter to proto.
func TriggerFilterToProto(filter *agentv1alpha1.TriggerFilter) *pb.TriggerFilter {
	if filter == nil {
		return nil
	}

	pbFilter := &pb.TriggerFilter{}

	for _, d := range filter.Data {
		pbFilter.Data = append(pbFilter.Data, &pb.DataFilter{
			Path:       d.Path,
			Type:       d.Type,
			Value:      d.Value,
			Comparator: d.Comparator,
		})
	}

	for _, e := range filter.Exprs {
		pbFilter.Exprs = append(pbFilter.Exprs, &pb.ExprFilter{
			Expr:   e.Expr,
			Fields: e.Fields,
		})
	}

	return pbFilter
}

// PushNotificationTargetToProto converts PushNotificationTarget to proto.
func PushNotificationTargetToProto(target *agentv1alpha1.PushNotificationTarget) *pb.PushNotificationTarget {
	if target == nil {
		return nil
	}

	pbTarget := &pb.PushNotificationTarget{
		Url: target.URL,
	}

	if target.AgentRef != nil {
		pbTarget.AgentRef = &pb.AgentRef{
			Name:      target.AgentRef.Name,
			Namespace: target.AgentRef.Namespace,
		}
	}

	if target.Authentication != nil {
		pbTarget.Authentication = &pb.TriggerPushAuth{
			Schemes: target.Authentication.Schemes,
		}
		if target.Authentication.CredentialsRef != nil {
			pbTarget.Authentication.CredentialsRef = SecretKeySelectorToProto(target.Authentication.CredentialsRef)
		}
	}

	if target.TokenRef != nil {
		pbTarget.TokenRef = SecretKeySelectorToProto(target.TokenRef)
	}

	return pbTarget
}

// TriggerLimitsToProto converts TriggerLimits to proto.
func TriggerLimitsToProto(limits *agentv1alpha1.TriggerLimits) *pb.TriggerLimits {
	if limits == nil {
		return nil
	}

	pbLimits := &pb.TriggerLimits{}

	if limits.MaxInvocationsPerHour != nil {
		pbLimits.MaxInvocationsPerHour = *limits.MaxInvocationsPerHour
	}

	if limits.MaxConcurrentTasks != nil {
		pbLimits.MaxConcurrentTasks = *limits.MaxConcurrentTasks
	}

	if limits.TokenBudgetPerEvent != nil {
		pbLimits.TokenBudgetPerEvent = *limits.TokenBudgetPerEvent
	}

	if limits.TokenBudgetPerHour != nil {
		pbLimits.TokenBudgetPerHour = *limits.TokenBudgetPerHour
	}

	if limits.DeadLetterSink != nil {
		pbLimits.DeadLetterSink = &pb.DeadLetterSinkRef{
			Uri: limits.DeadLetterSink.URI,
		}
	}

	return pbLimits
}

// AgentTriggerStatusToProto converts AgentTriggerStatus to proto.
func AgentTriggerStatusToProto(status *agentv1alpha1.AgentTriggerStatus) *pb.AgentTriggerStatus {
	if status == nil {
		return nil
	}

	pbStatus := &pb.AgentTriggerStatus{
		Phase:              AgentTriggerPhaseToProto(status.Phase),
		Conditions:         ConditionsToProto(status.Conditions),
		ObservedGeneration: status.ObservedGeneration,
		AgentEndpoint:      status.AgentEndpoint,
		SensorName:         status.SensorName,
	}

	if status.Invocations != nil {
		pbStatus.Invocations = &pb.InvocationCounters{
			Total:        status.Invocations.Total,
			Delivered:    status.Invocations.Delivered,
			Dropped:      status.Invocations.Dropped,
			DeadLettered: status.Invocations.DeadLettered,
		}
	}

	return pbStatus
}

// AgentTriggerListToProto converts a Kubernetes AgentTriggerList to proto.
func AgentTriggerListToProto(list *agentv1alpha1.AgentTriggerList) *pb.AgentTriggerList {
	if list == nil {
		return nil
	}

	pbList := &pb.AgentTriggerList{
		Metadata: ListMetaToProto(&list.ListMeta),
	}

	for i := range list.Items {
		pbList.Items = append(pbList.Items, AgentTriggerToProto(&list.Items[i]))
	}

	return pbList
}

// AgentTriggerFromProto converts proto AgentTrigger to Kubernetes.
func AgentTriggerFromProto(proto *pb.AgentTrigger) *agentv1alpha1.AgentTrigger {
	if proto == nil {
		return nil
	}

	trigger := &agentv1alpha1.AgentTrigger{}
	if proto.Metadata != nil {
		trigger.ObjectMeta = *ObjectMetaFromProto(proto.Metadata)
	}
	if proto.Spec != nil {
		trigger.Spec = *AgentTriggerSpecFromProto(proto.Spec)
	}
	return trigger
}

// AgentTriggerSpecFromProto converts proto AgentTriggerSpec to Kubernetes.
func AgentTriggerSpecFromProto(proto *pb.AgentTriggerSpec) *agentv1alpha1.AgentTriggerSpec {
	if proto == nil {
		return nil
	}

	spec := &agentv1alpha1.AgentTriggerSpec{}

	if proto.EventSource != nil {
		spec.EventSource = agentv1alpha1.EventSourceRef{
			Name:      proto.EventSource.Name,
			EventName: proto.EventSource.EventName,
		}
	}

	if proto.EventBus != nil {
		spec.EventBus = &agentv1alpha1.EventBusRef{
			Name: proto.EventBus.Name,
		}
	}

	if proto.Filter != nil {
		spec.Filter = TriggerFilterFromProto(proto.Filter)
	}

	if proto.Agent != nil {
		spec.Agent = agentv1alpha1.AgentRef{
			Name:      proto.Agent.Name,
			Namespace: proto.Agent.Namespace,
		}
	}

	if proto.Task != nil {
		spec.Task = &agentv1alpha1.TriggerTaskConfig{
			SessionKeyFrom: proto.Task.SessionKeyFrom,
			Metadata:       proto.Task.Metadata,
		}
	}

	if proto.PushNotification != nil {
		spec.PushNotification = PushNotificationTargetFromProto(proto.PushNotification)
	}

	if proto.Limits != nil {
		spec.Limits = TriggerLimitsFromProto(proto.Limits)
	}

	return spec
}

// TriggerFilterFromProto converts proto TriggerFilter to Kubernetes.
func TriggerFilterFromProto(proto *pb.TriggerFilter) *agentv1alpha1.TriggerFilter {
	if proto == nil {
		return nil
	}

	filter := &agentv1alpha1.TriggerFilter{}

	for _, d := range proto.Data {
		filter.Data = append(filter.Data, agentv1alpha1.DataFilter{
			Path:       d.Path,
			Type:       d.Type,
			Value:      d.Value,
			Comparator: d.Comparator,
		})
	}

	for _, e := range proto.Exprs {
		filter.Exprs = append(filter.Exprs, agentv1alpha1.ExprFilter{
			Expr:   e.Expr,
			Fields: e.Fields,
		})
	}

	return filter
}

// PushNotificationTargetFromProto converts proto PushNotificationTarget to Kubernetes.
func PushNotificationTargetFromProto(proto *pb.PushNotificationTarget) *agentv1alpha1.PushNotificationTarget {
	if proto == nil {
		return nil
	}

	target := &agentv1alpha1.PushNotificationTarget{
		URL: proto.Url,
	}

	if proto.AgentRef != nil {
		target.AgentRef = &agentv1alpha1.AgentRef{
			Name:      proto.AgentRef.Name,
			Namespace: proto.AgentRef.Namespace,
		}
	}

	if proto.Authentication != nil {
		target.Authentication = &agentv1alpha1.TriggerPushAuth{
			Schemes: proto.Authentication.Schemes,
		}
		if proto.Authentication.CredentialsRef != nil {
			target.Authentication.CredentialsRef = SecretKeySelectorFromProto(proto.Authentication.CredentialsRef)
		}
	}

	if proto.TokenRef != nil {
		target.TokenRef = SecretKeySelectorFromProto(proto.TokenRef)
	}

	return target
}

// TriggerLimitsFromProto converts proto TriggerLimits to Kubernetes.
func TriggerLimitsFromProto(proto *pb.TriggerLimits) *agentv1alpha1.TriggerLimits {
	if proto == nil {
		return nil
	}

	limits := &agentv1alpha1.TriggerLimits{}

	if proto.MaxInvocationsPerHour > 0 {
		limits.MaxInvocationsPerHour = &proto.MaxInvocationsPerHour
	}

	if proto.MaxConcurrentTasks > 0 {
		limits.MaxConcurrentTasks = &proto.MaxConcurrentTasks
	}

	if proto.TokenBudgetPerEvent > 0 {
		limits.TokenBudgetPerEvent = &proto.TokenBudgetPerEvent
	}

	if proto.TokenBudgetPerHour > 0 {
		limits.TokenBudgetPerHour = &proto.TokenBudgetPerHour
	}

	if proto.DeadLetterSink != nil {
		limits.DeadLetterSink = &agentv1alpha1.DeadLetterSinkRef{
			URI: proto.DeadLetterSink.Uri,
		}
	}

	return limits
}

// SecretKeySelectorToProto converts a Kubernetes SecretKeySelector to proto.
func SecretKeySelectorToProto(sel *corev1.SecretKeySelector) *pb.SecretKeySelector {
	if sel == nil {
		return nil
	}

	proto := &pb.SecretKeySelector{
		Name: sel.Name,
		Key:  sel.Key,
	}

	if sel.Optional != nil {
		proto.Optional = *sel.Optional
	}

	return proto
}

// SecretKeySelectorFromProto converts proto SecretKeySelector to Kubernetes.
func SecretKeySelectorFromProto(proto *pb.SecretKeySelector) *corev1.SecretKeySelector {
	if proto == nil {
		return nil
	}

	optional := proto.Optional
	return &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{Name: proto.Name},
		Key:                  proto.Key,
		Optional:             &optional,
	}
}

// AgentTriggerPhaseToProto converts AgentTriggerPhase to proto.
func AgentTriggerPhaseToProto(phase agentv1alpha1.AgentTriggerPhase) pb.AgentTriggerPhase {
	switch phase {
	case agentv1alpha1.AgentTriggerPhasePending:
		return pb.AgentTriggerPhase_AGENT_TRIGGER_PHASE_PENDING
	case agentv1alpha1.AgentTriggerPhaseRunning:
		return pb.AgentTriggerPhase_AGENT_TRIGGER_PHASE_RUNNING
	case agentv1alpha1.AgentTriggerPhaseFailed:
		return pb.AgentTriggerPhase_AGENT_TRIGGER_PHASE_FAILED
	default:
		return pb.AgentTriggerPhase_AGENT_TRIGGER_PHASE_UNSPECIFIED
	}
}

// AgentTriggerPhaseFromProto converts proto AgentTriggerPhase to Kubernetes.
func AgentTriggerPhaseFromProto(phase pb.AgentTriggerPhase) agentv1alpha1.AgentTriggerPhase {
	switch phase {
	case pb.AgentTriggerPhase_AGENT_TRIGGER_PHASE_PENDING:
		return agentv1alpha1.AgentTriggerPhasePending
	case pb.AgentTriggerPhase_AGENT_TRIGGER_PHASE_RUNNING:
		return agentv1alpha1.AgentTriggerPhaseRunning
	case pb.AgentTriggerPhase_AGENT_TRIGGER_PHASE_FAILED:
		return agentv1alpha1.AgentTriggerPhaseFailed
	default:
		return ""
	}
}
