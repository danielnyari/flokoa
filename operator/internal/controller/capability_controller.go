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
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/infra/verify"
)

const (
	// verifyBackoffInitial..verifyBackoffMax bound the transient-error
	// requeue backoff (Verified=Unknown/VerifyError).
	verifyBackoffInitial = 15 * time.Second
	verifyBackoffMax     = 5 * time.Minute

	// verifyRecheckInterval re-checks definitive False verdicts: signatures
	// can be published after the artifact (sign-after-push), and the digest
	// pin means nothing else about the artifact can change.
	verifyRecheckInterval = 10 * time.Minute
)

// CapabilityReconciler surfaces a Capability's policy and verification state
// in status. It owns no workloads: capabilities act through the Agents that
// attach them. With a Verifier wired, the Verified condition reports cosign
// signature verification of the digest-pinned artifact (roadmap 09).
type CapabilityReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Verifier checks artifact signatures; nil means verification is
	// disabled on this cluster (Verified stays Unknown/VerificationDisabled).
	Verifier verify.ArtifactVerifier

	// verifyState caches per-object verification verdicts for this operator
	// process. Re-verification happens when the spec generation changes
	// (digests are immutable, so a new digest is a new generation), when the
	// operator restarts (verifier config can only change with a restart), on
	// the periodic False re-check, and on transient-error retries.
	verifyStateMu sync.Mutex
	verifyState   map[ktypes.NamespacedName]verifyRecord
}

// verifyRecord is one cached verification verdict.
type verifyRecord struct {
	uid        ktypes.UID
	generation int64
	condition  metav1.Condition
	verifiedAt time.Time
	backoff    time.Duration
}

// verifyOutcome is the computed Verified condition plus requeue advice.
type verifyOutcome struct {
	condition    metav1.Condition
	requeueAfter time.Duration
}

// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=capabilities,verbs=get;list;watch
// +kubebuilder:rbac:groups=agent.flokoa.ai,resources=capabilities/status,verbs=get;update;patch

