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
	namespace = initializeTestNamespace()
	_, _ = fmt.Fprintf(GinkgoWriter, "Using operator namespace: %s\n", managerNamespace)
	_, _ = fmt.Fprintf(GinkgoWriter, "Using e2e workload namespace: %s\n", namespace)

	By("setting up Kubernetes client")
	var err error
	// Get kubeconfig from default location or KUBECONFIG env var
	kubeconfigPath := clientcmd.RecommendedHomeFile
	if envPath := os.Getenv("KUBECONFIG"); envPath != "" {
		kubeconfigPath = envPath
	}
	cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load kubeconfig")

	// Add our custom schemes
	testScheme = runtime.NewScheme()
	err = scheme.AddToScheme(testScheme)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to add core scheme")
	err = agentv1alpha1.AddToScheme(testScheme)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to add agent scheme")
	err = wfv1.AddToScheme(testScheme)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to add Argo Workflows scheme")

	// Create the client
	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create Kubernetes client")

	By("building the manager(Operator) and server images")
	cmd := exec.Command("make", "docker-build",
		fmt.Sprintf("IMG=%s", projectImage),
		fmt.Sprintf("SERVER_IMG=%s", serverImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) and server images")

	// TODO(user): If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Also, remove the following block.
	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	By("loading the server image on Kind")
	err = utils.LoadImageToKindClusterWithName(serverImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the server image into Kind")

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
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

	// Deploy the operator and server - shared across all tests
	ensureNamespaceReady := func(name string) {
		existingNs := &corev1.Namespace{}
		if err = k8sClient.Get(ctx, client.ObjectKey{Name: name}, existingNs); err == nil {
			if existingNs.Status.Phase == corev1.NamespaceTerminating {
				_, _ = fmt.Fprintf(GinkgoWriter, "Namespace %s is Terminating, waiting for deletion...\n", name)
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKey{Name: name}, &corev1.Namespace{})
				}).WithTimeout(120 * time.Second).WithPolling(2 * time.Second).Should(MatchError(ContainSubstring("not found")))
			}
		}
	}

	By("creating manager and workload namespaces")
	namespaces := []string{managerNamespace}
	if namespace != managerNamespace {
		namespaces = append(namespaces, namespace)
	}
	for _, nsName := range namespaces {
		ensureNamespaceReady(nsName)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nsName,
				Labels: map[string]string{
					"pod-security.kubernetes.io/enforce": "restricted",
				},
			},
		}
		err = k8sClient.Create(ctx, ns)
		if err != nil && !apierrors.IsAlreadyExists(err) {
			ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create namespace")
		}
	}

	By("installing CRDs")
	cmd = exec.Command("make", "install")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("deploying the controller-manager and server")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage), fmt.Sprintf("SERVER_IMG=%s", serverImage))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager and server")
})

var _ = AfterSuite(func() {
	// Clean up the operator deployment
	By("undeploying the controller-manager")
	cmd := exec.Command("make", "undeploy")
	_, _ = utils.Run(cmd)

	By("uninstalling CRDs")
	cmd = exec.Command("make", "uninstall")
	_, _ = utils.Run(cmd)

	By("removing test namespaces")
	for _, nsName := range []string{namespace, managerNamespace} {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
		_ = k8sClient.Delete(ctx, ns)
	}

	// Teardown CertManager after the suite if not skipped and if it was not already installed
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}
})
