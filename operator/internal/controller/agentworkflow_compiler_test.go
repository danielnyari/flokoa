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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestCompileToArgoWorkflow_SimpleSequential(t *testing.T) {
	timeout := metav1.Duration{Duration: 10 * time.Minute}
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workflow",
			Namespace: "default",
		},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Params: []agentv1alpha1.WorkflowParam{
				{Name: "topic", Value: "transformers"},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "research",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "researcher-agent",
						Message: "Find papers on {{params.topic}}",
					},
					Timeout: &timeout,
				},
				{
					Name: "summarize",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "summarizer-agent",
						Message: "Summarize: {{tasks.research.output}}",
					},
					DependsOn: []string{"research"},
				},
			},
		},
	}

	wf, err := compileToArgoWorkflow(awf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wf.Spec.Entrypoint != dagEntrypointName {
		t.Errorf("expected entrypoint %q, got %q", dagEntrypointName, wf.Spec.Entrypoint)
	}

	if wf.ObjectMeta.GenerateName != "test-workflow-" {
		t.Errorf("expected generateName %q, got %q", "test-workflow-", wf.ObjectMeta.GenerateName)
	}

	if len(wf.Spec.Arguments.Parameters) != 1 {
		t.Fatalf("expected 1 workflow parameter, got %d", len(wf.Spec.Arguments.Parameters))
	}
	if wf.Spec.Arguments.Parameters[0].Name != "topic" {
		t.Errorf("expected param name %q, got %q", "topic", wf.Spec.Arguments.Parameters[0].Name)
	}

	found := false
	for _, tmpl := range wf.Spec.Templates {
		if tmpl.Name == dagEntrypointName && tmpl.DAG != nil {
			found = true
			if len(tmpl.DAG.Tasks) != 2 {
				t.Errorf("expected 2 DAG tasks, got %d", len(tmpl.DAG.Tasks))
			}
			for _, dt := range tmpl.DAG.Tasks {
				if dt.Name == "summarize" {
					if len(dt.Dependencies) != 1 || dt.Dependencies[0] != "research" {
						t.Errorf("expected summarize to depend on research, got %v", dt.Dependencies)
					}
				}
			}
		}
	}
	if !found {
		t.Error("DAG entrypoint template not found")
	}

	if wf.Labels["agent.flokoa.ai/agentworkflow-name"] != "test-workflow" {
		t.Errorf("expected label, got %v", wf.Labels)
	}
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
						Message:   "Hello agent",
					},
				},
			},
		},
	}

	wf, err := compileToArgoWorkflow(awf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, tmpl := range wf.Spec.Templates {
		if tmpl.Name == "call-agent" && tmpl.Plugin != nil {
			found = true
			var pluginData map[string]json.RawMessage
			if err := json.Unmarshal(tmpl.Plugin.Value, &pluginData); err != nil {
				t.Fatalf("failed to unmarshal plugin: %v", err)
			}
			a2aData, ok := pluginData["a2a"]
			if !ok {
				t.Fatal("expected 'a2a' key in plugin spec")
			}
			var a2aSpec map[string]interface{}
			if err := json.Unmarshal(a2aData, &a2aSpec); err != nil {
				t.Fatalf("failed to unmarshal a2a spec: %v", err)
			}
			if a2aSpec["agent"] != "my-agent" {
				t.Errorf("expected agent 'my-agent', got %v", a2aSpec["agent"])
			}
			if a2aSpec["namespace"] != "agents" {
				t.Errorf("expected namespace 'agents', got %v", a2aSpec["namespace"])
			}
			if a2aSpec["message"] != "Hello agent" {
				t.Errorf("expected message 'Hello agent', got %v", a2aSpec["message"])
			}
		}
	}
	if !found {
		t.Error("agent template with plugin spec not found")
	}
}

func TestCompileToArgoWorkflow_EphemeralAgentTemplate(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "extract",
					AgentTask: &agentv1alpha1.EphemeralAgentTask{
						Entrypoint: "extract.py",
						Framework:  "pydantic-ai",
						Tools:      []string{"pdf-reader", "web-search"},
						Input:      "Extract findings",
					},
				},
			},
		},
	}

	wf, err := compileToArgoWorkflow(awf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, tmpl := range wf.Spec.Templates {
		if tmpl.Name == "extract" && tmpl.Container != nil {
			found = true
			if tmpl.Container.Image != defaultRuntimeImage {
				t.Errorf("expected default image %q, got %q", defaultRuntimeImage, tmpl.Container.Image)
			}
			envMap := make(map[string]string)
			for _, env := range tmpl.Container.Env {
				envMap[env.Name] = env.Value
			}
			if envMap["FLOKOA_FRAMEWORK"] != "pydantic-ai" {
				t.Errorf("expected framework env, got %v", envMap)
			}
			if envMap["FLOKOA_TOOLS"] != "pdf-reader,web-search" {
				t.Errorf("expected tools env, got %v", envMap)
			}
			if envMap["FLOKOA_INPUT"] != "Extract findings" {
				t.Errorf("expected input env, got %v", envMap)
			}
			if len(tmpl.Outputs.Parameters) != 1 {
				t.Errorf("expected 1 output param, got %d", len(tmpl.Outputs.Parameters))
			}
		}
	}
	if !found {
		t.Error("ephemeral agent template not found")
	}
}

