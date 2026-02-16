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
)

const (
	// defaultManagedTaskImage is the default flokoa runtime image for Marvin agent tasks.
	defaultManagedTaskImage = "ghcr.io/danielnyari/flokoa/managed-task:latest"

	// dagEntrypointName is the name of the DAG entrypoint template.
	dagEntrypointName = "main"

	// a2aPluginKey is the key used in the Argo plugin spec for A2A tasks.
	a2aPluginKey = "a2a"

	// agentTaskOutputParam is the output parameter name for task results.
	agentTaskOutputParam = "result"

	// agentTaskResponseParam is the output parameter name for the full A2A response.
	agentTaskResponseParam = "taskResponse"

	// taskConfigEnvVar is the environment variable name for the serialized task config.
	taskConfigEnvVar = "FLOKOA_TASK_CONFIG"

	// Mount paths for resolved ConfigMaps in agent task containers.
	agentTaskModelMountPath       = "/etc/flokoa/model.json"
	agentTaskToolsMountPath       = "/etc/flokoa/tools"
	agentTaskInstructionMountPath = "/etc/flokoa/instruction.txt"
)

// expressionRe matches {{...}} template expressions in DSL fields.
var expressionRe = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// compileToArgoWorkflow translates an AgentWorkflow DSL into an Argo Workflow CR.
// resolvedTasks contains pre-resolved Model/Tool ConfigMap info for agentTask tasks, keyed by task name.
func compileToArgoWorkflow(awf *agentv1alpha1.AgentWorkflow, resolvedTasks map[string]*resolvedAgentTaskInfo) (*wfv1.Workflow, error) {
	wf := &wfv1.Workflow{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Workflow",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: awf.Name + "-",
			Namespace:    awf.Namespace,
			Labels: map[string]string{
				"agent.flokoa.ai/agentworkflow-name": awf.Name,
				"app.kubernetes.io/managed-by":       "flokoa-operator",
			},
		},
		Spec: wfv1.WorkflowSpec{
			Entrypoint: dagEntrypointName,
		},
	}

	// Workflow-level parameters
	if len(awf.Spec.Params) > 0 {
		for _, p := range awf.Spec.Params {
			param := wfv1.Parameter{
				Name:  p.Name,
				Value: wfv1.AnyStringPtr(p.Value),
			}
			wf.Spec.Arguments.Parameters = append(wf.Spec.Arguments.Parameters, param)
		}
	}

	// Workflow-level timeout
	if awf.Spec.Timeout != nil {
		seconds := int64(awf.Spec.Timeout.Duration.Seconds())
		wf.Spec.ActiveDeadlineSeconds = &seconds
	}

	// Build templates and DAG tasks
	var dagTasks []wfv1.DAGTask
	var templates []wfv1.Template

	for _, task := range awf.Spec.Tasks {
		templateName := task.Name

		// Build the template for this task
		tmpl, err := buildTemplate(awf, task, resolvedTasks[task.Name])
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

	// Create the DAG entrypoint template
	dagTemplate := wfv1.Template{
		Name: dagEntrypointName,
		DAG: &wfv1.DAGTemplate{
			Tasks: dagTasks,
		},
	}
	templates = append([]wfv1.Template{dagTemplate}, templates...)

	wf.Spec.Templates = templates

	return wf, nil
}

// buildTemplate creates an Argo template for a single workflow task.
func buildTemplate(awf *agentv1alpha1.AgentWorkflow, task agentv1alpha1.WorkflowTask, resolved *resolvedAgentTaskInfo) (*wfv1.Template, error) {
	tmpl := &wfv1.Template{
		Name: task.Name,
	}

	// Per-task timeout
	if task.Timeout != nil {
		seconds := int64(task.Timeout.Duration.Seconds())
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
	case task.AgentTask != nil:
		if err := buildAgentTaskTemplate(tmpl, task.AgentTask, resolved); err != nil {
			return nil, err
		}
	case len(task.Switch) > 0:
		// Switch tasks are a no-op routing template; branching is in DAG when expressions.
		tmpl.Script = &wfv1.ScriptTemplate{
			Container: corev1.Container{
				Image:   "alpine:3.18",
				Command: []string{"echo"},
				Args:    []string{"switch-router"},
			},
		}
	}

	return tmpl, nil
}

