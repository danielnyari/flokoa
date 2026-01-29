package controller

import (
	corev1 "k8s.io/api/core/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// BedrockProviderHandler handles AWS Bedrock model configuration.
type BedrockProviderHandler struct{}

func (h *BedrockProviderHandler) BuildConfig(model *agentv1alpha1.Model, modelConfig *agentv1alpha1.ModelConfig) (*ModelProviderConfig, error) {
	config := buildBaseConfig(model, modelConfig)

	// Add Bedrock-specific configuration
	if model.Spec.Bedrock != nil {
		bedrockSpec := model.Spec.Bedrock

		config.Config["region"] = bedrockSpec.Region
		config.EnvVars = append(config.EnvVars, corev1.EnvVar{
			Name:  "AWS_REGION",
			Value: bedrockSpec.Region,
		})

		if bedrockSpec.InferenceProfileARN != "" {
			config.Config["inferenceProfileARN"] = bedrockSpec.InferenceProfileARN
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
