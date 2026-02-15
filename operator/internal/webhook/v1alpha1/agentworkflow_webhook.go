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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var agentworkflowlog = logf.Log.WithName("agentworkflow-resource")

// SetupAgentWorkflowWebhookWithManager registers the webhook for AgentWorkflow in the manager.
func SetupAgentWorkflowWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&agentv1alpha1.AgentWorkflow{}).
		WithValidator(&AgentWorkflowCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-agent-flokoa-ai-v1alpha1-agentworkflow,mutating=false,failurePolicy=fail,sideEffects=None,groups=agent.flokoa.ai,resources=agentworkflows,verbs=create;update,versions=v1alpha1,name=vagentworkflow-v1alpha1.kb.io,admissionReviewVersions=v1

// AgentWorkflowCustomValidator struct is responsible for validating the AgentWorkflow resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type AgentWorkflowCustomValidator struct{}

var _ webhook.CustomValidator = &AgentWorkflowCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type AgentWorkflow.
func (v *AgentWorkflowCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	agentworkflow, ok := obj.(*agentv1alpha1.AgentWorkflow)
	if !ok {
		return nil, fmt.Errorf("expected a AgentWorkflow object but got %T", obj)
	}
	agentworkflowlog.Info("Validation for AgentWorkflow upon creation", "name", agentworkflow.GetName())
	return nil, ValidateAgentWorkflow(agentworkflow)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type AgentWorkflow.
func (v *AgentWorkflowCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	agentworkflow, ok := newObj.(*agentv1alpha1.AgentWorkflow)
	if !ok {
		return nil, fmt.Errorf("expected a AgentWorkflow object for the newObj but got %T", newObj)
	}
	agentworkflowlog.Info("Validation for AgentWorkflow upon update", "name", agentworkflow.GetName())
	return nil, ValidateAgentWorkflow(agentworkflow)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type AgentWorkflow.
func (v *AgentWorkflowCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// expressionPattern matches {{...}} template expressions.
var expressionPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// ValidateAgentWorkflow performs all validation on an AgentWorkflow resource.
func ValidateAgentWorkflow(wf *agentv1alpha1.AgentWorkflow) error {
	var allErrs field.ErrorList
	specPath := field.NewPath("spec")
	tasksPath := specPath.Child("tasks")

	// W1: At least one task
	if len(wf.Spec.Tasks) == 0 {
		allErrs = append(allErrs, field.Required(tasksPath, "at least one task is required"))
	}

	// Build task name index for reference validation
	taskNames := make(map[string]int, len(wf.Spec.Tasks))
	for i, task := range wf.Spec.Tasks {
		taskNames[task.Name] = i
	}

	// Build param name index for expression validation
	paramNames := make(map[string]bool, len(wf.Spec.Params))
	for _, p := range wf.Spec.Params {
		paramNames[p.Name] = true
	}

	// W2: Unique task names
	seenNames := make(map[string]bool, len(wf.Spec.Tasks))
	for i, task := range wf.Spec.Tasks {
		taskPath := tasksPath.Index(i)
		if seenNames[task.Name] {
			allErrs = append(allErrs, field.Duplicate(taskPath.Child("name"), task.Name))
		}
		seenNames[task.Name] = true

		// W3: Exactly one task type
		typeCount := 0
		if task.Agent != nil {
			typeCount++
		}
		if task.AgentTask != nil {
			typeCount++
		}
		if task.WaitForSignal != nil {
			typeCount++
		}
		if len(task.Switch) > 0 {
			typeCount++
		}
		if typeCount == 0 {
			allErrs = append(allErrs, field.Required(taskPath,
				"exactly one of agent, agentTask, waitForSignal, or switch must be specified"))
		}
		if typeCount > 1 {
			allErrs = append(allErrs, field.Forbidden(taskPath,
				"only one of agent, agentTask, waitForSignal, or switch may be specified"))
		}

		// W4: Engine compatibility — Temporal-only features
		if wf.Spec.Engine == agentv1alpha1.EngineTypeArgo || wf.Spec.Engine == "" {
			if task.WaitForSignal != nil {
				allErrs = append(allErrs, field.Forbidden(taskPath.Child("waitForSignal"),
					"waitForSignal is only supported when engine is \"temporal\""))
			}
			if task.Loop != nil {
				allErrs = append(allErrs, field.Forbidden(taskPath.Child("loop"),
					"loop is only supported when engine is \"temporal\""))
			}
		}

		// W5b: Validate message parts — exactly one of text/data/file per part
		if task.Agent != nil {
			for j, part := range task.Agent.Message.Parts {
				partPath := taskPath.Child("agent", "message", "parts").Index(j)
				partTypeCount := 0
				if part.Text != nil {
					partTypeCount++
				}
				if part.Data != nil {
					partTypeCount++
				}
				if part.File != nil {
					partTypeCount++
				}
				if partTypeCount == 0 {
					allErrs = append(allErrs, field.Required(partPath,
						"exactly one of text, data, or file must be set"))
				}
				if partTypeCount > 1 {
					allErrs = append(allErrs, field.Forbidden(partPath,
						"only one of text, data, or file may be set per part"))
				}
			}
		}

		// W6: Valid dependsOn references
		for j, dep := range task.DependsOn {
			if _, ok := taskNames[dep]; !ok {
				allErrs = append(allErrs, field.NotFound(
					taskPath.Child("dependsOn").Index(j), dep))
			}
			if dep == task.Name {
				allErrs = append(allErrs, field.Invalid(
					taskPath.Child("dependsOn").Index(j), dep,
					"a task cannot depend on itself"))
			}
		}

		// W7: Valid switch references
		for j, sc := range task.Switch {
			switchPath := taskPath.Child("switch").Index(j)
			if sc.Then != "" {
				if _, ok := taskNames[sc.Then]; !ok {
					allErrs = append(allErrs, field.NotFound(switchPath.Child("then"), sc.Then))
				}
			}
			if sc.Default != "" {
				if _, ok := taskNames[sc.Default]; !ok {
					allErrs = append(allErrs, field.NotFound(switchPath.Child("default"), sc.Default))
				}
			}
		}

		// W8/W9: Expression validation
		allErrs = append(allErrs, validateExpressions(taskPath, task, taskNames, paramNames)...)
	}

	// W5: DAG cycle detection
	if cycleErr := detectDAGCycles(wf.Spec.Tasks); cycleErr != nil {
		allErrs = append(allErrs, field.Invalid(tasksPath, nil, cycleErr.Error()))
	}

	// Temporal engine not yet implemented
	if wf.Spec.Engine == agentv1alpha1.EngineTypeTemporal {
		allErrs = append(allErrs, field.Invalid(specPath.Child("engine"), string(agentv1alpha1.EngineTypeTemporal),
			"temporal engine is not yet supported; use \"argo\" or omit the engine field"))
	}

	return aggregateErrors(allErrs, wf.Name)
}

// validateExpressions validates all {{...}} expressions in a task.
func validateExpressions(taskPath *field.Path, task agentv1alpha1.WorkflowTask, taskNames map[string]int, paramNames map[string]bool) field.ErrorList {
	var allErrs field.ErrorList

	type exprField struct {
		path  *field.Path
		value string
	}

	var fields []exprField

	if task.Agent != nil {
		// Scan all text parts for expressions
		for i, part := range task.Agent.Message.Parts {
			if part.Text != nil {
				fields = append(fields, exprField{
					taskPath.Child("agent", "message", "parts").Index(i).Child("text", "text"),
					part.Text.Text,
				})
			}
		}
	}
	if task.AgentTask != nil {
		fields = append(fields, exprField{taskPath.Child("agentTask", "input"), task.AgentTask.Input})
	}
	if task.Condition != "" {
		fields = append(fields, exprField{taskPath.Child("condition"), task.Condition})
	}
	for i, sc := range task.Switch {
		if sc.Condition != "" {
			fields = append(fields, exprField{taskPath.Child("switch").Index(i).Child("condition"), sc.Condition})
		}
	}

	for _, f := range fields {
		matches := expressionPattern.FindAllStringSubmatch(f.value, -1)
		for _, match := range matches {
			expr := strings.TrimSpace(match[1])
			if !isValidExpression(expr, taskNames, paramNames) {
				allErrs = append(allErrs, field.Invalid(f.path, match[0],
					fmt.Sprintf("invalid expression: %s", match[0])))
			}
		}
	}

	return allErrs
}

// isValidExpression checks if an expression body is a valid reference.
func isValidExpression(expr string, taskNames map[string]int, paramNames map[string]bool) bool {
	// Check params.<name>
	if strings.HasPrefix(expr, "params.") {
		paramName := strings.TrimPrefix(expr, "params.")
		return paramNames[paramName]
	}

	// Check tasks.<name>.output[.<field>] or tasks.<name>.taskResponse
	if strings.HasPrefix(expr, "tasks.") {
		rest := strings.TrimPrefix(expr, "tasks.")
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) < 2 {
			return false
		}
		taskName := parts[0]
		if _, ok := taskNames[taskName]; !ok {
			return false
		}
		suffix := parts[1]
		if suffix == "output" || suffix == "taskResponse" || strings.HasPrefix(suffix, "output.") {
			return true
		}
		return false
	}

	return false
}

// detectDAGCycles performs a topological sort to detect cycles in task dependencies.
func detectDAGCycles(tasks []agentv1alpha1.WorkflowTask) error {
	adj := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		adj[task.Name] = task.DependsOn
	}

	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int, len(tasks))
	for _, task := range tasks {
		color[task.Name] = white
	}

	var visit func(name string) error
	visit = func(name string) error {
		color[name] = gray
		for _, dep := range adj[name] {
			if color[dep] == gray {
				return fmt.Errorf("dependency cycle detected involving task %q", dep)
			}
			if color[dep] == white {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		color[name] = black
		return nil
	}

	for _, task := range tasks {
		if color[task.Name] == white {
			if err := visit(task.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// aggregateErrors converts a field.ErrorList into a single error or nil.
func aggregateErrors(allErrs field.ErrorList, name string) error {
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: agentv1alpha1.GroupVersion.Group, Kind: "AgentWorkflow"},
		name,
		allErrs,
	)
}
