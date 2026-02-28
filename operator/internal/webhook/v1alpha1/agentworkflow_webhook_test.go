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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// textMessage creates an AgentMessage pointer with a single text part.
func textMessage(text string) *agentv1alpha1.AgentMessage {
	return &agentv1alpha1.AgentMessage{
		Parts: []agentv1alpha1.MessagePart{
			{Text: &agentv1alpha1.TextPart{Text: text}},
		},
	}
}

func TestValidateAgentWorkflow_Valid(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{
				{Name: "topic", Value: "AI safety"},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "research",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "researcher-agent",
						Message: textMessage("Research {{params.topic}}"),
					},
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

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid workflow, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_DuplicateTaskNames(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("hello")}},
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent2", Message: textMessage("world")}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for duplicate task names")
	}
}

func TestValidateAgentWorkflow_NoTaskType(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "empty-task"},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for task with no type")
	}
}

func TestValidateAgentWorkflow_MultipleTaskTypes(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:      "multi",
					Agent:     &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("hello")},
					AgentTask: &agentv1alpha1.AgentTask{Type: agentv1alpha1.MarvinTaskTypeRun},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for task with multiple types")
	}
}

func TestValidateAgentWorkflow_DAGCycle(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "a", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("hello")}, DependsOn: []string{"c"}},
				{Name: "b", Agent: &agentv1alpha1.AgentCall{Name: "agent2", Message: textMessage("hello")}, DependsOn: []string{"a"}},
				{Name: "c", Agent: &agentv1alpha1.AgentCall{Name: "agent3", Message: textMessage("hello")}, DependsOn: []string{"b"}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for DAG cycle")
	}
}

func TestValidateAgentWorkflow_InvalidDependsOn(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("hello")}, DependsOn: []string{"nonexistent"}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for invalid dependsOn reference")
	}
}

func TestValidateAgentWorkflow_SelfDependency(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("hello")}, DependsOn: []string{"task1"}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for self-dependency")
	}
}

func TestValidateAgentWorkflow_InvalidExpressionReference(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("Data: {{tasks.nonexistent.output}}")}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for invalid expression referencing nonexistent task")
	}
}

func TestValidateAgentWorkflow_InvalidParamReference(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: textMessage("Topic: {{params.undefined}}")}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for invalid param reference")
	}
}

func TestValidateAgentWorkflow_InvalidSwitchTarget(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "router", Switch: []agentv1alpha1.SwitchCase{{Condition: "true", Then: "nonexistent"}}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for switch target referencing nonexistent task")
	}
}

