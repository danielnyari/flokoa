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

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-prompt,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=prompts,verbs=create;update,versions=v1alpha1,name=vprompt-v1alpha1.kb.io,admissionReviewVersions=v1

// PromptCustomValidator validates Prompt resources.
type PromptCustomValidator struct{}

var _ webhook.CustomValidator = &PromptCustomValidator{}

func SetupPromptWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&Prompt{}).
		WithValidator(&PromptCustomValidator{}).
		Complete()
}

func (v *PromptCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	prompt, ok := obj.(*Prompt)
	if !ok {
		return nil, fmt.Errorf("expected a Prompt but got %T", obj)
	}
	return nil, validatePrompt(prompt)
}

func (v *PromptCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	prompt, ok := newObj.(*Prompt)
	if !ok {
		return nil, fmt.Errorf("expected a Prompt but got %T", newObj)
	}
	return nil, validatePrompt(prompt)
}

func (v *PromptCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validatePrompt(prompt *Prompt) error {
	var allErrs field.ErrorList

	sourcePath := field.NewPath("spec", "source")

	// P1: Exactly one of value or valueFrom must be specified
	if err := validateExactlyOneOf(sourcePath,
		[]string{"value", "valueFrom"},
		[]bool{prompt.Spec.Source.Value != nil, prompt.Spec.Source.ValueFrom != nil},
	); err != nil {
		allErrs = append(allErrs, err)
	}

	// P2: If value is set, it must not be empty
	if prompt.Spec.Source.Value != nil && *prompt.Spec.Source.Value == "" {
		allErrs = append(allErrs, field.Required(
			sourcePath.Child("value"),
			"prompt template value must not be empty",
		))
	}

	// P3: If valueFrom is set, name and key must be specified
	if prompt.Spec.Source.ValueFrom != nil {
		vfPath := sourcePath.Child("valueFrom")
		if prompt.Spec.Source.ValueFrom.Name == "" {
			allErrs = append(allErrs, field.Required(
				vfPath.Child("name"),
				"configMap name must be specified",
			))
		}
		if prompt.Spec.Source.ValueFrom.Key == "" {
			allErrs = append(allErrs, field.Required(
				vfPath.Child("key"),
				"configMap key must be specified",
			))
		}
	}

	// P4: Variable names must be unique
	if len(prompt.Spec.Variables) > 0 {
		seen := make(map[string]bool)
		varsPath := field.NewPath("spec", "variables")
		for i, v := range prompt.Spec.Variables {
			if seen[v.Name] {
				allErrs = append(allErrs, field.Duplicate(
					varsPath.Index(i).Child("name"),
					v.Name,
				))
			}
			seen[v.Name] = true
		}
	}

	return aggregateErrors("Prompt", prompt.Name, allErrs)
}
