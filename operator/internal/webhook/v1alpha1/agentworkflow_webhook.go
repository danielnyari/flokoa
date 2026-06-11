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

	if errs := validateAgentTaskFreeze(agentworkflow, nil); len(errs) > 0 {
		return nil, aggregateErrors(errs, agentworkflow.Name)
	}
	return nil, ValidateAgentWorkflow(agentworkflow)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type AgentWorkflow.
func (v *AgentWorkflowCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	agentworkflow, ok := newObj.(*agentv1alpha1.AgentWorkflow)
	if !ok {
		return nil, fmt.Errorf("expected a AgentWorkflow object for the newObj but got %T", newObj)
	}
	oldWorkflow, ok := oldObj.(*agentv1alpha1.AgentWorkflow)
	if !ok {
		return nil, fmt.Errorf("expected a AgentWorkflow object for the oldObj but got %T", oldObj)
	}
	agentworkflowlog.Info("Validation for AgentWorkflow upon update", "name", agentworkflow.GetName())

	if errs := validateAgentTaskFreeze(agentworkflow, oldWorkflow); len(errs) > 0 {
		return nil, aggregateErrors(errs, agentworkflow.Name)
	}
	return nil, ValidateAgentWorkflow(agentworkflow)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type AgentWorkflow.
func (v *AgentWorkflowCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// expressionPattern matches {{...}} template expressions.
var expressionPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// agentTaskFreezeMessage explains why agentTask usage is rejected.
const agentTaskFreezeMessage = "agentTask is no longer supported: the AgentWorkflow API is frozen as a " +
	"template-only resource and the agentTask runtime was removed in the v2.1 pivot; " +
	"use an agent task calling a deployed Agent instead"

// validateAgentTaskFreeze rejects new agentTask usage. The AgentWorkflow API is
// frozen; the agentTask runtime no longer exists. On update, tasks that already
// used agentTask in the old object (matched by task name) are tolerated so that
// pre-existing objects can still be updated (e.g. to remove the frozen tasks).
func validateAgentTaskFreeze(wf, old *agentv1alpha1.AgentWorkflow) field.ErrorList {
	var allErrs field.ErrorList

	oldAgentTasks := make(map[string]bool)
	if old != nil {
		for _, task := range old.Spec.Tasks {
			if task.AgentTask != nil { //nolint:staticcheck // the freeze guard must inspect the deprecated field
				oldAgentTasks[task.Name] = true
			}
		}
	}

	tasksPath := field.NewPath("spec").Child("tasks")
	for i, task := range wf.Spec.Tasks {
		if task.AgentTask == nil { //nolint:staticcheck // the freeze guard must inspect the deprecated field
			continue
		}
		if oldAgentTasks[task.Name] {
			continue
		}
		allErrs = append(allErrs, field.Forbidden(
			tasksPath.Index(i).Child("agentTask"), agentTaskFreezeMessage))
	}

	return allErrs
}

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
		if task.AgentTask != nil { //nolint:staticcheck // frozen field still counted for pre-existing objects
			typeCount++
		}
		if task.Container != nil {
			typeCount++
		}
		if task.HTTP != nil {
			typeCount++
		}
		if len(task.Switch) > 0 {
			typeCount++
		}
		if typeCount == 0 {
			allErrs = append(allErrs, field.Required(taskPath,
				"exactly one of agent, agentTask, container, http, or switch must be specified"))
		}
		if typeCount > 1 {
			allErrs = append(allErrs, field.Forbidden(taskPath,
				"only one of agent, agentTask, container, http, or switch may be specified"))
		}

		// W4+W5: Validate AgentCall (text/message exclusivity + message parts)
		if task.Agent != nil {
			allErrs = append(allErrs, validateAgentCall(taskPath.Child("agent"), task.Agent)...)
		}

		// Validate ContainerTask
		if task.Container != nil {
			allErrs = append(allErrs, validateContainerTask(taskPath.Child("container"), task.Container)...)
		}

		// Validate HTTPTask
		if task.HTTP != nil {
			allErrs = append(allErrs, validateHTTPTask(taskPath.Child("http"), task.HTTP)...)
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

	return aggregateErrors(allErrs, wf.Name)
}

// validateExpressions validates all {{...}} expressions in a task.
// validateAgentCall checks text/message mutual exclusivity and message part types.
func validateAgentCall(agentPath *field.Path, agent *agentv1alpha1.AgentCall) field.ErrorList {
	var allErrs field.ErrorList

	hasText := agent.Text != ""
	hasMessage := agent.Message != nil
	if !hasText && !hasMessage {
		allErrs = append(allErrs, field.Required(agentPath,
			"one of text or message must be specified"))
	}
	if hasText && hasMessage {
		allErrs = append(allErrs, field.Forbidden(agentPath,
			"text and message are mutually exclusive"))
	}

	if hasMessage {
		for j, part := range agent.Message.Parts {
			partPath := agentPath.Child("message", "parts").Index(j)
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

	return allErrs
}

// validateContainerTask checks that the container task has a non-empty image.
func validateContainerTask(containerPath *field.Path, ct *agentv1alpha1.ContainerTask) field.ErrorList {
	var allErrs field.ErrorList

	if ct.Image == "" {
		allErrs = append(allErrs, field.Required(containerPath.Child("image"),
			"image is required"))
	}

	return allErrs
}

// validateHTTPTask checks that the HTTP task has a valid URL and well-formed headers.
func validateHTTPTask(httpPath *field.Path, ht *agentv1alpha1.HTTPTask) field.ErrorList {
	var allErrs field.ErrorList

	if ht.URL == "" {
		allErrs = append(allErrs, field.Required(httpPath.Child("url"),
			"url is required"))
	}

	for i, header := range ht.Headers {
		headerPath := httpPath.Child("headers").Index(i)

		hasValue := header.Value != ""
		hasValueFrom := header.ValueFrom != nil

		if !hasValue && !hasValueFrom {
			allErrs = append(allErrs, field.Required(headerPath,
				"one of value or valueFrom must be specified"))
		}
		if hasValue && hasValueFrom {
			allErrs = append(allErrs, field.Forbidden(headerPath,
				"value and valueFrom are mutually exclusive"))
		}

		if hasValueFrom {
			hasSecret := header.ValueFrom.SecretKeyRef != nil
			hasCM := header.ValueFrom.ConfigMapKeyRef != nil

			if !hasSecret && !hasCM {
				allErrs = append(allErrs, field.Required(headerPath.Child("valueFrom"),
					"one of secretKeyRef or configMapKeyRef must be specified"))
			}
			if hasSecret && hasCM {
				allErrs = append(allErrs, field.Forbidden(headerPath.Child("valueFrom"),
					"secretKeyRef and configMapKeyRef are mutually exclusive"))
			}
		}
	}

	return allErrs
}

func validateExpressions(taskPath *field.Path, task agentv1alpha1.WorkflowTask, taskNames map[string]int, paramNames map[string]bool) field.ErrorList {
	var allErrs field.ErrorList

	type exprField struct {
		path  *field.Path
		value string
	}

	var fields []exprField

	if task.Agent != nil {
		// Scan text shorthand for expressions
		if task.Agent.Text != "" {
			fields = append(fields, exprField{
				taskPath.Child("agent", "text"),
				task.Agent.Text,
			})
		}
		// Scan all message text parts for expressions
		if task.Agent.Message != nil {
			for i, part := range task.Agent.Message.Parts {
				if part.Text != nil {
					fields = append(fields, exprField{
						taskPath.Child("agent", "message", "parts").Index(i).Child("text", "text"),
						part.Text.Text,
					})
				}
			}
		}
	}
	if task.AgentTask != nil { //nolint:staticcheck // frozen field still validated for pre-existing objects
		fields = append(fields, exprField{taskPath.Child("agentTask", "input"), task.AgentTask.Input}) //nolint:staticcheck // see above
	}
	if task.Container != nil {
		for i, env := range task.Container.Env {
			if env.Value != "" {
				fields = append(fields, exprField{
					taskPath.Child("container", "env").Index(i).Child("value"),
					env.Value,
				})
			}
		}
	}
	if task.HTTP != nil {
		if task.HTTP.URL != "" {
			fields = append(fields, exprField{taskPath.Child("http", "url"), task.HTTP.URL})
		}
		if task.HTTP.Body != "" {
			fields = append(fields, exprField{taskPath.Child("http", "body"), task.HTTP.Body})
		}
		for i, header := range task.HTTP.Headers {
			if header.Value != "" {
				fields = append(fields, exprField{
					taskPath.Child("http", "headers").Index(i).Child("value"),
					header.Value,
				})
			}
		}
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
	// Argo evaluation expressions ({{=...}}) pass through
	if strings.HasPrefix(expr, "=") {
		return true
	}

	// Check params.<name>
	if strings.HasPrefix(expr, "params.") {
		paramName := strings.TrimPrefix(expr, "params.")
		return paramNames[paramName]
	}

	// Check tasks.<name>.output[.<field>] or tasks.<name>.artifact
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
		if suffix == "output" || suffix == "artifact" || strings.HasPrefix(suffix, "output.") {
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
