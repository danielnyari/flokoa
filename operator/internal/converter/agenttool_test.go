package converter

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

const testToolNS = "default"

const testMCPURL = "https://mcp.example.com/mcp"

func TestAgentToolToProto_Nil(t *testing.T) {
	result := AgentToolToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToolToProto(t *testing.T) {
	tool := &agentv1alpha1.AgentTool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "petstore",
			Namespace: "tools",
		},
		Spec: agentv1alpha1.AgentToolSpec{
			Type:        agentv1alpha1.AgentToolTypeMCP,
			Description: "Petstore MCP server",
			URL:         testMCPURL,
			Transport:   agentv1alpha1.MCPTransportStreamableHTTP,
		},
		Status: agentv1alpha1.AgentToolStatus{
			ObservedGeneration: 4,
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"},
			},
		},
	}

	result := AgentToolToProto(tool)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Metadata == nil || result.Metadata.Name != "petstore" {
		t.Fatal("expected metadata with name petstore")
	}
	if result.Spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if result.Spec.Type != pb.AgentToolType_AGENT_TOOL_TYPE_MCP {
		t.Fatalf("expected mcp type, got %v", result.Spec.Type)
	}
	if result.Status == nil {
		t.Fatal("expected non-nil status")
	}
	if result.Status.ObservedGeneration != 4 {
		t.Fatalf("expected observed gen 4, got %d", result.Status.ObservedGeneration)
	}
	if len(result.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(result.Status.Conditions))
	}
}

func TestAgentToolSpecToProto_Nil(t *testing.T) {
	result := AgentToolSpecToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToolSpecToProto_URLVariant(t *testing.T) {
	timeout := int32(45)
	optional := true

	spec := &agentv1alpha1.AgentToolSpec{
		Type:        agentv1alpha1.AgentToolTypeMCP,
		Description: "Weather MCP server",
		URL:         testMCPURL,
		Transport:   agentv1alpha1.MCPTransportStreamableHTTP,
		Headers: map[string]string{
			"X-Client": "flokoa",
		},
		HeaderSecrets: []agentv1alpha1.SecretHeader{
			{
				Name: "Authorization",
				SecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "mcp-secret"},
					Key:                  "token",
					Optional:             &optional,
				},
			},
		},
		ToolPrefix:     "weather",
		AllowedTools:   []string{"search", "forecast"},
		TimeoutSeconds: &timeout,
	}

	result := AgentToolSpecToProto(spec)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Type != pb.AgentToolType_AGENT_TOOL_TYPE_MCP {
		t.Fatalf("expected mcp type, got %v", result.Type)
	}
	if result.Description != "Weather MCP server" {
		t.Fatalf("expected description, got %q", result.Description)
	}
	if result.Url != testMCPURL {
		t.Fatalf("expected url %s, got %q", testMCPURL, result.Url)
	}
	if result.ServiceRef != nil {
		t.Fatal("expected nil service ref for URL variant")
	}
	if result.Transport != pb.MCPTransport_MCP_TRANSPORT_STREAMABLE_HTTP {
		t.Fatalf("expected streamable HTTP transport, got %v", result.Transport)
	}
	if result.Headers["X-Client"] != "flokoa" {
		t.Fatal("expected header X-Client=flokoa")
	}
	if len(result.HeaderSecrets) != 1 {
		t.Fatalf("expected 1 header secret, got %d", len(result.HeaderSecrets))
	}
	hs := result.HeaderSecrets[0]
	if hs.Name != "Authorization" {
		t.Fatalf("expected header secret name Authorization, got %q", hs.Name)
	}
	if hs.SecretRef == nil || hs.SecretRef.Name != "mcp-secret" || hs.SecretRef.Key != "token" {
		t.Fatalf("expected secret ref mcp-secret/token, got %v", hs.SecretRef)
	}
	if !hs.SecretRef.Optional {
		t.Fatal("expected optional true")
	}
	if result.ToolPrefix != "weather" {
		t.Fatalf("expected tool prefix weather, got %q", result.ToolPrefix)
	}
	if len(result.AllowedTools) != 2 {
		t.Fatalf("expected 2 allowed tools, got %d", len(result.AllowedTools))
	}
	if result.TimeoutSeconds != 45 {
		t.Fatalf("expected timeout 45, got %d", result.TimeoutSeconds)
	}
}

