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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// cmpOpts are shared options for cmp.Diff comparisons on Argo Workflow objects.
var cmpOpts = cmp.Options{
	// time.Time has unexported fields; compare via Equal method.
	cmp.Comparer(func(x, y time.Time) bool { return x.Equal(y) }),
	// json.RawMessage: compare semantically (unmarshal both sides).
	cmp.Transformer("ParseJSON", func(b json.RawMessage) (v interface{}) {
		_ = json.Unmarshal(b, &v)
		return v
	}),
}

// --- Test helpers ---

// textMessage creates an AgentMessage with a single text part.
func textMessage(text string) agentv1alpha1.AgentMessage {
	return agentv1alpha1.AgentMessage{
		Parts: []agentv1alpha1.MessagePart{
			{Text: &agentv1alpha1.TextPart{Text: text}},
		},
	}
}

// jsonRaw creates an apiextensionsv1.JSON from a raw JSON string.
func jsonRaw(s string) *apiextensionsv1.JSON {
	return &apiextensionsv1.JSON{Raw: []byte(s)}
}

// int32Ptr returns a pointer to an int32.
func int32Ptr(v int32) *int32 { return &v }

// wantWorkflow builds the expected compiled Argo Workflow with standard metadata.
func wantWorkflow(name, namespace string, templates []wfv1.Template, opts ...func(*wfv1.Workflow)) *wfv1.Workflow {
	wf := &wfv1.Workflow{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "Workflow",
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name + "-",
			Namespace:    namespace,
			Labels: map[string]string{
				"agent.flokoa.ai/agentworkflow-name": name,
				"app.kubernetes.io/managed-by":       "flokoa-operator",
			},
		},
		Spec: wfv1.WorkflowSpec{
			Entrypoint: "main",
			Templates:  templates,
		},
	}
	for _, opt := range opts {
		opt(wf)
	}
	return wf
}

// withParams adds workflow-level argument parameters.
func withParams(params ...wfv1.Parameter) func(*wfv1.Workflow) {
	return func(wf *wfv1.Workflow) {
		wf.Spec.Arguments.Parameters = params
	}
}

// withTimeout sets the workflow-level active deadline.
func withTimeout(seconds int64) func(*wfv1.Workflow) {
	return func(wf *wfv1.Workflow) {
		wf.Spec.ActiveDeadlineSeconds = &seconds
	}
}

// dagTmpl creates the "main" DAG entrypoint template.
func dagTmpl(tasks ...wfv1.DAGTask) wfv1.Template {
	return wfv1.Template{
		Name: "main",
		DAG:  &wfv1.DAGTemplate{Tasks: tasks},
	}
}

// containerOutputs returns the standard output parameters for agent task containers.
func containerOutputs() wfv1.Outputs {
	return wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{Name: "result", ValueFrom: &wfv1.ValueFrom{Path: "/tmp/output"}},
		},
	}
}

// pluginOutputs returns the standard output parameters for A2A plugin templates.
func pluginOutputs() wfv1.Outputs {
	return wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{Name: "result"},
			{Name: "taskResponse"},
		},
	}
}

// taskConfigEnv serialises an agentTaskConfig into the FLOKOA_TASK_CONFIG env var.
func taskConfigEnv(cfg agentTaskConfig) corev1.EnvVar {
	data, _ := json.Marshal(cfg)
	return corev1.EnvVar{Name: taskConfigEnvVar, Value: string(data)}
}

// makePlugin builds a wfv1.Plugin from an A2A spec map.
func makePlugin(a2aSpec map[string]interface{}) *wfv1.Plugin {
	pluginData := map[string]interface{}{"a2a": a2aSpec}
	b, _ := json.Marshal(pluginData)
	p := &wfv1.Plugin{}
	p.Value = b
	return p
}

// pluginMessage builds the message structure that buildPluginMessage produces for a single text part.
func pluginTextMessage(text string) map[string]interface{} {
	return map[string]interface{}{
		"parts": []map[string]interface{}{
			{"text": map[string]interface{}{"text": text}},
		},
	}
}

// assertDiff fails the test if want and got differ.
func assertDiff(t *testing.T, want, got *wfv1.Workflow) {
	t.Helper()
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("compiled workflow mismatch (-want +got):\n%s", diff)
	}
}

// --- Tests ---

