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
	"errors"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/infra/verify"
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

		It("skips the status write when a second reconcile changes nothing", func() {
			Expect(k8sClient.Create(ctx, newCapability(agentv1alpha1.SchemaPolicyStrict))).To(Succeed())
			reconcileOnce()

			c := &agentv1alpha1.Capability{}
			Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
			settledVersion := c.ResourceVersion
			Expect(settledVersion).NotTo(BeEmpty())

			// Same generation, same verifier outcome: the reconcile must not
			// touch the object — a no-op status write would fan out Agent
			// reconciles through the capability watch.
			reconcileOnce()
			Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
			Expect(c.ResourceVersion).To(Equal(settledVersion),
				"a no-op reconcile must not write status")

			// A spec change (new generation) must write again: the stored
			// observedGeneration is stale.
			c.Spec.Version = "0.2.0"
			Expect(k8sClient.Update(ctx, c)).To(Succeed())
			reconcileOnce()
			Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
			Expect(c.Status.ObservedGeneration).To(Equal(c.Generation))
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

// stubVerifier is a programmable verify.ArtifactVerifier for envtests.
type stubVerifier struct {
	err          error
	calls        int
	artifactRef  string
	signatureRef string
}

func (s *stubVerifier) Verify(_ context.Context, artifactRef, signatureRef string) error {
	s.calls++
	s.artifactRef = artifactRef
	s.signatureRef = signatureRef
	return s.err
}

var _ = Describe("Capability controller Verified state machine", func() {
	var (
		ctx            context.Context
		capabilityName string
		nn             types.NamespacedName
	)

	newCapability := func() *agentv1alpha1.Capability {
		return &agentv1alpha1.Capability{
			ObjectMeta: metav1.ObjectMeta{Name: capabilityName, Namespace: "default"},
			Spec: agentv1alpha1.CapabilitySpec{
				Artifact:     "ghcr.io/danielnyari/capabilities/echo@sha256:" + strings.Repeat("a", 64),
				Version:      "0.1.0",
				Entrypoint:   "flokoa_echo.capability:Echo",
				SchemaPolicy: agentv1alpha1.SchemaPolicyPermissive,
				Requires:     agentv1alpha1.CapabilityRequires{FlokoaRunner: ">=0.2"},
			},
		}
	}

	reconcilerWith := func(v *stubVerifier) *CapabilityReconciler {
		r := &CapabilityReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		if v != nil {
			r.Verifier = v
		}
		return r
	}

	reconcile := func(r *CapabilityReconciler) reconcile.Result {
		result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	verifiedCondition := func() *metav1.Condition {
		c := &agentv1alpha1.Capability{}
		Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
		cond := meta.FindStatusCondition(c.Status.Conditions, agentv1alpha1.CapabilityConditionVerified)
		Expect(cond).NotTo(BeNil())
		Expect(cond.ObservedGeneration).To(Equal(c.Generation))
		return cond
	}

	BeforeEach(func() {
		ctx = context.Background()
		capabilityName = fmt.Sprintf("test-verify-%d", time.Now().UnixNano())
		nn = types.NamespacedName{Name: capabilityName, Namespace: "default"}
		Expect(k8sClient.Create(ctx, newCapability())).To(Succeed())
	})

	AfterEach(func() {
		c := &agentv1alpha1.Capability{}
		if err := k8sClient.Get(ctx, nn, c); err == nil {
			_ = k8sClient.Delete(ctx, c)
		}
	})

	It("reports Unknown/VerificationDisabled without a verifier", func() {
		result := reconcile(reconcilerWith(nil))
		Expect(result.RequeueAfter).To(BeZero())

		cond := verifiedCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
		Expect(cond.Reason).To(Equal(agentv1alpha1.CapabilityVerifiedReasonDisabled))
		Expect(cond.Message).To(ContainSubstring("not enabled"))
	})

	It("reports True/SignatureVerified with the digest in the message and settles", func() {
		stub := &stubVerifier{}
		r := reconcilerWith(stub)
		result := reconcile(r)
		Expect(result.RequeueAfter).To(BeZero())

		cond := verifiedCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(agentv1alpha1.CapabilityVerifiedReasonVerified))
		Expect(cond.Message).To(ContainSubstring("@sha256:" + strings.Repeat("a", 64)))
		Expect(stub.artifactRef).To(ContainSubstring("@sha256:"))
		Expect(stub.signatureRef).To(BeEmpty())

		// Digests are immutable: a second reconcile of the same generation
		// must not re-verify.
		reconcile(r)
		Expect(stub.calls).To(Equal(1))
	})

	It("reports False/SignatureMissing with a periodic re-check", func() {
		stub := &stubVerifier{err: &verify.SignatureMissingError{SignatureRef: "ghcr.io/sig", Err: errors.New("manifest unknown")}}
		result := reconcile(reconcilerWith(stub))
		Expect(result.RequeueAfter).To(Equal(verifyRecheckInterval))

		cond := verifiedCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(agentv1alpha1.CapabilityVerifiedReasonMissing))
	})

	It("reports False/SignatureInvalid and re-verifies after a generation bump", func() {
		stub := &stubVerifier{err: &verify.SignatureInvalidError{SignatureRef: "ghcr.io/sig", Err: errors.New("bad signature")}}
		r := reconcilerWith(stub)
		reconcile(r)

		cond := verifiedCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(agentv1alpha1.CapabilityVerifiedReasonInvalid))
		Expect(cond.Message).To(ContainSubstring("bad signature"))

		// Within the re-check interval the verdict is cached.
		reconcile(r)
		Expect(stub.calls).To(Equal(1))

		// A new digest is a new generation: the verdict resets immediately.
		c := &agentv1alpha1.Capability{}
		Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
		c.Spec.Artifact = "ghcr.io/danielnyari/capabilities/echo@sha256:" + strings.Repeat("b", 64)
		Expect(k8sClient.Update(ctx, c)).To(Succeed())
		stub.err = nil
		reconcile(r)
		Expect(stub.calls).To(Equal(2))
		Expect(verifiedCondition().Status).To(Equal(metav1.ConditionTrue))
	})

	It("keeps Unknown/VerifyError with backoff on transient errors", func() {
		stub := &stubVerifier{err: &verify.TransientError{Err: errors.New("registry unavailable")}}
		r := reconcilerWith(stub)
		result := reconcile(r)
		Expect(result.RequeueAfter).To(Equal(verifyBackoffInitial))

		cond := verifiedCondition()
		Expect(cond.Status).To(Equal(metav1.ConditionUnknown))
		Expect(cond.Reason).To(Equal(agentv1alpha1.CapabilityVerifiedReasonError))
		Expect(cond.Message).To(ContainSubstring("will be retried"))

		// Transient outcomes are never cached; the backoff doubles.
		result = reconcile(r)
		Expect(stub.calls).To(Equal(2))
		Expect(result.RequeueAfter).To(Equal(2 * verifyBackoffInitial))

		// Recovery: the registry comes back and the condition settles True.
		stub.err = nil
		result = reconcile(r)
		Expect(result.RequeueAfter).To(BeZero())
		Expect(verifiedCondition().Status).To(Equal(metav1.ConditionTrue))
	})

	It("passes spec.provenance.signatureRef through to the verifier", func() {
		c := &agentv1alpha1.Capability{}
		Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
		c.Spec.Provenance = &agentv1alpha1.CapabilityProvenance{SignatureRef: "ghcr.io/danielnyari/signatures/echo:custom"}
		Expect(k8sClient.Update(ctx, c)).To(Succeed())

		stub := &stubVerifier{}
		reconcile(reconcilerWith(stub))
		Expect(stub.signatureRef).To(Equal("ghcr.io/danielnyari/signatures/echo:custom"))
	})

	It("drops the cached verdict when the Capability is deleted", func() {
		// Establish a True verdict in the cache.
		stub := &stubVerifier{}
		r := reconcilerWith(stub)
		reconcile(r)
		Expect(stub.calls).To(Equal(1))

		// A second reconcile of the same generation must use the cache.
		reconcile(r)
		Expect(stub.calls).To(Equal(1))

		// Delete the Capability, then trigger the not-found reconcile path
		// so forgetVerifyState clears the cache entry.
		c := &agentv1alpha1.Capability{}
		Expect(k8sClient.Get(ctx, nn, c)).To(Succeed())
		Expect(k8sClient.Delete(ctx, c)).To(Succeed())

		notFoundResult, notFoundErr := r.Reconcile(ctx, ctrl.Request{NamespacedName: nn})
		Expect(notFoundErr).NotTo(HaveOccurred())
		Expect(notFoundResult.RequeueAfter).To(BeZero())

		// Recreate a Capability under the same name — its UID will differ.
		newCap := newCapability()
		Expect(k8sClient.Create(ctx, newCap)).To(Succeed())
		// The new object has a fresh UID: the old cache entry must not
		// satisfy the cache check, so the verifier must be called again.
		reconcile(r)
		Expect(stub.calls).To(Equal(2))
	})

	It("re-verifies a False verdict once the re-check interval elapses", func() {
		stub := &stubVerifier{err: &verify.SignatureMissingError{SignatureRef: "ghcr.io/sig", Err: errors.New("manifest unknown")}}
		r := reconcilerWith(stub)
		reconcile(r)
		Expect(stub.calls).To(Equal(1))

		// Within the interval: still cached, verifier not called again.
		reconcile(r)
		Expect(stub.calls).To(Equal(1))

		// Back-date the cached record so it looks older than verifyRecheckInterval.
		key := types.NamespacedName{Namespace: nn.Namespace, Name: nn.Name}
		r.verifyStateMu.Lock()
		if record, ok := r.verifyState[key]; ok {
			record.verifiedAt = record.verifiedAt.Add(-(verifyRecheckInterval + time.Second))
			r.verifyState[key] = record
		}
		r.verifyStateMu.Unlock()

		// Now the verdict is stale — the verifier must be called again.
		reconcile(r)
		Expect(stub.calls).To(Equal(2))
	})
})
