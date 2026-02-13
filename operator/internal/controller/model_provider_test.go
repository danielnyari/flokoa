package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

func TestGetProviderHandler(t *testing.T) {
	tests := []struct {
		name         string
		providerType agentv1alpha1.ProviderType
		wantOK       bool
	}{
		{name: "openai", providerType: agentv1alpha1.ProviderTypeOpenAI, wantOK: true},
		{name: "anthropic", providerType: agentv1alpha1.ProviderTypeAnthropic, wantOK: true},
		{name: "google", providerType: agentv1alpha1.ProviderTypeGoogle, wantOK: true},
		{name: "bedrock", providerType: agentv1alpha1.ProviderTypeBedrock, wantOK: true},
		{name: "unknown", providerType: agentv1alpha1.ProviderType("unknown"), wantOK: false},
		{name: "empty", providerType: agentv1alpha1.ProviderType(""), wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, ok := GetProviderHandler(tt.providerType)
			if ok != tt.wantOK {
				t.Fatalf("GetProviderHandler(%q) ok = %v, want %v", tt.providerType, ok, tt.wantOK)
			}
			if tt.wantOK && handler == nil {
				t.Fatal("expected non-nil handler")
			}
			if !tt.wantOK && handler != nil {
				t.Fatal("expected nil handler for unknown provider")
			}
		})
	}
}

func TestBuildBaseConfig(t *testing.T) {
	t.Run("openai provider", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				OpenAI:         &agentv1alpha1.OpenAIProviderSpec{BaseURL: "https://api.openai.com"},
				DefaultHeaders: map[string]string{"X-Custom": "value"},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{
				Model: "gpt-4o",
				Parameters: &agentv1alpha1.ModelParameters{
					Temperature: "0.7",
				},
			},
		}

		config := buildBaseConfig(provider, model)
		if config.Model != "gpt-4o" {
			t.Fatalf("expected model gpt-4o, got %q", config.Model)
		}
		if config.Provider.Type != agentv1alpha1.ProviderTypeOpenAI {
			t.Fatalf("expected openai provider type, got %q", config.Provider.Type)
		}
		if config.Provider.OpenAI == nil {
			t.Fatal("expected OpenAI spec to be set")
		}
		if config.Provider.OpenAI.BaseURL != "https://api.openai.com" {
			t.Fatalf("expected base URL, got %q", config.Provider.OpenAI.BaseURL)
		}
		if config.Provider.DefaultHeaders["X-Custom"] != "value" {
			t.Fatal("expected default headers to be propagated")
		}
		if config.Parameters == nil || config.Parameters.Temperature != "0.7" {
			t.Fatal("expected parameters to be propagated")
		}
	})

	t.Run("anthropic provider", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Anthropic: &agentv1alpha1.AnthropicProviderSpec{BaseURL: "https://api.anthropic.com"},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "claude-sonnet-4-20250514"},
		}

		config := buildBaseConfig(provider, model)
		if config.Provider.Anthropic == nil {
			t.Fatal("expected Anthropic spec to be set")
		}
		if config.Provider.OpenAI != nil || config.Provider.Google != nil || config.Provider.Bedrock != nil {
			t.Fatal("expected only Anthropic spec to be set")
		}
	})

	t.Run("google provider", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Google: &agentv1alpha1.GoogleProviderSpec{Project: "my-project", Location: "us-central1"},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "gemini-pro"},
		}

		config := buildBaseConfig(provider, model)
		if config.Provider.Google == nil {
			t.Fatal("expected Google spec to be set")
		}
		if config.Provider.Google.Project != "my-project" {
			t.Fatalf("expected project my-project, got %q", config.Provider.Google.Project)
		}
	})

	t.Run("bedrock provider", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Bedrock: &agentv1alpha1.BedrockProviderSpec{Region: "us-east-1"},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "anthropic.claude-v2"},
		}

		config := buildBaseConfig(provider, model)
		if config.Provider.Bedrock == nil {
			t.Fatal("expected Bedrock spec to be set")
		}
		if config.Provider.Bedrock.Region != "us-east-1" {
			t.Fatalf("expected region us-east-1, got %q", config.Provider.Bedrock.Region)
		}
	})

	t.Run("nil parameters", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "gpt-4o"},
		}

		config := buildBaseConfig(provider, model)
		if config.Parameters != nil {
			t.Fatal("expected nil parameters")
		}
	})
}

