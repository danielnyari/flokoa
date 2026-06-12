# Copyright 2026 Google LLC
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

"""Base class for authentication credential exchangers."""

from __future__ import annotations

import abc

from flokoa_common.auth.auth_credential import AuthCredential
from flokoa_common.auth.auth_schemes import AuthScheme


class AuthCredentialMissingError(Exception):
    """Exception raised when required authentication credentials are missing."""

    def __init__(self, message: str):
        super().__init__(message)
        self.message = message


class BaseAuthCredentialExchanger(abc.ABC):
    """Base class for authentication credential exchangers.

    Exchangers are async: implementations that need to talk to a token
    endpoint must do so without blocking the event loop (the exchange runs
    inside async tool calls).
    """

    @abc.abstractmethod
    async def exchange_credential(
        self,
        auth_scheme: AuthScheme,
        auth_credential: AuthCredential | None = None,
    ) -> AuthCredential | None:
        """Exchanges the provided authentication credential for a usable token/credential.

        Args:
            auth_scheme: The security scheme.
            auth_credential: The authentication credential.

        Returns:
            An updated AuthCredential object containing the fetched credential.
            For simple schemes like API key, it may return the original credential
            if no exchange is needed.

        Raises:
            NotImplementedError: If the method is not implemented by a subclass.
        """
        raise NotImplementedError("Subclasses must implement exchange_credential.")
