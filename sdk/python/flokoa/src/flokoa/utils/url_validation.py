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

"""Backwards-compatible re-export of the SSRF URL validator.

The implementation moved to :mod:`flokoa_common.utils.url_validation` so that
packages that must not depend on ``flokoa`` (``flokoa-common``,
``flokoa-openapi``) can use it. Import from ``flokoa_common`` in new code.
"""

from flokoa_common.utils.url_validation import SSRFError, validate_url

__all__ = ["SSRFError", "validate_url"]
