package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/rs/cors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/encoding/protojson"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/danielnyari/flokoa/internal/config"
	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
	uiFS "github.com/danielnyari/flokoa/ui"
)

// PublicGRPCMethods lists gRPC methods that skip authentication.
// Only health checks are public; reflection requires authentication
// to prevent unauthenticated API schema discovery.
var PublicGRPCMethods = []string{
	"/grpc.health.v1.Health/Check",
	"/grpc.health.v1.Health/Watch",
}

// Server wraps the gRPC server and its configuration.
type Server struct {
	grpcServer      *grpc.Server
	httpServer      *http.Server
	healthServer    *health.Server
	port            int
	httpPort        int
	authCfg         config.AuthConfig
	log             logr.Logger
	watchClient     client.WithWatch
	authInterceptor *AuthInterceptor
}

// NewServer creates a new gRPC server with optional reflection support.
// If authInterceptor is non-nil, it is added to the interceptor chain.
// watchClient enables SSE watch endpoints for real-time UI updates.
// If reflectionEnabled is true, gRPC server reflection is registered,
// allowing clients to discover services and their schemas at runtime.
func NewServer(
	port int,
	httpPort int,
	authCfg config.AuthConfig,
	log logr.Logger,
	authInterceptor *AuthInterceptor,
	watchClient client.WithWatch,
	reflectionEnabled bool,
	agentService pb.AgentServiceServer,
	modelService pb.ModelServiceServer,
	modelProviderService pb.ModelProviderServiceServer,
	agentToolService pb.AgentToolServiceServer,
	agentWorkflowService pb.AgentWorkflowServiceServer,
) *Server {
	// Build interceptor chains
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		LoggingUnaryInterceptor(log),
		RecoveryUnaryInterceptor(log),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		LoggingStreamInterceptor(log),
		RecoveryStreamInterceptor(log),
	}

	if authInterceptor != nil {
		unaryInterceptors = append(unaryInterceptors, authInterceptor.UnaryInterceptor())
		streamInterceptors = append(streamInterceptors, authInterceptor.StreamInterceptor())
	}

	// Create gRPC server with interceptors and message size limits
	const maxRecvMsgSize = 4 * 1024 * 1024 // 4 MB
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(maxRecvMsgSize),
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	// Register services
	pb.RegisterAgentServiceServer(grpcServer, agentService)
	pb.RegisterModelServiceServer(grpcServer, modelService)
	pb.RegisterModelProviderServiceServer(grpcServer, modelProviderService)
	pb.RegisterAgentToolServiceServer(grpcServer, agentToolService)
	pb.RegisterAgentWorkflowServiceServer(grpcServer, agentWorkflowService)

	// Register health service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	// Enable gRPC server reflection if configured
	if reflectionEnabled {
		reflection.Register(grpcServer)
		log.Info("gRPC server reflection enabled")
	}

	return &Server{
		grpcServer:      grpcServer,
		healthServer:    healthServer,
		port:            port,
		httpPort:        httpPort,
		authCfg:         authCfg,
		log:             log,
		watchClient:     watchClient,
		authInterceptor: authInterceptor,
	}
}

// Start starts both the gRPC server and HTTP gateway.
func (s *Server) Start(ctx context.Context) error {
	// Start gRPC server in a goroutine
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on gRPC port %d: %w", s.port, err)
	}

	// Set serving status
	s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	s.log.Info("Starting gRPC server", "port", s.port)

	go func() {
		if err := s.grpcServer.Serve(grpcListener); err != nil {
			s.log.Error(err, "gRPC server failed")
		}
	}()

	// Start HTTP gateway
	if err := s.startHTTPGateway(ctx); err != nil {
		return err
	}

	// Handle shutdown
	<-ctx.Done()
	s.log.Info("Shutting down servers")
	s.healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	s.grpcServer.GracefulStop()
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(context.Background()); err != nil {
			s.log.Error(err, "Failed to shutdown HTTP server")
		}
	}

	return nil
}

// hasUIFiles checks whether the embedded UI dist directory contains actual built files.
func hasUIFiles() bool {
	entries, err := fs.ReadDir(uiFS.DistFS, "dist")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() != ".gitkeep" {
			return true
		}
	}
	return false
}

// authConfigResponse is the JSON response for the /api/v1alpha1/auth/config endpoint.
type authConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	IssuerURL string `json:"issuerUrl,omitempty"`
	ClientID  string `json:"clientId,omitempty"`
}

