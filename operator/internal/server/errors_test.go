package server

import (
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

	It("should return nil for nil error", func() {
		Expect(mapKubernetesError(nil, "model")).To(Succeed())
	})

	It("should map NotFound to codes.NotFound", func() {
		err := apierrors.NewNotFound(gr, "test-model")
		grpcErr := mapKubernetesError(err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.NotFound))
	})

	It("should map AlreadyExists to codes.AlreadyExists", func() {
		err := apierrors.NewAlreadyExists(gr, "test-model")
		grpcErr := mapKubernetesError(err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.AlreadyExists))
	})

	It("should map Conflict to codes.Aborted", func() {
		err := apierrors.NewConflict(gr, "test-model", fmt.Errorf("conflict"))
		grpcErr := mapKubernetesError(err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.Aborted))
	})

	It("should map Forbidden to codes.PermissionDenied", func() {
		err := apierrors.NewForbidden(gr, "test-model", fmt.Errorf("not allowed"))
		grpcErr := mapKubernetesError(err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.PermissionDenied))
	})

	It("should map Unauthorized to codes.Unauthenticated", func() {
		err := apierrors.NewUnauthorized("bad credentials")
		grpcErr := mapKubernetesError(err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.Unauthenticated))
	})

	It("should map unknown errors to codes.Internal", func() {
		err := fmt.Errorf("something unexpected happened")
		grpcErr := mapKubernetesError(err, "model")
		Expect(status.Code(grpcErr)).To(Equal(codes.Internal))
	})
})
