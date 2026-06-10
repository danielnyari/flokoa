package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var pushTracer = otel.Tracer("flokoa-server/push-gateway")

// PushGatewayHandler forwards A2A push notifications to target agents.
// It receives notifications from sending agents and proxies them to the
// target agent's A2A endpoint with logging, metrics, and trace propagation.
type PushGatewayHandler struct {
	client     client.Client
	httpClient *http.Client
	log        logr.Logger
}

// NewPushGatewayHandler creates a new push gateway handler.
func NewPushGatewayHandler(c client.Client, log logr.Logger) *PushGatewayHandler {
	return &PushGatewayHandler{
		client:     c,
		httpClient: &http.Client{},
		log:        log,
	}
}

// ServeHTTP handles POST /api/v1alpha1/namespaces/{namespace}/agents/{name}/push
func (h *PushGatewayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	ctx, span := pushTracer.Start(r.Context(), fmt.Sprintf("push.forward %s", name),
		trace.WithAttributes(
			attribute.String("flokoa.push.target_agent", name),
			attribute.String("flokoa.push.target_namespace", namespace),
		))
	defer span.End()

	// Read the push notification body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error(err, "Failed to read push notification body")
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer func() { _ = r.Body.Close() }()

	// Resolve target agent's A2A endpoint
	agent := &agentv1alpha1.Agent{}
	if err := h.client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, agent); err != nil {
		if apierrors.IsNotFound(err) {
			http.Error(w, fmt.Sprintf("Agent %s/%s not found", namespace, name), http.StatusNotFound)
			return
		}
		h.log.Error(err, "Failed to get target agent", "agent", name, "namespace", namespace)
		http.Error(w, "Failed to resolve agent", http.StatusInternalServerError)
		return
	}

	if agent.Status.URL == "" {
		http.Error(w, fmt.Sprintf("Agent %s has no URL", name), http.StatusServiceUnavailable)
		return
	}

	// Forward the push notification to the target agent
	targetURL := agent.Status.URL
	fwdReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		h.log.Error(err, "Failed to create forward request")
		http.Error(w, "Failed to forward notification", http.StatusInternalServerError)
		return
	}

	fwdReq.ContentLength = int64(len(body))
	fwdReq.Header.Set("Content-Type", r.Header.Get("Content-Type"))

	resp, err := h.httpClient.Do(fwdReq)
	if err != nil {
		h.log.Error(err, "Failed to forward push notification", "target", targetURL)
		http.Error(w, "Failed to forward to agent", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	h.log.Info("Push notification forwarded",
		"targetAgent", name,
		"namespace", namespace,
		"status", resp.StatusCode,
	)

	// Relay the agent's response back
	w.WriteHeader(resp.StatusCode)
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "forwarded",
		"agentStatus": resp.StatusCode,
	}); err != nil {
		h.log.Error(err, "Failed to encode gateway response")
	}
	_ = respBody
}
