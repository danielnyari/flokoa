package verify

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	ociv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	sigstoreverify "github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"github.com/sigstore/sigstore/pkg/signature"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// simpleSigningMediaType is the media type of cosign signature layers.
	simpleSigningMediaType = "application/vnd.dev.cosign.simplesigning.v1+json"

	// signatureAnnotation holds the base64 signature over the simple signing
	// payload (the layer blob).
	signatureAnnotation = "dev.cosignproject.cosign/signature"

	// certificateAnnotation holds the PEM Fulcio signing certificate
	// (keyless signatures only).
	certificateAnnotation = "dev.sigstore.cosign/certificate"

	// maxPayloadBytes caps the simple signing payload download. Real payloads
	// are a few hundred bytes; the cap is pure defensiveness against a
	// hostile registry.
	maxPayloadBytes = 1 << 20
)

// wildcardRegexps are identity regexps that match everything: configuring one
// is signature theater, rejected at construction (identity-free keyless
// verification proves only that *someone* signed the digest).
var wildcardRegexps = map[string]bool{
	".*": true, ".+": true, "^.*$": true, "^.+$": true, "^": true, "$": true,
}

// Options configure a CosignVerifier. Exactly one mode is selected: KeyPath
// (key-based) or the Keyless pair.
type Options struct {
	// KeyPath is the path of a PEM public key file mounted into the operator
	// pod (key-based mode).
	KeyPath string

	// KeylessIssuer is the exact OIDC issuer required on the Fulcio signing
	// certificate (keyless mode), e.g.
	// https://token.actions.githubusercontent.com.
	KeylessIssuer string

	// KeylessIdentityRegexp is the regexp the certificate identity (SAN) must
	// match (keyless mode). Required: wildcard-only patterns are rejected.
	KeylessIdentityRegexp string

	// RegistrySecretName optionally names a kubernetes.io/dockerconfigjson
	// Secret in Namespace used for registry access on top of the ambient
	// keychain (private artifact registries).
	RegistrySecretName string

	// Namespace is the operator namespace holding RegistrySecretName.
	Namespace string

	// SecretReader reads RegistrySecretName; nil disables the Secret keychain.
	SecretReader client.Reader
}

// CosignVerifier verifies classic cosign OCI signatures (the sidecar-tag
// convention) for digest-pinned artifacts.
//
// Key-based mode is implemented directly over go-containerregistry: fetch the
// signature manifest, verify the signature annotation over each simple
// signing payload with the configured public key, and check the payload's
// docker-manifest-digest claim. The classic key-based cosign format is a
// stable, documented spec, and the direct implementation avoids depending on
// cosign's (very large) library surface.
//
// Keyless mode reconstructs a Sigstore bundle from the signature image's
// annotations (certificate + Rekor bundle) and verifies it with
// sigstore-go/pkg/verify against the public-good trusted root (TUF, fetched
// lazily and cached in-process; DisableLocalCache keeps the read-only rootfs
// happy), enforcing the configured certificate identity policy. The bundle
// reconstruction follows sigstore-go's oci-image-verification example.
type CosignVerifier struct {
	opts Options

	// keyVerifier is set in key-based mode.
	keyVerifier signature.Verifier

	// certIdentity is set in keyless mode.
	certIdentity *sigstoreverify.CertificateIdentity

	// trustedRoot caches the lazily fetched sigstore trusted root (keyless),
	// refreshed when older than trustedRootTTL.
	trustedRootMu        sync.Mutex
	trustedRoot          root.TrustedMaterial
	trustedRootFetchedAt time.Time

	// fetchRoot fetches the trusted root from TUF; tests inject a fake.
	// nil means the public-good default.
	fetchRoot func() (root.TrustedMaterial, error)
}

// trustedRootTTL bounds how long a fetched TUF trusted root is served from
// the in-process cache before a re-fetch. Sigstore key material rotates
// rarely, but a process that never re-fetches would miss a rotation for its
// whole lifetime.
var trustedRootTTL = 24 * time.Hour

var _ ArtifactVerifier = &CosignVerifier{}

