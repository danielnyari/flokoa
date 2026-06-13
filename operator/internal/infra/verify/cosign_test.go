package verify

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/sigstore/sigstore-go/pkg/root"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// --- construction / config validation ---

func writeTestPublicKey(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cosign.pub")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, priv
}

func TestNewCosignVerifierConfigValidation(t *testing.T) {
	keyPath, _ := writeTestPublicKey(t)

	cases := []struct {
		name    string
		opts    Options
		wantErr string
	}{
		{
			name:    "no mode configured",
			opts:    Options{},
			wantErr: "requires both an OIDC issuer and a certificate identity regexp",
		},
		{
			name:    "keyless without identity regexp",
			opts:    Options{KeylessIssuer: "https://token.actions.githubusercontent.com"},
			wantErr: "requires both an OIDC issuer and a certificate identity regexp",
		},
		{
			name:    "keyless without issuer",
			opts:    Options{KeylessIdentityRegexp: "^https://github.com/danielnyari/"},
			wantErr: "requires both an OIDC issuer and a certificate identity regexp",
		},
		{
			name: "wildcard-only identity regexp",
			opts: Options{
				KeylessIssuer:         "https://token.actions.githubusercontent.com",
				KeylessIdentityRegexp: ".*",
			},
			wantErr: "matches every identity",
		},
		{
			name: "anchored wildcard identity regexp",
			opts: Options{
				KeylessIssuer:         "https://token.actions.githubusercontent.com",
				KeylessIdentityRegexp: "^.*$",
			},
			wantErr: "matches every identity",
		},
		{
			name: "invalid identity regexp",
			opts: Options{
				KeylessIssuer:         "https://token.actions.githubusercontent.com",
				KeylessIdentityRegexp: "([unclosed",
			},
			wantErr: "compiling the cosign keyless identity regexp",
		},
		{
			name: "both modes configured",
			opts: Options{
				KeyPath:               keyPath,
				KeylessIssuer:         "https://token.actions.githubusercontent.com",
				KeylessIdentityRegexp: "^https://github.com/danielnyari/",
			},
			wantErr: "cannot use both a public key",
		},
		{
			name:    "missing key file",
			opts:    Options{KeyPath: filepath.Join(t.TempDir(), "absent.pub")},
			wantErr: "reading the cosign public key",
		},
		{
			name: "valid keyless",
			opts: Options{
				KeylessIssuer:         "https://token.actions.githubusercontent.com",
				KeylessIdentityRegexp: "^https://github.com/danielnyari/flokoa/",
			},
		},
		{
			name: "valid key-based",
			opts: Options{KeyPath: keyPath},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := NewCosignVerifier(tc.opts)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid config, got %v", err)
				}
				if v == nil {
					t.Fatal("expected a verifier")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v should contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestNewCosignVerifierRejectsGarbageKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cosign.pub")
	if err := os.WriteFile(path, []byte("not a pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewCosignVerifier(Options{KeyPath: path})
	if err == nil || !strings.Contains(err.Error(), "parsing the cosign public key") {
		t.Fatalf("garbage key must be rejected at construction, got %v", err)
	}
}

// --- key-based verification against an in-memory registry ---

// testRegistry pushes a tiny artifact image into an in-memory OCI registry
// and returns its digest-pinned reference.
func testRegistry(t *testing.T) (*httptest.Server, name.Digest) {
	t.Helper()
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)

	host := strings.TrimPrefix(srv.URL, "http://")
	img, err := mutate.AppendLayers(empty.Image,
		static.NewLayer([]byte("wheelhouse"), types.MediaType("application/vnd.oci.image.layer.v1.tar")))
	if err != nil {
		t.Fatal(err)
	}
	tag, err := name.NewTag(host + "/capabilities/echo:0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(tag, img); err != nil {
		t.Fatal(err)
	}
	digest, err := img.Digest()
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := name.NewDigest(fmt.Sprintf("%s/capabilities/echo@%s", host, digest))
	if err != nil {
		t.Fatal(err)
	}
	return srv, artifact
}

// signPayload produces the cosign simple signing payload for the artifact
// digest plus its base64 ECDSA signature.
func signPayload(t *testing.T, priv *ecdsa.PrivateKey, manifestDigest string) ([]byte, string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"critical": map[string]any{
			"identity": map[string]any{"docker-reference": "test"},
			"image":    map[string]any{"docker-manifest-digest": manifestDigest},
			"type":     "cosign container image signature",
		},
		"optional": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(payload)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, h[:])
	if err != nil {
		t.Fatal(err)
	}
	return payload, base64.StdEncoding.EncodeToString(sig)
}

// pushSignatureImage writes a cosign-shaped signature image at ref.
func pushSignatureImage(t *testing.T, ref name.Reference, payload []byte, sigB64 string) {
	t.Helper()
	img, err := mutate.Append(empty.Image, mutate.Addendum{
		Layer: static.NewLayer(payload, types.MediaType(simpleSigningMediaType)),
		Annotations: map[string]string{
			signatureAnnotation: sigB64,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(ref, img); err != nil {
		t.Fatal(err)
	}
}

// sidecarTag computes the cosign sidecar tag for a digest-pinned artifact.
func sidecarTag(t *testing.T, artifact name.Digest) name.Tag {
	t.Helper()
	tag := strings.Replace(artifact.DigestStr(), "sha256:", "sha256-", 1) + ".sig"
	return artifact.Context().Tag(tag)
}

func keyVerifier(t *testing.T, keyPath string) *CosignVerifier {
	t.Helper()
	v, err := NewCosignVerifier(Options{KeyPath: keyPath})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestVerifyKeyBasedSuccess(t *testing.T) {
	keyPath, priv := writeTestPublicKey(t)
	_, artifact := testRegistry(t)

	payload, sigB64 := signPayload(t, priv, artifact.DigestStr())
	pushSignatureImage(t, sidecarTag(t, artifact), payload, sigB64)

	if err := keyVerifier(t, keyPath).Verify(context.Background(), artifact.String(), ""); err != nil {
		t.Fatalf("a valid signature must verify, got %v", err)
	}
}

func TestVerifyKeyBasedExplicitSignatureRef(t *testing.T) {
	keyPath, priv := writeTestPublicKey(t)
	_, artifact := testRegistry(t)

	sigRef, err := name.NewTag(artifact.Context().Name() + ":custom-signature-location")
	if err != nil {
		t.Fatal(err)
	}
	payload, sigB64 := signPayload(t, priv, artifact.DigestStr())
	pushSignatureImage(t, sigRef, payload, sigB64)

	if err := keyVerifier(t, keyPath).Verify(context.Background(), artifact.String(), sigRef.String()); err != nil {
		t.Fatalf("a valid signature at an explicit signatureRef must verify, got %v", err)
	}
}

func TestVerifyKeyBasedMissingSignature(t *testing.T) {
	keyPath, _ := writeTestPublicKey(t)
	_, artifact := testRegistry(t)

	err := keyVerifier(t, keyPath).Verify(context.Background(), artifact.String(), "")
	if !IsSignatureMissing(err) {
		t.Fatalf("no signature tag must classify as missing, got %v", err)
	}
	if IsTransient(err) {
		t.Fatal("a missing signature is a definitive verdict, not transient")
	}
}

func TestVerifyKeyBasedWrongKey(t *testing.T) {
	keyPath, _ := writeTestPublicKey(t)
	_, otherKey := writeTestPublicKey(t)
	_, artifact := testRegistry(t)

	// Signed by a different key than the configured one.
	payload, sigB64 := signPayload(t, otherKey, artifact.DigestStr())
	pushSignatureImage(t, sidecarTag(t, artifact), payload, sigB64)

	err := keyVerifier(t, keyPath).Verify(context.Background(), artifact.String(), "")
	if !IsSignatureInvalid(err) {
		t.Fatalf("a wrong-key signature must classify as invalid, got %v", err)
	}
}

func TestVerifyKeyBasedWrongDigestClaim(t *testing.T) {
	keyPath, priv := writeTestPublicKey(t)
	_, artifact := testRegistry(t)

	// A valid signature over a payload claiming a DIFFERENT digest: the
	// crypto holds but the claim binding must reject it.
	payload, sigB64 := signPayload(t, priv, "sha256:"+strings.Repeat("b", 64))
	pushSignatureImage(t, sidecarTag(t, artifact), payload, sigB64)

	err := keyVerifier(t, keyPath).Verify(context.Background(), artifact.String(), "")
	if !IsSignatureInvalid(err) {
		t.Fatalf("a signature for another digest must classify as invalid, got %v", err)
	}
	if !strings.Contains(err.Error(), "not the pinned artifact digest") {
		t.Fatalf("the error should name the digest mismatch, got %v", err)
	}
}

func TestVerifyKeyBasedNoSignatureLayers(t *testing.T) {
	keyPath, _ := writeTestPublicKey(t)
	_, artifact := testRegistry(t)

	// A signature image with no simple signing layers.
	img, err := mutate.AppendLayers(empty.Image,
		static.NewLayer([]byte("{}"), types.MediaType("application/json")))
	if err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(sidecarTag(t, artifact), img); err != nil {
		t.Fatal(err)
	}

	err = keyVerifier(t, keyPath).Verify(context.Background(), artifact.String(), "")
	if !IsSignatureMissing(err) {
		t.Fatalf("a signature manifest without signature layers must classify as missing, got %v", err)
	}
}

func TestVerifyKeyBasedRegistryDownIsTransient(t *testing.T) {
	keyPath, _ := writeTestPublicKey(t)
	srv, artifact := testRegistry(t)
	srv.Close() // the registry is now unreachable

	err := keyVerifier(t, keyPath).Verify(context.Background(), artifact.String(), "")
	if err == nil {
		t.Fatal("expected an error from an unreachable registry")
	}
	if !IsTransient(err) {
		t.Fatalf("an unreachable registry must classify as transient, got %v", err)
	}
	if IsSignatureMissing(err) || IsSignatureInvalid(err) {
		t.Fatalf("a registry outage must never produce a definitive verdict, got %v", err)
	}
}

func TestVerifyRejectsUndigestedArtifactRef(t *testing.T) {
	keyPath, _ := writeTestPublicKey(t)
	err := keyVerifier(t, keyPath).Verify(context.Background(), "ghcr.io/danielnyari/capabilities/echo:0.1.0", "")
	if !IsSignatureInvalid(err) {
		t.Fatalf("a tag-only ref must be a definitive failure, got %v", err)
	}
}

func TestCheckDigestClaim(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	payloadFor := func(claimed string) []byte {
		return fmt.Appendf(nil, `{"critical":{"image":{"docker-manifest-digest":%q}}}`, claimed)
	}

	cases := []struct {
		name    string
		payload []byte
		wantErr string
	}{
		{
			name:    "exact match",
			payload: payloadFor(digest),
		},
		{
			name: "algorithm prefix differs only in case",
			// Digest algorithm identifiers are case-insensitive; a signer
			// emitting SHA256: must not be rejected.
			payload: payloadFor("SHA256:" + strings.Repeat("a", 64)),
		},
		{
			name:    "different digest rejected",
			payload: payloadFor("sha256:" + strings.Repeat("b", 64)),
			wantErr: "not the pinned artifact digest",
		},
		{
			name:    "missing claim rejected",
			payload: []byte(`{"critical":{"image":{}}}`),
			wantErr: "claims no docker-manifest-digest",
		},
		{
			name:    "non-JSON payload rejected",
			payload: []byte("not json"),
			wantErr: "not valid JSON",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkDigestClaim(tc.payload, digest)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected the claim to bind, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v should contain %q", err, tc.wantErr)
			}
		})
	}
}

// --- public key type restrictions ---

func writePublicKeyPEM(t *testing.T, pub any) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cosign.pub")
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewCosignVerifierPublicKeyTypeRestrictions(t *testing.T) {
	rsa1024, err := rsa.GenerateKey(rand.Reader, 1024) //nolint:gosec // deliberately weak: the rejection under test
	if err != nil {
		t.Fatal(err)
	}
	rsa2048, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	edPub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	x25519, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		pub     any
		wantErr string
	}{
		{name: "ed25519 accepted", pub: edPub},
		{name: "rsa 2048 accepted", pub: &rsa2048.PublicKey},
		{name: "rsa 1024 rejected", pub: &rsa1024.PublicKey, wantErr: "1024 bits; at least 2048 bits"},
		{name: "x25519 rejected naming the type", pub: x25519.PublicKey(), wantErr: "unsupported public key type *ecdh.PublicKey"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, err := NewCosignVerifier(Options{KeyPath: writePublicKeyPEM(t, tc.pub)})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected the key to be accepted, got %v", err)
				}
				if v == nil {
					t.Fatal("expected a verifier")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %v should contain %q", err, tc.wantErr)
			}
		})
	}
}

// --- TUF trusted-root TTL ---

// fakeTrustedMaterial is a minimal root.TrustedMaterial for cache tests.
type fakeTrustedMaterial struct {
	root.BaseTrustedMaterial
}

func keylessVerifier(t *testing.T) *CosignVerifier {
	t.Helper()
	v, err := NewCosignVerifier(Options{
		KeylessIssuer:         "https://token.actions.githubusercontent.com",
		KeylessIdentityRegexp: "^https://github.com/danielnyari/flokoa/",
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestFetchTrustedRootCachesWithinTTL(t *testing.T) {
	v := keylessVerifier(t)
	calls := 0
	v.fetchRoot = func() (root.TrustedMaterial, error) {
		calls++
		return &fakeTrustedMaterial{}, nil
	}

	ctx := context.Background()
	first, err := v.fetchTrustedRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := v.fetchTrustedRoot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("a fresh root must be served from cache, fetched %d times", calls)
	}
	if first != second {
		t.Fatal("the cached root instance must be returned")
	}
}

func TestFetchTrustedRootRefetchesAfterTTL(t *testing.T) {
	v := keylessVerifier(t)
	calls := 0
	v.fetchRoot = func() (root.TrustedMaterial, error) {
		calls++
		return &fakeTrustedMaterial{}, nil
	}

	ctx := context.Background()
	if _, err := v.fetchTrustedRoot(ctx); err != nil {
		t.Fatal(err)
	}

	// Back-date the fetch beyond the TTL (same idiom as the controller's
	// verifyState back-dating).
	v.trustedRootMu.Lock()
	v.trustedRootFetchedAt = time.Now().Add(-(trustedRootTTL + time.Second))
	v.trustedRootMu.Unlock()

	if _, err := v.fetchTrustedRoot(ctx); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("an expired root must be re-fetched, fetched %d times", calls)
	}
}

func TestFetchTrustedRootKeepsStaleRootOnRefetchFailure(t *testing.T) {
	v := keylessVerifier(t)
	calls := 0
	stale := &fakeTrustedMaterial{}
	v.fetchRoot = func() (root.TrustedMaterial, error) {
		calls++
		if calls > 1 {
			return nil, errors.New("tuf mirror unavailable")
		}
		return stale, nil
	}

	ctx := context.Background()
	if _, err := v.fetchTrustedRoot(ctx); err != nil {
		t.Fatal(err)
	}
	v.trustedRootMu.Lock()
	v.trustedRootFetchedAt = time.Now().Add(-(trustedRootTTL + time.Second))
	v.trustedRootMu.Unlock()

	// The re-fetch fails: the stale root must keep serving (soft failure),
	// never a transient verify error.
	got, err := v.fetchTrustedRoot(ctx)
	if err != nil {
		t.Fatalf("a re-fetch failure with a cached root must be soft, got %v", err)
	}
	if got != root.TrustedMaterial(stale) {
		t.Fatal("the stale root must be returned on re-fetch failure")
	}
	if calls != 2 {
		t.Fatalf("expected exactly one re-fetch attempt, got %d fetches", calls)
	}
}

func TestFetchTrustedRootFirstFetchFailureIsAnError(t *testing.T) {
	v := keylessVerifier(t)
	v.fetchRoot = func() (root.TrustedMaterial, error) {
		return nil, errors.New("tuf mirror unavailable")
	}
	if _, err := v.fetchTrustedRoot(context.Background()); err == nil {
		t.Fatal("a first fetch failure has no fallback and must error")
	}
}

// --- error classification ---

func TestErrorClassificationHelpers(t *testing.T) {
	missing := &SignatureMissingError{SignatureRef: "r", Err: errors.New("404")}
	invalid := &SignatureInvalidError{SignatureRef: "r", Err: errors.New("bad sig")}
	transient := &TransientError{Err: errors.New("connection refused")}
	plain := errors.New("anything unclassified")

	if !IsSignatureMissing(missing) || IsSignatureMissing(invalid) || IsSignatureMissing(transient) {
		t.Fatal("IsSignatureMissing misclassifies")
	}
	if !IsSignatureInvalid(invalid) || IsSignatureInvalid(missing) {
		t.Fatal("IsSignatureInvalid misclassifies")
	}
	// Unclassified errors default to transient: an unexpected failure mode
	// must degrade to retry, never to a false verdict.
	if !IsTransient(transient) || !IsTransient(plain) {
		t.Fatal("IsTransient must cover explicit and unclassified errors")
	}
	if IsTransient(missing) || IsTransient(invalid) || IsTransient(nil) {
		t.Fatal("IsTransient misclassifies definitive verdicts or nil")
	}
	// Wrapped definitive verdicts keep their classification.
	wrapped := fmt.Errorf("outer: %w", invalid)
	if !IsSignatureInvalid(wrapped) || IsTransient(wrapped) {
		t.Fatal("wrapping must preserve the classification")
	}
}

// --- registry Secret keychain ---

func registrySecret(secretName, namespace string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       data,
	}
}

func TestKeychainFromDockerConfigSecret(t *testing.T) {
	auth := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	doc := fmt.Sprintf(`{"auths":{"https://ghcr.io":{"auth":%q},"registry.example.com":{"username":"u2","password":"p2"}}}`, auth)
	kc, err := keychainFromDockerConfigSecret(registrySecret("reg", "flokoa-system",
		map[string][]byte{corev1.DockerConfigJsonKey: []byte(doc)}))
	if err != nil {
		t.Fatal(err)
	}

	reg, err := name.NewRegistry("ghcr.io")
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := kc.Resolve(reg)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := authenticator.Authorization()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "user" || cfg.Password != "pass" {
		t.Fatalf("auth-entry credentials = %q/%q", cfg.Username, cfg.Password)
	}

	other, err := name.NewRegistry("registry.example.com")
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err = kc.Resolve(other)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err = authenticator.Authorization()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "u2" || cfg.Password != "p2" {
		t.Fatalf("username/password credentials = %q/%q", cfg.Username, cfg.Password)
	}

	unknown, err := name.NewRegistry("unknown.example.com")
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err = kc.Resolve(unknown)
	if err != nil {
		t.Fatal(err)
	}
	if authenticator != authn.Anonymous {
		t.Fatalf("an unmatched registry must resolve to anonymous, got %v", authenticator)
	}
}

func TestKeychainSecretWithoutDockerConfigKey(t *testing.T) {
	_, err := keychainFromDockerConfigSecret(registrySecret("reg", "flokoa-system",
		map[string][]byte{"wrong-key": []byte("{}")}))
	if err == nil || !strings.Contains(err.Error(), corev1.DockerConfigJsonKey) {
		t.Fatalf("a Secret without .dockerconfigjson must be rejected naming the key, got %v", err)
	}
}

func TestVerifierKeychainSecretReadFailureIsTransient(t *testing.T) {
	keyPath, _ := writeTestPublicKey(t)
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	reader := fake.NewClientBuilder().WithScheme(scheme).Build() // no Secret exists

	v, err := NewCosignVerifier(Options{
		KeyPath:            keyPath,
		RegistrySecretName: "absent-secret",
		Namespace:          "flokoa-system",
		SecretReader:       reader,
	})
	if err != nil {
		t.Fatal(err)
	}
	verr := v.Verify(context.Background(),
		"ghcr.io/danielnyari/capabilities/echo@sha256:"+strings.Repeat("a", 64), "")
	if !IsTransient(verr) {
		t.Fatalf("a missing registry Secret must be transient (operator misconfiguration), got %v", verr)
	}
}
