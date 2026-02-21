package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/converter"
)

const (
	// sseHeartbeatInterval is how often we send a heartbeat comment to keep the connection alive.
	sseHeartbeatInterval = 30 * time.Second
)

// watchEventType converts a Kubernetes watch event type to the proto EventType int.
func watchEventType(t watch.EventType) string {
	switch t {
	case watch.Added:
		return "ADDED"
	case watch.Modified:
		return "MODIFIED"
	case watch.Deleted:
		return "DELETED"
	case watch.Bookmark:
		return "BOOKMARK"
	default:
		return "ERROR"
	}
}

// sseEvent is the JSON envelope sent over SSE for watch events.
type sseEvent struct {
	Type   string      `json:"type"`
	Object interface{} `json:"object"`
}

// writeSSE writes a single SSE event to the response writer and flushes.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, data []byte) error {
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// writeSSEHeartbeat sends an SSE comment to keep the connection alive.
func writeSSEHeartbeat(w http.ResponseWriter, flusher http.Flusher) error {
	if _, err := fmt.Fprint(w, ":\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

// watchAgentsHandler creates an HTTP handler that streams Agent watch events as SSE.
func watchAgentsHandler(watchClient client.WithWatch, log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")

		opts := []client.ListOption{}
		if namespace != "" {
			opts = append(opts, client.InNamespace(namespace))
		}

		var list agentv1alpha1.AgentList
		watcher, err := watchClient.Watch(r.Context(), &list, opts...)
		if err != nil {
			log.Error(err, "Failed to start agent watch")
			http.Error(w, "failed to start watch", http.StatusInternalServerError)
			return
		}
		defer watcher.Stop()

		streamSSE(w, r, watcher, log, func(obj client.Object) interface{} {
			if agent, ok := obj.(*agentv1alpha1.Agent); ok {
				return converter.AgentToProto(agent)
			}
			return nil
		})
	}
}

// watchModelsHandler creates an HTTP handler that streams Model watch events as SSE.
func watchModelsHandler(watchClient client.WithWatch, log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")

		opts := []client.ListOption{}
		if namespace != "" {
			opts = append(opts, client.InNamespace(namespace))
		}

		var list agentv1alpha1.ModelList
		watcher, err := watchClient.Watch(r.Context(), &list, opts...)
		if err != nil {
			log.Error(err, "Failed to start model watch")
			http.Error(w, "failed to start watch", http.StatusInternalServerError)
			return
		}
		defer watcher.Stop()

		streamSSE(w, r, watcher, log, func(obj client.Object) interface{} {
			if model, ok := obj.(*agentv1alpha1.Model); ok {
				return converter.ModelToProto(model)
			}
			return nil
		})
	}
}

// watchModelProvidersHandler creates an HTTP handler that streams ModelProvider watch events as SSE.
func watchModelProvidersHandler(watchClient client.WithWatch, log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")

		opts := []client.ListOption{}
		if namespace != "" {
			opts = append(opts, client.InNamespace(namespace))
		}

		var list agentv1alpha1.ModelProviderList
		watcher, err := watchClient.Watch(r.Context(), &list, opts...)
		if err != nil {
			log.Error(err, "Failed to start model provider watch")
			http.Error(w, "failed to start watch", http.StatusInternalServerError)
			return
		}
		defer watcher.Stop()

		streamSSE(w, r, watcher, log, func(obj client.Object) interface{} {
			if provider, ok := obj.(*agentv1alpha1.ModelProvider); ok {
				return converter.ModelProviderToProto(provider)
			}
			return nil
		})
	}
}

// watchAgentToolsHandler creates an HTTP handler that streams AgentTool watch events as SSE.
func watchAgentToolsHandler(watchClient client.WithWatch, log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")

		opts := []client.ListOption{}
		if namespace != "" {
			opts = append(opts, client.InNamespace(namespace))
		}

		var list agentv1alpha1.AgentToolList
		watcher, err := watchClient.Watch(r.Context(), &list, opts...)
		if err != nil {
			log.Error(err, "Failed to start agent tool watch")
			http.Error(w, "failed to start watch", http.StatusInternalServerError)
			return
		}
		defer watcher.Stop()

		streamSSE(w, r, watcher, log, func(obj client.Object) interface{} {
			if tool, ok := obj.(*agentv1alpha1.AgentTool); ok {
				return converter.AgentToolToProto(tool)
			}
			return nil
		})
	}
}

// watchAgentWorkflowsHandler creates an HTTP handler that streams AgentWorkflow watch events as SSE.
func watchAgentWorkflowsHandler(watchClient client.WithWatch, log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")

		opts := []client.ListOption{}
		if namespace != "" {
			opts = append(opts, client.InNamespace(namespace))
		}

		var list agentv1alpha1.AgentWorkflowList
		watcher, err := watchClient.Watch(r.Context(), &list, opts...)
		if err != nil {
			log.Error(err, "Failed to start agent workflow watch")
			http.Error(w, "failed to start watch", http.StatusInternalServerError)
			return
		}
		defer watcher.Stop()

		streamSSE(w, r, watcher, log, func(obj client.Object) interface{} {
			if awf, ok := obj.(*agentv1alpha1.AgentWorkflow); ok {
				return converter.AgentWorkflowToProto(awf)
			}
			return nil
		})
	}
}

