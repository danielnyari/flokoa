package converter

import (
	"testing"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestModelProviderToProto_Nil(t *testing.T) {
	result := ModelProviderToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelProviderToProto(t *testing.T) {
	useSystemCAs := true
	provider := &agentv1alpha1.ModelProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openai-provider",
			Namespace: "providers",
		},
		Spec: agentv1alpha1.ModelProviderSpec{
			APIKeySecretRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "api-secret"},
				Key:                  "api-key",
			},
			OpenAI: &agentv1alpha1.OpenAIProviderSpec{
				BaseURL: "https://api.openai.com",
			},
			DefaultHeaders: map[string]string{"X-Custom": "value"},
			TLS: &agentv1alpha1.TLSConfig{
				InsecureSkipVerify: false,
				UseSystemCAs:       &useSystemCAs,
			},
		},
		Status: agentv1alpha1.ModelProviderStatus{
			Provider:           agentv1alpha1.ProviderTypeOpenAI,
			ObservedGeneration: 2,
			SecretHash:         "abc123",
			Ready:              true,
		},
	}

	result := ModelProviderToProto(provider)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Metadata == nil || result.Metadata.Name != "openai-provider" {
		t.Fatal("expected metadata with name")
	}
	if result.Spec == nil {
		t.Fatal("expected spec")
	}
	if result.Status == nil {
		t.Fatal("expected status")
	}
}

func TestModelProviderSpecToProto_Nil(t *testing.T) {
	result := ModelProviderSpecToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelProviderSpecToProto_OpenAI(t *testing.T) {
	spec := &agentv1alpha1.ModelProviderSpec{
		APIKeySecretRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "openai-secret"},
			Key:                  "api-key",
		},
		OpenAI: &agentv1alpha1.OpenAIProviderSpec{
			BaseURL: "https://custom.openai.com",
		},
		DefaultHeaders: map[string]string{"X-H": "V"},
	}

	result := ModelProviderSpecToProto(spec)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ApiKeySecretRef == nil {
		t.Fatal("expected api key secret ref")
	}
	if result.ApiKeySecretRef.Name != "openai-secret" {
		t.Fatalf("expected openai-secret, got %q", result.ApiKeySecretRef.Name)
	}
	if result.ApiKeySecretRef.Key != "api-key" {
		t.Fatalf("expected api-key, got %q", result.ApiKeySecretRef.Key)
	}
	if result.Openai == nil {
		t.Fatal("expected openai spec")
	}
	if result.Openai.BaseUrl != "https://custom.openai.com" {
		t.Fatalf("expected base url, got %q", result.Openai.BaseUrl)
	}
	if result.DefaultHeaders["X-H"] != "V" {
		t.Fatal("expected default headers")
	}
	if result.Anthropic != nil || result.Google != nil || result.Bedrock != nil {
		t.Fatal("expected only openai to be set")
	}
}

func TestModelProviderSpecToProto_Anthropic(t *testing.T) {
	spec := &agentv1alpha1.ModelProviderSpec{
		Anthropic: &agentv1alpha1.AnthropicProviderSpec{
			BaseURL: "https://custom.anthropic.com",
		},
	}

	result := ModelProviderSpecToProto(spec)
	if result.Anthropic == nil {
		t.Fatal("expected anthropic spec")
	}
	if result.Anthropic.BaseUrl != "https://custom.anthropic.com" {
		t.Fatalf("expected base url, got %q", result.Anthropic.BaseUrl)
	}
	if result.Openai != nil {
		t.Fatal("expected nil openai")
	}
}

func TestModelProviderSpecToProto_Google(t *testing.T) {
	spec := &agentv1alpha1.ModelProviderSpec{
		Google: &agentv1alpha1.GoogleProviderSpec{
			Project:  "my-project",
			Location: "us-central1",
		},
	}

	result := ModelProviderSpecToProto(spec)
	if result.Google == nil {
		t.Fatal("expected google spec")
	}
	if result.Google.Project != "my-project" {
		t.Fatalf("expected project, got %q", result.Google.Project)
	}
	if result.Google.Location != "us-central1" {
		t.Fatalf("expected location, got %q", result.Google.Location)
	}
}

func TestModelProviderSpecToProto_Bedrock(t *testing.T) {
	spec := &agentv1alpha1.ModelProviderSpec{
		Bedrock: &agentv1alpha1.BedrockProviderSpec{
			Region: "us-east-1",
		},
	}

	result := ModelProviderSpecToProto(spec)
	if result.Bedrock == nil {
		t.Fatal("expected bedrock spec")
	}
	if result.Bedrock.Region != "us-east-1" {
		t.Fatalf("expected region, got %q", result.Bedrock.Region)
	}
}

