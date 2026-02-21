"""TTL-based caching for Flokoa configuration loading.

This module provides caching infrastructure for model configs, tools, and agent cards
with configurable TTL (Time To Live) and file modification detection.

Environment Variables:
    FLOKOA_CACHE_TTL_SECONDS: TTL for cached configs in seconds (default: 60)
    FLOKOA_CACHE_ENABLED: Enable/disable caching (default: true)
"""

from __future__ import annotations

import os
import threading
import time
from dataclasses import dataclass, field
from typing import Any, TypeVar

T = TypeVar("T")

# Default TTL in seconds
DEFAULT_CACHE_TTL_SECONDS = 60


def get_cache_ttl() -> float:
    """Get the cache TTL from environment variable or use default."""
    try:
        return float(os.environ.get("FLOKOA_CACHE_TTL_SECONDS", DEFAULT_CACHE_TTL_SECONDS))
    except ValueError:
        return DEFAULT_CACHE_TTL_SECONDS


def is_cache_enabled() -> bool:
    """Check if caching is enabled via environment variable."""
    return os.environ.get("FLOKOA_CACHE_ENABLED", "true").lower() in ("true", "1", "yes")


@dataclass
class CachedEntry[T]:
    """A cached entry with TTL and file modification tracking."""

    value: T
    cached_at: float
    ttl_seconds: float
    file_paths: list[str] = field(default_factory=list)
    file_mtimes: dict[str, float] = field(default_factory=dict)

    def is_expired(self) -> bool:
        """Check if the cache entry has expired based on TTL."""
        return time.time() - self.cached_at > self.ttl_seconds

    def are_files_modified(self) -> bool:
        """Check if any tracked files have been modified since caching."""
        for path in self.file_paths:
            try:
                current_mtime = os.path.getmtime(path)
                cached_mtime = self.file_mtimes.get(path, 0)
                if current_mtime > cached_mtime:
                    return True
            except OSError:
                # File doesn't exist or can't be accessed - consider it modified
                return True
        return False

    def is_valid(self) -> bool:
        """Check if the cache entry is still valid (not expired and files not modified)."""
        return not self.is_expired() and not self.are_files_modified()


class ConfigCache:
    """Thread-safe cache for configuration data with TTL support.

    This cache supports:
    - TTL-based expiration
    - File modification detection
    - Thread-safe operations
    - Selective invalidation

    Example:
        cache = ConfigCache()

        # Cache model config
        config = load_model_config_from_file()
        cache.set("model_config", config, file_paths=["/etc/flokoa/model.json"])

        # Get cached config (returns None if expired or files modified)
        cached_config = cache.get("model_config")
    """

    def __init__(self, ttl_seconds: float | None = None):
        """Initialize the cache.

        Args:
            ttl_seconds: Override TTL for all entries. If None, uses FLOKOA_CACHE_TTL_SECONDS
                        environment variable or default (60 seconds).
        """
        self._cache: dict[str, CachedEntry[Any]] = {}
        self._lock = threading.RLock()
        self._ttl_seconds = ttl_seconds if ttl_seconds is not None else get_cache_ttl()

    @property
    def ttl_seconds(self) -> float:
        """Get the TTL in seconds."""
        return self._ttl_seconds

    def get(self, key: str) -> Any | None:
        """Get a cached value if it exists and is still valid.

        Args:
            key: The cache key.

        Returns:
            The cached value if valid, None otherwise.
        """
        if not is_cache_enabled():
            return None

        with self._lock:
            entry = self._cache.get(key)
            if entry is None:
                return None

            if not entry.is_valid():
                # Entry expired or files modified - remove it
                del self._cache[key]
                return None

            return entry.value

    def set(
        self,
        key: str,
        value: T,
        file_paths: list[str] | None = None,
        ttl_seconds: float | None = None,
    ) -> None:
        """Cache a value with optional file tracking.

        Args:
            key: The cache key.
            value: The value to cache.
            file_paths: Optional list of file paths to track for modifications.
            ttl_seconds: Optional TTL override for this specific entry.
        """
        if not is_cache_enabled():
            return

        file_paths = file_paths or []
        file_mtimes: dict[str, float] = {}

        # Record current modification times for tracked files
        for path in file_paths:
            try:
                file_mtimes[path] = os.path.getmtime(path)
            except OSError:
                # File doesn't exist - track it with 0 mtime so any creation triggers refresh
                file_mtimes[path] = 0

        entry = CachedEntry(
            value=value,
            cached_at=time.time(),
            ttl_seconds=ttl_seconds if ttl_seconds is not None else self._ttl_seconds,
            file_paths=file_paths,
            file_mtimes=file_mtimes,
        )

        with self._lock:
            self._cache[key] = entry

    def invalidate(self, key: str) -> bool:
        """Invalidate a specific cache entry.

        Args:
            key: The cache key to invalidate.

        Returns:
            True if the entry was found and removed, False otherwise.
        """
        with self._lock:
            if key in self._cache:
                del self._cache[key]
                return True
            return False

    def invalidate_all(self) -> None:
        """Invalidate all cache entries."""
        with self._lock:
            self._cache.clear()

    def is_valid(self, key: str) -> bool:
        """Check if a cache entry exists and is valid.

        Args:
            key: The cache key to check.

        Returns:
            True if the entry exists and is valid, False otherwise.
        """
        if not is_cache_enabled():
            return False

        with self._lock:
            entry = self._cache.get(key)
            return entry is not None and entry.is_valid()

    def get_entry(self, key: str) -> CachedEntry[Any] | None:
        """Get the full cache entry (for inspection/debugging).

        Args:
            key: The cache key.

        Returns:
            The CachedEntry if it exists (may be expired), None otherwise.
        """
        with self._lock:
            return self._cache.get(key)

    def keys(self) -> list[str]:
        """Get all cache keys."""
        with self._lock:
            return list(self._cache.keys())


# Global cache instance for shared use
_global_cache = ConfigCache()


def get_global_cache() -> ConfigCache:
    """Get the global cache instance.

    Returns:
        The global ConfigCache instance.
    """
    return _global_cache


def reset_global_cache() -> None:
    """Reset the global cache (useful for testing)."""
    global _global_cache
    _global_cache = ConfigCache()


# Cache keys
CACHE_KEY_MODEL_CONFIG = "model_config"
CACHE_KEY_TOOLS = "tools"
CACHE_KEY_AGENT_CARD = "agent_card"