func TestCompileToArgoWorkflow_RetryStrategy(t *testing.T) {
	factor := int32(2)
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			RetryStrategy: &agentv1alpha1.WorkflowRetryStrategy{
				Limit: 3,
				Backoff: &agentv1alpha1.WorkflowBackoff{
					Duration: "30s",
					Factor:   &factor,
				},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:  "task1",
					Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "hello"},
				},
			},
		},
	}

	wf, err := compileToArgoWorkflow(awf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tmpl := range wf.Spec.Templates {
		if tmpl.Name == "task1" {
			if tmpl.RetryStrategy == nil {
				t.Fatal("expected retry strategy on template")
			}
			if tmpl.RetryStrategy.Limit.IntVal != 3 {
				t.Errorf("expected retry limit 3, got %d", tmpl.RetryStrategy.Limit.IntVal)
			}
			if tmpl.RetryStrategy.Backoff == nil {
				t.Fatal("expected backoff")
			}
			if tmpl.RetryStrategy.Backoff.Duration != "30s" {
				t.Errorf("expected duration 30s, got %s", tmpl.RetryStrategy.Backoff.Duration)
			}
		}
	}
}

func TestCompileToArgoWorkflow_WorkflowTimeout(t *testing.T) {
	timeout := metav1.Duration{Duration: 1 * time.Hour}
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Timeout: &timeout,
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "hello"}},
			},
		},
	}

	wf, err := compileToArgoWorkflow(awf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wf.Spec.ActiveDeadlineSeconds == nil || *wf.Spec.ActiveDeadlineSeconds != 3600 {
		t.Errorf("expected activeDeadlineSeconds 3600, got %v", wf.Spec.ActiveDeadlineSeconds)
	}
}

func TestCompileToArgoWorkflow_FanOutFanIn(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "fan-out", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "a", Agent: &agentv1alpha1.AgentCall{Name: "agent-a", Message: "task a"}},
				{Name: "b", Agent: &agentv1alpha1.AgentCall{Name: "agent-b", Message: "task b"}},
				{Name: "c", Agent: &agentv1alpha1.AgentCall{Name: "agent-c", Message: "task c"}},
				{
					Name:      "merge",
					Agent:     &agentv1alpha1.AgentCall{Name: "agent-merge", Message: "merge results"},
					DependsOn: []string{"a", "b", "c"},
				},
			},
		},
	}

	wf, err := compileToArgoWorkflow(awf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tmpl := range wf.Spec.Templates {
		if tmpl.DAG != nil {
			for _, dt := range tmpl.DAG.Tasks {
				if dt.Name == "merge" {
					if len(dt.Dependencies) != 3 {
						t.Errorf("expected 3 dependencies, got %d", len(dt.Dependencies))
					}
				}
				if dt.Name == "a" || dt.Name == "b" || dt.Name == "c" {
					if len(dt.Dependencies) != 0 {
						t.Errorf("expected no dependencies for %s, got %v", dt.Name, dt.Dependencies)
					}
				}
			}
		}
	}
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

func TestCompileToArgoWorkflow_Condition(t *testing.T) {
	awf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "classify", Agent: &agentv1alpha1.AgentCall{Name: "classifier", Message: "classify"}},
				{
					Name:      "technical",
					Agent:     &agentv1alpha1.AgentCall{Name: "tech-support", Message: "help"},
					DependsOn: []string{"classify"},
					Condition: "{{tasks.classify.output.category}} == technical",
				},
			},
		},
	}

	wf, err := compileToArgoWorkflow(awf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tmpl := range wf.Spec.Templates {
		if tmpl.DAG != nil {
			for _, dt := range tmpl.DAG.Tasks {
				if dt.Name == "technical" {
					if dt.When == "" {
						t.Error("expected when clause on conditional task")
					}
					expected := "{{tasks.classify.outputs.parameters.result}} == technical"
					if dt.When != expected {
						t.Errorf("expected when %q, got %q", expected, dt.When)
					}
				}
			}
		}
	}
}