// NewCosignVerifier validates the configuration and builds the verifier.
// Invalid configuration (no mode, both modes, identity-free or wildcard-only
// keyless policy, unreadable key) fails here so the operator refuses to start
// rather than producing meaningless Verified conditions.
func NewCosignVerifier(opts Options) (*CosignVerifier, error) {
	v := &CosignVerifier{opts: opts}

	keyMode := opts.KeyPath != ""
	keylessConfigured := opts.KeylessIssuer != "" || opts.KeylessIdentityRegexp != ""

	switch {
	case keyMode && keylessConfigured:
		return nil, fmt.Errorf("cosign verification cannot use both a public key (%s) and keyless settings; configure exactly one mode", opts.KeyPath)
	case keyMode:
		pem, err := os.ReadFile(opts.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("reading the cosign public key: %w", err)
		}
		pub, err := cryptoutils.UnmarshalPEMToPublicKey(pem)
		if err != nil {
			return nil, fmt.Errorf("parsing the cosign public key at %s: %w", opts.KeyPath, err)
		}
		if err := checkPublicKeyStrength(pub); err != nil {
			return nil, fmt.Errorf("the cosign public key at %s is not usable: %w", opts.KeyPath, err)
		}
		verifier, err := signature.LoadVerifier(pub, crypto.SHA256)
		if err != nil {
			return nil, fmt.Errorf("loading a verifier for the cosign public key at %s: %w", opts.KeyPath, err)
		}
		v.keyVerifier = verifier
	default:
		// Keyless: an identity policy is required. Identity-free or
		// wildcard-only keyless config is signature theater — it proves only
		// that someone, anyone, signed the digest.
		if opts.KeylessIssuer == "" || opts.KeylessIdentityRegexp == "" {
			return nil, fmt.Errorf("cosign keyless verification requires both an OIDC issuer and a certificate identity regexp; got issuer=%q identityRegexp=%q", opts.KeylessIssuer, opts.KeylessIdentityRegexp)
		}
		if wildcardRegexps[strings.TrimSpace(opts.KeylessIdentityRegexp)] {
			return nil, fmt.Errorf("cosign keyless identity regexp %q matches every identity; configure a regexp that names the expected signer", opts.KeylessIdentityRegexp)
		}
		if _, err := regexp.Compile(opts.KeylessIdentityRegexp); err != nil {
			return nil, fmt.Errorf("compiling the cosign keyless identity regexp: %w", err)
		}
		certID, err := sigstoreverify.NewShortCertificateIdentity(
			opts.KeylessIssuer, "", "", opts.KeylessIdentityRegexp)
		if err != nil {
			return nil, fmt.Errorf("building the cosign keyless identity policy: %w", err)
		}
		v.certIdentity = &certID
	}

	return v, nil
}

// minRSAKeyBits is the smallest RSA modulus accepted for signature
// verification; anything below is rejected at construction.
const minRSAKeyBits = 2048

// checkPublicKeyStrength restricts the key-based mode to the key types
// cosign actually signs with — ECDSA, Ed25519, and RSA of at least 2048
// bits. Anything else (weak RSA, DSA, X25519, …) is misconfiguration and
// fails construction with the offending type named, rather than producing
// Verified=True conditions backed by weak or meaningless cryptography.
func checkPublicKeyStrength(pub crypto.PublicKey) error {
	switch key := pub.(type) {
	case *ecdsa.PublicKey, ed25519.PublicKey:
		return nil
	case *rsa.PublicKey:
		if bits := key.N.BitLen(); bits < minRSAKeyBits {
			return fmt.Errorf("RSA key is %d bits; at least %d bits are required", bits, minRSAKeyBits)
		}
		return nil
	default:
		return fmt.Errorf("unsupported public key type %T; supported: ECDSA, Ed25519, RSA (>= %d bits)", pub, minRSAKeyBits)
	}
}

