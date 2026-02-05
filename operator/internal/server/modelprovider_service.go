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

// ModelProviderService implements the ModelProviderService gRPC service.
type ModelProviderService struct {
	pb.UnimplementedModelProviderServiceServer
	client client.Client
}

// NewModelProviderService creates a new ModelProviderService.
func NewModelProviderService(c client.Client) *ModelProviderService {
	return &ModelProviderService{client: c}
}

// GetModelProvider retrieves a ModelProvider by name and namespace.
func (s *ModelProviderService) GetModelProvider(ctx context.Context, req *pb.GetModelProviderRequest) (*pb.ModelProvider, error) {
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	var provider agentv1alpha1.ModelProvider
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &provider); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "model provider not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelProviderToProto(&provider), nil
}

// ListModelProviders lists ModelProviders in a namespace or across all namespaces.
func (s *ModelProviderService) ListModelProviders(ctx context.Context, req *pb.ListModelProvidersRequest) (*pb.ModelProviderList, error) {
	var providerList agentv1alpha1.ModelProviderList
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

	if err := s.client.List(ctx, &providerList, opts...); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelProviderListToProto(&providerList), nil
}

// CreateModelProvider creates a new ModelProvider.
func (s *ModelProviderService) CreateModelProvider(ctx context.Context, req *pb.CreateModelProviderRequest) (*pb.ModelProvider, error) {
	if req.ModelProvider == nil {
		return nil, status.Error(codes.InvalidArgument, "model_provider is required")
	}

	provider := converter.ModelProviderFromProto(req.ModelProvider)
	if req.Namespace != "" {
		provider.Namespace = req.Namespace
	}

	if err := s.client.Create(ctx, provider); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelProviderToProto(provider), nil
}

// UpdateModelProvider updates an existing ModelProvider.
func (s *ModelProviderService) UpdateModelProvider(ctx context.Context, req *pb.UpdateModelProviderRequest) (*pb.ModelProvider, error) {
	if req.ModelProvider == nil {
		return nil, status.Error(codes.InvalidArgument, "model_provider is required")
	}

	// Get existing provider
	var existing agentv1alpha1.ModelProvider
	key := client.ObjectKey{
		Namespace: req.ModelProvider.Metadata.Namespace,
		Name:      req.ModelProvider.Metadata.Name,
	}
	if err := s.client.Get(ctx, key, &existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "model provider not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Apply update
	updated := converter.ModelProviderFromProto(req.ModelProvider)
	updated.ResourceVersion = existing.ResourceVersion

	if err := s.client.Update(ctx, updated); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelProviderToProto(updated), nil
}

// DeleteModelProvider deletes a ModelProvider.
func (s *ModelProviderService) DeleteModelProvider(ctx context.Context, req *pb.DeleteModelProviderRequest) (*pb.DeleteModelProviderResponse, error) {
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	provider := &agentv1alpha1.ModelProvider{}
	provider.Name = req.Name
	provider.Namespace = req.Namespace

	if err := s.client.Delete(ctx, provider); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "model provider not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.DeleteModelProviderResponse{}, nil
}

// WatchModelProviders watches for changes to ModelProviders.
func (s *ModelProviderService) WatchModelProviders(req *pb.WatchModelProvidersRequest, stream pb.ModelProviderService_WatchModelProvidersServer) error {
	return status.Error(codes.Unimplemented, "watch not yet implemented")
}

// UpdateModelProviderStatus updates only the status subresource.
func (s *ModelProviderService) UpdateModelProviderStatus(ctx context.Context, req *pb.UpdateModelProviderStatusRequest) (*pb.ModelProvider, error) {
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	var provider agentv1alpha1.ModelProvider
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &provider); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "model provider not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Update status fields
	if req.Status != nil {
		provider.Status.Ready = req.Status.Ready
		provider.Status.ObservedGeneration = req.Status.ObservedGeneration
	}

	if err := s.client.Status().Update(ctx, &provider); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelProviderToProto(&provider), nil
}
