package server

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/converter"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// AgentService implements the read-only AgentService gRPC service.
type AgentService struct {
	pb.UnimplementedAgentServiceServer
	client client.Client
}

// NewAgentService creates a new AgentService.
func NewAgentService(c client.Client) *AgentService {
	return &AgentService{client: c}
}

// GetAgent retrieves an Agent by name and namespace.
func (s *AgentService) GetAgent(ctx context.Context, req *pb.GetAgentRequest) (*pb.Agent, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	var agent agentv1alpha1.Agent
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &agent); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent")
	}

	return converter.AgentToProto(&agent), nil
}

// ListAgents lists Agents in a namespace or across all namespaces.
func (s *AgentService) ListAgents(ctx context.Context, req *pb.ListAgentsRequest) (*pb.AgentList, error) {
	var agentList agentv1alpha1.AgentList
	opts := []client.ListOption{}

	if req.Namespace != "" {
		opts = append(opts, client.InNamespace(req.Namespace))
	}

	if req.Options != nil {
		if req.Options.LabelSelector != "" {
			selector, err := metav1.ParseToLabelSelector(req.Options.LabelSelector)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid label selector: %s", err.Error())
			}
			labelSelector, err := metav1.LabelSelectorAsSelector(selector)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "invalid label selector: %s", err.Error())
			}
			opts = append(opts, client.MatchingLabelsSelector{Selector: labelSelector})
		}

		if req.Options.Limit > 0 {
			opts = append(opts, client.Limit(req.Options.Limit))
		}

		if req.Options.Continue != "" {
			opts = append(opts, client.Continue(req.Options.Continue))
		}
	}

	if err := s.client.List(ctx, &agentList, opts...); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent")
	}

	return converter.AgentListToProto(&agentList), nil
}

// WatchAgents watches for changes to Agents.
func (s *AgentService) WatchAgents(_ *pb.WatchAgentsRequest, _ pb.AgentService_WatchAgentsServer) error {
	return status.Error(codes.Unimplemented, "watch not yet implemented: requires informer-based streaming")
}