// Verify implements ArtifactVerifier.
func (v *CosignVerifier) Verify(ctx context.Context, artifactRef, signatureRef string) error {
	artifact, err := name.NewDigest(artifactRef)
	if err != nil {
		// Admission enforces digest pinning; an unparseable ref here is a
		// malformed CR, not a registry blip — a definitive verdict.
		return &SignatureInvalidError{SignatureRef: signatureRef,
			Err: fmt.Errorf("artifact ref is not digest-pinned: %w", err)}
	}

	sigRef, err := v.signatureLocation(artifact, signatureRef)
	if err != nil {
		return &SignatureInvalidError{SignatureRef: signatureRef,
			Err: fmt.Errorf("invalid signatureRef: %w", err)}
	}

	keychain, err := v.keychain(ctx)
	if err != nil {
		return &TransientError{Err: fmt.Errorf("loading registry credentials: %w", err)}
	}
	remoteOpts := []remote.Option{
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(keychain),
	}

	sigImage, err := remote.Image(sigRef, remoteOpts...)
	if err != nil {
		return classifyFetchError(sigRef.String(), err)
	}
	manifest, err := sigImage.Manifest()
	if err != nil {
		return classifyFetchError(sigRef.String(), err)
	}

	layers := signatureLayers(manifest)
	if len(layers) == 0 {
		return &SignatureMissingError{SignatureRef: sigRef.String(),
			Err: fmt.Errorf("the signature manifest carries no cosign signature layers")}
	}

	// The sigstore verifier (keyless mode) is layer-independent: construct
	// it once per Verify call, not once per signature layer.
	var keylessVerifier *sigstoreverify.Verifier
	if v.keyVerifier == nil {
		keylessVerifier, err = v.newKeylessVerifier(ctx)
		if err != nil {
			return err
		}
	}

	// Any verifying signature wins (cosign semantics: a signature image can
	// carry several signatures; one valid one suffices).
	failures := make([]string, 0, len(layers))
	for _, layer := range layers {
		payload, err := layerPayload(ctx, sigImage, layer)
		if err != nil {
			return classifyFetchError(sigRef.String(), err)
		}

		var verifyErr error
		if v.keyVerifier != nil {
			verifyErr = v.verifyWithKey(layer, payload)
		} else {
			verifyErr = v.verifyKeyless(keylessVerifier, layer, payload)
		}
		if verifyErr == nil {
			// The cryptographic signature holds; bind it to *this* artifact:
			// the simple signing payload must claim the pinned digest.
			if claimErr := checkDigestClaim(payload, artifact.DigestStr()); claimErr != nil {
				failures = append(failures, claimErr.Error())
				continue
			}
			return nil
		}
		// Only explicitly transient failures (trust root unavailable, …)
		// abort the verdict; everything else is this layer's definitive
		// rejection and the next signature may still verify.
		var transient *TransientError
		if errors.As(verifyErr, &transient) {
			return verifyErr
		}
		failures = append(failures, verifyErr.Error())
	}

	return &SignatureInvalidError{SignatureRef: sigRef.String(),
		Err: fmt.Errorf("%d signature(s) present, none verified: %s", len(layers), strings.Join(failures, "; "))}
}

// verifyWithKey checks one signature layer against the configured public key.
func (v *CosignVerifier) verifyWithKey(layer ociv1.Descriptor, payload []byte) error {
	sigB64 := layer.Annotations[signatureAnnotation]
	if sigB64 == "" {
		return fmt.Errorf("signature layer %s has no %s annotation", layer.Digest, signatureAnnotation)
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("signature layer %s annotation is not base64: %w", layer.Digest, err)
	}
	if err := v.keyVerifier.VerifySignature(bytes.NewReader(sig), bytes.NewReader(payload)); err != nil {
		return fmt.Errorf("signature layer %s does not verify with the configured public key: %w", layer.Digest, err)
	}
	return nil
}

// newKeylessVerifier builds the layer-independent sigstore verifier for one
// Verify call: trusted root (TTL-cached) + verification options. Failures
// are transient — they are trust-infrastructure availability, never a
// verdict about the artifact.
func (v *CosignVerifier) newKeylessVerifier(ctx context.Context) (*sigstoreverify.Verifier, error) {
	trustedRoot, err := v.fetchTrustedRoot(ctx)
	if err != nil {
		return nil, &TransientError{Err: fmt.Errorf("fetching the sigstore trusted root: %w", err)}
	}
	verifier, err := sigstoreverify.NewVerifier(trustedRoot,
		sigstoreverify.WithSignedCertificateTimestamps(1),
		sigstoreverify.WithTransparencyLog(1),
		sigstoreverify.WithObserverTimestamps(1),
	)
	if err != nil {
		return nil, &TransientError{Err: fmt.Errorf("building the sigstore verifier: %w", err)}
	}
	return verifier, nil
}

// verifyKeyless checks one signature layer's reconstructed Sigstore bundle:
// Fulcio certificate chain + SCT, Rekor inclusion, signature over the
// payload, and the configured certificate identity.
func (v *CosignVerifier) verifyKeyless(verifier *sigstoreverify.Verifier, layer ociv1.Descriptor, payload []byte) error {
	if layer.Annotations[certificateAnnotation] == "" {
		return fmt.Errorf("signature layer %s carries no Fulcio certificate; expected a keyless cosign signature", layer.Digest)
	}

	b, err := bundleFromSignatureLayer(layer)
	if err != nil {
		return fmt.Errorf("signature layer %s: %w", layer.Digest, err)
	}

	policy := sigstoreverify.NewPolicy(
		sigstoreverify.WithArtifact(bytes.NewReader(payload)),
		sigstoreverify.WithCertificateIdentity(*v.certIdentity),
	)
	if _, err := verifier.Verify(b, policy); err != nil {
		return fmt.Errorf("signature layer %s failed keyless verification: %w", layer.Digest, err)
	}
	return nil
}

