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
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validAgent() *Agent {
	return &Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "support", Namespace: "default"},
		Spec: AgentSpec{
			Card: AgentCardOverride{Name: "support", Description: "d", Version: "1"},
			Spec: &AgentSpecFragment{Model: "openai:gpt-5-mini"},
		},
	}
}

func jsonOf(t *testing.T, v any) *apiextensionsv1.JSON {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return &apiextensionsv1.JSON{Raw: raw}
}

func TestAgentWebhookAcceptsValidAgent(t *testing.T) {
	v := &AgentCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validAgent()); err != nil {
		t.Fatal(err)
	}
}

func TestAgentWebhookRejectsSessionIsolation(t *testing.T) {
	agent := validAgent()
	agent.Spec.Runtime.Isolation = IsolationSession

	v := &AgentCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("expected session-tier rejection, got %v", err)
	}
}

func TestAgentWebhookRejectsCapabilityAttachments(t *testing.T) {
	agent := validAgent()
	agent.Spec.Capabilities = []CapabilityAttachment{{Ref: NamespacedRef{Name: "shields"}}}

	v := &AgentCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "roadmap 08") {
		t.Fatalf("expected Capability rejection, got %v", err)
	}
}

func TestAgentWebhookRejectsClassPathCapabilityNames(t *testing.T) {
	cases := []string{
		"pydantic_ai_harness.shields:Shields",
		"some.module.Cap",
		"flokoa.platform/telemetry",
	}
	for _, name := range cases {
		agent := validAgent()
		agent.Spec.Spec.Capabilities = []NativeCapabilityEntry{{Name: name}}

		v := &AgentCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), agent)
		if err == nil || !strings.Contains(err.Error(), "Capability resources") {
			t.Fatalf("capability name %q must be rejected, got %v", name, err)
		}
	}
}

func TestAgentWebhookAllowsNativeCapabilityNames(t *testing.T) {
	agent := validAgent()
	agent.Spec.Spec.Capabilities = []NativeCapabilityEntry{
		{Name: "WebSearch"},
		{Name: "MCP", Config: jsonOf(t, map[string]any{"url": "http://tools.default.svc.cluster.local/mcp"})},
	}

	v := &AgentCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), agent); err != nil {
		t.Fatal(err)
	}
}

func TestAgentWebhookRequiresMatchingSecretRefs(t *testing.T) {
	agent := validAgent()
	agent.Spec.Spec.Capabilities = []NativeCapabilityEntry{
		{Name: "MCP", Config: jsonOf(t, map[string]any{
			"url":     "http://tools.default.svc.cluster.local/mcp",
			"headers": map[string]string{"Authorization": "${secret:kb-token}"},
		})},
	}

	v := &AgentCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "kb-token") {
		t.Fatalf("placeholder without secretRefs entry must be rejected, got %v", err)
	}

	agent.Spec.SecretRefs = map[string]corev1.SecretKeySelector{
		"kb-token": {LocalObjectReference: corev1.LocalObjectReference{Name: "kb"}, Key: "token"},
	}
	if _, err := v.ValidateCreate(context.Background(), agent); err != nil {
		t.Fatalf("matching secretRefs entry must pass, got %v", err)
	}
}

func TestAgentWebhookRejectsBadSecretRefNames(t *testing.T) {
	agent := validAgent()
	agent.Spec.SecretRefs = map[string]corev1.SecretKeySelector{
		"bad name!": {LocalObjectReference: corev1.LocalObjectReference{Name: "s"}, Key: "k"},
	}

	v := &AgentCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil {
		t.Fatal("expected invalid secret ref name to be rejected")
	}
}

func TestAgentWebhookRejectsDuplicateSkillIDs(t *testing.T) {
	agent := validAgent()
	agent.Spec.Card.Skills = []AgentSkill{
		{ID: "s1", Name: "one", Description: "d", Tags: []string{"t"}},
		{ID: "s1", Name: "two", Description: "d", Tags: []string{"t"}},
	}

	v := &AgentCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "s1") {
		t.Fatalf("expected duplicate skill rejection, got %v", err)
	}
}

func TestAgentWebhookUpdateValidates(t *testing.T) {
	agent := validAgent()
	agent.Spec.Runtime.Isolation = IsolationSession

	v := &AgentCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), validAgent(), agent)
	if err == nil {
		t.Fatal("expected update validation to reject session isolation")
	}
}
