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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// AgentToolType represents the type of tool
// +kubebuilder:validation:Enum=http-api
type AgentToolType string

const (
	AgentToolTypeHTTPAPI AgentToolType = "http-api"
)

// HTTPMethod represents HTTP methods
// +kubebuilder:validation:Enum=GET;POST;PUT;PATCH;DELETE
type HTTPMethod string

const (
	HTTPMethodGet    HTTPMethod = "GET"
	HTTPMethodPost   HTTPMethod = "POST"
	HTTPMethodPut    HTTPMethod = "PUT"
	HTTPMethodPatch  HTTPMethod = "PATCH"
	HTTPMethodDelete HTTPMethod = "DELETE"
)

// AgentToolSpec defines the desired state of AgentTool
type AgentToolSpec struct {
	// Type of tool
	Type AgentToolType `json:"type"`

	// Human-readable description for the LLM
	Description string `json:"description"`

	// HTTP API specific configuration
	// +optional
	HTTPApi *HTTPApiSpec `json:"httpApi,omitempty"`

	// Input schema - JSON Schema defining what the agent provides
	// For GET: becomes query params, for POST/PUT/PATCH: becomes JSON body
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	InputSchema *runtime.RawExtension `json:"inputSchema,omitempty"`

	// Output schema - JSON Schema defining what the tool returns
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	OutputSchema *runtime.RawExtension `json:"outputSchema,omitempty"`

	// Reference to an OpenAPI spec (alternative to inputSchema/outputSchema)
	// +optional
	OpenApiSchemaRef *OpenApiSchemaRef `json:"openApiSchemaRef,omitempty"`
}

// HTTPApiSpec defines configuration for HTTP API tools
type HTTPApiSpec struct {
	// External API URL
	// +optional
	URL string `json:"url,omitempty"`

	// Reference to a Kubernetes service (alternative to URL)
	// +optional
	ServiceRef *ServiceRef `json:"serviceRef,omitempty"`

	// Path on the service (used with serviceRef, or appended to URL)
	// +optional
	Path string `json:"path,omitempty"`

	// HTTP method
	Method HTTPMethod `json:"method"`

	// Timeout in seconds
	// +kubebuilder:default=30
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// HTTP headers to include
	// +optional
	Headers map[string]string `json:"headers,omitempty"`
}

// ServiceRef references a Kubernetes service
type ServiceRef struct {
	// Service name
	Name string `json:"name"`

	// Namespace (defaults to AgentTool's namespace)
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Port number
	// +optional
	Port *int32 `json:"port,omitempty"`

	// Port name (alternative to port number)
	// +optional
	PortName string `json:"portName,omitempty"`
}

// OpenApiSchemaRef references an OpenAPI specification
type OpenApiSchemaRef struct {
	// URL to fetch the OpenAPI spec from
	// +optional
	URL string `json:"url,omitempty"`

	// Reference to a ConfigMap containing the OpenAPI spec
	// +optional
	ConfigMapRef *ConfigMapKeyRef `json:"configMapRef,omitempty"`
}

// ConfigMapKeyRef references a key in a ConfigMap
type ConfigMapKeyRef struct {
	// ConfigMap name
	Name string `json:"name"`

	// Key in the ConfigMap
	Key string `json:"key"`
}

// AgentToolStatus defines the observed state of AgentTool
type AgentToolStatus struct {
	// Standard conditions
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Observed generation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
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
