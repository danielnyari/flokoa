package converter

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

	if spec.HTTPApi != nil {
		pbSpec.HttpApi = HTTPApiSpecToProto(spec.HTTPApi)
	}

	return pbSpec
}

// HTTPApiSpecToProto converts HTTPApiSpec to proto.
func HTTPApiSpecToProto(spec *agentv1alpha1.HTTPApiSpec) *pb.HTTPApiSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.HTTPApiSpec{
		Url:    spec.URL,
		Path:   spec.Path,
		Method: HTTPMethodToProto(spec.Method),
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

	return pbSpec
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

	if proto.HttpApi != nil {
		spec.HTTPApi = HTTPApiSpecFromProto(proto.HttpApi)
	}

	return spec
}

// HTTPApiSpecFromProto converts proto HTTPApiSpec to Kubernetes.
func HTTPApiSpecFromProto(proto *pb.HTTPApiSpec) *agentv1alpha1.HTTPApiSpec {
	if proto == nil {
		return nil
	}

	spec := &agentv1alpha1.HTTPApiSpec{
		URL:     proto.Url,
		Path:    proto.Path,
		Method:  HTTPMethodFromProto(proto.Method),
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

	return spec
}

// AgentToolTypeToProto converts AgentToolType enum to proto.
func AgentToolTypeToProto(t agentv1alpha1.AgentToolType) pb.AgentToolType {
	switch t {
	case agentv1alpha1.AgentToolTypeHTTPAPI:
		return pb.AgentToolType_AGENT_TOOL_TYPE_HTTP_API
	default:
		return pb.AgentToolType_AGENT_TOOL_TYPE_UNSPECIFIED
	}
}

// AgentToolTypeFromProto converts proto AgentToolType to Kubernetes.
func AgentToolTypeFromProto(t pb.AgentToolType) agentv1alpha1.AgentToolType {
	switch t {
	case pb.AgentToolType_AGENT_TOOL_TYPE_HTTP_API:
		return agentv1alpha1.AgentToolTypeHTTPAPI
	default:
		return ""
	}
}

// HTTPMethodToProto converts HTTPMethod enum to proto.
func HTTPMethodToProto(m agentv1alpha1.HTTPMethod) pb.HTTPMethod {
	switch m {
	case agentv1alpha1.HTTPMethodGet:
		return pb.HTTPMethod_HTTP_METHOD_GET
	case agentv1alpha1.HTTPMethodPost:
		return pb.HTTPMethod_HTTP_METHOD_POST
	case agentv1alpha1.HTTPMethodPut:
		return pb.HTTPMethod_HTTP_METHOD_PUT
	case agentv1alpha1.HTTPMethodPatch:
		return pb.HTTPMethod_HTTP_METHOD_PATCH
	case agentv1alpha1.HTTPMethodDelete:
		return pb.HTTPMethod_HTTP_METHOD_DELETE
	default:
		return pb.HTTPMethod_HTTP_METHOD_UNSPECIFIED
	}
}

// HTTPMethodFromProto converts proto HTTPMethod to Kubernetes.
func HTTPMethodFromProto(m pb.HTTPMethod) agentv1alpha1.HTTPMethod {
	switch m {
	case pb.HTTPMethod_HTTP_METHOD_GET:
		return agentv1alpha1.HTTPMethodGet
	case pb.HTTPMethod_HTTP_METHOD_POST:
		return agentv1alpha1.HTTPMethodPost
	case pb.HTTPMethod_HTTP_METHOD_PUT:
		return agentv1alpha1.HTTPMethodPut
	case pb.HTTPMethod_HTTP_METHOD_PATCH:
		return agentv1alpha1.HTTPMethodPatch
	case pb.HTTPMethod_HTTP_METHOD_DELETE:
		return agentv1alpha1.HTTPMethodDelete
	default:
		return ""
	}
}
