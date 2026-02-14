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

// AgentToolService implements the AgentToolService gRPC service.
type AgentToolService struct {
	pb.UnimplementedAgentToolServiceServer
	client client.Client
}

// NewAgentToolService creates a new AgentToolService.
func NewAgentToolService(c client.Client) *AgentToolService {
	return &AgentToolService{client: c}
}

// GetAgentTool retrieves an AgentTool by name and namespace.
func (s *AgentToolService) GetAgentTool(ctx context.Context, req *pb.GetAgentToolRequest) (*pb.AgentTool, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	var tool agentv1alpha1.AgentTool
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &tool); err != nil {
		return nil, mapKubernetesError(err, "agent tool")
	}

	return converter.AgentToolToProto(&tool), nil
}

// ListAgentTools lists AgentTools in a namespace or across all namespaces.
func (s *AgentToolService) ListAgentTools(ctx context.Context, req *pb.ListAgentToolsRequest) (*pb.AgentToolList, error) {
	var toolList agentv1alpha1.AgentToolList
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

	if err := s.client.List(ctx, &toolList, opts...); err != nil {
		return nil, mapKubernetesError(err, "agent tool")
	}

	return converter.AgentToolListToProto(&toolList), nil
}

// CreateAgentTool creates a new AgentTool.
func (s *AgentToolService) CreateAgentTool(ctx context.Context, req *pb.CreateAgentToolRequest) (*pb.AgentTool, error) {
	if req.AgentTool == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_tool is required")
	}
	if req.AgentTool.Metadata == nil || req.AgentTool.Metadata.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_tool metadata.name is required")
	}
	if req.AgentTool.Spec == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_tool spec is required")
	}

	tool := converter.AgentToolFromProto(req.AgentTool)
	if req.Namespace != "" {
		tool.Namespace = req.Namespace
	}
	if tool.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required (via request namespace or agent_tool metadata)")
	}

	if err := s.client.Create(ctx, tool); err != nil {
		return nil, mapKubernetesError(err, "agent tool")
	}

	return converter.AgentToolToProto(tool), nil
}

// UpdateAgentTool updates an existing AgentTool.
func (s *AgentToolService) UpdateAgentTool(ctx context.Context, req *pb.UpdateAgentToolRequest) (*pb.AgentTool, error) {
	if req.AgentTool == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_tool is required")
	}
	if req.AgentTool.Metadata == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_tool metadata is required")
	}
	if req.AgentTool.Metadata.Name == "" || req.AgentTool.Metadata.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_tool metadata.name and metadata.namespace are required")
	}

	var existing agentv1alpha1.AgentTool
	key := client.ObjectKey{
		Namespace: req.AgentTool.Metadata.Namespace,
		Name:      req.AgentTool.Metadata.Name,
	}
	if err := s.client.Get(ctx, key, &existing); err != nil {
		return nil, mapKubernetesError(err, "agent tool")
	}

	updated := converter.AgentToolFromProto(req.AgentTool)
	updated.ResourceVersion = existing.ResourceVersion

	if err := s.client.Update(ctx, updated); err != nil {
		return nil, mapKubernetesError(err, "agent tool")
	}

	return converter.AgentToolToProto(updated), nil
}

// DeleteAgentTool deletes an AgentTool.
func (s *AgentToolService) DeleteAgentTool(ctx context.Context, req *pb.DeleteAgentToolRequest) (*pb.DeleteAgentToolResponse, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	tool := &agentv1alpha1.AgentTool{}
	tool.Name = req.Name
	tool.Namespace = req.Namespace

	if err := s.client.Delete(ctx, tool); err != nil {
		return nil, mapKubernetesError(err, "agent tool")
	}

	return &pb.DeleteAgentToolResponse{}, nil
}

// WatchAgentTools watches for changes to AgentTools.
func (s *AgentToolService) WatchAgentTools(_ *pb.WatchAgentToolsRequest, _ pb.AgentToolService_WatchAgentToolsServer) error {
	return status.Error(codes.Unimplemented, "watch not yet implemented: requires informer-based streaming")
}

// UpdateAgentToolStatus updates only the status subresource.
func (s *AgentToolService) UpdateAgentToolStatus(ctx context.Context, req *pb.UpdateAgentToolStatusRequest) (*pb.AgentTool, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.Status == nil {
		return nil, status.Error(codes.InvalidArgument, "status is required")
	}

	var tool agentv1alpha1.AgentTool
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &tool); err != nil {
		return nil, mapKubernetesError(err, "agent tool")
	}

	tool.Status.ObservedGeneration = req.Status.ObservedGeneration
	if req.Status.Conditions != nil {
		tool.Status.Conditions = converter.ConditionsFromProto(req.Status.Conditions)
	}

	if err := s.client.Status().Update(ctx, &tool); err != nil {
		return nil, mapKubernetesError(err, "agent tool")
	}

	return converter.AgentToolToProto(&tool), nil
}
