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

const testNamespace = "default"

// validAgentToolSpec creates a minimal valid AgentToolSpec for testing.
func validAgentToolSpec() agentv1alpha1.AgentToolSpec {
	return agentv1alpha1.AgentToolSpec{
		Type:        agentv1alpha1.AgentToolTypeMCP,
		Description: "A test tool for integration testing",
		URL:         "https://mcp.example.com/mcp",
	}
}

var _ = Describe("AgentToolService", func() {
	var svc *AgentToolService

	BeforeEach(func() {
		svc = NewAgentToolService(k8sClient)
	})

	Context("GetAgentTool", func() {
		It("should return InvalidArgument when namespace is empty", func() {
			_, err := svc.GetAgentTool(ctx, &pb.GetAgentToolRequest{Name: "test"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when name is empty", func() {
			_, err := svc.GetAgentTool(ctx, &pb.GetAgentToolRequest{Namespace: testNamespace})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return NotFound for non-existent tool", func() {
			_, err := svc.GetAgentTool(ctx, &pb.GetAgentToolRequest{
				Namespace: testNamespace,
				Name:      "nonexistent",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})

		It("should return the tool when it exists", func() {
			tool := &agentv1alpha1.AgentTool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tool-get",
					Namespace: testNamespace,
				},
				Spec: validAgentToolSpec(),
			}
			Expect(k8sClient.Create(ctx, tool)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tool)
			})

			result, err := svc.GetAgentTool(ctx, &pb.GetAgentToolRequest{
				Namespace: testNamespace,
				Name:      "test-tool-get",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Metadata.Name).To(Equal("test-tool-get"))
			Expect(result.Spec.Description).To(Equal("A test tool for integration testing"))
			Expect(result.Spec.Type).To(Equal(pb.AgentToolType_AGENT_TOOL_TYPE_MCP))
		})
	})

	Context("ListAgentTools", func() {
		It("should return an empty list when no tools exist", func() {
			result, err := svc.ListAgentTools(ctx, &pb.ListAgentToolsRequest{
				Namespace: "nonexistent-ns",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).To(BeEmpty())
		})

		It("should list tools with label selector", func() {
			spec := validAgentToolSpec()
			spec.Description = "List test tool"
			tool := &agentv1alpha1.AgentTool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tool-list",
					Namespace: testNamespace,
					Labels:    map[string]string{"test": "tool-list"},
				},
				Spec: spec,
			}
			Expect(k8sClient.Create(ctx, tool)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tool)
			})

			result, err := svc.ListAgentTools(ctx, &pb.ListAgentToolsRequest{
				Namespace: testNamespace,
				Options: &pb.ListOptions{
					LabelSelector: "test=tool-list",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).To(HaveLen(1))
			Expect(result.Items[0].Spec.Description).To(Equal("List test tool"))
		})
	})

	Context("CreateAgentTool", func() {
		It("should return InvalidArgument when agent_tool is nil", func() {
			_, err := svc.CreateAgentTool(ctx, &pb.CreateAgentToolRequest{})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when metadata.name is missing", func() {
			_, err := svc.CreateAgentTool(ctx, &pb.CreateAgentToolRequest{
				AgentTool: &pb.AgentTool{
					Metadata: &pb.ObjectMeta{},
					Spec: &pb.AgentToolSpec{
						Type:        pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI,
						Description: "Test",
					},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when spec is missing", func() {
			_, err := svc.CreateAgentTool(ctx, &pb.CreateAgentToolRequest{
				AgentTool: &pb.AgentTool{
					Metadata: &pb.ObjectMeta{Name: "test"},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should create a tool successfully", func() {
			result, err := svc.CreateAgentTool(ctx, &pb.CreateAgentToolRequest{
				Namespace: testNamespace,
				AgentTool: &pb.AgentTool{
					Metadata: &pb.ObjectMeta{
						Name:      "test-tool-create",
						Namespace: testNamespace,
					},
					Spec: &pb.AgentToolSpec{
						Type:        pb.AgentToolType_AGENT_TOOL_TYPE_MCP,
						Description: "Created tool",
						Url:         "https://mcp.example.com/mcp",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Metadata.Name).To(Equal("test-tool-create"))
			Expect(result.Metadata.Uid).NotTo(BeEmpty())

			DeferCleanup(func() {
				t := &agentv1alpha1.AgentTool{}
				t.Name = "test-tool-create"
				t.Namespace = testNamespace
				_ = k8sClient.Delete(ctx, t)
			})
		})

		It("should return AlreadyExists when tool already exists", func() {
			tool := &agentv1alpha1.AgentTool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tool-dup",
					Namespace: testNamespace,
				},
				Spec: validAgentToolSpec(),
			}
			Expect(k8sClient.Create(ctx, tool)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tool)
			})

			_, err := svc.CreateAgentTool(ctx, &pb.CreateAgentToolRequest{
				Namespace: testNamespace,
				AgentTool: &pb.AgentTool{
					Metadata: &pb.ObjectMeta{
						Name:      "test-tool-dup",
						Namespace: testNamespace,
					},
					Spec: &pb.AgentToolSpec{
						Type:        pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI,
						Description: "Another tool",
					},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.AlreadyExists))
		})
	})

	Context("UpdateAgentTool", func() {
		It("should return InvalidArgument when agent_tool is nil", func() {
			_, err := svc.UpdateAgentTool(ctx, &pb.UpdateAgentToolRequest{})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should update a tool successfully", func() {
			tool := &agentv1alpha1.AgentTool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tool-update",
					Namespace: testNamespace,
				},
				Spec: validAgentToolSpec(),
			}
			Expect(k8sClient.Create(ctx, tool)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tool)
			})

			result, err := svc.UpdateAgentTool(ctx, &pb.UpdateAgentToolRequest{
				AgentTool: &pb.AgentTool{
					Metadata: &pb.ObjectMeta{
						Name:      "test-tool-update",
						Namespace: testNamespace,
					},
					Spec: &pb.AgentToolSpec{
						Type:        pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI,
						Description: "Updated description",
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Spec.Description).To(Equal("Updated description"))
		})

		It("should return NotFound for non-existent tool", func() {
			_, err := svc.UpdateAgentTool(ctx, &pb.UpdateAgentToolRequest{
				AgentTool: &pb.AgentTool{
					Metadata: &pb.ObjectMeta{
						Name:      "nonexistent",
						Namespace: testNamespace,
					},
					Spec: &pb.AgentToolSpec{
						Type:        pb.AgentToolType_AGENT_TOOL_TYPE_OPENAPI,
						Description: "Test",
					},
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})
	})

	Context("DeleteAgentTool", func() {
		It("should return InvalidArgument when namespace is empty", func() {
			_, err := svc.DeleteAgentTool(ctx, &pb.DeleteAgentToolRequest{Name: "test"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should delete a tool successfully", func() {
			tool := &agentv1alpha1.AgentTool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tool-delete",
					Namespace: testNamespace,
				},
				Spec: validAgentToolSpec(),
			}
			Expect(k8sClient.Create(ctx, tool)).To(Succeed())

			result, err := svc.DeleteAgentTool(ctx, &pb.DeleteAgentToolRequest{
				Namespace: testNamespace,
				Name:      "test-tool-delete",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("should return NotFound for non-existent tool", func() {
			_, err := svc.DeleteAgentTool(ctx, &pb.DeleteAgentToolRequest{
				Namespace: testNamespace,
				Name:      "nonexistent-tool-delete",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})
	})

	Context("UpdateAgentToolStatus", func() {
		It("should return InvalidArgument when status is nil", func() {
			_, err := svc.UpdateAgentToolStatus(ctx, &pb.UpdateAgentToolStatusRequest{
				Namespace: testNamespace,
				Name:      "test",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should update tool status successfully", func() {
			tool := &agentv1alpha1.AgentTool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tool-status",
					Namespace: testNamespace,
				},
				Spec: validAgentToolSpec(),
			}
			Expect(k8sClient.Create(ctx, tool)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, tool)
			})

			result, err := svc.UpdateAgentToolStatus(ctx, &pb.UpdateAgentToolStatusRequest{
				Namespace: testNamespace,
				Name:      "test-tool-status",
				Status: &pb.AgentToolStatus{
					ObservedGeneration: 1,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Status.ObservedGeneration).To(Equal(int64(1)))
		})
	})

	Context("WatchAgentTools", func() {
		It("should return Unimplemented", func() {
			err := svc.WatchAgentTools(&pb.WatchAgentToolsRequest{}, nil)
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.Unimplemented))
		})
	})
})
