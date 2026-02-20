# Copyright 2026 Flokoa Contributors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""URL validation utilities to prevent SSRF (Server-Side Request Forgery) attacks."""

from __future__ import annotations

import ipaddress
import logging
import socket
from urllib.parse import urlparse

logger = logging.getLogger("flokoa." + __name__)

# IP ranges that should be blocked to prevent SSRF
_BLOCKED_IP_NETWORKS = [
    ipaddress.ip_network("0.0.0.0/8"),  # "This" network
    ipaddress.ip_network("10.0.0.0/8"),  # Private
    ipaddress.ip_network("100.64.0.0/10"),  # Shared address space (CGN)
    ipaddress.ip_network("127.0.0.0/8"),  # Loopback
    ipaddress.ip_network("169.254.0.0/16"),  # Link-local / cloud metadata
    ipaddress.ip_network("172.16.0.0/12"),  # Private
    ipaddress.ip_network("192.0.0.0/24"),  # IETF protocol assignments
    ipaddress.ip_network("192.168.0.0/16"),  # Private
    ipaddress.ip_network("::1/128"),  # IPv6 loopback
    ipaddress.ip_network("fc00::/7"),  # IPv6 unique local
    ipaddress.ip_network("fe80::/10"),  # IPv6 link-local
    ipaddress.ip_network("::ffff:127.0.0.0/104"),  # IPv4-mapped loopback
    ipaddress.ip_network("::ffff:10.0.0.0/104"),  # IPv4-mapped private
    ipaddress.ip_network("::ffff:172.16.0.0/108"),  # IPv4-mapped private
    ipaddress.ip_network("::ffff:192.168.0.0/112"),  # IPv4-mapped private
    ipaddress.ip_network("::ffff:169.254.0.0/112"),  # IPv4-mapped link-local
]


class SSRFError(ValueError):
    """Raised when a URL fails SSRF validation."""


def _is_ip_blocked(ip_str: str) -> bool:
    """Check if an IP address falls within a blocked range."""
    try:
        addr = ipaddress.ip_address(ip_str)
    except ValueError:
        return False
    return any(addr in network for network in _BLOCKED_IP_NETWORKS)


def validate_url(url: str, *, allow_internal: bool = False) -> str:
    """Validate a URL to prevent SSRF attacks.

    Checks that the URL:
    - Uses HTTPS or HTTP scheme
    - Does not resolve to a private/internal IP address
    - Does not target common metadata endpoints

    Args:
        url: The URL to validate.
        allow_internal: If True, skip IP-range checks (for trusted/local use).

    Returns:
        The validated URL.

    Raises:
        SSRFError: If the URL fails validation.
    """
    if not url:
        raise SSRFError("URL must not be empty")

    parsed = urlparse(url)

    if parsed.scheme not in ("http", "https"):
        raise SSRFError(f"URL scheme must be http or https, got: {parsed.scheme!r}")

    hostname = parsed.hostname
    if not hostname:
        raise SSRFError("URL must contain a valid hostname")

    if allow_internal:
        return url

    # Block direct IP addresses in private ranges
    if _is_ip_blocked(hostname):
        raise SSRFError(f"URL targets a blocked IP range: {hostname}")

    # Resolve hostname and check all resulting IPs
    try:
        addr_infos = socket.getaddrinfo(hostname, None, socket.AF_UNSPEC, socket.SOCK_STREAM)
    except socket.gaierror:
        # DNS resolution failed; allow the request to proceed and let the
        # HTTP client report the real error.
        logger.debug("DNS resolution failed for %s; skipping IP validation", hostname)
        return url

    for _family, _, _, _, sockaddr in addr_infos:
        ip_str = sockaddr[0]
        if _is_ip_blocked(ip_str):
            raise SSRFError(f"URL hostname {hostname!r} resolves to blocked IP {ip_str}")

    return url