// fetchTrustedRoot lazily fetches and caches the sigstore public-good trusted
// root, re-fetching once the cached copy is older than trustedRootTTL. Lazy
// on purpose: a TUF outage then degrades to Verified=Unknown with a retry
// instead of crash-looping the operator at startup. A *re-fetch* failure is
// softer still: the stale root keeps serving (logged), because sigstore key
// material rotates far slower than TUF outages happen. The in-memory cache
// (DisableLocalCache) keeps the operator's read-only root filesystem happy.
func (v *CosignVerifier) fetchTrustedRoot(ctx context.Context) (root.TrustedMaterial, error) {
	v.trustedRootMu.Lock()
	defer v.trustedRootMu.Unlock()
	if v.trustedRoot != nil && time.Since(v.trustedRootFetchedAt) < trustedRootTTL {
		return v.trustedRoot, nil
	}

	fetch := v.fetchRoot
	if fetch == nil {
		fetch = fetchPublicGoodTrustedRoot
	}
	trustedRoot, err := fetch()
	if err != nil {
		if v.trustedRoot != nil {
			log.FromContext(ctx).Error(err,
				"re-fetching the sigstore trusted root failed; keeping the stale cached root",
				"fetchedAt", v.trustedRootFetchedAt)
			return v.trustedRoot, nil
		}
		return nil, err
	}
	v.trustedRoot = trustedRoot
	v.trustedRootFetchedAt = time.Now()
	return trustedRoot, nil
}

// fetchPublicGoodTrustedRoot fetches the sigstore public-good trusted root
// via TUF without touching the local filesystem.
func fetchPublicGoodTrustedRoot() (root.TrustedMaterial, error) {
	opts := tuf.DefaultOptions()
	opts.DisableLocalCache = true
	tufClient, err := tuf.New(opts)
	if err != nil {
		return nil, err
	}
	return root.GetTrustedRoot(tufClient)
}

// signatureLocation resolves where the signature image lives: the explicit
// signatureRef when set, else the cosign sidecar tag sha256-<hex>.sig in the
// artifact repository.
func (v *CosignVerifier) signatureLocation(artifact name.Digest, signatureRef string) (name.Reference, error) {
	if signatureRef != "" {
		return name.ParseReference(signatureRef)
	}
	sigTag := strings.Replace(artifact.DigestStr(), "sha256:", "sha256-", 1) + ".sig"
	return artifact.Context().Tag(sigTag), nil
}

// keychain assembles registry credentials: the optional dockerconfigjson
// Secret (private artifact registries) layered over the ambient keychain.
func (v *CosignVerifier) keychain(ctx context.Context) (authn.Keychain, error) {
	if v.opts.RegistrySecretName == "" || v.opts.SecretReader == nil {
		return authn.DefaultKeychain, nil
	}
	var secret corev1.Secret
	key := client.ObjectKey{Namespace: v.opts.Namespace, Name: v.opts.RegistrySecretName}
	if err := v.opts.SecretReader.Get(ctx, key, &secret); err != nil {
		return nil, fmt.Errorf("reading registry Secret %s: %w", key, err)
	}
	secretKeychain, err := keychainFromDockerConfigSecret(&secret)
	if err != nil {
		return nil, fmt.Errorf("parsing registry Secret %s: %w", key, err)
	}
	return authn.NewMultiKeychain(secretKeychain, authn.DefaultKeychain), nil
}

// dockerConfigKeychain resolves registry credentials from one parsed
// kubernetes.io/dockerconfigjson document. The matching is deliberately
// simple — exact registry host, with scheme prefixes stripped from the
// config keys — which covers the artifact-registry use case
// (ghcr.io / private registries) without pulling in kubelet's full
// credential-matching machinery.
type dockerConfigKeychain struct {
	auths map[string]authn.AuthConfig
}

var _ authn.Keychain = &dockerConfigKeychain{}

