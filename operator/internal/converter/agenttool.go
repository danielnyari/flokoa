package converter

import (
	corev1 "k8s.io/api/core/v1"

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
		Type:         AgentToolTypeToProto(spec.Type),
		Description:  spec.Description,
		Url:          spec.URL,
		Path:         spec.Path,
		Transport:    MCPTransportToProto(spec.Transport),
		Headers:      spec.Headers,
		ToolPrefix:   spec.ToolPrefix,
		AllowedTools: spec.AllowedTools,
	}
	if spec.ServiceRef != nil {
		pbSpec.ServiceRef = ServiceRefToProto(spec.ServiceRef)
	}
	for _, hs := range spec.HeaderSecrets {
		pbSpec.HeaderSecrets = append(pbSpec.HeaderSecrets, &pb.SecretHeader{
			Name: hs.Name,
			SecretRef: &pb.SecretKeySelector{
				Name:     hs.SecretRef.Name,
				Key:      hs.SecretRef.Key,
				Optional: hs.SecretRef.Optional != nil && *hs.SecretRef.Optional,
			},
		})
	}
	if spec.TimeoutSeconds != nil {
		pbSpec.TimeoutSeconds = *spec.TimeoutSeconds
	}

	return pbSpec
}

// ServiceRefToProto converts a ServiceRef to proto.
func ServiceRefToProto(ref *agentv1alpha1.ServiceRef) *pb.ServiceRef {
	if ref == nil {
		return nil
	}
	pbRef := &pb.ServiceRef{
		Name:      ref.Name,
		Namespace: ref.Namespace,
		PortName:  ref.PortName,
	}
	if ref.Port != nil {
		pbRef.Port = *ref.Port
	}
	return pbRef
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

// AgentToolFromProto converts a proto AgentTool to the Kubernetes type.
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

// AgentToolSpecFromProto converts a proto AgentToolSpec to the Kubernetes type.
func AgentToolSpecFromProto(proto *pb.AgentToolSpec) *agentv1alpha1.AgentToolSpec {
	if proto == nil {
		return nil
	}

	spec := &agentv1alpha1.AgentToolSpec{
		Type:         AgentToolTypeFromProto(proto.Type),
		Description:  proto.Description,
		URL:          proto.Url,
		Path:         proto.Path,
		Transport:    MCPTransportFromProto(proto.Transport),
		Headers:      proto.Headers,
		ToolPrefix:   proto.ToolPrefix,
		AllowedTools: proto.AllowedTools,
	}
	if proto.ServiceRef != nil {
		spec.ServiceRef = &agentv1alpha1.ServiceRef{
			Name:      proto.ServiceRef.Name,
			Namespace: proto.ServiceRef.Namespace,
			PortName:  proto.ServiceRef.PortName,
		}
		if proto.ServiceRef.Port != 0 {
			spec.ServiceRef.Port = &proto.ServiceRef.Port
		}
	}
	for _, hs := range proto.HeaderSecrets {
		header := agentv1alpha1.SecretHeader{Name: hs.Name}
		if hs.SecretRef != nil {
			header.SecretRef = corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: hs.SecretRef.Name},
				Key:                  hs.SecretRef.Key,
			}
			if hs.SecretRef.Optional {
				optional := true
				header.SecretRef.Optional = &optional
			}
		}
		spec.HeaderSecrets = append(spec.HeaderSecrets, header)
	}
	if proto.TimeoutSeconds != 0 {
		spec.TimeoutSeconds = &proto.TimeoutSeconds
	}

	return spec
}

// AgentToolTypeToProto converts the tool type enum to proto.
func AgentToolTypeToProto(t agentv1alpha1.AgentToolType) pb.AgentToolType {
	switch t {
	case agentv1alpha1.AgentToolTypeMCP:
		return pb.AgentToolType_AGENT_TOOL_TYPE_MCP
	case agentv1alpha1.AgentToolTypeOpenAPI:
		return pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI
	default:
		return pb.AgentToolType_AGENT_TOOL_TYPE_UNSPECIFIED
	}
}

// AgentToolTypeFromProto converts the proto tool type to the Kubernetes enum.
func AgentToolTypeFromProto(t pb.AgentToolType) agentv1alpha1.AgentToolType {
	switch t {
	case pb.AgentToolType_AGENT_TOOL_TYPE_MCP:
		return agentv1alpha1.AgentToolTypeMCP
	case pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI:
		return agentv1alpha1.AgentToolTypeOpenAPI
	default:
		return ""
	}
}

// MCPTransportToProto converts the transport enum to proto.
func MCPTransportToProto(t agentv1alpha1.MCPTransport) pb.MCPTransport {
	switch t {
	case agentv1alpha1.MCPTransportStreamableHTTP:
		return pb.MCPTransport_MCP_TRANSPORT_STREAMABLE_HTTP
	case agentv1alpha1.MCPTransportSSE:
		return pb.MCPTransport_MCP_TRANSPORT_SSE
	default:
		return pb.MCPTransport_MCP_TRANSPORT_UNSPECIFIED
	}
}

// MCPTransportFromProto converts the proto transport to the Kubernetes enum.
func MCPTransportFromProto(t pb.MCPTransport) agentv1alpha1.MCPTransport {
	switch t {
	case pb.MCPTransport_MCP_TRANSPORT_STREAMABLE_HTTP:
		return agentv1alpha1.MCPTransportStreamableHTTP
	case pb.MCPTransport_MCP_TRANSPORT_SSE:
		return agentv1alpha1.MCPTransportSSE
	default:
		return ""
	}
}
