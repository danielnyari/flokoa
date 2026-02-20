package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/config"
	"github.com/danielnyari/flokoa/internal/server"
)

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("server")

	// Load configuration
	cfg := config.LoadServerConfig()

	// Register schemes
	if err := agentv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add Flokoa scheme")
		os.Exit(1)
	}
	if err := wfv1.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add Argo Workflows scheme")
		os.Exit(1)
	}

	// Create Kubernetes client
	restConfig := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client")
		os.Exit(1)
	}

	// Handle shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize auth interceptor if enabled
	var authInterceptor *server.AuthInterceptor
	if cfg.Auth.Enabled {
		if cfg.Auth.IssuerURL == "" {
			log.Error(nil, "AUTH_OIDC_ISSUER_URL is required when auth is enabled")
			os.Exit(1)
		}
		log.Info("Initializing OIDC auth", "issuer", cfg.Auth.IssuerURL, "clientID", cfg.Auth.ClientID)

		var initErr error
		authInterceptor, initErr = server.NewAuthInterceptor(ctx, server.AuthInterceptorConfig{
			IssuerURL:     cfg.Auth.IssuerURL,
			Audience:      cfg.Auth.ClientID,
			PublicMethods: server.PublicGRPCMethods,
		}, log.WithName("auth"))
		if initErr != nil {
			log.Error(initErr, "Failed to initialize OIDC auth interceptor")
			os.Exit(1)
		}
		log.Info("OIDC auth initialized successfully")
	} else {
		log.Info("Auth disabled, all endpoints are public")
	}

	// Create services
	agentService := server.NewAgentService(k8sClient)
	modelService := server.NewModelService(k8sClient)
	modelProviderService := server.NewModelProviderService(k8sClient)
	agentToolService := server.NewAgentToolService(k8sClient)
	agentWorkflowService := server.NewAgentWorkflowService(k8sClient)

	// Create and start gRPC server
	grpcServer := server.NewServer(
		cfg.Port,
		cfg.HTTPPort,
		cfg.Auth,
		log,
		authInterceptor,
		agentService,
		modelService,
		modelProviderService,
		agentToolService,
		agentWorkflowService,
	)

	if err := grpcServer.Start(ctx); err != nil {
		log.Error(err, "Server failed")
		os.Exit(1)
	}
}
