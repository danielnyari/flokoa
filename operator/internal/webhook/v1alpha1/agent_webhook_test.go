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
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/spec"
)

func validAgent() *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{Name: "support", Namespace: "default"},
		Spec: agentv1alpha1.AgentSpec{
			Card: agentv1alpha1.AgentCardOverride{Name: "support", Description: "d", Version: "1"},
			Spec: &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"},
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
	agent.Spec.Runtime.Isolation = agentv1alpha1.IsolationSession

	v := &AgentCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "session") {
		t.Fatalf("expected session-tier rejection, got %v", err)
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
		agent.Spec.Spec.Capabilities = []agentv1alpha1.NativeCapabilityEntry{{Name: name}}

		v := &AgentCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), agent)
		if err == nil || !strings.Contains(err.Error(), "Capability resources") {
			t.Fatalf("capability name %q must be rejected, got %v", name, err)
		}
	}
}

func TestAgentWebhookAllowsNativeCapabilityNames(t *testing.T) {
	agent := validAgent()
	agent.Spec.Spec.Capabilities = []agentv1alpha1.NativeCapabilityEntry{
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
	agent.Spec.Spec.Capabilities = []agentv1alpha1.NativeCapabilityEntry{
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
	agent.Spec.Card.Skills = []agentv1alpha1.AgentSkill{
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
	agent.Spec.Runtime.Isolation = agentv1alpha1.IsolationSession

	v := &AgentCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), validAgent(), agent)
	if err == nil {
		t.Fatal("expected update validation to reject session isolation")
	}
}

// --- Capability attachment admission (roadmap 08) ---

const kbConfigSchema = `{
	"type": "object",
	"required": ["endpoint"],
	"properties": {
		"endpoint": {"type": "string", "pattern": "^https://"},
		"maxResults": {"type": "integer"}
	},
	"additionalProperties": false
}`

func capabilityCR(name string, mutate ...func(*agentv1alpha1.Capability)) *agentv1alpha1.Capability {
	c := &agentv1alpha1.Capability{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec: agentv1alpha1.CapabilitySpec{
			Artifact:     "ghcr.io/danielnyari/capabilities/" + name + "@sha256:" + strings.Repeat("a", 64),
			Version:      "0.1.0",
			Entrypoint:   "flokoa_" + name + ".capability:KB",
			SchemaPolicy: agentv1alpha1.SchemaPolicyStrict,
			ConfigSchema: &apiextensionsv1.JSON{Raw: []byte(kbConfigSchema)},
			Requires: agentv1alpha1.CapabilityRequires{
				Python:       "3.13",
				PydanticAI:   ">=1.100,<2",
				FlokoaRunner: ">=0.2",
			},
		},
	}
	for _, m := range mutate {
		m(c)
	}
	return c
}

func readerWith(t *testing.T, objs ...client.Object) client.Reader {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := agentv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func attachedAgent(t *testing.T, config map[string]any, capNames ...string) *agentv1alpha1.Agent {
	t.Helper()
	agent := validAgent()
	for _, name := range capNames {
		att := agentv1alpha1.CapabilityAttachment{Ref: agentv1alpha1.NamespacedRef{Name: name}}
		if config != nil {
			att.Config = jsonOf(t, config)
		}
		agent.Spec.Capabilities = append(agent.Spec.Capabilities, att)
	}
	return agent
}

func TestAgentWebhookAdmitsCompatibleCapability(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t, capabilityCR("kb"))}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	warnings, err := v.ValidateCreate(context.Background(), agent)
	if err != nil {
		t.Fatalf("compatible capability must be admitted, got %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestAgentWebhookDeniesConfigSchemaViolation(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t, capabilityCR("kb"))}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com", "maxResults": "five"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil {
		t.Fatal("config violating the published schema must be denied")
	}
	for _, want := range []string{"kb", "/maxResults"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("denial %q should contain %q", err.Error(), want)
		}
	}
}

func TestAgentWebhookDeniesMissingRequiredConfig(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t, capabilityCR("kb"))}
	// No config at all: the schema's required endpoint is missing.
	agent := attachedAgent(t, nil, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("missing required config must be denied naming the property, got %v", err)
	}
}

func TestAgentWebhookAllowsPlaceholderConfigWithSecretRef(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t, capabilityCR("kb"))}
	agent := attachedAgent(t, map[string]any{"endpoint": "${secret:kb-endpoint}"}, "kb")

	// Placeholder without a matching secretRefs entry: rejected statically.
	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "kb-endpoint") {
		t.Fatalf("placeholder without secretRefs entry must be rejected, got %v", err)
	}

	// With the matching secretRefs entry the placeholder satisfies the
	// schema's pattern constraint (shape validated, value resolves in the runner).
	agent.Spec.SecretRefs = map[string]corev1.SecretKeySelector{
		"kb-endpoint": {LocalObjectReference: corev1.LocalObjectReference{Name: "kb"}, Key: "endpoint"},
	}
	if _, err := v.ValidateCreate(context.Background(), agent); err != nil {
		t.Fatalf("placeholder config with secretRefs must be admitted, got %v", err)
	}
}

