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

package controller

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/telemetry"
)

const (
	// dagEntrypointName is the name of the DAG entrypoint template.
	dagEntrypointName = "main"

	// a2aPluginKey is the key used in the Argo plugin spec for A2A tasks.
	a2aPluginKey = "a2a"

	// agentTaskOutputParam is the output parameter name for task results.
	agentTaskOutputParam = "result"

	// agentTaskArtifactParam is the output parameter name for the A2A Artifact JSON.
	agentTaskArtifactParam = "artifact"

	// traceparentEnvVar is the env var used to propagate W3C traceparent into agent task pods.
	traceparentEnvVar = "FLOKOA_TRACEPARENT"

	// traceparentWorkflowParam is the Argo workflow-level parameter that carries the traceparent value.
	traceparentWorkflowParam = "_flokoa_traceparent"

	// defaultWorkflowServiceAccount is the default ServiceAccount for workflow pods.
	defaultWorkflowServiceAccount = "flokoa-workflow"
)

// CompilerOptions holds operator-level settings that affect compilation.
type CompilerOptions struct {
	// ArtifactIOEnabled switches task I/O from Argo parameters to artifacts.
	ArtifactIOEnabled bool
	// ArtifactGCStrategy is the garbage collection strategy for artifacts (e.g. "OnWorkflowCompletion").
	ArtifactGCStrategy string
}

// expressionRe matches {{...}} template expressions in DSL fields.
var expressionRe = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// compileToArgoWorkflowTemplate translates an AgentWorkflow DSL into an Argo WorkflowTemplate CR.
// Each AgentWorkflow compiles to a single WorkflowTemplate; individual runs are Argo Workflow CRs
// created from the template via WorkflowTemplateRef.
func compileToArgoWorkflowTemplate(awf *agentv1alpha1.AgentWorkflow, opts CompilerOptions) (*wfv1.WorkflowTemplate, error) {
	wft := &wfv1.WorkflowTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "WorkflowTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      awf.Name,
			Namespace: awf.Namespace,
			Labels: map[string]string{
				"agent.flokoa.ai/agentworkflow-name": awf.Name,
				"app.kubernetes.io/managed-by":       "flokoa-operator",
			},
		},
		Spec: wfv1.WorkflowSpec{
			Entrypoint: dagEntrypointName,
		},
	}

	// Service account for workflow pods.
	saName := awf.Spec.ServiceAccountName
	if saName == "" {
		saName = defaultWorkflowServiceAccount
	}
	wft.Spec.ServiceAccountName = saName

	if awf.Spec.AutomountServiceAccountToken != nil {
		wft.Spec.AutomountServiceAccountToken = awf.Spec.AutomountServiceAccountToken
	} else {
		automount := true
		wft.Spec.AutomountServiceAccountToken = &automount
	}

	// Inject traceparent as a workflow-level parameter so it is available to
	// all templates via {{workflow.parameters._flokoa_traceparent}}.
	// The actual value is provided at run submission time; the default is a
	// UUID7-based traceparent so direct argo submit / kubectl usage also works.
	wft.Spec.Arguments.Parameters = append(wft.Spec.Arguments.Parameters, wfv1.Parameter{
		Name:    traceparentWorkflowParam,
		Default: wfv1.AnyStringPtr(telemetry.NewTraceparent()),
	})

	// Workflow-level parameters
	if len(awf.Spec.Params) > 0 {
		for _, p := range awf.Spec.Params {
			param := wfv1.Parameter{
				Name:  p.Name,
				Value: wfv1.AnyStringPtr(p.Value),
			}
			if p.Description != "" {
				param.Description = wfv1.AnyStringPtr(p.Description)
			}
			wft.Spec.Arguments.Parameters = append(wft.Spec.Arguments.Parameters, param)
		}
	}

	// Workflow-level timeout
	if awf.Spec.Timeout != nil {
		seconds := int64(awf.Spec.Timeout.Seconds())
		wft.Spec.ActiveDeadlineSeconds = &seconds
	}

	// Workflow-level artifact GC when artifact I/O is enabled.
	if opts.ArtifactIOEnabled && opts.ArtifactGCStrategy != "" {
		wft.Spec.ArtifactGC = &wfv1.WorkflowLevelArtifactGC{
			ArtifactGC: wfv1.ArtifactGC{
				Strategy: wfv1.ArtifactGCStrategy(opts.ArtifactGCStrategy),
			},
		}
	}

	// Build templates and DAG tasks
	dagTasks := make([]wfv1.DAGTask, 0, len(awf.Spec.Tasks))
	templates := make([]wfv1.Template, 0, len(awf.Spec.Tasks)+1)

	for _, task := range awf.Spec.Tasks {
		templateName := task.Name

		// Build the template for this task
		tmpl, err := buildTemplate(awf, task, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to build template for task %q: %w", task.Name, err)
		}
		templates = append(templates, *tmpl)

		// Build the DAG task referencing this template
		dagTask := wfv1.DAGTask{
			Name:     task.Name,
			Template: templateName,
		}

		// dependsOn -> Dependencies
		if len(task.DependsOn) > 0 {
			dagTask.Dependencies = task.DependsOn
		}

		// condition -> When
		if task.Condition != "" {
			dagTask.When = translateConditionExpr(task.Condition)
		}

		dagTasks = append(dagTasks, dagTask)

		// Handle switch tasks: generate additional DAG tasks for each branch
		if len(task.Switch) > 0 {
			switchTasks := buildSwitchDAGTasks(task)
			dagTasks = append(dagTasks, switchTasks...)
		}
	}

	// Inject workflow parameters for HTTP configMapKeyRef headers.
	// Argo's HTTPHeaderSource only supports secretKeyRef natively, so we compile
	// configMapKeyRef headers to workflow parameters and reference them in header values.
	cmParamsSeen := map[string]bool{}
	for _, task := range awf.Spec.Tasks {
		if task.HTTP == nil {
			continue
		}
		for _, header := range task.HTTP.Headers {
			if header.ValueFrom == nil || header.ValueFrom.ConfigMapKeyRef == nil {
				continue
			}
			ref := header.ValueFrom.ConfigMapKeyRef
			paramName := fmt.Sprintf("_cm_%s_%s", ref.Name, ref.Key)
			if cmParamsSeen[paramName] {
				continue
			}
			cmParamsSeen[paramName] = true
			wft.Spec.Arguments.Parameters = append(wft.Spec.Arguments.Parameters, wfv1.Parameter{
				Name: paramName,
				ValueFrom: &wfv1.ValueFrom{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
						Key:                  ref.Key,
					},
				},
			})
		}
	}

	// Create the DAG entrypoint template
	dagTemplate := wfv1.Template{
		Name: dagEntrypointName,
		DAG: &wfv1.DAGTemplate{
			Tasks: dagTasks,
		},
	}
	templates = append([]wfv1.Template{dagTemplate}, templates...)

	wft.Spec.Templates = templates

	return wft, nil
}

