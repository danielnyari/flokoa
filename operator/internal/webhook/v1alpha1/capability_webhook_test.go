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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/spec"
)

// tagOnlyArtifact is a non-digest-pinned artifact reference, rejected by both
// the CRD pattern and the Capability webhook.
const tagOnlyArtifact = "ghcr.io/danielnyari/capabilities/kb:v0.1.0"

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
	c.Spec.Artifact = tagOnlyArtifact

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
	c.Spec.Artifact = tagOnlyArtifact

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), capabilityCR("kb"), c)
	if err == nil {
		t.Fatal("update validation must apply the same checks")
	}
}

// --- DNS-label name rule (roadmap 09) ---

func TestCapabilityWebhookRequiresDNSLabelName(t *testing.T) {
	// Runner pods derive container/volume names from cap-<name>, so the CR
	// name must be a DNS label — stricter than the DNS-subdomain rule object
	// names get by default.
	cases := []string{
		"my.cap",                // dots: valid object name, invalid DNS label
		"My-Cap",                // uppercase
		strings.Repeat("x", 64), // longer than 63
	}
	v := &CapabilityCustomValidator{}
	for _, name := range cases {
		c := capabilityCR("kb")
		c.Name = name
		_, err := v.ValidateCreate(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "DNS label") {
			t.Fatalf("Capability name %q must be denied naming the DNS-label rule, got %v", name, err)
		}
		if !strings.Contains(err.Error(), "cap-<name>") {
			t.Fatalf("the denial should explain the cap-<name> derivation, got %v", err)
		}
	}
}

func TestCapabilityWebhookAcceptsDNSLabelName(t *testing.T) {
	v := &CapabilityCustomValidator{}
	c := capabilityCR("kb")
	c.Name = "echo-tools-2"
	if _, err := v.ValidateCreate(context.Background(), c); err != nil {
		t.Fatalf("a DNS-label name must be admitted, got %v", err)
	}
}

func TestCapabilityWebhookNameRuleIsCreateOnly(t *testing.T) {
	// Names are immutable, so update does not re-check them: a pre-rule CR
	// with a non-label name must stay editable.
	v := &CapabilityCustomValidator{}
	c := capabilityCR("kb")
	c.Name = "legacy.name"
	if _, err := v.ValidateUpdate(context.Background(), c, c); err != nil {
		t.Fatalf("update must not re-check the immutable name, got %v", err)
	}
}

// --- per-source artifact required/forbidden rule (source tiers PR1) ---

func TestCapabilityWebhookBuiltinForbidsArtifact(t *testing.T) {
	// source: builtin must not set an artifact — built-in capabilities are
	// baked into the runner image and never delivered.
	c := capabilityCR("kb")
	c.Spec.Source = agentv1alpha1.CapabilitySourceBuiltin
	// capabilityCR sets a digest-pinned artifact by default.

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil {
		t.Fatal("source builtin with an artifact must be denied")
	}
	for _, want := range []string{"source builtin must not set artifact", "baked into the runner image"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("denial %q should contain %q", err.Error(), want)
		}
	}
}

// builtinOpenAPICR builds a source: builtin Capability CR that matches the
// operator's embedded built-in metadata for flokoa-openapi (the shape the chart
// ships). Mutators can corrupt it to test the metadata-match denial.
func builtinOpenAPICR(t *testing.T, mutate ...func(*agentv1alpha1.Capability)) *agentv1alpha1.Capability {
	t.Helper()
	info, ok := spec.BuiltinCapability(spec.DefaultRunnerVersion, "flokoa-openapi")
	if !ok {
		t.Fatalf("embedded baseline missing flokoa-openapi built-in — run `make runner-contract`")
	}
	c := &agentv1alpha1.Capability{
		ObjectMeta: metav1.ObjectMeta{Name: "flokoa-openapi", Namespace: "default"},
		Spec: agentv1alpha1.CapabilitySpec{
			Source:            agentv1alpha1.CapabilitySourceBuiltin,
			Version:           spec.DefaultRunnerVersion,
			Entrypoint:        info.Entrypoint,
			SerializationName: info.SerializationName,
			SchemaPolicy:      agentv1alpha1.SchemaPolicyStrict,
			ConfigSchema:      &apiextensionsv1.JSON{Raw: info.ConfigSchema},
			Requires: agentv1alpha1.CapabilityRequires{
				Python:       info.Requires.Python,
				PydanticAI:   info.Requires.PydanticAI,
				FlokoaRunner: info.Requires.FlokoaRunner,
			},
		},
	}
	for _, m := range mutate {
		m(c)
	}
	return c
}

func TestCapabilityWebhookBuiltinMatchingMetadataAdmitted(t *testing.T) {
	// A source: builtin CR with no artifact that matches the embedded built-in
	// metadata (the chart-shipped shape) is admitted.
	v := &CapabilityCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), builtinOpenAPICR(t)); err != nil {
		t.Fatalf("a built-in CR matching the embedded metadata must be admitted, got %v", err)
	}
}

func TestCapabilityWebhookBuiltinUnknownNameDenied(t *testing.T) {
	// A source: builtin CR whose name is not a real built-in is denied — the
	// no-trust-me rule that closes the spoofing hole.
	c := capabilityCR("kb")
	c.Spec.Source = agentv1alpha1.CapabilitySourceBuiltin
	c.Spec.Artifact = ""

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil {
		t.Fatal("a fake source: builtin CR must be denied")
	}
	if !strings.Contains(err.Error(), "not a built-in capability") {
		t.Errorf("denial %q should explain the name is not a built-in", err.Error())
	}
}

