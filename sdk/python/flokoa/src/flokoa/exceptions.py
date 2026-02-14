class CancelNotSupportedError(Exception):
    """Raised when an attempt is made to cancel an agent that does not support cancellation."""

    pass


class ModelNotConfiguredError(Exception):
    """Raised when a model is not properly configured for an agent. Includes cases where no model configuration is provided via the Flokoa CRD and the agent itself lacks a model."""

    pass


class ProviderNotConfiguredError(Exception):
    """Raised when a provider is not configured but is required for an operation."""

    pass
