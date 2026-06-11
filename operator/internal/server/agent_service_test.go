package server

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// validAgentSpec creates a minimal valid AgentSpec for testing.
func validAgentSpec() agentv1alpha1.AgentSpec {
	return agentv1alpha1.AgentSpec{
		Card: agentv1alpha1.AgentCardOverride{
			Name:        "Test Agent",
			Description: "A test agent",
			Version:     "1.0.0",
			Skills: []agentv1alpha1.AgentSkill{
				{
					ID:          "test-skill",
					Name:        "Test Skill",
					Description: "A test skill",
					Tags:        []string{"test"},
				},
			},
		},
		Spec: &agentv1alpha1.AgentSpecFragment{
			Model: "openai:gpt-5-mini",
		},
	}
}

var _ = Describe("AgentService", func() {
	var svc *AgentService

	BeforeEach(func() {
		svc = NewAgentService(k8sClient)
	})

	Context("GetAgent", func() {
		It("should return InvalidArgument when namespace is empty", func() {
			_, err := svc.GetAgent(ctx, &pb.GetAgentRequest{Name: "test"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return InvalidArgument when name is empty", func() {
			_, err := svc.GetAgent(ctx, &pb.GetAgentRequest{Namespace: "default"})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should return NotFound for non-existent agent", func() {
			_, err := svc.GetAgent(ctx, &pb.GetAgentRequest{
				Namespace: "default",
				Name:      "nonexistent",
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.NotFound))
		})

		It("should return the agent when it exists", func() {
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent-get",
					Namespace: "default",
				},
				Spec: validAgentSpec(),
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, agent)
			})

			result, err := svc.GetAgent(ctx, &pb.GetAgentRequest{
				Namespace: "default",
				Name:      "test-agent-get",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Metadata.Name).To(Equal("test-agent-get"))
			Expect(result.Metadata.Namespace).To(Equal("default"))
		})
	})

	Context("ListAgents", func() {
		It("should return an empty list when no agents exist in namespace", func() {
			result, err := svc.ListAgents(ctx, &pb.ListAgentsRequest{
				Namespace: "nonexistent-ns",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Items).To(BeEmpty())
		})

		It("should list agents in a namespace", func() {
			spec := validAgentSpec()
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-agent-list",
					Namespace: "default",
					Labels:    map[string]string{"test": "list"},
				},
				Spec: spec,
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, agent)
			})

			result, err := svc.ListAgents(ctx, &pb.ListAgentsRequest{
				Namespace: "default",
				Options: &pb.ListOptions{
					LabelSelector: "test=list",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).To(HaveLen(1))
			Expect(result.Items[0].Metadata.Name).To(Equal("test-agent-list"))
		})

		It("should return InvalidArgument for invalid label selector", func() {
			_, err := svc.ListAgents(ctx, &pb.ListAgentsRequest{
				Namespace: "default",
				Options: &pb.ListOptions{
					LabelSelector: "!!!invalid!!!",
				},
			})
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.InvalidArgument))
		})

		It("should support pagination with limit", func() {
			for i := 0; i < 3; i++ {
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test-agent-page-%c", rune('a'+i)),
						Namespace: "default",
						Labels:    map[string]string{"test": "pagination"},
					},
					Spec: validAgentSpec(),
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())
				DeferCleanup(func() {
					_ = k8sClient.Delete(ctx, agent)
				})
			}

			result, err := svc.ListAgents(ctx, &pb.ListAgentsRequest{
				Namespace: "default",
				Options: &pb.ListOptions{
					LabelSelector: "test=pagination",
					Limit:         2,
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Items).To(HaveLen(2))
			Expect(result.Metadata.Continue).NotTo(BeEmpty())
		})
	})

	Context("WatchAgents", func() {
		It("should return Unimplemented", func() {
			err := svc.WatchAgents(&pb.WatchAgentsRequest{}, nil)
			Expect(err).To(HaveOccurred())
			Expect(status.Code(err)).To(Equal(codes.Unimplemented))
		})
	})
})
