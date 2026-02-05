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

// ModelService implements the ModelService gRPC service.
type ModelService struct {
	pb.UnimplementedModelServiceServer
	client client.Client
}

// NewModelService creates a new ModelService.
func NewModelService(c client.Client) *ModelService {
	return &ModelService{client: c}
}

// GetModel retrieves a Model by name and namespace.
func (s *ModelService) GetModel(ctx context.Context, req *pb.GetModelRequest) (*pb.Model, error) {
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	var model agentv1alpha1.Model
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &model); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "model not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelToProto(&model), nil
}

// ListModels lists Models in a namespace or across all namespaces.
func (s *ModelService) ListModels(ctx context.Context, req *pb.ListModelsRequest) (*pb.ModelList, error) {
	var modelList agentv1alpha1.ModelList
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

	if err := s.client.List(ctx, &modelList, opts...); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelListToProto(&modelList), nil
}

// CreateModel creates a new Model.
func (s *ModelService) CreateModel(ctx context.Context, req *pb.CreateModelRequest) (*pb.Model, error) {
	if req.Model == nil {
		return nil, status.Error(codes.InvalidArgument, "model is required")
	}

	model := converter.ModelFromProto(req.Model)
	if req.Namespace != "" {
		model.Namespace = req.Namespace
	}

	if err := s.client.Create(ctx, model); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelToProto(model), nil
}

// UpdateModel updates an existing Model.
func (s *ModelService) UpdateModel(ctx context.Context, req *pb.UpdateModelRequest) (*pb.Model, error) {
	if req.Model == nil {
		return nil, status.Error(codes.InvalidArgument, "model is required")
	}

	// Get existing model
	var existing agentv1alpha1.Model
	key := client.ObjectKey{
		Namespace: req.Model.Metadata.Namespace,
		Name:      req.Model.Metadata.Name,
	}
	if err := s.client.Get(ctx, key, &existing); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "model not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Apply update
	updated := converter.ModelFromProto(req.Model)
	updated.ResourceVersion = existing.ResourceVersion

	if err := s.client.Update(ctx, updated); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelToProto(updated), nil
}

// DeleteModel deletes a Model.
func (s *ModelService) DeleteModel(ctx context.Context, req *pb.DeleteModelRequest) (*pb.DeleteModelResponse, error) {
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	model := &agentv1alpha1.Model{}
	model.Name = req.Name
	model.Namespace = req.Namespace

	if err := s.client.Delete(ctx, model); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "model not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.DeleteModelResponse{}, nil
}

// WatchModels watches for changes to Models.
func (s *ModelService) WatchModels(req *pb.WatchModelsRequest, stream pb.ModelService_WatchModelsServer) error {
	return status.Error(codes.Unimplemented, "watch not yet implemented")
}

// UpdateModelStatus updates only the status subresource.
func (s *ModelService) UpdateModelStatus(ctx context.Context, req *pb.UpdateModelStatusRequest) (*pb.Model, error) {
	if req.Name == "" || req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "name and namespace are required")
	}

	var model agentv1alpha1.Model
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &model); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, status.Error(codes.NotFound, "model not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Update status fields
	if req.Status != nil {
		model.Status.Ready = req.Status.Ready
		model.Status.ObservedGeneration = req.Status.ObservedGeneration
	}

	if err := s.client.Status().Update(ctx, &model); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return converter.ModelToProto(&model), nil
}
