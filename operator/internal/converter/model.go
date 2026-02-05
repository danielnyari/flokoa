package converter

import (
	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// ModelToProto converts a Kubernetes Model to proto.
func ModelToProto(model *agentv1alpha1.Model) *pb.Model {
	if model == nil {
		return nil
	}

	return &pb.Model{
		Metadata: ObjectMetaToProto(&model.ObjectMeta),
		Spec:     ModelSpecToProto(&model.Spec),
		Status:   ModelStatusToProto(&model.Status),
	}
}

// ModelSpecToProto converts ModelSpec to proto.
func ModelSpecToProto(spec *agentv1alpha1.ModelSpec) *pb.ModelSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.ModelSpec{
		Model: spec.Model,
		ProviderRef: &pb.ProviderRef{
			Name:      spec.ProviderRef.Name,
			Namespace: spec.ProviderRef.Namespace,
		},
	}

	if spec.Parameters != nil {
		pbSpec.Parameters = ModelParametersToProto(spec.Parameters)
	}

	return pbSpec
}

// ModelParametersToProto converts ModelParameters to proto.
func ModelParametersToProto(params *agentv1alpha1.ModelParameters) *pb.ModelParameters {
	if params == nil {
		return nil
	}

	pbParams := &pb.ModelParameters{
		Temperature:      params.Temperature,
		TopP:             params.TopP,
		PresencePenalty:  params.PresencePenalty,
		FrequencyPenalty: params.FrequencyPenalty,
		StopSequences:    params.StopSequences,
		ExtraHeaders:     params.ExtraHeaders,
	}

	if params.MaxTokens != nil {
		pbParams.MaxTokens = *params.MaxTokens
	}
	if params.TopK != nil {
		pbParams.TopK = *params.TopK
	}
	if params.TimeOut != nil {
		pbParams.Timeout = *params.TimeOut
	}
	if params.ParallelToolCalls != nil {
		pbParams.ParallelToolCalls = *params.ParallelToolCalls
	}
	if params.Seed != nil {
		pbParams.Seed = *params.Seed
	}
	if params.LogitBias != nil {
		pbParams.LogitBias = params.LogitBias
	}

	return pbParams
}

// ModelStatusToProto converts ModelStatus to proto.
func ModelStatusToProto(status *agentv1alpha1.ModelStatus) *pb.ModelStatus {
	if status == nil {
		return nil
	}

	pbStatus := &pb.ModelStatus{
		Conditions:         ConditionsToProto(status.Conditions),
		ObservedGeneration: status.ObservedGeneration,
		Ready:              status.Ready,
	}

	if status.ResolvedProvider != nil {
		pbStatus.ResolvedProvider = &pb.ResolvedProviderInfo{
			Provider:  ProviderTypeToProto(status.ResolvedProvider.Provider),
			Namespace: status.ResolvedProvider.Namespace,
			Name:      status.ResolvedProvider.Name,
		}
	}

	return pbStatus
}

// ModelListToProto converts a Kubernetes ModelList to proto.
func ModelListToProto(list *agentv1alpha1.ModelList) *pb.ModelList {
	if list == nil {
		return nil
	}

	pbList := &pb.ModelList{
		Metadata: ListMetaToProto(&list.ListMeta),
	}

	for i := range list.Items {
		pbList.Items = append(pbList.Items, ModelToProto(&list.Items[i]))
	}

	return pbList
}

// ModelFromProto converts proto Model to Kubernetes.
func ModelFromProto(proto *pb.Model) *agentv1alpha1.Model {
	if proto == nil {
		return nil
	}

	model := &agentv1alpha1.Model{}
	if proto.Metadata != nil {
		model.ObjectMeta = *ObjectMetaFromProto(proto.Metadata)
	}
	if proto.Spec != nil {
		model.Spec = *ModelSpecFromProto(proto.Spec)
	}
	return model
}

// ModelSpecFromProto converts proto ModelSpec to Kubernetes.
func ModelSpecFromProto(proto *pb.ModelSpec) *agentv1alpha1.ModelSpec {
	if proto == nil {
		return nil
	}

	spec := &agentv1alpha1.ModelSpec{
		Model: proto.Model,
	}

	if proto.ProviderRef != nil {
		spec.ProviderRef = agentv1alpha1.ProviderRef{
			Name:      proto.ProviderRef.Name,
			Namespace: proto.ProviderRef.Namespace,
		}
	}

	if proto.Parameters != nil {
		spec.Parameters = ModelParametersFromProto(proto.Parameters)
	}

	return spec
}

// ModelParametersFromProto converts proto ModelParameters to Kubernetes.
func ModelParametersFromProto(proto *pb.ModelParameters) *agentv1alpha1.ModelParameters {
	if proto == nil {
		return nil
	}

	params := &agentv1alpha1.ModelParameters{
		Temperature:      proto.Temperature,
		TopP:             proto.TopP,
		PresencePenalty:  proto.PresencePenalty,
		FrequencyPenalty: proto.FrequencyPenalty,
		StopSequences:    proto.StopSequences,
		ExtraHeaders:     proto.ExtraHeaders,
		LogitBias:        proto.LogitBias,
	}

	if proto.MaxTokens > 0 {
		params.MaxTokens = &proto.MaxTokens
	}
	if proto.TopK > 0 {
		params.TopK = &proto.TopK
	}
	if proto.Timeout > 0 {
		params.TimeOut = &proto.Timeout
	}
	if proto.ParallelToolCalls {
		params.ParallelToolCalls = &proto.ParallelToolCalls
	}
	if proto.Seed != 0 {
		params.Seed = &proto.Seed
	}

	return params
}

// ProviderTypeToProto converts ProviderType enum to proto.
func ProviderTypeToProto(pt agentv1alpha1.ProviderType) pb.ProviderType {
	switch pt {
	case agentv1alpha1.ProviderTypeOpenAI:
		return pb.ProviderType_PROVIDER_TYPE_OPENAI
	case agentv1alpha1.ProviderTypeAnthropic:
		return pb.ProviderType_PROVIDER_TYPE_ANTHROPIC
	case agentv1alpha1.ProviderTypeGoogle:
		return pb.ProviderType_PROVIDER_TYPE_GOOGLE
	case agentv1alpha1.ProviderTypeBedrock:
		return pb.ProviderType_PROVIDER_TYPE_BEDROCK
	default:
		return pb.ProviderType_PROVIDER_TYPE_UNSPECIFIED
	}
}

// ProviderTypeFromProto converts proto ProviderType to Kubernetes.
func ProviderTypeFromProto(pt pb.ProviderType) agentv1alpha1.ProviderType {
	switch pt {
	case pb.ProviderType_PROVIDER_TYPE_OPENAI:
		return agentv1alpha1.ProviderTypeOpenAI
	case pb.ProviderType_PROVIDER_TYPE_ANTHROPIC:
		return agentv1alpha1.ProviderTypeAnthropic
	case pb.ProviderType_PROVIDER_TYPE_GOOGLE:
		return agentv1alpha1.ProviderTypeGoogle
	case pb.ProviderType_PROVIDER_TYPE_BEDROCK:
		return agentv1alpha1.ProviderTypeBedrock
	default:
		return ""
	}
}
