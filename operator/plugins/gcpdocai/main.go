package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	executor "github.com/argoproj/argo-workflows/v3/pkg/plugins/executor"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/danielnyari/flokoa/internal/telemetry"
	"github.com/danielnyari/flokoa/operator/plugins/gcpdocai/plugin"
)

const (
	defaultPort         = "4355"
	defaultReadTimeout  = 30 * time.Second
	defaultWriteTimeout = 30 * time.Second
	tokenPath           = "/var/run/argo/token"
)

var scheme = runtime.NewScheme()

var argoToken string

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	// Read the Argo token for authorization.
	if token, err := os.ReadFile(tokenPath); err == nil {
		argoToken = string(token)
		log.Printf("Loaded Argo token for authorization")
	} else if os.Getenv("FLOKOA_DEV_MODE") == "true" {
		log.Printf("Warning: Argo token not available at %s (%v) — running without auth (FLOKOA_DEV_MODE=true)", tokenPath, err)
	} else {
		log.Fatalf("FATAL: cannot read Argo token at %s: %v. "+
			"Ensure the sidecar securityContext includes runAsGroup: 0. "+
			"Set FLOKOA_DEV_MODE=true to bypass (development only).", tokenPath, err)
	}
}

func main() {
	// Initialize OpenTelemetry distributed tracing.
	otelShutdown, err := telemetry.Init(context.Background(), "flokoa-gcpdocai-plugin")
	if err != nil {
		log.Fatalf("Failed to initialize OpenTelemetry: %v", err)
	}
	defer func() {
		if shutdownErr := otelShutdown(context.Background()); shutdownErr != nil {
			log.Printf("Failed to shut down OpenTelemetry: %v", shutdownErr)
		}
	}()

	// Wrap the default HTTP transport with OTEL instrumentation.
	http.DefaultTransport = otelhttp.NewTransport(http.DefaultTransport)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	// Create Kubernetes client for ConfigMap state persistence.
	k8sClient, err := createK8sClient()
	if err != nil {
		log.Printf("Warning: failed to create Kubernetes client, state will not persist across restarts: %v", err)
	}

	// Determine the namespace for persisting plugin state.
	pluginNamespace := os.Getenv("POD_NAMESPACE")
	if pluginNamespace == "" {
		pluginNamespace = "default"
	}

	// Create the plugin with persistent state storage
	p := plugin.New(k8sClient, pluginNamespace)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/template.execute", handleExecuteTemplate(p))
	mux.HandleFunc("/healthz", handleHealthz)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  defaultReadTimeout,
		WriteTimeout: defaultWriteTimeout,
	}

	// Start server in goroutine
	go func() {
		log.Printf("GCP Document AI executor plugin listening on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for shutdown signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	log.Println("Server stopped")
}

// handleExecuteTemplate handles the /api/v1/template.execute endpoint.
func handleExecuteTemplate(p *plugin.Plugin) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Check authorization token
		if argoToken != "" {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer "+argoToken {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}

		// Decode request
		var req executor.ExecuteTemplateRequest
		if err := json.NewDecoder(r.Body).Decode(&req.Body); err != nil {
			writeError(w, fmt.Sprintf("failed to decode request: %v", err), http.StatusBadRequest)
			return
		}

		// Check if this template is for our plugin (has "gcpdocai" key)
		if !isGCPDocAITemplate(&req.Body) {
			// Return empty response to indicate we can't handle this template
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("{}"))
			return
		}

		// Execute the plugin
		reply, err := p.ExecuteTemplate(r.Context(), req.Body)
		if err != nil {
			writeError(w, fmt.Sprintf("plugin execution failed: %v", err), http.StatusInternalServerError)
			return
		}

		// Write response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(reply); err != nil {
			log.Printf("Failed to encode response: %v", err)
		}
	}
}

// handleHealthz handles health check requests.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// writeError writes an error response as JSON.
func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	reply := executor.ExecuteTemplateReply{
		Node: nil,
	}

	if err := json.NewEncoder(w).Encode(reply); err != nil {
		log.Printf("Failed to encode error response: %v", err)
	}

	log.Printf("Error: %s", message)
}

// isGCPDocAITemplate checks if the template contains a "gcpdocai" plugin spec.
func isGCPDocAITemplate(args *executor.ExecuteTemplateArgs) bool {
	if args == nil || args.Template == nil || args.Template.Plugin == nil {
		return false
	}

	var pluginData map[string]json.RawMessage
	if err := json.Unmarshal(args.Template.Plugin.Value, &pluginData); err != nil {
		return false
	}

	_, ok := pluginData["gcpdocai"]
	return ok
}

// createK8sClient creates a Kubernetes client for ConfigMap state persistence.
func createK8sClient() (client.Client, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	c, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, nil
}
