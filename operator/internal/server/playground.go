package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const (
	playgroundTimeout      = 5 * time.Minute
	playgroundPollInterval = 2 * time.Second
	playgroundMaxPolls     = 150 // 5 min / 2 sec
)

// PlaygroundHandler handles playground chat requests, bridging AG-UI to A2A.
type PlaygroundHandler struct {
	client client.Client
	log    logr.Logger
}

// playgroundRequest is the JSON body of a playground chat request.
type playgroundRequest struct {
	Message string           `json:"message"`
	History []playgroundTurn `json:"history,omitempty"`
}

type playgroundTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AG-UI event types emitted as SSE named events.
type aguiEvent struct {
	Type      string `json:"type"`
	RunID     string `json:"runId,omitempty"`
	ThreadID  string `json:"threadId,omitempty"`
	MessageID string `json:"messageId,omitempty"`
	Role      string `json:"role,omitempty"`
	Delta     string `json:"delta,omitempty"`
	Message   string `json:"message,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

func (h *PlaygroundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespace := r.PathValue("namespace")
	name := r.PathValue("name")
	if namespace == "" || name == "" {
		http.Error(w, "namespace and name are required", http.StatusBadRequest)
		return
	}

	var req playgroundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	// Look up the Agent CR to get the A2A endpoint URL.
	var agent agentv1alpha1.Agent
	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := h.client.Get(r.Context(), key, &agent); err != nil {
		h.log.Error(err, "Failed to get agent", "namespace", namespace, "name", name)
		http.Error(w, fmt.Sprintf("agent not found: %v", err), http.StatusNotFound)
		return
	}

	endpoint := agent.Status.URL
	if endpoint == "" {
		http.Error(w, "agent has no endpoint URL", http.StatusServiceUnavailable)
		return
	}

	// Set up SSE streaming.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		h.log.V(1).Info("Could not disable write deadline for playground SSE", "error", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	runID := newUUID()
	threadID := newUUID()
	messageID := newUUID()

	// Emit RUN_STARTED
	h.writeEvent(w, flusher, "RUN_STARTED", aguiEvent{
		Type:      "RUN_STARTED",
		RunID:     runID,
		ThreadID:  threadID,
		Timestamp: nowMs(),
	})

	// Call the agent via A2A and stream back AG-UI events.
	text, err := h.callAgent(r.Context(), endpoint, req)
	if err != nil {
		h.log.Error(err, "A2A call failed", "agent", name, "namespace", namespace)
		h.writeEvent(w, flusher, "RUN_ERROR", aguiEvent{
			Type:      "RUN_ERROR",
			RunID:     runID,
			Message:   err.Error(),
			Timestamp: nowMs(),
		})
		return
	}

	// Emit text message events.
	h.writeEvent(w, flusher, "TEXT_MESSAGE_START", aguiEvent{
		Type:      "TEXT_MESSAGE_START",
		MessageID: messageID,
		Role:      "assistant",
		Timestamp: nowMs(),
	})

	h.writeEvent(w, flusher, "TEXT_MESSAGE_CONTENT", aguiEvent{
		Type:      "TEXT_MESSAGE_CONTENT",
		MessageID: messageID,
		Delta:     text,
		Timestamp: nowMs(),
	})

	h.writeEvent(w, flusher, "TEXT_MESSAGE_END", aguiEvent{
		Type:      "TEXT_MESSAGE_END",
		MessageID: messageID,
		Timestamp: nowMs(),
	})

	h.writeEvent(w, flusher, "RUN_FINISHED", aguiEvent{
		Type:      "RUN_FINISHED",
		RunID:     runID,
		ThreadID:  threadID,
		Timestamp: nowMs(),
	})
}

// callAgent sends a message to an A2A agent and returns the response text.
func (h *PlaygroundHandler) callAgent(ctx context.Context, endpoint string, req playgroundRequest) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, playgroundTimeout)
	defer cancel()

	// Build the A2A message.
	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: req.Message})

	// Try endpoint candidates (url and url/a2a).
	candidates := playgroundEndpointCandidates(endpoint)
	var (
		result       a2a.SendMessageResult
		sendErr      error
		usedEndpoint string
	)

	for _, candidate := range candidates {
		h.log.V(1).Info("Trying A2A endpoint", "endpoint", candidate)

		a2aClient, err := createA2AClient(ctx, candidate)
		if err != nil {
			sendErr = err
			continue
		}

		params := &a2a.MessageSendParams{
			Message: message,
		}

		result, sendErr = a2aClient.SendMessage(ctx, params)
		if sendErr == nil {
			usedEndpoint = candidate
			break
		}

		h.log.V(1).Info("A2A send failed, trying next", "endpoint", candidate, "error", sendErr)
	}

	if sendErr != nil {
		return "", fmt.Errorf("failed to send A2A message: %w", sendErr)
	}

	// Extract result from the A2A response.
	switch r := result.(type) {
	case *a2a.Task:
		if r == nil {
			return "", fmt.Errorf("received nil task from agent")
		}
		if r.Status.State.Terminal() {
			return extractPlaygroundText(r), nil
		}
		// Task is still running — poll for completion.
		return h.pollTask(ctx, usedEndpoint, r.ID)

	case *a2a.Message:
		if r == nil {
			return "", fmt.Errorf("received nil message from agent")
		}
		text := extractTextFromContentParts(r.Parts)
		if text != "" {
			return text, nil
		}
		// If we got a message back with a task ID, poll it.
		if r.TaskID != "" {
			return h.pollTask(ctx, usedEndpoint, r.TaskID)
		}
		return "", fmt.Errorf("received empty response from agent")

	default:
		return "", fmt.Errorf("unexpected A2A response type: %T", result)
	}
}

// pollTask polls an A2A task until it reaches a terminal state.
func (h *PlaygroundHandler) pollTask(ctx context.Context, endpoint string, taskID a2a.TaskID) (string, error) {
	a2aClient, err := createA2AClient(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to create A2A client for polling: %w", err)
	}

	for i := 0; i < playgroundMaxPolls; i++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(playgroundPollInterval):
		}

		task, err := a2aClient.GetTask(ctx, &a2a.TaskQueryParams{ID: taskID})
		if err != nil {
			h.log.V(1).Info("Poll error", "taskID", taskID, "error", err)
			continue
		}

		if task.Status.State.Terminal() {
			return extractPlaygroundText(task), nil
		}
	}

	return "", fmt.Errorf("A2A task timed out after polling")
}

// extractPlaygroundText gets the response text from a completed A2A task.
func extractPlaygroundText(task *a2a.Task) string {
	if task == nil {
		return ""
	}
	// Try status message first.
	if task.Status.Message != nil {
		if text := extractTextFromContentParts(task.Status.Message.Parts); text != "" {
			return text
		}
	}
	// Try last agent message in history.
	for i := len(task.History) - 1; i >= 0; i-- {
		if task.History[i].Role == a2a.MessageRoleAgent {
			if text := extractTextFromContentParts(task.History[i].Parts); text != "" {
				return text
			}
		}
	}
	// Try artifacts.
	for _, artifact := range task.Artifacts {
		if text := extractTextFromContentParts(artifact.Parts); text != "" {
			return text
		}
	}
	return ""
}

// extractTextFromContentParts extracts text from A2A content parts.
func extractTextFromContentParts(parts a2a.ContentParts) string {
	var texts []string
	for _, part := range parts {
		if tp, ok := part.(a2a.TextPart); ok {
			texts = append(texts, tp.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// createA2AClient creates an A2A client for the given endpoint.
func createA2AClient(ctx context.Context, endpoint string) (*a2aclient.Client, error) {
	endpoints := []a2a.AgentInterface{{
		URL:       endpoint,
		Transport: a2a.TransportProtocolJSONRPC,
	}}
	return a2aclient.NewFromEndpoints(ctx, endpoints,
		a2aclient.WithConfig(a2aclient.Config{Polling: true}),
	)
}

// playgroundEndpointCandidates returns endpoint URLs to try (same pattern as A2A plugin).
func playgroundEndpointCandidates(endpoint string) []string {
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

	// De-duplicate while preserving order.
	seen := make(map[string]struct{}, len(candidates))
	unique := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if _, ok := seen[c]; ok || c == "" {
			continue
		}
		seen[c] = struct{}{}
		unique = append(unique, c)
	}
	return unique
}

// writeEvent writes a named SSE event and flushes.
func (h *PlaygroundHandler) writeEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, evt aguiEvent) {
	data, err := json.Marshal(evt)
	if err != nil {
		h.log.Error(err, "Failed to marshal AG-UI event")
		return
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data); err != nil {
		h.log.V(1).Info("Failed to write SSE event", "error", err)
		return
	}
	flusher.Flush()
}

// nowMs returns the current time as Unix milliseconds.
func nowMs() int64 {
	return time.Now().UnixMilli()
}

// newUUID generates a random UUID v4 string.
func newUUID() string {
	var uuid [16]byte
	if _, err := rand.Read(uuid[:]); err != nil {
		// Fallback to timestamp-based if crypto/rand fails (shouldn't happen).
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
