"""Tests for cached loading functions in utils module."""

import json
import time

import pytest

import flokoa.utils as utils_module

MINIMAL_OPENAPI_SPEC = {
    "openapi": "3.0.0",
    "info": {"title": "Test API", "version": "1.0.0"},
    "servers": [{"url": "https://api.example.com"}],
    "paths": {
        "/test": {
            "get": {
                "operationId": "testEndpoint",
                "summary": "Test tool",
                "responses": {"200": {"description": "OK"}},
            }
        }
    },
}
from flokoa.cache import (
    CACHE_KEY_MODEL_CONFIG,
    ConfigCache,
    reset_global_cache,
)
from flokoa.utils import (
    invalidate_config_cache,
    is_config_cache_valid,
    load_model_config,
    load_tools,
)


@pytest.fixture(autouse=True)
def reset_cache():
    """Reset the global cache before each test."""
    reset_global_cache()
    yield
    reset_global_cache()


class TestLoadToolsCaching:
    """Tests for cached tool loading."""

    def test_load_tools_uses_cache(self, tmp_path, monkeypatch):
        tools_dir = tmp_path / "flokoa" / "tools"
        tools_dir.mkdir(parents=True)
        monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_dir) + "/")

        tool_data = {
            "name": "test_tool",
            "spec": {
                "type": "openapi",
                "description": "Test tool",
                "openApi": {
                    "openApiSchema": {"value": MINIMAL_OPENAPI_SPEC},
                    "url": "https://api.example.com",
                },
            },
        }
        tool_file = tools_dir / "test_tool.json"
        tool_file.write_text(json.dumps(tool_data))

        cache = ConfigCache()

        # First load - should read from file
        tools1 = load_tools(cache=cache)
        assert len(tools1) == 1

        # Modify the file content (but not mtime yet for test purposes)
        # Second load - should use cache
        tools2 = load_tools(cache=cache)
        assert tools2 == tools1

    def test_load_tools_reloads_when_file_modified(self, tmp_path, monkeypatch):
        tools_dir = tmp_path / "flokoa" / "tools"
        tools_dir.mkdir(parents=True)
        monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_dir) + "/")

        tool_data = {
            "name": "test_tool",
            "spec": {
                "type": "openapi",
                "description": "Test tool",
                "openApi": {
                    "openApiSchema": {"value": MINIMAL_OPENAPI_SPEC},
                    "url": "https://api.example.com",
                },
            },
        }
        tool_file = tools_dir / "test_tool.json"
        tool_file.write_text(json.dumps(tool_data))

        cache = ConfigCache()

        # First load
        tools1 = load_tools(cache=cache)
        assert tools1[0].name == "test_tool"

        # Wait and modify file
        time.sleep(0.01)
        tool_data["name"] = "updated_tool"
        tool_file.write_text(json.dumps(tool_data))

        # Load again - should reload
        tools2 = load_tools(cache=cache)
        assert tools2[0].name == "updated_tool"

    def test_load_tools_without_cache(self, tmp_path, monkeypatch):
        tools_dir = tmp_path / "flokoa" / "tools"
        tools_dir.mkdir(parents=True)
        monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_dir) + "/")

        tool_data = {
            "name": "test_tool",
            "spec": {
                "type": "openapi",
                "description": "Test tool",
                "openApi": {
                    "openApiSchema": {"value": MINIMAL_OPENAPI_SPEC},
                    "url": "https://api.example.com",
                },
            },
        }
        tool_file = tools_dir / "test_tool.json"
        tool_file.write_text(json.dumps(tool_data))

        # Load without caching
        tools = load_tools(use_cache=False)
        assert len(tools) == 1

    def test_load_tools_returns_empty_when_no_tools_dir(self, tmp_path, monkeypatch):
        nonexistent_dir = tmp_path / "nonexistent" / "tools"
        monkeypatch.setattr(utils_module, "TOOLS_PATH", str(nonexistent_dir) + "/")

        tools = load_tools()
        assert tools == []

    def test_load_tools_detects_new_file(self, tmp_path, monkeypatch):
        tools_dir = tmp_path / "flokoa" / "tools"
        tools_dir.mkdir(parents=True)
        monkeypatch.setattr(utils_module, "TOOLS_PATH", str(tools_dir) + "/")

        cache = ConfigCache()

        # First load - empty
        tools1 = load_tools(cache=cache)
        assert len(tools1) == 0

        # Add a new tool file
        time.sleep(0.01)  # Ensure mtime changes
        tool_data = {
            "name": "new_tool",
            "spec": {
                "type": "openapi",
                "description": "New tool",
                "openApi": {
                    "openApiSchema": {"value": MINIMAL_OPENAPI_SPEC},
                    "url": "https://api.example.com",
                },
            },
        }
        tool_file = tools_dir / "new_tool.json"
        tool_file.write_text(json.dumps(tool_data))

        # Load again - should detect new file via directory mtime
        tools2 = load_tools(cache=cache)
        assert len(tools2) == 1
        assert tools2[0].name == "new_tool"


