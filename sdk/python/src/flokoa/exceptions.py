class CancelNotSupportedError(Exception):
    """Raised when an attempt is made to cancel an agent that does not support cancellation."""

    pass
