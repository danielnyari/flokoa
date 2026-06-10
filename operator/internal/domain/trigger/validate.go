package trigger

import (
	"fmt"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ValidateSpec validates the AgentTrigger spec (non-I/O parts).
func ValidateSpec(trigger *agentv1alpha1.AgentTrigger) error {
	spec := &trigger.Spec

	// EventSource is required
	if spec.EventSource.Name == "" {
		return fmt.Errorf("eventSource.name is required")
	}
	if spec.EventSource.EventName == "" {
		return fmt.Errorf("eventSource.eventName is required")
	}

	// Agent is required
	if spec.Agent.Name == "" {
		return fmt.Errorf("agent.name is required")
	}

	// PushNotification: exactly one of agentRef or url must be set (if pushNotification is specified)
	if spec.PushNotification != nil {
		hasAgentRef := spec.PushNotification.AgentRef != nil
		hasURL := spec.PushNotification.URL != ""

		if !hasAgentRef && !hasURL {
			return fmt.Errorf("pushNotification: exactly one of agentRef or url must be specified")
		}
		if hasAgentRef && hasURL {
			return fmt.Errorf("pushNotification: agentRef and url are mutually exclusive")
		}

		if hasAgentRef && spec.PushNotification.AgentRef.Name == "" {
			return fmt.Errorf("pushNotification.agentRef.name is required")
		}
	}

	// Validate data filters
	if spec.Filter != nil {
		for i, df := range spec.Filter.Data {
			if df.Path == "" {
				return fmt.Errorf("filter.data[%d].path is required", i)
			}
			if len(df.Value) == 0 {
				return fmt.Errorf("filter.data[%d].value must have at least one entry", i)
			}
		}
		for i, ef := range spec.Filter.Exprs {
			if ef.Expr == "" {
				return fmt.Errorf("filter.exprs[%d].expr is required", i)
			}
		}
	}

	return nil
}
