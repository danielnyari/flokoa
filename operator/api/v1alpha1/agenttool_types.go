/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentToolType represents the type of tool.
// +kubebuilder:validation:Enum=mcp;openapi
type AgentToolType string

const (
	// AgentToolTypeMCP represents a declarative MCP endpoint.
	AgentToolTypeMCP AgentToolType = "mcp"
	// AgentToolTypeOpenAPI is retired: front REST APIs with an MCP adapter or
	// a capability instead. The admission webhook rejects it with a migration
	// pointer; the value remains in the enum only so users get that message
	// rather than a bare schema error.
	AgentToolTypeOpenAPI AgentToolType = "openapi"
)

// MCPTransport selects the MCP transport protocol.
// +kubebuilder:validation:Enum=streamableHTTP;sse
type MCPTransport string

const (
	// MCPTransportStreamableHTTP is the default Streamable HTTP transport.
	MCPTransportStreamableHTTP MCPTransport = "streamableHTTP"
	// MCPTransportSSE is the legacy Server-Sent Events transport.
	MCPTransportSSE MCPTransport = "sse"
)

// AgentToolSpec defines the desired state of AgentTool: a declarative MCP
// endpoint — the cluster-resource references a raw AgentSpec cannot express.
// It compiles to an MCP capability entry in the resolved spec, with
// ${secret:…} placeholders for header secrets.
type AgentToolSpec struct {
	// Type specifies the kind of tool.
	// +kubebuilder:default=mcp
	// +optional
	Type AgentToolType `json:"type,omitempty"`

	// Description is a human-readable description of the MCP server,
	// surfaced to the model where supported.
	// +optional
	Description string `json:"description,omitempty"`

	// URL is the full URL of the MCP server. Mutually exclusive with ServiceRef.
	// +optional
	URL string `json:"url,omitempty"`

	// ServiceRef references an in-cluster Service serving MCP.
	// Mutually exclusive with URL.
	// +optional
	ServiceRef *ServiceRef `json:"serviceRef,omitempty"`

	// Path is the HTTP path of the MCP endpoint when using ServiceRef.
	// Defaults to "/mcp" (streamableHTTP) or "/sse" (sse).
	// +optional
	Path string `json:"path,omitempty"`

	// Transport selects the MCP transport protocol.
	// +kubebuilder:default=streamableHTTP
	// +optional
	Transport MCPTransport `json:"transport,omitempty"`

	// Headers are additional HTTP headers to send to the MCP server.
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// HeaderSecrets populate headers from Secrets. Values are delivered as
	// ${secret:…} placeholders resolved in the runner — never written into
	// the compiled spec.
	// +optional
	HeaderSecrets []SecretHeader `json:"headerSecrets,omitempty"`

	// ToolPrefix prefixes every tool name from this server (e.g. "petstore"
	// turns "search" into "petstore_search"), avoiding collisions between
	// servers.
	// +optional
	ToolPrefix string `json:"toolPrefix,omitempty"`

	// AllowedTools filters the server's tools to this list.
	// +optional
	AllowedTools []string `json:"allowedTools,omitempty"`

	// TimeoutSeconds is the request timeout in seconds for MCP calls.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// SecretHeader sources an HTTP header value from a Secret key.
type SecretHeader struct {
	// Name is the HTTP header name (e.g. "Authorization").
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// SecretRef selects the Secret key holding the header value.
	SecretRef corev1.SecretKeySelector `json:"secretRef"`
}

// ServiceRef references a Kubernetes service.
type ServiceRef struct {
	// Name is the service name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace is the service namespace. Defaults to the AgentTool's namespace.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Port is the service port number.
	// Mutually exclusive with PortName.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port *int32 `json:"port,omitempty"`

	// PortName is the service port name.
	// Mutually exclusive with Port.
	// +optional
	PortName string `json:"portName,omitempty"`
}

// AgentToolStatus defines the observed state of AgentTool.
type AgentToolStatus struct {
	// Conditions represent the latest available observations of the tool's state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
// +kubebuilder:printcolumn:name="Transport",type="string",JSONPath=".spec.transport"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// AgentTool is the Schema for the agenttools API
type AgentTool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentToolSpec   `json:"spec,omitempty"`
	Status AgentToolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentToolList contains a list of AgentTool
type AgentToolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AgentTool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AgentTool{}, &AgentToolList{})
}
