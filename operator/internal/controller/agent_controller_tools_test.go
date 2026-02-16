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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

var _ = Describe("Agent Controller - Tools", func() {
	Context("When reconciling an Agent with tools", func() {
		const agentNamespace = "default"

		var (
			ctx                context.Context
			agentName          string
			typeNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			agentName = fmt.Sprintf("test-agent-%d", time.Now().UnixNano())
			typeNamespacedName = types.NamespacedName{
				Name:      agentName,
				Namespace: agentNamespace,
			}
		})

		AfterEach(func() {
			cleanupAgent(ctx, typeNamespacedName)
		})

		// listAgentToolsByAgent returns AgentTool CRs labeled for the given agent name.
		listAgentToolsByAgent := func(agentName string) []agentv1alpha1.AgentTool {
			list := &agentv1alpha1.AgentToolList{}
			ExpectWithOffset(1, k8sClient.List(ctx, list,
				client.InNamespace(agentNamespace),
				client.MatchingLabels{"flokoa.ai/agent": agentName},
			)).To(Succeed())
			return list.Items
		}

		// cleanupAgentTools removes finalizers and deletes all AgentTools in the namespace
		// matching the given label selector. Pass nil to clean up all AgentTools.
		cleanupAgentTools := func(labels client.MatchingLabels) {
			list := &agentv1alpha1.AgentToolList{}
			opts := []client.ListOption{client.InNamespace(agentNamespace)}
			if labels != nil {
				opts = append(opts, labels)
			}
			_ = k8sClient.List(ctx, list, opts...)
			for _, at := range list.Items {
				if controllerutil.ContainsFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer") {
					controllerutil.RemoveFinalizer(&at, "agent.flokoa.ai/agenttool-finalizer")
					_ = k8sClient.Update(ctx, &at)
				}
				_ = k8sClient.Delete(ctx, &at)
			}
		}

		Context("Inline tools", func() {
			AfterEach(func() {
				cleanupAgentTools(client.MatchingLabels{"flokoa.ai/agent": agentName})
			})

			It("should create AgentTool CR and mount inline tools", func() {
				By("Creating an Agent with inline tools")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								Name: "weather-api",
								Template: &agentv1alpha1.AgentToolSpec{
									Type:        agentv1alpha1.AgentToolTypeOpenAPI,
									Description: "Get weather information",
									OpenApi: &agentv1alpha1.OpenApiToolSpec{
										URL: "https://api.weather.com/v1/weather",
										OpenApiSchema: agentv1alpha1.OpenApiSchema{
											EndpointPath: "/openapi.json",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying AgentTool CR was created for inline tool")
				agentTools := listAgentToolsByAgent(agentName)
				Expect(agentTools).To(HaveLen(1))
				agentTool := agentTools[0]

				Expect(agentTool.Spec.Type).To(Equal(agentv1alpha1.AgentToolTypeOpenAPI))
				Expect(agentTool.Spec.Description).To(Equal("Get weather information"))
				Expect(agentTool.Labels).To(HaveKeyWithValue("flokoa.ai/agent", agentName))
				Expect(agentTool.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "inline-tool"))

				By("Simulating AgentTool controller creating ConfigMap")
				// ConfigMap name follows the convention: "{agenttool-name}-spec"
				configMapName := agentTool.Name + "-spec"
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: agentNamespace,
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi","description":"Get weather information"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Reconciling again to create deployment with volume mounts")
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment has volume mount for the tool")
				deployment := getDeployment(ctx, typeNamespacedName)

				// Check volumes - volume name "tool-weather-api" uses the tool name from test input
				toolVolume := findVolume(deployment.Spec.Template.Spec, "tool-weather-api")
				Expect(toolVolume).NotTo(BeNil())
				Expect(toolVolume.ConfigMap.Name).To(Equal(configMapName))

				// Check volume mounts
				container := firstContainer(deployment)
				toolMount := findVolumeMount(container, "tool-weather-api")
				Expect(toolMount).NotTo(BeNil())
				Expect(toolMount.MountPath).To(Equal("/etc/flokoa/tools/weather-api"))
				Expect(toolMount.ReadOnly).To(BeTrue())

				By("Verifying ToolsReady condition is set")
				Eventually(func() bool {
					agent := getAgent(ctx, typeNamespacedName)
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeToolsReady)
					return condition != nil && condition.Status == metav1.ConditionTrue
				}, testTimeout, testInterval).Should(BeTrue())

				By("Verifying LastToolSync is set")
				agent = getAgent(ctx, typeNamespacedName)
				Expect(agent.Status.LastToolSync).NotTo(BeNil())
			})

			It("should create multiple AgentTool CRs with unique mounts", func() {
				By("Creating an Agent with multiple inline tools")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								Name: "tool-one",
								Template: &agentv1alpha1.AgentToolSpec{
									Type:        agentv1alpha1.AgentToolTypeOpenAPI,
									Description: "First tool",
									OpenApi: &agentv1alpha1.OpenApiToolSpec{
										URL: "https://api.example.com/one",
										OpenApiSchema: agentv1alpha1.OpenApiSchema{
											EndpointPath: "/openapi.json",
										},
									},
								},
							},
							{
								Name: "tool-two",
								Template: &agentv1alpha1.AgentToolSpec{
									Type:        agentv1alpha1.AgentToolTypeOpenAPI,
									Description: "Second tool",
									OpenApi: &agentv1alpha1.OpenApiToolSpec{
										URL: "https://api.example.com/two",
										OpenApiSchema: agentv1alpha1.OpenApiSchema{
											EndpointPath: "/openapi.json",
										},
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying both AgentTool CRs were created")
				agentTools := listAgentToolsByAgent(agentName)
				Expect(agentTools).To(HaveLen(2))

				By("Simulating AgentTool controller creating ConfigMaps")
				for _, at := range agentTools {
					cm := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      at.Name + "-spec",
							Namespace: agentNamespace,
						},
						Data: map[string]string{"spec.json": `{"type":"openapi"}`},
					}
					Expect(k8sClient.Create(ctx, cm)).To(Succeed())
				}

				By("Reconciling again to create deployment with volume mounts")
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment has both volume mounts")
				deployment := getDeployment(ctx, typeNamespacedName)

				container := firstContainer(deployment)
				Expect(container.VolumeMounts).To(HaveLen(3)) // 2 tools + 1 agent-card

				mountPaths := make([]string, 0, 3)
				for _, vm := range container.VolumeMounts {
					mountPaths = append(mountPaths, vm.MountPath)
				}
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/tool-one"))
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/tool-two"))
				Expect(mountPaths).To(ContainElement("/etc/flokoa/agent-card.json"))
			})
		})

		Context("Tool references", func() {
			var agentToolName string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("test-agenttool-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup the AgentTool resource
				agentTool := &agentv1alpha1.AgentTool{}
				err := k8sClient.Get(ctx, types.NamespacedName{Name: agentToolName, Namespace: agentNamespace}, agentTool)
				if err == nil {
					if controllerutil.ContainsFinalizer(agentTool, "agent.flokoa.ai/agenttool-finalizer") {
						controllerutil.RemoveFinalizer(agentTool, "agent.flokoa.ai/agenttool-finalizer")
						_ = k8sClient.Update(ctx, agentTool)
					}
					_ = k8sClient.Delete(ctx, agentTool)
				}
			})

			It("should mount referenced AgentTool ConfigMap", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "External weather API tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.weather.com/v2",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap (simulating AgentTool controller)")
				configMapName := agentToolName + "-spec"
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: agentNamespace,
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi","description":"External weather API tool"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent that references the AgentTool")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying Deployment mounts the referenced AgentTool ConfigMap")
				deployment := getDeployment(ctx, typeNamespacedName)

				// Volume name is "tool-{agentToolName}" where agentToolName is the test variable
				volumeName := "tool-" + agentToolName
				toolVolume := findVolume(deployment.Spec.Template.Spec, volumeName)
				Expect(toolVolume).NotTo(BeNil())
				Expect(toolVolume.ConfigMap.Name).To(HaveSuffix("-spec"))
				Expect(toolVolume.ConfigMap.Name).To(Equal(configMapName))

				// Check volume mounts
				container := firstContainer(deployment)
				toolMount := findVolumeMount(container, volumeName)
				Expect(toolMount).NotTo(BeNil())
				Expect(toolMount.MountPath).To(HavePrefix("/etc/flokoa/tools/"))
				Expect(toolMount.MountPath).To(ContainSubstring(agentToolName))
				Expect(toolMount.ReadOnly).To(BeTrue())
			})

			It("should fail when referenced AgentTool does not exist", func() {
				By("Creating an Agent that references a non-existent AgentTool")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: "non-existent-tool",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				r := newAgentReconciler()

				// First reconcile adds finalizer
				result, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile should fail due to missing AgentTool
				_, err = reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("non-existent-tool"))

				By("Verifying ToolsReady condition shows failure")
				Eventually(func() bool {
					agent := getAgent(ctx, typeNamespacedName)
					condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeToolsReady)
					return condition != nil && condition.Status == metav1.ConditionFalse
				}, testTimeout, testInterval).Should(BeTrue())
			})

			It("should use custom name when specified in ToolRef", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Some API tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap")
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName + "-spec",
						Namespace: agentNamespace,
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent with a custom name for the tool reference")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								Name: "my-custom-tool-name",
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying volume mount uses custom name")
				deployment := getDeployment(ctx, typeNamespacedName)

				container := firstContainer(deployment)
				toolMount := findVolumeMount(container, "tool-my-custom-tool-name")
				Expect(toolMount).NotTo(BeNil())
				Expect(toolMount.MountPath).To(Equal("/etc/flokoa/tools/my-custom-tool-name"))
			})
		})

		Context("Mixed inline and referenced tools", func() {
			var agentToolName string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("test-agenttool-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				cleanupAgentTools(nil)
			})

			It("should handle both inline and referenced tools together", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Referenced tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com/ref",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap")
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName + "-spec",
						Namespace: agentNamespace,
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent with both inline and referenced tools")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								Name: "inline-tool",
								Template: &agentv1alpha1.AgentToolSpec{
									Type:        agentv1alpha1.AgentToolTypeOpenAPI,
									Description: "Inline tool",
									OpenApi: &agentv1alpha1.OpenApiToolSpec{
										URL: "https://api.example.com/inline",
										OpenApiSchema: agentv1alpha1.OpenApiSchema{
											EndpointPath: "/openapi.json",
										},
									},
								},
							},
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying AgentTool CR was created for inline tool")
				inlineTools := listAgentToolsByAgent(agentName)
				Expect(inlineTools).To(HaveLen(1))
				inlineAgentTool := inlineTools[0]

				By("Simulating AgentTool controller creating ConfigMap for inline tool")
				inlineConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      inlineAgentTool.Name + "-spec",
						Namespace: agentNamespace,
					},
					Data: map[string]string{"spec.json": `{"type":"openapi"}`},
				}
				Expect(k8sClient.Create(ctx, inlineConfigMap)).To(Succeed())

				By("Reconciling again to create deployment with volume mounts")
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Deployment has both volume mounts")
				deployment := getDeployment(ctx, typeNamespacedName)

				container := firstContainer(deployment)
				Expect(container.VolumeMounts).To(HaveLen(3)) // 2 tools + 1 agent-card

				mountPaths := make([]string, 0, 3)
				for _, vm := range container.VolumeMounts {
					mountPaths = append(mountPaths, vm.MountPath)
				}
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/inline-tool"))
				Expect(mountPaths).To(ContainElement("/etc/flokoa/tools/" + agentToolName))
				Expect(mountPaths).To(ContainElement("/etc/flokoa/agent-card.json"))

				By("Verifying ToolsReady condition shows 2 tools synced")
				agent = getAgent(ctx, typeNamespacedName)
				condition := meta.FindStatusCondition(agent.Status.Conditions, ConditionTypeToolsReady)
				Expect(condition).NotTo(BeNil())
				Expect(condition.Message).To(ContainSubstring("2 tools"))
			})
		})

		Context("Tool change propagation", func() {
			var agentToolName string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("test-agenttool-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				cleanupAgentTools(nil)
			})

			It("should add tools-hash annotation to pod template", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Test tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap")
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName + "-spec",
						Namespace: agentNamespace,
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi","description":"Test tool"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent that references the AgentTool")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying pod template has tools-hash annotation")
				deployment := getDeployment(ctx, typeNamespacedName)

				Expect(deployment.Spec.Template.Annotations).To(HaveKey("flokoa.ai/tools-hash"))
				initialHash := deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
				Expect(initialHash).NotTo(BeEmpty())
			})

			It("should update tools-hash when ConfigMap changes", func() {
				By("Creating an AgentTool resource")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Original description",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap")
				configMapNN := types.NamespacedName{
					Name:      agentToolName + "-spec",
					Namespace: agentNamespace,
				}
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapNN.Name,
						Namespace: configMapNN.Namespace,
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi","description":"Original description"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent that references the AgentTool")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Name:  "agent",
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Reconciling the Agent")
				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Getting the initial tools-hash")
				deployment := getDeployment(ctx, typeNamespacedName)
				initialHash := deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
				Expect(initialHash).NotTo(BeEmpty())

				By("Updating the ConfigMap content")
				Expect(k8sClient.Get(ctx, configMapNN, toolConfigMap)).To(Succeed())
				toolConfigMap.Data["spec.json"] = `{"type":"openapi","description":"Updated description"}`
				Expect(k8sClient.Update(ctx, toolConfigMap)).To(Succeed())

				By("Reconciling the Agent again")
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying the tools-hash has changed")
				Expect(k8sClient.Get(ctx, typeNamespacedName, deployment)).To(Succeed())
				updatedHash := deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
				Expect(updatedHash).NotTo(BeEmpty())
				Expect(updatedHash).NotTo(Equal(initialHash))
			})

			It("should not have tools-hash annotation when no tools are configured", func() {
				By("Creating an Agent without tools")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
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
				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying pod template has no tools-hash annotation")
				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(deployment.Spec.Template.Annotations).To(BeNil())
			})

			It("should produce same hash for same ConfigMap data", func() {
				By("Creating two ConfigMaps with identical data")
				data1 := map[string]string{
					"spec.json": `{"type":"openapi","description":"Test"}`,
					"other.txt": "some content",
				}
				data2 := map[string]string{
					"other.txt": "some content",
					"spec.json": `{"type":"openapi","description":"Test"}`,
				}

				hash1 := hashConfigMapData(data1)
				hash2 := hashConfigMapData(data2)

				Expect(hash1).To(Equal(hash2))
				Expect(hash1).NotTo(BeEmpty())
			})

			It("should produce different hash for different ConfigMap data", func() {
				By("Creating two ConfigMaps with different data")
				data1 := map[string]string{
					"spec.json": `{"type":"openapi","description":"Original"}`,
				}
				data2 := map[string]string{
					"spec.json": `{"type":"openapi","description":"Modified"}`,
				}

				hash1 := hashConfigMapData(data1)
				hash2 := hashConfigMapData(data2)

				Expect(hash1).NotTo(Equal(hash2))
			})

			It("should return empty hash for empty ConfigMap data", func() {
				hash := hashConfigMapData(map[string]string{})
				Expect(hash).To(BeEmpty())

				hash = hashConfigMapData(nil)
				Expect(hash).To(BeEmpty())
			})
		})

		Context("findAgentsForAgentTool handler", func() {
			var agentToolName string
			var agent1Name string
			var agent2Name string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("shared-tool-%d", time.Now().UnixNano())
				agent1Name = fmt.Sprintf("agent1-%d", time.Now().UnixNano())
				agent2Name = fmt.Sprintf("agent2-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup agents
				for _, name := range []string{agent1Name, agent2Name} {
					cleanupAgent(ctx, types.NamespacedName{Name: name, Namespace: agentNamespace})
				}

				// Cleanup AgentTools
				cleanupAgentTools(nil)
			})

			It("should find all agents referencing an AgentTool", func() {
				By("Creating a shared AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Shared tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating two agents that reference the same tool")
				for _, name := range []string{agent1Name, agent2Name} {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Image: "nginx:latest",
									},
								},
							},
							Tools: []agentv1alpha1.ToolEntry{
								{
									ToolRef: &agentv1alpha1.ToolRef{
										Name: agentToolName,
									},
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())
				}

				By("Calling findAgentsForAgentTool")
				r := newAgentReconciler()
				requests := r.findAgentsForAgentTool(ctx, agentTool)

				By("Verifying both agents are returned")
				Expect(requests).To(HaveLen(2))

				names := []string{requests[0].Name, requests[1].Name}
				Expect(names).To(ContainElement(agent1Name))
				Expect(names).To(ContainElement(agent2Name))
			})

			It("should find agent for inline tool via label", func() {
				By("Creating an inline AgentTool with agent label")
				inlineAgentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-inline-tool", agent1Name),
						Namespace: agentNamespace,
						Labels: map[string]string{
							"flokoa.ai/agent": agent1Name,
						},
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Inline tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, inlineAgentTool)).To(Succeed())

				By("Calling findAgentsForAgentTool")
				r := newAgentReconciler()
				requests := r.findAgentsForAgentTool(ctx, inlineAgentTool)

				By("Verifying the owning agent is returned")
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal(agent1Name))
			})

			It("should return empty for unreferenced AgentTool", func() {
				By("Creating an AgentTool that no agent references")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Unreferenced tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Calling findAgentsForAgentTool")
				r := newAgentReconciler()
				requests := r.findAgentsForAgentTool(ctx, agentTool)

				By("Verifying no agents are returned")
				Expect(requests).To(BeEmpty())
			})
		})

		Context("findAgentsForConfigMap handler", func() {
			var agentToolName string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("cm-tool-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				cleanupAgentTools(nil)
			})

			It("should find agent for agenttool-spec ConfigMap", func() {
				By("Creating an AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Test tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap with proper labels")
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName + "-spec",
						Namespace: agentNamespace,
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating an Agent that references the tool")
				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentSpec{
						CardOverride: minimalCard(),
						Runtime: agentv1alpha1.RuntimeSpec{
							Type: agentv1alpha1.RuntimeTypeStandard,
							Standard: &agentv1alpha1.StandardRuntimeSpec{
								Container: corev1.Container{
									Image: "nginx:latest",
								},
							},
						},
						Tools: []agentv1alpha1.ToolEntry{
							{
								ToolRef: &agentv1alpha1.ToolRef{
									Name: agentToolName,
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				By("Calling findAgentsForConfigMap")
				r := newAgentReconciler()
				requests := r.findAgentsForConfigMap(ctx, toolConfigMap)

				By("Verifying the agent is returned")
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal(agentName))
			})

			It("should find agent for inline-tool-spec ConfigMap via label", func() {
				By("Creating an inline tool ConfigMap with agent label")
				inlineConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentName + "-inline-tool-spec",
						Namespace: agentNamespace,
						Labels: map[string]string{
							"app.kubernetes.io/component": "inline-tool-spec",
							"flokoa.ai/agent":             agentName,
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi"}`,
					},
				}
				Expect(k8sClient.Create(ctx, inlineConfigMap)).To(Succeed())

				By("Calling findAgentsForConfigMap")
				r := newAgentReconciler()
				requests := r.findAgentsForConfigMap(ctx, inlineConfigMap)

				By("Verifying the agent is returned")
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Name).To(Equal(agentName))
			})

			It("should ignore ConfigMaps without tool component label", func() {
				By("Creating a ConfigMap without tool labels")
				regularConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "regular-configmap",
						Namespace: agentNamespace,
					},
					Data: map[string]string{
						"config": "some value",
					},
				}
				Expect(k8sClient.Create(ctx, regularConfigMap)).To(Succeed())

				By("Calling findAgentsForConfigMap")
				r := newAgentReconciler()
				requests := r.findAgentsForConfigMap(ctx, regularConfigMap)

				By("Verifying no agents are returned")
				Expect(requests).To(BeEmpty())
			})
		})

		Context("Multiple agents with shared tool", func() {
			var agentToolName string
			var agent1Name string
			var agent2Name string

			BeforeEach(func() {
				agentToolName = fmt.Sprintf("shared-tool-%d", time.Now().UnixNano())
				agent1Name = fmt.Sprintf("agent1-%d", time.Now().UnixNano())
				agent2Name = fmt.Sprintf("agent2-%d", time.Now().UnixNano())
			})

			AfterEach(func() {
				// Cleanup agents
				for _, name := range []string{agent1Name, agent2Name} {
					cleanupAgent(ctx, types.NamespacedName{Name: name, Namespace: agentNamespace})
				}

				// Cleanup AgentTools
				cleanupAgentTools(nil)
			})

			It("should update all agents when shared tool ConfigMap changes", func() {
				By("Creating a shared AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Shared tool",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Creating the AgentTool's ConfigMap")
				configMapNN := types.NamespacedName{
					Name:      agentToolName + "-spec",
					Namespace: agentNamespace,
				}
				toolConfigMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapNN.Name,
						Namespace: configMapNN.Namespace,
						Labels: map[string]string{
							"app.kubernetes.io/name":      agentToolName,
							"app.kubernetes.io/component": "agenttool-spec",
						},
					},
					Data: map[string]string{
						"spec.json": `{"type":"openapi","description":"Original"}`,
					},
				}
				Expect(k8sClient.Create(ctx, toolConfigMap)).To(Succeed())

				By("Creating two agents referencing the shared tool")
				r := newAgentReconciler()

				initialHashes := make(map[string]string)

				for _, name := range []string{agent1Name, agent2Name} {
					agent := &agentv1alpha1.Agent{
						ObjectMeta: metav1.ObjectMeta{
							Name:      name,
							Namespace: agentNamespace,
						},
						Spec: agentv1alpha1.AgentSpec{
							CardOverride: minimalCard(),
							Runtime: agentv1alpha1.RuntimeSpec{
								Type: agentv1alpha1.RuntimeTypeStandard,
								Standard: &agentv1alpha1.StandardRuntimeSpec{
									Container: corev1.Container{
										Image: "nginx:latest",
									},
								},
							},
							Tools: []agentv1alpha1.ToolEntry{
								{
									ToolRef: &agentv1alpha1.ToolRef{
										Name: agentToolName,
									},
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, agent)).To(Succeed())

					nn := types.NamespacedName{Name: name, Namespace: agentNamespace}
					reconcileAgent(ctx, r, nn)

					// Get initial hash
					deployment := getDeployment(ctx, nn)
					initialHashes[name] = deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
				}

				By("Updating the shared ConfigMap")
				Expect(k8sClient.Get(ctx, configMapNN, toolConfigMap)).To(Succeed())
				toolConfigMap.Data["spec.json"] = `{"type":"openapi","description":"Updated"}`
				Expect(k8sClient.Update(ctx, toolConfigMap)).To(Succeed())

				By("Reconciling both agents")
				for _, name := range []string{agent1Name, agent2Name} {
					nn := types.NamespacedName{Name: name, Namespace: agentNamespace}
					_, err := reconcileOnce(ctx, r, nn)
					Expect(err).NotTo(HaveOccurred())
				}

				By("Verifying both agents have updated hashes")
				for _, name := range []string{agent1Name, agent2Name} {
					nn := types.NamespacedName{Name: name, Namespace: agentNamespace}
					deployment := getDeployment(ctx, nn)
					newHash := deployment.Spec.Template.Annotations["flokoa.ai/tools-hash"]
					Expect(newHash).NotTo(Equal(initialHashes[name]))
				}
			})
		})
	})
})
