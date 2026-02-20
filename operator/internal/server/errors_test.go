package server

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = Describe("mapKubernetesError", func() {
	gr := schema.GroupResource{Group: "agent.flokoa.ai", Resource: "models"}
	ctx := context.Background()

	It("should return nil for nil error", func() {
		Expect(mapKubernetesError(ctx, nil, "model")).To(Succeed())
	})

	It("should map NotFound to codes.NotFound", func() {
		err := apierrors.NewNotFound(gr, "test-model")
		grpcErr := mapKubernetesError(ctx, err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.NotFound))
	})

	It("should map AlreadyExists to codes.AlreadyExists", func() {
		err := apierrors.NewAlreadyExists(gr, "test-model")
		grpcErr := mapKubernetesError(ctx, err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.AlreadyExists))
	})

	It("should map Conflict to codes.Aborted", func() {
		err := apierrors.NewConflict(gr, "test-model", fmt.Errorf("conflict"))
		grpcErr := mapKubernetesError(ctx, err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.Aborted))
	})

	It("should map Forbidden to codes.PermissionDenied", func() {
		err := apierrors.NewForbidden(gr, "test-model", fmt.Errorf("not allowed"))
		grpcErr := mapKubernetesError(ctx, err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.PermissionDenied))
	})

	It("should map Unauthorized to codes.Unauthenticated", func() {
		err := apierrors.NewUnauthorized("bad credentials")
		grpcErr := mapKubernetesError(ctx, err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.Unauthenticated))
	})

	It("should map unknown errors to codes.Internal without leaking details", func() {
		err := fmt.Errorf("something unexpected happened with internal details")
		grpcErr := mapKubernetesError(ctx, err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.Internal))
		Expect(status.Convert(grpcErr).Message()).To(Equal("internal server error"))
	})

	It("should not leak error details for Forbidden errors", func() {
		err := apierrors.NewForbidden(gr, "test-model", fmt.Errorf("RBAC: access denied for serviceaccount system:foo"))
		grpcErr := mapKubernetesError(ctx, err, "model")
		msg := status.Convert(grpcErr).Message()
		Expect(msg).ToNot(ContainSubstring("RBAC"))
		Expect(msg).ToNot(ContainSubstring("serviceaccount"))
		Expect(msg).To(Equal("insufficient permissions for model"))
	})

	It("should not leak error details for Unauthorized errors", func() {
		err := apierrors.NewUnauthorized("token expired at 2026-01-01")
		grpcErr := mapKubernetesError(ctx, err, "model")
		msg := status.Convert(grpcErr).Message()
		Expect(msg).ToNot(ContainSubstring("token"))
		Expect(msg).To(Equal("authentication required"))
	})
})
