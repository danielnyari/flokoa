package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
)

// expectedHash computes the same SHA256-based key that ExtractSessionKey produces.
func expectedHash(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:16])
}

func TestExtractSessionKey(t *testing.T) {
	tests := []struct {
		name           string
		sessionKeyFrom string
		eventData      json.RawMessage
		want           string
	}{
		{
			name:           "empty expression returns empty string",
			sessionKeyFrom: "",
			eventData:      json.RawMessage(`{"userId":"abc"}`),
			want:           "",
		},
		{
			name:           "string value extraction",
			sessionKeyFrom: "$.userId",
			eventData:      json.RawMessage(`{"userId":"user-42"}`),
			want:           expectedHash("user-42"),
		},
		{
			name:           "numeric value extraction",
			sessionKeyFrom: "$.count",
			eventData:      json.RawMessage(`{"count":7}`),
			want:           expectedHash(fmt.Sprintf("%v", float64(7))),
		},
		{
			name:           "nested path extraction",
			sessionKeyFrom: "$.body.user.id",
			eventData:      json.RawMessage(`{"body":{"user":{"id":"nested-123"}}}`),
			want:           expectedHash("nested-123"),
		},
		{
			name:           "invalid JSONPath returns empty string",
			sessionKeyFrom: "$[invalid???",
			eventData:      json.RawMessage(`{"foo":"bar"}`),
			want:           "",
		},
		{
			name:           "invalid JSON data returns empty string",
			sessionKeyFrom: "$.userId",
			eventData:      json.RawMessage(`not valid json`),
			want:           "",
		},
		{
			name:           "missing field returns empty string",
			sessionKeyFrom: "$.nonexistent",
			eventData:      json.RawMessage(`{"userId":"abc"}`),
			want:           "",
		},
		{
			name:           "boolean value extraction",
			sessionKeyFrom: "$.active",
			eventData:      json.RawMessage(`{"active":true}`),
			want:           expectedHash("true"),
		},
		{
			name:           "object value is JSON-marshaled",
			sessionKeyFrom: "$.meta",
			eventData:      json.RawMessage(`{"meta":{"key":"val"}}`),
			want:           expectedHash(`{"key":"val"}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractSessionKey(tt.sessionKeyFrom, tt.eventData)
			if got != tt.want {
				t.Errorf("ExtractSessionKey(%q, %s) = %q, want %q",
					tt.sessionKeyFrom, tt.eventData, got, tt.want)
			}
		})
	}
}

func TestExtractSessionKey_Determinism(t *testing.T) {
	data := json.RawMessage(`{"userId":"deterministic-test"}`)
	expr := "$.userId"

	first := ExtractSessionKey(expr, data)
	if first == "" {
		t.Fatal("expected non-empty session key")
	}

	for i := range 100 {
		got := ExtractSessionKey(expr, data)
		if got != first {
			t.Fatalf("iteration %d: got %q, want %q (non-deterministic)", i, got, first)
		}
	}
}

func TestExtractSessionKey_OutputLength(t *testing.T) {
	data := json.RawMessage(`{"id":"len-check"}`)
	got := ExtractSessionKey("$.id", data)

	// SHA256 truncated to first 16 bytes => 32 hex characters
	if len(got) != 32 {
		t.Fatalf("expected 32-char hex string, got %d chars: %q", len(got), got)
	}
}