func TestAgentToolSpecToProto_ServiceRefVariant(t *testing.T) {
	port := int32(8080)
	spec := &agentv1alpha1.AgentToolSpec{
		Type: agentv1alpha1.AgentToolTypeMCP,
		ServiceRef: &agentv1alpha1.ServiceRef{
			Name:      "weather-svc",
			Namespace: testToolNS,
			Port:      &port,
		},
		Path:      "/mcp",
		Transport: agentv1alpha1.MCPTransportSSE,
	}

	result := AgentToolSpecToProto(spec)
	if result.Url != "" {
		t.Fatalf("expected empty url for service ref variant, got %q", result.Url)
	}
	if result.ServiceRef == nil {
		t.Fatal("expected service ref to be set")
	}
	if result.ServiceRef.Name != "weather-svc" || result.ServiceRef.Namespace != testToolNS {
		t.Fatalf("expected default/weather-svc, got %v", result.ServiceRef)
	}
	if result.ServiceRef.Port != 8080 {
		t.Fatalf("expected port 8080, got %d", result.ServiceRef.Port)
	}
	if result.Path != "/mcp" {
		t.Fatalf("expected path /mcp, got %q", result.Path)
	}
	if result.Transport != pb.MCPTransport_MCP_TRANSPORT_SSE {
		t.Fatalf("expected sse transport, got %v", result.Transport)
	}
}

func TestAgentToolSpecToProto_NilOptionalFields(t *testing.T) {
	spec := &agentv1alpha1.AgentToolSpec{
		Type: agentv1alpha1.AgentToolTypeMCP,
		URL:  testMCPURL,
	}

	result := AgentToolSpecToProto(spec)
	if result.TimeoutSeconds != 0 {
		t.Fatalf("expected 0 timeout for nil, got %d", result.TimeoutSeconds)
	}
	if result.ServiceRef != nil {
		t.Fatal("expected nil service ref")
	}
	if len(result.HeaderSecrets) != 0 {
		t.Fatalf("expected no header secrets, got %d", len(result.HeaderSecrets))
	}
	if result.Transport != pb.MCPTransport_MCP_TRANSPORT_UNSPECIFIED {
		t.Fatalf("expected unspecified transport for empty string, got %v", result.Transport)
	}
}

func TestServiceRefToProto_Nil(t *testing.T) {
	result := ServiceRefToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestServiceRefToProto(t *testing.T) {
	port := int32(9090)
	ref := &agentv1alpha1.ServiceRef{
		Name:      "svc",
		Namespace: "ns",
		Port:      &port,
		PortName:  "http",
	}

	result := ServiceRefToProto(ref)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "svc" {
		t.Fatalf("expected name svc, got %q", result.Name)
	}
	if result.Namespace != "ns" {
		t.Fatalf("expected namespace ns, got %q", result.Namespace)
	}
	if result.Port != 9090 {
		t.Fatalf("expected port 9090, got %d", result.Port)
	}
	if result.PortName != "http" {
		t.Fatalf("expected port name http, got %q", result.PortName)
	}
}

func TestServiceRefToProto_NilPort(t *testing.T) {
	ref := &agentv1alpha1.ServiceRef{
		Name:     "svc",
		PortName: "grpc",
	}

	result := ServiceRefToProto(ref)
	if result.Port != 0 {
		t.Fatalf("expected 0 port for nil, got %d", result.Port)
	}
	if result.PortName != "grpc" {
		t.Fatalf("expected port name grpc, got %q", result.PortName)
	}
}

func TestAgentToolStatusToProto_Nil(t *testing.T) {
	result := AgentToolStatusToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToolStatusToProto(t *testing.T) {
	status := &agentv1alpha1.AgentToolStatus{
		ObservedGeneration: 9,
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionFalse, Reason: "Pending"},
		},
	}

	result := AgentToolStatusToProto(status)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ObservedGeneration != 9 {
		t.Fatalf("expected observed gen 9, got %d", result.ObservedGeneration)
	}
	if len(result.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(result.Conditions))
	}
}

func TestAgentToolListToProto_Nil(t *testing.T) {
	result := AgentToolListToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToolListToProto(t *testing.T) {
	list := &agentv1alpha1.AgentToolList{
		ListMeta: metav1.ListMeta{ResourceVersion: "7"},
		Items: []agentv1alpha1.AgentTool{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "tool-1"},
				Spec:       agentv1alpha1.AgentToolSpec{Type: agentv1alpha1.AgentToolTypeMCP, URL: testMCPURL},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "tool-2"},
				Spec:       agentv1alpha1.AgentToolSpec{Type: agentv1alpha1.AgentToolTypeMCP, URL: testMCPURL},
			},
		},
	}

	result := AgentToolListToProto(list)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Metadata == nil || result.Metadata.ResourceVersion != "7" {
		t.Fatal("expected metadata")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Items[0].Metadata.Name != "tool-1" {
		t.Fatalf("expected tool-1, got %q", result.Items[0].Metadata.Name)
	}
}

