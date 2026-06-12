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
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Capability Controller", func() {
	Context("When reconciling a Capability resource", func() {
		var (
			ctx            context.Context
			capabilityName string
			nn             types.NamespacedName
		)

		newCapability := func(policy agentv1alpha1.SchemaPolicy) *agentv1alpha1.Capability {
			c := &agentv1alpha1.Capability{
				ObjectMeta: metav1.ObjectMeta{Name: capabilityName, Namespace: "default"},
				Spec: agentv1alpha1.CapabilitySpec{
					Artifact:     "ghcr.io/danielnyari/capabilities/kb@sha256:" + strings.Repeat("a", 64),
					Version:      "0.1.0",
					Entrypoint:   "flokoa_kb.capability:KB",
					SchemaPolicy: policy,
					Requires:     agentv1alpha1.CapabilityRequires{FlokoaRunner: ">=0.2"},
				},
			}
			if policy != agentv1alpha1.SchemaPolicyPermissive {
				c.Spec.ConfigSchema = &apiextensionsv1.JSON{Raw: []byte(`{"type":"object"}`)}
			}
			return c
		}

		reconcileOnce := func() {
			r := &CapabilityReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		}

		BeforeEach(func() {
			ctx = context.Background()
			capabilityName = fmt.Sprintf("test-capability-%d", time.Now().UnixNano())
			nn = types.NamespacedName{Name: capabilityName, Namespace: "default"}
		})

		AfterEach(func() {
			c := &agentv1alpha1.Capability{}
			if err := k8sClient.Get(ctx, nn, c); err == nil {
				_ = k8sClient.Delete(ctx, c)
			}
		})

		It("surfaces Permissive=False and Verified=Unknown for a strict capability", func() {
			Expect(k8sClient.Create(ctx, newCapability(agentv1alpha1.SchemaPolicyStrict))).To(Succeed())
			reconcileOnce()

			c := &agentv1alpha1.Capability{}
			Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
			Expect(c.Status.ObservedGeneration).To(Equal(c.Generation))

			permissive := meta.FindStatusCondition(c.Status.Conditions, agentv1alpha1.CapabilityConditionPermissive)
			Expect(permissive).NotTo(BeNil())
			Expect(permissive.Status).To(Equal(metav1.ConditionFalse))

			verified := meta.FindStatusCondition(c.Status.Conditions, agentv1alpha1.CapabilityConditionVerified)
			Expect(verified).NotTo(BeNil())
			Expect(verified.Status).To(Equal(metav1.ConditionUnknown))
		})

		It("loudly flags Permissive=True for a permissive capability", func() {
			Expect(k8sClient.Create(ctx, newCapability(agentv1alpha1.SchemaPolicyPermissive))).To(Succeed())
			reconcileOnce()

			c := &agentv1alpha1.Capability{}
			Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())

			permissive := meta.FindStatusCondition(c.Status.Conditions, agentv1alpha1.CapabilityConditionPermissive)
			Expect(permissive).NotTo(BeNil())
			Expect(permissive.Status).To(Equal(metav1.ConditionTrue))
			Expect(permissive.Message).To(ContainSubstring("not validated"))
		})
	})
})
