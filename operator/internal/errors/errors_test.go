package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestPermanentError(t *testing.T) {
	inner := fmt.Errorf("invalid spec")
	err := NewPermanent(inner)

	if !IsPermanent(err) {
		t.Fatal("expected IsPermanent to return true")
	}
	if IsDependency(err) {
		t.Fatal("expected IsDependency to return false")
	}
	if IsTransient(err) {
		t.Fatal("expected IsTransient to return false")
	}
	if err.Error() != "invalid spec" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
	if !errors.Is(err, inner) {
		t.Fatal("expected Unwrap to return inner error")
	}
}

func TestPermanentErrorf(t *testing.T) {
	err := NewPermanentf("unsupported provider: %s", "foo")
	if !IsPermanent(err) {
		t.Fatal("expected IsPermanent to return true")
	}
	if err.Error() != "unsupported provider: foo" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
}

func TestDependencyError(t *testing.T) {
	inner := fmt.Errorf("model not ready")
	err := NewDependency(inner)

	if IsPermanent(err) {
		t.Fatal("expected IsPermanent to return false")
	}
	if !IsDependency(err) {
		t.Fatal("expected IsDependency to return true")
	}
	if IsTransient(err) {
		t.Fatal("expected IsTransient to return false")
	}
	if err.Error() != "model not ready" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
}

func TestDependencyErrorf(t *testing.T) {
	err := NewDependencyf("model %s/%s is not ready", "ns", "m1")
	if !IsDependency(err) {
		t.Fatal("expected IsDependency to return true")
	}
	if err.Error() != "model ns/m1 is not ready" {
		t.Fatalf("unexpected message: %s", err.Error())
	}
}

func TestTransientError(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	err := NewTransient(inner)

	if IsPermanent(err) {
		t.Fatal("expected IsPermanent to return false")
	}
	if IsDependency(err) {
		t.Fatal("expected IsDependency to return false")
	}
	if !IsTransient(err) {
		t.Fatal("expected IsTransient to return true")
	}
}

func TestWrappedErrors(t *testing.T) {
	inner := NewDependencyf("model not ready")
	wrapped := fmt.Errorf("reconcile failed: %w", inner)

	if !IsDependency(wrapped) {
		t.Fatal("expected IsDependency to detect wrapped DependencyError")
	}
}

func TestNilErrors(t *testing.T) {
	if IsPermanent(nil) {
		t.Fatal("expected IsPermanent(nil) to return false")
	}
	if IsDependency(nil) {
		t.Fatal("expected IsDependency(nil) to return false")
	}
	if IsTransient(nil) {
		t.Fatal("expected IsTransient(nil) to return false")
	}
}
