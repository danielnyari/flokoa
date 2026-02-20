package server

import (
	"context"

	"github.com/go-logr/logr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// mapKubernetesError converts Kubernetes API errors to appropriate gRPC status codes.
// Sensitive error details are logged server-side and not exposed to clients (fixes #104).
func mapKubernetesError(ctx context.Context, err error, resourceKind string) error {
	if err == nil {
		return nil
	}

	logger := logr.FromContextOrDiscard(ctx)

	switch {
	case apierrors.IsNotFound(err):
		return status.Errorf(codes.NotFound, "%s not found", resourceKind)
	case apierrors.IsAlreadyExists(err):
		return status.Errorf(codes.AlreadyExists, "%s already exists", resourceKind)
	case apierrors.IsConflict(err):
		return status.Errorf(codes.Aborted, "%s has been modified, please retry", resourceKind)
	case apierrors.IsInvalid(err):
		return status.Errorf(codes.InvalidArgument, "invalid %s: %s", resourceKind, err.Error())
	case apierrors.IsForbidden(err):
		logger.Error(err, "Forbidden access to Kubernetes resource", "resource", resourceKind)
		return status.Errorf(codes.PermissionDenied, "insufficient permissions for %s", resourceKind)
	case apierrors.IsUnauthorized(err):
		logger.Error(err, "Unauthorized access to Kubernetes resource", "resource", resourceKind)
		return status.Errorf(codes.Unauthenticated, "authentication required")
	case apierrors.IsServiceUnavailable(err):
		logger.Error(err, "Kubernetes service unavailable", "resource", resourceKind)
		return status.Errorf(codes.Unavailable, "service temporarily unavailable")
	case apierrors.IsTooManyRequests(err):
		logger.Error(err, "Kubernetes rate limited", "resource", resourceKind)
		return status.Errorf(codes.ResourceExhausted, "too many requests, please retry later")
	default:
		logger.Error(err, "Internal Kubernetes API error", "resource", resourceKind)
		return status.Errorf(codes.Internal, "internal server error")
	}
}
