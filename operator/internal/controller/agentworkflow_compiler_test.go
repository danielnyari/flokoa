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
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const taskNameRoute = "route"

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

// textMessage creates an AgentMessage pointer with a single text part.
func textMessage(text string) *agentv1alpha1.AgentMessage {
	return &agentv1alpha1.AgentMessage{
		Parts: []agentv1alpha1.MessagePart{
			{Text: &agentv1alpha1.TextPart{Text: text}},
		},
	}
}

// wantWorkflowTemplate builds the expected compiled Argo WorkflowTemplate with standard metadata.
// It always prepends the _flokoa_traceparent parameter to match compiler behavior.
//
//nolint:unparam // namespace is kept as a parameter for symmetry with the compiler API.
func wantWorkflowTemplate(name, namespace string, templates []wfv1.Template, opts ...func(*wfv1.WorkflowTemplate)) *wfv1.WorkflowTemplate {
	wft := &wfv1.WorkflowTemplate{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "WorkflowTemplate",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"agent.flokoa.ai/agentworkflow-name": name,
				"app.kubernetes.io/managed-by":       "flokoa-operator",
			},
		},
		Spec: wfv1.WorkflowSpec{
			Entrypoint:                   "main",
			ServiceAccountName:           "flokoa-workflow",
			AutomountServiceAccountToken: boolPtr(true),
			Templates:                    templates,
			Arguments: wfv1.Arguments{
				Parameters: []wfv1.Parameter{
					{Name: "_flokoa_traceparent"},
				},
			},
		},
	}
	for _, opt := range opts {
		opt(wft)
	}
	return wft
}

// withParams adds workflow-level argument parameters after the traceparent parameter.
func withParams(params ...wfv1.Parameter) func(*wfv1.WorkflowTemplate) {
	return func(wft *wfv1.WorkflowTemplate) {
		wft.Spec.Arguments.Parameters = append(wft.Spec.Arguments.Parameters, params...)
	}
}

// withTimeout sets the workflow-level active deadline.
func withTimeout(seconds int64) func(*wfv1.WorkflowTemplate) {
	return func(wft *wfv1.WorkflowTemplate) {
		wft.Spec.ActiveDeadlineSeconds = &seconds
	}
}

// dagTmpl creates the "main" DAG entrypoint template.
func dagTmpl(tasks ...wfv1.DAGTask) wfv1.Template {
	return wfv1.Template{
		Name: "main",
		DAG:  &wfv1.DAGTemplate{Tasks: tasks},
	}
}

// pluginOutputs returns the standard output parameters for A2A plugin templates.
func pluginOutputs() wfv1.Outputs {
	supplied := &wfv1.ValueFrom{Supplied: &wfv1.SuppliedValueFrom{}}
	return wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{Name: "result", ValueFrom: supplied},
			{Name: "artifact", ValueFrom: supplied},
		},
	}
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

// int32Ptr returns a pointer to an int32 value.
func int32Ptr(v int32) *int32 { return &v }

// parseQuantity parses a resource quantity string.
func parseQuantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

