"""Tests for the caching module."""

import os
import time

from flokoa.cache import (
    CACHE_KEY_MODEL_CONFIG,
    CACHE_KEY_TOOLS,
    CachedEntry,
    ConfigCache,
    get_cache_ttl,
    get_global_cache,
    is_cache_enabled,
    reset_global_cache,
)


class TestCachedEntry:
    """Tests for CachedEntry dataclass."""

    def test_is_expired_returns_false_when_fresh(self):
        entry = CachedEntry(
            value="test",
            cached_at=time.time(),
            ttl_seconds=60.0,
        )
        assert entry.is_expired() is False

    def test_is_expired_returns_true_when_expired(self):
        entry = CachedEntry(
            value="test",
            cached_at=time.time() - 120,  # 2 minutes ago
            ttl_seconds=60.0,
        )
        assert entry.is_expired() is True

    def test_are_files_modified_returns_false_when_no_files(self):
        entry = CachedEntry(
            value="test",
            cached_at=time.time(),
            ttl_seconds=60.0,
        )
        assert entry.are_files_modified() is False

    def test_are_files_modified_returns_true_when_file_modified(self, tmp_path):
        test_file = tmp_path / "test.json"
        test_file.write_text("{}")

        # Cache with current mtime
        entry = CachedEntry(
            value="test",
            cached_at=time.time(),
            ttl_seconds=60.0,
            file_paths=[str(test_file)],
            file_mtimes={str(test_file): os.path.getmtime(str(test_file))},
        )

        assert entry.are_files_modified() is False

        # Modify file
        time.sleep(0.01)  # Ensure mtime changes
        test_file.write_text('{"updated": true}')

        assert entry.are_files_modified() is True

    def test_are_files_modified_returns_true_when_file_missing(self, tmp_path):
        missing_file = tmp_path / "missing.json"
        entry = CachedEntry(
            value="test",
            cached_at=time.time(),
            ttl_seconds=60.0,
            file_paths=[str(missing_file)],
            file_mtimes={str(missing_file): 0},
        )
        assert entry.are_files_modified() is True

    def test_is_valid_returns_true_when_fresh_and_files_unchanged(self, tmp_path):
        test_file = tmp_path / "test.json"
        test_file.write_text("{}")

        entry = CachedEntry(
            value="test",
            cached_at=time.time(),
            ttl_seconds=60.0,
            file_paths=[str(test_file)],
            file_mtimes={str(test_file): os.path.getmtime(str(test_file))},
        )

        assert entry.is_valid() is True

    def test_is_valid_returns_false_when_expired(self):
        entry = CachedEntry(
            value="test",
            cached_at=time.time() - 120,
            ttl_seconds=60.0,
        )
        assert entry.is_valid() is False

    def test_is_valid_returns_false_when_files_modified(self, tmp_path):
        test_file = tmp_path / "test.json"
        test_file.write_text("{}")

        entry = CachedEntry(
            value="test",
            cached_at=time.time(),
            ttl_seconds=60.0,
            file_paths=[str(test_file)],
            file_mtimes={str(test_file): os.path.getmtime(str(test_file)) - 10},
        )

        assert entry.is_valid() is False


