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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-model,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=models,verbs=create;update,versions=v1alpha1,name=vmodel-v1alpha1.kb.io,admissionReviewVersions=v1

// ModelCustomValidator validates Model resources.
// Reader is used for cross-resource reference validation (fixes #94).
//
// +kubebuilder:object:generate=false
type ModelCustomValidator struct {
	Reader client.Reader
}

var _ webhook.CustomValidator = &ModelCustomValidator{}

func SetupModelWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&Model{}).
		WithValidator(&ModelCustomValidator{Reader: mgr.GetClient()}).
		Complete()
}

func (v *ModelCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	model, ok := obj.(*Model)
	if !ok {
		return nil, fmt.Errorf("expected a Model but got %T", obj)
	}
	warnings := v.validateReferences(ctx, model)
	return warnings, validateModel(model)
}

func (v *ModelCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	model, ok := newObj.(*Model)
	if !ok {
		return nil, fmt.Errorf("expected a Model but got %T", newObj)
	}
	warnings := v.validateReferences(ctx, model)
	return warnings, validateModel(model)
}

func (v *ModelCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateModel(model *Model) error {
	var allErrs field.ErrorList

	if model.Spec.Parameters != nil {
		paramsPath := field.NewPath("spec", "parameters")

		// M1: At most one provider-specific parameter block
		if err := validateAtMostOneOf(
			paramsPath,
			[]string{"openai", "anthropic", "google", "bedrock"},
			[]bool{
				model.Spec.Parameters.OpenAI != nil,
				model.Spec.Parameters.Anthropic != nil,
				model.Spec.Parameters.Google != nil,
				model.Spec.Parameters.Bedrock != nil,
			},
		); err != nil {
			allErrs = append(allErrs, err)
		}

		// M2/M3: Anthropic thinking validation
		if model.Spec.Parameters.Anthropic != nil && model.Spec.Parameters.Anthropic.Thinking != nil {
			thinking := model.Spec.Parameters.Anthropic.Thinking
			thinkingPath := paramsPath.Child("anthropic", "thinking")

			// M2: type=enabled requires budgetTokens
			if thinking.Type == ThinkingTypeEnabled && thinking.BudgetTokens == nil {
				allErrs = append(allErrs, field.Required(
					thinkingPath.Child("budgetTokens"),
					"budgetTokens is required when thinking type is \"enabled\"",
				))
			}

			// M3: budgetTokens must be < maxTokens (if both set)
			if thinking.BudgetTokens != nil && model.Spec.Parameters.MaxTokens != nil {
				if *thinking.BudgetTokens >= *model.Spec.Parameters.MaxTokens {
					allErrs = append(allErrs, field.Invalid(
						thinkingPath.Child("budgetTokens"),
						*thinking.BudgetTokens,
						fmt.Sprintf("budgetTokens (%d) must be less than maxTokens (%d)",
							*thinking.BudgetTokens, *model.Spec.Parameters.MaxTokens),
					))
				}
			}
		}
	}

	return aggregateErrors("Model", model.Name, allErrs)
}

// validateReferences checks that cross-resource references exist.
// These are returned as warnings (not errors) to avoid ordering issues (fixes #94).
func (v *ModelCustomValidator) validateReferences(ctx context.Context, model *Model) admission.Warnings {
	if v.Reader == nil {
		return nil
	}

	var warnings admission.Warnings

	// Check ModelProvider reference
	ns := model.Spec.ProviderRef.Namespace
	if ns == "" {
		ns = model.Namespace
	}
	provider := &ModelProvider{}
	if err := v.Reader.Get(ctx, types.NamespacedName{Name: model.Spec.ProviderRef.Name, Namespace: ns}, provider); err != nil {
		warnings = append(warnings,
			fmt.Sprintf("referenced ModelProvider %s/%s not found — the model will not become ready until it exists", ns, model.Spec.ProviderRef.Name))
	}

	return warnings
}