// buildTemplate creates an Argo template for a single workflow task.
func buildTemplate(awf *agentv1alpha1.AgentWorkflow, task agentv1alpha1.WorkflowTask, opts CompilerOptions) (*wfv1.Template, error) {
	tmpl := &wfv1.Template{
		Name: task.Name,
	}

	// Per-task timeout
	if task.Timeout != nil {
		seconds := int64(task.Timeout.Seconds())
		tmpl.ActiveDeadlineSeconds = &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: int32(seconds),
		}
	}

	// Per-task retry strategy (falls back to workflow-level)
	retryStrategy := task.RetryStrategy
	if retryStrategy == nil {
		retryStrategy = awf.Spec.RetryStrategy
	}
	if retryStrategy != nil {
		tmpl.RetryStrategy = translateRetryStrategy(retryStrategy)
	}

	switch {
	case task.Agent != nil:
		if err := buildAgentTemplate(tmpl, task.Agent); err != nil {
			return nil, err
		}
	case task.AgentTask != nil: //nolint:staticcheck // the freeze guard must inspect the deprecated field
		// The agentTask runtime was removed in the v2.1 pivot; AgentWorkflow is
		// frozen as a template-only resource. The webhook rejects new agentTask
		// usage; this guards pre-existing objects.
		return nil, fmt.Errorf("agentTask is no longer supported: its runtime was removed and the AgentWorkflow API is frozen (use an agent task calling a deployed Agent instead)")
	case task.Container != nil:
		buildContainerTemplate(tmpl, task.Container, opts)
	case task.HTTP != nil:
		buildHTTPTemplate(tmpl, task.HTTP)
	case len(task.Switch) > 0:
		// Switch tasks are a no-op routing template; branching is in DAG when expressions.
		tmpl.Script = &wfv1.ScriptTemplate{
			Container: corev1.Container{
				Image:   "alpine:3.18",
				Command: []string{"echo"},
				Args:    []string{"switch-router"},
			},
		}
	default:
		return nil, fmt.Errorf("task has no recognized type (must set one of agent, agentTask, container, http, or switch)")
	}

	return tmpl, nil
}

