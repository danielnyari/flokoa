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

// AgentTriggerService implements the AgentTriggerService gRPC service.
type AgentTriggerService struct {
	pb.UnimplementedAgentTriggerServiceServer
	client client.Client
}

// NewAgentTriggerService creates a new AgentTriggerService.
func NewAgentTriggerService(c client.Client) *AgentTriggerService {
	return &AgentTriggerService{client: c}
}

// GetAgentTrigger retrieves an AgentTrigger by name and namespace.
func (s *AgentTriggerService) GetAgentTrigger(ctx context.Context, req *pb.GetAgentTriggerRequest) (*pb.AgentTrigger, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	var trigger agentv1alpha1.AgentTrigger
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &trigger); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent trigger")
	}

	return converter.AgentTriggerToProto(&trigger), nil
}

// ListAgentTriggers lists AgentTriggers in a namespace or across all namespaces.
func (s *AgentTriggerService) ListAgentTriggers(ctx context.Context, req *pb.ListAgentTriggersRequest) (*pb.AgentTriggerList, error) {
	var triggerList agentv1alpha1.AgentTriggerList
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

	if err := s.client.List(ctx, &triggerList, opts...); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent trigger")
	}

	return converter.AgentTriggerListToProto(&triggerList), nil
}

// CreateAgentTrigger creates a new AgentTrigger.
func (s *AgentTriggerService) CreateAgentTrigger(ctx context.Context, req *pb.CreateAgentTriggerRequest) (*pb.AgentTrigger, error) {
	if req.AgentTrigger == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_trigger is required")
	}
	if req.AgentTrigger.Metadata == nil || req.AgentTrigger.Metadata.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_trigger metadata.name is required")
	}
	if req.AgentTrigger.Spec == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_trigger spec is required")
	}

	trigger := converter.AgentTriggerFromProto(req.AgentTrigger)
	if req.Namespace != "" {
		trigger.Namespace = req.Namespace
	}
	if trigger.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required (via request namespace or agent_trigger metadata)")
	}

	if err := s.client.Create(ctx, trigger); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent trigger")
	}

	return converter.AgentTriggerToProto(trigger), nil
}

// UpdateAgentTrigger updates an existing AgentTrigger.
func (s *AgentTriggerService) UpdateAgentTrigger(ctx context.Context, req *pb.UpdateAgentTriggerRequest) (*pb.AgentTrigger, error) {
	if req.AgentTrigger == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_trigger is required")
	}
	if req.AgentTrigger.Metadata == nil {
		return nil, status.Error(codes.InvalidArgument, "agent_trigger metadata is required")
	}
	if req.AgentTrigger.Metadata.Name == "" || req.AgentTrigger.Metadata.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_trigger metadata.name and metadata.namespace are required")
	}

	var existing agentv1alpha1.AgentTrigger
	key := client.ObjectKey{
		Namespace: req.AgentTrigger.Metadata.Namespace,
		Name:      req.AgentTrigger.Metadata.Name,
	}
	if err := s.client.Get(ctx, key, &existing); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent trigger")
	}

	updated := converter.AgentTriggerFromProto(req.AgentTrigger)
	updated.ResourceVersion = existing.ResourceVersion

	if err := s.client.Update(ctx, updated); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent trigger")
	}

	return converter.AgentTriggerToProto(updated), nil
}

// DeleteAgentTrigger deletes an AgentTrigger.
func (s *AgentTriggerService) DeleteAgentTrigger(ctx context.Context, req *pb.DeleteAgentTriggerRequest) (*pb.DeleteAgentTriggerResponse, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	trigger := &agentv1alpha1.AgentTrigger{}
	trigger.Name = req.Name
	trigger.Namespace = req.Namespace

	if err := s.client.Delete(ctx, trigger); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent trigger")
	}

	return &pb.DeleteAgentTriggerResponse{}, nil
}

// WatchAgentTriggers watches for changes to AgentTriggers.
func (s *AgentTriggerService) WatchAgentTriggers(_ *pb.WatchAgentTriggersRequest, _ pb.AgentTriggerService_WatchAgentTriggersServer) error {
	return status.Error(codes.Unimplemented, "watch not yet implemented: requires informer-based streaming")
}

// UpdateAgentTriggerStatus updates only the status subresource.
func (s *AgentTriggerService) UpdateAgentTriggerStatus(ctx context.Context, req *pb.UpdateAgentTriggerStatusRequest) (*pb.AgentTrigger, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.Status == nil {
		return nil, status.Error(codes.InvalidArgument, "status is required")
	}

	var trigger agentv1alpha1.AgentTrigger
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &trigger); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent trigger")
	}

	trigger.Status.Phase = converter.AgentTriggerPhaseFromProto(req.Status.Phase)
	trigger.Status.ObservedGeneration = req.Status.ObservedGeneration
	trigger.Status.AgentEndpoint = req.Status.AgentEndpoint
	trigger.Status.SensorName = req.Status.SensorName

	if req.Status.Conditions != nil {
		trigger.Status.Conditions = converter.ConditionsFromProto(req.Status.Conditions)
	}

	if req.Status.Invocations != nil {
		trigger.Status.Invocations = &agentv1alpha1.InvocationCounters{
			Total:        req.Status.Invocations.Total,
			Delivered:    req.Status.Invocations.Delivered,
			Dropped:      req.Status.Invocations.Dropped,
			DeadLettered: req.Status.Invocations.DeadLettered,
		}
	}

	if err := s.client.Status().Update(ctx, &trigger); err != nil {
		return nil, mapKubernetesError(ctx, err, "agent trigger")
	}

	return converter.AgentTriggerToProto(&trigger), nil
}
