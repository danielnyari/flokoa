package agent

import (
	"fmt"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ValidateSpec checks the Agent spec for consistency before compilation.
// This is a pure validation function with no I/O; admission performs the same
// checks with field paths, this guards direct API writes that bypassed the
// webhook.
func ValidateSpec(agent *agentv1alpha1.Agent) error {
	if agent.Spec.Runtime.Isolation == agentv1alpha1.IsolationSession {
		return fmt.Errorf("runtime.isolation %q is not available yet (roadmap P1)", agentv1alpha1.IsolationSession)
	}

	// An agent needs a model from somewhere: the fragment or a Model ref.
	hasInlineModel := agent.Spec.Spec != nil && agent.Spec.Spec.Model != ""
	if !hasInlineModel && agent.Spec.ModelRef == nil {
		return fmt.Errorf("a model is required: set spec.modelRef or spec.spec.model")
	}

	return nil
}