func TestValidateAgentWorkflow_FanOutFanIn(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "proposal", Value: "Migrate to microservices"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "technical", Agent: &agentv1alpha1.AgentCall{Name: "tech-advisor", Message: textMessage("Review: {{params.proposal}}")}},
				{Name: "cost", Agent: &agentv1alpha1.AgentCall{Name: "cost-analyst", Message: textMessage("Costs: {{params.proposal}}")}},
				{Name: "risk", Agent: &agentv1alpha1.AgentCall{Name: "risk-assessor", Message: textMessage("Risks: {{params.proposal}}")}},
				{
					Name: "synthesize",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "summarizer",
						Message: textMessage("Tech: {{tasks.technical.output}} Cost: {{tasks.cost.output}} Risk: {{tasks.risk.output}}"),
					},
					DependsOn: []string{"technical", "cost", "risk"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid fan-out/fan-in workflow, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_MultiPartMessage(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "query"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "analyze",
					Agent: &agentv1alpha1.AgentCall{
						Name: "analyzer",
						Message: &agentv1alpha1.AgentMessage{
							Parts: []agentv1alpha1.MessagePart{
								{Text: &agentv1alpha1.TextPart{Text: "Analyze: {{params.query}}"}},
								{File: &agentv1alpha1.FilePart{
									File: agentv1alpha1.FileContent{URI: "s3://bucket/data.csv"},
								}},
							},
							ContextID: "ctx-1",
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid multi-part message workflow, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_InvalidPartNoType(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task1",
					Agent: &agentv1alpha1.AgentCall{
						Name: "agent1",
						Message: &agentv1alpha1.AgentMessage{
							Parts: []agentv1alpha1.MessagePart{
								{}, // empty part — no text, data, or file
							},
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for message part with no type set")
	}
}

func TestValidateAgentWorkflow_InvalidPartMultipleTypes(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task1",
					Agent: &agentv1alpha1.AgentCall{
						Name: "agent1",
						Message: &agentv1alpha1.AgentMessage{
							Parts: []agentv1alpha1.MessagePart{
								{
									Text: &agentv1alpha1.TextPart{Text: "hello"},
									File: &agentv1alpha1.FilePart{File: agentv1alpha1.FileContent{URI: "s3://x"}},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for message part with multiple types set")
	}
}

func TestValidateAgentWorkflow_ExpressionInMultiPart(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task1",
					Agent: &agentv1alpha1.AgentCall{
						Name: "agent1",
						Message: &agentv1alpha1.AgentMessage{
							Parts: []agentv1alpha1.MessagePart{
								{Text: &agentv1alpha1.TextPart{Text: "Valid plain text"}},
								{Text: &agentv1alpha1.TextPart{Text: "Bad ref: {{tasks.nonexistent.output}}"}},
							},
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for invalid expression in second text part")
	}
}

func TestValidateAgentWorkflow_AgentTextShorthand(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "topic", Value: "AI"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task1",
					Agent: &agentv1alpha1.AgentCall{
						Name: "agent1",
						Text: "Research {{params.topic}}",
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid workflow with text shorthand, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_AgentTextAndMessage(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task1",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "agent1",
						Text:    "hello",
						Message: textMessage("hello"),
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for agent with both text and message")
	}
}

func TestValidateAgentWorkflow_AgentNeitherTextNorMessage(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task1",
					Agent: &agentv1alpha1.AgentCall{
						Name: "agent1",
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for agent with neither text nor message")
	}
}

func TestValidateAgentWorkflow_TextShorthandExpressionValidation(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task1",
					Agent: &agentv1alpha1.AgentCall{
						Name: "agent1",
						Text: "Data: {{tasks.nonexistent.output}}",
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for invalid expression in text shorthand")
	}
}

func TestValidateAgentWorkflow_ArtifactReference(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "research", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Text: "find things"}},
				{
					Name:      "use-artifact",
					Agent:     &agentv1alpha1.AgentCall{Name: "agent2", Text: "Process: {{tasks.research.artifact}}"},
					DependsOn: []string{"research"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("unexpected error for artifact reference: %v", err)
	}
}

func TestValidateAgentWorkflow_TaskResponseRejected(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "call", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Text: "hello"}},
				{
					Name:      "use-response",
					Agent:     &agentv1alpha1.AgentCall{Name: "agent2", Text: "Response: {{tasks.call.taskResponse}}"},
					DependsOn: []string{"call"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for taskResponse reference (removed)")
	}
}

func TestValidateAgentWorkflow_FieldAccessExpression(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "research", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Text: "find things"}},
				{
					Name:      "extract",
					Agent:     &agentv1alpha1.AgentCall{Name: "agent2", Text: "Field: {{tasks.research.output.findings.summary}}"},
					DependsOn: []string{"research"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("unexpected error for field access expression: %v", err)
	}
}

func TestValidateAgentWorkflow_ArgoExpressionPassthrough(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:  "task1",
					Agent: &agentv1alpha1.AgentCall{Name: "agent1", Text: "{{=sprig.fromJson(tasks['x'].outputs.parameters['artifact']).parts[0].data.field}}"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("unexpected error for Argo expression passthrough: %v", err)
	}
}

func TestValidateAgentWorkflow_ValidContainerTask(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "input"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "preprocess",
					Container: &agentv1alpha1.ContainerTask{
						Image:   "python:3.12",
						Command: []string{"python", "-c"},
						Args:    []string{"print('hello')"},
						Env: []corev1.EnvVar{
							{Name: "INPUT", Value: "{{params.input}}"},
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid container task, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_ContainerAndAgentConflict(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:      "conflict",
					Agent:     &agentv1alpha1.AgentCall{Name: "agent1", Text: "hello"},
					Container: &agentv1alpha1.ContainerTask{Image: "alpine"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for task with both agent and container")
	}
}

func TestValidateAgentWorkflow_ContainerEmptyImage(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:      "no-image",
					Container: &agentv1alpha1.ContainerTask{Image: ""},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for container with empty image")
	}
}

func TestValidateAgentWorkflow_ContainerExpressionValidation(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "task1",
					Container: &agentv1alpha1.ContainerTask{
						Image: "alpine",
						Env: []corev1.EnvVar{
							{Name: "BAD", Value: "{{tasks.nonexistent.output}}"},
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for invalid expression in container env")
	}
}

func TestValidateAgentWorkflow_ValidHTTPTask(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
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
						SuccessCondition: "response.statusCode == 200",
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid HTTP task, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_HTTPWithSecretHeader(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com/data",
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

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid HTTP task with secret header, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_HTTPEmptyURL(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{URL: ""},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for HTTP task with empty URL")
	}
}

func TestValidateAgentWorkflow_HTTPHeaderBothValueAndValueFrom(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com",
						Headers: []agentv1alpha1.HTTPHeader{
							{
								Name:  "Auth",
								Value: "Bearer token",
								ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "secret"},
										Key:                  "key",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for HTTP header with both value and valueFrom")
	}
}

func TestValidateAgentWorkflow_HTTPHeaderValueFromBothSources(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL: "https://api.example.com",
						Headers: []agentv1alpha1.HTTPHeader{
							{
								Name: "Auth",
								ValueFrom: &agentv1alpha1.HTTPHeaderValueFrom{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "secret"},
										Key:                  "key",
									},
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: "cm"},
										Key:                  "key",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for HTTP header valueFrom with both secretKeyRef and configMapKeyRef")
	}
}

func TestValidateAgentWorkflow_HTTPExpressionValidation(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "fetch",
					HTTP: &agentv1alpha1.HTTPTask{
						URL:  "https://api.example.com/{{tasks.nonexistent.output}}",
						Body: "{{params.undefined}}",
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for invalid expressions in HTTP task")
	}
}

func TestValidateAgentWorkflow_HTTPAndContainerConflict(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:      "conflict",
					HTTP:      &agentv1alpha1.HTTPTask{URL: "https://example.com"},
					Container: &agentv1alpha1.ContainerTask{Image: "alpine"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for task with both HTTP and container")
	}
}

func TestValidateAgentWorkflow_MixedTaskTypesValid(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "query"}},
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
						Image:   "python:3.12",
						Command: []string{"python", "preprocess.py"},
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
						Text: "Analyze: {{tasks.preprocess.output}} for {{params.query}}",
					},
					DependsOn: []string{"preprocess"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid mixed-type workflow, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_ValidGCPDocAITask(t *testing.T) {
	chunkSize := int32(1024)
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "inputUri"}, {Name: "outputUri"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process",
					GCPDocAI: &agentv1alpha1.GCPDocAITask{
						ProcessorName: "projects/my-project/locations/us/processors/abc123",
						Location:      "us",
						InputDocuments: agentv1alpha1.GCPDocAIInputConfig{
							Documents: []agentv1alpha1.GCPDocAIGCSDocument{
								{GCSUri: "{{params.inputUri}}", MimeType: "application/pdf"},
							},
						},
						OutputConfig: agentv1alpha1.GCPDocAIOutputConfig{
							GCSUri: "{{params.outputUri}}",
						},
						ProcessOptions: &agentv1alpha1.GCPDocAIProcessOptions{
							LayoutConfig: &agentv1alpha1.GCPDocAILayoutConfig{
								ChunkingConfig: &agentv1alpha1.GCPDocAIChunkingConfig{
									ChunkSize:               &chunkSize,
									IncludeAncestorHeadings: true,
								},
							},
						},
						SkipHumanReview: true,
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid GCPDocAI task, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_GCPDocAIEmptyProcessorName(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process",
					GCPDocAI: &agentv1alpha1.GCPDocAITask{
						ProcessorName: "",
						Location:      "us",
						InputDocuments: agentv1alpha1.GCPDocAIInputConfig{
							Documents: []agentv1alpha1.GCPDocAIGCSDocument{
								{GCSUri: "gs://bucket/doc.pdf", MimeType: "application/pdf"},
							},
						},
						OutputConfig: agentv1alpha1.GCPDocAIOutputConfig{
							GCSUri: "gs://bucket/output/",
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for GCPDocAI task with empty processorName")
	}
}

func TestValidateAgentWorkflow_GCPDocAINoInputDocuments(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process",
					GCPDocAI: &agentv1alpha1.GCPDocAITask{
						ProcessorName:  "projects/p/locations/us/processors/x",
						Location:       "us",
						InputDocuments: agentv1alpha1.GCPDocAIInputConfig{},
						OutputConfig: agentv1alpha1.GCPDocAIOutputConfig{
							GCSUri: "gs://bucket/output/",
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for GCPDocAI task with no input documents")
	}
}

func TestValidateAgentWorkflow_GCPDocAIEmptyOutputGCSUri(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process",
					GCPDocAI: &agentv1alpha1.GCPDocAITask{
						ProcessorName: "projects/p/locations/us/processors/x",
						Location:      "us",
						InputDocuments: agentv1alpha1.GCPDocAIInputConfig{
							GCSPrefix: &agentv1alpha1.GCPDocAIGCSPrefix{
								GCSUriPrefix: "gs://bucket/input/",
							},
						},
						OutputConfig: agentv1alpha1.GCPDocAIOutputConfig{
							GCSUri: "",
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for GCPDocAI task with empty output gcsUri")
	}
}

func TestValidateAgentWorkflow_GCPDocAIAndAgentConflict(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:  "conflict",
					Agent: &agentv1alpha1.AgentCall{Name: "agent1", Text: "hello"},
					GCPDocAI: &agentv1alpha1.GCPDocAITask{
						ProcessorName: "projects/p/locations/us/processors/x",
						Location:      "us",
						InputDocuments: agentv1alpha1.GCPDocAIInputConfig{
							Documents: []agentv1alpha1.GCPDocAIGCSDocument{
								{GCSUri: "gs://bucket/doc.pdf", MimeType: "application/pdf"},
							},
						},
						OutputConfig: agentv1alpha1.GCPDocAIOutputConfig{
							GCSUri: "gs://bucket/output/",
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for task with both agent and gcpDocAI")
	}
}

func TestValidateAgentWorkflow_GCPDocAIExpressionValidation(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process",
					GCPDocAI: &agentv1alpha1.GCPDocAITask{
						ProcessorName: "projects/p/locations/us/processors/x",
						Location:      "us",
						InputDocuments: agentv1alpha1.GCPDocAIInputConfig{
							Documents: []agentv1alpha1.GCPDocAIGCSDocument{
								{GCSUri: "{{tasks.nonexistent.output}}", MimeType: "application/pdf"},
							},
						},
						OutputConfig: agentv1alpha1.GCPDocAIOutputConfig{
							GCSUri: "{{params.undefined}}",
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for invalid expressions in GCPDocAI task")
	}
}

func TestValidateAgentWorkflow_GCPDocAIWithGCSPrefix(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "prefix"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process",
					GCPDocAI: &agentv1alpha1.GCPDocAITask{
						ProcessorName: "projects/p/locations/eu/processors/y",
						Location:      "eu",
						InputDocuments: agentv1alpha1.GCPDocAIInputConfig{
							GCSPrefix: &agentv1alpha1.GCPDocAIGCSPrefix{
								GCSUriPrefix: "{{params.prefix}}",
							},
						},
						OutputConfig: agentv1alpha1.GCPDocAIOutputConfig{
							GCSUri:    "gs://bucket/output/",
							FieldMask: "text,pages.layout",
						},
					},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid GCPDocAI task with GCS prefix, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_GCPDocAIPipelineWithAgent(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{{Name: "inputUri"}, {Name: "outputUri"}},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "process-document",
					GCPDocAI: &agentv1alpha1.GCPDocAITask{
						ProcessorName: "projects/p/locations/us/processors/x",
						Location:      "us",
						InputDocuments: agentv1alpha1.GCPDocAIInputConfig{
							Documents: []agentv1alpha1.GCPDocAIGCSDocument{
								{GCSUri: "{{params.inputUri}}", MimeType: "application/pdf"},
							},
						},
						OutputConfig: agentv1alpha1.GCPDocAIOutputConfig{
							GCSUri: "{{params.outputUri}}",
						},
					},
				},
				{
					Name: "analyze",
					Agent: &agentv1alpha1.AgentCall{
						Name: "analyzer",
						Text: "Analyze: {{tasks.process-document.output}}",
					},
					DependsOn: []string{"process-document"},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid GCPDocAI -> agent pipeline, got error: %v", err)
	}
}
