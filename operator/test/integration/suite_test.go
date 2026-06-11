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

// Package integration runs the operator end-to-end without a container
// runtime: a real API server (envtest), the real controller manager (real
// watches — not hand-cranked reconciles), the real e2e fixtures, and the
// real Python runner as a subprocess speaking A2A over HTTP.
//
// This is the Docker-free counterpart of the Kind e2e suite. What it cannot
// cover (and Kind still does): image builds and pulls, kubelet/pod
// scheduling, in-cluster webhook serving via cert-manager, and the Argo
// Workflows integration.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentapp "github.com/danielnyari/flokoa/internal/app/agent"
	"github.com/danielnyari/flokoa/internal/app/agent/compiler"
	"github.com/danielnyari/flokoa/internal/controller"
	"github.com/danielnyari/flokoa/internal/infra/builder"
	"github.com/danielnyari/flokoa/internal/infra/repo"
	"github.com/danielnyari/flokoa/internal/spec"
)

const testNamespace = "integration"

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator Integration Suite (envtest + manager + runner subprocess)")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	Expect(agentv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	By("starting the envtest API server")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	if dir := firstEnvtestBinaryDir(); dir != "" {
		testEnv.BinaryAssetsDirectory = dir
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	Expect(k8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNamespace},
	})).To(Succeed())

	By("starting the controller manager (production wiring)")
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme.Scheme,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		LeaderElection:         false,
	})
	Expect(err).NotTo(HaveOccurred())

	mgrClient := mgr.GetClient()
	agentToolRepo := &repo.AgentToolRepoImpl{Client: mgrClient}
	serviceRepo := &repo.ServiceRepoImpl{Client: mgrClient}
	appService := agentapp.NewService(agentapp.Deps{
		AgentTools:    agentToolRepo,
		Models:        &repo.ModelRepoImpl{Client: mgrClient},
		Providers:     &repo.ModelProviderRepoImpl{Client: mgrClient},
		Instructions:  &repo.InstructionRepoImpl{Client: mgrClient},
		ConfigMaps:    &repo.ConfigMapRepoImpl{Client: mgrClient},
		Deployments:   &repo.DeploymentRepoImpl{Client: mgrClient},
		Services:      serviceRepo,
		ServiceReader: serviceRepo,
		Secrets:       &repo.SecretRepoImpl{Client: mgrClient},
		OwnerSetter:   &repo.OwnerSetterImpl{Scheme: mgr.GetScheme()},
	}, agentapp.Config{
		DefaultRunnerVersion:  spec.DefaultRunnerVersion,
		RunnerImageRepository: builder.DefaultRunnerImageRepository,
		// Mirror the production default: telemetry injected cluster-wide.
		Injected: []compiler.InjectedCapability{{Name: "flokoa.platform/telemetry"}},
	})

	Expect((&controller.AgentReconciler{
		Client: mgrClient, Scheme: mgr.GetScheme(), AppService: appService,
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&controller.AgentToolReconciler{
		Client: mgrClient, Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&controller.ModelReconciler{
		Client: mgrClient, Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&controller.ModelProviderReconciler{
		Client: mgrClient, Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)).To(Succeed())
	Expect((&controller.InstructionReconciler{
		Client: mgrClient, Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr)).To(Succeed())

	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()
	Expect(mgr.GetCache().WaitForCacheSync(ctx)).To(BeTrue())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	if testEnv != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
})

// firstEnvtestBinaryDir mirrors the unit suites: lets the suite run from IDEs
// where KUBEBUILDER_ASSETS isn't exported.
func firstEnvtestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// repoRoot resolves the monorepo root from this package's location.
func repoRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Clean(filepath.Join(wd, "..", "..", ".."))
}

func nn(name string) client.ObjectKey {
	return client.ObjectKey{Name: name, Namespace: testNamespace}
}

func fixturePath(name string) string {
	return filepath.Join("..", "e2e", "testdata", name)
}
