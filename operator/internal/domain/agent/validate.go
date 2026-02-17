package agent

import (
	"fmt"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ValidateSpec checks the Agent spec for consistency.
// This is a pure validation function with no I/O.
func ValidateSpec(agent *agentv1alpha1.Agent) error {
	// Validate instruction entry if present
	if agent.Spec.Instruction != nil {
		if agent.Spec.Instruction.Template != "" && agent.Spec.Instruction.InstructionRef != nil {
			return fmt.Errorf("instruction.inline and instruction.instructionRef are mutually exclusive")
		}
		if agent.Spec.Instruction.Template == "" && agent.Spec.Instruction.InstructionRef == nil {
			return fmt.Errorf("instruction must have either inline or instructionRef set")
		}
	}

	switch agent.Spec.Runtime.Type {
	case agentv1alpha1.RuntimeTypeStandard:
		if agent.Spec.Runtime.Template != nil {
			return fmt.Errorf("runtime.managed must not be set when runtime.type is %q", agentv1alpha1.RuntimeTypeStandard)
		}
	case agentv1alpha1.RuntimeTypeTemplate:
		if agent.Spec.Runtime.Standard != nil {
			return fmt.Errorf("runtime.standard must not be set when runtime.type is %q", agentv1alpha1.RuntimeTypeTemplate)
		}
		if agent.Spec.Runtime.Template == nil {
			return fmt.Errorf("runtime.managed is required when runtime.type is %q", agentv1alpha1.RuntimeTypeTemplate)
		}
		if agent.Spec.Model == nil {
			return fmt.Errorf("spec.model is required when runtime.type is %q", agentv1alpha1.RuntimeTypeTemplate)
		}
		if agent.Spec.Instruction == nil {
			return fmt.Errorf("spec.instruction is required when runtime.type is %q", agentv1alpha1.RuntimeTypeTemplate)
		}
	default:
		return fmt.Errorf("unsupported runtime type: %q", agent.Spec.Runtime.Type)
	}
	return nil
}
