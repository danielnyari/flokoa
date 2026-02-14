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

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-instruction,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=instructions,verbs=create;update,versions=v1alpha1,name=vinstruction-v1alpha1.kb.io,admissionReviewVersions=v1

// InstructionCustomValidator validates Instruction resources.
type InstructionCustomValidator struct{}

var _ webhook.CustomValidator = &InstructionCustomValidator{}

func SetupInstructionWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&Instruction{}).
		WithValidator(&InstructionCustomValidator{}).
		Complete()
}

func (v *InstructionCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	instruction, ok := obj.(*Instruction)
	if !ok {
		return nil, fmt.Errorf("expected an Instruction but got %T", obj)
	}
	return nil, validateInstruction(instruction)
}

func (v *InstructionCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	instruction, ok := newObj.(*Instruction)
	if !ok {
		return nil, fmt.Errorf("expected an Instruction but got %T", newObj)
	}
	return nil, validateInstruction(instruction)
}

func (v *InstructionCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateInstruction(instruction *Instruction) error {
	var allErrs field.ErrorList

	if instruction.Spec.Content == "" {
		allErrs = append(allErrs, field.Required(
			field.NewPath("spec", "content"),
			"instruction content must not be empty",
		))
	}

	return aggregateErrors("Instruction", instruction.Name, allErrs)
}
