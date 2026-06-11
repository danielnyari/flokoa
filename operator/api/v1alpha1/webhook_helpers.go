/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"
	"net/url"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// validateExactlyOneOf checks that exactly one of the given values is "set"
// (non-nil for pointers, non-empty for strings). Returns nil if valid.
func validateExactlyOneOf(fldPath *field.Path, names []string, set []bool) *field.Error {
	count := 0
	for _, s := range set {
		if s {
			count++
		}
	}
	joined := strings.Join(names, ", ")
	if count == 0 {
		return field.Required(fldPath, fmt.Sprintf("exactly one of %s must be specified", joined))
	}
	if count > 1 {
		return field.Forbidden(fldPath, fmt.Sprintf("only one of %s can be specified", joined))
	}
	return nil
}

// validateHTTPURL checks that the given string is a valid HTTP or HTTPS URL.
// Returns a field.Error if the URL is empty, uses a disallowed scheme, or is malformed.
func validateHTTPURL(fldPath *field.Path, rawURL string) *field.Error {
	if rawURL == "" {
		return nil // Empty URLs are handled by required-field checks elsewhere
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return field.Invalid(fldPath, rawURL, fmt.Sprintf("invalid URL: %v", err))
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return field.Invalid(fldPath, rawURL, "URL scheme must be http or https")
	}
	if parsed.Host == "" {
		return field.Invalid(fldPath, rawURL, "URL must contain a valid host")
	}
	return nil
}

// aggregateErrors converts a field.ErrorList into an API status error, or nil if empty.
func aggregateErrors(kind, name string, allErrs field.ErrorList) error {
	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: GroupVersion.Group, Kind: kind},
		name,
		allErrs,
	)
}
