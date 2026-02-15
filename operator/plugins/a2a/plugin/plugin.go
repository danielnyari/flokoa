package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
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

// buildA2AMessage converts the plugin's A2AMessage into an a2a.Message.
func buildA2AMessage(msg *A2AMessage) *a2a.Message {
	role := a2a.MessageRoleUser
	if msg.Role == "agent" {
		role = a2a.MessageRoleAgent
	}

	var parts []a2a.Part
	for _, p := range msg.Parts {
		switch {
		case p.Text != nil:
			tp := a2a.TextPart{Text: p.Text.Text}
			if len(p.Text.Metadata) > 0 {
				tp.Metadata = make(map[string]any, len(p.Text.Metadata))
				for k, v := range p.Text.Metadata {
					tp.Metadata[k] = v
				}
			}
			parts = append(parts, tp)
		case p.Data != nil:
			dp := a2a.DataPart{Data: p.Data.Data}
			if len(p.Data.Metadata) > 0 {
				dp.Metadata = make(map[string]any, len(p.Data.Metadata))
				for k, v := range p.Data.Metadata {
					dp.Metadata[k] = v
				}
			}
			parts = append(parts, dp)
		case p.File != nil:
			var fp a2a.FilePart
			if p.File.File.URI != "" {
				fp.File = &a2a.FileURI{
					FileMeta: a2a.FileMeta{
						Name:     p.File.File.Name,
						MimeType: p.File.File.MimeType,
					},
					URI: p.File.File.URI,
				}
			} else if p.File.File.Bytes != "" {
				fp.File = &a2a.FileBytes{
					FileMeta: a2a.FileMeta{
						Name:     p.File.File.Name,
						MimeType: p.File.File.MimeType,
					},
					Bytes: p.File.File.Bytes,
				}
			}
			if len(p.File.Metadata) > 0 {
				fp.Metadata = make(map[string]any, len(p.File.Metadata))
				for k, v := range p.File.Metadata {
					fp.Metadata[k] = v
				}
			}
			parts = append(parts, fp)
		}
	}

	message := a2a.NewMessage(role, parts...)
	if msg.ContextID != "" {
		message.ContextID = msg.ContextID
	}
	if len(msg.ReferenceTaskIDs) > 0 {
		refIDs := make([]a2a.TaskID, len(msg.ReferenceTaskIDs))
		for i, id := range msg.ReferenceTaskIDs {
			refIDs[i] = a2a.TaskID(id)
		}
		message.ReferenceTasks = refIDs
	}
	if len(msg.Extensions) > 0 {
		message.Extensions = msg.Extensions
	}
	if len(msg.Metadata) > 0 {
		message.Metadata = msg.Metadata
	}

	return message
}

// buildA2ASendConfig converts the plugin's A2ASendConfig into an a2a.MessageSendConfig.
func buildA2ASendConfig(cfg *A2ASendConfig) *a2a.MessageSendConfig {
	if cfg == nil {
		return nil
	}
	result := &a2a.MessageSendConfig{}
	if len(cfg.AcceptedOutputModes) > 0 {
		result.AcceptedOutputModes = cfg.AcceptedOutputModes
	}
	if cfg.Blocking != nil {
		result.Blocking = cfg.Blocking
	}
	if cfg.HistoryLength != nil {
		result.HistoryLength = cfg.HistoryLength
	}
	return result
}

// sendTask sends a new task to the A2A agent
func (p *Plugin) sendTask(ctx context.Context, key string, spec *A2ASpec, endpoint string) (*executor.ExecuteTemplateReply, error) {
	// Build the A2A message from structured parts
	message := buildA2AMessage(&spec.Message)
	params := &a2a.MessageSendParams{
		Message: message,
		Config:  buildA2ASendConfig(spec.Config),
	}

	candidates := endpointCandidates(endpoint)
	var (
		result       a2a.SendMessageResult
		err          error
		usedEndpoint string
	)

	for i, candidate := range candidates {
		log.Printf("Sending A2A message: endpoint=%s attempt=%d/%d", candidate, i+1, len(candidates))

		// Create A2A client
		a2aClient, clientErr := p.createClient(ctx, candidate)
		if clientErr != nil {
			err = clientErr
			log.Printf("A2A client creation failed: endpoint=%s error=%v", candidate, clientErr)
			continue
		}

		result, err = a2aClient.SendMessage(ctx, params)
		if err == nil {
			usedEndpoint = candidate
			break
		}

		log.Printf("A2A send failed: endpoint=%s error=%v", candidate, err)
	}

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
		Endpoint:  usedEndpoint,
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
		log.Printf("A2A get task failed: endpoint=%s taskID=%s error=%v", progress.Endpoint, progress.TaskID, err)
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

func endpointCandidates(endpoint string) []string {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if trimmed == "" {
		return []string{endpoint}
	}

	candidates := []string{trimmed}
	if strings.HasSuffix(trimmed, "/a2a") {
		candidates = append(candidates, strings.TrimSuffix(trimmed, "/a2a"))
	} else {
		candidates = append(candidates, trimmed+"/a2a")
	}

	// de-duplicate while preserving order
	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := seen[candidate]; ok || candidate == "" {
			continue
		}
		seen[candidate] = struct{}{}
		unique = append(unique, candidate)
	}

	return unique
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
	if len(spec.Message.Parts) == 0 {
		return nil, fmt.Errorf("message must have at least one part")
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