func TestCompileToArgoWorkflow_SimpleSequential(t *testing.T) {
	timeout := metav1.Duration{Duration: 10 * time.Minute}
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workflow", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{
				{Name: "topic", Value: "transformers"},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "research",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "researcher-agent",
						Message: textMessage("Find papers on {{params.topic}}"),
					},
					Timeout: &timeout,
				},
				{
					Name: "summarize",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "summarizer-agent",
						Message: textMessage("Summarize: {{tasks.research.output}}"),
					},
					DependsOn: []string{"research"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("test-workflow", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "research", Template: "research"},
				wfv1.DAGTask{Name: "summarize", Template: "summarize", Dependencies: []string{"research"}},
			),
			{
				Name:                  "research",
				Plugin:                makePlugin(map[string]interface{}{"agent": "researcher-agent", "message": pluginTextMessage("Find papers on {{params.topic}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				ActiveDeadlineSeconds: &intstr.IntOrString{Type: intstr.Int, IntVal: 600},
				Outputs:               pluginOutputs(),
			},
			{
				Name:    "summarize",
				Plugin:  makePlugin(map[string]interface{}{"agent": "summarizer-agent", "message": pluginTextMessage("Summarize: {{tasks.research.output}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
		},
		withParams(wfv1.Parameter{Name: "topic", Value: wfv1.AnyStringPtr("transformers")}),
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTemplate(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "call-agent",
					Agent: &agentv1alpha1.AgentCall{
						Name:      "my-agent",
						Namespace: "agents",
						Message:   textMessage("Hello agent"),
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "call-agent", Template: "call-agent"}),
			{
				Name: "call-agent",
				Plugin: makePlugin(map[string]interface{}{
					"agent":       "my-agent",
					"namespace":   "agents",
					"message":     pluginTextMessage("Hello agent"),
					"traceparent": "{{workflow.parameters._flokoa_traceparent}}",
				}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTemplateMultiPart(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "multi-part",
					Agent: &agentv1alpha1.AgentCall{
						Name: "my-agent",
						Message: agentv1alpha1.AgentMessage{
							Role: agentv1alpha1.MessageRoleUser,
							Parts: []agentv1alpha1.MessagePart{
								{Text: &agentv1alpha1.TextPart{Text: "Analyze this data"}},
								{File: &agentv1alpha1.FilePart{
									File: agentv1alpha1.FileContent{
										URI:      "s3://bucket/data.csv",
										MimeType: "text/csv",
										Name:     "data.csv",
									},
								}},
							},
							ContextID: "ctx-123",
						},
						Config: &agentv1alpha1.MessageSendConfig{
							AcceptedOutputModes: []string{"text", "application/json"},
							Blocking:            boolPtr(true),
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "multi-part", Template: "multi-part"}),
			{
				Name: "multi-part",
				Plugin: makePlugin(map[string]interface{}{
					"agent":       "my-agent",
					"traceparent": "{{workflow.parameters._flokoa_traceparent}}",
					"message": map[string]interface{}{
						"role":      "user",
						"contextId": "ctx-123",
						"parts": []map[string]interface{}{
							{"text": map[string]interface{}{"text": "Analyze this data"}},
							{"file": map[string]interface{}{
								"file": map[string]interface{}{
									"name":     "data.csv",
									"mimeType": "text/csv",
									"bytes":    "",
									"uri":      "s3://bucket/data.csv",
								},
							}},
						},
					},
					"config": map[string]interface{}{
						"acceptedOutputModes": []string{"text", "application/json"},
						"blocking":            true,
					},
				}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskRun(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "run-test", Namespace: "ml"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "research",
					AgentTask: &agentv1alpha1.AgentTask{
						Type: agentv1alpha1.MarvinTaskTypeRun,
						Instruction: &agentv1alpha1.InstructionEntry{
							Template: "Research {{params.topic}}",
						},
						Context: map[string]string{
							"domain": "machine learning",
							"prev":   "{{tasks.prep.output}}",
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("run-test", "ml",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "research", Template: "research"}),
			{
				Name: "research",
				Container: &corev1.Container{
					Image:   defaultManagedTaskImage,
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:         "run",
							Instructions: "Research {{workflow.parameters.topic}}",
							Context: map[string]string{
								"domain": "machine learning",
								"prev":   "{{tasks.prep.outputs.parameters.result}}",
							},
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskClassify(t *testing.T) {
	multiLabel := true
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "classify-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "classify",
					AgentTask: &agentv1alpha1.AgentTask{
						Type:       agentv1alpha1.MarvinTaskTypeClassify,
						Input:      "This product exceeded all my expectations!",
						Labels:     []string{"positive", "negative", "neutral"},
						MultiLabel: &multiLabel,
						Instruction: &agentv1alpha1.InstructionEntry{
							Template: "Classify sentiment",
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("classify-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "classify", Template: "classify"}),
			{
				Name: "classify",
				Container: &corev1.Container{
					Image:   defaultManagedTaskImage,
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:         "classify",
							Instructions: "Classify sentiment",
							Input:        "This product exceeded all my expectations!",
							Labels:       []string{"positive", "negative", "neutral"},
							MultiLabel:   &multiLabel,
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskExtract(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "extract-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "extract-names",
					AgentTask: &agentv1alpha1.AgentTask{
						Type:  agentv1alpha1.MarvinTaskTypeExtract,
						Input: "Alice and Bob went to the store.",
						ResultType: &agentv1alpha1.StructuredIOSchema{
							Name:        "PersonName",
							Description: "A person's name",
							JSONSchema:  jsonRaw(`{"type":"string"}`),
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("extract-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "extract-names", Template: "extract-names"}),
			{
				Name: "extract-names",
				Container: &corev1.Container{
					Image:   defaultManagedTaskImage,
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:  "extract",
							Input: "Alice and Bob went to the store.",
							ResultType: &agentTaskResultType{
								Name:        "PersonName",
								Description: "A person's name",
								JSONSchema:  json.RawMessage(`{"type":"string"}`),
							},
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskCast(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "cast-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "cast-data",
					AgentTask: &agentv1alpha1.AgentTask{
						Type:  agentv1alpha1.MarvinTaskTypeCast,
						Input: "{{tasks.source.output}}",
						ResultType: &agentv1alpha1.StructuredIOSchema{
							Name:        "Summary",
							Description: "A structured summary",
							JSONSchema:  jsonRaw(`{"type":"object","properties":{"title":{"type":"string"}}}`),
						},
						Instruction: &agentv1alpha1.InstructionEntry{
							Template: "Transform to summary format",
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("cast-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "cast-data", Template: "cast-data"}),
			{
				Name: "cast-data",
				Container: &corev1.Container{
					Image:   defaultManagedTaskImage,
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:         "cast",
							Instructions: "Transform to summary format",
							Input:        "{{tasks.source.outputs.parameters.result}}",
							ResultType: &agentTaskResultType{
								Name:        "Summary",
								Description: "A structured summary",
								JSONSchema:  json.RawMessage(`{"type":"object","properties":{"title":{"type":"string"}}}`),
							},
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskGenerate(t *testing.T) {
	count := int32(10)
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "gen-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "generate-examples",
					AgentTask: &agentv1alpha1.AgentTask{
						Type: agentv1alpha1.MarvinTaskTypeGenerate,
						ResultType: &agentv1alpha1.StructuredIOSchema{
							Name:        "TestCase",
							Description: "A test case",
							JSONSchema:  jsonRaw(`{"type":"object","properties":{"input":{"type":"string"},"expected":{"type":"string"}}}`),
						},
						Count: &count,
						Instruction: &agentv1alpha1.InstructionEntry{
							Template: "Generate diverse test cases",
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("gen-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "generate-examples", Template: "generate-examples"}),
			{
				Name: "generate-examples",
				Container: &corev1.Container{
					Image:   defaultManagedTaskImage,
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:         "generate",
							Instructions: "Generate diverse test cases",
							Count:        &count,
							ResultType: &agentTaskResultType{
								Name:        "TestCase",
								Description: "A test case",
								JSONSchema:  json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"},"expected":{"type":"string"}}}`),
							},
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskWithInlineAgent(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "agent-run",
					AgentTask: &agentv1alpha1.AgentTask{
						Type: agentv1alpha1.MarvinTaskTypeRun,
						Instruction: &agentv1alpha1.InstructionEntry{
							Template: "Do analysis",
						},
						Agent: &agentv1alpha1.MarvinAgentSpec{
							Name:         "analyst",
							Instructions: "You are a data analyst",
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("agent-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "agent-run", Template: "agent-run"}),
			{
				Name: "agent-run",
				Container: &corev1.Container{
					Image:   defaultManagedTaskImage,
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:         "run",
							Instructions: "Do analysis",
							Agent: &agentTaskAgentConfig{
								Name:         "analyst",
								Instructions: "You are a data analyst",
							},
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskCustomImage(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "custom-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "custom",
					AgentTask: &agentv1alpha1.AgentTask{
						Type:  agentv1alpha1.MarvinTaskTypeRun,
						Image: "my-registry/custom-task:v1",
						Instruction: &agentv1alpha1.InstructionEntry{
							Template: "Run task",
						},
						Env: []corev1.EnvVar{
							{Name: "CUSTOM_VAR", Value: "custom-value"},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("custom-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "custom", Template: "custom"}),
			{
				Name: "custom",
				Container: &corev1.Container{
					Image:   "my-registry/custom-task:v1",
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:         "run",
							Instructions: "Run task",
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
						{Name: "CUSTOM_VAR", Value: "custom-value"},
					},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskWithResolvedVolumes(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "resolved-test", Namespace: "production"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "with-model",
					AgentTask: &agentv1alpha1.AgentTask{
						Type: agentv1alpha1.MarvinTaskTypeRun,
						Instruction: &agentv1alpha1.InstructionEntry{
							Template: "Run with model",
						},
					},
				},
			},
		},
	}

	resolved := map[string]*resolvedAgentTaskInfo{
		"with-model": {
			modelInfo: &resolvedModelInfo{
				configMapName: "wf-task-model-cm",
				envVars: []corev1.EnvVar{
					{Name: "MODEL_PROVIDER", Value: "openai"},
				},
				secretEnvVars: []corev1.EnvVar{
					{
						Name: "OPENAI_API_KEY",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "openai-secret"},
								Key:                  "api-key",
							},
						},
					},
				},
			},
			toolConfigMaps: []toolConfigMapInfo{
				{toolName: "web-search", configMapName: "tool-web-search-cm"},
				{toolName: "calculator", configMapName: "tool-calculator-cm"},
			},
			instructionConfigMapName: "wf-task-instruction-cm",
		},
	}

	got, err := compileToArgoWorkflow(awf, resolved, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("resolved-test", "production",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "with-model", Template: "with-model"}),
			{
				Name: "with-model",
				Container: &corev1.Container{
					Image:   defaultManagedTaskImage,
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:         "run",
							Instructions: "Run with model",
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
						{Name: "MODEL_PROVIDER", Value: "openai"},
						{
							Name: "OPENAI_API_KEY",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "openai-secret"},
									Key:                  "api-key",
								},
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "model-config", MountPath: agentTaskModelMountPath, SubPath: "model.json", ReadOnly: true},
						{Name: "tool-web-search", MountPath: agentTaskToolsMountPath + "/web-search.json", SubPath: "spec.json", ReadOnly: true},
						{Name: "tool-calculator", MountPath: agentTaskToolsMountPath + "/calculator.json", SubPath: "spec.json", ReadOnly: true},
						{Name: "instruction", MountPath: agentTaskInstructionMountPath, SubPath: "instruction.txt", ReadOnly: true},
					},
				},
				Volumes: []corev1.Volume{
					{Name: "model-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "wf-task-model-cm"}}}},
					{Name: "tool-web-search", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "tool-web-search-cm"}}}},
					{Name: "tool-calculator", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "tool-calculator-cm"}}}},
					{Name: "instruction", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "wf-task-instruction-cm"}}}},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTaskModelOnly(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "model-only", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task",
					AgentTask: &agentv1alpha1.AgentTask{
						Type:        agentv1alpha1.MarvinTaskTypeRun,
						Instruction: &agentv1alpha1.InstructionEntry{Template: "hello"},
					},
				},
			},
		},
	}

	resolved := map[string]*resolvedAgentTaskInfo{
		"task": {
			modelInfo: &resolvedModelInfo{configMapName: "model-cm"},
		},
	}

	got, err := compileToArgoWorkflow(awf, resolved, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("model-only", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "task", Template: "task"}),
			{
				Name: "task",
				Container: &corev1.Container{
					Image:   defaultManagedTaskImage,
					Command: []string{"python", "-m", "flokoa_managed_task"},
					Env: []corev1.EnvVar{
						taskConfigEnv(agentTaskConfig{
							Type:         "run",
							Instructions: "hello",
						}),
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "model-config", MountPath: agentTaskModelMountPath, SubPath: "model.json", ReadOnly: true},
					},
				},
				Volumes: []corev1.Volume{
					{Name: "model-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "model-cm"}}}},
				},
				Outputs: containerOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_RetryStrategy(t *testing.T) {
	factor := int32(2)
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "retry-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			RetryStrategy: &agentv1alpha1.WorkflowRetryStrategy{
				Limit: 3,
				Backoff: &agentv1alpha1.WorkflowBackoff{
					Duration: "30s",
					Factor:   &factor,
				},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("hello")}},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	limit := intstr.FromInt32(3)
	backoffFactor := intstr.FromInt32(2)
	want := wantWorkflow("retry-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "task1", Template: "task1"}),
			{
				Name:    "task1",
				Plugin:  makePlugin(map[string]interface{}{"agent": "agent1", "message": pluginTextMessage("hello"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
				RetryStrategy: &wfv1.RetryStrategy{
					Limit: &limit,
					Backoff: &wfv1.Backoff{
						Duration: "30s",
						Factor:   &backoffFactor,
					},
				},
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_WorkflowTimeout(t *testing.T) {
	timeout := metav1.Duration{Duration: 1 * time.Hour}
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "timeout-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Timeout: &timeout,
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("hello")}},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("timeout-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "task1", Template: "task1"}),
			{
				Name:    "task1",
				Plugin:  makePlugin(map[string]interface{}{"agent": "agent1", "message": pluginTextMessage("hello"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
		},
		withTimeout(3600),
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_FanOutFanIn(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "fan-out", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "a", Agent: &agentv1alpha1.AgentCall{Name: "agent-a", Message: textMessage("task a")}},
				{Name: "b", Agent: &agentv1alpha1.AgentCall{Name: "agent-b", Message: textMessage("task b")}},
				{Name: "c", Agent: &agentv1alpha1.AgentCall{Name: "agent-c", Message: textMessage("task c")}},
				{
					Name:      "merge",
					Agent:     &agentv1alpha1.AgentCall{Name: "agent-merge", Message: textMessage("merge results")},
					DependsOn: []string{"a", "b", "c"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("fan-out", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "a", Template: "a"},
				wfv1.DAGTask{Name: "b", Template: "b"},
				wfv1.DAGTask{Name: "c", Template: "c"},
				wfv1.DAGTask{Name: "merge", Template: "merge", Dependencies: []string{"a", "b", "c"}},
			),
			{Name: "a", Plugin: makePlugin(map[string]interface{}{"agent": "agent-a", "message": pluginTextMessage("task a"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "b", Plugin: makePlugin(map[string]interface{}{"agent": "agent-b", "message": pluginTextMessage("task b"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "c", Plugin: makePlugin(map[string]interface{}{"agent": "agent-c", "message": pluginTextMessage("task c"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "merge", Plugin: makePlugin(map[string]interface{}{"agent": "agent-merge", "message": pluginTextMessage("merge results"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
		},
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_Condition(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "cond-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "classify", Agent: &agentv1alpha1.AgentCall{Name: "classifier", Message: textMessage("classify")}},
				{
					Name:      "technical",
					Agent:     &agentv1alpha1.AgentCall{Name: "tech-support", Message: textMessage("help")},
					DependsOn: []string{"classify"},
					Condition: "{{tasks.classify.output.category}} == technical",
				},
			},
		},
	}

	got, err := compileToArgoWorkflow(awf, nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflow("cond-test", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "classify", Template: "classify"},
				wfv1.DAGTask{
					Name:         "technical",
					Template:     "technical",
					Dependencies: []string{"classify"},
					When:         "{{tasks.classify.outputs.parameters.result}} == technical",
				},
			),
			{Name: "classify", Plugin: makePlugin(map[string]interface{}{"agent": "classifier", "message": pluginTextMessage("classify"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "technical", Plugin: makePlugin(map[string]interface{}{"agent": "tech-support", "message": pluginTextMessage("help"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
		},
	)

	assertDiff(t, want, got)
}

func TestTranslateExpressions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "params reference",
			input:    "Find papers on {{params.topic}}",
			expected: "Find papers on {{workflow.parameters.topic}}",
		},
		{
			name:     "task output reference",
			input:    "Summarize: {{tasks.research.output}}",
			expected: "Summarize: {{tasks.research.outputs.parameters.result}}",
		},
		{
			name:     "task output field access",
			input:    "Category: {{tasks.classify.output.category}}",
			expected: "Category: {{tasks.classify.outputs.parameters.result}}",
		},
		{
			name:     "task response reference",
			input:    "Response: {{tasks.call.taskResponse}}",
			expected: "Response: {{tasks.call.outputs.parameters.taskResponse}}",
		},
		{
			name:     "multiple expressions",
			input:    "Tech: {{tasks.tech.output}} Cost: {{tasks.cost.output}}",
			expected: "Tech: {{tasks.tech.outputs.parameters.result}} Cost: {{tasks.cost.outputs.parameters.result}}",
		},
		{
			name:     "no expressions",
			input:    "plain text message",
			expected: "plain text message",
		},
		{
			name:     "expression with spaces",
			input:    "{{ params.topic }}",
			expected: "{{workflow.parameters.topic}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TranslateExpressions(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
