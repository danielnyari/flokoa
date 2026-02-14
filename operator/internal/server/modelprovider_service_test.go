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

var _ = Describe("ModelProviderService", func() {
	var svc *ModelProviderService

	BeforeEach(func() {
		svc = NewModelProviderService(k8sClient)
	})

	Context("GetModelProvider", func() {
		It("should return InvalidArgument when namespace is empty", func() {
			_, err := svc.GetModelProvider(ctx, &pb.GetModelProviderRequest{Name: "test"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when name is empty", func() {
			_, err := svc.GetModelProvider(ctx, &pb.GetModelProviderRequest{Namespace: "default"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return NotFound for non-existent provider", func() {
			_, err := svc.GetModelProvider(ctx, &pb.GetModelProviderRequest{
				Namespace: "default",
				Name:      "nonexistent",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})

		It("should return the provider when it exists", func() {
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-get",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{
						BaseURL: "https://api.openai.com/v1",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider)
			})

			result, err := svc.GetModelProvider(ctx, &pb.GetModelProviderRequest{
				Namespace: "default",
				Name:      "test-provider-get",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Metadata.Name).To(Equal("test-provider-get"))
			Expect(result.Spec.Openai).NotTo(BeNil())
			Expect(result.Spec.Openai.BaseUrl).To(Equal("https://api.openai.com/v1"))
		})
	})

	Context("ListModelProviders", func() {
		It("should return an empty list when no providers exist", func() {
			result, err := svc.ListModelProviders(ctx, &pb.ListModelProvidersRequest{
				Namespace: "nonexistent-ns",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).To(BeEmpty())
		})

		It("should list providers with label selector", func() {
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-list",
					Namespace: "default",
					Labels:    map[string]string{"test": "provider-list"},
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					Anthropic: &agentv1alpha1.AnthropicProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider)
			})

			result, err := svc.ListModelProviders(ctx, &pb.ListModelProvidersRequest{
				Namespace: "default",
				Options: &pb.ListOptions{
					LabelSelector: "test=provider-list",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).To(HaveLen(1))
			Expect(result.Items[0].Spec.Anthropic).NotTo(BeNil())
		})
	})

	Context("CreateModelProvider", func() {
		It("should return InvalidArgument when model_provider is nil", func() {
			_, err := svc.CreateModelProvider(ctx, &pb.CreateModelProviderRequest{})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when metadata.name is missing", func() {
			_, err := svc.CreateModelProvider(ctx, &pb.CreateModelProviderRequest{
				ModelProvider: &pb.ModelProvider{
					Metadata: &pb.ObjectMeta{},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should create a provider successfully", func() {
			result, err := svc.CreateModelProvider(ctx, &pb.CreateModelProviderRequest{
				Namespace: "default",
				ModelProvider: &pb.ModelProvider{
					Metadata: &pb.ObjectMeta{
						Name:      "test-provider-create",
						Namespace: "default",
					},
					Spec: &pb.ModelProviderSpec{
						Openai: &pb.OpenAIProviderSpec{
							BaseUrl: "https://api.openai.com/v1",
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Metadata.Name).To(Equal("test-provider-create"))
			Expect(result.Metadata.Uid).NotTo(BeEmpty())

			DeferCleanup(func() {
				p := &agentv1alpha1.ModelProvider{}
				p.Name = "test-provider-create"
				p.Namespace = "default"
				_ = k8sClient.Delete(ctx, p)
			})
		})

		It("should return AlreadyExists when provider already exists", func() {
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-dup",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider)
			})

			_, err := svc.CreateModelProvider(ctx, &pb.CreateModelProviderRequest{
				Namespace: "default",
				ModelProvider: &pb.ModelProvider{
					Metadata: &pb.ObjectMeta{
						Name:      "test-provider-dup",
						Namespace: "default",
					},
					Spec: &pb.ModelProviderSpec{
						Openai: &pb.OpenAIProviderSpec{},
					},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.AlreadyExists))
		})
	})

	Context("UpdateModelProvider", func() {
		It("should return InvalidArgument when model_provider is nil", func() {
			_, err := svc.UpdateModelProvider(ctx, &pb.UpdateModelProviderRequest{})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should update a provider successfully", func() {
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-update",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{
						BaseURL: "https://api.openai.com/v1",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider)
			})

			result, err := svc.UpdateModelProvider(ctx, &pb.UpdateModelProviderRequest{
				ModelProvider: &pb.ModelProvider{
					Metadata: &pb.ObjectMeta{
						Name:      "test-provider-update",
						Namespace: "default",
					},
					Spec: &pb.ModelProviderSpec{
						Openai: &pb.OpenAIProviderSpec{
							BaseUrl: "https://custom-openai.example.com/v1",
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Spec.Openai.BaseUrl).To(Equal("https://custom-openai.example.com/v1"))
		})
	})

	Context("DeleteModelProvider", func() {
		It("should return InvalidArgument when namespace is empty", func() {
			_, err := svc.DeleteModelProvider(ctx, &pb.DeleteModelProviderRequest{Name: "test"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should delete a provider successfully", func() {
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-delete",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			result, err := svc.DeleteModelProvider(ctx, &pb.DeleteModelProviderRequest{
				Namespace: "default",
				Name:      "test-provider-delete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("should return NotFound for non-existent provider", func() {
			_, err := svc.DeleteModelProvider(ctx, &pb.DeleteModelProviderRequest{
				Namespace: "default",
				Name:      "nonexistent-provider-delete",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})
	})

	Context("UpdateModelProviderStatus", func() {
		It("should return InvalidArgument when status is nil", func() {
			_, err := svc.UpdateModelProviderStatus(ctx, &pb.UpdateModelProviderStatusRequest{
				Namespace: "default",
				Name:      "test",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should update provider status successfully", func() {
			provider := &agentv1alpha1.ModelProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-provider-status",
					Namespace: "default",
				},
				Spec: agentv1alpha1.ModelProviderSpec{
					OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, provider)
			})

			result, err := svc.UpdateModelProviderStatus(ctx, &pb.UpdateModelProviderStatusRequest{
				Namespace: "default",
				Name:      "test-provider-status",
				Status: &pb.ModelProviderStatus{
					Ready:              true,
					ObservedGeneration: 1,
					Provider:           pb.ProviderType_PROVIDER_TYPE_OPENAI,
					SecretHash:         "abc123",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status.Ready).To(BeTrue())
			Expect(result.Status.Provider).To(Equal(pb.ProviderType_PROVIDER_TYPE_OPENAI))
			Expect(result.Status.SecretHash).To(Equal("abc123"))
		})
	})

	Context("WatchModelProviders", func() {
		It("should return Unimplemented", func() {
			err := svc.WatchModelProviders(&pb.WatchModelProvidersRequest{}, nil)
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.Unimplemented))
		})
	})
})
