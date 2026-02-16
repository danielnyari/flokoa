"""Tests for the integrations registry (get_executor_cls)."""

import pytest

from flokoa.integrations import get_executor_cls
from flokoa_types import IntegrationType


class TestGetExecutorCls:
    def test_pydantic_ai_executor_loaded(self):
        cls = get_executor_cls(IntegrationType.PYDANTIC_AI)
        from flokoa.integrations.pydantic_ai.agent_executor import PydanticAIAgentExecutor

        assert cls is PydanticAIAgentExecutor

    def test_google_adk_executor_loaded(self):
        cls = get_executor_cls(IntegrationType.GOOGLE_ADK)
        from flokoa.integrations.google_adk.agent_executor import GoogleADKAgentExecutor

        assert cls is GoogleADKAgentExecutor

    def test_unknown_integration_raises(self):
        # Create a fake IntegrationType value that won't be loaded
        # We can test the error path by using a string that's not registered
        from unittest.mock import patch

        with patch.dict("flokoa.integrations._loaded", clear=True):
            with pytest.raises(ImportError, match="is not installed"):
                get_executor_cls(IntegrationType.PYDANTIC_AI)
