package tool

import (
	"fmt"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ValidateSpec validates the AgentTool spec (non-I/O parts).
// ConfigMap reference validation requires I/O and is not included here.
func ValidateSpec(agentTool *agentv1alpha1.AgentTool) error {
	if agentTool.Spec.OpenApi == nil {
		return fmt.Errorf("openApi is required when type is %q", agentTool.Spec.Type)
	}

	// Validate that exactly one of URL or ServiceRef is specified
	openApi := agentTool.Spec.OpenApi
	if openApi.URL == "" && openApi.ServiceRef == nil {
		return fmt.Errorf("either url or serviceRef must be specified")
	}
	if openApi.URL != "" && openApi.ServiceRef != nil {
		return fmt.Errorf("url and serviceRef are mutually exclusive")
	}

	// Validate the OpenAPI schema source
	schema := &openApi.OpenApiSchema
	sources := 0
	if schema.Value != nil && schema.Value.Raw != nil {
		sources++
	}
	if schema.ValueFrom != nil {
		sources++
	}
	if schema.EndpointPath != "" {
		sources++
	}

	if sources == 0 {
		return fmt.Errorf("openApiSchema is required: exactly one of value, valueFrom, or endpointPath must be specified")
	}
	if sources > 1 {
		return fmt.Errorf("openApiSchema: only one of value, valueFrom, or endpointPath may be specified")
	}

	return nil
}
