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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	capabilitydomain "github.com/danielnyari/flokoa/internal/domain/capability"
)

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-capability,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=capabilities,verbs=create;update,versions=v1alpha1,name=vcapability-v1alpha1.kb.io,admissionReviewVersions=v1

// CapabilityCustomValidator validates Capability resources: digest-pinned
// artifact, entrypoint format, schema-policy coherence, a compilable
// configSchema, parseable requires specifiers, and well-formed dependency
// pins (roadmap 08). The CRD schema enforces the syntactic patterns too;
// the webhook adds precise messages and the cross-field policy rules.
//
// +kubebuilder:object:generate=false
type CapabilityCustomValidator struct{}

var _ webhook.CustomValidator = &CapabilityCustomValidator{}

// SetupCapabilityWebhookWithManager registers the webhook for Capability in the manager.
func SetupCapabilityWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&agentv1alpha1.Capability{}).
		WithValidator(&CapabilityCustomValidator{}).
		Complete()
}

func (v *CapabilityCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	capCR, ok := obj.(*agentv1alpha1.Capability)
	if !ok {
		return nil, fmt.Errorf("expected a Capability but got %T", obj)
	}

	// Create-only (names are immutable): the builder derives container and
	// volume names from cap-<name>, which must be RFC 1123 DNS labels —
	// stricter than the DNS-subdomain rule object names get by default
	// (no dots, at most 63 characters).
	var nameErrs field.ErrorList
	if msgs := validation.IsDNS1123Label(capCR.Name); len(msgs) > 0 {
		nameErrs = append(nameErrs, field.Invalid(field.NewPath("metadata", "name"), capCR.Name, fmt.Sprintf(
			"Capability names must be valid DNS labels (lowercase alphanumerics and '-', at most 63 characters) because "+
				"runner pods derive container and volume names from cap-<name>: %s", strings.Join(msgs, "; "))))
	}

	warnings, allErrs := validateCapability(capCR)
	return warnings, aggregateFieldErrors("Capability", capCR.Name, append(nameErrs, allErrs...))
}

func (v *CapabilityCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	capCR, ok := newObj.(*agentv1alpha1.Capability)
	if !ok {
		return nil, fmt.Errorf("expected a Capability but got %T", newObj)
	}
	warnings, allErrs := validateCapability(capCR)
	return warnings, aggregateFieldErrors("Capability", capCR.Name, allErrs)
}

func (v *CapabilityCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

var (
	digestPinnedRe = regexp.MustCompile(`@sha256:[a-f0-9]{64}$`)
	entrypointRe   = regexp.MustCompile(`^[\w.]+:[A-Za-z_]\w*$`)
)

func validateCapability(capCR *agentv1alpha1.Capability) (admission.Warnings, field.ErrorList) {
	var warnings admission.Warnings
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	if !digestPinnedRe.MatchString(capCR.Spec.Artifact) {
		allErrs = append(allErrs, field.Invalid(specPath.Child("artifact"), capCR.Spec.Artifact,
			"artifact must be digest-pinned (…@sha256:<64 hex chars>); tags are not immutable"))
	}

	if !entrypointRe.MatchString(capCR.Spec.Entrypoint) {
		allErrs = append(allErrs, field.Invalid(specPath.Child("entrypoint"), capCR.Spec.Entrypoint,
			"entrypoint must be module:attr where attr is the capability class itself (a single identifier, no factories or aliases)"))
	}

	// The compiled-spec entry name (serializationName, else the entrypoint
	// class) may not claim a reserved platform name or carry path punctuation.
	entryName := capabilitydomain.EntryName(capCR.Spec.Entrypoint, capCR.Spec.SerializationName)
	if err := capabilitydomain.ValidateEntryName(entryName); err != nil {
		fld := specPath.Child("entrypoint")
		if capCR.Spec.SerializationName != "" {
			fld = specPath.Child("serializationName")
		}
		allErrs = append(allErrs, field.Invalid(fld, entryName, err.Error()))
	}

	// Schema-policy coherence: strict requires a published schema; permissive
	// is the loud opt-out (surfaced in status and at every Agent attachment).
	switch capCR.Spec.SchemaPolicy {
	case agentv1alpha1.SchemaPolicyPermissive:
		warnings = append(warnings, fmt.Sprintf(
			"Capability %s has schemaPolicy: permissive — attached agent config is not validated at admission", capCR.Name))
	default: // strict (the CRD default)
		if capCR.Spec.ConfigSchema == nil {
			allErrs = append(allErrs, field.Required(specPath.Child("configSchema"),
				"schemaPolicy strict requires a configSchema; publish one or set schemaPolicy: permissive (the loud opt-out)"))
		}
	}

	if capCR.Spec.ConfigSchema != nil {
		if err := capabilitydomain.CompileSchema(capCR.Spec.ConfigSchema.Raw); err != nil {
			allErrs = append(allErrs, field.Invalid(specPath.Child("configSchema"), string(capCR.Spec.ConfigSchema.Raw),
				err.Error()))
		}
	}

	req := capabilitydomain.Requires{
		Python:       capCR.Spec.Requires.Python,
		PydanticAI:   capCR.Spec.Requires.PydanticAI,
		FlokoaRunner: capCR.Spec.Requires.FlokoaRunner,
	}
	if err := capabilitydomain.ValidateRequires(req); err != nil {
		allErrs = append(allErrs, field.Invalid(specPath.Child("requires"), "", err.Error()))
	}

	for i, pin := range capCR.Spec.Dependencies {
		if _, _, err := capabilitydomain.ParsePin(pin); err != nil {
			allErrs = append(allErrs, field.Invalid(specPath.Child("dependencies").Index(i), pin, err.Error()))
		}
	}

	return warnings, allErrs
}
