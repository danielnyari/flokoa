package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// BedrockProviderHandler handles AWS Bedrock model configuration.
type BedrockProviderHandler struct{}

func (h *BedrockProviderHandler) BuildConfig(provider *agentv1alpha1.ModelProvider, model *agentv1alpha1.Model) (*ResolvedModelConfig, error) {
	config := buildBaseConfig(provider, model)

	// Add Bedrock-specific provider configuration
	if provider.Spec.Bedrock != nil {
		bedrockSpec := provider.Spec.Bedrock

		if bedrockSpec.Region != "" {
			config.Config["region"] = bedrockSpec.Region
			config.EnvVars = append(config.EnvVars, corev1.EnvVar{
				Name:  "AWS_REGION",
				Value: bedrockSpec.Region,
			})
		}

		if bedrockSpec.InferenceProfileARN != "" {
			config.Config["inferenceProfileARN"] = bedrockSpec.InferenceProfileARN
		}
	}

	// Add Bedrock-specific parameters from Model
	if model.Spec.Parameters != nil && model.Spec.Parameters.Bedrock != nil {
		bedrockParams := model.Spec.Parameters.Bedrock
		params := make(map[string]any)

		if bedrockParams.GuardrailConfig != nil {
			guardrail := make(map[string]any)
			if bedrockParams.GuardrailConfig.GuardrailIdentifier != "" {
				guardrail["guardrailIdentifier"] = bedrockParams.GuardrailConfig.GuardrailIdentifier
			}
			if bedrockParams.GuardrailConfig.GuardrailVersion != "" {
				guardrail["guardrailVersion"] = bedrockParams.GuardrailConfig.GuardrailVersion
			}
			if bedrockParams.GuardrailConfig.Trace != "" {
				guardrail["trace"] = bedrockParams.GuardrailConfig.Trace
			}
			params["guardrailConfig"] = guardrail
		}

		if bedrockParams.PerformanceConfiguration != nil {
			perf := make(map[string]any)
			if bedrockParams.PerformanceConfiguration.Latency != "" {
				perf["latency"] = bedrockParams.PerformanceConfiguration.Latency
			}
			params["performanceConfiguration"] = perf
		}

		if len(bedrockParams.RequestMetadata) > 0 {
			params["requestMetadata"] = bedrockParams.RequestMetadata
		}

		if len(bedrockParams.AdditionalModelResponseFieldsPaths) > 0 {
			params["additionalModelResponseFieldsPaths"] = bedrockParams.AdditionalModelResponseFieldsPaths
		}

		if len(bedrockParams.PromptVariables) > 0 {
			params["promptVariables"] = bedrockParams.PromptVariables
		}

		if bedrockParams.CacheToolDefinitions != nil {
			params["cacheToolDefinitions"] = *bedrockParams.CacheToolDefinitions
		}

		if bedrockParams.CacheInstructions != nil {
			params["cacheInstructions"] = *bedrockParams.CacheInstructions
		}

		if bedrockParams.CacheMessages != nil {
			params["cacheMessages"] = *bedrockParams.CacheMessages
		}

		if bedrockParams.ServiceTier != nil && bedrockParams.ServiceTier.Type != "" {
			params["serviceTier"] = map[string]string{
				"type": bedrockParams.ServiceTier.Type,
			}
		}

		if len(params) > 0 {
			if config.Parameters == nil {
				config.Parameters = &ModelParametersConfig{}
			}
			config.Parameters.Bedrock = params
		}
	}

	// Note: AWS credentials are typically handled via:
	// 1. IAM roles for service accounts (IRSA) - recommended for EKS
	// 2. Instance profiles - for EC2-based clusters
	// 3. Environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY) - less secure
	//
	// The SDK will automatically pick up credentials from the environment.
	// Users should configure their cluster to provide credentials via IRSA or similar.

	return config, nil
}
