// Package errors defines structured error types for the Flokoa operator.
// Controllers use these types to determine requeue behavior:
// - PermanentError: operation will never succeed, do not requeue
// - DependencyError: a referenced resource is not found or not ready, requeue after interval
// - TransientError: temporary failure, requeue with backoff
package errors

import (
	"errors"
	"fmt"
)

// PermanentError indicates the operation will never succeed and should not be retried.
// Examples: invalid spec, unsupported provider type, validation failure.
type PermanentError struct {
	Err error
}

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// NewPermanent wraps an existing error as a PermanentError.
func NewPermanent(err error) error {
	return &PermanentError{Err: err}
}

// NewPermanentf creates a new PermanentError with a formatted message.
func NewPermanentf(format string, args ...any) error {
	return &PermanentError{Err: fmt.Errorf(format, args...)}
}

// DependencyError indicates a referenced resource is not found or not ready.
// Controllers should requeue after a fixed interval.
type DependencyError struct {
	Err error
}

func (e *DependencyError) Error() string { return e.Err.Error() }
func (e *DependencyError) Unwrap() error { return e.Err }

// NewDependency wraps an existing error as a DependencyError.
func NewDependency(err error) error {
	return &DependencyError{Err: err}
}

// NewDependencyf creates a new DependencyError with a formatted message.
func NewDependencyf(format string, args ...any) error {
	return &DependencyError{Err: fmt.Errorf(format, args...)}
}

// TransientError indicates a temporary failure that should be retried with backoff.
// Examples: network errors, API server unavailable.
type TransientError struct {
	Err error
}

func (e *TransientError) Error() string { return e.Err.Error() }
func (e *TransientError) Unwrap() error { return e.Err }

// NewTransient wraps an existing error as a TransientError.
func NewTransient(err error) error {
	return &TransientError{Err: err}
}

// IsPermanent returns true if err (or any error in its chain) is a PermanentError.
func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}

// IsDependency returns true if err (or any error in its chain) is a DependencyError.
func IsDependency(err error) bool {
	var de *DependencyError
	return errors.As(err, &de)
}

// IsTransient returns true if err (or any error in its chain) is a TransientError.
func IsTransient(err error) bool {
	var te *TransientError
	return errors.As(err, &te)
}
