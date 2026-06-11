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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentapp "github.com/danielnyari/flokoa/internal/app/agent"
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
	"github.com/danielnyari/flokoa/internal/infra/builder"
	"github.com/danielnyari/flokoa/internal/infra/repo"
	"github.com/danielnyari/flokoa/internal/spec"
)

const (
	testTimeout  = time.Second * 10
	testInterval = time.Millisecond * 250
)

// newAgentReconciler wires an AgentReconciler against the envtest client the
// same way cmd/main.go does in production: repo implementations + the agent
// application service (composition compiler).
func newAgentReconciler() *AgentReconciler {
	serviceRepo := &repo.ServiceRepoImpl{Client: k8sClient}
	appService := agentapp.NewService(agentapp.Deps{
		AgentTools:    &repo.AgentToolRepoImpl{Client: k8sClient},
		Models:        &repo.ModelRepoImpl{Client: k8sClient},
		Providers:     &repo.ModelProviderRepoImpl{Client: k8sClient},
		Instructions:  &repo.InstructionRepoImpl{Client: k8sClient},
		ConfigMaps:    &repo.ConfigMapRepoImpl{Client: k8sClient},
		Deployments:   &repo.DeploymentRepoImpl{Client: k8sClient},
		Services:      serviceRepo,
		ServiceReader: serviceRepo,
		Secrets:       &repo.SecretRepoImpl{Client: k8sClient},
		OwnerSetter:   &repo.OwnerSetterImpl{Scheme: k8sClient.Scheme()},
	}, agentapp.Config{
		DefaultRunnerVersion:  spec.DefaultRunnerVersion,
		RunnerImageRepository: builder.DefaultRunnerImageRepository,
	})
	return &AgentReconciler{
		Client:     k8sClient,
		Scheme:     k8sClient.Scheme(),
		AppService: appService,
	}
}

// reconcileOnce performs a single reconcile pass.
func reconcileOnce(ctx context.Context, r *AgentReconciler, nn types.NamespacedName) (ctrl.Result, error) {
	return r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
}

// reconcileAgent reconciles twice: the first pass adds the finalizer and
// requeues, the second compiles and emits resources.
func reconcileAgent(ctx context.Context, r *AgentReconciler, nn types.NamespacedName) {
	GinkgoHelper()
	_, err := reconcileOnce(ctx, r, nn)
	Expect(err).NotTo(HaveOccurred())
	_, err = reconcileOnce(ctx, r, nn)
	Expect(err).NotTo(HaveOccurred())
}

func getAgent(ctx context.Context, nn types.NamespacedName) *agentv1alpha1.Agent {
	GinkgoHelper()
	agent := &agentv1alpha1.Agent{}
	Expect(k8sClient.Get(ctx, nn, agent)).To(Succeed())
	return agent
}

func getDeployment(ctx context.Context, nn types.NamespacedName) *appsv1.Deployment {
	GinkgoHelper()
	deployment := &appsv1.Deployment{}
	Eventually(func() error {
		return k8sClient.Get(ctx, nn, deployment)
	}, testTimeout, testInterval).Should(Succeed())
	return deployment
}

func getService(ctx context.Context, nn types.NamespacedName) *corev1.Service {
	GinkgoHelper()
	service := &corev1.Service{}
	Eventually(func() error {
		return k8sClient.Get(ctx, nn, service)
	}, testTimeout, testInterval).Should(Succeed())
	return service
}

// getSpecConfigMap fetches the compiled-spec ConfigMap for an agent.
func getSpecConfigMap(ctx context.Context, nn types.NamespacedName) *corev1.ConfigMap {
	GinkgoHelper()
	cm := &corev1.ConfigMap{}
	Eventually(func() error {
		return k8sClient.Get(ctx, types.NamespacedName{
			Name:      builder.SpecConfigMapName(nn.Name),
			Namespace: nn.Namespace,
		}, cm)
	}, testTimeout, testInterval).Should(Succeed())
	return cm
}

