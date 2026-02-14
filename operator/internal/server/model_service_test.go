package server

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

var _ = Describe("ModelService", func() {
	var svc *ModelService

	BeforeEach(func() {
		svc = NewModelService(k8sClient)
	})

	Context("GetModel", func() {
		It("should return InvalidArgument when namespace is empty", func() {
			_, err := svc.GetModel(ctx, &pb.GetModelRequest{Name: "test"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when name is empty", func() {
			_, err := svc.GetModel(ctx, &pb.GetModelRequest{Namespace: "default"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return NotFound for non-existent model", func() {
			_, err := svc.GetModel(ctx, &pb.GetModelRequest{
				Namespace: "default",
				Name:      "nonexistent",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})

		It("should return the model when it exists", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-get",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelSpec{
					Model: "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{
						Name: "openai-provider",
					},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, model)
			})

			result, err := svc.GetModel(ctx, &pb.GetModelRequest{
				Namespace: "default",
				Name:      "test-model-get",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Metadata.Name).To(Equal("test-model-get"))
			Expect(result.Spec.Model).To(Equal("gpt-4o"))
			Expect(result.Spec.ProviderRef.Name).To(Equal("openai-provider"))
		})
	})

	Context("ListModels", func() {
		It("should return an empty list when no models exist", func() {
			result, err := svc.ListModels(ctx, &pb.ListModelsRequest{
				Namespace: "nonexistent-ns",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).To(BeEmpty())
		})

		It("should list models with label selector", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-list",
					Namespace: "default",
					Labels:    map[string]string{"test": "model-list"},
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "claude-sonnet-4-20250514",
					ProviderRef: agentv1alpha1.ProviderRef{Name: "anthropic-provider"},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, model)
			})

			result, err := svc.ListModels(ctx, &pb.ListModelsRequest{
				Namespace: "default",
				Options: &pb.ListOptions{
					LabelSelector: "test=model-list",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).To(HaveLen(1))
			Expect(result.Items[0].Spec.Model).To(Equal("claude-sonnet-4-20250514"))
		})
	})

	Context("CreateModel", func() {
		It("should return InvalidArgument when model is nil", func() {
			_, err := svc.CreateModel(ctx, &pb.CreateModelRequest{})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when metadata.name is missing", func() {
			_, err := svc.CreateModel(ctx, &pb.CreateModelRequest{
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{},
					Spec:     &pb.ModelSpec{Model: "gpt-4o"},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when spec is missing", func() {
			_, err := svc.CreateModel(ctx, &pb.CreateModelRequest{
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{Name: "test"},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when spec.model is missing", func() {
			_, err := svc.CreateModel(ctx, &pb.CreateModelRequest{
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{Name: "test"},
					Spec:     &pb.ModelSpec{},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when provider_ref is missing", func() {
			_, err := svc.CreateModel(ctx, &pb.CreateModelRequest{
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{Name: "test"},
					Spec:     &pb.ModelSpec{Model: "gpt-4o"},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when namespace is missing", func() {
			_, err := svc.CreateModel(ctx, &pb.CreateModelRequest{
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{Name: "test"},
					Spec: &pb.ModelSpec{
						Model:       "gpt-4o",
						ProviderRef: &pb.ProviderRef{Name: "openai"},
					},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should create a model successfully", func() {
			result, err := svc.CreateModel(ctx, &pb.CreateModelRequest{
				Namespace: "default",
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{
						Name:      "test-model-create",
						Namespace: "default",
					},
					Spec: &pb.ModelSpec{
						Model:       "gpt-4o",
						ProviderRef: &pb.ProviderRef{Name: "openai-provider"},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Metadata.Name).To(Equal("test-model-create"))
			Expect(result.Metadata.Uid).NotTo(BeEmpty())

			DeferCleanup(func() {
				m := &agentv1alpha1.Model{}
				m.Name = "test-model-create"
				m.Namespace = "default"
				_ = k8sClient.Delete(ctx, m)
			})
		})

		It("should return AlreadyExists when model already exists", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-dup",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: "openai"},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, model)
			})

			_, err := svc.CreateModel(ctx, &pb.CreateModelRequest{
				Namespace: "default",
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{
						Name:      "test-model-dup",
						Namespace: "default",
					},
					Spec: &pb.ModelSpec{
						Model:       "gpt-4o",
						ProviderRef: &pb.ProviderRef{Name: "openai"},
					},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.AlreadyExists))
		})
	})

	Context("UpdateModel", func() {
		It("should return InvalidArgument when model is nil", func() {
			_, err := svc.UpdateModel(ctx, &pb.UpdateModelRequest{})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when metadata is nil", func() {
			_, err := svc.UpdateModel(ctx, &pb.UpdateModelRequest{
				Model: &pb.Model{},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return NotFound for non-existent model", func() {
			_, err := svc.UpdateModel(ctx, &pb.UpdateModelRequest{
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{
						Name:      "nonexistent",
						Namespace: "default",
					},
					Spec: &pb.ModelSpec{
						Model:       "gpt-4o",
						ProviderRef: &pb.ProviderRef{Name: "openai"},
					},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})

		It("should update a model successfully", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-update",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: "openai-provider"},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, model)
			})

			result, err := svc.UpdateModel(ctx, &pb.UpdateModelRequest{
				Model: &pb.Model{
					Metadata: &pb.ObjectMeta{
						Name:      "test-model-update",
						Namespace: "default",
					},
					Spec: &pb.ModelSpec{
						Model:       "gpt-4o-mini",
						ProviderRef: &pb.ProviderRef{Name: "openai-provider"},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Spec.Model).To(Equal("gpt-4o-mini"))
		})
	})

	Context("DeleteModel", func() {
		It("should return InvalidArgument when namespace is empty", func() {
			_, err := svc.DeleteModel(ctx, &pb.DeleteModelRequest{Name: "test"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when name is empty", func() {
			_, err := svc.DeleteModel(ctx, &pb.DeleteModelRequest{Namespace: "default"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should delete a model successfully", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-delete",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: "openai-provider"},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())

			result, err := svc.DeleteModel(ctx, &pb.DeleteModelRequest{
				Namespace: "default",
				Name:      "test-model-delete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("should return NotFound for non-existent model", func() {
			_, err := svc.DeleteModel(ctx, &pb.DeleteModelRequest{
				Namespace: "default",
				Name:      "nonexistent-delete",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})
	})

	Context("UpdateModelStatus", func() {
		It("should return InvalidArgument when namespace is empty", func() {
			_, err := svc.UpdateModelStatus(ctx, &pb.UpdateModelStatusRequest{Name: "test"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when status is nil", func() {
			_, err := svc.UpdateModelStatus(ctx, &pb.UpdateModelStatusRequest{
				Namespace: "default",
				Name:      "test",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return NotFound for non-existent model", func() {
			_, err := svc.UpdateModelStatus(ctx, &pb.UpdateModelStatusRequest{
				Namespace: "default",
				Name:      "nonexistent",
				Status:    &pb.ModelStatus{Ready: true},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})

		It("should update model status successfully", func() {
			model := &agentv1alpha1.Model{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-model-status",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelSpec{
					Model:       "gpt-4o",
					ProviderRef: agentv1alpha1.ProviderRef{Name: "openai-provider"},
				},
			}
			Expect(k8sClient.Create(ctx, model)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, model)
			})

			result, err := svc.UpdateModelStatus(ctx, &pb.UpdateModelStatusRequest{
				Namespace: "default",
				Name:      "test-model-status",
				Status: &pb.ModelStatus{
					Ready:              true,
					ObservedGeneration: 1,
					ResolvedProvider: &pb.ResolvedProviderInfo{
						Provider:  pb.ProviderType_PROVIDER_TYPE_OPENAI,
						Namespace: "default",
						Name:      "openai-provider",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status.Ready).To(BeTrue())
			Expect(result.Status.ObservedGeneration).To(Equal(int64(1)))
			Expect(result.Status.ResolvedProvider).NotTo(BeNil())
			Expect(result.Status.ResolvedProvider.Provider).To(Equal(pb.ProviderType_PROVIDER_TYPE_OPENAI))
		})
	})

	Context("WatchModels", func() {
		It("should return Unimplemented", func() {
			err := svc.WatchModels(&pb.WatchModelsRequest{}, nil)
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.Unimplemented))
		})
	})
})