func keychainFromDockerConfigSecret(secret *corev1.Secret) (authn.Keychain, error) {
	raw, ok := secret.Data[corev1.DockerConfigJsonKey]
	if !ok {
		return nil, fmt.Errorf("the Secret has no %s key (expected type %s)",
			corev1.DockerConfigJsonKey, corev1.SecretTypeDockerConfigJson)
	}
	var doc struct {
		Auths map[string]struct {
			Username      string `json:"username"`
			Password      string `json:"password"`
			Auth          string `json:"auth"`
			IdentityToken string `json:"identitytoken"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid dockerconfigjson: %w", err)
	}

	kc := &dockerConfigKeychain{auths: map[string]authn.AuthConfig{}}
	for registry, entry := range doc.Auths {
		username, password := entry.Username, entry.Password
		if entry.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(entry.Auth)
			if err != nil {
				return nil, fmt.Errorf("invalid auth entry for %s: %w", registry, err)
			}
			if user, pass, found := strings.Cut(string(decoded), ":"); found {
				username, password = user, pass
			}
		}
		host := strings.TrimPrefix(strings.TrimPrefix(registry, "https://"), "http://")
		host, _, _ = strings.Cut(host, "/")
		kc.auths[host] = authn.AuthConfig{
			Username:      username,
			Password:      password,
			IdentityToken: entry.IdentityToken,
		}
	}
	return kc, nil
}

// Resolve implements authn.Keychain.
func (k *dockerConfigKeychain) Resolve(target authn.Resource) (authn.Authenticator, error) {
	if cfg, ok := k.auths[target.RegistryStr()]; ok {
		return authn.FromConfig(cfg), nil
	}
	return authn.Anonymous, nil
}

// signatureLayers returns the manifest's simple signing layers.
func signatureLayers(manifest *ociv1.Manifest) []ociv1.Descriptor {
	var layers []ociv1.Descriptor
	for _, layer := range manifest.Layers {
		if string(layer.MediaType) == simpleSigningMediaType {
			layers = append(layers, layer)
		}
	}
	return layers
}

// layerPayload downloads one simple signing payload (the signed message).
func layerPayload(_ context.Context, img ociv1.Image, desc ociv1.Descriptor) ([]byte, error) {
	layer, err := img.LayerByDigest(desc.Digest)
	if err != nil {
		return nil, err
	}
	// Simple signing payloads are stored uncompressed; Compressed returns the
	// blob exactly as uploaded — the bytes the signature covers.
	rc, err := layer.Compressed()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	payload, err := io.ReadAll(io.LimitReader(rc, maxPayloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxPayloadBytes {
		return nil, fmt.Errorf("simple signing payload %s exceeds %d bytes", desc.Digest, maxPayloadBytes)
	}
	return payload, nil
}

// simpleSigningPayload is the subset of the cosign simple signing claim
// document needed for the digest binding check.
type simpleSigningPayload struct {
	Critical struct {
		Image struct {
			DockerManifestDigest string `json:"docker-manifest-digest"`
		} `json:"image"`
	} `json:"critical"`
}

// checkDigestClaim binds a verified signature to the pinned artifact: the
// simple signing payload must claim exactly the artifact digest. Without this
// check a valid signature for any other image in the repository would pass.
func checkDigestClaim(payload []byte, artifactDigest string) error {
	var doc simpleSigningPayload
	if err := json.Unmarshal(payload, &doc); err != nil {
		return fmt.Errorf("simple signing payload is not valid JSON: %w", err)
	}
	claimed := doc.Critical.Image.DockerManifestDigest
	if claimed == "" {
		return fmt.Errorf("simple signing payload claims no docker-manifest-digest")
	}
	// Case-insensitive: digest algorithm prefixes ("sha256:") are
	// case-insensitive identifiers, and hex carries no case information.
	if !strings.EqualFold(claimed, artifactDigest) {
		return fmt.Errorf("signature is for digest %s, not the pinned artifact digest %s", claimed, artifactDigest)
	}
	return nil
}

// classifyFetchError maps registry errors: a missing signature manifest is a
// definitive SignatureMissing verdict; everything else (network failures,
// 5xx, auth problems) is transient — misconfigured credentials must surface
// as a retryable Unknown, never as an unverified artifact.
func classifyFetchError(sigRef string, err error) error {
	var terr *transport.Error
	if errors.As(err, &terr) && terr.StatusCode == http.StatusNotFound {
		return &SignatureMissingError{SignatureRef: sigRef, Err: err}
	}
	return &TransientError{Err: fmt.Errorf("fetching the signature at %s: %w", sigRef, err)}
}
