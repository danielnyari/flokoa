package model

import (
	"fmt"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// ValidateProviderParams validates that provider-specific parameters match the provider type.
// This is a pure validation function with no I/O.
func ValidateProviderParams(params *agentv1alpha1.ModelParameters, providerType agentv1alpha1.ProviderType) error {
	if params == nil {
		return nil
	}

	count := 0
	var setProvider agentv1alpha1.ProviderType

	if params.OpenAI != nil {
		count++
		setProvider = agentv1alpha1.ProviderTypeOpenAI
	}
	if params.Anthropic != nil {
		count++
		setProvider = agentv1alpha1.ProviderTypeAnthropic
	}
	if params.Google != nil {
		count++
		setProvider = agentv1alpha1.ProviderTypeGoogle
	}
	if params.Bedrock != nil {
		count++
		setProvider = agentv1alpha1.ProviderTypeBedrock
	}

	if count > 1 {
		return fmt.Errorf("only one provider-specific parameters block can be set, found %d", count)
	}

	if count == 1 && setProvider != providerType {
		return fmt.Errorf("provider-specific parameters (%s) do not match the ModelProvider type (%s)", setProvider, providerType)
	}

	return nil
}