func TestAddAPIKeyEnvVar(t *testing.T) {
	t.Run("adds secret env var", func(t *testing.T) {
		config := &ResolvedModelConfig{}
		secretRef := &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
			Key:                  "api-key",
		}

		addAPIKeyEnvVar(config, secretRef, "OPENAI_API_KEY")

		if len(config.SecretEnvVars) != 1 {
			t.Fatalf("expected 1 secret env var, got %d", len(config.SecretEnvVars))
		}
		if config.SecretEnvVars[0].Name != "OPENAI_API_KEY" {
			t.Fatalf("expected OPENAI_API_KEY, got %q", config.SecretEnvVars[0].Name)
		}
		if config.SecretEnvVars[0].ValueFrom == nil || config.SecretEnvVars[0].ValueFrom.SecretKeyRef == nil {
			t.Fatal("expected secret key ref to be set")
		}
		if config.SecretEnvVars[0].ValueFrom.SecretKeyRef.Name != "my-secret" {
			t.Fatalf("expected secret name my-secret, got %q", config.SecretEnvVars[0].ValueFrom.SecretKeyRef.Name)
		}
	})

	t.Run("nil secret ref is noop", func(t *testing.T) {
		config := &ResolvedModelConfig{}
		addAPIKeyEnvVar(config, nil, "OPENAI_API_KEY")

		if len(config.SecretEnvVars) != 0 {
			t.Fatalf("expected 0 secret env vars, got %d", len(config.SecretEnvVars))
		}
	})
}

func TestOpenAIProviderHandler_BuildConfig(t *testing.T) {
	handler := &OpenAIProviderHandler{}

	t.Run("with base URL and API key", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{
					BaseURL: "https://custom-openai.example.com",
				},
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "openai-secret"},
					Key:                  "api-key",
				},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "gpt-4o"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config.Model != "gpt-4o" {
			t.Fatalf("expected model gpt-4o, got %q", config.Model)
		}

		// Check API key env var
		if len(config.SecretEnvVars) != 1 {
			t.Fatalf("expected 1 secret env var, got %d", len(config.SecretEnvVars))
		}
		if config.SecretEnvVars[0].Name != "OPENAI_API_KEY" {
			t.Fatalf("expected OPENAI_API_KEY, got %q", config.SecretEnvVars[0].Name)
		}

		// Check base URL env var
		foundBaseURL := false
		for _, env := range config.EnvVars {
			if env.Name == "OPENAI_BASE_URL" && env.Value == "https://custom-openai.example.com" {
				foundBaseURL = true
			}
		}
		if !foundBaseURL {
			t.Fatal("expected OPENAI_BASE_URL env var")
		}
	})

	t.Run("without base URL", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "gpt-4o"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, env := range config.EnvVars {
			if env.Name == "OPENAI_BASE_URL" {
				t.Fatal("expected no OPENAI_BASE_URL env var when BaseURL is empty")
			}
		}
	})

	t.Run("without API key", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "gpt-4o"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(config.SecretEnvVars) != 0 {
			t.Fatalf("expected 0 secret env vars, got %d", len(config.SecretEnvVars))
		}
	})
}

func TestAnthropicProviderHandler_BuildConfig(t *testing.T) {
	handler := &AnthropicProviderHandler{}

	t.Run("with base URL and API key", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Anthropic: &agentv1alpha1.AnthropicProviderSpec{
					BaseURL: "https://custom-anthropic.example.com",
				},
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "anthropic-secret"},
					Key:                  "api-key",
				},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "claude-sonnet-4-20250514"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config.Model != "claude-sonnet-4-20250514" {
			t.Fatalf("expected model claude-sonnet-4-20250514, got %q", config.Model)
		}

		// Check API key env var
		if len(config.SecretEnvVars) != 1 {
			t.Fatalf("expected 1 secret env var, got %d", len(config.SecretEnvVars))
		}
		if config.SecretEnvVars[0].Name != "ANTHROPIC_API_KEY" {
			t.Fatalf("expected ANTHROPIC_API_KEY, got %q", config.SecretEnvVars[0].Name)
		}

		// Check base URL env var
		foundBaseURL := false
		for _, env := range config.EnvVars {
			if env.Name == "ANTHROPIC_BASE_URL" && env.Value == "https://custom-anthropic.example.com" {
				foundBaseURL = true
			}
		}
		if !foundBaseURL {
			t.Fatal("expected ANTHROPIC_BASE_URL env var")
		}
	})

	t.Run("without base URL", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "claude-sonnet-4-20250514"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, env := range config.EnvVars {
			if env.Name == "ANTHROPIC_BASE_URL" {
				t.Fatal("expected no ANTHROPIC_BASE_URL env var when BaseURL is empty")
			}
		}
	})
}

