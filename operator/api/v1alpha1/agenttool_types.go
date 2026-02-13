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
	"k8s.io/apimachinery/pkg/runtime"
)

// AgentToolType represents the type of tool.
// +kubebuilder:validation:Enum=openapi
type AgentToolType string

const (
	// AgentToolTypeOpenAPI represents a tool backed by an OpenAPI specification.
	AgentToolTypeOpenAPI AgentToolType = "openapi"
)

// AgentToolSpec defines the desired state of AgentTool.
type AgentToolSpec struct {
	// Type specifies the kind of tool (e.g., openapi).
	// +kubebuilder:validation:Required
	Type AgentToolType `json:"type"`

	// Description is a human-readable description for the LLM to understand the tool's purpose.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Description string `json:"description"`

	// OpenApi contains OpenAPI tool configuration.
	// Required when Type is "openapi".
	// +optional
	OpenApi *OpenApiToolSpec `json:"openApi,omitempty"`
}

// OpenApiToolSpec defines configuration for OpenAPI-based tools.
// Either URL or ServiceRef must be specified to define the target API.
// The OpenApiSchema field is required and defines where the OpenAPI specification is sourced from.
type OpenApiToolSpec struct {
	// URL is the base URL of the external API.
	// Mutually exclusive with ServiceRef.
	// +optional
	URL string `json:"url,omitempty"`

	// ServiceRef references a Kubernetes service as the target API.
	// Mutually exclusive with URL.
	// +optional
	ServiceRef *ServiceRef `json:"serviceRef,omitempty"`

	// OpenApiSchema specifies the source of the OpenAPI specification.
	// Exactly one of Value, ValueFrom, or EndpointPath must be specified.
	// +kubebuilder:validation:Required
	OpenApiSchema OpenApiSchema `json:"openApiSchema"`

	// TimeoutSeconds is the request timeout in seconds for API calls.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// Headers are additional HTTP headers to include in API requests.
	// +optional
	Headers map[string]string `json:"headers,omitempty"`
}

// OpenApiSchema defines where the OpenAPI specification is sourced from.
// Exactly one of Value, ValueFrom, or EndpointPath must be specified.
type OpenApiSchema struct {
	// Value contains the OpenAPI specification inline.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	Value *runtime.RawExtension `json:"value,omitempty"`

	// ValueFrom references a ConfigMap containing the OpenAPI specification.
	// +optional
	ValueFrom *corev1.ConfigMapKeySelector `json:"valueFrom,omitempty"`

	// EndpointPath is a path on the target service/URL where the OpenAPI spec is served
	// (e.g., "/openapi.json", "/docs/openapi.json", "/swagger.json").
	// The operator or runtime will fetch the spec from this path on the target.
	// +optional
	EndpointPath string `json:"endpointPath,omitempty"`
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
