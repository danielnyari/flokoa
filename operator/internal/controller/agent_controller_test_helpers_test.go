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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

const (
	testTimeout  = time.Second * 10
	testInterval = time.Millisecond * 250
)

// minimalCard creates a minimal valid AgentCard for testing
func minimalCard() agentv1alpha1.AgentCardOverride {
	return agentv1alpha1.AgentCardOverride{
		Name:        "Test Agent",
		Description: "A test agent",
		Version:     "1.0.0",
		Skills: []agentv1alpha1.AgentSkill{
			{
				ID:          "test-skill",
				Name:        "Test Skill",
				Description: "A test skill",
				Tags:        []string{"test"},
			},
		},
	}
}

// newAgentReconciler creates a new AgentReconciler using the test k8sClient.
func newAgentReconciler() *AgentReconciler {
	return &AgentReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}
}

// reconcileAgent performs the standard two-step reconcile:
// first adds the finalizer, second creates resources.
func reconcileAgent(ctx context.Context, r *AgentReconciler, nn types.NamespacedName) {
	// First reconcile adds finalizer
	result, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, result.RequeueAfter).To(BeNumerically(">", 0))

	// Second reconcile creates resources
	_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

// reconcileOnce performs a single reconcile call (for re-reconcile scenarios).
func reconcileOnce(ctx context.Context, r *AgentReconciler, nn types.NamespacedName) (reconcile.Result, error) {
	return r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
}

// getDeployment fetches the Deployment matching the given NamespacedName, with Eventually.
func getDeployment(ctx context.Context, nn types.NamespacedName) *appsv1.Deployment {
	deployment := &appsv1.Deployment{}
	EventuallyWithOffset(1, func() error {
		return k8sClient.Get(ctx, nn, deployment)
	}, testTimeout, testInterval).Should(Succeed())
	return deployment
}

// getService fetches the Service matching the given NamespacedName, with Eventually.
func getService(ctx context.Context, nn types.NamespacedName) *corev1.Service {
	service := &corev1.Service{}
	EventuallyWithOffset(1, func() error {
		return k8sClient.Get(ctx, nn, service)
	}, testTimeout, testInterval).Should(Succeed())
	return service
}

// getAgent fetches a fresh copy of the Agent.
func getAgent(ctx context.Context, nn types.NamespacedName) *agentv1alpha1.Agent {
	agent := &agentv1alpha1.Agent{}
	ExpectWithOffset(1, k8sClient.Get(ctx, nn, agent)).To(Succeed())
	return agent
}

// getConfigMap fetches a ConfigMap by name and namespace.
func getConfigMap(ctx context.Context, nn types.NamespacedName) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{}
	EventuallyWithOffset(1, func() error {
		return k8sClient.Get(ctx, nn, cm)
	}, testTimeout, testInterval).Should(Succeed())
	return cm
}

// findEnvVar returns the first env var with the given name, or nil.
func findEnvVar(container corev1.Container, name string) *corev1.EnvVar {
	for i := range container.Env {
		if container.Env[i].Name == name {
			return &container.Env[i]
		}
	}
	return nil
}

// findVolume returns the first volume with the given name, or nil.
func findVolume(spec corev1.PodSpec, name string) *corev1.Volume {
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == name {
			return &spec.Volumes[i]
		}
	}
	return nil
}

// findVolumeMount returns the first volume mount with the given name, or nil.
func findVolumeMount(container corev1.Container, name string) *corev1.VolumeMount {
	for i := range container.VolumeMounts {
		if container.VolumeMounts[i].Name == name {
			return &container.VolumeMounts[i]
		}
	}
	return nil
}

// cleanupAgent removes the finalizer (if present) and deletes the Agent, waiting for removal.
func cleanupAgent(ctx context.Context, nn types.NamespacedName) {
	agent := &agentv1alpha1.Agent{}
	err := k8sClient.Get(ctx, nn, agent)
	if errors.IsNotFound(err) {
		return
	}
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
		controllerutil.RemoveFinalizer(agent, agentFinalizer)
		ExpectWithOffset(1, k8sClient.Update(ctx, agent)).To(Succeed())
	}

	ExpectWithOffset(1, k8sClient.Delete(ctx, agent)).To(Succeed())

	EventuallyWithOffset(1, func() bool {
		return errors.IsNotFound(k8sClient.Get(ctx, nn, agent))
	}, testTimeout, testInterval).Should(BeTrue())
}

// firstContainer returns the first container from a deployment's pod template.
func firstContainer(d *appsv1.Deployment) corev1.Container {
	ExpectWithOffset(1, d.Spec.Template.Spec.Containers).NotTo(BeEmpty())
	return d.Spec.Template.Spec.Containers[0]
}
