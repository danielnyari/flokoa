/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"
	"path/filepath"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentapp "github.com/danielnyari/flokoa/internal/app/agent"
	"github.com/danielnyari/flokoa/internal/app/agent/compiler"
	triggerapp "github.com/danielnyari/flokoa/internal/app/trigger"
	"github.com/danielnyari/flokoa/internal/controller"
	"github.com/danielnyari/flokoa/internal/infra/builder"
	"github.com/danielnyari/flokoa/internal/infra/repo"
	"github.com/danielnyari/flokoa/internal/spec"
	"github.com/danielnyari/flokoa/internal/telemetry"
	webhookagentv1alpha1 "github.com/danielnyari/flokoa/internal/webhook/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(agentv1alpha1.AddToScheme(scheme))
	utilruntime.Must(wfv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var enableWebhooks bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", false,
		"Enable admission webhooks. Requires TLS certificates (e.g., from cert-manager).")

	var runnerImageRepository string
	flag.StringVar(&runnerImageRepository, "runner-image-repository", builder.DefaultRunnerImageRepository,
		"Generic runner image repository (no tag); the runner version selects the tag.")

	var telemetryOTLPEndpoint string
	flag.StringVar(&telemetryOTLPEndpoint, "telemetry-otlp-endpoint", "",
		"OTLP endpoint configured on runner pods (OTEL_EXPORTER_OTLP_ENDPOINT); empty disables export.")

	var injectTelemetry bool
	flag.BoolVar(&injectTelemetry, "inject-telemetry", true,
		"Inject the flokoa.platform/telemetry capability into every compiled spec. "+
			"Cluster policy only — per-Agent opt-out does not exist by design.")

	var artifactIOEnabled bool
	var artifactGCStrategy string
	flag.BoolVar(&artifactIOEnabled, "artifact-io-enabled", false,
		"Switch AgentWorkflow task I/O from Argo parameters to artifacts backed by object storage.")
	flag.StringVar(&artifactGCStrategy, "artifact-gc-strategy", "OnWorkflowCompletion",
		"Artifact garbage collection strategy when artifact I/O is enabled (e.g., OnWorkflowCompletion, OnWorkflowDeletion).")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Initialize OpenTelemetry distributed tracing.
	otelShutdown, err := telemetry.Init(context.Background(), "flokoa-operator")
	if err != nil {
		setupLog.Error(err, "failed to initialize OpenTelemetry")
		os.Exit(1)
	}
	defer func() {
		if shutdownErr := otelShutdown(context.Background()); shutdownErr != nil {
			setupLog.Error(shutdownErr, "failed to shut down OpenTelemetry")
		}
	}()

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher, webhookCertWatcher *certwatcher.CertWatcher

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		var err error
		webhookCertWatcher, err = certwatcher.New(
			filepath.Join(webhookCertPath, webhookCertName),
			filepath.Join(webhookCertPath, webhookCertKey),
		)
		if err != nil {
			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
			os.Exit(1)
		}

		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
			config.GetCertificate = webhookCertWatcher.GetCertificate
		})
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: webhookTLSOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "fdd0c1f7.flokoa.ai",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Build repository implementations for agent app service.
	k8sClient := mgr.GetClient()
	agentToolRepo := &repo.AgentToolRepoImpl{Client: k8sClient}
	instructionRepo := &repo.InstructionRepoImpl{Client: k8sClient}
	serviceRepo := &repo.ServiceRepoImpl{Client: k8sClient}
	agentAppService := agentapp.NewService(agentapp.Deps{
		AgentTools:    agentToolRepo,
		Models:        &repo.ModelRepoImpl{Client: k8sClient},
		Providers:     &repo.ModelProviderRepoImpl{Client: k8sClient},
		Instructions:  instructionRepo,
		Capabilities:  &repo.CapabilityRepoImpl{Client: k8sClient},
		ConfigMaps:    &repo.ConfigMapRepoImpl{Client: k8sClient},
		Deployments:   &repo.DeploymentRepoImpl{Client: k8sClient},
		Services:      serviceRepo,
		ServiceReader: serviceRepo,
		Secrets:       &repo.SecretRepoImpl{Client: k8sClient},
		OwnerSetter:   &repo.OwnerSetterImpl{Scheme: mgr.GetScheme()},
	}, agentapp.Config{
		DefaultRunnerVersion:  spec.DefaultRunnerVersion,
		RunnerImageRepository: runnerImageRepository,
		Injected:              injectedCapabilities(injectTelemetry),
		OTLPEndpoint:          telemetryOTLPEndpoint,
	})

	if err := (&controller.AgentReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		AppService: agentAppService,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Agent")
		os.Exit(1)
	}
	if err := (&controller.AgentToolReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AgentTool")
		os.Exit(1)
	}
	if err := (&controller.ModelProviderReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ModelProvider")
		os.Exit(1)
	}
	if err := (&controller.ModelReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Model")
		os.Exit(1)
	}
	if err := (&controller.InstructionReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Instruction")
		os.Exit(1)
	}
	if err := (&controller.CapabilityReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Capability")
		os.Exit(1)
	}
	if enableWebhooks {
		if err := webhookagentv1alpha1.SetupAgentWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Agent")
			os.Exit(1)
		}
		if err := webhookagentv1alpha1.SetupCapabilityWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Capability")
			os.Exit(1)
		}
		if err := agentv1alpha1.SetupAgentToolWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AgentTool")
			os.Exit(1)
		}
		if err := agentv1alpha1.SetupModelWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Model")
			os.Exit(1)
		}
		if err := agentv1alpha1.SetupModelProviderWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ModelProvider")
			os.Exit(1)
		}
		if err := agentv1alpha1.SetupInstructionWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "Instruction")
			os.Exit(1)
		}
		if err := webhookagentv1alpha1.SetupAgentWorkflowWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AgentWorkflow")
			os.Exit(1)
		}
		if err := agentv1alpha1.SetupAgentTriggerWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "AgentTrigger")
			os.Exit(1)
		}
	}
	if err := (&controller.AgentWorkflowReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		CompilerOptions: controller.CompilerOptions{
			ArtifactIOEnabled:  artifactIOEnabled,
			ArtifactGCStrategy: artifactGCStrategy,
		},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AgentWorkflow")
		os.Exit(1)
	}

	// Build trigger app service and register AgentTrigger controller.
	triggerAppService := triggerapp.NewService(triggerapp.Deps{
		Agents:      &repo.AgentRepoImpl{Client: k8sClient},
		ConfigMaps:  &repo.ConfigMapRepoImpl{Client: k8sClient},
		Secrets:     &repo.SecretRepoImpl{Client: k8sClient},
		OwnerSetter: &repo.OwnerSetterImpl{Scheme: mgr.GetScheme()},
	})
	if err := (&controller.AgentTriggerReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		AppService: triggerAppService,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AgentTrigger")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	if webhookCertWatcher != nil {
		setupLog.Info("Adding webhook certificate watcher to manager")
		if err := mgr.Add(webhookCertWatcher); err != nil {
			setupLog.Error(err, "unable to add webhook certificate watcher to manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// injectedCapabilities assembles the platform capability entries appended to
// every compiled spec (cluster policy; roadmap 07). The compiler appends them
// after all user entries. session-persistence and budget-guardrail join here
// in P1 (roadmap 13/14).
func injectedCapabilities(telemetryEnabled bool) []compiler.InjectedCapability {
	var injected []compiler.InjectedCapability
	if telemetryEnabled {
		injected = append(injected, compiler.InjectedCapability{Name: "flokoa.platform/telemetry"})
	}
	return injected
}