// buildAgentTemplate populates a template with the A2A plugin spec for calling a deployed agent.
func buildAgentTemplate(tmpl *wfv1.Template, agent *agentv1alpha1.AgentCall) error {
	// Normalize: expand text shorthand to a full AgentMessage if needed.
	msg := agent.Message
	if msg == nil {
		msg = &agentv1alpha1.AgentMessage{
			Parts: []agentv1alpha1.MessagePart{
				{Text: &agentv1alpha1.TextPart{Text: agent.Text}},
			},
		}
	}

	// Translate DSL expressions (e.g. {{params.x}}, {{tasks.y.output}}) in message
	// text parts to Argo workflow syntax before embedding in the plugin spec.
	translatedMessage := translateAgentMessage(msg)

	a2aSpec := map[string]interface{}{
		"agent":   agent.Name,
		"message": buildPluginMessage(translatedMessage),
		// Argo substitutes the workflow parameter reference at runtime so the
		// A2A plugin receives the actual traceparent value.
		"traceparent": fmt.Sprintf("{{workflow.parameters.%s}}", traceparentWorkflowParam),
	}
	if agent.Namespace != "" {
		a2aSpec["namespace"] = agent.Namespace
	}
	if agent.Config != nil {
		a2aSpec["config"] = buildPluginSendConfig(agent.Config)
	}

	pluginData := map[string]interface{}{
		a2aPluginKey: a2aSpec,
	}

	pluginJSON, err := json.Marshal(pluginData)
	if err != nil {
		return fmt.Errorf("failed to marshal A2A plugin spec: %w", err)
	}

	tmpl.Plugin = &wfv1.Plugin{}
	tmpl.Plugin.Value = pluginJSON

	// Declare output parameters as "supplied" — the A2A executor plugin
	// fills them via NodeResult at runtime.
	supplied := &wfv1.ValueFrom{Supplied: &wfv1.SuppliedValueFrom{}}
	tmpl.Outputs = wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{Name: agentTaskOutputParam, ValueFrom: supplied},
			{Name: agentTaskArtifactParam, ValueFrom: supplied},
		},
	}

	return nil
}

// rawJSONMapToAny converts a map of apiextensionsv1.JSON values to map[string]interface{}.
func rawJSONMapToAny(m map[string]apiextensionsv1.JSON) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		var val interface{}
		if err := json.Unmarshal(v.Raw, &val); err == nil {
			result[k] = val
		}
	}
	return result
}

// buildPluginMessage converts the CRD AgentMessage to a plugin-compatible message structure.
func buildPluginMessage(msg *agentv1alpha1.AgentMessage) map[string]interface{} {
	result := map[string]interface{}{}

	if msg.Role != "" {
		result["role"] = string(msg.Role)
	}

	parts := make([]map[string]interface{}, 0, len(msg.Parts))
	for _, p := range msg.Parts {
		part := map[string]interface{}{}
		if p.Text != nil {
			textPart := map[string]interface{}{"text": p.Text.Text}
			if len(p.Text.Metadata) > 0 {
				textPart["metadata"] = rawJSONMapToAny(p.Text.Metadata)
			}
			part["text"] = textPart
		}
		if p.Data != nil {
			dataPart := map[string]interface{}{"data": p.Data.Data.Raw}
			if len(p.Data.Metadata) > 0 {
				dataPart["metadata"] = rawJSONMapToAny(p.Data.Metadata)
			}
			part["data"] = dataPart
		}
		if p.File != nil {
			filePart := map[string]interface{}{
				"file": map[string]interface{}{
					"name":     p.File.File.Name,
					"mimeType": p.File.File.MimeType,
					"bytes":    p.File.File.Bytes,
					"uri":      p.File.File.URI,
				},
			}
			if len(p.File.Metadata) > 0 {
				filePart["metadata"] = rawJSONMapToAny(p.File.Metadata)
			}
			part["file"] = filePart
		}
		parts = append(parts, part)
	}
	result["parts"] = parts

	if msg.ContextID != "" {
		result["contextId"] = msg.ContextID
	}
	if len(msg.ReferenceTaskIDs) > 0 {
		result["referenceTaskIds"] = msg.ReferenceTaskIDs
	}
	if len(msg.Extensions) > 0 {
		result["extensions"] = msg.Extensions
	}
	if msg.TaskID != "" {
		result["taskId"] = msg.TaskID
	}
	if len(msg.Metadata) > 0 {
		result["metadata"] = rawJSONMapToAny(msg.Metadata)
	}

	return result
}

