package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

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

	// Register scheme
	if err := agentv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		log.Error(err, "Failed to add scheme")
		os.Exit(1)
	}

	// Create Kubernetes client
	restConfig := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client")
		os.Exit(1)
	}

	// Create services
	agentService := server.NewAgentService(k8sClient)
	modelService := server.NewModelService(k8sClient)
	modelProviderService := server.NewModelProviderService(k8sClient)
	agentToolService := server.NewAgentToolService(k8sClient)

	// Create and start gRPC server
	grpcServer := server.NewServer(
		cfg.Port,
		cfg.HTTPPort,
		log,
		agentService,
		modelService,
		modelProviderService,
		agentToolService,
	)

	// Handle shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := grpcServer.Start(ctx); err != nil {
		log.Error(err, "Server failed")
		os.Exit(1)
	}
}