func TestAgentWebhookDeniesIncompatibleRunner(t *testing.T) {
	incompatible := capabilityCR("kb", func(c *agentv1alpha1.Capability) {
		c.Spec.Requires.PydanticAI = ">=2,<3"
	})
	v := &AgentCustomValidator{Reader: readerWith(t, incompatible)}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil {
		t.Fatal("incompatible requires tuple must be denied")
	}

	baseline, berr := spec.RunnerBaseline(spec.DefaultRunnerVersion)
	if berr != nil {
		t.Fatal(berr)
	}
	want := fmt.Sprintf(`capability "kb" requires pydantic-ai ">=2,<3" but runner %s provides pydantic-ai %q`,
		baseline.RunnerVersion, baseline.PydanticAI)
	if !strings.Contains(err.Error(), want) {
		t.Errorf("denial %q should contain %q (both tuples named)", err.Error(), want)
	}
}

func TestAgentWebhookDeniesDependencyConflict(t *testing.T) {
	a := capabilityCR("shields", func(c *agentv1alpha1.Capability) {
		c.Spec.SerializationName = "Shields"
		c.Spec.Dependencies = []string{"pydantic-ai-harness==0.2.1"}
	})
	b := capabilityCR("planning", func(c *agentv1alpha1.Capability) {
		c.Spec.SerializationName = "Planning"
		c.Spec.Dependencies = []string{"pydantic-ai-harness==0.3.0"}
	})
	v := &AgentCustomValidator{Reader: readerWith(t, a, b)}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "shields", "planning")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil {
		t.Fatal("conflicting dependency pins must be denied")
	}
	want := `capabilities "default/shields" and "default/planning" pin conflicting versions of pydantic-ai-harness (0.2.1 vs 0.3.0)`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("denial %q should contain %q", err.Error(), want)
	}
}

func TestAgentWebhookDeniesBaselineCollision(t *testing.T) {
	a := capabilityCR("kb", func(c *agentv1alpha1.Capability) {
		c.Spec.Dependencies = []string{"httpx==0.20.0"}
	})
	v := &AgentCustomValidator{Reader: readerWith(t, a)}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "baseline pins httpx==") {
		t.Fatalf("baseline pin collision must be denied naming both versions, got %v", err)
	}
}

func TestAgentWebhookDeniesStrictCapabilityWithoutSchema(t *testing.T) {
	// A strict Capability without a configSchema is malformed (the Capability
	// webhook denies it), but such a CR can exist if webhooks were disabled.
	// The Agent webhook must deny the attachment rather than silently skip
	// config validation.
	broken := capabilityCR("kb", func(c *agentv1alpha1.Capability) {
		c.Spec.ConfigSchema = nil // strict (default) + no schema
	})
	v := &AgentCustomValidator{Reader: readerWith(t, broken)}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "configSchema") {
		t.Fatalf("strict capability without configSchema must deny the attachment, got %v", err)
	}
}

func TestAgentWebhookWarnsOnPermissiveCapability(t *testing.T) {
	permissive := capabilityCR("freeform", func(c *agentv1alpha1.Capability) {
		c.Spec.SchemaPolicy = agentv1alpha1.SchemaPolicyPermissive
		c.Spec.ConfigSchema = nil
	})
	v := &AgentCustomValidator{Reader: readerWith(t, permissive)}
	agent := attachedAgent(t, map[string]any{"anything": "goes"}, "freeform")

	warnings, err := v.ValidateCreate(context.Background(), agent)
	if err != nil {
		t.Fatalf("permissive capability config must be admitted, got %v", err)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "permissive") && strings.Contains(w, "freeform") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a loud permissive warning naming the capability, got %v", warnings)
	}
}

func TestAgentWebhookWarnsOnMissingCapability(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t)}
	agent := attachedAgent(t, nil, "ghost")

	warnings, err := v.ValidateCreate(context.Background(), agent)
	if err != nil {
		t.Fatalf("missing Capability follows the ordering-tolerant reference pattern (compile re-checks), got %v", err)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "ghost") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a missing-Capability warning, got %v", warnings)
	}
}