class TestLoadModelConfigCaching:
    """Tests for cached model config loading."""

    def test_load_model_config_uses_cache(self, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        config_data = {
            "provider": {"type": "openai"},
            "model": "gpt-4o",
        }
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        cache = ConfigCache()

        # First load
        config1 = load_model_config(cache=cache)
        assert config1 is not None
        assert config1.model == "gpt-4o"

        # Second load - should use cache
        config2 = load_model_config(cache=cache)
        assert config2 is config1

    def test_load_model_config_reloads_when_file_modified(self, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        config_data = {
            "provider": {"type": "openai"},
            "model": "gpt-4o",
        }
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        cache = ConfigCache()

        # First load
        config1 = load_model_config(cache=cache)
        assert config1.model == "gpt-4o"

        # Wait and modify file
        time.sleep(0.01)
        config_data["model"] = "gpt-4o-mini"
        model_config_path.write_text(json.dumps(config_data))

        # Load again - should reload
        config2 = load_model_config(cache=cache)
        assert config2.model == "gpt-4o-mini"

    def test_load_model_config_without_cache(self, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        config_data = {
            "provider": {"type": "openai"},
            "model": "gpt-4o",
        }
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        # Load without caching
        config = load_model_config(use_cache=False)
        assert config is not None
        assert config.model == "gpt-4o"

    def test_load_model_config_returns_none_when_file_missing(self, tmp_path, monkeypatch):
        missing_path = tmp_path / "missing.json"
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(missing_path))

        config = load_model_config()
        assert config is None

    def test_load_model_config_cache_expires(self, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        config_data = {
            "provider": {"type": "openai"},
            "model": "gpt-4o",
        }
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        cache = ConfigCache(ttl_seconds=0.01)

        # First load
        config1 = load_model_config(cache=cache)
        assert config1.model == "gpt-4o"

        # Wait for TTL to expire
        time.sleep(0.02)

        # Modify file
        config_data["model"] = "gpt-4o-mini"
        model_config_path.write_text(json.dumps(config_data))

        # Load again - should reload due to TTL expiry
        config2 = load_model_config(cache=cache)
        assert config2.model == "gpt-4o-mini"


class TestInvalidateConfigCache:
    """Tests for cache invalidation functions."""

    def test_invalidate_config_cache(self, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        config_data = {
            "provider": {"type": "openai"},
            "model": "gpt-4o",
        }
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        cache = ConfigCache()

        # Load to populate cache
        load_model_config(cache=cache)
        assert cache.is_valid(CACHE_KEY_MODEL_CONFIG)

        # Invalidate
        invalidate_config_cache(cache=cache)
        assert not cache.is_valid(CACHE_KEY_MODEL_CONFIG)

    def test_is_config_cache_valid(self, tmp_path, monkeypatch):
        model_config_path = tmp_path / "model.json"
        config_data = {
            "provider": {"type": "openai"},
            "model": "gpt-4o",
        }
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        cache = ConfigCache()

        # Before loading - not valid
        assert not is_config_cache_valid(CACHE_KEY_MODEL_CONFIG, cache=cache)

        # After loading - valid
        load_model_config(cache=cache)
        assert is_config_cache_valid(CACHE_KEY_MODEL_CONFIG, cache=cache)


class TestCachingWithEnvVars:
    """Tests for caching behavior with environment variables."""

    def test_caching_disabled_via_env(self, tmp_path, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_ENABLED", "false")

        model_config_path = tmp_path / "model.json"
        config_data = {
            "provider": {"type": "openai"},
            "model": "gpt-4o",
        }
        model_config_path.write_text(json.dumps(config_data))
        monkeypatch.setattr(utils_module, "MODEL_CONFIG_PATH", str(model_config_path))

        cache = ConfigCache()

        # Load - should still work but not cache
        config1 = load_model_config(cache=cache)
        assert config1.model == "gpt-4o"

        # Cache should not be populated when disabled
        assert cache.get(CACHE_KEY_MODEL_CONFIG) is None

    def test_custom_ttl_via_env(self, tmp_path, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_TTL_SECONDS", "5")

        # Create a new cache to pick up the env var
        cache = ConfigCache()
        assert cache.ttl_seconds == 5.0
