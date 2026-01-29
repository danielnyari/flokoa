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

// AgentToolType represents the type of tool.
// +kubebuilder:validation:Enum=http-api
type AgentToolType string

const (
	// AgentToolTypeHTTPAPI represents a tool that calls an HTTP API.
	AgentToolTypeHTTPAPI AgentToolType = "http-api"
)

// HTTPMethod represents HTTP methods for API calls.
// +kubebuilder:validation:Enum=GET;POST;PUT;PATCH;DELETE
type HTTPMethod string

const (
	// HTTPMethodGet represents the HTTP GET method.
	HTTPMethodGet HTTPMethod = "GET"
	// HTTPMethodPost represents the HTTP POST method.
	HTTPMethodPost HTTPMethod = "POST"
	// HTTPMethodPut represents the HTTP PUT method.
	HTTPMethodPut HTTPMethod = "PUT"
	// HTTPMethodPatch represents the HTTP PATCH method.
	HTTPMethodPatch HTTPMethod = "PATCH"
	// HTTPMethodDelete represents the HTTP DELETE method.
	HTTPMethodDelete HTTPMethod = "DELETE"
)

// AgentToolSpec defines the desired state of AgentTool.
type AgentToolSpec struct {
	// Type specifies the kind of tool (e.g., http-api).
	// +kubebuilder:validation:Required
	Type AgentToolType `json:"type"`

	// Description is a human-readable description for the LLM to understand the tool's purpose.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Description string `json:"description"`

	// HTTPApi contains HTTP API-specific configuration.
	// Required when Type is "http-api".
	// +optional
	HTTPApi *HTTPApiSpec `json:"httpApi,omitempty"`

	// InputSchema is a JSON Schema defining the input the agent provides to the tool.
	// For GET requests: becomes query parameters.
	// For POST/PUT/PATCH requests: becomes the JSON body.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	InputSchema *runtime.RawExtension `json:"inputSchema,omitempty"`

	// OutputSchema is a JSON Schema defining what the tool returns.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	OutputSchema *runtime.RawExtension `json:"outputSchema,omitempty"`

	// OpenApiSchemaRef references an OpenAPI spec as an alternative to InputSchema/OutputSchema.
	// +optional
	OpenApiSchemaRef *OpenApiSchemaRef `json:"openApiSchemaRef,omitempty"`
}

// HTTPApiSpec defines configuration for HTTP API tools.
// Either URL or ServiceRef must be specified to define the target endpoint.
type HTTPApiSpec struct {
	// URL is the external API endpoint to call.
	// Mutually exclusive with ServiceRef.
	// +optional
	URL string `json:"url,omitempty"`

	// ServiceRef references a Kubernetes service as the target endpoint.
	// Mutually exclusive with URL.
	// +optional
	ServiceRef *ServiceRef `json:"serviceRef,omitempty"`

	// Path is appended to the URL or service endpoint.
	// +optional
	Path string `json:"path,omitempty"`

	// Method is the HTTP method to use for the request.
	// +kubebuilder:validation:Required
	Method HTTPMethod `json:"method"`

	// TimeoutSeconds is the request timeout in seconds.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// Headers are additional HTTP headers to include in the request.
	// +optional
	Headers map[string]string `json:"headers,omitempty"`
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

// OpenApiSchemaRef references an OpenAPI specification.
// Either URL or ConfigMapRef must be specified.
type OpenApiSchemaRef struct {
	// URL is the endpoint to fetch the OpenAPI spec from.
	// Mutually exclusive with ConfigMapRef.
	// +optional
	URL string `json:"url,omitempty"`

	// ConfigMapRef references a ConfigMap containing the OpenAPI spec.
	// Mutually exclusive with URL.
	// +optional
	ConfigMapRef *ConfigMapKeyRef `json:"configMapRef,omitempty"`
}

// ConfigMapKeyRef references a key in a ConfigMap.
type ConfigMapKeyRef struct {
	// Name is the ConfigMap name.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the key in the ConfigMap containing the data.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
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
