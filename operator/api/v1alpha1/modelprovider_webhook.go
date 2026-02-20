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

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-modelprovider,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=modelproviders,verbs=create;update,versions=v1alpha1,name=vmodelprovider-v1alpha1.kb.io,admissionReviewVersions=v1

// ModelProviderCustomValidator validates ModelProvider resources.
type ModelProviderCustomValidator struct{}

var _ webhook.CustomValidator = &ModelProviderCustomValidator{}

func SetupModelProviderWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&ModelProvider{}).
		WithValidator(&ModelProviderCustomValidator{}).
		Complete()
}

func (v *ModelProviderCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	mp, ok := obj.(*ModelProvider)
	if !ok {
		return nil, fmt.Errorf("expected a ModelProvider but got %T", obj)
	}
	return nil, validateModelProvider(mp)
}

func (v *ModelProviderCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	mp, ok := newObj.(*ModelProvider)
	if !ok {
		return nil, fmt.Errorf("expected a ModelProvider but got %T", newObj)
	}
	return nil, validateModelProvider(mp)
}

func (v *ModelProviderCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateModelProvider(mp *ModelProvider) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// P1: Exactly one provider block must be set
	if err := validateExactlyOneOf(
		specPath,
		[]string{"openai", "anthropic", "google", "bedrock"},
		[]bool{
			mp.Spec.OpenAI != nil,
			mp.Spec.Anthropic != nil,
			mp.Spec.Google != nil,
			mp.Spec.Bedrock != nil,
		},
	); err != nil {
		allErrs = append(allErrs, err)
	}

	// P2: Validate BaseURL fields use HTTP/HTTPS (prevents SSRF via file://, gopher://, etc.)
	if mp.Spec.OpenAI != nil && mp.Spec.OpenAI.BaseURL != "" {
		if err := validateHTTPURL(specPath.Child("openai", "baseURL"), mp.Spec.OpenAI.BaseURL); err != nil {
			allErrs = append(allErrs, err)
		}
	}
	if mp.Spec.Anthropic != nil && mp.Spec.Anthropic.BaseURL != "" {
		if err := validateHTTPURL(specPath.Child("anthropic", "baseURL"), mp.Spec.Anthropic.BaseURL); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	return aggregateErrors("ModelProvider", mp.Name, allErrs)
}
