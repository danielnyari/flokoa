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
	"encoding/json"
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

	if model.Spec.Settings != nil && model.Spec.Settings.Extra != nil {
		extraPath := field.NewPath("spec", "settings", "extra")

		// Extra must be a JSON object: its keys merge into the compiled
		// model_settings, and typed fields win conflicts.
		var extra map[string]json.RawMessage
		if err := json.Unmarshal(model.Spec.Settings.Extra.Raw, &extra); err != nil {
			allErrs = append(allErrs, field.Invalid(
				extraPath, string(model.Spec.Settings.Extra.Raw),
				"extra must be a JSON object of model settings"))
		} else {
			for key := range extra {
				if typedSettingsKeys[key] {
					allErrs = append(allErrs, field.Forbidden(
						extraPath.Key(key),
						fmt.Sprintf("%q has a typed field on spec.settings; set it there", key)))
				}
			}
		}
	}

	return aggregateErrors("Model", model.Name, allErrs)
}

// typedSettingsKeys are the compiled model_settings keys owned by typed
// ModelSettings fields. Entries in settings.extra may not shadow them.
var typedSettingsKeys = map[string]bool{
	"max_tokens":          true,
	"temperature":         true,
	"top_p":               true,
	"top_k":               true,
	"timeout":             true,
	"parallel_tool_calls": true,
	"seed":                true,
	"presence_penalty":    true,
	"frequency_penalty":   true,
	"logit_bias":          true,
	"stop_sequences":      true,
	"extra_headers":       true,
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