// compiledDoc parses the agent-spec.yaml document out of the spec ConfigMap.
func compiledDoc(cm *corev1.ConfigMap) map[string]any {
	GinkgoHelper()
	Expect(cm.Data).To(HaveKey(builder.AgentSpecConfigMapKey))
	doc := map[string]any{}
	Expect(yaml.Unmarshal([]byte(cm.Data[builder.AgentSpecConfigMapKey]), &doc)).To(Succeed())
	return doc
}

func firstContainer(deployment *appsv1.Deployment) corev1.Container {
	GinkgoHelper()
	Expect(deployment.Spec.Template.Spec.Containers).NotTo(BeEmpty())
	return deployment.Spec.Template.Spec.Containers[0]
}

// envByName indexes a container's env vars by name.
func envByName(container corev1.Container) map[string]corev1.EnvVar {
	out := map[string]corev1.EnvVar{}
	for _, e := range container.Env {
		out[e.Name] = e
	}
	return out
}

// minimalCard returns the smallest A2A card the CRD accepts.
func minimalCard() agentv1alpha1.AgentCardOverride {
	return agentv1alpha1.AgentCardOverride{
		Name:        "test-agent",
		Description: "A test agent",
		Version:     "0.1.0",
		Skills:      []agentv1alpha1.AgentSkill{},
	}
}

// minimalAgent builds a valid Agent: an inline fragment with a model plus the
// required card.
func minimalAgent(nn types.NamespacedName) *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nn.Name,
			Namespace: nn.Namespace,
		},
		Spec: agentv1alpha1.AgentSpec{
			Spec: &agentv1alpha1.AgentSpecFragment{
				Model: "openai:gpt-5-mini",
			},
			Card: minimalCard(),
		},
	}
}

// cleanupAgent removes an Agent and its emitted resources. Envtest has no
// garbage collector, so owned objects are deleted explicitly.
func cleanupAgent(ctx context.Context, nn types.NamespacedName) {
	agent := &agentv1alpha1.Agent{}
	if err := k8sClient.Get(ctx, nn, agent); err == nil {
		if controllerutil.ContainsFinalizer(agent, agentFinalizer) {
			controllerutil.RemoveFinalizer(agent, agentFinalizer)
			_ = k8sClient.Update(ctx, agent)
		}
		_ = k8sClient.Delete(ctx, agent)
	}
	_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace}})
	_ = k8sClient.Delete(ctx, &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: nn.Name, Namespace: nn.Namespace}})
	_ = k8sClient.Delete(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
		Name: builder.SpecConfigMapName(nn.Name), Namespace: nn.Namespace,
	}})
}

