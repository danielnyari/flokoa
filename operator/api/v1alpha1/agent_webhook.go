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
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-agent,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=agents,verbs=create;update,versions=v1alpha1,name=vagent-v1alpha1.kb.io,admissionReviewVersions=v1

// AgentCustomValidator validates Agent resources.
// Reader is used for cross-resource reference validation (fixes #94).
//
// +kubebuilder:object:generate=false
type AgentCustomValidator struct {
	Reader client.Reader
}

var _ webhook.CustomValidator = &AgentCustomValidator{}

func SetupAgentWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&Agent{}).
		WithValidator(&AgentCustomValidator{Reader: mgr.GetClient()}).
		Complete()
}

func (v *AgentCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	agent, ok := obj.(*Agent)
	if !ok {
		return nil, fmt.Errorf("expected an Agent but got %T", obj)
	}
	return v.validateReferences(ctx, agent), validateAgent(agent)
}

func (v *AgentCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	agent, ok := newObj.(*Agent)
	if !ok {
		return nil, fmt.Errorf("expected an Agent but got %T", newObj)
	}
	return v.validateReferences(ctx, agent), validateAgent(agent)
}

func (v *AgentCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// secretPlaceholderRe matches the runtime contract's placeholder grammar (§3).
var secretPlaceholderRe = regexp.MustCompile(`\$\{secret:([^}]*)\}`)

// validSecretRefName matches the NAME grammar of ${secret:NAME}.
var validSecretRefName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func validateAgent(agent *Agent) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// Runtime: the session isolation tier ships with the session router (P1).
	if agent.Spec.Runtime.Isolation == IsolationSession {
		allErrs = append(allErrs, field.Forbidden(
			specPath.Child("runtime", "isolation"),
			"the session isolation tier is not available yet (roadmap P1: session router + pools); use \"shared\"",
		))
	}

	// Capability CR attachments ship with the Capability CRD (roadmap 08).
	if len(agent.Spec.Capabilities) > 0 {
		allErrs = append(allErrs, field.Forbidden(
			specPath.Child("capabilities"),
			"Capability references are not available yet (roadmap 08); native capabilities go in spec.spec.capabilities",
		))
	}

	// Fragment capability names: baseline-native names only. Harness and
	// third-party capabilities ship exclusively through Capability CRs;
	// class/module paths are their signature. flokoa.platform/* names are
	// operator-injected and cannot be user-set.
	if agent.Spec.Spec != nil {
		fragPath := specPath.Child("spec")
		for i, cap := range agent.Spec.Spec.Capabilities {
			capPath := fragPath.Child("capabilities").Index(i).Child("name")
			if strings.ContainsAny(cap.Name, "./:") {
				allErrs = append(allErrs, field.Invalid(capPath, cap.Name,
					"only baseline-native capability names are allowed inline (e.g. WebSearch, MCP, Thinking); "+
						"harness and third-party capabilities ship as Capability resources",
				))
			}
		}

		// ${secret:NAME} placeholders must have matching secretRefs keys.
		for _, name := range fragmentSecretPlaceholders(agent.Spec.Spec) {
			if !validSecretRefName.MatchString(name) {
				allErrs = append(allErrs, field.Invalid(fragPath, fmt.Sprintf("${secret:%s}", name),
					"secret placeholder names must match [A-Za-z0-9._-]+"))
				continue
			}
			if _, ok := agent.Spec.SecretRefs[name]; !ok {
				allErrs = append(allErrs, field.Invalid(fragPath, fmt.Sprintf("${secret:%s}", name),
					fmt.Sprintf("placeholder has no matching spec.secretRefs[%q] entry", name)))
			}
		}
	}

	// secretRefs keys must satisfy the placeholder NAME grammar.
	for name := range agent.Spec.SecretRefs {
		if !validSecretRefName.MatchString(name) {
			allErrs = append(allErrs, field.Invalid(
				specPath.Child("secretRefs").Key(name), name,
				"secret ref names must match [A-Za-z0-9._-]+"))
		}
	}

	// Unique skill IDs on the card.
	seenSkills := make(map[string]bool)
	for i, skill := range agent.Spec.Card.Skills {
		if seenSkills[skill.ID] {
			allErrs = append(allErrs, field.Duplicate(
				specPath.Child("card", "skills").Index(i).Child("id"), skill.ID))
		}
		seenSkills[skill.ID] = true
	}

	return aggregateErrors("Agent", agent.Name, allErrs)
}

// fragmentSecretPlaceholders scans every string value of the fragment for
// ${secret:NAME} placeholders. Scanning the JSON form keeps this exhaustive
// as the fragment grows fields.
func fragmentSecretPlaceholders(frag *AgentSpecFragment) []string {
	raw, err := json.Marshal(frag)
	if err != nil {
		return nil
	}
	matches := secretPlaceholderRe.FindAllStringSubmatch(string(raw), -1)
	names := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		if !seen[m[1]] {
			names = append(names, m[1])
			seen[m[1]] = true
		}
	}
	return names
}

// validateReferences checks that cross-resource references exist.
// These are returned as warnings (not errors) to avoid ordering issues during
// resource creation (fixes #94).
func (v *AgentCustomValidator) validateReferences(ctx context.Context, agent *Agent) admission.Warnings {
	if v.Reader == nil {
		return nil
	}

	var warnings admission.Warnings

	refNamespace := func(ns string) string {
		if ns == "" {
			return agent.Namespace
		}
		return ns
	}

	if agent.Spec.ModelRef != nil {
		ns := refNamespace(agent.Spec.ModelRef.Namespace)
		model := &Model{}
		if err := v.Reader.Get(ctx, types.NamespacedName{Name: agent.Spec.ModelRef.Name, Namespace: ns}, model); err != nil {
			warnings = append(warnings,
				fmt.Sprintf("referenced Model %s/%s not found — the agent will not reconcile until it exists", ns, agent.Spec.ModelRef.Name))
		}
	}

	for _, ref := range agent.Spec.InstructionRefs {
		ns := refNamespace(ref.Namespace)
		instr := &Instruction{}
		if err := v.Reader.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, instr); err != nil {
			warnings = append(warnings,
				fmt.Sprintf("referenced Instruction %s/%s not found — the agent will not reconcile until it exists", ns, ref.Name))
		}
	}

	for _, ref := range agent.Spec.Tools {
		ns := refNamespace(ref.Namespace)
		at := &AgentTool{}
		if err := v.Reader.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, at); err != nil {
			warnings = append(warnings,
				fmt.Sprintf("referenced AgentTool %s/%s not found — the agent will not reconcile until it exists", ns, ref.Name))
		}
	}

	return warnings
}
