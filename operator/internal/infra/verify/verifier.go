// Package verify checks capability artifact provenance: cosign signature
// verification for the digest-pinned OCI wheelhouse artifacts referenced by
// Capability CRs (roadmap 09). Digest pinning (admission) already guarantees
// integrity — content addressing means a registry can at worst deny service —
// so this package answers the *provenance* question: who published this
// digest. Verification runs at Capability reconcile, never in the pod path.
//
// The package isolates the sigstore dependency tree behind ArtifactVerifier:
// the controller, webhook, and compiler only ever see the interface and the
// error classification, and their tests run against fakes.
package verify

import (
	"context"
	"errors"
	"fmt"
)

// ArtifactVerifier checks the signature of a digest-pinned artifact.
type ArtifactVerifier interface {
	// Verify checks the signature for a digest-pinned artifact ref.
	// signatureRef optionally names a non-default signature location
	// (spec.provenance.signatureRef); empty means the cosign sidecar-tag
	// convention (…:sha256-<hex>.sig in the artifact repository).
	//
	// A nil return means the signature verified. Failures are classified:
	// SignatureMissingError and SignatureInvalidError are definitive verdicts
	// for this digest; TransientError (and any unclassified error) means no
	// verdict was reached and the caller should retry.
	Verify(ctx context.Context, artifactRef, signatureRef string) error
}

// SignatureMissingError is the definitive "no signature exists" verdict:
// the signature manifest is absent or carries no cosign signature layers.
type SignatureMissingError struct {
	// SignatureRef is the location that was checked.
	SignatureRef string
	Err          error
}

func (e *SignatureMissingError) Error() string {
	return fmt.Sprintf("no cosign signature found at %s: %v", e.SignatureRef, e.Err)
}

func (e *SignatureMissingError) Unwrap() error { return e.Err }

// SignatureInvalidError is the definitive "a signature exists but does not
// verify" verdict: bad cryptographic signature, a payload signed for a
// different digest, or a certificate identity outside the configured policy.
type SignatureInvalidError struct {
	// SignatureRef is the location of the rejected signature.
	SignatureRef string
	Err          error
}

func (e *SignatureInvalidError) Error() string {
	return fmt.Sprintf("cosign signature at %s did not verify: %v", e.SignatureRef, e.Err)
}

func (e *SignatureInvalidError) Unwrap() error { return e.Err }

// TransientError means no verdict was reached: the registry or the sigstore
// trust infrastructure was unavailable, or credentials could not be loaded.
// Callers must treat it as retryable — never as an unverified artifact —
// so a registry outage cannot flip Verified=False and (under the
// requireVerified policy) brick Agent admission.
type TransientError struct {
	Err error
}

func (e *TransientError) Error() string {
	return fmt.Sprintf("verification did not complete: %v", e.Err)
}

func (e *TransientError) Unwrap() error { return e.Err }

// IsSignatureMissing reports whether err carries a SignatureMissingError.
func IsSignatureMissing(err error) bool {
	var t *SignatureMissingError
	return errors.As(err, &t)
}

// IsSignatureInvalid reports whether err carries a SignatureInvalidError.
func IsSignatureInvalid(err error) bool {
	var t *SignatureInvalidError
	return errors.As(err, &t)
}

// IsTransient reports whether err should be treated as retryable: an explicit
// TransientError, or any error that carries no definitive classification.
// Defaulting unclassified errors to transient is deliberate — an unexpected
// failure mode must degrade to "retry" rather than to a false verdict.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}
	return !IsSignatureMissing(err) && !IsSignatureInvalid(err)
}
