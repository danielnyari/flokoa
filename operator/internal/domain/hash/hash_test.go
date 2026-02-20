package hash

import (
	"testing"
)

func TestConfigMapData(t *testing.T) {
	tests := []struct {
		name string
		data map[string]string
		want string
	}{
		{
			name: "nil map returns empty",
			data: nil,
			want: "",
		},
		{
			name: "empty map returns empty",
			data: map[string]string{},
			want: "",
		},
		{
			name: "single entry produces non-empty hash",
			data: map[string]string{"key": "value"},
			want: "", // just check non-empty below
		},
		{
			name: "deterministic across calls",
			data: map[string]string{"a": "1", "b": "2"},
			want: "", // check equality below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConfigMapData(tt.data)
			if len(tt.data) == 0 {
				if got != "" {
					t.Errorf("ConfigMapData(%v) = %q, want empty string", tt.data, got)
				}
				return
			}

			if got == "" {
				t.Errorf("ConfigMapData(%v) returned empty string, want non-empty", tt.data)
			}

			if len(got) != 16 {
				t.Errorf("ConfigMapData(%v) length = %d, want 16", tt.data, len(got))
			}

			// Verify determinism
			got2 := ConfigMapData(tt.data)
			if got != got2 {
				t.Errorf("ConfigMapData not deterministic: %q != %q", got, got2)
			}
		})
	}
}

func TestConfigMapData_OrderIndependent(t *testing.T) {
	data1 := map[string]string{"a": "1", "b": "2", "c": "3"}
	data2 := map[string]string{"c": "3", "a": "1", "b": "2"}

	hash1 := ConfigMapData(data1)
	hash2 := ConfigMapData(data2)

	if hash1 != hash2 {
		t.Errorf("ConfigMapData should be order-independent: %q != %q", hash1, hash2)
	}
}

func TestConfigMapData_DifferentDataDifferentHash(t *testing.T) {
	hash1 := ConfigMapData(map[string]string{"key": "value1"})
	hash2 := ConfigMapData(map[string]string{"key": "value2"})

	if hash1 == hash2 {
		t.Errorf("different data should produce different hashes: both = %q", hash1)
	}
}

func TestJSONStruct(t *testing.T) {
	type sample struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	t.Run("deterministic across calls", func(t *testing.T) {
		s := sample{Name: "test", Count: 42}
		h1, err := JSONStruct(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		h2, err := JSONStruct(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if h1 != h2 {
			t.Errorf("not deterministic: %q != %q", h1, h2)
		}
	})

	t.Run("16-character hex string", func(t *testing.T) {
		h, err := JSONStruct(sample{Name: "a", Count: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(h) != 16 {
			t.Errorf("length = %d, want 16", len(h))
		}
	})

	t.Run("different structs produce different hashes", func(t *testing.T) {
		h1, _ := JSONStruct(sample{Name: "a", Count: 1})
		h2, _ := JSONStruct(sample{Name: "b", Count: 2})
		if h1 == h2 {
			t.Errorf("different structs should produce different hashes: both = %q", h1)
		}
	})

	t.Run("error on unmarshalable value", func(t *testing.T) {
		_, err := JSONStruct(func() {})
		if err == nil {
			t.Error("expected error for unmarshalable value, got nil")
		}
	})
}

func TestSecretVersions(t *testing.T) {
	tests := []struct {
		name     string
		versions map[string]string
		want     string
	}{
		{
			name:     "nil map returns empty",
			versions: nil,
			want:     "",
		},
		{
			name:     "empty map returns empty",
			versions: map[string]string{},
			want:     "",
		},
		{
			name:     "single secret produces non-empty hash",
			versions: map[string]string{"my-secret": "12345"},
			want:     "", // just check non-empty
		},
		{
			name:     "missing secret included",
			versions: map[string]string{"my-secret": "missing"},
			want:     "", // just check non-empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SecretVersions(tt.versions)
			if len(tt.versions) == 0 {
				if got != "" {
					t.Errorf("SecretVersions(%v) = %q, want empty string", tt.versions, got)
				}
				return
			}

			if got == "" {
				t.Errorf("SecretVersions(%v) returned empty string, want non-empty", tt.versions)
			}

			if len(got) != 16 {
				t.Errorf("SecretVersions(%v) length = %d, want 16", tt.versions, len(got))
			}

			// Verify determinism
			got2 := SecretVersions(tt.versions)
			if got != got2 {
				t.Errorf("SecretVersions not deterministic: %q != %q", got, got2)
			}
		})
	}
}

func TestSecretVersions_OrderIndependent(t *testing.T) {
	v1 := map[string]string{"secret-a": "100", "secret-b": "200"}
	v2 := map[string]string{"secret-b": "200", "secret-a": "100"}

	hash1 := SecretVersions(v1)
	hash2 := SecretVersions(v2)

	if hash1 != hash2 {
		t.Errorf("SecretVersions should be order-independent: %q != %q", hash1, hash2)
	}
}

func TestSecretVersions_DifferentVersionsDifferentHash(t *testing.T) {
	hash1 := SecretVersions(map[string]string{"secret": "100"})
	hash2 := SecretVersions(map[string]string{"secret": "200"})

	if hash1 == hash2 {
		t.Errorf("different versions should produce different hashes: both = %q", hash1)
	}
}
