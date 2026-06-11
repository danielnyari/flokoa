package agent

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func baseAgent() *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "default"},
		Spec: agentv1alpha1.AgentSpec{
			Card: agentv1alpha1.AgentCardOverride{Name: "a", Description: "d", Version: "1"},
			Spec: &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"},
		},
	}
}

func TestValidateSpecAcceptsInlineModel(t *testing.T) {
	if err := ValidateSpec(baseAgent()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateSpecAcceptsModelRef(t *testing.T) {
	agent := baseAgent()
	agent.Spec.Spec = nil
	agent.Spec.ModelRef = &agentv1alpha1.NamespacedRef{Name: "gpt"}
	if err := ValidateSpec(agent); err != nil {
		t.Fatal(err)
	}
}

func TestValidateSpecRequiresAModel(t *testing.T) {
	agent := baseAgent()
	agent.Spec.Spec = nil
	err := ValidateSpec(agent)
	if err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected model-required error, got %v", err)
	}
}

func TestValidateSpecRejectsSessionIsolation(t *testing.T) {
	agent := baseAgent()
	agent.Spec.Runtime.Isolation = agentv1alpha1.IsolationSession
	err := ValidateSpec(agent)
	if err == nil || !strings.Contains(err.Error(), "isolation") {
		t.Fatalf("expected isolation rejection, got %v", err)
	}
}

func TestValidateSpecRejectsCapabilityAttachments(t *testing.T) {
	agent := baseAgent()
	agent.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{
		{Ref: agentv1alpha1.NamespacedRef{Name: "shields"}},
	}
	err := ValidateSpec(agent)
	if err == nil || !strings.Contains(err.Error(), "08") {
		t.Fatalf("expected capability rejection pointing to roadmap 08, got %v", err)
	}
}