func TestAgentWebhookDeniesDuplicateAttachments(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t, capabilityCR("kb"))}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb", "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "Duplicate") {
		t.Fatalf("duplicate capability attachments must be denied, got %v", err)
	}
}

func TestAgentWebhookUpdateRunsCapabilityChecks(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t, capabilityCR("kb"))}
	old := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")
	updated := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com", "maxResults": "five"}, "kb")

	_, err := v.ValidateUpdate(context.Background(), old, updated)
	if err == nil || !strings.Contains(err.Error(), "/maxResults") {
		t.Fatalf("update path must run the config-schema check, got %v", err)
	}
}

func TestAgentWebhookRejectsCrossNamespaceCapabilityRef(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t, capabilityCR("kb"))}
	agent := validAgent()
	agent.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{{
		Ref: agentv1alpha1.NamespacedRef{Name: "kb", Namespace: "other"},
	}}

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "cross-namespace") {
		t.Fatalf("cross-namespace capability ref must be denied generically, got %v", err)
	}
	// The denial must not echo the foreign capability's internals.
	if strings.Contains(err.Error(), "pydantic-ai") || strings.Contains(err.Error(), "sha256") {
		t.Errorf("cross-namespace denial leaked capability internals: %v", err)
	}
}

func TestAgentWebhookRejectsCollidingEntryNames(t *testing.T) {
	a := capabilityCR("kb-a") // entrypoint flokoa_kb-a.capability:KB → entry "KB"
	b := capabilityCR("kb-b") // entrypoint flokoa_kb-b.capability:KB → entry "KB"
	v := &AgentCustomValidator{Reader: readerWith(t, a, b)}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb-a", "kb-b")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "spec entry \"KB\"") {
		t.Fatalf("colliding entry names must be denied, got %v", err)
	}
}

func TestAgentWebhookRejectsNonObjectCapabilityConfig(t *testing.T) {
	permissive := capabilityCR("freeform", func(c *agentv1alpha1.Capability) {
		c.Spec.SchemaPolicy = agentv1alpha1.SchemaPolicyPermissive
		c.Spec.ConfigSchema = nil
	})
	v := &AgentCustomValidator{Reader: readerWith(t, permissive)}
	agent := validAgent()
	agent.Spec.Capabilities = []agentv1alpha1.CapabilityAttachment{{
		Ref:    agentv1alpha1.NamespacedRef{Name: "freeform"},
		Config: &apiextensionsv1.JSON{Raw: []byte(`["not","an","object"]`)},
	}}

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "must be a JSON object") {
		t.Fatalf("non-object config must be denied even for permissive capabilities, got %v", err)
	}
}

func TestAgentWebhookDeniesUnknownRunnerVersionWithCapabilities(t *testing.T) {
	v := &AgentCustomValidator{Reader: readerWith(t, capabilityCR("kb"))}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")
	agent.Spec.Runtime.RunnerVersion = "9.9.9"

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "9.9.9") {
		t.Fatalf("unknown runner version must be denied when capabilities are attached, got %v", err)
	}
}

// --- requireVerified cluster policy (roadmap 09) ---

// withVerified returns a mutator stamping the Verified condition.
func withVerified(status metav1.ConditionStatus, reason, message string) func(*agentv1alpha1.Capability) {
	return func(c *agentv1alpha1.Capability) {
		c.Status.Conditions = []metav1.Condition{{
			Type:               agentv1alpha1.CapabilityConditionVerified,
			Status:             status,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
		}}
	}
}

func requireVerifiedValidator(t *testing.T, objs ...client.Object) *AgentCustomValidator {
	t.Helper()
	return &AgentCustomValidator{
		Reader:                      readerWith(t, objs...),
		RequireVerifiedCapabilities: true,
	}
}

func TestAgentWebhookRequireVerifiedAdmitsVerifiedCapability(t *testing.T) {
	verified := capabilityCR("kb", withVerified(metav1.ConditionTrue,
		agentv1alpha1.CapabilityVerifiedReasonVerified, "cosign signature verified"))
	v := requireVerifiedValidator(t, verified)
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	if _, err := v.ValidateCreate(context.Background(), agent); err != nil {
		t.Fatalf("a verified capability must be admitted under requireVerified, got %v", err)
	}
}

func TestAgentWebhookRequireVerifiedDeniesUnverifiedCapability(t *testing.T) {
	unverified := capabilityCR("kb", withVerified(metav1.ConditionFalse,
		agentv1alpha1.CapabilityVerifiedReasonInvalid, "signature did not verify"))
	v := requireVerifiedValidator(t, unverified)
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil {
		t.Fatal("an unverified capability must be denied under requireVerified")
	}
	// Exact message content is product surface: capability, status, reason.
	for _, want := range []string{
		"Capability default/kb is not verified",
		"Verified=False",
		"reason SignatureInvalid",
		"requireVerified",
		"signature did not verify",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("denial %q should contain %q", err.Error(), want)
		}
	}
}

