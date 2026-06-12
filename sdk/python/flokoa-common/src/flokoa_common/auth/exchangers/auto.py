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

"""Auto-dispatching credential exchanger."""

from __future__ import annotations

from flokoa_common.auth.auth_credential import AuthCredential, AuthCredentialTypes
from flokoa_common.auth.auth_schemes import AuthScheme

from .base import BaseAuthCredentialExchanger
from .oauth2 import OAuth2CredentialExchanger
from .service_account import ServiceAccountCredentialExchanger


class AutoAuthCredentialExchanger(BaseAuthCredentialExchanger):
    """Automatically selects the appropriate credential exchanger based on the auth scheme.

    Optionally, an override can be provided to use a specific exchanger for a
    given auth scheme.

    Exchanger instances are created once per credential type and reused, so
    exchangers that keep an in-memory token cache (e.g. the service account
    exchanger) retain it across calls.

    Example (common case):
    ```
    exchanger = AutoAuthCredentialExchanger()
    auth_credential = await exchanger.exchange_credential(
        auth_scheme=service_account_scheme,
        auth_credential=service_account_credential,
    )
    # Returns an oauth token in the form of a bearer token.
    ```

    Example (use CustomAuthExchanger for OAuth2):
    ```
    exchanger = AutoAuthCredentialExchanger(
        custom_exchangers={
            AuthCredentialTypes.OAUTH2: CustomAuthExchanger,
        }
    )
    ```

    Attributes:
      exchangers: A dictionary mapping auth credential type to credential
        exchanger class.
    """

    def __init__(
        self,
        custom_exchangers: dict[str, type[BaseAuthCredentialExchanger]] | None = None,
    ):
        """Initializes the AutoAuthCredentialExchanger.

        Args:
          custom_exchangers: Optional dictionary for adding or overriding auth
            exchangers. The key is the auth credential type, and the value is
            the credential exchanger class.
        """
        self.exchangers: dict[str, type[BaseAuthCredentialExchanger]] = {
            AuthCredentialTypes.OAUTH2: OAuth2CredentialExchanger,
            AuthCredentialTypes.OPEN_ID_CONNECT: OAuth2CredentialExchanger,
            AuthCredentialTypes.SERVICE_ACCOUNT: ServiceAccountCredentialExchanger,
        }

        if custom_exchangers:
            self.exchangers.update(custom_exchangers)

        self._instances: dict[type[BaseAuthCredentialExchanger], BaseAuthCredentialExchanger] = {}

    async def exchange_credential(
        self,
        auth_scheme: AuthScheme,
        auth_credential: AuthCredential | None = None,
    ) -> AuthCredential | None:
        """Automatically exchanges for the credential using the appropriate credential exchanger.

        Args:
            auth_scheme (AuthScheme): The security scheme.
            auth_credential (AuthCredential): Optional. The authentication
              credential.

        Returns: (AuthCredential)
            A new AuthCredential object containing the exchanged credential,
            or the original credential when no exchanger handles its type.
        """
        if not auth_credential:
            return None

        exchanger_class = self.exchangers.get(auth_credential.auth_type)

        if not exchanger_class:
            return auth_credential

        exchanger = self._instances.get(exchanger_class)
        if exchanger is None:
            exchanger = exchanger_class()
            self._instances[exchanger_class] = exchanger
        return await exchanger.exchange_credential(auth_scheme, auth_credential)