class TestConfigCache:
    """Tests for ConfigCache class."""

    def test_init_with_default_ttl(self):
        cache = ConfigCache()
        assert cache.ttl_seconds == 60.0

    def test_init_with_custom_ttl(self):
        cache = ConfigCache(ttl_seconds=120.0)
        assert cache.ttl_seconds == 120.0

    def test_set_and_get(self):
        cache = ConfigCache()
        cache.set("key", "value")

        result = cache.get("key")

        assert result == "value"

    def test_get_returns_none_for_missing_key(self):
        cache = ConfigCache()
        assert cache.get("missing") is None

    def test_get_returns_none_when_expired(self):
        cache = ConfigCache(ttl_seconds=0.01)
        cache.set("key", "value")

        time.sleep(0.02)

        assert cache.get("key") is None

    def test_get_returns_none_when_file_modified(self, tmp_path):
        cache = ConfigCache()
        test_file = tmp_path / "test.json"
        test_file.write_text("{}")

        cache.set("key", "value", file_paths=[str(test_file)])

        # Modify file
        time.sleep(0.01)
        test_file.write_text('{"updated": true}')

        assert cache.get("key") is None

    def test_set_with_file_tracking(self, tmp_path):
        cache = ConfigCache()
        test_file = tmp_path / "test.json"
        test_file.write_text("{}")

        cache.set("key", "value", file_paths=[str(test_file)])

        entry = cache.get_entry("key")
        assert entry is not None
        assert str(test_file) in entry.file_paths
        assert str(test_file) in entry.file_mtimes

    def test_set_with_custom_ttl(self):
        cache = ConfigCache(ttl_seconds=60.0)
        cache.set("key", "value", ttl_seconds=5.0)

        entry = cache.get_entry("key")
        assert entry is not None
        assert entry.ttl_seconds == 5.0

    def test_invalidate_removes_entry(self):
        cache = ConfigCache()
        cache.set("key", "value")

        result = cache.invalidate("key")

        assert result is True
        assert cache.get("key") is None

    def test_invalidate_returns_false_for_missing_key(self):
        cache = ConfigCache()
        assert cache.invalidate("missing") is False

    def test_invalidate_all_clears_cache(self):
        cache = ConfigCache()
        cache.set("key1", "value1")
        cache.set("key2", "value2")

        cache.invalidate_all()

        assert cache.get("key1") is None
        assert cache.get("key2") is None

    def test_is_valid_returns_true_for_valid_entry(self):
        cache = ConfigCache()
        cache.set("key", "value")

        assert cache.is_valid("key") is True

    def test_is_valid_returns_false_for_missing_entry(self):
        cache = ConfigCache()
        assert cache.is_valid("missing") is False

    def test_is_valid_returns_false_for_expired_entry(self):
        cache = ConfigCache(ttl_seconds=0.01)
        cache.set("key", "value")

        time.sleep(0.02)

        assert cache.is_valid("key") is False

    def test_keys_returns_all_keys(self):
        cache = ConfigCache()
        cache.set("key1", "value1")
        cache.set("key2", "value2")

        keys = cache.keys()

        assert "key1" in keys
        assert "key2" in keys
        assert len(keys) == 2

    def test_caching_disabled_via_env(self, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_ENABLED", "false")

        cache = ConfigCache()
        cache.set("key", "value")

        # When disabled, get always returns None
        assert cache.get("key") is None

    def test_caching_enabled_via_env(self, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_ENABLED", "true")

        cache = ConfigCache()
        cache.set("key", "value")

        assert cache.get("key") == "value"


class TestCacheFunctions:
    """Tests for module-level cache functions."""

    def test_get_cache_ttl_returns_default(self, monkeypatch):
        monkeypatch.delenv("FLOKOA_CACHE_TTL_SECONDS", raising=False)
        assert get_cache_ttl() == 60.0

    def test_get_cache_ttl_from_env(self, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_TTL_SECONDS", "120")
        assert get_cache_ttl() == 120.0

    def test_get_cache_ttl_invalid_env_returns_default(self, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_TTL_SECONDS", "invalid")
        assert get_cache_ttl() == 60.0

    def test_is_cache_enabled_default(self, monkeypatch):
        monkeypatch.delenv("FLOKOA_CACHE_ENABLED", raising=False)
        assert is_cache_enabled() is True

    def test_is_cache_enabled_false(self, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_ENABLED", "false")
        assert is_cache_enabled() is False

    def test_is_cache_enabled_true(self, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_ENABLED", "true")
        assert is_cache_enabled() is True

    def test_is_cache_enabled_yes(self, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_ENABLED", "yes")
        assert is_cache_enabled() is True

    def test_is_cache_enabled_one(self, monkeypatch):
        monkeypatch.setenv("FLOKOA_CACHE_ENABLED", "1")
        assert is_cache_enabled() is True


class TestGlobalCache:
    """Tests for global cache instance."""

    def test_get_global_cache_returns_instance(self):
        cache = get_global_cache()
        assert isinstance(cache, ConfigCache)

    def test_get_global_cache_returns_same_instance(self):
        cache1 = get_global_cache()
        cache2 = get_global_cache()
        assert cache1 is cache2

    def test_reset_global_cache_creates_new_instance(self):
        cache1 = get_global_cache()
        cache1.set("key", "value")

        reset_global_cache()
        cache2 = get_global_cache()

        assert cache1 is not cache2
        assert cache2.get("key") is None


class TestCacheKeys:
    """Tests for predefined cache keys."""

    def test_cache_key_constants(self):
        assert CACHE_KEY_MODEL_CONFIG == "model_config"
        assert CACHE_KEY_TOOLS == "tools"
