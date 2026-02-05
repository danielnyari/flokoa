package converter

import (
	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// ModelProviderToProto converts a Kubernetes ModelProvider to proto.
func ModelProviderToProto(provider *agentv1alpha1.ModelProvider) *pb.ModelProvider {
	if provider == nil {
		return nil
	}

	return &pb.ModelProvider{
		Metadata: ObjectMetaToProto(&provider.ObjectMeta),
		Spec:     ModelProviderSpecToProto(&provider.Spec),
		Status:   ModelProviderStatusToProto(&provider.Status),
	}
}

// ModelProviderSpecToProto converts ModelProviderSpec to proto.
func ModelProviderSpecToProto(spec *agentv1alpha1.ModelProviderSpec) *pb.ModelProviderSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.ModelProviderSpec{
		DefaultHeaders: spec.DefaultHeaders,
	}

	if spec.APIKeySecretRef != nil {
		pbSpec.ApiKeySecretRef = &pb.SecretKeySelector{
			Name: spec.APIKeySecretRef.Name,
			Key:  spec.APIKeySecretRef.Key,
		}
	}

	if spec.OpenAI != nil {
		pbSpec.Openai = &pb.OpenAIProviderSpec{
			BaseUrl:        spec.OpenAI.BaseURL,
			OrganizationId: spec.OpenAI.OrganizationID,
		}
		if spec.OpenAI.TimeoutSeconds != nil {
			pbSpec.Openai.TimeoutSeconds = *spec.OpenAI.TimeoutSeconds
		}
	}

	if spec.Anthropic != nil {
		pbSpec.Anthropic = &pb.AnthropicProviderSpec{
			BaseUrl: spec.Anthropic.BaseURL,
		}
		if spec.Anthropic.TimeoutSeconds != nil {
			pbSpec.Anthropic.TimeoutSeconds = *spec.Anthropic.TimeoutSeconds
		}
	}

	if spec.Google != nil {
		pbSpec.Google = &pb.GoogleProviderSpec{
			Project:  spec.Google.Project,
			Location: spec.Google.Location,
		}
		if spec.Google.TimeoutSeconds != nil {
			pbSpec.Google.TimeoutSeconds = *spec.Google.TimeoutSeconds
		}
	}

	if spec.Bedrock != nil {
		pbSpec.Bedrock = &pb.BedrockProviderSpec{
			Region:              spec.Bedrock.Region,
			InferenceProfileArn: spec.Bedrock.InferenceProfileARN,
		}
	}

	if spec.TLS != nil {
		pbSpec.Tls = &pb.TLSConfig{
			InsecureSkipVerify: spec.TLS.InsecureSkipVerify,
		}
		if spec.TLS.UseSystemCAs != nil {
			pbSpec.Tls.UseSystemCas = *spec.TLS.UseSystemCAs
		}
	}

	return pbSpec
}

// ModelProviderStatusToProto converts ModelProviderStatus to proto.
func ModelProviderStatusToProto(status *agentv1alpha1.ModelProviderStatus) *pb.ModelProviderStatus {
	if status == nil {
		return nil
	}

	return &pb.ModelProviderStatus{
		Provider:           ProviderTypeToProto(status.Provider),
		Conditions:         ConditionsToProto(status.Conditions),
		ObservedGeneration: status.ObservedGeneration,
		SecretHash:         status.SecretHash,
		Ready:              status.Ready,
	}
}

// ModelProviderListToProto converts a Kubernetes ModelProviderList to proto.
func ModelProviderListToProto(list *agentv1alpha1.ModelProviderList) *pb.ModelProviderList {
	if list == nil {
		return nil
	}

	pbList := &pb.ModelProviderList{
		Metadata: ListMetaToProto(&list.ListMeta),
	}

	for i := range list.Items {
		pbList.Items = append(pbList.Items, ModelProviderToProto(&list.Items[i]))
	}

	return pbList
}

// ModelProviderFromProto converts proto ModelProvider to Kubernetes.
func ModelProviderFromProto(proto *pb.ModelProvider) *agentv1alpha1.ModelProvider {
	if proto == nil {
		return nil
	}

	provider := &agentv1alpha1.ModelProvider{}
	if proto.Metadata != nil {
		provider.ObjectMeta = *ObjectMetaFromProto(proto.Metadata)
	}
	if proto.Spec != nil {
		provider.Spec = *ModelProviderSpecFromProto(proto.Spec)
	}
	return provider
}

// ModelProviderSpecFromProto converts proto ModelProviderSpec to Kubernetes.
func ModelProviderSpecFromProto(proto *pb.ModelProviderSpec) *agentv1alpha1.ModelProviderSpec {
	if proto == nil {
		return nil
	}

	spec := &agentv1alpha1.ModelProviderSpec{
		DefaultHeaders: proto.DefaultHeaders,
	}

	if proto.Openai != nil {
		spec.OpenAI = &agentv1alpha1.OpenAIProviderSpec{
			BaseURL:        proto.Openai.BaseUrl,
			OrganizationID: proto.Openai.OrganizationId,
		}
		if proto.Openai.TimeoutSeconds > 0 {
			spec.OpenAI.TimeoutSeconds = &proto.Openai.TimeoutSeconds
		}
	}

	if proto.Anthropic != nil {
		spec.Anthropic = &agentv1alpha1.AnthropicProviderSpec{
			BaseURL: proto.Anthropic.BaseUrl,
		}
		if proto.Anthropic.TimeoutSeconds > 0 {
			spec.Anthropic.TimeoutSeconds = &proto.Anthropic.TimeoutSeconds
		}
	}

	if proto.Google != nil {
		spec.Google = &agentv1alpha1.GoogleProviderSpec{
			Project:  proto.Google.Project,
			Location: proto.Google.Location,
		}
		if proto.Google.TimeoutSeconds > 0 {
			spec.Google.TimeoutSeconds = &proto.Google.TimeoutSeconds
		}
	}

	if proto.Bedrock != nil {
		spec.Bedrock = &agentv1alpha1.BedrockProviderSpec{
			Region:              proto.Bedrock.Region,
			InferenceProfileARN: proto.Bedrock.InferenceProfileArn,
		}
	}

	return spec
}
