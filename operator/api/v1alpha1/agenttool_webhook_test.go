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
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func validTool() *AgentTool {
	return &AgentTool{
		ObjectMeta: metav1.ObjectMeta{Name: "kb", Namespace: "default"},
		Spec: AgentToolSpec{
			Type:       AgentToolTypeMCP,
			ServiceRef: &ServiceRef{Name: "kb-tools", Port: int32Ptr(8080)},
		},
	}
}

func int32Ptr(v int32) *int32 { return &v }

func TestAgentToolWebhookAcceptsValidMCPTool(t *testing.T) {
	v := &AgentToolCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validTool()); err != nil {
		t.Fatal(err)
	}
}

func TestAgentToolWebhookRejectsOpenAPIWithMigrationPointer(t *testing.T) {
	tool := validTool()
	tool.Spec.Type = AgentToolTypeOpenAPI

	v := &AgentToolCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), tool)
	if err == nil || !strings.Contains(err.Error(), "MCP adapter") {
		t.Fatalf("expected retirement message with migration pointer, got %v", err)
	}
}

func TestAgentToolWebhookRequiresURLOrServiceRef(t *testing.T) {
	tool := validTool()
	tool.Spec.ServiceRef = nil

	v := &AgentToolCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), tool); err == nil {
		t.Fatal("expected rejection when neither url nor serviceRef is set")
	}

	tool.Spec.URL = "http://tools.example.com/mcp"
	tool.Spec.ServiceRef = &ServiceRef{Name: "x"}
	if _, err := v.ValidateCreate(context.Background(), tool); err == nil {
		t.Fatal("expected rejection when both url and serviceRef are set")
	}
}

func TestAgentToolWebhookRejectsNonHTTPURL(t *testing.T) {
	tool := validTool()
	tool.Spec.ServiceRef = nil
	tool.Spec.URL = "file:///etc/passwd"

	v := &AgentToolCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), tool); err == nil {
		t.Fatal("expected non-HTTP URL rejection")
	}
}

func TestAgentToolWebhookPathRules(t *testing.T) {
	tool := validTool()
	tool.Spec.Path = "mcp"

	v := &AgentToolCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), tool); err == nil {
		t.Fatal("expected rejection of relative path")
	}

	tool = validTool()
	tool.Spec.ServiceRef = nil
	tool.Spec.URL = "http://tools.example.com/mcp"
	tool.Spec.Path = "/mcp"
	if _, err := v.ValidateCreate(context.Background(), tool); err == nil {
		t.Fatal("expected rejection of path with url")
	}
}

func TestAgentToolWebhookPortExclusivity(t *testing.T) {
	tool := validTool()
	tool.Spec.ServiceRef.PortName = "http"

	v := &AgentToolCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), tool); err == nil {
		t.Fatal("expected rejection when both port and portName are set")
	}
}

func TestAgentToolWebhookHeaderCollisions(t *testing.T) {
	tool := validTool()
	tool.Spec.Headers = map[string]string{"Authorization": "static"}
	tool.Spec.HeaderSecrets = []SecretHeader{{
		Name: "authorization",
		SecretRef: corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "s"},
			Key:                  "k",
		},
	}}

	v := &AgentToolCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), tool); err == nil {
		t.Fatal("expected case-insensitive header collision rejection")
	}
}

func TestAgentToolWebhookSSETransportWarning(t *testing.T) {
	tool := validTool()
	tool.Spec.ServiceRef = nil
	tool.Spec.URL = "http://tools.example.com/mcp"
	tool.Spec.Transport = MCPTransportSSE

	v := &AgentToolCustomValidator{}
	warnings, err := v.ValidateCreate(context.Background(), tool)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "/sse") {
		t.Fatalf("expected sse/url mismatch warning, got %v", warnings)
	}
}
