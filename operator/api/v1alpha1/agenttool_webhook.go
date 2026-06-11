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
	"strings"

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
	return collectAgentToolWarnings(tool), validateAgentTool(tool)
}

func (v *AgentToolCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	tool, ok := newObj.(*AgentTool)
	if !ok {
		return nil, fmt.Errorf("expected an AgentTool but got %T", newObj)
	}
	return collectAgentToolWarnings(tool), validateAgentTool(tool)
}

func (v *AgentToolCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func collectAgentToolWarnings(tool *AgentTool) admission.Warnings {
	var warnings admission.Warnings
	// SSE servers conventionally serve under /sse; a mismatched explicit path
	// usually means the transport field and the endpoint disagree.
	if tool.Spec.Transport == MCPTransportSSE && tool.Spec.URL != "" && !strings.HasSuffix(strings.TrimSuffix(tool.Spec.URL, "/"), "/sse") {
		warnings = append(warnings,
			"transport is \"sse\" but the URL does not end in /sse; the MCP client infers the transport from the URL")
	}
	return warnings
}

func validateAgentTool(tool *AgentTool) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	spec := &tool.Spec

	// The OpenAPI tool type is retired with the v2.1 pivot.
	if spec.Type == AgentToolTypeOpenAPI {
		allErrs = append(allErrs, field.Forbidden(
			specPath.Child("type"),
			"the openapi tool type is retired: front REST APIs with an MCP adapter or a Capability instead "+
				"(see docs/agenttool.md for the migration path)",
		))
		return aggregateErrors("AgentTool", tool.Name, allErrs)
	}

	// Exactly one of url or serviceRef.
	if err := validateExactlyOneOf(
		specPath,
		[]string{"url", "serviceRef"},
		[]bool{spec.URL != "", spec.ServiceRef != nil},
	); err != nil {
		allErrs = append(allErrs, err)
	}

	// URL must be a valid HTTP/HTTPS URL (prevents SSRF via file://, gopher://, etc.)
	if spec.URL != "" {
		if err := validateHTTPURL(specPath.Child("url"), spec.URL); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	// Path only applies to serviceRef-based endpoints and must be absolute.
	if spec.Path != "" {
		if spec.ServiceRef == nil {
			allErrs = append(allErrs, field.Forbidden(specPath.Child("path"),
				"path applies only with serviceRef; put the path in the url instead"))
		} else if !strings.HasPrefix(spec.Path, "/") {
			allErrs = append(allErrs, field.Invalid(specPath.Child("path"), spec.Path,
				"path must start with /"))
		}
	}

	// ServiceRef port exclusivity.
	if spec.ServiceRef != nil {
		if err := validateExactlyOneOf(
			specPath.Child("serviceRef"),
			[]string{"port", "portName"},
			[]bool{spec.ServiceRef.Port != nil, spec.ServiceRef.PortName != ""},
		); err != nil {
			allErrs = append(allErrs, err)
		}
	}

	// Header secrets must not collide with static headers or each other.
	seenHeaders := map[string]string{}
	for name := range spec.Headers {
		seenHeaders[strings.ToLower(name)] = "headers"
	}
	for i, hs := range spec.HeaderSecrets {
		key := strings.ToLower(hs.Name)
		if prev, dup := seenHeaders[key]; dup {
			allErrs = append(allErrs, field.Duplicate(
				specPath.Child("headerSecrets").Index(i).Child("name"),
				fmt.Sprintf("%s (already set via %s)", hs.Name, prev)))
		}
		seenHeaders[key] = "headerSecrets"
	}

	return aggregateErrors("AgentTool", tool.Name, allErrs)
}
