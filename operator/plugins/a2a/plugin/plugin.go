package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	executor "github.com/argoproj/argo-workflows/v3/pkg/plugins/executor"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/danielnyari/flokoa/internal/telemetry"
)

// Plugin implements the Argo Workflows executor plugin for A2A protocol
type Plugin struct {
	resolver *Resolver
	// tasks stores in-progress task state keyed by workflow UID + template name.
	// Backed by a ConfigMap for persistence across restarts (fixes #97).
	tasks *StateStore
}

// New creates a new A2A executor plugin with persistent state storage.
// The namespace is used for the backing ConfigMap that persists task state.
func New(k8sClient client.Client, namespace string) *Plugin {
	return &Plugin{
		resolver: NewResolver(k8sClient),
		tasks:    NewStateStore(k8sClient, namespace),
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

	// Restore the distributed trace context from the traceparent injected by
	// the controller into the Argo workflow parameter.
	if spec.Traceparent != "" {
		ctx = telemetry.ContextFromTraceparent(ctx, spec.Traceparent)
	}

	ctx, span := telemetry.Tracer("flokoa.a2a-plugin").Start(ctx, "a2a.execute_template",
		trace.WithAttributes(
			attribute.String("a2a.agent", spec.Agent),
			attribute.String("a2a.template", args.Template.Name),
		),
	)
	defer span.End()

	// Use workflow namespace as default if not specified
	namespace := spec.Namespace
	if namespace == "" {
		namespace = args.Workflow.ObjectMeta.Namespace
	}

	// Generate key for tracking this task
	key := taskKey(args.Workflow.ObjectMeta.UID, args.Template.Name)

	// Check if this is a requeue (we have an in-progress task)
	if progress, ok := p.tasks.Load(key); ok {
		return p.pollTask(ctx, key, spec, progress)
	}

	// New task: resolve endpoint and send
	endpoint, err := p.resolver.Resolve(ctx, spec.Agent, namespace)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to resolve agent endpoint")
		return failedReply(fmt.Sprintf("failed to resolve agent endpoint: %v", err)), nil
	}
	span.SetAttributes(attribute.String("a2a.endpoint", endpoint))

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
	if msg.TaskID != "" {
		message.TaskID = a2a.TaskID(msg.TaskID)
	}
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
	if cfg.PushNotificationConfig != nil {
		pushCfg := &a2a.PushConfig{
			URL: cfg.PushNotificationConfig.URL,
		}
		if cfg.PushNotificationConfig.ID != "" {
			pushCfg.ID = cfg.PushNotificationConfig.ID
		}
		if cfg.PushNotificationConfig.Token != "" {
			pushCfg.Token = cfg.PushNotificationConfig.Token
		}
		if cfg.PushNotificationConfig.Authentication != nil {
			pushCfg.Auth = &a2a.PushAuthInfo{
				Schemes:     cfg.PushNotificationConfig.Authentication.Schemes,
				Credentials: cfg.PushNotificationConfig.Authentication.Credentials,
			}
		}
		result.PushConfig = pushCfg
	}
	return result
}

// sendTask sends a new task to the A2A agent
func (p *Plugin) sendTask(ctx context.Context, key string, spec *A2ASpec, endpoint string) (*executor.ExecuteTemplateReply, error) {
	ctx, span := telemetry.Tracer("flokoa.a2a-plugin").Start(ctx, "a2a.send",
		trace.WithAttributes(
			attribute.String("a2a.agent", spec.Agent),
			attribute.String("a2a.endpoint", endpoint),
		),
	)
	defer span.End()

	// Build the A2A message from structured parts
	message := buildA2AMessage(&spec.Message)

	// Inject the child span's traceparent into the A2A message metadata so the
	// downstream agent can restore the trace context even if HTTP header
	// propagation is not available.
	childTraceparent := telemetry.ExtractTraceparent(ctx)
	if childTraceparent != "" {
		if message.Metadata == nil {
			message.Metadata = make(map[string]any)
		}
		message.Metadata["traceparent"] = childTraceparent
	}

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
		if r == nil {
			return failedReply("received nil task from A2A agent"), nil
		}
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
	ctx, span := telemetry.Tracer("flokoa.a2a-plugin").Start(ctx, "a2a.poll",
		trace.WithAttributes(
			attribute.String("a2a.task_id", progress.TaskID),
			attribute.String("a2a.endpoint", progress.Endpoint),
		),
	)
	defer span.End()

	// Check for timeout
	if progress.IsTimedOut() {
		p.tasks.Delete(key)
		span.SetStatus(codes.Error, "task timed out")
		return failedReply(fmt.Sprintf("A2A task timed out after %v", progress.Timeout)), nil
	}

	// Create A2A client
	a2aClient, err := p.createClient(ctx, progress.Endpoint)
	if err != nil {
		span.RecordError(err)
		return failedReply(fmt.Sprintf("failed to create A2A client: %v", err)), nil
	}

	// Query the task
	task, err := a2aClient.GetTask(ctx, &a2a.TaskQueryParams{
		ID: a2a.TaskID(progress.TaskID),
	})
	if err != nil {
		log.Printf("A2A get task failed: endpoint=%s taskID=%s error=%v", progress.Endpoint, progress.TaskID, err)
		span.RecordError(err)

		// Distinguish transient errors from permanent ones: requeue on transient
		// failures so that network blips don't permanently fail the workflow step.
		progress.PollErrors++
		if progress.PollErrors < MaxPollErrors {
			log.Printf("Transient poll error (%d/%d), requeueing: %v", progress.PollErrors, MaxPollErrors, err)
			return &executor.ExecuteTemplateReply{
				Node: &wfv1.NodeResult{
					Phase:   wfv1.NodeRunning,
					Message: fmt.Sprintf("A2A poll error (attempt %d/%d): %v", progress.PollErrors, MaxPollErrors, err),
				},
				Requeue: &metav1.Duration{Duration: DefaultPollInterval},
			}, nil
		}
		return failedReply(fmt.Sprintf("failed to get A2A task after %d attempts: %v", progress.PollErrors, err)), nil
	}

	// Reset error counter on successful poll
	progress.PollErrors = 0

	span.SetAttributes(attribute.String("a2a.task_state", string(task.Status.State)))

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

	// Extract result text and artifact JSON from task
	result := extractResultFromTask(task)
	artifact := extractArtifactJSON(task)

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
						Name:  "artifact",
						Value: wfv1.AnyStringPtr(artifact),
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

// extractArtifactJSON returns the first artifact from a task as JSON, or "{}" if none.
func extractArtifactJSON(task *a2a.Task) string {
	if len(task.Artifacts) > 0 {
		data, err := json.Marshal(task.Artifacts[0])
		if err == nil {
			return string(data)
		}
	}
	return "{}"
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
