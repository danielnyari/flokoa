package tool

import (
	"fmt"
	"strings"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ValidateSpec validates the AgentTool spec (non-I/O parts). The admission
// webhook performs the same checks with field paths; this guards direct API
// writes that bypassed it.
func ValidateSpec(agentTool *agentv1alpha1.AgentTool) error {
	spec := &agentTool.Spec

	if spec.Type == agentv1alpha1.AgentToolTypeOpenAPI {
		return fmt.Errorf("the openapi tool type is retired: front REST APIs with an MCP adapter or a Capability instead")
	}

	if spec.URL == "" && spec.ServiceRef == nil {
		return fmt.Errorf("either url or serviceRef must be specified")
	}
	if spec.URL != "" && spec.ServiceRef != nil {
		return fmt.Errorf("url and serviceRef are mutually exclusive")
	}

	if spec.Path != "" {
		if spec.ServiceRef == nil {
			return fmt.Errorf("path applies only with serviceRef; put the path in the url instead")
		}
		if !strings.HasPrefix(spec.Path, "/") {
			return fmt.Errorf("path must start with /")
		}
	}

	if spec.ServiceRef != nil && spec.ServiceRef.Port != nil && spec.ServiceRef.PortName != "" {
		return fmt.Errorf("serviceRef.port and serviceRef.portName are mutually exclusive")
	}

	return nil
}