// startHTTPGateway starts the HTTP/REST gateway with embedded UI.
func (s *Server) startHTTPGateway(ctx context.Context) error {
	// Create gRPC-Gateway mux.
	// EmitUnpopulated ensures proto3 zero values (0, "", false) are included
	// in JSON responses so the UI always receives a consistent schema.
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions: protojson.MarshalOptions{
				EmitUnpopulated: true,
			},
		}),
	)

	// Connect to gRPC server
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	grpcAddr := fmt.Sprintf("localhost:%d", s.port)

	// Register gateway handlers
	if err := pb.RegisterAgentServiceHandlerFromEndpoint(ctx, gwMux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register agent gateway: %w", err)
	}
	if err := pb.RegisterModelServiceHandlerFromEndpoint(ctx, gwMux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register model gateway: %w", err)
	}
	if err := pb.RegisterModelProviderServiceHandlerFromEndpoint(ctx, gwMux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register model provider gateway: %w", err)
	}
	if err := pb.RegisterAgentToolServiceHandlerFromEndpoint(ctx, gwMux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register agent tool gateway: %w", err)
	}
	if err := pb.RegisterAgentWorkflowServiceHandlerFromEndpoint(ctx, gwMux, grpcAddr, opts); err != nil {
		return fmt.Errorf("failed to register agent workflow gateway: %w", err)
	}

	// Create HTTP mux
	mux := http.NewServeMux()

	// Serve Swagger UI from the generated openapiv2 directory.
	// Clean the path to prevent directory traversal via the environment variable.
	swaggerDir := os.Getenv("SWAGGER_DIR")
	if swaggerDir == "" {
		swaggerDir = "server/gen/openapiv2"
	}
	mux.Handle("/swagger/", http.StripPrefix("/swagger/", http.FileServer(http.Dir(swaggerDir))))

	// Auth configuration endpoint (public — the SPA needs this before it has a token)
	mux.HandleFunc("/api/v1alpha1/auth/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := authConfigResponse{
			Enabled:   s.authCfg.Enabled,
			IssuerURL: s.authCfg.IssuerURL,
			ClientID:  s.authCfg.ClientID,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			s.log.Error(err, "Failed to encode auth config response")
		}
	})

	// Register SSE watch endpoints for real-time UI updates.
	// These must be registered before the catch-all /api/ gateway route
	// because Go 1.22+ ServeMux uses most-specific-pattern-wins matching.
	if s.watchClient != nil {
		registerWatchRoutes(mux, s.watchClient, s.log, authMiddleware(s.authInterceptor))
	}

	// Handle API requests with gateway
	mux.Handle("/api/", gwMux)

	// Health check endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			s.log.Error(err, "Failed to encode health check response")
		}
	})

	// Serve embedded UI or fallback to Swagger UI redirect
	if hasUIFiles() {
		s.log.Info("Serving embedded UI")
		distSubFS, err := fs.Sub(uiFS.DistFS, "dist")
		if err != nil {
			return fmt.Errorf("failed to create sub filesystem for UI: %w", err)
		}
		fileServer := http.FileServer(http.FS(distSubFS))

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// Let /api/, /swagger/, /healthz be handled by their own handlers
			path := r.URL.Path
			if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/swagger/") || path == "/healthz" {
				http.NotFound(w, r)
				return
			}

			// Try to serve the file directly
			// For SPA routing, serve index.html for paths that don't match a static file
			f, err := distSubFS.Open(strings.TrimPrefix(path, "/"))
			if err != nil {
				// File not found - serve index.html for SPA client-side routing
				r.URL.Path = "/"
				fileServer.ServeHTTP(w, r)
				return
			}
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
		})
	} else {
		s.log.Info("No embedded UI found, redirecting to Swagger UI")
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
				return
			}
			http.NotFound(w, r)
		})
	}

	// CORS middleware
	c := cors.New(cors.Options{
		AllowedOrigins:   s.authCfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	// WriteTimeout is set to 0 because SSE watch connections are long-lived.
	// Individual SSE handlers use http.ResponseController to manage write deadlines.
	s.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", s.httpPort),
		Handler:           c.Handler(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	s.log.Info("Starting HTTP gateway", "port", s.httpPort, "swagger-ui", fmt.Sprintf("http://localhost:%d/swagger/", s.httpPort))

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error(err, "HTTP gateway failed")
		}
	}()

	return nil
}

// Stop gracefully stops the server.
func (s *Server) Stop() {
	s.grpcServer.GracefulStop()
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(context.Background()); err != nil {
			s.log.Error(err, "Failed to shutdown HTTP server")
		}
	}
}