func (r *CapabilityReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	capability := &agentv1alpha1.Capability{}
	if err := r.Get(ctx, req.NamespacedName, capability); err != nil {
		if apierrors.IsNotFound(err) {
			r.forgetVerifyState(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !capability.DeletionTimestamp.IsZero() {
		r.forgetVerifyState(req.NamespacedName)
		return ctrl.Result{}, nil
	}

	outcome := r.verifiedCondition(ctx, capability)

	if capabilityStatusUpToDate(capability, outcome.condition) {
		// Skipping the no-op write matters: every status write fans out
		// Agent reconciles through the capability watch.
		return ctrl.Result{RequeueAfter: outcome.requeueAfter}, nil
	}

	err := updateStatusWithRetry(ctx, r.Client, capability, func() {
		applyCapabilityStatus(capability)
		outcome.condition.ObservedGeneration = capability.Generation
		meta.SetStatusCondition(&capability.Status.Conditions, outcome.condition)
	})
	if err != nil {
		logger.Error(err, "Failed to update Capability status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: outcome.requeueAfter}, nil
}

// verifiedCondition runs the Verified state machine:
//
//	verification disabled        → Unknown / VerificationDisabled
//	signature verified           → True    / SignatureVerified (digest in message)
//	signature missing            → False   / SignatureMissing  (periodic re-check)
//	signature invalid            → False   / SignatureInvalid  (periodic re-check)
//	transient verification error → Unknown / VerifyError + backoff requeue
func (r *CapabilityReconciler) verifiedCondition(ctx context.Context, capability *agentv1alpha1.Capability) verifyOutcome {
	if r.Verifier == nil {
		return verifyOutcome{condition: metav1.Condition{
			Type:    agentv1alpha1.CapabilityConditionVerified,
			Status:  metav1.ConditionUnknown,
			Reason:  agentv1alpha1.CapabilityVerifiedReasonDisabled,
			Message: "cosign verification is not enabled on this cluster",
		}}
	}

	key := ktypes.NamespacedName{Namespace: capability.Namespace, Name: capability.Name}
	if cached, ok := r.cachedVerdict(key, capability); ok {
		return cached
	}

	signatureRef := ""
	if capability.Spec.Provenance != nil {
		signatureRef = capability.Spec.Provenance.SignatureRef
	}
	err := r.Verifier.Verify(ctx, capability.Spec.Artifact, signatureRef)

	outcome := classifyVerification(capability.Spec.Artifact, err)
	return r.rememberVerdict(key, capability, outcome, err)
}

// classifyVerification maps a verifier result onto the Verified condition.
func classifyVerification(artifact string, err error) verifyOutcome {
	switch {
	case err == nil:
		return verifyOutcome{condition: metav1.Condition{
			Type:    agentv1alpha1.CapabilityConditionVerified,
			Status:  metav1.ConditionTrue,
			Reason:  agentv1alpha1.CapabilityVerifiedReasonVerified,
			Message: fmt.Sprintf("cosign signature verified for %s", artifact),
		}}
	case verify.IsSignatureMissing(err):
		return verifyOutcome{
			condition: metav1.Condition{
				Type:    agentv1alpha1.CapabilityConditionVerified,
				Status:  metav1.ConditionFalse,
				Reason:  agentv1alpha1.CapabilityVerifiedReasonMissing,
				Message: err.Error(),
			},
			requeueAfter: verifyRecheckInterval,
		}
	case verify.IsSignatureInvalid(err):
		return verifyOutcome{
			condition: metav1.Condition{
				Type:    agentv1alpha1.CapabilityConditionVerified,
				Status:  metav1.ConditionFalse,
				Reason:  agentv1alpha1.CapabilityVerifiedReasonInvalid,
				Message: err.Error(),
			},
			requeueAfter: verifyRecheckInterval,
		}
	default:
		// Transient (and any unclassified) error: no verdict — never poison
		// the condition with False on a registry blip.
		return verifyOutcome{
			condition: metav1.Condition{
				Type:    agentv1alpha1.CapabilityConditionVerified,
				Status:  metav1.ConditionUnknown,
				Reason:  agentv1alpha1.CapabilityVerifiedReasonError,
				Message: fmt.Sprintf("verification has not completed and will be retried: %v", err),
			},
		}
	}
}

// cachedVerdict returns the remembered outcome when re-verification is not
// due: same object (UID), same generation, and either a True verdict
// (settled for this digest) or a False verdict younger than the re-check
// interval. Transient outcomes are never cached.
func (r *CapabilityReconciler) cachedVerdict(key ktypes.NamespacedName, capability *agentv1alpha1.Capability) (verifyOutcome, bool) {
	r.verifyStateMu.Lock()
	defer r.verifyStateMu.Unlock()
	record, ok := r.verifyState[key]
	if !ok || record.uid != capability.UID || record.generation != capability.Generation {
		return verifyOutcome{}, false
	}
	if record.condition.Status == metav1.ConditionTrue {
		return verifyOutcome{condition: record.condition}, true
	}
	if age := time.Since(record.verifiedAt); age < verifyRecheckInterval {
		return verifyOutcome{condition: record.condition, requeueAfter: verifyRecheckInterval - age}, true
	}
	return verifyOutcome{}, false
}

// rememberVerdict stores definitive outcomes, computes the transient backoff
// (doubling, capped) for retryable ones, and returns the outcome with its
// requeue advice settled.
func (r *CapabilityReconciler) rememberVerdict(key ktypes.NamespacedName, capability *agentv1alpha1.Capability, outcome verifyOutcome, err error) verifyOutcome {
	r.verifyStateMu.Lock()
	defer r.verifyStateMu.Unlock()
	if r.verifyState == nil {
		r.verifyState = map[ktypes.NamespacedName]verifyRecord{}
	}

	if err != nil && verify.IsTransient(err) {
		backoff := verifyBackoffInitial
		if prev, ok := r.verifyState[key]; ok && prev.backoff > 0 {
			backoff = min(prev.backoff*2, verifyBackoffMax)
		}
		// Keep only the backoff progression — a transient outcome is not a
		// verdict and must be retried on the next reconcile.
		r.verifyState[key] = verifyRecord{uid: capability.UID, generation: capability.Generation, backoff: backoff}
		outcome.requeueAfter = backoff
		return outcome
	}

	r.verifyState[key] = verifyRecord{
		uid:        capability.UID,
		generation: capability.Generation,
		condition:  outcome.condition,
		verifiedAt: time.Now(),
	}
	return outcome
}

// forgetVerifyState drops the cached verdict for a deleted Capability.
func (r *CapabilityReconciler) forgetVerifyState(key ktypes.NamespacedName) {
	r.verifyStateMu.Lock()
	defer r.verifyStateMu.Unlock()
	delete(r.verifyState, key)
}

// capabilityStatusUpToDate reports whether writing the computed status would
// change nothing: it applies the same mutation Reconcile would to a deep
// copy and compares observedGeneration plus every condition's type, status,
// reason, message, and observedGeneration (lastTransitionTime is derived and
// deliberately ignored).
func capabilityStatusUpToDate(capability *agentv1alpha1.Capability, verified metav1.Condition) bool {
	desired := capability.DeepCopy()
	applyCapabilityStatus(desired)
	verified.ObservedGeneration = desired.Generation
	meta.SetStatusCondition(&desired.Status.Conditions, verified)

	if capability.Status.ObservedGeneration != desired.Status.ObservedGeneration {
		return false
	}
	if len(capability.Status.Conditions) != len(desired.Status.Conditions) {
		return false
	}
	for _, want := range desired.Status.Conditions {
		got := meta.FindStatusCondition(capability.Status.Conditions, want.Type)
		if got == nil ||
			got.Status != want.Status ||
			got.Reason != want.Reason ||
			got.Message != want.Message ||
			got.ObservedGeneration != want.ObservedGeneration {
			return false
		}
	}
	return true
}

// applyCapabilityStatus computes the verification-independent status in
// place: observedGeneration and the Permissive condition.
func applyCapabilityStatus(capability *agentv1alpha1.Capability) {
	capability.Status.ObservedGeneration = capability.Generation

	// Permissive is the loud surfacing of schemaPolicy: permissive (product
	// brief §4): visible in status, printcolumns, and CLI output.
	permissive := metav1.Condition{
		Type:               agentv1alpha1.CapabilityConditionPermissive,
		Status:             metav1.ConditionFalse,
		Reason:             "SchemaValidated",
		Message:            "attached agent config is validated against spec.configSchema at admission",
		ObservedGeneration: capability.Generation,
	}
	if capability.Spec.SchemaPolicy == agentv1alpha1.SchemaPolicyPermissive {
		permissive.Status = metav1.ConditionTrue
		permissive.Reason = "SchemaPolicyPermissive"
		permissive.Message = "schemaPolicy is permissive: attached agent config is not validated at admission"
	}
	meta.SetStatusCondition(&capability.Status.Conditions, permissive)
}

func (r *CapabilityReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&agentv1alpha1.Capability{}).
		Named("capability").
		Complete(r)
}