// buildPluginSendConfig converts the CRD MessageSendConfig to a plugin-compatible config structure.
func buildPluginSendConfig(cfg *agentv1alpha1.MessageSendConfig) map[string]interface{} {
	result := map[string]interface{}{}
	if len(cfg.AcceptedOutputModes) > 0 {
		result["acceptedOutputModes"] = cfg.AcceptedOutputModes
	}
	if cfg.Blocking != nil {
		result["blocking"] = *cfg.Blocking
	}
	if cfg.HistoryLength != nil {
		result["historyLength"] = *cfg.HistoryLength
	}
	if cfg.PushNotificationConfig != nil {
		pushCfg := map[string]interface{}{
			"url": cfg.PushNotificationConfig.URL,
		}
		if cfg.PushNotificationConfig.ID != "" {
			pushCfg["id"] = cfg.PushNotificationConfig.ID
		}
		if cfg.PushNotificationConfig.Token != "" {
			pushCfg["token"] = cfg.PushNotificationConfig.Token
		}
		if cfg.PushNotificationConfig.Authentication != nil {
			pushCfg["authentication"] = map[string]interface{}{
				"schemes":     cfg.PushNotificationConfig.Authentication.Schemes,
				"credentials": cfg.PushNotificationConfig.Authentication.Credentials,
			}
		}
		result["pushNotificationConfig"] = pushCfg
	}
	return result
}

// buildContainerTemplate populates a template for an arbitrary container task.
func buildContainerTemplate(tmpl *wfv1.Template, ct *agentv1alpha1.ContainerTask, opts CompilerOptions) {
	// Translate DSL expressions in env var values.
	env := make([]corev1.EnvVar, 0, len(ct.Env)+1)
	env = append(env, corev1.EnvVar{
		Name:  traceparentEnvVar,
		Value: fmt.Sprintf("{{workflow.parameters.%s}}", traceparentWorkflowParam),
	})
	for _, e := range ct.Env {
		translated := e.DeepCopy()
		if translated.Value != "" {
			translated.Value = TranslateExpressions(translated.Value)
		}
		env = append(env, *translated)
	}

	container := corev1.Container{
		Image:        ct.Image,
		Command:      ct.Command,
		Args:         ct.Args,
		Env:          env,
		WorkingDir:   ct.WorkingDir,
		VolumeMounts: ct.VolumeMounts,
	}
	if ct.Resources != nil {
		container.Resources = *ct.Resources
	}

	tmpl.Container = &container

	// Output parameters: the container writes to /tmp/result and /tmp/artifact.
	// Artifact gets a default of "{}" since containers may not produce structured output.
	if opts.ArtifactIOEnabled {
		tmpl.Outputs = wfv1.Outputs{
			Artifacts: wfv1.Artifacts{
				{
					Name: agentTaskOutputParam,
					ArtifactLocation: wfv1.ArtifactLocation{
						Raw: &wfv1.RawArtifact{Data: ""},
					},
				},
				{
					Name: agentTaskArtifactParam,
					ArtifactLocation: wfv1.ArtifactLocation{
						Raw: &wfv1.RawArtifact{Data: "{}"},
					},
				},
			},
		}
	} else {
		emptyDefault := wfv1.AnyStringPtr("{}")
		tmpl.Outputs = wfv1.Outputs{
			Parameters: []wfv1.Parameter{
				{
					Name: agentTaskOutputParam,
					ValueFrom: &wfv1.ValueFrom{
						Path: "/tmp/result",
					},
				},
				{
					Name:    agentTaskArtifactParam,
					Default: emptyDefault,
					ValueFrom: &wfv1.ValueFrom{
						Path: "/tmp/artifact",
					},
				},
			},
		}
	}
}

