package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// ConfigMapData computes a deterministic hash of ConfigMap data for change detection.
// Keys are sorted to ensure consistent output regardless of map iteration order.
// Returns an empty string if the data map is empty.
func ConfigMapData(data map[string]string) string {
	if len(data) == 0 {
		return ""
	}

	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(data[k]))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// JSONStruct computes a deterministic hash of a struct by JSON-serializing it.
// Go's encoding/json produces deterministic output for the same struct values
// (map keys are sorted). Returns a 16-character hex string.
func JSONStruct(v interface{}) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])[:16], nil
}

// SecretVersions computes a hash over secret name:version pairs.
// This is the pure part of secret hash computation — the caller is responsible
// for fetching the secret resource versions and identifying missing secrets.
// The versions map should contain secretName -> resourceVersion (or "missing").
// Returns an empty string if the versions map is empty.
func SecretVersions(versions map[string]string) string {
	if len(versions) == 0 {
		return ""
	}

	names := make([]string, 0, len(versions))
	for name := range versions {
		names = append(names, name)
	}
	sort.Strings(names)

	var accumulator string
	for _, name := range names {
		accumulator += name + ":" + versions[name] + ";"
	}

	h := sha256.Sum256([]byte(accumulator))
	return hex.EncodeToString(h[:])[:16]
}
