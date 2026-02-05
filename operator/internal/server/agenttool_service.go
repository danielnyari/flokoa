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
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	var tool agentv1alpha1.AgentTool
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &tool); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "agent tool not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
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
				return nil, status.Error(codes.InvalidArgument, "invalid label selector")
			}
			labelSelector, err := metav1.LabelSelectorAsSelector(selector)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, "invalid label selector")
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.AgentToolListToProto(&toolList), nil
}

// CreateAgentTool creates a new AgentTool.
func (s *AgentToolService) CreateAgentTool(ctx context.Context, req *pb.CreateAgentToolRequest) (*pb.AgentTool, error) {
	if req.AgentTool == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_tool is required")
	}

	tool := converter.AgentToolFromProto(req.AgentTool)
	if req.Namespace != "" {
		tool.Namespace = req.Namespace
	}

	if err := s.client.Create(ctx, tool); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.AgentToolToProto(tool), nil
}

// UpdateAgentTool updates an existing AgentTool.
func (s *AgentToolService) UpdateAgentTool(ctx context.Context, req *pb.UpdateAgentToolRequest) (*pb.AgentTool, error) {
	if req.AgentTool == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_tool is required")
	}

	// Get existing tool
	var existing agentv1alpha1.AgentTool
	key := client.ObjectKey{
		Namespace: req.AgentTool.Metadata.Namespace,
		Name:      req.AgentTool.Metadata.Name,
	}
	if err := s.client.Get(ctx, key, &existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "agent tool not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Apply update
	updated := converter.AgentToolFromProto(req.AgentTool)
	updated.ResourceVersion = existing.ResourceVersion

	if err := s.client.Update(ctx, updated); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.AgentToolToProto(updated), nil
}

// DeleteAgentTool deletes an AgentTool.
func (s *AgentToolService) DeleteAgentTool(ctx context.Context, req *pb.DeleteAgentToolRequest) (*pb.DeleteAgentToolResponse, error) {
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	tool := &agentv1alpha1.AgentTool{}
	tool.Name = req.Name
	tool.Namespace = req.Namespace

	if err := s.client.Delete(ctx, tool); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "agent tool not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.DeleteAgentToolResponse{}, nil
}

// WatchAgentTools watches for changes to AgentTools.
func (s *AgentToolService) WatchAgentTools(req *pb.WatchAgentToolsRequest, stream pb.AgentToolService_WatchAgentToolsServer) error {
	return status.Error(codes.Unimplemented, "watch not yet implemented")
}

// UpdateAgentToolStatus updates only the status subresource.
func (s *AgentToolService) UpdateAgentToolStatus(ctx context.Context, req *pb.UpdateAgentToolStatusRequest) (*pb.AgentTool, error) {
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	var tool agentv1alpha1.AgentTool
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &tool); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "agent tool not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Update status fields
	if req.Status != nil {
		tool.Status.ObservedGeneration = req.Status.ObservedGeneration
	}

	if err := s.client.Status().Update(ctx, &tool); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.AgentToolToProto(&tool), nil
}