func TestAgentWebhookRequireVerifiedDeniesMissingSignature(t *testing.T) {
	missing := capabilityCR("kb", withVerified(metav1.ConditionFalse,
		agentv1alpha1.CapabilityVerifiedReasonMissing, "no cosign signature found"))
	v := requireVerifiedValidator(t, missing)
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "reason SignatureMissing") {
		t.Fatalf("an unsigned capability must be denied naming the reason, got %v", err)
	}
}

func TestAgentWebhookRequireVerifiedInFlightReadsAsRetryable(t *testing.T) {
	// Transient nuance (§4.6): Unknown/VerifyError must read as a retryable
	// in-flight verification, never as "invalid".
	inFlight := capabilityCR("kb", withVerified(metav1.ConditionUnknown,
		agentv1alpha1.CapabilityVerifiedReasonError, "registry unavailable"))
	v := requireVerifiedValidator(t, inFlight)
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil {
		t.Fatal("an in-flight verification must still deny under requireVerified")
	}
	for _, want := range []string{"verification is in flight", "Verified=Unknown", "reason VerifyError", "retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("denial %q should contain %q", err.Error(), want)
		}
	}
	// The aggregate error wrapper always says `is invalid`; the policy
	// message itself must not call the capability unverified or its
	// signature invalid.
	for _, reject := range []string{"not verified", "SignatureInvalid"} {
		if strings.Contains(err.Error(), reject) {
			t.Errorf("an in-flight denial must not read as %q: %q", reject, err.Error())
		}
	}
}

func TestAgentWebhookRequireVerifiedNoConditionReadsAsRetryable(t *testing.T) {
	// A Capability the controller hasn't reconciled yet has no Verified
	// condition at all — also in flight, also retryable.
	fresh := capabilityCR("kb")
	v := requireVerifiedValidator(t, fresh)
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil {
		t.Fatal("a not-yet-verified capability must be denied under requireVerified")
	}
	for _, want := range []string{"has no Verified condition yet", "verification is in flight", "retry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("denial %q should contain %q", err.Error(), want)
		}
	}
}

func TestAgentWebhookRequireVerifiedNamesDisabledVerification(t *testing.T) {
	// Misconfiguration surface: the policy is on but verification is off
	// (the chart and the operator entrypoint both refuse this; the webhook
	// still phrases it helpfully if it happens).
	disabled := capabilityCR("kb", withVerified(metav1.ConditionUnknown,
		agentv1alpha1.CapabilityVerifiedReasonDisabled, "cosign verification is not enabled on this cluster"))
	v := requireVerifiedValidator(t, disabled)
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	_, err := v.ValidateCreate(context.Background(), agent)
	if err == nil || !strings.Contains(err.Error(), "cosign verification is disabled on this cluster") {
		t.Fatalf("the disabled-verification case must be named explicitly, got %v", err)
	}
}

func TestAgentWebhookRequireVerifiedDeniesMissingCapability(t *testing.T) {
	// Without the policy a missing CR is an ordering-tolerant warning; under
	// requireVerified an unverifiable capability must never deploy.
	v := requireVerifiedValidator(t) // no Capability exists
	agent := attachedAgent(t, nil, "kb")

	warnings, err := v.ValidateCreate(context.Background(), agent)
	if err == nil {
		t.Fatal("a missing Capability must be denied under requireVerified")
	}
	for _, want := range []string{
		"Capability default/kb was not found",
		"requireVerified",
		"Verified condition to become True",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("denial %q should contain %q", err.Error(), want)
		}
	}
	for _, w := range warnings {
		if strings.Contains(w, "not found") {
			t.Errorf("the missing-CR warning must be replaced by the denial, got warning %q", w)
		}
	}
}

func TestAgentWebhookWithoutPolicyIgnoresVerifiedCondition(t *testing.T) {
	// requireVerified off (the default): an unverified capability attaches
	// fine — verification is observability, not policy.
	unverified := capabilityCR("kb", withVerified(metav1.ConditionFalse,
		agentv1alpha1.CapabilityVerifiedReasonMissing, "no cosign signature found"))
	v := &AgentCustomValidator{Reader: readerWith(t, unverified)}
	agent := attachedAgent(t, map[string]any{"endpoint": "https://kb.example.com"}, "kb")

	if _, err := v.ValidateCreate(context.Background(), agent); err != nil {
		t.Fatalf("without requireVerified the Verified condition must not gate admission, got %v", err)
	}
}