var _ = Describe("Agent Controller", func() {
	Context("When reconciling an Agent resource", func() {
		var (
			ctx                context.Context
			agentName          string
			typeNamespacedName types.NamespacedName
		)

		const agentNamespace = "default"

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

		Context("Basic reconciliation with an inline fragment", func() {
			It("should compile the spec and create ConfigMap, Deployment, and Service", func() {
				Expect(k8sClient.Create(ctx, minimalAgent(typeNamespacedName))).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying the compiled-spec ConfigMap carries the spec and the card")
				cm := getSpecConfigMap(ctx, typeNamespacedName)
				Expect(cm.Data).To(HaveKey(builder.AgentSpecConfigMapKey))
				Expect(cm.Data).To(HaveKey(builder.AgentCardConfigMapKey))

				doc := compiledDoc(cm)
				Expect(doc["model"]).To(Equal("openai:gpt-5-mini"))
				Expect(doc["name"]).To(Equal(agentName), "spec name defaults to the CR name")

				var card map[string]any
				Expect(json.Unmarshal([]byte(cm.Data[builder.AgentCardConfigMapKey]), &card)).To(Succeed())
				Expect(card["name"]).To(Equal("test-agent"))
				Expect(card["version"]).To(Equal("0.1.0"))

				Expect(cm.Annotations).To(HaveKey("flokoa.ai/spec-hash"))
				Expect(cm.Annotations["flokoa.ai/runner-version"]).To(Equal(spec.DefaultRunnerVersion))
				Expect(cm.Annotations["flokoa.ai/schema-digest"]).To(HavePrefix("sha256:"))

				By("Verifying the Deployment runs the generic runner image")
				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
				container := firstContainer(deployment)
				Expect(container.Name).To(Equal("agent"))
				Expect(container.Image).To(Equal(
					fmt.Sprintf("%s:%s", builder.DefaultRunnerImageRepository, spec.DefaultRunnerVersion),
				))

				By("Verifying the runtime-contract environment")
				env := envByName(container)
				Expect(env).To(HaveKey("FLOKOA_PUBLIC_URL"))
				Expect(env["FLOKOA_PUBLIC_URL"].Value).To(Equal(builder.PublishedURL(agentName, agentNamespace)))
				Expect(env["FLOKOA_EXPECTED_RUNNER_VERSION"].Value).To(Equal(spec.DefaultRunnerVersion))
				Expect(env["FLOKOA_EXPECTED_SCHEMA_DIGEST"].Value).To(HavePrefix("sha256:"))
				Expect(env["OTEL_SERVICE_NAME"].Value).To(Equal(agentName))
				Expect(env["OTEL_RESOURCE_ATTRIBUTES"].Value).To(ContainSubstring("flokoa.agent.name=" + agentName))

				By("Verifying the Service publishes 80 -> 8080")
				service := getService(ctx, typeNamespacedName)
				Expect(service.Spec.Ports).To(HaveLen(1))
				Expect(service.Spec.Ports[0].Port).To(Equal(int32(80)))
				Expect(service.Spec.Ports[0].TargetPort).To(Equal(intstr.FromInt32(8080)))

				By("Verifying status: specHash, runner version, URL, and conditions")
				agent := getAgent(ctx, typeNamespacedName)
				Expect(agent.Status.SpecHash).NotTo(BeEmpty())
				Expect(agent.Status.RunnerVersion).To(Equal(spec.DefaultRunnerVersion))
				Expect(agent.Status.Phase).To(Equal(agentv1alpha1.AgentPhasePending))
				Expect(agent.Status.URL).To(Equal(builder.PublishedURL(agentName, agentNamespace)))
				Expect(agent.Status.ObservedGeneration).To(Equal(agent.Generation))

				specValid := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSpecValid)
				Expect(specValid).NotTo(BeNil())
				Expect(specValid.Status).To(Equal(metav1.ConditionTrue))
				Expect(specValid.Reason).To(Equal(agentdomain.ReasonSpecCompiled))

				ready := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeReady)
				Expect(ready).NotTo(BeNil())
				Expect(ready.Status).To(Equal(metav1.ConditionFalse))
				Expect(ready.Reason).To(Equal(agentdomain.ReasonDeploymentNotReady))

				secretsReady := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeSecretsReady)
				Expect(secretsReady).NotTo(BeNil())
				Expect(secretsReady.Status).To(Equal(metav1.ConditionTrue))

				By("Verifying the pod template carries the spec hash for rollouts")
				Expect(deployment.Spec.Template.Annotations).To(
					HaveKeyWithValue("flokoa.ai/spec-hash", agent.Status.SpecHash),
				)
			})

			It("should add finalizer on first reconcile and requeue", func() {
				Expect(k8sClient.Create(ctx, minimalAgent(typeNamespacedName))).To(Succeed())

				r := newAgentReconciler()
				result, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(time.Second))

				agent := getAgent(ctx, typeNamespacedName)
				Expect(controllerutil.ContainsFinalizer(agent, agentFinalizer)).To(BeTrue())
			})

			It("should handle reconcile request for non-existent Agent gracefully", func() {
				r := newAgentReconciler()
				_, err := reconcileOnce(ctx, r, types.NamespacedName{
					Name: "non-existent-agent", Namespace: agentNamespace,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Deletion handling", func() {
			It("should remove finalizer and allow deletion when DeletionTimestamp is set", func() {
				Expect(k8sClient.Create(ctx, minimalAgent(typeNamespacedName))).To(Succeed())

				r := newAgentReconciler()
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying finalizer is present")
				agent := getAgent(ctx, typeNamespacedName)
				Expect(controllerutil.ContainsFinalizer(agent, agentFinalizer)).To(BeTrue())

				By("Deleting the agent (sets deletion timestamp)")
				Expect(k8sClient.Delete(ctx, agent)).To(Succeed())

				By("Reconciling the deleted agent removes the finalizer")
				_, err = reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					return errors.IsNotFound(k8sClient.Get(ctx, typeNamespacedName, &agentv1alpha1.Agent{}))
				}, testTimeout, testInterval).Should(BeTrue())
			})
		})

		Context("Runtime configuration", func() {
			It("should respect replica count, custom image, resources, and user env", func() {
				replicas := int32(3)
				agent := minimalAgent(typeNamespacedName)
				agent.Spec.Runtime = agentv1alpha1.AgentRuntime{
					Image: "registry.example.com/custom-runner:v9",
					Env: []corev1.EnvVar{
						{Name: "MY_FLAG", Value: "on"},
						// User entries win name conflicts with operator env.
						{Name: "OTEL_SERVICE_NAME", Value: "user-override"},
					},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					DeploymentOverrides: agentv1alpha1.DeploymentOverrides{
						Replicas: &replicas,
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))

				container := firstContainer(deployment)
				Expect(container.Image).To(Equal("registry.example.com/custom-runner:v9"))
				Expect(container.Resources.Limits.Cpu().String()).To(Equal("500m"))
				Expect(container.Resources.Requests.Memory().String()).To(Equal("128Mi"))

				env := envByName(container)
				Expect(env["MY_FLAG"].Value).To(Equal("on"))
				Expect(env["OTEL_SERVICE_NAME"].Value).To(Equal("user-override"))

				names := map[string]int{}
				for _, e := range container.Env {
					names[e.Name]++
				}
				Expect(names["OTEL_SERVICE_NAME"]).To(Equal(1), "conflicting operator env must be dropped, not duplicated")
			})

			It("should pin the runner image tag to spec.runtime.runnerVersion", func() {
				agent := minimalAgent(typeNamespacedName)
				agent.Spec.Runtime.RunnerVersion = spec.DefaultRunnerVersion // only embedded schema versions compile
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(firstContainer(deployment).Image).To(Equal(
					fmt.Sprintf("%s:%s", builder.DefaultRunnerImageRepository, spec.DefaultRunnerVersion),
				))

				agentObj := getAgent(ctx, typeNamespacedName)
				Expect(agentObj.Status.RunnerVersion).To(Equal(spec.DefaultRunnerVersion))
			})
		})

		Context("Labels and owner references", func() {
			It("should label and own all emitted resources", func() {
				Expect(k8sClient.Create(ctx, minimalAgent(typeNamespacedName))).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", agentName))
				Expect(deployment.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "flokoa-operator"))
				Expect(deployment.Labels).To(HaveKeyWithValue("flokoa.ai/agent", agentName))
				Expect(deployment.OwnerReferences).To(HaveLen(1))
				Expect(deployment.OwnerReferences[0].Kind).To(Equal("Agent"))
				Expect(*deployment.OwnerReferences[0].Controller).To(BeTrue())

				service := getService(ctx, typeNamespacedName)
				Expect(service.Spec.Selector).To(HaveKeyWithValue("flokoa.ai/agent", agentName))
				Expect(service.OwnerReferences).To(HaveLen(1))
				Expect(service.OwnerReferences[0].Name).To(Equal(agentName))

				cm := getSpecConfigMap(ctx, typeNamespacedName)
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "agent-spec"))
				Expect(cm.OwnerReferences).To(HaveLen(1))
				Expect(cm.OwnerReferences[0].Name).To(Equal(agentName))
			})
		})

		Context("Composition: ModelRef + InstructionRefs + Tools", func() {
			var (
				suffix          int64
				secretName      string
				providerName    string
				modelName       string
				instr1Name      string
				instr2Name      string
				toolName        string
				cleanupObjects  []client.Object
				registerCleanup func(obj client.Object)
			)

			BeforeEach(func() {
				suffix = time.Now().UnixNano()
				secretName = fmt.Sprintf("openai-key-%d", suffix)
				providerName = fmt.Sprintf("comp-provider-%d", suffix)
				modelName = fmt.Sprintf("comp-model-%d", suffix)
				instr1Name = fmt.Sprintf("comp-instr1-%d", suffix)
				instr2Name = fmt.Sprintf("comp-instr2-%d", suffix)
				toolName = fmt.Sprintf("comp-tool-%d", suffix)
				cleanupObjects = nil
				registerCleanup = func(obj client.Object) {
					cleanupObjects = append(cleanupObjects, obj)
				}
			})

			AfterEach(func() {
				for _, obj := range cleanupObjects {
					_ = k8sClient.Delete(ctx, obj)
				}
			})

			// setupComposition creates a ready Model+Provider chain, two
			// Instructions, and an MCP AgentTool.
			setupComposition := func() {
				GinkgoHelper()

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: agentNamespace},
					Data:       map[string][]byte{"api-key": []byte("sk-test")},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				registerCleanup(secret)

				provider := &agentv1alpha1.ModelProvider{
					ObjectMeta: metav1.ObjectMeta{Name: providerName, Namespace: agentNamespace},
					Spec: agentv1alpha1.ModelProviderSpec{
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
							Key:                  "api-key",
						},
						OpenAI: &agentv1alpha1.OpenAIProviderSpec{},
					},
				}
				Expect(k8sClient.Create(ctx, provider)).To(Succeed())
				registerCleanup(provider)

				providerReconciler := &ModelProviderReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
				_, err := providerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: providerName, Namespace: agentNamespace},
				})
				Expect(err).NotTo(HaveOccurred())

				maxTokens := int32(1024)
				model := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: agentNamespace},
					Spec: agentv1alpha1.ModelSpec{
						Model:       "gpt-5-mini",
						ProviderRef: agentv1alpha1.ProviderRef{Name: providerName},
						Settings: &agentv1alpha1.ModelSettings{
							Temperature: "0.7",
							MaxTokens:   &maxTokens,
						},
					},
				}
				Expect(k8sClient.Create(ctx, model)).To(Succeed())
				registerCleanup(model)

				modelReconciler := &ModelReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
				_, err = modelReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: types.NamespacedName{Name: modelName, Namespace: agentNamespace},
				})
				Expect(err).NotTo(HaveOccurred())

				instr1 := &agentv1alpha1.Instruction{
					ObjectMeta: metav1.ObjectMeta{Name: instr1Name, Namespace: agentNamespace},
					Spec:       agentv1alpha1.InstructionSpec{Content: "You are a support agent."},
				}
				Expect(k8sClient.Create(ctx, instr1)).To(Succeed())
				registerCleanup(instr1)

				instr2 := &agentv1alpha1.Instruction{
					ObjectMeta: metav1.ObjectMeta{Name: instr2Name, Namespace: agentNamespace},
					Spec:       agentv1alpha1.InstructionSpec{Content: "Answer from the knowledge base only."},
				}
				Expect(k8sClient.Create(ctx, instr2)).To(Succeed())
				registerCleanup(instr2)

				tool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{Name: toolName, Namespace: agentNamespace},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeMCP,
						Description: "KB tools",
						URL:         "http://kb-tools.tools.svc.cluster.local:8080/mcp",
					},
				}
				Expect(k8sClient.Create(ctx, tool)).To(Succeed())
				registerCleanup(tool)
			}

			It("should compile model, instructions in order, and MCP tool capability", func() {
				setupComposition()

				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: agentNamespace},
					Spec: agentv1alpha1.AgentSpec{
						ModelRef:        &agentv1alpha1.NamespacedRef{Name: modelName},
						InstructionRefs: []agentv1alpha1.NamespacedRef{{Name: instr1Name}, {Name: instr2Name}},
						Tools:           []agentv1alpha1.NamespacedRef{{Name: toolName}},
						Spec: &agentv1alpha1.AgentSpecFragment{
							Instructions: []string{"Inline instruction last."},
							ModelSettings: &agentv1alpha1.ModelSettings{
								Temperature: "0.1", // inline wins over the Model's 0.7
							},
						},
						Card: minimalCard(),
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				By("Verifying the compiled document")
				cm := getSpecConfigMap(ctx, typeNamespacedName)
				doc := compiledDoc(cm)

				Expect(doc["model"]).To(Equal("openai:gpt-5-mini"), "provider prefix applies to the Model identifier")

				Expect(doc["instructions"]).To(Equal([]any{
					"You are a support agent.",
					"Answer from the knowledge base only.",
					"Inline instruction last.",
				}), "instructionRefs compose in declared order before fragment instructions")

				settings, ok := doc["model_settings"].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(settings["temperature"]).To(BeNumerically("==", 0.1), "inline settings win per-key")
				Expect(settings["max_tokens"]).To(BeNumerically("==", 1024), "non-conflicting Model settings survive")

				caps, ok := doc["capabilities"].([]any)
				Expect(ok).To(BeTrue())
				Expect(caps).To(HaveLen(1))
				mcpEntry, ok := caps[0].(map[string]any)
				Expect(ok).To(BeTrue())
				Expect(mcpEntry).To(HaveKey("MCP"))
				mcp := mcpEntry["MCP"].(map[string]any)
				Expect(mcp["url"]).To(Equal("http://kb-tools.tools.svc.cluster.local:8080/mcp"))
				Expect(mcp["id"]).To(Equal(toolName))

				By("Verifying the provider env projection on the Deployment")
				deployment := getDeployment(ctx, typeNamespacedName)
				env := envByName(firstContainer(deployment))
				Expect(env).To(HaveKey("OPENAI_API_KEY"))
				Expect(env["OPENAI_API_KEY"].ValueFrom.SecretKeyRef.Name).To(Equal(secretName))

				By("Verifying status")
				agentObj := getAgent(ctx, typeNamespacedName)
				Expect(agentObj.Status.SpecHash).NotTo(BeEmpty())

				specValid := meta.FindStatusCondition(agentObj.Status.Conditions, agentdomain.ConditionTypeSpecValid)
				Expect(specValid).NotTo(BeNil())
				Expect(specValid.Status).To(Equal(metav1.ConditionTrue))

				secretsReady := meta.FindStatusCondition(agentObj.Status.Conditions, agentdomain.ConditionTypeSecretsReady)
				Expect(secretsReady).NotTo(BeNil())
				Expect(secretsReady.Status).To(Equal(metav1.ConditionTrue), "the API-key secret exists")
			})

			It("should recompile and change specHash when a referenced Instruction changes", func() {
				setupComposition()

				agent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: agentNamespace},
					Spec: agentv1alpha1.AgentSpec{
						Spec:            &agentv1alpha1.AgentSpecFragment{Model: "openai:gpt-5-mini"},
						InstructionRefs: []agentv1alpha1.NamespacedRef{{Name: instr1Name}},
						Card:            minimalCard(),
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				r := newAgentReconciler()
				reconcileAgent(ctx, r, typeNamespacedName)

				firstHash := getAgent(ctx, typeNamespacedName).Status.SpecHash
				Expect(firstHash).NotTo(BeEmpty())

				By("Editing the Instruction content (fleet propagation)")
				instr := &agentv1alpha1.Instruction{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instr1Name, Namespace: agentNamespace}, instr)).To(Succeed())
				instr.Spec.Content = "You are a billing agent now."
				Expect(k8sClient.Update(ctx, instr)).To(Succeed())

				By("Re-reconciling the agent (the watch would enqueue this in production)")
				_, err := reconcileOnce(ctx, r, typeNamespacedName)
				Expect(err).NotTo(HaveOccurred())

				agentObj := getAgent(ctx, typeNamespacedName)
				Expect(agentObj.Status.SpecHash).NotTo(BeEmpty())
				Expect(agentObj.Status.SpecHash).NotTo(Equal(firstHash), "specHash must change when the composition changes")

				By("Verifying the new content reached the spec ConfigMap and the pod template")
				cm := getSpecConfigMap(ctx, typeNamespacedName)
				Expect(cm.Data[builder.AgentSpecConfigMapKey]).To(ContainSubstring("You are a billing agent now."))

				deployment := getDeployment(ctx, typeNamespacedName)
				Expect(deployment.Spec.Template.Annotations).To(
					HaveKeyWithValue("flokoa.ai/spec-hash", agentObj.Status.SpecHash),
				)
			})
		})

		Context("Watch handlers", func() {
			It("findAgentsForModel should return agents referencing the given Model", func() {
				modelName := fmt.Sprintf("watch-model-%d", time.Now().UnixNano())

				agent := minimalAgent(typeNamespacedName)
				agent.Spec.Spec = nil
				agent.Spec.ModelRef = &agentv1alpha1.NamespacedRef{Name: modelName}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				model := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{Name: modelName, Namespace: agentNamespace},
					Spec: agentv1alpha1.ModelSpec{
						Model:       "gpt-5-mini",
						ProviderRef: agentv1alpha1.ProviderRef{Name: "some-provider"},
					},
				}
				Expect(k8sClient.Create(ctx, model)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, model) }()

				r := newAgentReconciler()
				requests := r.findAgentsForModel(ctx, model)
				Expect(requests).To(ContainElement(reconcile.Request{NamespacedName: typeNamespacedName}))
			})

			It("findAgentsForModel should not return agents referencing a different Model", func() {
				agent := minimalAgent(typeNamespacedName)
				agent.Spec.ModelRef = &agentv1alpha1.NamespacedRef{Name: "other-model"}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				unrelated := &agentv1alpha1.Model{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("unrelated-model-%d", time.Now().UnixNano()),
						Namespace: agentNamespace,
					},
					Spec: agentv1alpha1.ModelSpec{
						Model:       "gpt-5-mini",
						ProviderRef: agentv1alpha1.ProviderRef{Name: "some-provider"},
					},
				}
				Expect(k8sClient.Create(ctx, unrelated)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, unrelated) }()

				r := newAgentReconciler()
				requests := r.findAgentsForModel(ctx, unrelated)
				Expect(requests).NotTo(ContainElement(reconcile.Request{NamespacedName: typeNamespacedName}))
			})

			It("findAgentsForInstruction should return agents with a matching instructionRef", func() {
				instructionName := fmt.Sprintf("watch-instr-%d", time.Now().UnixNano())

				agent := minimalAgent(typeNamespacedName)
				agent.Spec.InstructionRefs = []agentv1alpha1.NamespacedRef{{Name: instructionName}}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				instruction := &agentv1alpha1.Instruction{
					ObjectMeta: metav1.ObjectMeta{Name: instructionName, Namespace: agentNamespace},
					Spec:       agentv1alpha1.InstructionSpec{Content: "Shared instruction content"},
				}
				Expect(k8sClient.Create(ctx, instruction)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, instruction) }()

				r := newAgentReconciler()
				requests := r.findAgentsForInstruction(ctx, instruction)
				Expect(requests).To(ContainElement(reconcile.Request{NamespacedName: typeNamespacedName}))
			})

			It("findAgentsForAgentTool should return agents referencing the tool", func() {
				toolName := fmt.Sprintf("watch-tool-%d", time.Now().UnixNano())

				agent := minimalAgent(typeNamespacedName)
				agent.Spec.Tools = []agentv1alpha1.NamespacedRef{{Name: toolName}}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				tool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{Name: toolName, Namespace: agentNamespace},
					Spec:       agentv1alpha1.AgentToolSpec{URL: "http://mcp.example.com/mcp"},
				}
				Expect(k8sClient.Create(ctx, tool)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, tool) }()

				r := newAgentReconciler()
				requests := r.findAgentsForAgentTool(ctx, tool)
				Expect(requests).To(ContainElement(reconcile.Request{NamespacedName: typeNamespacedName}))
			})

			It("findAgentsForSecret should return agents with a matching secretRef", func() {
				secretName := fmt.Sprintf("watch-secret-%d", time.Now().UnixNano())

				agent := minimalAgent(typeNamespacedName)
				agent.Spec.SecretRefs = map[string]corev1.SecretKeySelector{
					"github-token": {
						LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
						Key:                  "token",
					},
				}
				Expect(k8sClient.Create(ctx, agent)).To(Succeed())

				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: agentNamespace},
					Data:       map[string][]byte{"token": []byte("t")},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, secret) }()

				r := newAgentReconciler()
				requests := r.findAgentsForSecret(ctx, secret)
				Expect(requests).To(ContainElement(reconcile.Request{NamespacedName: typeNamespacedName}))
			})
		})
	})
})
