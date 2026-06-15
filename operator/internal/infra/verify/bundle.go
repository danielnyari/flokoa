package verify

// Sigstore bundle reconstruction from a classic cosign OCI signature layer.
// Adapted from sigstore-go's examples/oci-image-verification (Apache-2.0,
// Copyright 2023 The Sigstore Authors): the signature image's layer
// annotations carry the Fulcio certificate, the Rekor bundle, and the
// signature; reassembled as a protobuf Sigstore bundle they verify through
// the stable sigstore-go/pkg/verify path.

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"

	ociv1 "github.com/google/go-containerregistry/pkg/v1"
	protobundle "github.com/sigstore/protobuf-specs/gen/pb-go/bundle/v1"
	protocommon "github.com/sigstore/protobuf-specs/gen/pb-go/common/v1"
	protorekor "github.com/sigstore/protobuf-specs/gen/pb-go/rekor/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
)

const (
	// rekorBundleAnnotation holds cosign's JSON Rekor bundle (inclusion
	// promise + canonicalized entry body).
	rekorBundleAnnotation = "dev.sigstore.cosign/bundle"

	// timestampAnnotation holds an optional RFC 3161 signed timestamp.
	timestampAnnotation = "dev.sigstore.cosign/rfc3161timestamp"
)

// rekorBundleDoc mirrors cosign's Rekor bundle annotation JSON.
type rekorBundleDoc struct {
	SignedEntryTimestamp string `json:"SignedEntryTimestamp"`
	Payload              struct {
		Body           string `json:"body"`
		IntegratedTime int64  `json:"integratedTime"`
		LogIndex       int64  `json:"logIndex"`
		LogID          string `json:"logID"`
	} `json:"Payload"`
}

// bundleFromSignatureLayer reconstructs the Sigstore bundle for one cosign
// signature layer: signing certificate, transparency log entry, optional
// RFC 3161 timestamp, and the message signature over the simple signing
// payload (whose digest is the layer digest).
func bundleFromSignatureLayer(layer ociv1.Descriptor) (*bundle.Bundle, error) {
	verificationMaterial, err := verificationMaterialFromLayer(layer)
	if err != nil {
		return nil, err
	}
	msgSignature, err := messageSignatureFromLayer(layer)
	if err != nil {
		return nil, err
	}
	mediaType, err := bundle.MediaTypeString("0.1")
	if err != nil {
		return nil, fmt.Errorf("resolving the bundle media type: %w", err)
	}
	return bundle.NewBundle(&protobundle.Bundle{
		MediaType:            mediaType,
		VerificationMaterial: verificationMaterial,
		Content:              msgSignature,
	})
}

func verificationMaterialFromLayer(layer ociv1.Descriptor) (*protobundle.VerificationMaterial, error) {
	block, _ := pem.Decode([]byte(layer.Annotations[certificateAnnotation]))
	if block == nil {
		return nil, errors.New("the certificate annotation is not PEM")
	}

	tlogEntries, err := tlogEntriesFromLayer(layer)
	if err != nil {
		return nil, err
	}

	material := &protobundle.VerificationMaterial{
		Content: &protobundle.VerificationMaterial_X509CertificateChain{
			X509CertificateChain: &protocommon.X509CertificateChain{
				Certificates: []*protocommon.X509Certificate{{RawBytes: block.Bytes}},
			},
		},
		TlogEntries: tlogEntries,
	}

	if ts := layer.Annotations[timestampAnnotation]; ts != "" {
		timestamps, err := timestampsFromAnnotation(ts)
		if err != nil {
			return nil, err
		}
		material.TimestampVerificationData = timestamps
	}
	return material, nil
}

func tlogEntriesFromLayer(layer ociv1.Descriptor) ([]*protorekor.TransparencyLogEntry, error) {
	raw := layer.Annotations[rekorBundleAnnotation]
	if raw == "" {
		return nil, errors.New("the signature carries no Rekor bundle annotation")
	}
	var doc rekorBundleDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("parsing the Rekor bundle annotation: %w", err)
	}

	logID, err := hex.DecodeString(doc.Payload.LogID)
	if err != nil {
		return nil, fmt.Errorf("decoding the Rekor logID: %w", err)
	}
	signedEntryTimestamp, err := base64.StdEncoding.DecodeString(doc.SignedEntryTimestamp)
	if err != nil {
		return nil, fmt.Errorf("decoding the Rekor SignedEntryTimestamp: %w", err)
	}
	body, err := base64.StdEncoding.DecodeString(doc.Payload.Body)
	if err != nil {
		return nil, fmt.Errorf("decoding the Rekor entry body: %w", err)
	}
	var entry struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(body, &entry); err != nil {
		return nil, fmt.Errorf("parsing the Rekor entry body: %w", err)
	}

	return []*protorekor.TransparencyLogEntry{{
		LogIndex:          doc.Payload.LogIndex,
		LogId:             &protocommon.LogId{KeyId: logID},
		KindVersion:       &protorekor.KindVersion{Kind: entry.Kind, Version: entry.APIVersion},
		IntegratedTime:    doc.Payload.IntegratedTime,
		InclusionPromise:  &protorekor.InclusionPromise{SignedEntryTimestamp: signedEntryTimestamp},
		CanonicalizedBody: body,
	}}, nil
}

func timestampsFromAnnotation(annotation string) (*protobundle.TimestampVerificationData, error) {
	var keyVals map[string]string
	if err := json.Unmarshal([]byte(annotation), &keyVals); err != nil {
		return nil, fmt.Errorf("parsing the RFC 3161 timestamp annotation: %w", err)
	}
	der, err := base64.StdEncoding.DecodeString(keyVals["SignedRFC3161Timestamp"])
	if err != nil {
		return nil, fmt.Errorf("decoding the RFC 3161 timestamp: %w", err)
	}
	return &protobundle.TimestampVerificationData{
		Rfc3161Timestamps: []*protocommon.RFC3161SignedTimestamp{{SignedTimestamp: der}},
	}, nil
}

func messageSignatureFromLayer(layer ociv1.Descriptor) (*protobundle.Bundle_MessageSignature, error) {
	if layer.Digest.Algorithm != "sha256" {
		return nil, fmt.Errorf("unsupported signature payload digest algorithm %q", layer.Digest.Algorithm)
	}
	digest, err := hex.DecodeString(layer.Digest.Hex)
	if err != nil {
		return nil, fmt.Errorf("decoding the payload digest: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(layer.Annotations[signatureAnnotation])
	if err != nil {
		return nil, fmt.Errorf("decoding the signature annotation: %w", err)
	}
	if len(sig) == 0 {
		return nil, errors.New("the signature annotation is empty")
	}
	return &protobundle.Bundle_MessageSignature{
		MessageSignature: &protocommon.MessageSignature{
			MessageDigest: &protocommon.HashOutput{
				Algorithm: protocommon.HashAlgorithm_SHA2_256,
				Digest:    digest,
			},
			Signature: sig,
		},
	}, nil
}