// buildHTTPTemplate populates a template for an HTTP request task.
func buildHTTPTemplate(tmpl *wfv1.Template, ht *agentv1alpha1.HTTPTask) {
	// Translate DSL expressions in URL, body, and header values.
	url := TranslateExpressions(ht.URL)
	body := TranslateExpressions(ht.Body)

	method := ht.Method
	if method == "" {
		method = "GET"
	}

	headers := make([]wfv1.HTTPHeader, 0, len(ht.Headers))
	for _, h := range ht.Headers {
		argoHeader := wfv1.HTTPHeader{Name: h.Name}

		switch {
		case h.ValueFrom != nil && h.ValueFrom.SecretKeyRef != nil:
			argoHeader.ValueFrom = &wfv1.HTTPHeaderSource{
				SecretKeyRef: h.ValueFrom.SecretKeyRef.DeepCopy(),
			}
		case h.ValueFrom != nil && h.ValueFrom.ConfigMapKeyRef != nil:
			// ConfigMapKeyRef is compiled to a workflow parameter reference.
			ref := h.ValueFrom.ConfigMapKeyRef
			paramName := fmt.Sprintf("_cm_%s_%s", ref.Name, ref.Key)
			argoHeader.Value = fmt.Sprintf("{{workflow.parameters.%s}}", paramName)
		default:
			argoHeader.Value = TranslateExpressions(h.Value)
		}

		headers = append(headers, argoHeader)
	}

	httpSpec := &wfv1.HTTP{
		URL:    url,
		Method: method,
		Body:   body,
	}
	if len(headers) > 0 {
		httpSpec.Headers = headers
	}
	tmpl.HTTP = httpSpec
	if ht.SuccessCondition != "" {
		tmpl.HTTP.SuccessCondition = ht.SuccessCondition
	}

	// HTTP output parameters: Argo auto-captures response.body to the "result" output.
	// We use expression-based ValueFrom for custom output extraction.
	tmpl.Outputs = wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{
				Name: agentTaskOutputParam,
				ValueFrom: &wfv1.ValueFrom{
					Expression: "response.body",
				},
			},
			{
				Name: agentTaskArtifactParam,
				ValueFrom: &wfv1.ValueFrom{
					Expression: "toJson({statusCode: response.statusCode, headers: response.headers, body: response.body})",
				},
			},
		},
	}
}

// buildSwitchDAGTasks generates additional DAG tasks for switch routing.
func buildSwitchDAGTasks(task agentv1alpha1.WorkflowTask) []wfv1.DAGTask {
	var tasks []wfv1.DAGTask

	for _, sc := range task.Switch {
		if sc.Then != "" && sc.Condition != "" {
			dagTask := wfv1.DAGTask{
				Name:         fmt.Sprintf("%s-%s", task.Name, sc.Then),
				Template:     sc.Then,
				Dependencies: []string{task.Name},
				When:         translateConditionExpr(sc.Condition),
			}
			tasks = append(tasks, dagTask)
		}
		if sc.Default != "" {
			dagTask := wfv1.DAGTask{
				Name:         fmt.Sprintf("%s-%s", task.Name, sc.Default),
				Template:     sc.Default,
				Dependencies: []string{task.Name},
			}
			tasks = append(tasks, dagTask)
		}
	}

	return tasks
}

// translateRetryStrategy converts a DSL retry strategy to an Argo retry strategy.
func translateRetryStrategy(rs *agentv1alpha1.WorkflowRetryStrategy) *wfv1.RetryStrategy {
	limit := intstr.FromInt32(rs.Limit)
	argoRS := &wfv1.RetryStrategy{
		Limit: &limit,
	}

	if rs.Backoff != nil {
		argoBackoff := &wfv1.Backoff{
			Duration: rs.Backoff.Duration,
		}
		if rs.Backoff.Factor != nil {
			factor := intstr.FromInt32(*rs.Backoff.Factor)
			argoBackoff.Factor = &factor
		}
		argoRS.Backoff = argoBackoff
	}

	return argoRS
}

