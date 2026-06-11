package converter

import (
	"fmt"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"google.golang.org/protobuf/types/known/timestamppb"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// AgentWorkflowToProto converts a Kubernetes AgentWorkflow to proto.
func AgentWorkflowToProto(awf *agentv1alpha1.AgentWorkflow) *pb.AgentWorkflow {
	if awf == nil {
		return nil
	}

	return &pb.AgentWorkflow{
		Metadata: ObjectMetaToProto(&awf.ObjectMeta),
		Spec:     AgentWorkflowSpecToProto(&awf.Spec),
		Status:   AgentWorkflowStatusToProto(&awf.Status),
	}
}

// AgentWorkflowSpecToProto converts AgentWorkflowSpec to proto.
func AgentWorkflowSpecToProto(spec *agentv1alpha1.AgentWorkflowSpec) *pb.AgentWorkflowSpec {
	if spec == nil {
		return nil
	}

	pbSpec := &pb.AgentWorkflowSpec{
		Description: spec.Description,
	}

	for _, p := range spec.Params {
		pbSpec.Params = append(pbSpec.Params, &pb.WorkflowParam{
			Name:        p.Name,
			Description: p.Description,
			Value:       p.Value,
		})
	}

	for _, t := range spec.Tasks {
		pbSpec.Tasks = append(pbSpec.Tasks, WorkflowTaskToProto(&t))
	}

	if spec.Timeout != nil {
		pbSpec.Timeout = spec.Timeout.Duration.String()
	}

	return pbSpec
}

// WorkflowTaskToProto converts a WorkflowTask to proto.
func WorkflowTaskToProto(task *agentv1alpha1.WorkflowTask) *pb.WorkflowTask {
	if task == nil {
		return nil
	}

	pbTask := &pb.WorkflowTask{
		Name:      task.Name,
		DependsOn: task.DependsOn,
		Condition: task.Condition,
	}

	switch {
	case task.Agent != nil:
		pbTask.Type = "agent"
	case task.AgentTask != nil: //nolint:staticcheck // frozen field still reported for pre-existing objects
		pbTask.Type = "agentTask"
	case len(task.Switch) > 0:
		pbTask.Type = "switch"
	}

	return pbTask
}

// AgentWorkflowStatusToProto converts AgentWorkflowStatus to proto.
func AgentWorkflowStatusToProto(status *agentv1alpha1.AgentWorkflowStatus) *pb.AgentWorkflowStatus {
	if status == nil {
		return nil
	}

	return &pb.AgentWorkflowStatus{
		Ready:                status.Ready,
		WorkflowTemplateName: status.WorkflowTemplateName,
		SpecHash:             status.SpecHash,
		Conditions:           ConditionsToProto(status.Conditions),
		ObservedGeneration:   status.ObservedGeneration,
	}
}

// AgentWorkflowListToProto converts a Kubernetes AgentWorkflowList to proto.
func AgentWorkflowListToProto(list *agentv1alpha1.AgentWorkflowList) *pb.AgentWorkflowList {
	if list == nil {
		return nil
	}

	pbList := &pb.AgentWorkflowList{
		Metadata: ListMetaToProto(&list.ListMeta),
	}

	for i := range list.Items {
		pbList.Items = append(pbList.Items, AgentWorkflowToProto(&list.Items[i]))
	}

	return pbList
}

// ArgoWorkflowToRunProto converts an Argo Workflow to a WorkflowRun proto.
// If includeNodes is true, the full node tree is included.
func ArgoWorkflowToRunProto(wf *wfv1.Workflow, includeNodes bool) *pb.WorkflowRun {
	if wf == nil {
		return nil
	}

	run := &pb.WorkflowRun{
		Metadata: ObjectMetaToProto(&wf.ObjectMeta),
		Phase:    ArgoPhaseToRunPhase(wf.Status.Phase),
		Message:  wf.Status.Message,
	}

	if wf.Status.Progress != "" {
		run.Progress = string(wf.Status.Progress)
	}

	if !wf.Status.StartedAt.IsZero() {
		run.StartedAt = timestamppb.New(wf.Status.StartedAt.Time)
	}
	if !wf.Status.FinishedAt.IsZero() {
		run.FinishedAt = timestamppb.New(wf.Status.FinishedAt.Time)
	}

	// Extract workflow-level parameters (excluding internal ones)
	run.Parameters = make(map[string]string)
	for _, p := range wf.Spec.Arguments.Parameters {
		if p.Name == "_flokoa_traceparent" {
			continue
		}
		if p.Value != nil {
			run.Parameters[p.Name] = string(*p.Value)
		}
	}

	if includeNodes {
		for _, node := range wf.Status.Nodes {
			run.Nodes = append(run.Nodes, ArgoNodeToRunNodeProto(&node))
		}
	}

	return run
}

// ArgoNodeToRunNodeProto converts an Argo node status to a WorkflowRunNode proto.
func ArgoNodeToRunNodeProto(node *wfv1.NodeStatus) *pb.WorkflowRunNode {
	if node == nil {
		return nil
	}

	pbNode := &pb.WorkflowRunNode{
		Id:           node.ID,
		Name:         node.Name,
		DisplayName:  node.DisplayName,
		Type:         ArgoNodeTypeToProto(node.Type),
		Phase:        ArgoNodePhaseToRunPhase(node.Phase),
		Message:      node.Message,
		TemplateName: node.TemplateName,
		Children:     node.Children,
	}

	if !node.StartedAt.IsZero() {
		pbNode.StartedAt = timestamppb.New(node.StartedAt.Time)
	}
	if !node.FinishedAt.IsZero() {
		pbNode.FinishedAt = timestamppb.New(node.FinishedAt.Time)
	}

	// Extract inputs
	if node.Inputs != nil {
		pbNode.Inputs = make(map[string]string)
		for _, p := range node.Inputs.Parameters {
			if p.Value != nil {
				pbNode.Inputs[p.Name] = string(*p.Value)
			}
		}
	}

	// Extract outputs
	if node.Outputs != nil {
		pbNode.Outputs = make(map[string]string)
		for _, p := range node.Outputs.Parameters {
			if p.Value != nil {
				pbNode.Outputs[p.Name] = string(*p.Value)
			}
		}
	}

	return pbNode
}

// ArgoWorkflowListToRunListProto converts a list of Argo Workflows to a WorkflowRunList proto.
func ArgoWorkflowListToRunListProto(wfList *wfv1.WorkflowList) *pb.WorkflowRunList {
	if wfList == nil {
		return nil
	}

	pbList := &pb.WorkflowRunList{
		Metadata: &pb.ListMeta{
			ResourceVersion: wfList.ResourceVersion,
			Continue:        wfList.Continue,
		},
	}

	for i := range wfList.Items {
		pbList.Items = append(pbList.Items, ArgoWorkflowToRunProto(&wfList.Items[i], false))
	}

	return pbList
}

// ArgoPhaseToRunPhase converts an Argo WorkflowPhase to a RunPhase proto.
func ArgoPhaseToRunPhase(phase wfv1.WorkflowPhase) pb.RunPhase {
	switch phase {
	case wfv1.WorkflowPending:
		return pb.RunPhase_RUN_PHASE_PENDING
	case wfv1.WorkflowRunning:
		return pb.RunPhase_RUN_PHASE_RUNNING
	case wfv1.WorkflowSucceeded:
		return pb.RunPhase_RUN_PHASE_SUCCEEDED
	case wfv1.WorkflowFailed:
		return pb.RunPhase_RUN_PHASE_FAILED
	case wfv1.WorkflowError:
		return pb.RunPhase_RUN_PHASE_ERROR
	default:
		return pb.RunPhase_RUN_PHASE_UNSPECIFIED
	}
}

// ArgoNodePhaseToRunPhase converts an Argo NodePhase to a RunPhase proto.
func ArgoNodePhaseToRunPhase(phase wfv1.NodePhase) pb.RunPhase {
	switch phase {
	case wfv1.NodePending:
		return pb.RunPhase_RUN_PHASE_PENDING
	case wfv1.NodeRunning:
		return pb.RunPhase_RUN_PHASE_RUNNING
	case wfv1.NodeSucceeded:
		return pb.RunPhase_RUN_PHASE_SUCCEEDED
	case wfv1.NodeFailed:
		return pb.RunPhase_RUN_PHASE_FAILED
	case wfv1.NodeError:
		return pb.RunPhase_RUN_PHASE_ERROR
	default:
		return pb.RunPhase_RUN_PHASE_UNSPECIFIED
	}
}

// ArgoNodeTypeToProto converts an Argo NodeType to a NodeType proto.
func ArgoNodeTypeToProto(nt wfv1.NodeType) pb.NodeType {
	switch nt {
	case wfv1.NodeTypePod:
		return pb.NodeType_NODE_TYPE_POD
	case wfv1.NodeTypeSteps:
		return pb.NodeType_NODE_TYPE_STEPS
	case wfv1.NodeTypeDAG:
		return pb.NodeType_NODE_TYPE_DAG
	case wfv1.NodeTypeTaskGroup:
		return pb.NodeType_NODE_TYPE_TASK_GROUP
	case wfv1.NodeTypeRetry:
		return pb.NodeType_NODE_TYPE_RETRY
	case wfv1.NodeTypeSkipped:
		return pb.NodeType_NODE_TYPE_SKIPPED
	case wfv1.NodeTypeSuspend:
		return pb.NodeType_NODE_TYPE_SUSPEND
	default:
		// Plugin and other types
		if nt == "Plugin" {
			return pb.NodeType_NODE_TYPE_PLUGIN
		}
		return pb.NodeType_NODE_TYPE_UNSPECIFIED
	}
}

// WorkflowRunLabelSelector returns a label selector for finding workflow runs
// belonging to a specific AgentWorkflow.
func WorkflowRunLabelSelector(workflowName string) string {
	return fmt.Sprintf("agent.flokoa.ai/agentworkflow-name=%s", workflowName)
}
