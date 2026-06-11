package converter

import (
	"encoding/json"

	"google.golang.org/protobuf/types/known/structpb"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

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

	return &pb.ModelSpec{
		Model: spec.Model,
		ProviderRef: &pb.ProviderRef{
			Name:      spec.ProviderRef.Name,
			Namespace: spec.ProviderRef.Namespace,
		},
		Settings: ModelSettingsToProto(spec.Settings),
	}
}

// ModelSettingsToProto converts ModelSettings to proto.
func ModelSettingsToProto(settings *agentv1alpha1.ModelSettings) *pb.ModelSettings {
	if settings == nil {
		return nil
	}

	pbSettings := &pb.ModelSettings{
		Temperature:      settings.Temperature,
		TopP:             settings.TopP,
		PresencePenalty:  settings.PresencePenalty,
		FrequencyPenalty: settings.FrequencyPenalty,
		LogitBias:        settings.LogitBias,
		StopSequences:    settings.StopSequences,
		ExtraHeaders:     settings.ExtraHeaders,
	}
	if settings.MaxTokens != nil {
		pbSettings.MaxTokens = *settings.MaxTokens
	}
	if settings.TopK != nil {
		pbSettings.TopK = *settings.TopK
	}
	if settings.TimeoutSeconds != nil {
		pbSettings.TimeoutSeconds = *settings.TimeoutSeconds
	}
	if settings.ParallelToolCalls != nil {
		pbSettings.ParallelToolCalls = *settings.ParallelToolCalls
	}
	if settings.Seed != nil {
		pbSettings.Seed = *settings.Seed
	}
	pbSettings.Extra = jsonToStruct(settings.Extra)

	return pbSettings
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

// ModelFromProto converts a proto Model to the Kubernetes type.
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

// ModelSpecFromProto converts a proto ModelSpec to the Kubernetes type.
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
	if proto.Settings != nil {
		spec.Settings = ModelSettingsFromProto(proto.Settings)
	}

	return spec
}

// ModelSettingsFromProto converts proto ModelSettings to the Kubernetes type.
func ModelSettingsFromProto(proto *pb.ModelSettings) *agentv1alpha1.ModelSettings {
	if proto == nil {
		return nil
	}

	settings := &agentv1alpha1.ModelSettings{
		Temperature:      proto.Temperature,
		TopP:             proto.TopP,
		PresencePenalty:  proto.PresencePenalty,
		FrequencyPenalty: proto.FrequencyPenalty,
		LogitBias:        proto.LogitBias,
		StopSequences:    proto.StopSequences,
		ExtraHeaders:     proto.ExtraHeaders,
	}
	if proto.MaxTokens != 0 {
		settings.MaxTokens = &proto.MaxTokens
	}
	if proto.TopK != 0 {
		settings.TopK = &proto.TopK
	}
	if proto.TimeoutSeconds != 0 {
		settings.TimeoutSeconds = &proto.TimeoutSeconds
	}
	if proto.ParallelToolCalls {
		parallel := true
		settings.ParallelToolCalls = &parallel
	}
	if proto.Seed != 0 {
		settings.Seed = &proto.Seed
	}
	if proto.Extra != nil {
		settings.Extra = structToJSON(proto.Extra)
	}

	return settings
}

// structToJSON converts a proto Struct to an apiextensions JSON object.
func structToJSON(s *structpb.Struct) *apiextensionsv1.JSON {
	if s == nil {
		return nil
	}
	raw, err := json.Marshal(s.AsMap())
	if err != nil {
		return nil
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

// ProviderTypeToProto converts the provider type enum to proto.
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

// ProviderTypeFromProto converts the proto provider type to the Kubernetes enum.
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