func TestModelProviderSpecToProto_TLS(t *testing.T) {
	useSystemCAs := true
	spec := &agentv1alpha1.ModelProviderSpec{
		OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
		TLS: &agentv1alpha1.TLSConfig{
			InsecureSkipVerify: true,
			UseSystemCAs:       &useSystemCAs,
		},
	}

	result := ModelProviderSpecToProto(spec)
	if result.Tls == nil {
		t.Fatal("expected TLS config")
	}
	if !result.Tls.InsecureSkipVerify {
		t.Fatal("expected insecure skip verify true")
	}
	if !result.Tls.UseSystemCas {
		t.Fatal("expected use system CAs true")
	}
}

func TestModelProviderSpecToProto_TLS_NilUseSystemCAs(t *testing.T) {
	spec := &agentv1alpha1.ModelProviderSpec{
		OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
		TLS: &agentv1alpha1.TLSConfig{
			InsecureSkipVerify: false,
		},
	}

	result := ModelProviderSpecToProto(spec)
	if result.Tls == nil {
		t.Fatal("expected TLS config")
	}
	if result.Tls.UseSystemCas {
		t.Fatal("expected use system CAs false for nil")
	}
}

func TestModelProviderSpecToProto_NilOptionalFields(t *testing.T) {
	spec := &agentv1alpha1.ModelProviderSpec{}

	result := ModelProviderSpecToProto(spec)
	if result.ApiKeySecretRef != nil {
		t.Fatal("expected nil api key ref")
	}
	if result.Openai != nil {
		t.Fatal("expected nil openai")
	}
	if result.Anthropic != nil {
		t.Fatal("expected nil anthropic")
	}
	if result.Google != nil {
		t.Fatal("expected nil google")
	}
	if result.Bedrock != nil {
		t.Fatal("expected nil bedrock")
	}
	if result.Tls != nil {
		t.Fatal("expected nil tls")
	}
}

func TestModelProviderStatusToProto_Nil(t *testing.T) {
	result := ModelProviderStatusToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelProviderStatusToProto(t *testing.T) {
	status := &agentv1alpha1.ModelProviderStatus{
		Provider:           agentv1alpha1.ProviderTypeOpenAI,
		ObservedGeneration: 4,
		SecretHash:         "hash123",
		Ready:              true,
		Conditions: []metav1.Condition{
			{Type: "Ready", Status: metav1.ConditionTrue, Reason: "OK"},
		},
	}

	result := ModelProviderStatusToProto(status)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Provider != pb.ProviderType_PROVIDER_TYPE_OPENAI {
		t.Fatalf("expected openai provider, got %v", result.Provider)
	}
	if result.ObservedGeneration != 4 {
		t.Fatalf("expected observed gen 4, got %d", result.ObservedGeneration)
	}
	if result.SecretHash != "hash123" {
		t.Fatalf("expected hash, got %q", result.SecretHash)
	}
	if !result.Ready {
		t.Fatal("expected ready true")
	}
	if len(result.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(result.Conditions))
	}
}

func TestModelProviderListToProto_Nil(t *testing.T) {
	result := ModelProviderListToProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelProviderListToProto(t *testing.T) {
	list := &agentv1alpha1.ModelProviderList{
		ListMeta: metav1.ListMeta{ResourceVersion: "30"},
		Items: []agentv1alpha1.ModelProvider{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "provider-1"},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "provider-2"},
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
				},
			},
		},
	}

	result := ModelProviderListToProto(list)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Metadata == nil || result.Metadata.ResourceVersion != "30" {
		t.Fatal("expected metadata")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
}

func TestModelProviderListToProto_Empty(t *testing.T) {
	list := &agentv1alpha1.ModelProviderList{
		Items: []agentv1alpha1.ModelProvider{},
	}

	result := ModelProviderListToProto(list)
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
}

