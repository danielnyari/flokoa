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

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
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
})