// watchWorkflowRunsHandler creates an HTTP handler that streams Argo Workflow (run) watch events as SSE.
func watchWorkflowRunsHandler(watchClient client.WithWatch, log logr.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		namespace := r.PathValue("namespace")
		workflowName := r.PathValue("workflowName")

		opts := []client.ListOption{}
		if namespace != "" {
			opts = append(opts, client.InNamespace(namespace))
		}
		if workflowName != "" {
			opts = append(opts, client.MatchingLabels{
				"agent.flokoa.ai/agentworkflow-name": workflowName,
			})
		}

		var list wfv1.WorkflowList
		watcher, err := watchClient.Watch(r.Context(), &list, opts...)
		if err != nil {
			log.Error(err, "Failed to start workflow run watch")
			http.Error(w, "failed to start watch", http.StatusInternalServerError)
			return
		}
		defer watcher.Stop()

		streamSSE(w, r, watcher, log, func(obj client.Object) interface{} {
			if wf, ok := obj.(*wfv1.Workflow); ok {
				return converter.ArgoWorkflowToRunProto(wf, true)
			}
			return nil
		})
	}
}

// streamSSE is the core SSE streaming loop shared by all watch handlers.
// It writes SSE headers, then loops reading from the watcher channel, converting
// each event to JSON and writing it as an SSE data event. It sends periodic
// heartbeat comments to keep the connection alive.
func streamSSE(
	w http.ResponseWriter,
	r *http.Request,
	watcher watch.Interface,
	log logr.Logger,
	convert func(client.Object) interface{},
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Disable write deadline for this long-lived connection (Go 1.20+).
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		log.V(1).Info("Could not disable write deadline for SSE", "error", err)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	flusher.Flush()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	ch := watcher.ResultChan()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if err := writeSSEHeartbeat(w, flusher); err != nil {
				return
			}
		case event, ok := <-ch:
			if !ok {
				// Watch channel closed — the client should reconnect.
				return
			}

			obj, isObj := event.Object.(client.Object)
			if !isObj {
				continue
			}

			converted := convert(obj)
			if converted == nil {
				continue
			}

			evt := sseEvent{
				Type:   watchEventType(event.Type),
				Object: converted,
			}

			data, err := json.Marshal(evt)
			if err != nil {
				log.Error(err, "Failed to marshal watch event")
				continue
			}

			if err := writeSSE(w, flusher, data); err != nil {
				return
			}
		}
	}
}

// registerWatchRoutes registers all SSE watch endpoints on the given mux.
// These are separate from the gRPC-gateway routes because SSE requires
// text/event-stream content type and long-lived connections.
func registerWatchRoutes(mux *http.ServeMux, watchClient client.WithWatch, log logr.Logger, authFn func(http.Handler) http.Handler) {
	watchLog := log.WithName("watch")

	// Agent watches
	mux.Handle("GET /api/v1alpha1/watch/agents", authFn(watchAgentsHandler(watchClient, watchLog)))
	mux.Handle("GET /api/v1alpha1/watch/namespaces/{namespace}/agents", authFn(watchAgentsHandler(watchClient, watchLog)))

	// Model watches
	mux.Handle("GET /api/v1alpha1/watch/models", authFn(watchModelsHandler(watchClient, watchLog)))
	mux.Handle("GET /api/v1alpha1/watch/namespaces/{namespace}/models", authFn(watchModelsHandler(watchClient, watchLog)))

	// ModelProvider watches
	mux.Handle("GET /api/v1alpha1/watch/modelproviders", authFn(watchModelProvidersHandler(watchClient, watchLog)))
	mux.Handle("GET /api/v1alpha1/watch/namespaces/{namespace}/modelproviders", authFn(watchModelProvidersHandler(watchClient, watchLog)))

	// AgentTool watches
	mux.Handle("GET /api/v1alpha1/watch/agenttools", authFn(watchAgentToolsHandler(watchClient, watchLog)))
	mux.Handle("GET /api/v1alpha1/watch/namespaces/{namespace}/agenttools", authFn(watchAgentToolsHandler(watchClient, watchLog)))

	// AgentWorkflow watches
	mux.Handle("GET /api/v1alpha1/watch/agentworkflows", authFn(watchAgentWorkflowsHandler(watchClient, watchLog)))
	mux.Handle("GET /api/v1alpha1/watch/namespaces/{namespace}/agentworkflows", authFn(watchAgentWorkflowsHandler(watchClient, watchLog)))

	// WorkflowRun watches (Argo Workflows)
	mux.Handle("GET /api/v1alpha1/watch/namespaces/{namespace}/agentworkflows/{workflowName}/runs", authFn(watchWorkflowRunsHandler(watchClient, watchLog)))
}

// authMiddleware wraps an HTTP handler with Bearer token validation.
// If authInterceptor is nil (auth disabled), it returns a no-op wrapper.
func authMiddleware(authInterceptor *AuthInterceptor) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if authInterceptor == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Validate Bearer token from header or query param
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				// Also check query param for SSE (EventSource can't set headers)
				if token := r.URL.Query().Get("access_token"); token != "" {
					authHeader = "Bearer " + token
				}
			}

			if authHeader == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			const prefix = "Bearer "
			if len(authHeader) < len(prefix) || authHeader[:len(prefix)] != prefix {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token := authHeader[len(prefix):]

			if _, err := authInterceptor.VerifyHTTPToken(r.Context(), token); err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
