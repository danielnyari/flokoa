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

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	wfv1 "github.com/argoproj/argo-workflows/v3/pkg/apis/workflow/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/test/utils"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if CertManager is already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false

	// projectImage is the name of the controller image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/operator:v0.0.1"

	// serverImage is the name of the server image which will be build and loaded
	// with the code source changes to be tested.
	serverImage = "example.com/server:v0.0.1"

	// petstoreImage is the non-root petstore image built from test/e2e/testdata/petstore.Dockerfile
	petstoreImage = "petstore:test"

	// runnerImage is the generic runner image agents run on by default.
	// It must match the operator's default runner image repository and
	// pinned runner version (builder.DefaultRunnerImageRepository +
	// spec.DefaultRunnerVersion).
	runnerImage = "ghcr.io/danielnyari/flokoa-runner:0.2.0"

	// k8sClient is the Kubernetes client for interacting with the cluster
	k8sClient client.Client
	// cfg is the rest config for the cluster
	cfg *rest.Config
	// testScheme is the scheme used for the test client
	testScheme *runtime.Scheme
	// ctx is the context for client operations
	ctx context.Context
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purposed to be used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting operator integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	ctx = context.Background()

	By("creating randomized test namespace")
	namespace = initializeTestNamespace()
	_, _ = fmt.Fprintf(GinkgoWriter, "Using namespace: %s\n", namespace)

	By("setting up Kubernetes client")
	var err error
	kubeconfigPath := clientcmd.RecommendedHomeFile
	if envPath := os.Getenv("KUBECONFIG"); envPath != "" {
		kubeconfigPath = envPath
	}
	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load kubeconfig")

	testScheme = runtime.NewScheme()
	err = scheme.AddToScheme(testScheme)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to add core scheme")
	err = agentv1alpha1.AddToScheme(testScheme)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to add agent scheme")
	err = wfv1.AddToScheme(testScheme)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to add Argo Workflows scheme")

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Kubernetes client")

	By("building the manager(Operator) and server images")
	cmd := exec.Command("make", "docker-build",
		fmt.Sprintf("IMG=%s", projectImage),
		fmt.Sprintf("SERVER_IMG=%s", serverImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) and server images")

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	By("loading the server image on Kind")
	err = utils.LoadImageToKindClusterWithName(serverImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the server image into Kind")

	By("building the non-root petstore image for e2e tests")
	cmd = exec.Command("docker", "build",
		"-f", "test/e2e/testdata/petstore.Dockerfile",
		"-t", petstoreImage,
		"test/e2e/testdata/")
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the petstore image")

	By("loading the petstore image on Kind")
	err = utils.LoadImageToKindClusterWithName(petstoreImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the petstore image into Kind")

	By("building the generic runner image for agents")
	cmd = exec.Command("make", "docker-build-flokoa-cli",
		fmt.Sprintf("FLOKOA_CLI_IMG=%s", runnerImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the runner image")

	By("loading the runner image on Kind")
	err = utils.LoadImageToKindClusterWithName(runnerImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the runner image into Kind")

	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}
	}

	// Ensure namespace is ready (wait for terminating ns from previous runs)
	existingNs := &corev1.Namespace{}
	if err = k8sClient.Get(ctx, client.ObjectKey{Name: namespace}, existingNs); err == nil {
		if existingNs.Status.Phase == corev1.NamespaceTerminating {
			_, _ = fmt.Fprintf(GinkgoWriter, "Namespace %s is Terminating, waiting for deletion...\n", namespace)
			existingNs.Spec.Finalizers = nil
			_ = k8sClient.SubResource("finalize").Update(ctx, existingNs)
			Eventually(func() bool {
				getErr := k8sClient.Get(ctx, client.ObjectKey{Name: namespace}, &corev1.Namespace{})
				return apierrors.IsNotFound(getErr)
			}).WithTimeout(120 * time.Second).WithPolling(2 * time.Second).Should(BeTrue())
		}
	}

	By("creating test namespace")
	// Use "baseline" PSS — Argo workflow pods don't comply with "restricted".
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"pod-security.kubernetes.io/enforce": "baseline",
			},
		},
	}
	err = k8sClient.Create(ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create namespace")
	}

	By("installing CRDs")
	cmd = exec.Command("make", "install")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("deploying the controller-manager and server")
	cmd = exec.Command("make", "deploy",
		fmt.Sprintf("IMG=%s", projectImage),
		fmt.Sprintf("SERVER_IMG=%s", serverImage),
		fmt.Sprintf("DEPLOY_NAMESPACE=%s", namespace))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager and server")
})

var _ = AfterSuite(func() {
	if namespace == "" || k8sClient == nil {
		return // BeforeSuite never completed; nothing to clean up
	}

	By("cleaning up cluster-scoped resources")
	deleteClusterRoleBinding(metricsRoleBindingName)

	By("uninstalling Argo Workflows (if installed)")
	if utils.IsArgoWorkflowsInstalled(ctx, k8sClient) {
		utils.UninstallArgoWorkflows(ctx, k8sClient)
	}

	By("undeploying the controller-manager")
	cmd := exec.Command("make", "undeploy", fmt.Sprintf("DEPLOY_NAMESPACE=%s", namespace))
	_, _ = utils.Run(cmd)

	By("uninstalling CRDs")
	cmd = exec.Command("make", "uninstall")
	_, _ = utils.Run(cmd)

	By("deleting the test namespace")
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	if err := k8sClient.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
		_, _ = fmt.Fprintf(GinkgoWriter, "Failed to delete namespace %s: %v\n", namespace, err)
	}

	By("waiting for the test namespace to be fully removed")
	Eventually(func() bool {
		existing := &corev1.Namespace{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: namespace}, existing)
		if apierrors.IsNotFound(err) {
			return true
		}
		// If stuck in Terminating, strip finalizers
		if err == nil && existing.Status.Phase == corev1.NamespaceTerminating {
			_, _ = fmt.Fprintf(GinkgoWriter, "Namespace %s stuck in Terminating, clearing finalizers...\n", namespace)
			existing.Spec.Finalizers = nil
			_ = k8sClient.SubResource("finalize").Update(ctx, existing)
		}
		return false
	}).WithTimeout(120*time.Second).WithPolling(2*time.Second).Should(BeTrue(),
		fmt.Sprintf("Namespace %s was not deleted within 2 minutes", namespace))

	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}

	_, _ = fmt.Fprintf(GinkgoWriter, "Cleanup complete.\n")
})
