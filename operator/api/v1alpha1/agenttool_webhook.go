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

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-agenttool,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=agenttools,verbs=create;update,versions=v1alpha1,name=vagenttool-v1alpha1.kb.io,admissionReviewVersions=v1

// AgentToolCustomValidator validates AgentTool resources.
type AgentToolCustomValidator struct{}

var _ webhook.CustomValidator = &AgentToolCustomValidator{}

func SetupAgentToolWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&AgentTool{}).
		WithValidator(&AgentToolCustomValidator{}).
		Complete()
}

func (v *AgentToolCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	tool, ok := obj.(*AgentTool)
	if !ok {
		return nil, fmt.Errorf("expected an AgentTool but got %T", obj)
	}
	return nil, validateAgentTool(tool)
}

func (v *AgentToolCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	tool, ok := newObj.(*AgentTool)
	if !ok {
		return nil, fmt.Errorf("expected an AgentTool but got %T", newObj)
	}
	return nil, validateAgentTool(tool)
}

func (v *AgentToolCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateAgentTool(tool *AgentTool) error {
	return validateAgentToolSpec(&tool.Spec, field.NewPath("spec"), "AgentTool", tool.Name)
}

// validateAgentToolSpec validates the AgentToolSpec. Extracted so it can also
// be used for inline tool templates in Agent validation.
func validateAgentToolSpec(spec *AgentToolSpec, specPath *field.Path, kind, name string) error {
	var allErrs field.ErrorList

	// T1: When type=openapi, openApi must be set
	if spec.Type == AgentToolTypeOpenAPI {
		if spec.OpenApi == nil {
			allErrs = append(allErrs, field.Required(
				specPath.Child("openApi"),
				"openApi configuration is required when type is \"openapi\"",
			))
		} else {
			allErrs = append(allErrs, validateOpenApiToolSpec(spec.OpenApi, specPath.Child("openApi"))...)
		}
	}

	return aggregateErrors(kind, name, allErrs)
}

func validateOpenApiToolSpec(spec *OpenApiToolSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// T2: Exactly one of url or serviceRef
	if err := validateExactlyOneOf(
		fldPath,
		[]string{"url", "serviceRef"},
		[]bool{spec.URL != "", spec.ServiceRef != nil},
	); err != nil {
		allErrs = append(allErrs, err)
	}

	// T5: URL must be a valid HTTP/HTTPS URL (prevents SSRF via file://, gopher://, etc.)
	if spec.URL != "" {
		if err := validateHTTPURL(fldPath.Child("url"), spec.URL); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	// T3: Exactly one of value, valueFrom, or endpointPath
	if err := validateExactlyOneOf(
		fldPath.Child("openApiSchema"),
		[]string{"value", "valueFrom", "endpointPath"},
		[]bool{
			spec.OpenApiSchema.Value != nil,
			spec.OpenApiSchema.ValueFrom != nil,
			spec.OpenApiSchema.EndpointPath != "",
		},
	); err != nil {
		allErrs = append(allErrs, err)
	}

	// T4: ServiceRef port exclusivity
	if spec.ServiceRef != nil {
		if err := validateExactlyOneOf(
			fldPath.Child("serviceRef"),
			[]string{"port", "portName"},
			[]bool{spec.ServiceRef.Port != nil, spec.ServiceRef.PortName != ""},
		); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	return allErrs
}