// translateConditionExpr converts a DSL condition expression to an Argo "when" expression.
func translateConditionExpr(expr string) string {
	return expressionRe.ReplaceAllStringFunc(expr, func(match string) string {
		inner := expressionRe.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		trimmed := strings.TrimSpace(inner[1])
		return translateExpression(trimmed)
	})
}

// translateExpression converts a single DSL expression body to its Argo equivalent.
func translateExpression(expr string) string {
	return translateExpressionWithMode(expr, false)
}

// translateExpressionWithMode converts a single DSL expression body to its Argo equivalent.
// When artifactIO is true, task output references use outputs.artifacts instead of outputs.parameters.
func translateExpressionWithMode(expr string, artifactIO bool) string {
	// Pass through Argo evaluation expressions ({{=...}})
	if strings.HasPrefix(expr, "=") {
		return "{{" + expr + "}}"
	}

	// params.<name> -> {{workflow.parameters.<name>}}
	if strings.HasPrefix(expr, "params.") {
		paramName := strings.TrimPrefix(expr, "params.")
		return fmt.Sprintf("{{workflow.parameters.%s}}", paramName)
	}

	outputKind := "parameters"
	if artifactIO {
		outputKind = "artifacts"
	}

	// tasks.<name>.output -> {{tasks.<name>.outputs.parameters.result}}
	// tasks.<name>.output.<field> -> Argo expression with fromJson for field access
	// tasks.<name>.artifact -> {{tasks.<name>.outputs.parameters.artifact}}
	if strings.HasPrefix(expr, "tasks.") {
		rest := strings.TrimPrefix(expr, "tasks.")
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) < 2 {
			return "{{" + expr + "}}"
		}
		taskName := parts[0]
		suffix := parts[1]

		// tasks.<name>.artifact → raw artifact parameter
		if suffix == "artifact" {
			return fmt.Sprintf("{{tasks.%s.outputs.%s.%s}}", taskName, outputKind, agentTaskArtifactParam)
		}

		// tasks.<name>.output → plain text result parameter
		if suffix == "output" {
			return fmt.Sprintf("{{tasks.%s.outputs.%s.%s}}", taskName, outputKind, agentTaskOutputParam)
		}

		// tasks.<name>.output.<path> → Argo expression with fromJson for field access
		if strings.HasPrefix(suffix, "output.") {
			fieldPath := strings.TrimPrefix(suffix, "output.")
			return fmt.Sprintf(
				"{{=sprig.fromJson(tasks['%s'].outputs.%s['%s']).parts[0].data.%s}}",
				taskName, outputKind, agentTaskArtifactParam, fieldPath,
			)
		}

		return "{{" + expr + "}}"
	}

	// Unknown expression - pass through
	return "{{" + expr + "}}"
}

// translateAgentMessage returns a copy of the message with DSL expressions
// in text parts translated to Argo workflow syntax.
func translateAgentMessage(msg *agentv1alpha1.AgentMessage) *agentv1alpha1.AgentMessage {
	translated := msg.DeepCopy()
	for i := range translated.Parts {
		if translated.Parts[i].Text != nil {
			translated.Parts[i].Text.Text = TranslateExpressions(translated.Parts[i].Text.Text)
		}
	}
	return translated
}

// TranslateExpressions replaces all DSL expressions in a string with Argo-compatible references.
// Exported for testing.
func TranslateExpressions(input string) string {
	return TranslateExpressionsWithMode(input, false)
}

// TranslateExpressionsWithMode replaces DSL expressions with Argo references.
// When artifactIO is true, task output references use outputs.artifacts instead of outputs.parameters.
func TranslateExpressionsWithMode(input string, artifactIO bool) string {
	return expressionRe.ReplaceAllStringFunc(input, func(match string) string {
		inner := expressionRe.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		return translateExpressionWithMode(strings.TrimSpace(inner[1]), artifactIO)
	})
}
