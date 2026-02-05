package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "github.com/danielnyari/flokoa/server/gen/go/flokoa/agent/v1alpha1"
)

// Server wraps the gRPC server and its configuration.
type Server struct {
	grpcServer   *grpc.Server
	httpServer   *http.Server
	healthServer *health.Server
	port         int
	httpPort     int
	log          logr.Logger
}

// NewServer creates a new gRPC server with reflection enabled.
func NewServer(
	port int,
	httpPort int,
	log logr.Logger,
	agentService pb.AgentServiceServer,
	modelService pb.ModelServiceServer,
	modelProviderService pb.ModelProviderServiceServer,
	agentToolService pb.AgentToolServiceServer,
) *Server {
	// Create gRPC server with interceptors
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			LoggingUnaryInterceptor(log),
			RecoveryUnaryInterceptor(log),
		),
		grpc.ChainStreamInterceptor(
			LoggingStreamInterceptor(log),
			RecoveryStreamInterceptor(log),
		),
	)

	// Register services
	pb.RegisterAgentServiceServer(grpcServer, agentService)
	pb.RegisterModelServiceServer(grpcServer, modelService)
	pb.RegisterModelProviderServiceServer(grpcServer, modelProviderService)
	pb.RegisterAgentToolServiceServer(grpcServer, agentToolService)

	// Register health service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	// Enable gRPC reflection
	reflection.Register(grpcServer)

	return &Server{
		grpcServer:   grpcServer,
		healthServer: healthServer,
		port:         port,
		httpPort:     httpPort,
		log:          log,
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
		s.httpServer.Shutdown(context.Background())
	}

	return nil
}

// startHTTPGateway starts the HTTP/REST gateway with Swagger UI.
func (s *Server) startHTTPGateway(ctx context.Context) error {
	// Create gRPC-Gateway mux
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{}),
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

	// Create HTTP mux
	mux := http.NewServeMux()

	// Serve Swagger UI from the generated openapiv2 directory
	swaggerDir := os.Getenv("SWAGGER_DIR")
	if swaggerDir == "" {
		swaggerDir = "server/gen/openapiv2"
	}
	mux.Handle("/swagger/", http.StripPrefix("/swagger/", http.FileServer(http.Dir(swaggerDir))))

	// Handle API requests with gateway
	mux.Handle("/api/", gwMux)

	// Health check endpoint
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Root redirect to Swagger UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/swagger/", http.StatusMovedPermanently)
			return
		}
		http.NotFound(w, r)
	})

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.httpPort),
		Handler: corsMiddleware(mux),
	}

	s.log.Info("Starting HTTP gateway with Swagger UI", "port", s.httpPort, "swagger-ui", fmt.Sprintf("http://localhost:%d/swagger/", s.httpPort))

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error(err, "HTTP gateway failed")
		}
	}()

	return nil
}

// corsMiddleware adds CORS headers for development.
func corsMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// Stop gracefully stops the server.
func (s *Server) Stop() {
	s.grpcServer.GracefulStop()
	if s.httpServer != nil {
		s.httpServer.Shutdown(context.Background())
	}
}
