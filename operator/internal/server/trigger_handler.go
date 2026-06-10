package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	triggerapp "github.com/danielnyari/flokoa/internal/app/trigger"
	triggerdomain "github.com/danielnyari/flokoa/internal/domain/trigger"
)

var triggerTracer = otel.Tracer("flokoa-server/trigger")

// TriggerInvokePayload is the JSON payload sent from the Argo Events Sensor
// HTTP trigger to flokoa-server's invoke endpoint.
type TriggerInvokePayload struct {
	Data        json.RawMessage `json:"data"`
	EventID     string          `json:"eventId"`
	EventType   string          `json:"eventType"`
	EventSource string          `json:"eventSource"`
	EventTime   string          `json:"eventTime"`
}

// TriggerHandler handles event invocations from Argo Events Sensors.
type TriggerHandler struct {
	client  client.Client
	limiter *TriggerLimiter
	log     logr.Logger
}

// NewTriggerHandler creates a new trigger handler.
func NewTriggerHandler(c client.Client, log logr.Logger) *TriggerHandler {
	return &TriggerHandler{
		client:  c,
		limiter: NewTriggerLimiter(),
		log:     log,
	}
}

// ServeHTTP handles POST /api/v1alpha1/namespaces/{namespace}/agenttriggers/{name}/invoke
func (h *TriggerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	namespace := r.PathValue("namespace")
	name := r.PathValue("name")
	if namespace == "" || name == "" {
		http.Error(w, "Missing namespace or name in path", http.StatusBadRequest)
		return
	}

	ctx, span := triggerTracer.Start(r.Context(), fmt.Sprintf("agenttrigger.invoke %s", name),
		trace.WithAttributes(
			attribute.String("flokoa.trigger.name", name),
			attribute.String("flokoa.trigger.namespace", namespace),
		))
	defer span.End()

	// Parse payload
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error(err, "Failed to read request body")
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	var payload TriggerInvokePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		h.log.Error(err, "Failed to parse event payload")
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	span.SetAttributes(
		attribute.String("flokoa.event.type", payload.EventType),
		attribute.String("flokoa.event.source", payload.EventSource),
		attribute.String("flokoa.event.id", payload.EventID),
	)

	// Load trigger config from ConfigMap
	config, err := h.loadTriggerConfig(ctx, namespace, name)
	if err != nil {
		h.log.Error(err, "Failed to load trigger config", "trigger", name, "namespace", namespace)
		http.Error(w, "Trigger config not found", http.StatusNotFound)
		return
	}

	// Check rate limits
	if err := h.limiter.Check(name, namespace, config.Limits); err != nil {
		h.log.Info("Event dropped due to rate limit", "trigger", name, "reason", err.Error())
		span.SetAttributes(attribute.String("flokoa.drop.reason", err.Error()))
		// TODO: Route to dead letter sink if configured
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	h.log.Info("Event received for trigger",
		"trigger", name,
		"namespace", namespace,
		"eventType", payload.EventType,
		"eventId", payload.EventID,
		"agentEndpoint", config.AgentEndpoint,
	)

	// TODO: Implement full A2A SendMessage invocation pipeline
	// 1. Extract session key from payload (if sessionKeyFrom is configured)
	// 2. Build A2A SendMessage request with event data as DataPart
	// 3. Send to agent endpoint via a2a-go client
	// 4. Record task ID, update invocation counters
	// 5. Record Prometheus metrics

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "accepted"}); err != nil {
		h.log.Error(err, "Failed to encode response")
	}
}

// loadTriggerConfig reads the trigger configuration from the ConfigMap.
func (h *TriggerHandler) loadTriggerConfig(ctx context.Context, namespace, name string) (*triggerapp.TriggerConfig, error) {
	cmName := triggerdomain.ConfigMapName(name)

	cm := &corev1.ConfigMap{}
	if err := h.client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: namespace}, cm); err != nil {
		return nil, fmt.Errorf("failed to get trigger config ConfigMap %s: %w", cmName, err)
	}

	configJSON, ok := cm.Data["config.json"]
	if !ok {
		return nil, fmt.Errorf("config.json not found in ConfigMap %s", cmName)
	}

	config := &triggerapp.TriggerConfig{}
	if err := json.Unmarshal([]byte(configJSON), config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trigger config: %w", err)
	}

	return config, nil
}
