"""Global test configuration for Flokoa tests."""

import pytest

from flokoa.cache import reset_global_cache


@pytest.fixture(autouse=True)
def reset_cache_before_each_test():
    """Reset the global cache before and after each test.

    This ensures tests don't interfere with each other through cached data.
    """
    reset_global_cache()
    yield
    reset_global_cache()
