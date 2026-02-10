package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	executor "github.com/argoproj/argo-workflows/v3/pkg/plugins/executor"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Plugin implements the Argo Workflows executor plugin for A2A protocol
type Plugin struct {
	resolver *Resolver
	// tasks stores in-progress task state keyed by workflow UID + template name
	tasks sync.Map
}

// New creates a new A2A executor plugin
func New(k8sClient client.Client) *Plugin {
	return &Plugin{
		resolver: NewResolver(k8sClient),
	}
}

// taskKey generates a unique key for tracking tasks
func taskKey(workflowUID, templateName string) string {
	return fmt.Sprintf("%s/%s", workflowUID, templateName)
}

// ExecuteTemplate handles the execution of an A2A plugin template
func (p *Plugin) ExecuteTemplate(ctx context.Context, args executor.ExecuteTemplateArgs) (*executor.ExecuteTemplateReply, error) {
	// Parse the A2A spec from the template
	spec, err := parseA2ASpec(args.Template)
	if err != nil {
		return failedReply(fmt.Sprintf("failed to parse A2A spec: %v", err)), nil
	}

	// Use workflow namespace as default if not specified
	namespace := spec.Namespace
	if namespace == "" {
		namespace = args.Workflow.ObjectMeta.Namespace
	}

	// Generate key for tracking this task
	key := taskKey(args.Workflow.ObjectMeta.UID, args.Template.Name)

	// Check if this is a requeue (we have an in-progress task)
	if state, ok := p.tasks.Load(key); ok {
		progress := state.(*ProgressState)
		return p.pollTask(ctx, key, spec, progress)
	}

	// New task: resolve endpoint and send
	endpoint, err := p.resolver.Resolve(ctx, spec.Agent, namespace)
	if err != nil {
		return failedReply(fmt.Sprintf("failed to resolve agent endpoint: %v", err)), nil
	}

	return p.sendTask(ctx, key, spec, endpoint)
}

// sendTask sends a new task to the A2A agent
func (p *Plugin) sendTask(ctx context.Context, key string, spec *A2ASpec, endpoint string) (*executor.ExecuteTemplateReply, error) {
	// Create A2A client
	a2aClient, err := p.createClient(ctx, endpoint)
	if err != nil {
		return failedReply(fmt.Sprintf("failed to create A2A client: %v", err)), nil
	}

	// Create and send the message
	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: spec.Message})
	params := &a2a.MessageSendParams{
		Message: message,
	}

	result, err := a2aClient.SendMessage(ctx, params)
	if err != nil {
		return failedReply(fmt.Sprintf("failed to send A2A message: %v", err)), nil
	}

	// Extract task ID from the result
	var taskID a2a.TaskID
	var contextID string

	switch r := result.(type) {
	case *a2a.Task:
		taskID = r.ID
		contextID = r.ContextID
		// Check if task is already in terminal state
		if r.Status.State.Terminal() {
			return p.taskToReply(r), nil
		}
	case *a2a.Message:
		// If we got a message back directly, the task might be complete
		// We need to extract task ID and poll
		taskID = r.TaskID
		contextID = r.ContextID
		if taskID == "" {
			return failedReply("received message response without task ID"), nil
		}
	default:
		return failedReply(fmt.Sprintf("unexpected response type: %T", result)), nil
	}

	// Create progress state for polling
	progress := &ProgressState{
		TaskID:    string(taskID),
		ContextID: contextID,
		Endpoint:  endpoint,
		StartTime: time.Now(),
		Timeout:   spec.GetTimeout(),
	}

	// Store the state for subsequent polls
	p.tasks.Store(key, progress)

	// Return running state with requeue
	return &executor.ExecuteTemplateReply{
		Node: &wfv1.NodeResult{
			Phase:   wfv1.NodeRunning,
			Message: "A2A task submitted, waiting for completion",
		},
		Requeue: &metav1.Duration{Duration: DefaultPollInterval},
	}, nil
}

// pollTask polls an existing A2A task for completion
func (p *Plugin) pollTask(ctx context.Context, key string, spec *A2ASpec, progress *ProgressState) (*executor.ExecuteTemplateReply, error) {
	// Check for timeout
	if progress.IsTimedOut() {
		p.tasks.Delete(key)
		return failedReply(fmt.Sprintf("A2A task timed out after %v", progress.Timeout)), nil
	}

	// Create A2A client
	a2aClient, err := p.createClient(ctx, progress.Endpoint)
	if err != nil {
		return failedReply(fmt.Sprintf("failed to create A2A client: %v", err)), nil
	}

	// Query the task
	task, err := a2aClient.GetTask(ctx, &a2a.TaskQueryParams{
		ID: a2a.TaskID(progress.TaskID),
	})
	if err != nil {
		return failedReply(fmt.Sprintf("failed to get A2A task: %v", err)), nil
	}

	// Check if task is in terminal state
	if task.Status.State.Terminal() {
		p.tasks.Delete(key)
		return p.taskToReply(task), nil
	}

	// Task is still running, requeue
	return &executor.ExecuteTemplateReply{
		Node: &wfv1.NodeResult{
			Phase:   wfv1.NodeRunning,
			Message: fmt.Sprintf("A2A task in state: %s", task.Status.State),
		},
		Requeue: &metav1.Duration{Duration: DefaultPollInterval},
	}, nil
}

