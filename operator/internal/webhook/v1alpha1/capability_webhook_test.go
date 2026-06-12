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

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestCapabilityWebhookAcceptsValidCapability(t *testing.T) {
	v := &CapabilityCustomValidator{}
	warnings, err := v.ValidateCreate(context.Background(), capabilityCR("kb"))
	if err != nil {
		t.Fatalf("valid strict capability must be admitted, got %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestCapabilityWebhookRequiresDigestPinnedArtifact(t *testing.T) {
	c := capabilityCR("kb")
	c.Spec.Artifact = "ghcr.io/danielnyari/capabilities/kb:v0.1.0"

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "digest-pinned") {
		t.Fatalf("tag-only artifact must be denied as not digest-pinned, got %v", err)
	}
}

func TestCapabilityWebhookRequiresEntrypointFormat(t *testing.T) {
	c := capabilityCR("kb")
	c.Spec.Entrypoint = "flokoa_kb.capability"

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "module:attr") {
		t.Fatalf("entrypoint without :attr must be denied, got %v", err)
	}
}

func TestCapabilityWebhookStrictRequiresConfigSchema(t *testing.T) {
	c := capabilityCR("kb")
	c.Spec.ConfigSchema = nil

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil {
		t.Fatal("strict policy without configSchema must be denied")
	}
	for _, want := range []string{"strict", "permissive"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("denial %q should mention %q (the policy and the loud opt-out)", err.Error(), want)
		}
	}
}

func TestCapabilityWebhookPermissiveWarnsLoudly(t *testing.T) {
	c := capabilityCR("kb")
	c.Spec.SchemaPolicy = agentv1alpha1.SchemaPolicyPermissive
	c.Spec.ConfigSchema = nil

	v := &CapabilityCustomValidator{}
	warnings, err := v.ValidateCreate(context.Background(), c)
	if err != nil {
		t.Fatalf("permissive capability must be admitted, got %v", err)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "permissive") {
			found = true
		}
	}
	if !found {
		t.Fatalf("permissive must be loudly surfaced as a warning, got %v", warnings)
	}
}

func TestCapabilityWebhookRejectsUncompilableSchema(t *testing.T) {
	c := capabilityCR("kb")
	c.Spec.ConfigSchema.Raw = []byte(`{"type": 12}`)

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "configSchema") {
		t.Fatalf("uncompilable schema must be denied, got %v", err)
	}
}

func TestCapabilityWebhookRejectsInvalidRequiresSpecifier(t *testing.T) {
	c := capabilityCR("kb")
	c.Spec.Requires.PydanticAI = "not-a-specifier"

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "not-a-specifier") {
		t.Fatalf("invalid PEP 440 specifier must be denied naming it, got %v", err)
	}
}

func TestCapabilityWebhookRejectsInvalidDependencyPin(t *testing.T) {
	c := capabilityCR("kb")
	c.Spec.Dependencies = []string{"httpx>=0.20"}

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "name==version") {
		t.Fatalf("range dependency must be denied (pins only), got %v", err)
	}
}

func TestCapabilityWebhookUpdateValidates(t *testing.T) {
	c := capabilityCR("kb")
	c.Spec.Artifact = "ghcr.io/danielnyari/capabilities/kb:v0.1.0"

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), capabilityCR("kb"), c)
	if err == nil {
		t.Fatal("update validation must apply the same checks")
	}
}