func TestGoogleProviderHandler_BuildConfig(t *testing.T) {
	handler := &GoogleProviderHandler{}

	t.Run("with project, location, API key and service account", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Google: &agentv1alpha1.GoogleProviderSpec{
					Project:  "my-project",
					Location: "us-central1",
					ServiceAccountKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "gcp-sa-secret"},
						Key:                  "key.json",
					},
				},
				APIKeySecretRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "google-api-secret"},
					Key:                  "api-key",
				},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "gemini-pro"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config.Model != "gemini-pro" {
			t.Fatalf("expected model gemini-pro, got %q", config.Model)
		}

		// Check GOOGLE_API_KEY
		foundAPIKey := false
		for _, env := range config.SecretEnvVars {
			if env.Name == "GOOGLE_API_KEY" {
				foundAPIKey = true
			}
		}
		if !foundAPIKey {
			t.Fatal("expected GOOGLE_API_KEY secret env var")
		}

		// Check service account key
		foundSA := false
		for _, env := range config.SecretEnvVars {
			if env.Name == "GOOGLE_APPLICATION_CREDENTIALS_JSON" {
				foundSA = true
			}
		}
		if !foundSA {
			t.Fatal("expected GOOGLE_APPLICATION_CREDENTIALS_JSON secret env var")
		}

		// Check project and location env vars
		foundProject := false
		foundLocation := false
		for _, env := range config.EnvVars {
			if env.Name == "GOOGLE_CLOUD_PROJECT" && env.Value == "my-project" {
				foundProject = true
			}
			if env.Name == "GOOGLE_CLOUD_REGION" && env.Value == "us-central1" {
				foundLocation = true
			}
		}
		if !foundProject {
			t.Fatal("expected GOOGLE_CLOUD_PROJECT env var")
		}
		if !foundLocation {
			t.Fatal("expected GOOGLE_CLOUD_REGION env var")
		}
	})

	t.Run("without optional fields", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Google: &agentv1alpha1.GoogleProviderSpec{},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "gemini-pro"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(config.EnvVars) != 0 {
			t.Fatalf("expected 0 env vars for empty Google spec, got %d", len(config.EnvVars))
		}
		if len(config.SecretEnvVars) != 0 {
			t.Fatalf("expected 0 secret env vars, got %d", len(config.SecretEnvVars))
		}
	})
}

func TestBedrockProviderHandler_BuildConfig(t *testing.T) {
	handler := &BedrockProviderHandler{}

	t.Run("with region", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Bedrock: &agentv1alpha1.BedrockProviderSpec{
					Region: "us-east-1",
				},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "anthropic.claude-v2"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config.Model != "anthropic.claude-v2" {
			t.Fatalf("expected model anthropic.claude-v2, got %q", config.Model)
		}

		// Check AWS_REGION env var
		foundRegion := false
		for _, env := range config.EnvVars {
			if env.Name == "AWS_REGION" && env.Value == "us-east-1" {
				foundRegion = true
			}
		}
		if !foundRegion {
			t.Fatal("expected AWS_REGION env var")
		}

		// Bedrock does not use API key secret
		if len(config.SecretEnvVars) != 0 {
			t.Fatalf("expected 0 secret env vars for bedrock, got %d", len(config.SecretEnvVars))
		}
	})

	t.Run("without region", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Bedrock: &agentv1alpha1.BedrockProviderSpec{},
			},
		}
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "anthropic.claude-v2"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(config.EnvVars) != 0 {
			t.Fatalf("expected 0 env vars for empty bedrock spec, got %d", len(config.EnvVars))
		}
	})

	t.Run("nil bedrock spec", func(t *testing.T) {
		provider := &agentv1alpha1.ModelProvider{
			Spec: agentv1alpha1.ModelProviderSpec{
				Bedrock: nil,
			},
		}
		// Since GetProviderType returns empty for nil specs, buildBaseConfig won't set bedrock
		// but the handler is still callable
		model := &agentv1alpha1.Model{
			Spec: agentv1alpha1.ModelSpec{Model: "test-model"},
		}

		config, err := handler.BuildConfig(provider, model)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(config.EnvVars) != 0 {
			t.Fatalf("expected 0 env vars, got %d", len(config.EnvVars))
		}
	})
}