func TestAgentToolListToProto_Empty(t *testing.T) {
	list := &agentv1alpha1.AgentToolList{
		Items: []agentv1alpha1.AgentTool{},
	}

	result := AgentToolListToProto(list)
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
}

func TestAgentToolFromProto_Nil(t *testing.T) {
	result := AgentToolFromProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToolFromProto(t *testing.T) {
	proto := &pb.AgentTool{
		Metadata: &pb.ObjectMeta{
			Name:      "petstore",
			Namespace: "tools",
		},
		Spec: &pb.AgentToolSpec{
			Type:        pb.AgentToolType_AGENT_TOOL_TYPE_MCP,
			Description: "Petstore MCP server",
			Url:         testMCPURL,
			Transport:   pb.MCPTransport_MCP_TRANSPORT_STREAMABLE_HTTP,
		},
	}

	result := AgentToolFromProto(proto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "petstore" || result.Namespace != "tools" {
		t.Fatalf("expected tools/petstore, got %s/%s", result.Namespace, result.Name)
	}
	if result.Spec.Type != agentv1alpha1.AgentToolTypeMCP {
		t.Fatalf("expected mcp type, got %q", result.Spec.Type)
	}
	if result.Spec.URL != testMCPURL {
		t.Fatalf("expected url, got %q", result.Spec.URL)
	}
	if result.Spec.Transport != agentv1alpha1.MCPTransportStreamableHTTP {
		t.Fatalf("expected streamableHTTP transport, got %q", result.Spec.Transport)
	}
}

func TestAgentToolFromProto_NilFields(t *testing.T) {
	result := AgentToolFromProto(&pb.AgentTool{})
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "" {
		t.Fatalf("expected empty name, got %q", result.Name)
	}
}

func TestAgentToolSpecFromProto_Nil(t *testing.T) {
	result := AgentToolSpecFromProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestAgentToolSpecFromProto_ServiceRef(t *testing.T) {
	proto := &pb.AgentToolSpec{
		Type: pb.AgentToolType_AGENT_TOOL_TYPE_MCP,
		ServiceRef: &pb.ServiceRef{
			Name:      "weather-svc",
			Namespace: testToolNS,
			Port:      8080,
		},
		Path:      "/sse",
		Transport: pb.MCPTransport_MCP_TRANSPORT_SSE,
	}

	result := AgentToolSpecFromProto(proto)
	if result.ServiceRef == nil {
		t.Fatal("expected service ref to be set")
	}
	if result.ServiceRef.Name != "weather-svc" || result.ServiceRef.Namespace != testToolNS {
		t.Fatalf("expected default/weather-svc, got %v", result.ServiceRef)
	}
	if result.ServiceRef.Port == nil || *result.ServiceRef.Port != 8080 {
		t.Fatal("expected port 8080")
	}
	if result.Path != "/sse" {
		t.Fatalf("expected path /sse, got %q", result.Path)
	}
	if result.Transport != agentv1alpha1.MCPTransportSSE {
		t.Fatalf("expected sse transport, got %q", result.Transport)
	}
}

func TestAgentToolSpecFromProto_ZeroPort(t *testing.T) {
	proto := &pb.AgentToolSpec{
		ServiceRef: &pb.ServiceRef{
			Name:     "svc",
			PortName: "http",
		},
	}

	result := AgentToolSpecFromProto(proto)
	if result.ServiceRef.Port != nil {
		t.Fatal("expected nil port for 0")
	}
	if result.ServiceRef.PortName != "http" {
		t.Fatalf("expected port name http, got %q", result.ServiceRef.PortName)
	}
}

func TestAgentToolSpecRoundTrip(t *testing.T) {
	timeout := int32(45)
	optional := true

	original := &agentv1alpha1.AgentToolSpec{
		Type:        agentv1alpha1.AgentToolTypeMCP,
		Description: "Weather MCP server",
		URL:         testMCPURL,
		Transport:   agentv1alpha1.MCPTransportStreamableHTTP,
		Headers: map[string]string{
			"X-Client": "flokoa",
		},
		HeaderSecrets: []agentv1alpha1.SecretHeader{
			{
				Name: "Authorization",
				SecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "mcp-secret"},
					Key:                  "token",
					Optional:             &optional,
				},
			},
			{
				Name: "X-Api-Key",
				SecretRef: corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "key-secret"},
					Key:                  "api-key",
				},
			},
		},
		ToolPrefix:     "weather",
		AllowedTools:   []string{"search", "forecast"},
		TimeoutSeconds: &timeout,
	}

	roundTrip := AgentToolSpecFromProto(AgentToolSpecToProto(original))
	if roundTrip == nil {
		t.Fatal("expected non-nil round-trip spec")
	}
	if !reflect.DeepEqual(original, roundTrip) {
		t.Fatalf("expected round-trip to preserve spec:\noriginal:  %#v\nroundTrip: %#v", original, roundTrip)
	}
}

