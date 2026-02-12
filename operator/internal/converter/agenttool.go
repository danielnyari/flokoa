package converter

// NOTE: This converter requires proto regeneration after the agenttool.proto update.
// Run `make proto-generate` to regenerate the Go proto types before building.

import (
	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// AgentToolToProto converts a Kubernetes AgentTool to proto.
func AgentToolToProto(tool *agentv1alpha1.AgentTool) *pb.AgentTool {
	if tool == nil {
		return nil
	}

	return &pb.AgentTool{
		Metadata: ObjectMetaToProto(&tool.ObjectMeta),
		Spec:     AgentToolSpecToProto(&tool.Spec),
		Status:   AgentToolStatusToProto(&tool.Status),
	}
}

// AgentToolSpecToProto converts AgentToolSpec to proto.
func AgentToolSpecToProto(spec *agentv1alpha1.AgentToolSpec) *pb.AgentToolSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.AgentToolSpec{
		Type:        AgentToolTypeToProto(spec.Type),
		Description: spec.Description,
	}

	if spec.OpenApi != nil {
		pbSpec.OpenApi = OpenApiToolSpecToProto(spec.OpenApi)
	}

	return pbSpec
}

// OpenApiToolSpecToProto converts OpenApiToolSpec to proto.
func OpenApiToolSpecToProto(spec *agentv1alpha1.OpenApiToolSpec) *pb.OpenApiToolSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.OpenApiToolSpec{
		Url: spec.URL,
	}

	if spec.TimeoutSeconds != nil {
		pbSpec.TimeoutSeconds = *spec.TimeoutSeconds
	}

	if spec.Headers != nil {
		pbSpec.Headers = spec.Headers
	}

	if spec.ServiceRef != nil {
		pbSpec.ServiceRef = &pb.ServiceRef{
			Name:      spec.ServiceRef.Name,
			Namespace: spec.ServiceRef.Namespace,
			PortName:  spec.ServiceRef.PortName,
		}
		if spec.ServiceRef.Port != nil {
			pbSpec.ServiceRef.Port = *spec.ServiceRef.Port
		}
	}

	pbSpec.OpenApiSchema = OpenApiSchemaToProto(&spec.OpenApiSchema)

	return pbSpec
}

// OpenApiSchemaToProto converts OpenApiSchema to proto.
func OpenApiSchemaToProto(schema *agentv1alpha1.OpenApiSchema) *pb.OpenApiSchema {
	if schema == nil {
		return nil
	}

	pbSchema := &pb.OpenApiSchema{
		EndpointPath: schema.EndpointPath,
	}

	if schema.ValueFrom != nil {
		pbSchema.ValueFrom = &pb.ConfigMapKeyRef{
			Name: schema.ValueFrom.Name,
			Key:  schema.ValueFrom.Key,
		}
	}

	// Note: schema.Value (runtime.RawExtension) -> proto Struct conversion
	// requires JSON parsing. Left as nil for now; implement when needed.

	return pbSchema
}

// AgentToolStatusToProto converts AgentToolStatus to proto.
func AgentToolStatusToProto(status *agentv1alpha1.AgentToolStatus) *pb.AgentToolStatus {
	if status == nil {
		return nil
	}

	return &pb.AgentToolStatus{
		Conditions:         ConditionsToProto(status.Conditions),
		ObservedGeneration: status.ObservedGeneration,
	}
}

// AgentToolListToProto converts a Kubernetes AgentToolList to proto.
func AgentToolListToProto(list *agentv1alpha1.AgentToolList) *pb.AgentToolList {
	if list == nil {
		return nil
	}

	pbList := &pb.AgentToolList{
		Metadata: ListMetaToProto(&list.ListMeta),
	}

	for i := range list.Items {
		pbList.Items = append(pbList.Items, AgentToolToProto(&list.Items[i]))
	}

	return pbList
}

// AgentToolFromProto converts proto AgentTool to Kubernetes.
func AgentToolFromProto(proto *pb.AgentTool) *agentv1alpha1.AgentTool {
	if proto == nil {
		return nil
	}

	tool := &agentv1alpha1.AgentTool{}
	if proto.Metadata != nil {
		tool.ObjectMeta = *ObjectMetaFromProto(proto.Metadata)
	}
	if proto.Spec != nil {
		tool.Spec = *AgentToolSpecFromProto(proto.Spec)
	}
	return tool
}

// AgentToolSpecFromProto converts proto AgentToolSpec to Kubernetes.
func AgentToolSpecFromProto(proto *pb.AgentToolSpec) *agentv1alpha1.AgentToolSpec {
	if proto == nil {
		return nil
	}

	spec := &agentv1alpha1.AgentToolSpec{
		Type:        AgentToolTypeFromProto(proto.Type),
		Description: proto.Description,
	}

	if proto.OpenApi != nil {
		spec.OpenApi = OpenApiToolSpecFromProto(proto.OpenApi)
	}

	return spec
}

// OpenApiToolSpecFromProto converts proto OpenApiToolSpec to Kubernetes.
func OpenApiToolSpecFromProto(proto *pb.OpenApiToolSpec) *agentv1alpha1.OpenApiToolSpec {
	if proto == nil {
		return nil
	}

	spec := &agentv1alpha1.OpenApiToolSpec{
		URL:     proto.Url,
		Headers: proto.Headers,
	}

	if proto.TimeoutSeconds > 0 {
		spec.TimeoutSeconds = &proto.TimeoutSeconds
	}

	if proto.ServiceRef != nil {
		spec.ServiceRef = &agentv1alpha1.ServiceRef{
			Name:      proto.ServiceRef.Name,
			Namespace: proto.ServiceRef.Namespace,
			PortName:  proto.ServiceRef.PortName,
		}
		if proto.ServiceRef.Port > 0 {
			spec.ServiceRef.Port = &proto.ServiceRef.Port
		}
	}

	if proto.OpenApiSchema != nil {
		spec.OpenApiSchema = *OpenApiSchemaFromProto(proto.OpenApiSchema)
	}

	return spec
}

// OpenApiSchemaFromProto converts proto OpenApiSchema to Kubernetes.
func OpenApiSchemaFromProto(proto *pb.OpenApiSchema) *agentv1alpha1.OpenApiSchema {
	if proto == nil {
		return nil
	}

	schema := &agentv1alpha1.OpenApiSchema{
		EndpointPath: proto.EndpointPath,
	}

	if proto.ValueFrom != nil {
		schema.ValueFrom = &agentv1alpha1.ConfigMapKeyRef{
			Name: proto.ValueFrom.Name,
			Key:  proto.ValueFrom.Key,
		}
	}

	// Note: proto Struct -> runtime.RawExtension conversion
	// requires JSON marshaling. Left as nil for now; implement when needed.

	return schema
}

// AgentToolTypeToProto converts AgentToolType enum to proto.
func AgentToolTypeToProto(t agentv1alpha1.AgentToolType) pb.AgentToolType {
	switch t {
	case agentv1alpha1.AgentToolTypeOpenAPI:
		return pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI
	default:
		return pb.AgentToolType_AGENT_TOOL_TYPE_UNSPECIFIED
	}
}

// AgentToolTypeFromProto converts proto AgentToolType to Kubernetes.
func AgentToolTypeFromProto(t pb.AgentToolType) agentv1alpha1.AgentToolType {
	switch t {
	case pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI:
		return agentv1alpha1.AgentToolTypeOpenAPI
	default:
		return ""
	}
}
