package server

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// mapKubernetesError converts Kubernetes API errors to appropriate gRPC status codes.
func mapKubernetesError(err error, resourceKind string) error {
	if err == nil {
		return nil
	}

	switch {
	case apierrors.IsNotFound(err):
		return status.Errorf(codes.NotFound, "%s not found", resourceKind)
	case apierrors.IsAlreadyExists(err):
		return status.Errorf(codes.AlreadyExists, "%s already exists", resourceKind)
	case apierrors.IsConflict(err):
		return status.Errorf(codes.Aborted, "conflict: %s has been modified, please retry", resourceKind)
	case apierrors.IsInvalid(err):
		return status.Errorf(codes.InvalidArgument, "invalid %s: %s", resourceKind, err.Error())
	case apierrors.IsForbidden(err):
		return status.Errorf(codes.PermissionDenied, "forbidden: %s", err.Error())
	case apierrors.IsUnauthorized(err):
		return status.Errorf(codes.Unauthenticated, "unauthorized: %s", err.Error())
	case apierrors.IsServiceUnavailable(err):
		return status.Errorf(codes.Unavailable, "service unavailable: %s", err.Error())
	case apierrors.IsTooManyRequests(err):
		return status.Errorf(codes.ResourceExhausted, "too many requests: %s", err.Error())
	default:
		return status.Errorf(codes.Internal, "internal error: %s", err.Error())
	}
}