// createClient creates an A2A client for the given endpoint
func (p *Plugin) createClient(ctx context.Context, endpoint string) (*a2aclient.Client, error) {
	endpoints := []a2a.AgentInterface{{
		URL:       endpoint,
		Transport: a2a.TransportProtocolJSONRPC,
	}}

	return a2aclient.NewFromEndpoints(ctx, endpoints,
		a2aclient.WithConfig(a2aclient.Config{
			Polling: true, // We handle polling ourselves
		}),
	)
}

// taskToReply converts an A2A task to an Argo ExecuteTemplateReply
func (p *Plugin) taskToReply(task *a2a.Task) *executor.ExecuteTemplateReply {
	var phase wfv1.NodePhase
	var message string

	switch task.Status.State {
	case a2a.TaskStateCompleted:
		phase = wfv1.NodeSucceeded
		message = "A2A task completed successfully"
	case a2a.TaskStateFailed:
		phase = wfv1.NodeFailed
		message = "A2A task failed"
		if task.Status.Message != nil {
			message = extractTextFromParts(task.Status.Message.Parts)
		}
	case a2a.TaskStateCanceled:
		phase = wfv1.NodeFailed
		message = "A2A task was canceled"
	case a2a.TaskStateRejected:
		phase = wfv1.NodeFailed
		message = "A2A task was rejected"
		if task.Status.Message != nil {
			message = extractTextFromParts(task.Status.Message.Parts)
		}
	default:
		phase = wfv1.NodeFailed
		message = fmt.Sprintf("A2A task in unexpected terminal state: %s", task.Status.State)
	}

	// Extract result text from task
	result := extractResultFromTask(task)

	// Marshal full task response as JSON
	taskJSON, _ := json.Marshal(task)

	return &executor.ExecuteTemplateReply{
		Node: &wfv1.NodeResult{
			Phase:   phase,
			Message: message,
			Outputs: &wfv1.Outputs{
				Parameters: []wfv1.Parameter{
					{
						Name:  "result",
						Value: wfv1.AnyStringPtr(result),
					},
					{
						Name:  "taskResponse",
						Value: wfv1.AnyStringPtr(string(taskJSON)),
					},
				},
			},
		},
	}
}

// parseA2ASpec extracts the A2A spec from the template
func parseA2ASpec(template *wfv1.Template) (*A2ASpec, error) {
	if template == nil || template.Plugin == nil {
		return nil, fmt.Errorf("template or plugin is nil")
	}

	// Plugin.Value contains the raw JSON like {"a2a": {...}}
	var pluginData map[string]json.RawMessage
	if err := json.Unmarshal(template.Plugin.Value, &pluginData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plugin data: %w", err)
	}

	a2aData, ok := pluginData["a2a"]
	if !ok {
		return nil, fmt.Errorf("no 'a2a' key found in plugin spec")
	}

	var spec A2ASpec
	if err := json.Unmarshal(a2aData, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal A2A spec: %w", err)
	}

	if spec.Agent == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if spec.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	return &spec, nil
}

// failedReply creates a failed ExecuteTemplateReply with the given message
func failedReply(message string) *executor.ExecuteTemplateReply {
	return &executor.ExecuteTemplateReply{
		Node: &wfv1.NodeResult{
			Phase:   wfv1.NodeFailed,
			Message: message,
		},
	}
}

// extractResultFromTask extracts the response text from a completed task
func extractResultFromTask(task *a2a.Task) string {
	// First try to get from status message
	if task.Status.Message != nil {
		if text := extractTextFromParts(task.Status.Message.Parts); text != "" {
			return text
		}
	}

	// Then try from history (last agent message)
	for i := len(task.History) - 1; i >= 0; i-- {
		msg := task.History[i]
		if msg.Role == a2a.MessageRoleAgent {
			if text := extractTextFromParts(msg.Parts); text != "" {
				return text
			}
		}
	}

	// Finally try from artifacts
	for _, artifact := range task.Artifacts {
		if text := extractTextFromParts(artifact.Parts); text != "" {
			return text
		}
	}

	return ""
}

// extractTextFromParts extracts text content from message parts
func extractTextFromParts(parts a2a.ContentParts) string {
	var texts []string
	for _, part := range parts {
		if textPart, ok := part.(a2a.TextPart); ok {
			texts = append(texts, textPart.Text)
		}
	}
	if len(texts) > 0 {
		return texts[0] // Return first text part
	}
	return ""
}