func TestCapabilityWebhookBuiltinMismatchedEntrypointDenied(t *testing.T) {
	// A real built-in name but a tampered entrypoint must be denied: the CR's
	// entrypoint must match the embedded metadata.
	c := builtinOpenAPICR(t, func(c *agentv1alpha1.Capability) {
		c.Spec.Entrypoint = "evil_module.capability:Backdoor"
	})

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil {
		t.Fatal("a built-in CR with a mismatched entrypoint must be denied")
	}
	if !strings.Contains(err.Error(), "does not match the built-in metadata") {
		t.Errorf("denial %q should explain the metadata mismatch", err.Error())
	}
}

func TestCapabilityWebhookBuiltinMismatchedSchemaDenied(t *testing.T) {
	// A real built-in with a tampered (permissive) configSchema must be denied:
	// the schema must match the embedded metadata (anti-spoofing defense in
	// depth).
	c := builtinOpenAPICR(t, func(c *agentv1alpha1.Capability) {
		c.Spec.ConfigSchema = &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)}
	})

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil {
		t.Fatal("a built-in CR with a mismatched configSchema must be denied")
	}
	if !strings.Contains(err.Error(), "configSchema does not match") {
		t.Errorf("denial %q should explain the schema mismatch", err.Error())
	}
}

func TestCapabilityWebhookBuiltinOmittedSchemaDenied(t *testing.T) {
	// A real built-in name + entrypoint, but the CR omits configSchema while the
	// embedded metadata HAS one. This must be denied (not skipped): a schema-less
	// CR would otherwise dodge the schema comparison and pass — built-in CRs are
	// generated by the chart and always carry the schema.
	c := builtinOpenAPICR(t, func(c *agentv1alpha1.Capability) {
		c.Spec.ConfigSchema = nil
		// strict policy requires a schema; switch to permissive so the *only*
		// failure surfaced is the built-in omitted-schema gate, not the
		// strict-requires-schema rule.
		c.Spec.SchemaPolicy = agentv1alpha1.SchemaPolicyPermissive
	})

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil {
		t.Fatal("a built-in CR omitting configSchema (while the built-in has one) must be denied")
	}
	if !strings.Contains(err.Error(), "the CR omits it") {
		t.Errorf("denial %q should explain the CR omitted the configSchema", err.Error())
	}
	if !strings.Contains(err.Error(), "do not hand-edit them") {
		t.Errorf("denial %q should point at the chart-generated built-in CRs", err.Error())
	}
}

func TestCapabilityWebhookBuiltinMismatchedSerializationNameDenied(t *testing.T) {
	// A real built-in name + real entrypoint, but a different serializationName
	// (the name the compiled spec uses), must be denied: any mismatch in the
	// identity triple (entrypoint, serializationName, schemaDigest) is a denial.
	c := builtinOpenAPICR(t, func(c *agentv1alpha1.Capability) {
		c.Spec.SerializationName = "evil.OpenAPI"
	})

	v := &CapabilityCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), c)
	if err == nil {
		t.Fatal("a built-in CR with a mismatched serializationName must be denied")
	}
	if !strings.Contains(err.Error(), "does not match the built-in metadata") {
		t.Errorf("denial %q should explain the metadata mismatch", err.Error())
	}
	if !strings.Contains(err.Error(), "serialization name") {
		t.Errorf("denial %q should name the field that mismatched", err.Error())
	}
}

func TestCapabilityWebhookNonBuiltinRequiresArtifact(t *testing.T) {
	// image | git | pypi require an artifact: an empty artifact (which the
	// widened CRD pattern allows structurally) is denied by the webhook.
	for _, source := range []agentv1alpha1.CapabilitySource{
		agentv1alpha1.CapabilitySourceImage,
		agentv1alpha1.CapabilitySourceGit,
		agentv1alpha1.CapabilitySourcePypi,
	} {
		c := capabilityCR("kb")
		c.Spec.Source = source
		c.Spec.Artifact = ""

		v := &CapabilityCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "digest-pinned") {
			t.Fatalf("source %q without an artifact must be denied as not digest-pinned, got %v", source, err)
		}
	}
}

func TestCapabilityWebhookNonBuiltinRequiresDigestPinnedArtifact(t *testing.T) {
	// A non-digest (tag-only) artifact is denied for every artifact tier.
	for _, source := range []agentv1alpha1.CapabilitySource{
		agentv1alpha1.CapabilitySourceImage,
		agentv1alpha1.CapabilitySourceGit,
		agentv1alpha1.CapabilitySourcePypi,
	} {
		c := capabilityCR("kb")
		c.Spec.Source = source
		c.Spec.Artifact = tagOnlyArtifact

		v := &CapabilityCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), c)
		if err == nil || !strings.Contains(err.Error(), "digest-pinned") {
			t.Fatalf("source %q with a tag-only artifact must be denied, got %v", source, err)
		}
	}
}

func TestCapabilityWebhookEmptySourceTreatedAsImage(t *testing.T) {
	// A CR with no source (the direct-call path that bypasses CRD defaulting)
	// is evaluated as the image tier it always was: a digest-pinned artifact
	// is required and admitted.
	c := capabilityCR("kb")
	c.Spec.Source = ""

	v := &CapabilityCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), c); err != nil {
		t.Fatalf("empty source with a digest-pinned artifact must be admitted, got %v", err)
	}

	c.Spec.Artifact = ""
	if _, err := v.ValidateCreate(context.Background(), c); err == nil {
		t.Fatal("empty source (image) without an artifact must be denied")
	}
}