// assertDiff fails the test if want and got differ.
// The _flokoa_traceparent default is randomly generated (UUID7-based), so we
// verify it is non-empty and then copy it from got→want before the full diff.
func assertDiff(t *testing.T, want, got *wfv1.WorkflowTemplate) {
	t.Helper()
	if len(got.Spec.Arguments.Parameters) > 0 && got.Spec.Arguments.Parameters[0].Name == traceparentWorkflowParam {
		if got.Spec.Arguments.Parameters[0].Default == nil || string(*got.Spec.Arguments.Parameters[0].Default) == "" {
			t.Error("expected _flokoa_traceparent parameter to have a non-empty default")
		}
		if len(want.Spec.Arguments.Parameters) > 0 && want.Spec.Arguments.Parameters[0].Name == traceparentWorkflowParam {
			want.Spec.Arguments.Parameters[0].Default = got.Spec.Arguments.Parameters[0].Default
		}
	}
	if diff := cmp.Diff(want, got, cmpOpts...); diff != "" {
		t.Errorf("compiled workflow template mismatch (-want +got):\n%s", diff)
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

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("test-workflow", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "research", Template: "research"},
				wfv1.DAGTask{Name: "summarize", Template: "summarize", Dependencies: []string{"research"}},
			),
			{
				Name:                  "research",
				Plugin:                makePlugin(map[string]interface{}{"agent": "researcher-agent", "message": pluginTextMessage("Find papers on {{workflow.parameters.topic}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				ActiveDeadlineSeconds: &intstr.IntOrString{Type: intstr.Int, IntVal: 600},
				Outputs:               pluginOutputs(),
			},
			{
				Name:    "summarize",
				Plugin:  makePlugin(map[string]interface{}{"agent": "summarizer-agent", "message": pluginTextMessage("Summarize: {{tasks.research.outputs.parameters.result}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
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

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("test", "default",
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

func TestCompileToArgoWorkflow_AgentMessageExpressionTranslation(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "expr-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{
				{Name: "question", Description: "The user's question", Value: "What is AI?"},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "ask",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "qa-agent",
						Message: textMessage("Answer this: {{params.question}}"),
					},
				},
				{
					Name: "review",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "reviewer-agent",
						Message: textMessage("Review this answer: {{tasks.ask.output}}"),
					},
					DependsOn: []string{"ask"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("expr-test", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "ask", Template: "ask"},
				wfv1.DAGTask{Name: "review", Template: "review", Dependencies: []string{"ask"}},
			),
			{
				Name:    "ask",
				Plugin:  makePlugin(map[string]interface{}{"agent": "qa-agent", "message": pluginTextMessage("Answer this: {{workflow.parameters.question}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
			{
				Name:    "review",
				Plugin:  makePlugin(map[string]interface{}{"agent": "reviewer-agent", "message": pluginTextMessage("Review this answer: {{tasks.ask.outputs.parameters.result}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
		},
		withParams(wfv1.Parameter{Name: "question", Description: wfv1.AnyStringPtr("The user's question"), Value: wfv1.AnyStringPtr("What is AI?")}),
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_AgentTextShorthand(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "text-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{
				{Name: "topic", Value: "AI safety"},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "research",
					Agent: &agentv1alpha1.AgentCall{
						Name: "researcher-agent",
						Text: "Find papers on {{params.topic}}",
					},
				},
				{
					Name: "summarize",
					Agent: &agentv1alpha1.AgentCall{
						Name: "summarizer-agent",
						Text: "Summarize: {{tasks.research.output}}",
					},
					DependsOn: []string{"research"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The text shorthand should produce the same compiled output as the full message form.
	want := wantWorkflowTemplate("text-test", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "research", Template: "research"},
				wfv1.DAGTask{Name: "summarize", Template: "summarize", Dependencies: []string{"research"}},
			),
			{
				Name:    "research",
				Plugin:  makePlugin(map[string]interface{}{"agent": "researcher-agent", "message": pluginTextMessage("Find papers on {{workflow.parameters.topic}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
			{
				Name:    "summarize",
				Plugin:  makePlugin(map[string]interface{}{"agent": "summarizer-agent", "message": pluginTextMessage("Summarize: {{tasks.research.outputs.parameters.result}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
		},
		withParams(wfv1.Parameter{Name: "topic", Value: wfv1.AnyStringPtr("AI safety")}),
	)

	assertDiff(t, want, got)
}

func TestCompileToArgoWorkflow_FieldAccessExpression(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "field-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "research",
					Agent: &agentv1alpha1.AgentCall{
						Name: "researcher-agent",
						Text: "Find papers",
					},
				},
				{
					Name: "extract",
					Agent: &agentv1alpha1.AgentCall{
						Name: "extractor-agent",
						Text: "Extract findings: {{tasks.research.output.findings}}",
					},
					DependsOn: []string{"research"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("field-test", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "research", Template: "research"},
				wfv1.DAGTask{Name: "extract", Template: "extract", Dependencies: []string{"research"}},
			),
			{
				Name:    "research",
				Plugin:  makePlugin(map[string]interface{}{"agent": "researcher-agent", "message": pluginTextMessage("Find papers"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
			{
				Name:    "extract",
				Plugin:  makePlugin(map[string]interface{}{"agent": "extractor-agent", "message": pluginTextMessage("Extract findings: {{=sprig.fromJson(tasks['research'].outputs.parameters['artifact']).parts[0].data.findings}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
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
						Message: &agentv1alpha1.AgentMessage{
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

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("test", "default",
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

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	limit := intstr.FromInt32(3)
	backoffFactor := intstr.FromInt32(2)
	want := wantWorkflowTemplate("retry-test", "default",
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

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("timeout-test", "default",
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

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("fan-out", "default",
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

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("cond-test", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "classify", Template: "classify"},
				wfv1.DAGTask{
					Name:         "technical",
					Template:     "technical",
					Dependencies: []string{"classify"},
					When:         "{{=sprig.fromJson(tasks['classify'].outputs.parameters['artifact']).parts[0].data.category}} == technical",
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
			expected: "Category: {{=sprig.fromJson(tasks['classify'].outputs.parameters['artifact']).parts[0].data.category}}",
		},
		{
			name:     "task artifact reference",
			input:    "Artifact: {{tasks.call.artifact}}",
			expected: "Artifact: {{tasks.call.outputs.parameters.artifact}}",
		},
		{
			name:     "field access with nested path",
			input:    "Value: {{tasks.x.output.a.b}}",
			expected: "Value: {{=sprig.fromJson(tasks['x'].outputs.parameters['artifact']).parts[0].data.a.b}}",
		},
		{
			name:     "field access with hyphenated task name",
			input:    "Field: {{tasks.my-task.output.field}}",
			expected: "Field: {{=sprig.fromJson(tasks['my-task'].outputs.parameters['artifact']).parts[0].data.field}}",
		},
		{
			name:     "Argo expression passthrough",
			input:    "{{=some.expr}}",
			expected: "{{=some.expr}}",
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

// --- Container task tests ---

// containerTaskOutputs returns the standard output parameters for container tasks (with default for artifact).
func containerTaskOutputs() wfv1.Outputs {
	return wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{Name: "result", ValueFrom: &wfv1.ValueFrom{Path: "/tmp/result"}},
			{Name: "artifact", Default: wfv1.AnyStringPtr("{}"), ValueFrom: &wfv1.ValueFrom{Path: "/tmp/artifact"}},
		},
	}
}

// httpOutputs returns the standard output parameters for HTTP tasks.
func httpOutputs() wfv1.Outputs {
	return wfv1.Outputs{
		Parameters: []wfv1.Parameter{
			{Name: "result", ValueFrom: &wfv1.ValueFrom{Expression: "response.body"}},
			{Name: "artifact", ValueFrom: &wfv1.ValueFrom{Expression: "toJson({statusCode: response.statusCode, headers: response.headers, body: response.body})"}},
		},
	}
}

func TestCompile_ContainerSimple(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "container-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "preprocess",
					Container: &agentv1alpha1.ContainerTask{
						Image:   "python:3.12",
						Command: []string{"python", "-c"},
						Args:    []string{"print('hello')"},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("container-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "preprocess", Template: "preprocess"}),
			{
				Name: "preprocess",
				Container: &corev1.Container{
					Image:   "python:3.12",
					Command: []string{"python", "-c"},
					Args:    []string{"print('hello')"},
					Env: []corev1.EnvVar{
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
				},
				Outputs: containerTaskOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_ContainerWithExpressions(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "container-expr", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "input"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "fetch", Agent: &agentv1alpha1.AgentCall{Name: "fetcher", Text: "fetch data"}},
				{
					Name: "process",
					Container: &agentv1alpha1.ContainerTask{
						Image: "python:3.12",
						Env: []corev1.EnvVar{
							{Name: "INPUT", Value: "{{params.input}}"},
							{Name: "DATA", Value: "{{tasks.fetch.output}}"},
						},
					},
					DependsOn: []string{"fetch"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("container-expr", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "fetch", Template: "fetch"},
				wfv1.DAGTask{Name: "process", Template: "process", Dependencies: []string{"fetch"}},
			),
			{
				Name:    "fetch",
				Plugin:  makePlugin(map[string]interface{}{"agent": "fetcher", "message": pluginTextMessage("fetch data"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
			{
				Name: "process",
				Container: &corev1.Container{
					Image: "python:3.12",
					Env: []corev1.EnvVar{
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
						{Name: "INPUT", Value: "{{workflow.parameters.input}}"},
						{Name: "DATA", Value: "{{tasks.fetch.outputs.parameters.result}}"},
					},
				},
				Outputs: containerTaskOutputs(),
			},
		},
		withParams(wfv1.Parameter{Name: "input", Value: wfv1.AnyStringPtr("")}),
	)

	assertDiff(t, want, got)
}

func TestCompile_ContainerWithResources(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "container-resources", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "heavy",
					Container: &agentv1alpha1.ContainerTask{
						Image:      "gpu-runner:latest",
						WorkingDir: "/workspace",
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    *parseQuantity("4"),
								corev1.ResourceMemory: *parseQuantity("8Gi"),
							},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("container-resources", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "heavy", Template: "heavy"}),
			{
				Name: "heavy",
				Container: &corev1.Container{
					Image:      "gpu-runner:latest",
					WorkingDir: "/workspace",
					Env: []corev1.EnvVar{
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    *parseQuantity("4"),
							corev1.ResourceMemory: *parseQuantity("8Gi"),
						},
					},
				},
				Outputs: containerTaskOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

// --- HTTP task tests ---

func TestCompile_HTTPGet(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-get", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL:    "https://api.example.com/data",
						Method: "GET",
						Headers: []agentv1alpha1.HTTPHeader{
							{Name: "Accept", Value: "application/json"},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("http-get", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "fetch", Template: "fetch"}),
			{
				Name: "fetch",
				HTTP: &wfv1.HTTP{
					URL:    "https://api.example.com/data",
					Method: "GET",
					Headers: []wfv1.HTTPHeader{
						{Name: "Accept", Value: "application/json"},
					},
				},
				Outputs: httpOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_HTTPPost(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-post", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "post-data",
					HTTP: &agentv1alpha1.HTTPTask{
						URL:              "https://api.example.com/submit",
						Method:           "POST",
						Body:             `{"key": "value"}`,
						SuccessCondition: "response.statusCode == 201",
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("http-post", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "post-data", Template: "post-data"}),
			{
				Name: "post-data",
				HTTP: &wfv1.HTTP{
					URL:              "https://api.example.com/submit",
					Method:           "POST",
					Body:             `{"key": "value"}`,
					SuccessCondition: "response.statusCode == 201",
				},
				Outputs: httpOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_HTTPSecretHeader(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-secret", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "auth-fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com/secure",
						Headers: []agentv1alpha1.HTTPHeader{
							{
								Name: "Authorization",
								ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "api-creds"},
										Key:                  "token",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("http-secret", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "auth-fetch", Template: "auth-fetch"}),
			{
				Name: "auth-fetch",
				HTTP: &wfv1.HTTP{
					URL:    "https://api.example.com/secure",
					Method: "GET",
					Headers: []wfv1.HTTPHeader{
						{
							Name: "Authorization",
							ValueFrom: &wfv1.HTTPHeaderSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "api-creds"},
									Key:                  "token",
								},
							},
						},
					},
				},
				Outputs: httpOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_HTTPConfigMapHeader(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-cm", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch-with-cm",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com/data",
						Headers: []agentv1alpha1.HTTPHeader{
							{
								Name: "X-Custom-Header",
								ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"},
										Key:                  "header-value",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("http-cm", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "fetch-with-cm", Template: "fetch-with-cm"}),
			{
				Name: "fetch-with-cm",
				HTTP: &wfv1.HTTP{
					URL:    "https://api.example.com/data",
					Method: "GET",
					Headers: []wfv1.HTTPHeader{
						{
							Name:  "X-Custom-Header",
							Value: "{{workflow.parameters._cm_my-config_header-value}}",
						},
					},
				},
				Outputs: httpOutputs(),
			},
		},
		withParams(wfv1.Parameter{
			Name: "_cm_my-config_header-value",
			ValueFrom: &wfv1.ValueFrom{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"},
					Key:                  "header-value",
				},
			},
		}),
	)

	assertDiff(t, want, got)
}

func TestCompile_HTTPExpressions(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-expr", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "base_url", Value: "https://api.example.com"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "prep", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Text: "prepare"}},
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL:    "{{params.base_url}}/data",
						Method: "POST",
						Body:   "{{tasks.prep.output}}",
						Headers: []agentv1alpha1.HTTPHeader{
							{Name: "X-Request-ID", Value: "{{params.base_url}}"},
						},
					},
					DependsOn: []string{"prep"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("http-expr", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "prep", Template: "prep"},
				wfv1.DAGTask{Name: "fetch", Template: "fetch", Dependencies: []string{"prep"}},
			),
			{
				Name:    "prep",
				Plugin:  makePlugin(map[string]interface{}{"agent": "agent1", "message": pluginTextMessage("prepare"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
			{
				Name: "fetch",
				HTTP: &wfv1.HTTP{
					URL:    "{{workflow.parameters.base_url}}/data",
					Method: "POST",
					Body:   "{{tasks.prep.outputs.parameters.result}}",
					Headers: []wfv1.HTTPHeader{
						{Name: "X-Request-ID", Value: "{{workflow.parameters.base_url}}"},
					},
				},
				Outputs: httpOutputs(),
			},
		},
		withParams(wfv1.Parameter{Name: "base_url", Value: wfv1.AnyStringPtr("https://api.example.com")}),
	)

	assertDiff(t, want, got)
}

func TestCompile_HTTPInPipeline(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-pipeline", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com/data",
					},
				},
				{
					Name: "analyze",
					Agent: &agentv1alpha1.AgentCall{
						Name: "analyzer",
						Text: "Analyze: {{tasks.fetch.output}}",
					},
					DependsOn: []string{"fetch"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("http-pipeline", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "fetch", Template: "fetch"},
				wfv1.DAGTask{Name: "analyze", Template: "analyze", Dependencies: []string{"fetch"}},
			),
			{
				Name: "fetch",
				HTTP: &wfv1.HTTP{
					URL:    "https://api.example.com/data",
					Method: "GET",
				},
				Outputs: httpOutputs(),
			},
			{
				Name:    "analyze",
				Plugin:  makePlugin(map[string]interface{}{"agent": "analyzer", "message": pluginTextMessage("Analyze: {{tasks.fetch.outputs.parameters.result}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

// --- Artifact I/O tests ---

func TestCompile_ArtifactIO_Container(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "artifact-container", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process",
					Container: &agentv1alpha1.ContainerTask{
						Image: "python:3.12",
					},
				},
			},
		},
	}

	opts := CompilerOptions{ArtifactIOEnabled: true, ArtifactGCStrategy: "OnWorkflowCompletion"}
	got, err := compileToArgoWorkflowTemplate(awf, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify artifact outputs are used instead of parameter outputs.
	tmpl := got.Spec.Templates[1] // index 0 is the DAG entrypoint
	if len(tmpl.Outputs.Artifacts) != 2 {
		t.Errorf("expected 2 artifact outputs, got %d", len(tmpl.Outputs.Artifacts))
	}
	if len(tmpl.Outputs.Parameters) != 0 {
		t.Errorf("expected no parameter outputs in artifact mode, got %d", len(tmpl.Outputs.Parameters))
	}
}

func TestCompile_ArtifactIO_WorkflowGC(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "artifact-gc", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Text: "hello"}},
			},
		},
	}

	opts := CompilerOptions{ArtifactIOEnabled: true, ArtifactGCStrategy: "OnWorkflowCompletion"}
	got, err := compileToArgoWorkflowTemplate(awf, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Spec.ArtifactGC == nil {
		t.Fatal("expected artifact GC to be set")
	}
	if got.Spec.ArtifactGC.Strategy != "OnWorkflowCompletion" {
		t.Errorf("expected strategy OnWorkflowCompletion, got %s", got.Spec.ArtifactGC.Strategy)
	}
}

func TestTranslateExpressions_ArtifactMode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "task output uses artifacts",
			input:    "{{tasks.research.output}}",
			expected: "{{tasks.research.outputs.artifacts.result}}",
		},
		{
			name:     "task artifact uses artifacts",
			input:    "{{tasks.call.artifact}}",
			expected: "{{tasks.call.outputs.artifacts.artifact}}",
		},
		{
			name:     "field access uses artifacts",
			input:    "{{tasks.x.output.field}}",
			expected: "{{=sprig.fromJson(tasks['x'].outputs.artifacts['artifact']).parts[0].data.field}}",
		},
		{
			name:     "params unchanged",
			input:    "{{params.topic}}",
			expected: "{{workflow.parameters.topic}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TranslateExpressionsWithMode(tt.input, true)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// --- Mixed task type test ---

func TestCompile_MixedTaskTypes(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "mixed-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch-data",
					HTTP: &agentv1alpha1.HTTPTask{
						URL:    "https://api.example.com/data",
						Method: "GET",
					},
				},
				{
					Name: "preprocess",
					Container: &agentv1alpha1.ContainerTask{
						Image: "python:3.12",
						Env: []corev1.EnvVar{
							{Name: "DATA", Value: "{{tasks.fetch-data.output}}"},
						},
					},
					DependsOn: []string{"fetch-data"},
				},
				{
					Name: "analyze",
					Agent: &agentv1alpha1.AgentCall{
						Name: "analyzer",
						Text: "Analyze: {{tasks.preprocess.output}}",
					},
					DependsOn: []string{"preprocess"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("mixed-test", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "fetch-data", Template: "fetch-data"},
				wfv1.DAGTask{Name: "preprocess", Template: "preprocess", Dependencies: []string{"fetch-data"}},
				wfv1.DAGTask{Name: "analyze", Template: "analyze", Dependencies: []string{"preprocess"}},
			),
			{
				Name: "fetch-data",
				HTTP: &wfv1.HTTP{
					URL:    "https://api.example.com/data",
					Method: "GET",
				},
				Outputs: httpOutputs(),
			},
			{
				Name: "preprocess",
				Container: &corev1.Container{
					Image: "python:3.12",
					Env: []corev1.EnvVar{
						{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"},
						{Name: "DATA", Value: "{{tasks.fetch-data.outputs.parameters.result}}"},
					},
				},
				Outputs: containerTaskOutputs(),
			},
			{
				Name:    "analyze",
				Plugin:  makePlugin(map[string]interface{}{"agent": "analyzer", "message": pluginTextMessage("Analyze: {{tasks.preprocess.outputs.parameters.result}}"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

// --- Switch task tests ---

func TestCompile_SwitchBasic(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-basic", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "classify", Agent: &agentv1alpha1.AgentCall{Name: "classifier", Text: "classify this"}},
				{
					Name: "route",
					Switch: []agentv1alpha1.SwitchCase{
						{Condition: "{{tasks.classify.output}} == positive", Then: "celebrate"},
						{Condition: "{{tasks.classify.output}} == negative", Then: "escalate"},
					},
					DependsOn: []string{"classify"},
				},
				{Name: "celebrate", Agent: &agentv1alpha1.AgentCall{Name: "happy-agent", Text: "celebrate!"}},
				{Name: "escalate", Agent: &agentv1alpha1.AgentCall{Name: "support-agent", Text: "help needed"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("switch-basic", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "classify", Template: "classify"},
				wfv1.DAGTask{Name: "route", Template: "route", Dependencies: []string{"classify"}},
				// Switch-generated DAG tasks
				wfv1.DAGTask{Name: "route-celebrate", Template: "celebrate", Dependencies: []string{"route"}, When: "{{tasks.classify.outputs.parameters.result}} == positive"},
				wfv1.DAGTask{Name: "route-escalate", Template: "escalate", Dependencies: []string{"route"}, When: "{{tasks.classify.outputs.parameters.result}} == negative"},
				wfv1.DAGTask{Name: "celebrate", Template: "celebrate"},
				wfv1.DAGTask{Name: "escalate", Template: "escalate"},
			),
			{Name: "classify", Plugin: makePlugin(map[string]interface{}{"agent": "classifier", "message": pluginTextMessage("classify this"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{
				Name: "route",
				Script: &wfv1.ScriptTemplate{
					Container: corev1.Container{
						Image:   "alpine:3.18",
						Command: []string{"echo"},
						Args:    []string{"switch-router"},
					},
				},
			},
			{Name: "celebrate", Plugin: makePlugin(map[string]interface{}{"agent": "happy-agent", "message": pluginTextMessage("celebrate!"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "escalate", Plugin: makePlugin(map[string]interface{}{"agent": "support-agent", "message": pluginTextMessage("help needed"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_SwitchWithDefault(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-default", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "check", Agent: &agentv1alpha1.AgentCall{Name: "checker", Text: "check priority"}},
				{
					Name: "route",
					Switch: []agentv1alpha1.SwitchCase{
						{Condition: "{{tasks.check.output}} == urgent", Then: "fast-track"},
						{Default: "normal-queue"},
					},
					DependsOn: []string{"check"},
				},
				{Name: "fast-track", Agent: &agentv1alpha1.AgentCall{Name: "priority-agent", Text: "handle urgently"}},
				{Name: "normal-queue", Agent: &agentv1alpha1.AgentCall{Name: "queue-agent", Text: "queue it"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("switch-default", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "check", Template: "check"},
				wfv1.DAGTask{Name: "route", Template: "route", Dependencies: []string{"check"}},
				wfv1.DAGTask{Name: "route-fast-track", Template: "fast-track", Dependencies: []string{"route"}, When: "{{tasks.check.outputs.parameters.result}} == urgent"},
				wfv1.DAGTask{Name: "route-normal-queue", Template: "normal-queue", Dependencies: []string{"route"}},
				wfv1.DAGTask{Name: "fast-track", Template: "fast-track"},
				wfv1.DAGTask{Name: "normal-queue", Template: "normal-queue"},
			),
			{Name: "check", Plugin: makePlugin(map[string]interface{}{"agent": "checker", "message": pluginTextMessage("check priority"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{
				Name: "route",
				Script: &wfv1.ScriptTemplate{
					Container: corev1.Container{
						Image:   "alpine:3.18",
						Command: []string{"echo"},
						Args:    []string{"switch-router"},
					},
				},
			},
			{Name: "fast-track", Plugin: makePlugin(map[string]interface{}{"agent": "priority-agent", "message": pluginTextMessage("handle urgently"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "normal-queue", Plugin: makePlugin(map[string]interface{}{"agent": "queue-agent", "message": pluginTextMessage("queue it"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_SwitchMultipleConditionsAndDefault(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-multi", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "classify", Agent: &agentv1alpha1.AgentCall{Name: "classifier", Text: "classify sentiment"}},
				{
					Name: "route",
					Switch: []agentv1alpha1.SwitchCase{
						{Condition: "{{tasks.classify.output}} == positive", Then: "celebrate"},
						{Condition: "{{tasks.classify.output}} == negative", Then: "escalate"},
						{Condition: "{{tasks.classify.output}} == neutral", Then: "archive"},
						{Default: "review"},
					},
					DependsOn: []string{"classify"},
				},
				{Name: "celebrate", Agent: &agentv1alpha1.AgentCall{Name: "agent-a", Text: "a"}},
				{Name: "escalate", Agent: &agentv1alpha1.AgentCall{Name: "agent-b", Text: "b"}},
				{Name: "archive", Agent: &agentv1alpha1.AgentCall{Name: "agent-c", Text: "c"}},
				{Name: "review", Agent: &agentv1alpha1.AgentCall{Name: "agent-d", Text: "d"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dagTemplate := got.Spec.Templates[0]
	if dagTemplate.DAG == nil {
		t.Fatal("expected DAG template")
	}

	switchTasks := make(map[string]string)
	for _, dt := range dagTemplate.DAG.Tasks {
		if len(dt.Dependencies) == 1 && dt.Dependencies[0] == taskNameRoute && dt.Name != taskNameRoute {
			switchTasks[dt.Name] = dt.When
		}
	}

	if when, ok := switchTasks["route-celebrate"]; !ok {
		t.Error("missing route-celebrate DAG task")
	} else if when == "" {
		t.Error("route-celebrate should have a When expression")
	}
	if when, ok := switchTasks["route-escalate"]; !ok {
		t.Error("missing route-escalate DAG task")
	} else if when == "" {
		t.Error("route-escalate should have a When expression")
	}
	if when, ok := switchTasks["route-archive"]; !ok {
		t.Error("missing route-archive DAG task")
	} else if when == "" {
		t.Error("route-archive should have a When expression")
	}
	if when, ok := switchTasks["route-review"]; !ok {
		t.Error("missing route-review DAG task")
	} else if when != "" {
		t.Errorf("route-review (default) should not have When, got %q", when)
	}
	if len(switchTasks) != 4 {
		t.Errorf("expected 4 switch-generated DAG tasks, got %d", len(switchTasks))
	}
}

// --- Agent message advanced features tests ---

func TestCompile_AgentMessageWithDataPart(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "data-part-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "analyze",
					Agent: &agentv1alpha1.AgentCall{
						Name: "analyzer-agent",
						Message: &agentv1alpha1.AgentMessage{
							Parts: []agentv1alpha1.MessagePart{
								{Text: &agentv1alpha1.TextPart{Text: "Analyze with constraints"}},
								{Data: &agentv1alpha1.DataPart{
									Data: apiextensionsv1.JSON{Raw: []byte(`{"budget":500000,"timeline":"6 months"}`)},
								}},
							},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("data-part-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "analyze", Template: "analyze"}),
			{
				Name: "analyze",
				Plugin: makePlugin(map[string]interface{}{
					"agent":       "analyzer-agent",
					"traceparent": "{{workflow.parameters._flokoa_traceparent}}",
					"message": map[string]interface{}{
						"parts": []map[string]interface{}{
							{"text": map[string]interface{}{"text": "Analyze with constraints"}},
							{"data": map[string]interface{}{
								"data": []byte(`{"budget":500000,"timeline":"6 months"}`),
							}},
						},
					},
				}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_AgentMessageFullMetadata(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "metadata-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "full-msg",
					Agent: &agentv1alpha1.AgentCall{
						Name: "my-agent",
						Message: &agentv1alpha1.AgentMessage{
							Role: agentv1alpha1.MessageRoleAgent,
							Parts: []agentv1alpha1.MessagePart{
								{Text: &agentv1alpha1.TextPart{Text: "continue"}},
							},
							ContextID:        "ctx-abc",
							TaskID:           "task-xyz",
							ReferenceTaskIDs: []string{"ref-1", "ref-2"},
							Extensions:       []string{"urn:a2a:ext:streaming"},
							Metadata: map[string]apiextensionsv1.JSON{
								"source":   {Raw: []byte(`"workflow"`)},
								"priority": {Raw: []byte(`1`)},
							},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("metadata-test", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "full-msg", Template: "full-msg"}),
			{
				Name: "full-msg",
				Plugin: makePlugin(map[string]interface{}{
					"agent":       "my-agent",
					"traceparent": "{{workflow.parameters._flokoa_traceparent}}",
					"message": map[string]interface{}{
						"role": "agent",
						"parts": []map[string]interface{}{
							{"text": map[string]interface{}{"text": "continue"}},
						},
						"contextId":        "ctx-abc",
						"taskId":           "task-xyz",
						"referenceTaskIds": []string{"ref-1", "ref-2"},
						"extensions":       []string{"urn:a2a:ext:streaming"},
						"metadata": map[string]interface{}{
							"source":   "workflow",
							"priority": float64(1),
						},
					},
				}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

// --- Agent config tests ---

func TestCompile_AgentConfigHistoryLength(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "config-hl", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task",
					Agent: &agentv1alpha1.AgentCall{
						Name: "my-agent",
						Text: "hello",
						Config: &agentv1alpha1.MessageSendConfig{
							HistoryLength: int32Ptr(10),
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("config-hl", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "task", Template: "task"}),
			{
				Name: "task",
				Plugin: makePlugin(map[string]interface{}{
					"agent":       "my-agent",
					"message":     pluginTextMessage("hello"),
					"traceparent": "{{workflow.parameters._flokoa_traceparent}}",
					"config": map[string]interface{}{
						"historyLength": int32(10),
					},
				}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_AgentConfigPushNotification(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "config-push", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task",
					Agent: &agentv1alpha1.AgentCall{
						Name: "my-agent",
						Text: "hello",
						Config: &agentv1alpha1.MessageSendConfig{
							PushNotificationConfig: &agentv1alpha1.PushNotificationConfig{
								URL:   "https://hooks.example.com/notify",
								ID:    "wf-123",
								Token: "verify-me",
								Authentication: &agentv1alpha1.PushNotificationAuth{
									Schemes:     []string{"Bearer"},
									Credentials: "secret-token",
								},
							},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("config-push", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "task", Template: "task"}),
			{
				Name: "task",
				Plugin: makePlugin(map[string]interface{}{
					"agent":       "my-agent",
					"message":     pluginTextMessage("hello"),
					"traceparent": "{{workflow.parameters._flokoa_traceparent}}",
					"config": map[string]interface{}{
						"pushNotificationConfig": map[string]interface{}{
							"url":   "https://hooks.example.com/notify",
							"id":    "wf-123",
							"token": "verify-me",
							"authentication": map[string]interface{}{
								"schemes":     []string{"Bearer"},
								"credentials": "secret-token",
							},
						},
					},
				}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_AgentConfigFull(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "config-full", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task",
					Agent: &agentv1alpha1.AgentCall{
						Name: "my-agent",
						Text: "hello",
						Config: &agentv1alpha1.MessageSendConfig{
							AcceptedOutputModes: []string{"text", "application/json"},
							Blocking:            boolPtr(true),
							HistoryLength:       int32Ptr(5),
							PushNotificationConfig: &agentv1alpha1.PushNotificationConfig{
								URL:   "https://hooks.example.com/notify",
								Token: "token123",
							},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("config-full", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "task", Template: "task"}),
			{
				Name: "task",
				Plugin: makePlugin(map[string]interface{}{
					"agent":       "my-agent",
					"message":     pluginTextMessage("hello"),
					"traceparent": "{{workflow.parameters._flokoa_traceparent}}",
					"config": map[string]interface{}{
						"acceptedOutputModes": []string{"text", "application/json"},
						"blocking":            true,
						"historyLength":       int32(5),
						"pushNotificationConfig": map[string]interface{}{
							"url":   "https://hooks.example.com/notify",
							"token": "token123",
						},
					},
				}),
				Outputs: pluginOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

// --- Retry and timeout tests ---

func TestCompile_PerTaskRetryOverridesWorkflow(t *testing.T) {
	wfFactor := int32(2)
	taskFactor := int32(3)
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "retry-override", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			RetryStrategy: &agentv1alpha1.WorkflowRetryStrategy{
				Limit:   5,
				Backoff: &agentv1alpha1.WorkflowBackoff{Duration: "1m", Factor: &wfFactor},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task-a", Agent: &agentv1alpha1.AgentCall{Name: "agent-a", Text: "a"}},
				{
					Name:  "task-b",
					Agent: &agentv1alpha1.AgentCall{Name: "agent-b", Text: "b"},
					RetryStrategy: &agentv1alpha1.WorkflowRetryStrategy{
						Limit:   2,
						Backoff: &agentv1alpha1.WorkflowBackoff{Duration: "10s", Factor: &taskFactor},
					},
					DependsOn: []string{"task-a"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wfLimit := intstr.FromInt32(5)
	wfBackoffFactor := intstr.FromInt32(2)
	taskLimit := intstr.FromInt32(2)
	taskBackoffFactor := intstr.FromInt32(3)

	want := wantWorkflowTemplate("retry-override", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "task-a", Template: "task-a"},
				wfv1.DAGTask{Name: "task-b", Template: "task-b", Dependencies: []string{"task-a"}},
			),
			{
				Name:    "task-a",
				Plugin:  makePlugin(map[string]interface{}{"agent": "agent-a", "message": pluginTextMessage("a"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
				RetryStrategy: &wfv1.RetryStrategy{
					Limit:   &wfLimit,
					Backoff: &wfv1.Backoff{Duration: "1m", Factor: &wfBackoffFactor},
				},
			},
			{
				Name:    "task-b",
				Plugin:  makePlugin(map[string]interface{}{"agent": "agent-b", "message": pluginTextMessage("b"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}),
				Outputs: pluginOutputs(),
				RetryStrategy: &wfv1.RetryStrategy{
					Limit:   &taskLimit,
					Backoff: &wfv1.Backoff{Duration: "10s", Factor: &taskBackoffFactor},
				},
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_ContainerTimeout(t *testing.T) {
	timeout := metav1.Duration{Duration: 5 * time.Minute}
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "container-timeout", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:      "process",
					Container: &agentv1alpha1.ContainerTask{Image: "python:3.12"},
					Timeout:   &timeout,
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("container-timeout", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "process", Template: "process"}),
			{
				Name:                  "process",
				ActiveDeadlineSeconds: &intstr.IntOrString{Type: intstr.Int, IntVal: 300},
				Container: &corev1.Container{
					Image: "python:3.12",
					Env:   []corev1.EnvVar{{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"}},
				},
				Outputs: containerTaskOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_HTTPTimeout(t *testing.T) {
	timeout := metav1.Duration{Duration: 2 * time.Minute}
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-timeout", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:    "fetch",
					HTTP:    &agentv1alpha1.HTTPTask{URL: "https://api.example.com/data"},
					Timeout: &timeout,
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("http-timeout", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "fetch", Template: "fetch"}),
			{
				Name:                  "fetch",
				ActiveDeadlineSeconds: &intstr.IntOrString{Type: intstr.Int, IntVal: 120},
				HTTP:                  &wfv1.HTTP{URL: "https://api.example.com/data", Method: "GET"},
				Outputs:               httpOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

// --- DAG pattern tests ---

func TestCompile_DiamondDAG(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "diamond", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "A", Agent: &agentv1alpha1.AgentCall{Name: "agent-a", Text: "start"}},
				{Name: "B", Agent: &agentv1alpha1.AgentCall{Name: "agent-b", Text: "left"}, DependsOn: []string{"A"}},
				{Name: "C", Agent: &agentv1alpha1.AgentCall{Name: "agent-c", Text: "right"}, DependsOn: []string{"A"}},
				{Name: "D", Agent: &agentv1alpha1.AgentCall{Name: "agent-d", Text: "merge"}, DependsOn: []string{"B", "C"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("diamond", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "A", Template: "A"},
				wfv1.DAGTask{Name: "B", Template: "B", Dependencies: []string{"A"}},
				wfv1.DAGTask{Name: "C", Template: "C", Dependencies: []string{"A"}},
				wfv1.DAGTask{Name: "D", Template: "D", Dependencies: []string{"B", "C"}},
			),
			{Name: "A", Plugin: makePlugin(map[string]interface{}{"agent": "agent-a", "message": pluginTextMessage("start"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "B", Plugin: makePlugin(map[string]interface{}{"agent": "agent-b", "message": pluginTextMessage("left"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "C", Plugin: makePlugin(map[string]interface{}{"agent": "agent-c", "message": pluginTextMessage("right"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "D", Plugin: makePlugin(map[string]interface{}{"agent": "agent-d", "message": pluginTextMessage("merge"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
		},
	)

	assertDiff(t, want, got)
}

// --- Condition on non-agent task types ---

func TestCompile_ConditionOnContainerTask(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "cond-container", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "check", Agent: &agentv1alpha1.AgentCall{Name: "checker", Text: "check"}},
				{
					Name:      "process",
					Container: &agentv1alpha1.ContainerTask{Image: "python:3.12"},
					DependsOn: []string{"check"},
					Condition: "{{tasks.check.output}} == proceed",
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("cond-container", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "check", Template: "check"},
				wfv1.DAGTask{Name: "process", Template: "process", Dependencies: []string{"check"}, When: "{{tasks.check.outputs.parameters.result}} == proceed"},
			),
			{Name: "check", Plugin: makePlugin(map[string]interface{}{"agent": "checker", "message": pluginTextMessage("check"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{
				Name:      "process",
				Container: &corev1.Container{Image: "python:3.12", Env: []corev1.EnvVar{{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"}}},
				Outputs:   containerTaskOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

func TestCompile_ConditionOnHTTPTask(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "cond-http", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "check", Agent: &agentv1alpha1.AgentCall{Name: "checker", Text: "check approval"}},
				{
					Name:      "submit",
					HTTP:      &agentv1alpha1.HTTPTask{URL: "https://api.example.com/submit", Method: "POST"},
					DependsOn: []string{"check"},
					Condition: "{{tasks.check.output}} == approved",
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("cond-http", "default",
		[]wfv1.Template{
			dagTmpl(
				wfv1.DAGTask{Name: "check", Template: "check"},
				wfv1.DAGTask{Name: "submit", Template: "submit", Dependencies: []string{"check"}, When: "{{tasks.check.outputs.parameters.result}} == approved"},
			),
			{Name: "check", Plugin: makePlugin(map[string]interface{}{"agent": "checker", "message": pluginTextMessage("check approval"), "traceparent": "{{workflow.parameters._flokoa_traceparent}}"}), Outputs: pluginOutputs()},
			{Name: "submit", HTTP: &wfv1.HTTP{URL: "https://api.example.com/submit", Method: "POST"}, Outputs: httpOutputs()},
		},
	)

	assertDiff(t, want, got)
}

// --- Artifact I/O for agent and HTTP tasks ---

func TestCompile_ArtifactIO_AgentPlugin(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "artifact-agent", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Text: "hello"}},
			},
		},
	}

	opts := CompilerOptions{ArtifactIOEnabled: true, ArtifactGCStrategy: "OnWorkflowCompletion"}
	got, err := compileToArgoWorkflowTemplate(awf, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A2A plugin tasks still use parameter outputs even in artifact mode.
	tmpl := got.Spec.Templates[1]
	if len(tmpl.Outputs.Parameters) != 2 {
		t.Errorf("expected 2 parameter outputs for agent plugin in artifact mode, got %d", len(tmpl.Outputs.Parameters))
	}
	if len(tmpl.Outputs.Artifacts) != 0 {
		t.Errorf("expected no artifact outputs for agent plugin, got %d", len(tmpl.Outputs.Artifacts))
	}
}

func TestCompile_ArtifactIO_HTTPTask(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "artifact-http", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "fetch", HTTP: &agentv1alpha1.HTTPTask{URL: "https://api.example.com/data"}},
			},
		},
	}

	opts := CompilerOptions{ArtifactIOEnabled: true, ArtifactGCStrategy: "OnWorkflowCompletion"}
	got, err := compileToArgoWorkflowTemplate(awf, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// HTTP tasks still use parameter outputs even in artifact mode.
	tmpl := got.Spec.Templates[1]
	if len(tmpl.Outputs.Parameters) != 2 {
		t.Errorf("expected 2 parameter outputs for HTTP in artifact mode, got %d", len(tmpl.Outputs.Parameters))
	}
	if len(tmpl.Outputs.Artifacts) != 0 {
		t.Errorf("expected no artifact outputs for HTTP, got %d", len(tmpl.Outputs.Artifacts))
	}
}

// --- Container volume mounts test ---

func TestCompile_ContainerWithVolumeMounts(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "container-vols", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process",
					Container: &agentv1alpha1.ContainerTask{
						Image:   "python:3.12",
						Command: []string{"python", "run.py"},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "data", MountPath: "/data", ReadOnly: true},
							{Name: "config", MountPath: "/config"},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("container-vols", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "process", Template: "process"}),
			{
				Name: "process",
				Container: &corev1.Container{
					Image:   "python:3.12",
					Command: []string{"python", "run.py"},
					Env:     []corev1.EnvVar{{Name: "FLOKOA_TRACEPARENT", Value: "{{workflow.parameters._flokoa_traceparent}}"}},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "data", MountPath: "/data", ReadOnly: true},
						{Name: "config", MountPath: "/config"},
					},
				},
				Outputs: containerTaskOutputs(),
			},
		},
	)

	assertDiff(t, want, got)
}

// --- HTTP mixed headers test ---

func TestCompile_HTTPMixedHeaders(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-mixed-headers", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL:    "https://api.example.com/data",
						Method: "GET",
						Headers: []agentv1alpha1.HTTPHeader{
							{Name: "Accept", Value: "application/json"},
							{Name: "Authorization", ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "api-creds"}, Key: "token"}}},
							{Name: "X-Api-Version", ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}, Key: "version"}}},
						},
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("http-mixed-headers", "default",
		[]wfv1.Template{
			dagTmpl(wfv1.DAGTask{Name: "fetch", Template: "fetch"}),
			{
				Name: "fetch",
				HTTP: &wfv1.HTTP{
					URL:    "https://api.example.com/data",
					Method: "GET",
					Headers: []wfv1.HTTPHeader{
						{Name: "Accept", Value: "application/json"},
						{Name: "Authorization", ValueFrom: &wfv1.HTTPHeaderSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "api-creds"}, Key: "token"}}},
						{Name: "X-Api-Version", Value: "{{workflow.parameters._cm_api-config_version}}"},
					},
				},
				Outputs: httpOutputs(),
			},
		},
		withParams(wfv1.Parameter{
			Name:      "_cm_api-config_version",
			ValueFrom: &wfv1.ValueFrom{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "api-config"}, Key: "version"}},
		}),
	)

	assertDiff(t, want, got)
}

func TestCompile_CompletePipelineWithSwitch(t *testing.T) {
	timeout2m := metav1.Duration{Duration: 2 * time.Minute}
	timeout5m := metav1.Duration{Duration: 5 * time.Minute}
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "full-pipeline", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "api_url", Value: "https://data.example.com/v2"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:    "fetch",
					HTTP:    &agentv1alpha1.HTTPTask{URL: "{{params.api_url}}/records", Method: "GET", Headers: []agentv1alpha1.HTTPHeader{{Name: "Accept", Value: "application/json"}, {Name: "Authorization", ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "data-api-creds"}, Key: "token"}}}}},
					Timeout: &timeout2m,
				},
				{
					Name:      "preprocess",
					Container: &agentv1alpha1.ContainerTask{Image: "python:3.13-slim", Command: []string{"python", "preprocess.py"}, Env: []corev1.EnvVar{{Name: "RAW_DATA", Value: "{{tasks.fetch.output}}"}}},
					DependsOn: []string{"fetch"},
					Timeout:   &timeout5m,
				},
				{
					Name:      "classify",
					Container: &agentv1alpha1.ContainerTask{Image: "python:3.13-slim", Command: []string{"python", "classify.py"}, Env: []corev1.EnvVar{{Name: "INPUT", Value: "{{tasks.preprocess.output}}"}}},
					DependsOn: []string{"preprocess"},
				},
				{
					Name:      "route",
					Switch:    []agentv1alpha1.SwitchCase{{Condition: "{{tasks.classify.output}} == actionable", Then: "act"}, {Condition: "{{tasks.classify.output}} == informational", Then: "archive"}, {Default: "discard"}},
					DependsOn: []string{"classify"},
				},
				{Name: "act", Agent: &agentv1alpha1.AgentCall{Name: "action-agent", Text: "Process: {{tasks.preprocess.output}}"}},
				{Name: "archive", HTTP: &agentv1alpha1.HTTPTask{URL: "{{params.api_url}}/archive", Method: "POST", Headers: []agentv1alpha1.HTTPHeader{{Name: "Content-Type", Value: "application/json"}}, Body: `{"data": "{{tasks.preprocess.output}}", "label": "informational"}`}},
				{Name: "discard", Container: &agentv1alpha1.ContainerTask{Image: "alpine:3.20", Command: []string{"sh", "-c"}, Args: []string{"echo 'Discarded' >> /tmp/result"}}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify structure: 8 templates (1 DAG + 7 task), 10 DAG tasks (7 tasks + 3 switch branches)
	if len(got.Spec.Templates) != 8 {
		t.Errorf("expected 8 templates, got %d", len(got.Spec.Templates))
	}

	dagTmplGot := got.Spec.Templates[0]
	if dagTmplGot.DAG == nil {
		t.Fatal("expected DAG template at index 0")
	}
	if len(dagTmplGot.DAG.Tasks) != 10 {
		t.Errorf("expected 10 DAG tasks (7 tasks + 3 switch branches), got %d", len(dagTmplGot.DAG.Tasks))
	}

	// Verify switch-generated tasks
	dagTaskMap := make(map[string]wfv1.DAGTask)
	for _, dt := range dagTmplGot.DAG.Tasks {
		dagTaskMap[dt.Name] = dt
	}

	if dt, ok := dagTaskMap["route-act"]; !ok {
		t.Error("missing route-act DAG task")
	} else {
		if dt.When == "" {
			t.Error("route-act should have a When expression")
		}
		if len(dt.Dependencies) != 1 || dt.Dependencies[0] != taskNameRoute {
			t.Errorf("route-act should depend on route, got %v", dt.Dependencies)
		}
	}
	if dt, ok := dagTaskMap["route-archive"]; !ok {
		t.Error("missing route-archive DAG task")
	} else if dt.When == "" {
		t.Error("route-archive should have a When expression")
	}
	if dt, ok := dagTaskMap["route-discard"]; !ok {
		t.Error("missing route-discard DAG task")
	} else if dt.When != "" {
		t.Errorf("route-discard (default) should not have When, got %q", dt.When)
	}

	// Verify expression translations
	preprocessTmpl := got.Spec.Templates[2]
	if preprocessTmpl.Container == nil {
		t.Fatal("expected container template for preprocess")
	}
	for _, env := range preprocessTmpl.Container.Env {
		if env.Name == "RAW_DATA" && env.Value != "{{tasks.fetch.outputs.parameters.result}}" {
			t.Errorf("RAW_DATA env should be translated, got %q", env.Value)
		}
	}

	classifyTmpl := got.Spec.Templates[3]
	if classifyTmpl.Container == nil {
		t.Fatal("expected container template for classify")
	}
	for _, env := range classifyTmpl.Container.Env {
		if env.Name == "INPUT" && !strings.Contains(env.Value, "tasks.preprocess.outputs.parameters.result") {
			t.Errorf("classify input env should contain translated expression, got %q", env.Value)
		}
	}

	archiveTmpl := got.Spec.Templates[6]
	if archiveTmpl.HTTP == nil {
		t.Fatal("expected HTTP template for archive")
	}
	if archiveTmpl.HTTP.URL != "{{workflow.parameters.api_url}}/archive" {
		t.Errorf("archive URL not translated, got %q", archiveTmpl.HTTP.URL)
	}
	if !strings.Contains(archiveTmpl.HTTP.Body, "tasks.preprocess.outputs.parameters.result") {
		t.Errorf("archive body should contain translated expression, got %q", archiveTmpl.HTTP.Body)
	}
}

// =============================================================================
// Error path tests — verify the compiler rejects invalid inputs
// =============================================================================

func TestCompile_ErrorNoTaskType(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "empty-task"}, // no agent, agentTask, container, http, or switch
			},
		},
	}

	_, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err == nil {
		t.Fatal("expected error for task with no recognized type, got nil")
	}
	if !strings.Contains(err.Error(), "no recognized type") {
		t.Errorf("error should mention 'no recognized type', got: %v", err)
	}
}

func TestCompile_ErrorAgentTaskFrozen(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "frozen", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:      "task",
					AgentTask: &agentv1alpha1.AgentTask{Type: agentv1alpha1.AgentTaskTypeRun}, //nolint:staticcheck // exercising the freeze guard
				},
			},
		},
	}

	_, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err == nil {
		t.Fatal("expected error for agentTask task, got nil")
	}
	if !strings.Contains(err.Error(), "agentTask is no longer supported") {
		t.Errorf("error should mention the agentTask freeze, got: %v", err)
	}
}

func TestTranslateExpressions_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "unknown expression passes through",
			input:    "{{unknown.foo}}",
			expected: "{{unknown.foo}}",
		},
		{
			name:     "single-part task reference passes through",
			input:    "{{tasks.foo}}",
			expected: "{{tasks.foo}}",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			expected: "",
		},
		{
			name:     "text with no braces",
			input:    "just some text",
			expected: "just some text",
		},
		{
			name:     "deep nested field access",
			input:    "{{tasks.x.output.a.b.c.d}}",
			expected: "{{=sprig.fromJson(tasks['x'].outputs.parameters['artifact']).parts[0].data.a.b.c.d}}",
		},
		{
			name:     "task name with numbers",
			input:    "{{tasks.task123.output}}",
			expected: "{{tasks.task123.outputs.parameters.result}}",
		},
		{
			name:     "mixed params and task refs",
			input:    "Input: {{params.x}}, Output: {{tasks.a.output}}, Field: {{tasks.b.output.f}}",
			expected: "Input: {{workflow.parameters.x}}, Output: {{tasks.a.outputs.parameters.result}}, Field: {{=sprig.fromJson(tasks['b'].outputs.parameters['artifact']).parts[0].data.f}}",
		},
		{
			name:     "argo expression with spaces",
			input:    "{{= some.complex.expr }}",
			expected: "{{= some.complex.expr}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TranslateExpressions(tt.input)
			if result != tt.expected {
				t.Errorf("\n  input:    %q\n  expected: %q\n  got:      %q", tt.input, tt.expected, result)
			}
		})
	}
}

// =============================================================================
// Structural validation tests — verify properties that should hold for ALL
// compiled workflows, regardless of input specifics. These catch bugs that
// snapshot tests miss because they verify invariants rather than exact output.
// =============================================================================

func TestCompileStructural_AllDAGTasksReferenceExistingTemplates(t *testing.T) {
	testCases := []struct {
		name string
		awf  *agentv1alpha1.AgentWorkflow
		opts CompilerOptions
	}{
		{
			name: "simple sequential",
			awf: &agentv1alpha1.AgentWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"},
				Spec: agentv1alpha1.AgentWorkflowSpec{
					Tasks: []agentv1alpha1.WorkflowTask{
						{Name: "a", Agent: &agentv1alpha1.AgentCall{Name: "agent", Text: "hello"}},
						{Name: "b", Agent: &agentv1alpha1.AgentCall{Name: "agent", Text: "world"}, DependsOn: []string{"a"}},
					},
				},
			},
		},
		{
			name: "with switch",
			awf: &agentv1alpha1.AgentWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: "s2", Namespace: "default"},
				Spec: agentv1alpha1.AgentWorkflowSpec{
					Tasks: []agentv1alpha1.WorkflowTask{
						{Name: "classify", Agent: &agentv1alpha1.AgentCall{Name: "cls", Text: "classify"}},
						{
							Name: "route",
							Switch: []agentv1alpha1.SwitchCase{
								{Condition: "{{tasks.classify.output}} == yes", Then: "handle"},
								{Default: "skip"},
							},
							DependsOn: []string{"classify"},
						},
						{Name: "handle", Agent: &agentv1alpha1.AgentCall{Name: "h", Text: "handle"}},
						{Name: "skip", Agent: &agentv1alpha1.AgentCall{Name: "s", Text: "skip"}},
					},
				},
			},
		},
		{
			name: "mixed types",
			awf: &agentv1alpha1.AgentWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: "s3", Namespace: "default"},
				Spec: agentv1alpha1.AgentWorkflowSpec{
					Tasks: []agentv1alpha1.WorkflowTask{
						{Name: "fetch", HTTP: &agentv1alpha1.HTTPTask{URL: "https://example.com"}},
						{Name: "process", Container: &agentv1alpha1.ContainerTask{Image: "python:3.12"}, DependsOn: []string{"fetch"}},
						{Name: "analyze", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "go"}, DependsOn: []string{"process"}},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := compileToArgoWorkflowTemplate(tc.awf, tc.opts)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Build set of template names
			templateNames := map[string]bool{}
			for _, tmpl := range got.Spec.Templates {
				if templateNames[tmpl.Name] {
					t.Errorf("duplicate template name: %q", tmpl.Name)
				}
				templateNames[tmpl.Name] = true
			}

			// Every DAG task must reference an existing template
			dagTmpl := got.Spec.Templates[0]
			if dagTmpl.DAG == nil {
				t.Fatal("first template should be the DAG entrypoint")
			}
			for _, dt := range dagTmpl.DAG.Tasks {
				if !templateNames[dt.Template] {
					t.Errorf("DAG task %q references template %q which does not exist", dt.Name, dt.Template)
				}
			}
		})
	}
}

func TestCompileStructural_EntrypointIsMain(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "ep-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "t", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "x"}},
			},
		},
	}
	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Spec.Entrypoint != "main" {
		t.Errorf("entrypoint should be 'main', got %q", got.Spec.Entrypoint)
	}
	if got.Spec.Templates[0].Name != "main" {
		t.Errorf("first template should be 'main', got %q", got.Spec.Templates[0].Name)
	}
	if got.Spec.Templates[0].DAG == nil {
		t.Error("first template should be a DAG template")
	}
}

func TestCompileStructural_AllWorkflowParamsPresent(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "params-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{
				{Name: "alpha", Value: "a"},
				{Name: "beta", Value: "b"},
				{Name: "gamma", Description: "g desc", Value: "g"},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "t", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "{{params.alpha}}"}},
			},
		},
	}
	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	paramNames := map[string]bool{}
	for _, p := range got.Spec.Arguments.Parameters {
		paramNames[p.Name] = true
	}

	// Traceparent should always be present
	if !paramNames["_flokoa_traceparent"] {
		t.Error("missing _flokoa_traceparent parameter")
	}
	// All user params should be present
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if !paramNames[name] {
			t.Errorf("missing workflow parameter %q", name)
		}
	}
}

func TestCompileStructural_TraceparentInjected(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "tp-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "t", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "x"}},
			},
		},
	}
	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The first parameter must be _flokoa_traceparent with a non-empty default
	if len(got.Spec.Arguments.Parameters) == 0 {
		t.Fatal("expected at least one workflow parameter")
	}
	tp := got.Spec.Arguments.Parameters[0]
	if tp.Name != "_flokoa_traceparent" {
		t.Errorf("first parameter should be _flokoa_traceparent, got %q", tp.Name)
	}
	if tp.Default == nil || string(*tp.Default) == "" {
		t.Error("_flokoa_traceparent should have a non-empty default value")
	}
}

func TestCompileStructural_LabelsAndMetadata(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "my-workflow", Namespace: "prod"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "t", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "x"}},
			},
		},
	}
	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Name != "my-workflow" {
		t.Errorf("name should match AgentWorkflow name, got %q", got.Name)
	}
	if got.Namespace != "prod" {
		t.Errorf("namespace should match AgentWorkflow namespace, got %q", got.Namespace)
	}
	if got.Labels["agent.flokoa.ai/agentworkflow-name"] != "my-workflow" {
		t.Errorf("missing or wrong agentworkflow-name label: %v", got.Labels)
	}
	if got.Labels["app.kubernetes.io/managed-by"] != "flokoa-operator" {
		t.Errorf("missing or wrong managed-by label: %v", got.Labels)
	}
	if got.Kind != "WorkflowTemplate" {
		t.Errorf("Kind should be WorkflowTemplate, got %q", got.Kind)
	}
	if got.APIVersion != "argoproj.io/v1alpha1" {
		t.Errorf("APIVersion should be argoproj.io/v1alpha1, got %q", got.APIVersion)
	}
}

func TestCompileStructural_EveryTaskProducesOutputs(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "out-test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "agent-task", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "hello"}},
				{Name: "http-task", HTTP: &agentv1alpha1.HTTPTask{URL: "https://example.com"}, DependsOn: []string{"agent-task"}},
				{Name: "container-task", Container: &agentv1alpha1.ContainerTask{Image: "python:3"}, DependsOn: []string{"http-task"}},
			},
		},
	}
	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Skip the first template (DAG entrypoint) and check all others have outputs
	for _, tmpl := range got.Spec.Templates[1:] {
		hasParams := len(tmpl.Outputs.Parameters) > 0
		hasArtifacts := len(tmpl.Outputs.Artifacts) > 0
		if !hasParams && !hasArtifacts {
			t.Errorf("template %q has no outputs (parameters or artifacts)", tmpl.Name)
		}

		// Check that "result" and "artifact" output names are present
		outputNames := map[string]bool{}
		for _, p := range tmpl.Outputs.Parameters {
			outputNames[p.Name] = true
		}
		for _, a := range tmpl.Outputs.Artifacts {
			outputNames[a.Name] = true
		}
		if !outputNames["result"] {
			t.Errorf("template %q missing 'result' output", tmpl.Name)
		}
		if !outputNames["artifact"] {
			t.Errorf("template %q missing 'artifact' output", tmpl.Name)
		}
	}
}

// =============================================================================
// Strengthen tautological tests — replace existence checks with content checks
// =============================================================================

func TestCompile_SwitchConditionsHaveCorrectContent(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-content", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "classify", Agent: &agentv1alpha1.AgentCall{Name: "classifier", Text: "classify this"}},
				{
					Name: "route",
					Switch: []agentv1alpha1.SwitchCase{
						{Condition: "{{tasks.classify.output}} == positive", Then: "celebrate"},
						{Condition: "{{tasks.classify.output}} == negative", Then: "escalate"},
						{Default: "review"},
					},
					DependsOn: []string{"classify"},
				},
				{Name: "celebrate", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "a"}},
				{Name: "escalate", Agent: &agentv1alpha1.AgentCall{Name: "b", Text: "b"}},
				{Name: "review", Agent: &agentv1alpha1.AgentCall{Name: "c", Text: "c"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dagTmplGot := got.Spec.Templates[0]
	dagTaskMap := make(map[string]wfv1.DAGTask)
	for _, dt := range dagTmplGot.DAG.Tasks {
		dagTaskMap[dt.Name] = dt
	}

	// Verify the exact translated When expressions — not just their existence
	wantConditions := map[string]string{
		"route-celebrate": "{{tasks.classify.outputs.parameters.result}} == positive",
		"route-escalate":  "{{tasks.classify.outputs.parameters.result}} == negative",
		"route-review":    "", // default has no When
	}

	for name, wantWhen := range wantConditions {
		dt, ok := dagTaskMap[name]
		if !ok {
			t.Errorf("missing switch DAG task %q", name)
			continue
		}
		if dt.When != wantWhen {
			t.Errorf("task %q: When = %q, want %q", name, dt.When, wantWhen)
		}
		// All switch branches should depend on "route"
		if len(dt.Dependencies) != 1 || dt.Dependencies[0] != taskNameRoute {
			t.Errorf("task %q: Dependencies = %v, want [route]", name, dt.Dependencies)
		}
	}
}

func TestCompile_SwitchFieldAccessConditionTranslated(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-field", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "classify", Agent: &agentv1alpha1.AgentCall{Name: "cls", Text: "classify"}},
				{
					Name: "route",
					Switch: []agentv1alpha1.SwitchCase{
						{Condition: "{{tasks.classify.output.category}} == urgent", Then: "handle"},
						{Default: "ignore"},
					},
					DependsOn: []string{"classify"},
				},
				{Name: "handle", Agent: &agentv1alpha1.AgentCall{Name: "h", Text: "h"}},
				{Name: "ignore", Agent: &agentv1alpha1.AgentCall{Name: "i", Text: "i"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dagTmplGot := got.Spec.Templates[0]
	dagTaskMap := make(map[string]wfv1.DAGTask)
	for _, dt := range dagTmplGot.DAG.Tasks {
		dagTaskMap[dt.Name] = dt
	}

	dt := dagTaskMap["route-handle"]
	wantWhen := "{{=sprig.fromJson(tasks['classify'].outputs.parameters['artifact']).parts[0].data.category}} == urgent"
	if dt.When != wantWhen {
		t.Errorf("route-handle When:\n  got:  %q\n  want: %q", dt.When, wantWhen)
	}
}

func TestCompile_PipelineExpressionTranslationAccuracy(t *testing.T) {
	// This test replaces the weak strings.Contains checks in
	// TestCompile_CompletePipelineWithSwitch with exact assertions.
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "expr-accuracy", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "api_url", Value: "https://example.com"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{URL: "{{params.api_url}}/data"},
				},
				{
					Name: "preprocess",
					Container: &agentv1alpha1.ContainerTask{
						Image: "python:3.12",
						Env:   []corev1.EnvVar{{Name: "RAW_DATA", Value: "{{tasks.fetch.output}}"}},
					},
					DependsOn: []string{"fetch"},
				},
				{
					Name: "analyze",
					Agent: &agentv1alpha1.AgentCall{
						Name: "analyzer",
						Text: "Analyze: {{tasks.preprocess.output}}",
					},
					DependsOn: []string{"preprocess"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify HTTP URL translation
	fetchTmpl := got.Spec.Templates[1]
	if fetchTmpl.HTTP == nil {
		t.Fatal("expected HTTP template for 'fetch'")
	}
	if fetchTmpl.HTTP.URL != "{{workflow.parameters.api_url}}/data" {
		t.Errorf("fetch URL = %q, want %q", fetchTmpl.HTTP.URL, "{{workflow.parameters.api_url}}/data")
	}

	// Verify container env var translation
	preprocessTmpl := got.Spec.Templates[2]
	if preprocessTmpl.Container == nil {
		t.Fatal("expected container template for 'preprocess'")
	}
	foundRAWDATA := false
	for _, env := range preprocessTmpl.Container.Env {
		if env.Name == "RAW_DATA" {
			foundRAWDATA = true
			want := "{{tasks.fetch.outputs.parameters.result}}"
			if env.Value != want {
				t.Errorf("RAW_DATA = %q, want %q", env.Value, want)
			}
		}
	}
	if !foundRAWDATA {
		t.Error("preprocess template missing RAW_DATA env var")
	}

	// Verify agent message expression translation
	analyzeTmpl := got.Spec.Templates[3]
	if analyzeTmpl.Plugin == nil {
		t.Fatal("expected plugin template for 'analyze'")
	}
	var pluginData map[string]interface{}
	if err := json.Unmarshal(analyzeTmpl.Plugin.Value, &pluginData); err != nil {
		t.Fatalf("failed to unmarshal plugin spec: %v", err)
	}
	a2aData, ok := pluginData["a2a"].(map[string]interface{})
	if !ok {
		t.Fatal("missing 'a2a' key in plugin spec")
	}
	msgData, ok := a2aData["message"].(map[string]interface{})
	if !ok {
		t.Fatal("missing 'message' key in a2a spec")
	}
	parts, ok := msgData["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		t.Fatal("missing or empty 'parts' in message")
	}
	firstPart, ok := parts[0].(map[string]interface{})
	if !ok {
		t.Fatal("first part is not a map")
	}
	textPart, ok := firstPart["text"].(map[string]interface{})
	if !ok {
		t.Fatal("first part missing 'text' key")
	}
	gotText, _ := textPart["text"].(string)
	wantText := "Analyze: {{tasks.preprocess.outputs.parameters.result}}"
	if gotText != wantText {
		t.Errorf("analyze message text = %q, want %q", gotText, wantText)
	}
}

// =============================================================================
// HTTP defaults and edge cases
// =============================================================================

func TestCompile_HTTPDefaultMethodIsGET(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-default", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com/data",
						// Method intentionally omitted
					},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	httpTmpl := got.Spec.Templates[1]
	if httpTmpl.HTTP == nil {
		t.Fatal("expected HTTP template")
	}
	if httpTmpl.HTTP.Method != "GET" {
		t.Errorf("expected default method GET, got %q", httpTmpl.HTTP.Method)
	}
}

func TestCompile_HTTPDuplicateConfigMapHeaderDeduplication(t *testing.T) {
	// Two tasks reference the same configMapKeyRef — should produce only one workflow parameter.
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "http-dedup", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "req1",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com/a",
						Headers: []agentv1alpha1.HTTPHeader{
							{
								Name:      "X-Token",
								ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "shared-cm"}, Key: "token"}},
							},
						},
					},
				},
				{
					Name: "req2",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com/b",
						Headers: []agentv1alpha1.HTTPHeader{
							{
								Name:      "X-Token",
								ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{ConfigMapKeyRef: &corev1.ConfigMapKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "shared-cm"}, Key: "token"}},
							},
						},
					},
					DependsOn: []string{"req1"},
				},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Count how many parameters start with _cm_
	cmParamCount := 0
	for _, p := range got.Spec.Arguments.Parameters {
		if strings.HasPrefix(p.Name, "_cm_") {
			cmParamCount++
		}
	}
	if cmParamCount != 1 {
		t.Errorf("expected exactly 1 configMap parameter (deduplication), got %d", cmParamCount)
	}
}

// =============================================================================
// Switch task edge cases
// =============================================================================

func TestCompile_SwitchOnlyDefault(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-only-default", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "check", Agent: &agentv1alpha1.AgentCall{Name: "checker", Text: "check"}},
				{
					Name: "route",
					Switch: []agentv1alpha1.SwitchCase{
						{Default: "fallback"},
					},
					DependsOn: []string{"check"},
				},
				{Name: "fallback", Agent: &agentv1alpha1.AgentCall{Name: "fb", Text: "fallback"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dagTmplGot := got.Spec.Templates[0]
	dagTaskMap := make(map[string]wfv1.DAGTask)
	for _, dt := range dagTmplGot.DAG.Tasks {
		dagTaskMap[dt.Name] = dt
	}

	// The default route should have no When condition
	dt, ok := dagTaskMap["route-fallback"]
	if !ok {
		t.Fatal("missing route-fallback DAG task")
	}
	if dt.When != "" {
		t.Errorf("default-only switch branch should have no When, got %q", dt.When)
	}
	if len(dt.Dependencies) != 1 || dt.Dependencies[0] != taskNameRoute {
		t.Errorf("route-fallback should depend on 'route', got %v", dt.Dependencies)
	}
}

func TestCompile_SwitchRouterTemplateIsNoOp(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "switch-noop", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "classify", Agent: &agentv1alpha1.AgentCall{Name: "cls", Text: "classify"}},
				{
					Name: "route",
					Switch: []agentv1alpha1.SwitchCase{
						{Condition: "{{tasks.classify.output}} == go", Then: "proceed"},
					},
					DependsOn: []string{"classify"},
				},
				{Name: "proceed", Agent: &agentv1alpha1.AgentCall{Name: "p", Text: "proceed"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the "route" template — should be a script template (router)
	var routeTemplate *wfv1.Template
	for i := range got.Spec.Templates {
		if got.Spec.Templates[i].Name == taskNameRoute {
			routeTemplate = &got.Spec.Templates[i]
			break
		}
	}
	if routeTemplate == nil {
		t.Fatal("missing 'route' template")
	}
	if routeTemplate.Script == nil {
		t.Fatal("switch router template should use Script")
	}
	if routeTemplate.Script.Image != "alpine:3.18" {
		t.Errorf("switch router image = %q, want alpine:3.18", routeTemplate.Script.Image)
	}
	if routeTemplate.Plugin != nil || routeTemplate.Container != nil || routeTemplate.HTTP != nil {
		t.Error("switch router should only have Script set, not Plugin/Container/HTTP")
	}
}

// =============================================================================
// Retry strategy edge cases
// =============================================================================

func TestCompile_RetryStrategyWithoutBackoff(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "retry-no-backoff", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			RetryStrategy: &agentv1alpha1.WorkflowRetryStrategy{
				Limit: 5,
				// No Backoff
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "retry me"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tmpl := got.Spec.Templates[1]
	if tmpl.RetryStrategy == nil {
		t.Fatal("expected retry strategy on template")
	}
	if tmpl.RetryStrategy.Limit == nil || tmpl.RetryStrategy.Limit.IntValue() != 5 {
		t.Errorf("retry limit should be 5, got %v", tmpl.RetryStrategy.Limit)
	}
	if tmpl.RetryStrategy.Backoff != nil {
		t.Errorf("expected no backoff, got %v", tmpl.RetryStrategy.Backoff)
	}
}

func TestCompile_TimeoutConversionAccuracy(t *testing.T) {
	tests := []struct {
		name        string
		duration    time.Duration
		wantSeconds int32
	}{
		{"30 seconds", 30 * time.Second, 30},
		{"1 minute", 1 * time.Minute, 60},
		{"5 minutes", 5 * time.Minute, 300},
		{"1 hour", 1 * time.Hour, 3600},
		{"90 seconds", 90 * time.Second, 90},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeout := metav1.Duration{Duration: tt.duration}
			awf := &agentv1alpha1.AgentWorkflow{
				ObjectMeta: metav1.ObjectMeta{Name: "timeout-test", Namespace: "default"},
				Spec: agentv1alpha1.AgentWorkflowSpec{
					Tasks: []agentv1alpha1.WorkflowTask{
						{
							Name:    "task",
							Agent:   &agentv1alpha1.AgentCall{Name: "a", Text: "x"},
							Timeout: &timeout,
						},
					},
				},
			}

			got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tmpl := got.Spec.Templates[1]
			if tmpl.ActiveDeadlineSeconds == nil {
				t.Fatal("expected ActiveDeadlineSeconds to be set")
			}
			if tmpl.ActiveDeadlineSeconds.IntVal != tt.wantSeconds {
				t.Errorf("ActiveDeadlineSeconds = %d, want %d", tmpl.ActiveDeadlineSeconds.IntVal, tt.wantSeconds)
			}
		})
	}
}

func TestCompile_WorkflowTimeoutConversion(t *testing.T) {
	timeout := metav1.Duration{Duration: 30 * time.Minute}
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "wf-timeout", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Timeout: &timeout,
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "x"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Spec.ActiveDeadlineSeconds == nil {
		t.Fatal("expected workflow ActiveDeadlineSeconds to be set")
	}
	if *got.Spec.ActiveDeadlineSeconds != 1800 {
		t.Errorf("workflow ActiveDeadlineSeconds = %d, want 1800", *got.Spec.ActiveDeadlineSeconds)
	}
}

func TestCompileToArgoWorkflow_ServiceAccountDefault(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "sa-default", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "hello"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Spec.ServiceAccountName != "flokoa-workflow" {
		t.Errorf("ServiceAccountName = %q, want %q", got.Spec.ServiceAccountName, "flokoa-workflow")
	}
	if got.Spec.AutomountServiceAccountToken == nil || !*got.Spec.AutomountServiceAccountToken {
		t.Error("AutomountServiceAccountToken should default to true")
	}
}

func TestCompileToArgoWorkflow_ServiceAccountOverride(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "sa-override", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			ServiceAccountName:           "custom-sa",
			AutomountServiceAccountToken: boolPtr(false),
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task", Agent: &agentv1alpha1.AgentCall{Name: "a", Text: "hello"}},
			},
		},
	}

	got, err := compileToArgoWorkflowTemplate(awf, CompilerOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := wantWorkflowTemplate("sa-override", "default", []wfv1.Template{
		dagTmpl(wfv1.DAGTask{Name: "task", Template: "task"}),
		{Name: "task", Plugin: makePlugin(map[string]interface{}{
			"agent":       "a",
			"message":     pluginTextMessage("hello"),
			"traceparent": "{{workflow.parameters._flokoa_traceparent}}",
		}), Outputs: pluginOutputs()},
	}, func(wft *wfv1.WorkflowTemplate) {
		wft.Spec.ServiceAccountName = "custom-sa"
		wft.Spec.AutomountServiceAccountToken = boolPtr(false)
	})

	assertDiff(t, want, got)
}

// boolPtr is a small test helper (previously provided by deleted compat shims).
func boolPtr(b bool) *bool {
	return &b
}