func TestAgentToolTypeToProto(t *testing.T) {
	tests := []struct {
		input    agentv1alpha1.AgentToolType
		expected pb.AgentToolType
	}{
		{agentv1alpha1.AgentToolTypeMCP, pb.AgentToolType_AGENT_TOOL_TYPE_MCP},
		{agentv1alpha1.AgentToolTypeOpenAPI, pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI},
		{agentv1alpha1.AgentToolType("unknown"), pb.AgentToolType_AGENT_TOOL_TYPE_UNSPECIFIED},
		{agentv1alpha1.AgentToolType(""), pb.AgentToolType_AGENT_TOOL_TYPE_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := AgentToolTypeToProto(tt.input)
			if result != tt.expected {
				t.Fatalf("AgentToolTypeToProto(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAgentToolTypeFromProto(t *testing.T) {
	tests := []struct {
		input    pb.AgentToolType
		expected agentv1alpha1.AgentToolType
	}{
		{pb.AgentToolType_AGENT_TOOL_TYPE_MCP, agentv1alpha1.AgentToolTypeMCP},
		{pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI, agentv1alpha1.AgentToolTypeOpenAPI},
		{pb.AgentToolType_AGENT_TOOL_TYPE_UNSPECIFIED, agentv1alpha1.AgentToolType("")},
	}

	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			result := AgentToolTypeFromProto(tt.input)
			if result != tt.expected {
				t.Fatalf("AgentToolTypeFromProto(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMCPTransportToProto(t *testing.T) {
	tests := []struct {
		input    agentv1alpha1.MCPTransport
		expected pb.MCPTransport
	}{
		{agentv1alpha1.MCPTransportStreamableHTTP, pb.MCPTransport_MCP_TRANSPORT_STREAMABLE_HTTP},
		{agentv1alpha1.MCPTransportSSE, pb.MCPTransport_MCP_TRANSPORT_SSE},
		{agentv1alpha1.MCPTransport("unknown"), pb.MCPTransport_MCP_TRANSPORT_UNSPECIFIED},
		{agentv1alpha1.MCPTransport(""), pb.MCPTransport_MCP_TRANSPORT_UNSPECIFIED},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := MCPTransportToProto(tt.input)
			if result != tt.expected {
				t.Fatalf("MCPTransportToProto(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMCPTransportFromProto(t *testing.T) {
	tests := []struct {
		input    pb.MCPTransport
		expected agentv1alpha1.MCPTransport
	}{
		{pb.MCPTransport_MCP_TRANSPORT_STREAMABLE_HTTP, agentv1alpha1.MCPTransportStreamableHTTP},
		{pb.MCPTransport_MCP_TRANSPORT_SSE, agentv1alpha1.MCPTransportSSE},
		{pb.MCPTransport_MCP_TRANSPORT_UNSPECIFIED, agentv1alpha1.MCPTransport("")},
	}

	for _, tt := range tests {
		t.Run(tt.input.String(), func(t *testing.T) {
			result := MCPTransportFromProto(tt.input)
			if result != tt.expected {
				t.Fatalf("MCPTransportFromProto(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMCPTransportRoundTrip(t *testing.T) {
	transports := []agentv1alpha1.MCPTransport{
		agentv1alpha1.MCPTransportStreamableHTTP,
		agentv1alpha1.MCPTransportSSE,
	}

	for _, tr := range transports {
		t.Run(string(tr), func(t *testing.T) {
			proto := MCPTransportToProto(tr)
			roundTrip := MCPTransportFromProto(proto)
			if roundTrip != tr {
				t.Fatalf("round-trip failed: %q -> %v -> %q", tr, proto, roundTrip)
			}
		})
	}
}
