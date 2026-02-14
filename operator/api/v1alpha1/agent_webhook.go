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
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-agent,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=agents,verbs=create;update,versions=v1alpha1,name=vagent-v1alpha1.kb.io,admissionReviewVersions=v1

// AgentCustomValidator validates Agent resources.
type AgentCustomValidator struct{}

var _ webhook.CustomValidator = &AgentCustomValidator{}

func SetupAgentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&Agent{}).
		WithValidator(&AgentCustomValidator{}).
		Complete()
}

func (v *AgentCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	agent, ok := obj.(*Agent)
	if !ok {
		return nil, fmt.Errorf("expected an Agent but got %T", obj)
	}
	warnings := collectAgentWarnings(agent)
	return warnings, validateAgent(agent)
}

func (v *AgentCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	agent, ok := newObj.(*Agent)
	if !ok {
		return nil, fmt.Errorf("expected an Agent but got %T", newObj)
	}
	warnings := collectAgentWarnings(agent)
	return warnings, validateAgent(agent)
}

func (v *AgentCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func collectAgentWarnings(agent *Agent) admission.Warnings {
	var warnings admission.Warnings

	// A3: Warn if type=standard but template is also set
	if agent.Spec.Runtime.Type == RuntimeTypeStandard && agent.Spec.Runtime.Template != nil {
		warnings = append(warnings,
			"spec.runtime.template is set but will be ignored because runtime type is \"standard\"")
	}

	// A4: Warn if type=template but standard is also set
	if agent.Spec.Runtime.Type == RuntimeTypeTemplate && agent.Spec.Runtime.Standard != nil {
		warnings = append(warnings,
			"spec.runtime.standard is set but will be ignored because runtime type is \"template\"")
	}

	return warnings
}

func validateAgent(agent *Agent) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// A1: type=standard requires standard config
	runtimePath := specPath.Child("runtime")
	if agent.Spec.Runtime.Type == RuntimeTypeStandard && agent.Spec.Runtime.Standard == nil {
		allErrs = append(allErrs, field.Required(
			runtimePath.Child("standard"),
			"standard configuration is required when runtime type is \"standard\"",
		))
	}

	// A2: type=template requires template config
	if agent.Spec.Runtime.Type == RuntimeTypeTemplate && agent.Spec.Runtime.Template == nil {
		allErrs = append(allErrs, field.Required(
			runtimePath.Child("template"),
			"template configuration is required when runtime type is \"template\"",
		))
	}

	// A5: Instruction - exactly one of template or instructionRef (if set)
	if agent.Spec.Instruction != nil {
		instrPath := specPath.Child("instruction")
		hasTemplate := agent.Spec.Instruction.Template != ""
		hasRef := agent.Spec.Instruction.InstructionRef != nil
		if !hasTemplate && !hasRef {
			allErrs = append(allErrs, field.Required(instrPath,
				"exactly one of template or instructionRef must be specified"))
		}
		if hasTemplate && hasRef {
			allErrs = append(allErrs, field.Forbidden(instrPath,
				"template and instructionRef are mutually exclusive"))
		}
	}

	// A6/A7: Tools
	for i, tool := range agent.Spec.Tools {
		toolPath := specPath.Child("tools").Index(i)
		hasTemplate := tool.Template != nil
		hasRef := tool.ToolRef != nil

		// A6: Exactly one of template or toolRef
		if !hasTemplate && !hasRef {
			allErrs = append(allErrs, field.Required(toolPath,
				"exactly one of template or toolRef must be specified"))
		}
		if hasTemplate && hasRef {
			allErrs = append(allErrs, field.Forbidden(toolPath,
				"template and toolRef are mutually exclusive"))
		}

		// A7: Name required when template is set
		if hasTemplate && tool.Name == "" {
			allErrs = append(allErrs, field.Required(
				toolPath.Child("name"),
				"name is required when template is specified",
			))
		}
	}

	// A8: Unique skill IDs
	seenSkills := make(map[string]bool)
	for i, skill := range agent.Spec.CardOverride.Skills {
		if seenSkills[skill.ID] {
			allErrs = append(allErrs, field.Duplicate(
				specPath.Child("card", "skills").Index(i).Child("id"), skill.ID))
		}
		seenSkills[skill.ID] = true
	}

	return aggregateErrors("Agent", agent.Name, allErrs)
}
