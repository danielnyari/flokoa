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

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-agenttrigger,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=agenttriggers,verbs=create;update,versions=v1alpha1,name=vagenttrigger-v1alpha1.kb.io,admissionReviewVersions=v1

// AgentTriggerCustomValidator validates AgentTrigger resources.
type AgentTriggerCustomValidator struct{}

var _ webhook.CustomValidator = &AgentTriggerCustomValidator{}

// SetupAgentTriggerWebhookWithManager registers the webhook with the manager.
func SetupAgentTriggerWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&AgentTrigger{}).
		WithValidator(&AgentTriggerCustomValidator{}).
		Complete()
}

func (v *AgentTriggerCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	trigger, ok := obj.(*AgentTrigger)
	if !ok {
		return nil, fmt.Errorf("expected an AgentTrigger but got %T", obj)
	}
	return nil, validateAgentTrigger(trigger)
}

func (v *AgentTriggerCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	trigger, ok := newObj.(*AgentTrigger)
	if !ok {
		return nil, fmt.Errorf("expected an AgentTrigger but got %T", newObj)
	}
	return nil, validateAgentTrigger(trigger)
}

func (v *AgentTriggerCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateAgentTrigger(trigger *AgentTrigger) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")

	// EventSource validation
	esPath := specPath.Child("eventSource")
	if trigger.Spec.EventSource.Name == "" {
		allErrs = append(allErrs, field.Required(esPath.Child("name"), "eventSource name is required"))
	}
	if trigger.Spec.EventSource.EventName == "" {
		allErrs = append(allErrs, field.Required(esPath.Child("eventName"), "eventSource eventName is required"))
	}

	// Agent validation
	agentPath := specPath.Child("agent")
	if trigger.Spec.Agent.Name == "" {
		allErrs = append(allErrs, field.Required(agentPath.Child("name"), "agent name is required"))
	}

	// PushNotification validation
	if trigger.Spec.PushNotification != nil {
		pushPath := specPath.Child("pushNotification")
		hasAgentRef := trigger.Spec.PushNotification.AgentRef != nil
		hasURL := trigger.Spec.PushNotification.URL != ""

		if err := validateExactlyOneOf(pushPath, []string{"agentRef", "url"}, []bool{hasAgentRef, hasURL}); err != nil {
			allErrs = append(allErrs, err)
		}

		if hasAgentRef && trigger.Spec.PushNotification.AgentRef.Name == "" {
			allErrs = append(allErrs, field.Required(pushPath.Child("agentRef", "name"), "agentRef name is required"))
		}

		if hasURL {
			if err := validateHTTPURL(pushPath.Child("url"), trigger.Spec.PushNotification.URL); err != nil {
				allErrs = append(allErrs, err)
			}
		}
	}

	// Filter validation
	if trigger.Spec.Filter != nil {
		filterPath := specPath.Child("filter")
		for i, df := range trigger.Spec.Filter.Data {
			dfPath := filterPath.Child("data").Index(i)
			if df.Path == "" {
				allErrs = append(allErrs, field.Required(dfPath.Child("path"), "data filter path is required"))
			}
			if len(df.Value) == 0 {
				allErrs = append(allErrs, field.Required(dfPath.Child("value"), "data filter must have at least one value"))
			}
		}
		for i, ef := range trigger.Spec.Filter.Exprs {
			efPath := filterPath.Child("exprs").Index(i)
			if ef.Expr == "" {
				allErrs = append(allErrs, field.Required(efPath.Child("expr"), "expression is required"))
			}
		}
	}

	// Limits validation
	if trigger.Spec.Limits != nil && trigger.Spec.Limits.DeadLetterSink != nil {
		dlsPath := specPath.Child("limits", "deadLetterSink")
		if trigger.Spec.Limits.DeadLetterSink.URI == "" {
			allErrs = append(allErrs, field.Required(dlsPath.Child("uri"), "dead letter sink URI is required"))
		}
	}

	return aggregateErrors("AgentTrigger", trigger.Name, allErrs)
}
