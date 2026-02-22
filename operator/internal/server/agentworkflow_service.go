package server

import (
	"context"
	"fmt"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/converter"
	"github.com/danielnyari/flokoa/internal/telemetry"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// AgentWorkflowService implements the AgentWorkflowService gRPC service.
type AgentWorkflowService struct {
	pb.UnimplementedAgentWorkflowServiceServer
	client client.Client
}

// NewAgentWorkflowService creates a new AgentWorkflowService.
func NewAgentWorkflowService(c client.Client) *AgentWorkflowService {
	return &AgentWorkflowService{client: c}
}

// GetAgentWorkflow retrieves an AgentWorkflow by name and namespace.
func (s *AgentWorkflowService) GetAgentWorkflow(ctx context.Context, req *pb.GetAgentWorkflowRequest) (*pb.AgentWorkflow, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	var awf agentv1alpha1.AgentWorkflow
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.Name}
	if err := s.client.Get(ctx, key, &awf); err != nil {
		return nil, mapKubernetesError(ctx, err, "agentworkflow")
	}

	return converter.AgentWorkflowToProto(&awf), nil
}

// ListAgentWorkflows lists AgentWorkflows in a namespace or across all namespaces.
func (s *AgentWorkflowService) ListAgentWorkflows(ctx context.Context, req *pb.ListAgentWorkflowsRequest) (*pb.AgentWorkflowList, error) {
	var awfList agentv1alpha1.AgentWorkflowList
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

	if err := s.client.List(ctx, &awfList, opts...); err != nil {
		return nil, mapKubernetesError(ctx, err, "agentworkflow")
	}

	return converter.AgentWorkflowListToProto(&awfList), nil
}

// ListWorkflowRuns lists workflow runs (Argo Workflows) for a specific AgentWorkflow.
func (s *AgentWorkflowService) ListWorkflowRuns(ctx context.Context, req *pb.ListWorkflowRunsRequest) (*pb.WorkflowRunList, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.WorkflowName == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_name is required")
	}

	var wfList wfv1.WorkflowList
	opts := []client.ListOption{
		client.InNamespace(req.Namespace),
		client.MatchingLabels{
			"agent.flokoa.ai/agentworkflow-name": req.WorkflowName,
		},
	}

	if err := s.client.List(ctx, &wfList, opts...); err != nil {
		return nil, mapKubernetesError(ctx, err, "workflow run")
	}

	return converter.ArgoWorkflowListToRunListProto(&wfList), nil
}

// GetWorkflowRun retrieves a specific workflow run with full node details.
func (s *AgentWorkflowService) GetWorkflowRun(ctx context.Context, req *pb.GetWorkflowRunRequest) (*pb.WorkflowRun, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.RunName == "" {
		return nil, status.Error(codes.InvalidArgument, "run_name is required")
	}

	var wf wfv1.Workflow
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.RunName}
	if err := s.client.Get(ctx, key, &wf); err != nil {
		return nil, mapKubernetesError(ctx, err, "workflow run")
	}

	// Verify this run belongs to the requested workflow
	if req.WorkflowName != "" {
		label := wf.Labels["agent.flokoa.ai/agentworkflow-name"]
		if label != req.WorkflowName {
			return nil, status.Errorf(codes.NotFound, "workflow run %q does not belong to agentworkflow %q", req.RunName, req.WorkflowName)
		}
	}

	return converter.ArgoWorkflowToRunProto(&wf, true), nil
}

// SubmitWorkflowRun creates a new workflow run from the AgentWorkflow's template.
func (s *AgentWorkflowService) SubmitWorkflowRun(ctx context.Context, req *pb.SubmitWorkflowRunRequest) (*pb.WorkflowRun, error) {
	if req.Namespace == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}
	if req.WorkflowName == "" {
		return nil, status.Error(codes.InvalidArgument, "workflow_name is required")
	}

	// Verify the AgentWorkflow exists and is Ready
	var awf agentv1alpha1.AgentWorkflow
	key := client.ObjectKey{Namespace: req.Namespace, Name: req.WorkflowName}
	if err := s.client.Get(ctx, key, &awf); err != nil {
		return nil, mapKubernetesError(ctx, err, "agentworkflow")
	}

	if !awf.Status.Ready {
		return nil, status.Errorf(codes.FailedPrecondition, "agentworkflow %q is not ready", req.WorkflowName)
	}

	if awf.Status.WorkflowTemplateName == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "agentworkflow %q has no WorkflowTemplate", req.WorkflowName)
	}

	// Build workflow-level parameters
	params := make([]wfv1.Parameter, 0, len(req.Parameters)+1)
	for k, v := range req.Parameters {
		params = append(params, wfv1.Parameter{
			Name:  k,
			Value: wfv1.AnyStringPtr(v),
		})
	}

	// Always inject traceparent so the Argo WorkflowTemplate parameter is satisfied.
	// Use the caller's trace context if available, otherwise generate a fresh one
	// with a UUID7 trace ID so every run gets a unique, time-ordered trace.
	tp := telemetry.ExtractTraceparent(ctx)
	if tp == "" {
		tp = telemetry.NewTraceparent()
	}
	params = append(params, wfv1.Parameter{
		Name:  "_flokoa_traceparent",
		Value: wfv1.AnyStringPtr(tp),
	})

	// Create an Argo Workflow from the WorkflowTemplate
	wf := &wfv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", req.WorkflowName),
			Namespace:    req.Namespace,
			Labels: map[string]string{
				"agent.flokoa.ai/agentworkflow-name": req.WorkflowName,
				"app.kubernetes.io/managed-by":       "flokoa-server",
			},
		},
		Spec: wfv1.WorkflowSpec{
			WorkflowTemplateRef: &wfv1.WorkflowTemplateRef{
				Name: awf.Status.WorkflowTemplateName,
			},
			Arguments: wfv1.Arguments{
				Parameters: params,
			},
		},
	}

	if err := s.client.Create(ctx, wf); err != nil {
		return nil, mapKubernetesError(ctx, err, "workflow run")
	}

	return converter.ArgoWorkflowToRunProto(wf, false), nil
}
