package verify

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	ociv1 "github.com/google/go-containerregistry/pkg/v1"
)

// testCertPEM builds a self-signed certificate PEM (the bundle assembly only
// decodes the PEM; chain validation happens later, in sigstore-go).
func testCertPEM(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func rekorBundleAnnotationJSON(t *testing.T, payloadDigestHex string) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	digestBytes, err := hex.DecodeString(payloadDigestHex)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digestBytes)
	if err != nil {
		t.Fatal(err)
	}

	// A self-consistent hashedrekord body: bundle assembly hands it to
	// sigstore-go, which validates the entry cryptography while parsing.
	body, err := json.Marshal(map[string]any{
		"apiVersion": "0.0.1",
		"kind":       "hashedrekord",
		"spec": map[string]any{
			"data": map[string]any{
				"hash": map[string]any{"algorithm": "sha256", "value": payloadDigestHex},
			},
			"signature": map[string]any{
				"content": base64.StdEncoding.EncodeToString(sig),
				"publicKey": map[string]any{
					"content": base64.StdEncoding.EncodeToString(pubPEM),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := json.Marshal(map[string]any{
		"SignedEntryTimestamp": base64.StdEncoding.EncodeToString([]byte("set")),
		"Payload": map[string]any{
			"body":           base64.StdEncoding.EncodeToString(body),
			"integratedTime": 1701205628,
			"logIndex":       53194260,
			"logID":          strings.Repeat("ab", 32),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(doc)
}

func signatureLayerDescriptor(t *testing.T, annotations map[string]string) ociv1.Descriptor {
	t.Helper()
	digest, err := ociv1.NewHash("sha256:" + strings.Repeat("c", 64))
	if err != nil {
		t.Fatal(err)
	}
	return ociv1.Descriptor{
		MediaType:   simpleSigningMediaType,
		Digest:      digest,
		Annotations: annotations,
	}
}

func TestBundleFromSignatureLayer(t *testing.T) {
	layer := signatureLayerDescriptor(t, map[string]string{
		signatureAnnotation:   base64.StdEncoding.EncodeToString([]byte("sig-bytes")),
		certificateAnnotation: testCertPEM(t),
		rekorBundleAnnotation: rekorBundleAnnotationJSON(t, strings.Repeat("c", 64)),
	})

	b, err := bundleFromSignatureLayer(layer)
	if err != nil {
		t.Fatalf("a complete signature layer must assemble, got %v", err)
	}

	pb := b.Bundle
	if pb.GetVerificationMaterial().GetX509CertificateChain() == nil {
		t.Fatal("the bundle must carry the signing certificate chain")
	}
	entries := pb.GetVerificationMaterial().GetTlogEntries()
	if len(entries) != 1 {
		t.Fatalf("tlog entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.GetLogIndex() != 53194260 || entry.GetIntegratedTime() != 1701205628 {
		t.Fatalf("tlog entry fields = %+v", entry)
	}
	if entry.GetKindVersion().GetKind() != "hashedrekord" || entry.GetKindVersion().GetVersion() != "0.0.1" {
		t.Fatalf("kindVersion = %+v", entry.GetKindVersion())
	}
	wantLogID, _ := hex.DecodeString(strings.Repeat("ab", 32))
	if string(entry.GetLogId().GetKeyId()) != string(wantLogID) {
		t.Fatal("logID must be hex-decoded into the keyId")
	}
	msgSig := pb.GetMessageSignature()
	if msgSig == nil || string(msgSig.GetSignature()) != "sig-bytes" {
		t.Fatalf("message signature = %+v", msgSig)
	}
	wantDigest, _ := hex.DecodeString(strings.Repeat("c", 64))
	if string(msgSig.GetMessageDigest().GetDigest()) != string(wantDigest) {
		t.Fatal("the message digest must be the signature layer digest")
	}
}

func TestBundleFromSignatureLayerMissingCertificate(t *testing.T) {
	layer := signatureLayerDescriptor(t, map[string]string{
		signatureAnnotation:   base64.StdEncoding.EncodeToString([]byte("sig")),
		rekorBundleAnnotation: rekorBundleAnnotationJSON(t, strings.Repeat("c", 64)),
	})
	if _, err := bundleFromSignatureLayer(layer); err == nil || !strings.Contains(err.Error(), "not PEM") {
		t.Fatalf("a missing certificate must fail assembly, got %v", err)
	}
}

func TestBundleFromSignatureLayerMissingRekorBundle(t *testing.T) {
	layer := signatureLayerDescriptor(t, map[string]string{
		signatureAnnotation:   base64.StdEncoding.EncodeToString([]byte("sig")),
		certificateAnnotation: testCertPEM(t),
	})
	if _, err := bundleFromSignatureLayer(layer); err == nil || !strings.Contains(err.Error(), "Rekor bundle") {
		t.Fatalf("a missing Rekor bundle must fail assembly, got %v", err)
	}
}
