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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Agent Controller - AgentCard", func() {
	Context("When reconciling AgentCard ConfigMap", func() {
		const (
			agentNamespace = "default"
			timeout        = time.Second * 10
			interval       = time.Millisecond * 250
		)

		var (
			ctx                context.Context
			agentName          string
			typeNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			// Use unique name per test to avoid conflicts
			agentName = fmt.Sprintf("test-agent-%d", time.Now().UnixNano())
			typeNamespacedName = types.NamespacedName{
				Name:      agentName,
				Namespace: agentNamespace,
			}
		})

		AfterEach(func() {
			// Cleanup the Agent resource
			agent := &agentv1alpha1.Agent{}
			err := k8sClient.Get(ctx, typeNamespacedName, agent)
			if err == nil {
				By("Cleaning up the Agent resource")

				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
					controllerutil.RemoveFinalizer(agent, agentFinalizer)
					Expect(k8sClient.Update(ctx, agent)).To(Succeed())
				}

				Expect(k8sClient.Delete(ctx, agent)).To(Succeed())

				// Wait for deletion to complete
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agent)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}
		})

		It("should create AgentCard ConfigMap with correct JSON data", func() {
			By("Creating an Agent with a Card")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: agentv1alpha1.AgentCardOverride{
						Name:               "Test Agent",
						Description:        "A test agent for unit testing",
						Version:            "1.0.0",
						DefaultInputModes:  []agentv1alpha1.InputOutputMode{agentv1alpha1.InputOutputModeJSON},
						DefaultOutputModes: []agentv1alpha1.InputOutputMode{agentv1alpha1.InputOutputModeText},
						Capabilities: agentv1alpha1.AgentCapabilities{
							Streaming: true,
						},
						Skills: []agentv1alpha1.AgentSkill{
							{
								ID:          "skill-1",
								Name:        "Test Skill",
								Description: "A test skill",
								Tags:        []string{"test", "demo"},
								Examples:    []string{"example 1", "example 2"},
							},
						},
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{
							Container: corev1.Container{
								Name:  "agent",
								Image: "nginx:latest",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			By("Reconciling the Agent")
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile creates resources
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the AgentCard ConfigMap was created")
			configMapName := fmt.Sprintf("%s-agent-card", agentName)
			configMap := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentNamespace,
				}, configMap)
			}, timeout, interval).Should(Succeed())

			By("Verifying ConfigMap labels")
			Expect(configMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", agentName))
			Expect(configMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "agent-card"))
			Expect(configMap.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "flokoa-operator"))
			Expect(configMap.Labels).To(HaveKeyWithValue("flokoa.ai/agent", agentName))

			By("Verifying ConfigMap contains valid JSON")
			Expect(configMap.Data).To(HaveKey("agent-card.json"))
			cardJSON := configMap.Data["agent-card.json"]
			Expect(cardJSON).NotTo(BeEmpty())

			By("Verifying JSON content matches AgentCard")
			var card agentv1alpha1.AgentCardOverride
			err = json.Unmarshal([]byte(cardJSON), &card)
			Expect(err).NotTo(HaveOccurred())

			Expect(card.Name).To(Equal("Test Agent"))
			Expect(card.Description).To(Equal("A test agent for unit testing"))
			Expect(card.Version).To(Equal("1.0.0"))
			Expect(card.DefaultInputModes).To(ContainElement(agentv1alpha1.InputOutputModeJSON))
			Expect(card.DefaultOutputModes).To(ContainElement(agentv1alpha1.InputOutputModeText))
			Expect(card.Capabilities.Streaming).To(BeTrue())
			Expect(card.Skills).To(HaveLen(1))
			Expect(card.Skills[0].ID).To(Equal("skill-1"))
			Expect(card.Skills[0].Name).To(Equal("Test Skill"))
		})

		It("should mount AgentCard ConfigMap in Deployment", func() {
			By("Creating an Agent with a Card")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: agentv1alpha1.AgentCardOverride{
						Name:        "Mount Test Agent",
						Description: "Testing volume mount",
						Version:     "1.0.0",
						Skills:      []agentv1alpha1.AgentSkill{{ID: "test", Name: "Test", Description: "Test skill", Tags: []string{"test"}}},
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{
							Container: corev1.Container{
								Name:  "agent",
								Image: "nginx:latest",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			By("Reconciling the Agent")
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile creates resources
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Deployment has agent-card volume")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, timeout, interval).Should(Succeed())

			var agentCardVolume *corev1.Volume
			for i := range deployment.Spec.Template.Spec.Volumes {
				if deployment.Spec.Template.Spec.Volumes[i].Name == "agent-card" {
					agentCardVolume = &deployment.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(agentCardVolume).NotTo(BeNil())
			Expect(agentCardVolume.ConfigMap.Name).To(Equal(fmt.Sprintf("%s-agent-card", agentName)))

			By("Verifying container has agent-card volume mount")
			container := deployment.Spec.Template.Spec.Containers[0]
			var agentCardMount *corev1.VolumeMount
			for i := range container.VolumeMounts {
				if container.VolumeMounts[i].Name == "agent-card" {
					agentCardMount = &container.VolumeMounts[i]
					break
				}
			}
			Expect(agentCardMount).NotTo(BeNil())
			Expect(agentCardMount.MountPath).To(Equal("/etc/flokoa/agent-card.json"))
			Expect(agentCardMount.SubPath).To(Equal("agent-card.json"))
			Expect(agentCardMount.ReadOnly).To(BeTrue())
		})

		It("should inject FLOKOA_AGENT_URL environment variable", func() {
			By("Creating an Agent")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: agentv1alpha1.AgentCardOverride{
						Name:        "URL Test Agent",
						Description: "Testing URL injection",
						Version:     "1.0.0",
						Skills:      []agentv1alpha1.AgentSkill{{ID: "test", Name: "Test", Description: "Test skill", Tags: []string{"test"}}},
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{
							Container: corev1.Container{
								Name:  "agent",
								Image: "nginx:latest",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			By("Reconciling the Agent")
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile creates resources
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying container has FLOKOA_AGENT_URL env var")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, timeout, interval).Should(Succeed())

			container := deployment.Spec.Template.Spec.Containers[0]
			var agentURLEnv *corev1.EnvVar
			for i := range container.Env {
				if container.Env[i].Name == "FLOKOA_AGENT_URL" {
					agentURLEnv = &container.Env[i]
					break
				}
			}
			Expect(agentURLEnv).NotTo(BeNil())
			expectedURL := fmt.Sprintf("http://%s.%s.svc.cluster.local", agentName, agentNamespace)
			Expect(agentURLEnv.Value).To(Equal(expectedURL))
		})

		It("should update AgentCard ConfigMap when Agent spec changes", func() {
			By("Creating an Agent with initial Card")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: agentv1alpha1.AgentCardOverride{
						Name:        "Original Name",
						Description: "Original description",
						Version:     "1.0.0",
						Skills:      []agentv1alpha1.AgentSkill{{ID: "test", Name: "Test", Description: "Test skill", Tags: []string{"test"}}},
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{
							Container: corev1.Container{
								Name:  "agent",
								Image: "nginx:latest",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			By("Reconciling the Agent")
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile creates resources
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying initial ConfigMap content")
			configMapName := fmt.Sprintf("%s-agent-card", agentName)
			configMap := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      configMapName,
					Namespace: agentNamespace,
				}, configMap)
			}, timeout, interval).Should(Succeed())

			var initialCard agentv1alpha1.AgentCardOverride
			err = json.Unmarshal([]byte(configMap.Data["agent-card.json"]), &initialCard)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialCard.Name).To(Equal("Original Name"))

			By("Updating the Agent Card")
			err = k8sClient.Get(ctx, typeNamespacedName, agent)
			Expect(err).NotTo(HaveOccurred())

			agent.Spec.CardOverride.Name = "Updated Name"
			agent.Spec.CardOverride.Description = "Updated description"
			agent.Spec.CardOverride.Version = "2.0.0"
			Expect(k8sClient.Update(ctx, agent)).To(Succeed())

			By("Reconciling again")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ConfigMap was updated")
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      configMapName,
				Namespace: agentNamespace,
			}, configMap)
			Expect(err).NotTo(HaveOccurred())

			var updatedCard agentv1alpha1.AgentCardOverride
			err = json.Unmarshal([]byte(configMap.Data["agent-card.json"]), &updatedCard)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedCard.Name).To(Equal("Updated Name"))
			Expect(updatedCard.Description).To(Equal("Updated description"))
			Expect(updatedCard.Version).To(Equal("2.0.0"))
		})

		It("should preserve user-defined env vars when adding FLOKOA_AGENT_URL", func() {
			By("Creating an Agent with existing env vars")
			agent := &agentv1alpha1.Agent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: agentNamespace,
				},
				Spec: agentv1alpha1.AgentSpec{
					CardOverride: agentv1alpha1.AgentCardOverride{
						Name:        "Env Test Agent",
						Description: "Testing env preservation",
						Version:     "1.0.0",
						Skills:      []agentv1alpha1.AgentSkill{{ID: "test", Name: "Test", Description: "Test skill", Tags: []string{"test"}}},
					},
					Runtime: agentv1alpha1.RuntimeSpec{
						Type: agentv1alpha1.RuntimeTypeStandard,
						Standard: &agentv1alpha1.StandardRuntimeSpec{
							Container: corev1.Container{
								Name:  "agent",
								Image: "nginx:latest",
								Env: []corev1.EnvVar{
									{Name: "MY_VAR", Value: "my-value"},
									{Name: "ANOTHER_VAR", Value: "another-value"},
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, agent)).To(Succeed())

			By("Reconciling the Agent")
			controllerReconciler := &AgentReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// First reconcile adds finalizer
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			// Second reconcile creates resources
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying all env vars are present")
			deployment := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, deployment)
			}, timeout, interval).Should(Succeed())

			container := deployment.Spec.Template.Spec.Containers[0]
			envNames := make([]string, 0, len(container.Env))
			for _, env := range container.Env {
				envNames = append(envNames, env.Name)
			}

			Expect(envNames).To(ContainElement("MY_VAR"))
			Expect(envNames).To(ContainElement("ANOTHER_VAR"))
			Expect(envNames).To(ContainElement("FLOKOA_AGENT_URL"))
		})
	})
})