func TestModelProviderFromProto_Nil(t *testing.T) {
	result := ModelProviderFromProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelProviderFromProto(t *testing.T) {
	proto := &pb.ModelProvider{
		Metadata: &pb.ObjectMeta{
			Name:      "openai-provider",
			Namespace: "ns",
		},
		Spec: &pb.ModelProviderSpec{
			Openai: &pb.OpenAIProviderSpec{
				BaseUrl: "https://api.openai.com",
			},
			DefaultHeaders: map[string]string{"X-H": "V"},
		},
	}

	result := ModelProviderFromProto(proto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Name != "openai-provider" {
		t.Fatalf("expected name, got %q", result.Name)
	}
	if result.Spec.OpenAI == nil {
		t.Fatal("expected openai spec")
	}
	if result.Spec.OpenAI.BaseURL != "https://api.openai.com" {
		t.Fatalf("expected base url, got %q", result.Spec.OpenAI.BaseURL)
	}
	if result.Spec.DefaultHeaders["X-H"] != "V" {
		t.Fatal("expected default headers")
	}
}

func TestModelProviderFromProto_NilFields(t *testing.T) {
	proto := &pb.ModelProvider{}

	result := ModelProviderFromProto(proto)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestModelProviderSpecFromProto_Nil(t *testing.T) {
	result := ModelProviderSpecFromProto(nil)
	if result != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestModelProviderSpecFromProto_AllProviders(t *testing.T) {
	t.Run("openai", func(t *testing.T) {
		proto := &pb.ModelProviderSpec{
			Openai: &pb.OpenAIProviderSpec{BaseUrl: "https://openai.example.com"},
		}
		result := ModelProviderSpecFromProto(proto)
		if result.OpenAI == nil {
			t.Fatal("expected openai")
		}
		if result.OpenAI.BaseURL != "https://openai.example.com" {
			t.Fatalf("expected base url, got %q", result.OpenAI.BaseURL)
		}
	})

	t.Run("anthropic", func(t *testing.T) {
		proto := &pb.ModelProviderSpec{
			Anthropic: &pb.AnthropicProviderSpec{BaseUrl: "https://anthropic.example.com"},
		}
		result := ModelProviderSpecFromProto(proto)
		if result.Anthropic == nil {
			t.Fatal("expected anthropic")
		}
		if result.Anthropic.BaseURL != "https://anthropic.example.com" {
			t.Fatalf("expected base url, got %q", result.Anthropic.BaseURL)
		}
	})

	t.Run("google", func(t *testing.T) {
		proto := &pb.ModelProviderSpec{
			Google: &pb.GoogleProviderSpec{Project: "proj", Location: "loc"},
		}
		result := ModelProviderSpecFromProto(proto)
		if result.Google == nil {
			t.Fatal("expected google")
		}
		if result.Google.Project != "proj" {
			t.Fatalf("expected project, got %q", result.Google.Project)
		}
		if result.Google.Location != "loc" {
			t.Fatalf("expected location, got %q", result.Google.Location)
		}
	})

	t.Run("bedrock", func(t *testing.T) {
		proto := &pb.ModelProviderSpec{
			Bedrock: &pb.BedrockProviderSpec{Region: "us-west-2"},
		}
		result := ModelProviderSpecFromProto(proto)
		if result.Bedrock == nil {
			t.Fatal("expected bedrock")
		}
		if result.Bedrock.Region != "us-west-2" {
			t.Fatalf("expected region, got %q", result.Bedrock.Region)
		}
	})
}

func TestModelProviderSpecRoundTrip_OpenAI(t *testing.T) {
	original := &agentv1alpha1.ModelProviderSpec{
		OpenAI:         &agentv1alpha1.OpenAIProviderSpec{BaseURL: "https://openai.example.com"},
		DefaultHeaders: map[string]string{"X-Key": "val"},
	}

	proto := ModelProviderSpecToProto(original)
	result := ModelProviderSpecFromProto(proto)

	if result.OpenAI == nil || result.OpenAI.BaseURL != original.OpenAI.BaseURL {
		t.Fatal("openai base url mismatch")
	}
	if result.DefaultHeaders["X-Key"] != "val" {
		t.Fatal("default headers mismatch")
	}
}

func TestModelProviderSpecRoundTrip_Google(t *testing.T) {
	original := &agentv1alpha1.ModelProviderSpec{
		Google: &agentv1alpha1.GoogleProviderSpec{
			Project:  "project-1",
			Location: "us-central1",
		},
	}

	proto := ModelProviderSpecToProto(original)
	result := ModelProviderSpecFromProto(proto)

	if result.Google == nil {
		t.Fatal("expected google spec")
	}
	if result.Google.Project != original.Google.Project {
		t.Fatal("project mismatch")
	}
	if result.Google.Location != original.Google.Location {
		t.Fatal("location mismatch")
	}
}
