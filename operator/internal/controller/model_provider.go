package controller

// Re-exports from domain/model for backwards compatibility with existing tests
// and callers. These will be removed once all logic moves to the app layer.

import (
	modeldomain "github.com/danielnyari/flokoa/internal/domain/model"
)

// Type aliases for backwards compatibility.
type ResolvedModelConfig = modeldomain.ResolvedModelConfig
type ProviderConfig = modeldomain.ProviderConfig
type ProviderHandler = modeldomain.ProviderHandler

// Function re-exports for backwards compatibility.
var GetProviderHandler = modeldomain.GetProviderHandler
var buildBaseConfig = modeldomain.BuildBaseConfig
var addAPIKeyEnvVar = modeldomain.AddAPIKeyEnvVar
