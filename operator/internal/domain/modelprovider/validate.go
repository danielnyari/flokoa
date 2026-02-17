package modelprovider

import (
	"fmt"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ValidateProvider checks that exactly one provider block is set and returns the provider type.
// This is a pure validation function with no I/O.
func ValidateProvider(mp *agentv1alpha1.ModelProvider) (agentv1alpha1.ProviderType, error) {
	count := 0
	var providerType agentv1alpha1.ProviderType

	if mp.Spec.OpenAI != nil {
		count++
		providerType = agentv1alpha1.ProviderTypeOpenAI
	}
	if mp.Spec.Anthropic != nil {
		count++
		providerType = agentv1alpha1.ProviderTypeAnthropic
	}
	if mp.Spec.Google != nil {
		count++
		providerType = agentv1alpha1.ProviderTypeGoogle
	}
	if mp.Spec.Bedrock != nil {
		count++
		providerType = agentv1alpha1.ProviderTypeBedrock
	}

	if count == 0 {
		return "", fmt.Errorf("exactly one of openai, anthropic, google, or bedrock must be specified")
	}
	if count > 1 {
		return "", fmt.Errorf("only one of openai, anthropic, google, or bedrock can be specified, found %d", count)
	}

	return providerType, nil
}
