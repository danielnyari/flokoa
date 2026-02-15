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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestValidateAgentWorkflow_Valid(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Engine: agentv1alpha1.EngineTypeArgo,
			Params: []agentv1alpha1.WorkflowParam{
				{Name: "topic", Value: "AI safety"},
			},
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name: "research",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "researcher-agent",
						Message: "Research {{params.topic}}",
					},
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

	if err := ValidateAgentWorkflow(wf); err != nil {
		t.Errorf("expected valid workflow, got error: %v", err)
	}
}

func TestValidateAgentWorkflow_DuplicateTaskNames(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "hello"}},
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent2", Message: "world"}},
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
					Agent:     &agentv1alpha1.AgentCall{Name: "agent1", Message: "hello"},
					AgentTask: &agentv1alpha1.EphemeralAgentTask{Entrypoint: "run.py"},
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
				{Name: "a", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "hello"}, DependsOn: []string{"c"}},
				{Name: "b", Agent: &agentv1alpha1.AgentCall{Name: "agent2", Message: "hello"}, DependsOn: []string{"a"}},
				{Name: "c", Agent: &agentv1alpha1.AgentCall{Name: "agent3", Message: "hello"}, DependsOn: []string{"b"}},
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
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "hello"}, DependsOn: []string{"nonexistent"}},
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
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "hello"}, DependsOn: []string{"task1"}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for self-dependency")
	}
}

func TestValidateAgentWorkflow_WaitForSignalOnArgo(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Engine: agentv1alpha1.EngineTypeArgo,
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "wait", WaitForSignal: &agentv1alpha1.WaitForSignalSpec{Name: "approval"}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for waitForSignal on argo engine")
	}
}

func TestValidateAgentWorkflow_LoopOnArgo(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Engine: agentv1alpha1.EngineTypeArgo,
			Tasks: []agentv1alpha1.WorkflowTask{
				{
					Name:  "loopy",
					Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "hello"},
					Loop:  &agentv1alpha1.LoopSpec{Until: "{{tasks.loopy.output.done}} == true", MaxIterations: 5},
				},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for loop on argo engine")
	}
}

func TestValidateAgentWorkflow_TemporalNotSupported(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Engine: agentv1alpha1.EngineTypeTemporal,
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "wait", WaitForSignal: &agentv1alpha1.WaitForSignalSpec{Name: "approval"}},
			},
		},
	}

	if err := ValidateAgentWorkflow(wf); err == nil {
		t.Error("expected error for temporal engine (not yet supported)")
	}
}

func TestValidateAgentWorkflow_InvalidExpressionReference(t *testing.T) {
	wf := &agentv1alpha1.AgentWorkflow{
		ObjectMeta: metav1.ObjectMeta{Name: "test-wf"},
		Spec: agentv1alpha1.AgentWorkflowSpec{
			Tasks: []agentv1alpha1.WorkflowTask{
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "Data: {{tasks.nonexistent.output}}"}},
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
				{Name: "task1", Agent: &agentv1alpha1.AgentCall{Name: "agent1", Message: "Topic: {{params.undefined}}"}},
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
				{Name: "technical", Agent: &agentv1alpha1.AgentCall{Name: "tech-advisor", Message: "Review: {{params.proposal}}"}},
				{Name: "cost", Agent: &agentv1alpha1.AgentCall{Name: "cost-analyst", Message: "Costs: {{params.proposal}}"}},
				{Name: "risk", Agent: &agentv1alpha1.AgentCall{Name: "risk-assessor", Message: "Risks: {{params.proposal}}"}},
				{
					Name: "synthesize",
					Agent: &agentv1alpha1.AgentCall{
						Name:    "summarizer",
						Message: "Tech: {{tasks.technical.output}} Cost: {{tasks.cost.output}} Risk: {{tasks.risk.output}}",
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