// buildAgentTemplate populates a template with the A2A plugin spec for calling a deployed agent.
func buildAgentTemplate(tmpl *wfv1.Template, agent *agentv1alpha1.AgentCall) error {
	a2aSpec := map[string]interface{}{
		"agent":   agent.Name,
		"message": buildPluginMessage(&agent.Message),
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

	// Define output parameters for A2A tasks
	tmpl.Outputs = wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{Name: agentTaskOutputParam},
			{Name: agentTaskResponseParam},
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

// resolvedAgentTaskInfo holds pre-resolved ConfigMap info for a single agentTask.
// Populated by the AgentWorkflow controller before compilation.
type resolvedAgentTaskInfo struct {
	// modelInfo is the resolved Model/ModelProvider configuration (ConfigMap name, env vars, secrets).
	modelInfo *resolvedModelInfo
	// toolConfigMaps contains resolved tool ConfigMap info for volume mounting.
	toolConfigMaps []toolConfigMapInfo
	// agentModelInfo is the resolved model for the inline agent (if different from task-level).
	agentModelInfo *resolvedModelInfo
	// instructionConfigMapName is the name of the ConfigMap for an instruction ref.
	instructionConfigMapName string
}

// agentTaskConfig is the JSON structure serialized into the FLOKOA_TASK_CONFIG env var.
// It contains all inline configuration for the Marvin task runtime.
type agentTaskConfig struct {
	Type         string                `json:"type"`
	Instructions string                `json:"instructions,omitempty"`
	Input        string                `json:"input,omitempty"`
	ResultType   *agentTaskResultType  `json:"resultType,omitempty"`
	Labels       []string              `json:"labels,omitempty"`
	MultiLabel   *bool                 `json:"multiLabel,omitempty"`
	Count        *int32                `json:"count,omitempty"`
	Context      map[string]string     `json:"context,omitempty"`
	Agent        *agentTaskAgentConfig `json:"agent,omitempty"`
}

type agentTaskResultType struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	JSONSchema  json.RawMessage `json:"jsonSchema"`
}

type agentTaskAgentConfig struct {
	Name         string `json:"name"`
	Instructions string `json:"instructions,omitempty"`
}

// buildAgentTaskTemplate populates a template with a container spec for a Marvin agent task.
func buildAgentTaskTemplate(tmpl *wfv1.Template, agentTask *agentv1alpha1.AgentTask, resolved *resolvedAgentTaskInfo) error {
	image := agentTask.Image
	if image == "" {
		image = defaultManagedTaskImage
	}

	// Build the task config JSON
	cfg := agentTaskConfig{
		Type: string(agentTask.Type),
	}

	// Instruction (inline only — instructionRef is handled via volume mount)
	if agentTask.Instruction != nil && agentTask.Instruction.Template != "" {
		cfg.Instructions = TranslateExpressions(agentTask.Instruction.Template)
	}

	// Input with expression translation
	if agentTask.Input != "" {
		cfg.Input = TranslateExpressions(agentTask.Input)
	}

	// ResultType
	if agentTask.ResultType != nil {
		rt := &agentTaskResultType{
			Name:        agentTask.ResultType.Name,
			Description: agentTask.ResultType.Description,
		}
		if agentTask.ResultType.JSONSchema != nil {
			rt.JSONSchema = agentTask.ResultType.JSONSchema.Raw
		}
		cfg.ResultType = rt
	}

	// Classify-specific fields
	if len(agentTask.Labels) > 0 {
		cfg.Labels = agentTask.Labels
	}
	if agentTask.MultiLabel != nil {
		cfg.MultiLabel = agentTask.MultiLabel
	}

	// Generate-specific fields
	if agentTask.Count != nil {
		cfg.Count = agentTask.Count
	}

	// Context with expression translation
	if len(agentTask.Context) > 0 {
		cfg.Context = make(map[string]string, len(agentTask.Context))
		for k, v := range agentTask.Context {
			cfg.Context[k] = TranslateExpressions(v)
		}
	}

	// Inline agent config
	if agentTask.Agent != nil {
		cfg.Agent = &agentTaskAgentConfig{
			Name:         agentTask.Agent.Name,
			Instructions: agentTask.Agent.Instructions,
		}
	}

	taskConfigJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal agent task config: %w", err)
	}

	container := corev1.Container{
		Image:   image,
		Command: []string{"python", "-m", "flokoa_managed_task"},
		Env: append([]corev1.EnvVar{
			{
				Name:  taskConfigEnvVar,
				Value: string(taskConfigJSON),
			},
		}, agentTask.Env...),
	}

	// Resource requirements
	if agentTask.Resources != nil {
		container.Resources = *agentTask.Resources
	}

	// Volumes and volume mounts from resolved info
	var volumes []corev1.Volume

	if resolved != nil {
		// Mount model ConfigMap
		if resolved.modelInfo != nil && resolved.modelInfo.configMapName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "model-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: resolved.modelInfo.configMapName},
					},
				},
			})
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      "model-config",
				MountPath: agentTaskModelMountPath,
				SubPath:   "model.json",
				ReadOnly:  true,
			})
			// Add non-secret env vars from model provider
			container.Env = append(container.Env, resolved.modelInfo.envVars...)
			// Add secret env vars from model provider
			container.Env = append(container.Env, resolved.modelInfo.secretEnvVars...)
		}

		// Mount tool ConfigMaps
		for _, toolCM := range resolved.toolConfigMaps {
			volumeName := fmt.Sprintf("tool-%s", toolCM.toolName)
			volumes = append(volumes, corev1.Volume{
				Name: volumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: toolCM.configMapName},
					},
				},
			})
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      volumeName,
				MountPath: fmt.Sprintf("%s/%s.json", agentTaskToolsMountPath, toolCM.toolName),
				SubPath:   "spec.json",
				ReadOnly:  true,
			})
		}

		// Mount instruction ConfigMap (for instructionRef)
		if resolved.instructionConfigMapName != "" {
			volumes = append(volumes, corev1.Volume{
				Name: "instruction",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: resolved.instructionConfigMapName},
					},
				},
			})
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
				Name:      "instruction",
				MountPath: agentTaskInstructionMountPath,
				SubPath:   "instruction.txt",
				ReadOnly:  true,
			})
		}
	}

	tmpl.Container = &container
	if len(volumes) > 0 {
		tmpl.Volumes = volumes
	}

	// Output parameter: the container writes its result to /tmp/output
	tmpl.Outputs = wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{
				Name: agentTaskOutputParam,
				ValueFrom: &wfv1.ValueFrom{
					Path: "/tmp/output",
				},
			},
		},
	}

	return nil
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
	// params.<name> -> {{workflow.parameters.<name>}}
	if strings.HasPrefix(expr, "params.") {
		paramName := strings.TrimPrefix(expr, "params.")
		return fmt.Sprintf("{{workflow.parameters.%s}}", paramName)
	}

	// tasks.<name>.output -> {{tasks.<name>.outputs.parameters.result}}
	// tasks.<name>.output.<field> -> {{tasks.<name>.outputs.parameters.result}}
	// tasks.<name>.taskResponse -> {{tasks.<name>.outputs.parameters.taskResponse}}
	if strings.HasPrefix(expr, "tasks.") {
		rest := strings.TrimPrefix(expr, "tasks.")
		parts := strings.SplitN(rest, ".", 2)
		if len(parts) < 2 {
			return "{{" + expr + "}}"
		}
		taskName := parts[0]
		suffix := parts[1]

		if suffix == "taskResponse" {
			return fmt.Sprintf("{{tasks.%s.outputs.parameters.%s}}", taskName, agentTaskResponseParam)
		}
		// output or output.<field> both map to the result parameter
		return fmt.Sprintf("{{tasks.%s.outputs.parameters.%s}}", taskName, agentTaskOutputParam)
	}

	// Unknown expression - pass through
	return "{{" + expr + "}}"
}

// TranslateExpressions replaces all DSL expressions in a string with Argo-compatible references.
// Exported for testing.
func TranslateExpressions(input string) string {
	return expressionRe.ReplaceAllStringFunc(input, func(match string) string {
		inner := expressionRe.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		return translateExpression(strings.TrimSpace(inner[1]))
	})
}
