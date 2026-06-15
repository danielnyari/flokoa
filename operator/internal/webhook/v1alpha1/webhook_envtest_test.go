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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/spec"
)

// These specs drive the registered Agent/Capability validating webhooks
// through the real envtest API server, proving the config/webhook manifest
// paths route to the handlers and that CRD-level behavior (schemaPolicy
// default, attachment-config pruning, admission warnings) is exercised — the
// direct-call unit tests bypass all of that.
var _ = Describe("Capability and Agent webhooks (envtest)", func() {
	const ns = "default"

	uniqueName := func(prefix string) string {
		return fmt.Sprintf("%s-%d", prefix, GinkgoRandomSeed()+int64(GinkgoParallelProcess()))
	}

	validCapability := func(name string) *agentv1alpha1.Capability {
		return &agentv1alpha1.Capability{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: agentv1alpha1.CapabilitySpec{
				Artifact:     "ghcr.io/danielnyari/capabilities/" + name + "@sha256:" + strings.Repeat("a", 64),
				Version:      "0.1.0",
				Entrypoint:   "flokoa_kb.capability:KB",
				ConfigSchema: &apiextensionsv1.JSON{Raw: []byte(`{"type":"object","required":["endpoint"],"properties":{"endpoint":{"type":"string"}}}`)},
				Requires:     agentv1alpha1.CapabilityRequires{FlokoaRunner: ">=0.2"},
			},
		}
	}

	It("rejects a Capability with a tag-only artifact via the API server", func() {
		// The CRD digest pattern catches this at the schema layer (before the
		// webhook); either way admission must reject it.
		c := validCapability(uniqueName("cap-tag"))
		c.Spec.Artifact = tagOnlyArtifact

		err := k8sClient.Create(ctx, c)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "want an admission Invalid error, got %v", err)
	})

	It("defaults schemaPolicy to strict and then requires a configSchema (webhook path)", func() {
		c := validCapability(uniqueName("cap-default"))
		c.Spec.ConfigSchema = nil // strict (defaulted) + no schema → denied

		err := k8sClient.Create(ctx, c)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "want Invalid, got %v", err)
		Expect(err.Error()).To(ContainSubstring("configSchema"))
	})

	It("admits a valid Capability and denies an Agent whose attachment config violates its schema", func() {
		capName := uniqueName("cap-ok")
		Expect(k8sClient.Create(ctx, validCapability(capName))).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &agentv1alpha1.Capability{ObjectMeta: metav1.ObjectMeta{Name: capName, Namespace: ns}})
		})

		agent := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: uniqueName("agent-bad"), Namespace: ns},
			Spec: agentv1alpha1.AgentSpec{
				Card: agentv1alpha1.AgentCardOverride{Name: "a", Description: "d", Version: "1", Skills: []agentv1alpha1.AgentSkill{}},
				Spec: &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"},
				Capabilities: []agentv1alpha1.CapabilityAttachment{{
					Ref:    agentv1alpha1.NamespacedRef{Name: capName},
					Config: &apiextensionsv1.JSON{Raw: []byte(`{"endpoint":42}`)},
				}},
			},
		}
		err := k8sClient.Create(ctx, agent)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "want Invalid, got %v", err)
		Expect(err.Error()).To(ContainSubstring(capName))
	})

	It("admits an Agent whose attachment config satisfies the schema", func() {
		capName := uniqueName("cap-good")
		Expect(k8sClient.Create(ctx, validCapability(capName))).To(Succeed())
		agentName := uniqueName("agent-good")
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &agentv1alpha1.Agent{ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: ns}})
			_ = k8sClient.Delete(ctx, &agentv1alpha1.Capability{ObjectMeta: metav1.ObjectMeta{Name: capName, Namespace: ns}})
		})

		agent := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: ns},
			Spec: agentv1alpha1.AgentSpec{
				Card: agentv1alpha1.AgentCardOverride{Name: "a", Description: "d", Version: "1", Skills: []agentv1alpha1.AgentSkill{}},
				Spec: &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"},
				Capabilities: []agentv1alpha1.CapabilityAttachment{{
					Ref:    agentv1alpha1.NamespacedRef{Name: capName},
					Config: &apiextensionsv1.JSON{Raw: []byte(`{"endpoint":"https://kb.example.com"}`)},
				}},
			},
		}
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
	})

	// --- source tiers (PR1) through the API server ---

	It("defaults spec.source to image and admits a digest-pinned image-tier Capability", func() {
		name := uniqueName("cap-src-default")
		c := validCapability(name)
		// Source left unset: the CRD default mutates it to image.
		Expect(k8sClient.Create(ctx, c)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &agentv1alpha1.Capability{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}})
		})

		got := &agentv1alpha1.Capability{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, got)).To(Succeed())
		Expect(got.Spec.Source).To(Equal(agentv1alpha1.CapabilitySourceImage),
			"the CRD default must set source to image")
	})

	It("rejects a source: builtin Capability that sets an artifact (webhook)", func() {
		c := validCapability(uniqueName("cap-builtin-artifact"))
		c.Spec.Source = agentv1alpha1.CapabilitySourceBuiltin
		// validCapability sets a digest-pinned artifact.

		err := k8sClient.Create(ctx, c)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "want Invalid, got %v", err)
		Expect(err.Error()).To(ContainSubstring("source builtin must not set artifact"))
	})

	It("admits a source: builtin Capability that matches the embedded metadata (webhook)", func() {
		// A real built-in (flokoa-openapi) with no artifact, mirroring the
		// operator's embedded metadata, is admitted through the API server.
		info, ok := spec.BuiltinCapability(spec.DefaultRunnerVersion, "flokoa-openapi")
		Expect(ok).To(BeTrue(), "embedded baseline missing flokoa-openapi — run `make runner-contract`")
		c := &agentv1alpha1.Capability{
			ObjectMeta: metav1.ObjectMeta{Name: "flokoa-openapi", Namespace: ns},
			Spec: agentv1alpha1.CapabilitySpec{
				Source:            agentv1alpha1.CapabilitySourceBuiltin,
				Version:           spec.DefaultRunnerVersion,
				Entrypoint:        info.Entrypoint,
				SerializationName: info.SerializationName,
				ConfigSchema:      &apiextensionsv1.JSON{Raw: info.ConfigSchema},
				Requires: agentv1alpha1.CapabilityRequires{
					Python:       info.Requires.Python,
					PydanticAI:   info.Requires.PydanticAI,
					FlokoaRunner: info.Requires.FlokoaRunner,
				},
			},
		}
		Expect(k8sClient.Create(ctx, c)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &agentv1alpha1.Capability{ObjectMeta: metav1.ObjectMeta{Name: "flokoa-openapi", Namespace: ns}})
		})
	})

	It("rejects a source: builtin Capability that is not a real built-in (webhook)", func() {
		// The no-trust-me rule: a fake built-in name with no artifact is denied
		// because it doesn't match any embedded built-in metadata.
		name := uniqueName("cap-fake-builtin")
		c := validCapability(name)
		c.Spec.Source = agentv1alpha1.CapabilitySourceBuiltin
		c.Spec.Artifact = ""

		err := k8sClient.Create(ctx, c)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "want Invalid, got %v", err)
		Expect(err.Error()).To(ContainSubstring("not a built-in capability"))
	})

	It("rejects an image-tier Capability with no artifact (webhook)", func() {
		c := validCapability(uniqueName("cap-image-noartifact"))
		c.Spec.Source = agentv1alpha1.CapabilitySourceImage
		c.Spec.Artifact = "" // structurally allowed by the widened pattern; webhook denies

		err := k8sClient.Create(ctx, c)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "want Invalid, got %v", err)
		Expect(err.Error()).To(ContainSubstring("digest-pinned"))
	})

	It("rejects an unknown spec.source value (CRD enum)", func() {
		c := validCapability(uniqueName("cap-bad-source"))
		c.Spec.Source = agentv1alpha1.CapabilitySource("ftp")

		err := k8sClient.Create(ctx, c)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "want Invalid, got %v", err)
	})
})
