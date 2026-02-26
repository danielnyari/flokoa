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
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
)

// mockOpenAPITransport returns a minimal OpenAPI spec for any HTTP request.
// Used in tests where endpointPath triggers an HTTP fetch during resolveSpec.
type mockOpenAPITransport struct{}

func (t *mockOpenAPITransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	spec := `{"openapi":"3.0.0","info":{"title":"Test API","version":"1.0"},"paths":{}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(spec)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func mockHTTPClient() *http.Client {
	return &http.Client{Transport: &mockOpenAPITransport{}}
}

// toolDefinitionFromConfigMap extracts the spec from the ToolDefinition wrapper in the ConfigMap.
func toolDefinitionFromConfigMap(cm *corev1.ConfigMap) (string, agentv1alpha1.AgentToolSpec) {
	var td struct {
		Name string                      `json:"name"`
		Spec agentv1alpha1.AgentToolSpec `json:"spec"`
	}
	ExpectWithOffset(1, cm.Data).To(HaveKey("spec.json"))
	err := json.Unmarshal([]byte(cm.Data["spec.json"]), &td)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return td.Name, td.Spec
}

var _ = Describe("AgentTool Controller", func() {
	Context("When reconciling an AgentTool resource", func() {
		const (
			agentToolNamespace = "default"
			timeout            = time.Second * 10
			interval           = time.Millisecond * 250
		)

		var (
			ctx                context.Context
			agentToolName      string
			typeNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			ctx = context.Background()
			// Use unique name per test to avoid conflicts
			agentToolName = fmt.Sprintf("test-tool-%d", time.Now().UnixNano())
			typeNamespacedName = types.NamespacedName{
				Name:      agentToolName,
				Namespace: agentToolNamespace,
			}
		})

		AfterEach(func() {
			// Cleanup the AgentTool resource
			agentTool := &agentv1alpha1.AgentTool{}
			err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
			if err == nil {
				By("Cleaning up the AgentTool resource")

				// Remove finalizer if present to allow deletion
				if controllerutil.ContainsFinalizer(agentTool, agentToolFinalizer) {
					controllerutil.RemoveFinalizer(agentTool, agentToolFinalizer)
					Expect(k8sClient.Update(ctx, agentTool)).To(Succeed())
				}

				Expect(k8sClient.Delete(ctx, agentTool)).To(Succeed())

				// Wait for deletion to complete
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			}

			// Cleanup any ConfigMaps created
			cmName := fmt.Sprintf("%s-spec", agentToolName)
			cm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
			if err == nil {
				_ = k8sClient.Delete(ctx, cm)
			}
		})

		Context("Basic reconciliation", func() {
			It("should add finalizer and create ConfigMap for a new AgentTool", func() {
				By("Creating a new AgentTool resource")
				timeoutSeconds := int32(30)
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Test tool for fetching weather data",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.weather.com/v1",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
							TimeoutSeconds: &timeoutSeconds,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool to add finalizer")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds the finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				By("Verifying finalizer was added")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					if err != nil {
						return false
					}
					return controllerutil.ContainsFinalizer(agentTool, agentToolFinalizer)
				}, timeout, interval).Should(BeTrue())

				By("Reconciling the AgentTool again to create ConfigMap")
				// Second reconcile creates the ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying the ConfigMap was created")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				Expect(cm.Data).To(HaveKey("spec.json"))
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", agentToolName))
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "agenttool-spec"))
				Expect(cm.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "flokoa-operator"))

				By("Verifying ConfigMap contains valid ToolDefinition JSON")
				name, spec := toolDefinitionFromConfigMap(cm)
				Expect(name).To(Equal(agentToolName))
				Expect(spec.Type).To(Equal(agentv1alpha1.AgentToolTypeOpenAPI))
				Expect(spec.Description).To(Equal("Test tool for fetching weather data"))
				Expect(spec.OpenApi.URL).To(Equal("https://api.weather.com/v1"))
				// endpointPath is resolved: value should now be populated
				Expect(spec.OpenApi.OpenApiSchema.Value).NotTo(BeNil())
			})

			It("should handle reconcile request for non-existent AgentTool", func() {
				By("Reconciling a non-existent AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				nonExistentName := types.NamespacedName{
					Name:      "non-existent-tool",
					Namespace: agentToolNamespace,
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: nonExistentName,
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("OpenAPI schema source validation", func() {
			It("should validate and accept endpointPath schema source", func() {
				By("Creating an AgentTool with endpointPath schema source")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Weather API with endpoint path schema",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.weather.com/v1",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/docs/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile validates schema and creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Validated condition is True")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					if err != nil {
						return false
					}
					condition := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeValidated)
					return condition != nil && condition.Status == metav1.ConditionTrue
				}, timeout, interval).Should(BeTrue())

				condition := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeValidated)
				Expect(condition.Reason).To(Equal(ReasonValidationSuccess))
			})

			It("should validate and accept inline value schema source", func() {
				By("Creating an AgentTool with inline value schema")
				openApiSpec := map[string]interface{}{
					"openapi": "3.0.0",
					"info": map[string]interface{}{
						"title":   "Weather API",
						"version": "1.0",
					},
					"paths": map[string]interface{}{},
				}
				specBytes, _ := json.Marshal(openApiSpec)

				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Weather API with inline schema",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.weather.com/v1",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								Value: &runtime.RawExtension{Raw: specBytes},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile validates and creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Validated condition is True")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					if err != nil {
						return false
					}
					condition := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeValidated)
					return condition != nil && condition.Status == metav1.ConditionTrue
				}, timeout, interval).Should(BeTrue())

				By("Verifying ConfigMap contains inline value")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				_, spec := toolDefinitionFromConfigMap(cm)
				Expect(spec.OpenApi.OpenApiSchema.Value).NotTo(BeNil())
			})

			It("should validate OpenAPI schema from existing ConfigMap (valueFrom)", func() {
				By("Creating a ConfigMap with OpenAPI spec as JSON")
				openApiJSON := `{"openapi":"3.0.0","info":{"title":"Weather API","version":"1.0"},"paths":{}}`

				openApiCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "weather-openapi",
						Namespace: agentToolNamespace,
					},
					Data: map[string]string{
						"openapi.json": openApiJSON,
					},
				}
				Expect(k8sClient.Create(ctx, openApiCM)).To(Succeed())

				defer func() {
					_ = k8sClient.Delete(ctx, openApiCM)
				}()

				By("Creating an AgentTool with valueFrom schema source")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Weather API from OpenAPI spec",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.weather.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								ValueFrom: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "weather-openapi"},
									Key:                  "openapi.json",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile validates ConfigMap reference and resolves value
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Validated condition is True")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					if err != nil {
						return false
					}
					condition := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeValidated)
					return condition != nil && condition.Status == metav1.ConditionTrue
				}, timeout, interval).Should(BeTrue())

				By("Verifying ConfigMap contains resolved value from valueFrom")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				_, spec := toolDefinitionFromConfigMap(cm)
				Expect(spec.OpenApi.OpenApiSchema.Value).NotTo(BeNil())
			})

			It("should fail validation when valueFrom ConfigMap does not exist", func() {
				By("Creating an AgentTool referencing non-existent ConfigMap")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Tool with missing ConfigMap",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								ValueFrom: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "non-existent-configmap"},
									Key:                  "openapi.yaml",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile should fail due to missing ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).To(HaveOccurred())

				By("Verifying Validated condition is False")
				err = k8sClient.Get(ctx, typeNamespacedName, agentTool)
				Expect(err).NotTo(HaveOccurred())
				condition := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeValidated)
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Message).To(ContainSubstring("ConfigMap"))
			})

			It("should fail validation when valueFrom ConfigMap key does not exist", func() {
				By("Creating a ConfigMap without the expected key")
				cm := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "wrong-key-configmap",
						Namespace: agentToolNamespace,
					},
					Data: map[string]string{
						"different-key.yaml": "some content",
					},
				}
				Expect(k8sClient.Create(ctx, cm)).To(Succeed())

				defer func() {
					_ = k8sClient.Delete(ctx, cm)
				}()

				By("Creating an AgentTool referencing wrong key")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Tool with wrong ConfigMap key",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								ValueFrom: &corev1.ConfigMapKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "wrong-key-configmap"},
									Key:                  "openapi.yaml",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile should fail due to missing key
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).To(HaveOccurred())

				By("Verifying Validated condition is False with correct message")
				err = k8sClient.Get(ctx, typeNamespacedName, agentTool)
				Expect(err).NotTo(HaveOccurred())
				condition := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeValidated)
				Expect(condition).NotTo(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Message).To(ContainSubstring("key"))
			})
		})

		Context("Status updates", func() {
			It("should update observedGeneration", func() {
				By("Creating a new AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
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

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile updates status
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying observedGeneration matches generation")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					if err != nil {
						return false
					}
					return agentTool.Status.ObservedGeneration == agentTool.Generation
				}, timeout, interval).Should(BeTrue())
			})

			It("should set Stored condition on successful ConfigMap creation", func() {
				By("Creating a new AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Test tool for stored condition",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Stored condition is True")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					if err != nil {
						return false
					}
					condition := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeStored)
					return condition != nil && condition.Status == metav1.ConditionTrue
				}, timeout, interval).Should(BeTrue())

				condition := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeStored)
				Expect(condition.Reason).To(Equal(ReasonStorageSuccess))
			})
		})

		Context("ConfigMap management", func() {
			It("should update ConfigMap when spec changes", func() {
				By("Creating a new AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Original description",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com/v1",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling to create initial ConfigMap")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying initial ConfigMap")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				_, initialSpec := toolDefinitionFromConfigMap(cm)
				Expect(initialSpec.Description).To(Equal("Original description"))

				By("Updating the AgentTool spec")
				err = k8sClient.Get(ctx, typeNamespacedName, agentTool)
				Expect(err).NotTo(HaveOccurred())
				agentTool.Spec.Description = "Updated description"
				agentTool.Spec.OpenApi.URL = "https://api.example.com/v2"
				Expect(k8sClient.Update(ctx, agentTool)).To(Succeed())

				By("Reconciling again to update ConfigMap")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap was updated")
				Eventually(func() string {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
					if err != nil {
						return ""
					}
					_, spec := toolDefinitionFromConfigMap(cm)
					return spec.Description
				}, timeout, interval).Should(Equal("Updated description"))

				_, updatedSpec := toolDefinitionFromConfigMap(cm)
				Expect(updatedSpec.OpenApi.URL).To(Equal("https://api.example.com/v2"))
			})

			It("should set owner reference on ConfigMap", func() {
				By("Creating a new AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Test tool for owner reference",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap has owner reference")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				Expect(cm.OwnerReferences).To(HaveLen(1))
				Expect(cm.OwnerReferences[0].Name).To(Equal(agentToolName))
				Expect(cm.OwnerReferences[0].Kind).To(Equal("AgentTool"))
			})
		})

		Context("OpenAPI tool configurations", func() {
			It("should handle ServiceRef configuration", func() {
				By("Creating an AgentTool with ServiceRef")
				port := int32(8080)
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Tool with ServiceRef",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							ServiceRef: &agentv1alpha1.ServiceRef{
								Name:      "my-backend-service",
								Namespace: "backend",
								Port:      &port,
							},
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/api/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap contains resolved ServiceRef configuration")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				_, spec := toolDefinitionFromConfigMap(cm)
				Expect(spec.OpenApi.ServiceRef).NotTo(BeNil())
				Expect(spec.OpenApi.ServiceRef.Name).To(Equal("my-backend-service"))
				Expect(spec.OpenApi.ServiceRef.Namespace).To(Equal("backend"))
				Expect(*spec.OpenApi.ServiceRef.Port).To(Equal(int32(8080)))
				// serviceRef is resolved to url
				Expect(spec.OpenApi.URL).To(Equal("http://my-backend-service.backend.svc.cluster.local:8080"))
				// endpointPath was fetched, value should be populated
				Expect(spec.OpenApi.OpenApiSchema.Value).NotTo(BeNil())
			})

			It("should handle custom headers and timeout", func() {
				By("Creating an AgentTool with custom headers and timeout")
				timeoutSeconds := int32(60)
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Tool with custom headers",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
							TimeoutSeconds: &timeoutSeconds,
							Headers: map[string]string{
								"Content-Type":  "application/json",
								"Authorization": "Bearer ${API_KEY}",
								"X-Custom":      "custom-value",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap contains headers and timeout")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				_, spec := toolDefinitionFromConfigMap(cm)
				Expect(*spec.OpenApi.TimeoutSeconds).To(Equal(int32(60)))
				Expect(spec.OpenApi.Headers).To(HaveLen(3))
				Expect(spec.OpenApi.Headers["Content-Type"]).To(Equal("application/json"))
				Expect(spec.OpenApi.Headers["Authorization"]).To(Equal("Bearer ${API_KEY}"))
				Expect(spec.OpenApi.Headers["X-Custom"]).To(Equal("custom-value"))
			})
		})

		Context("Deletion handling", func() {
			It("should delete ConfigMap when AgentTool is deleted", func() {
				By("Creating a new AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeOpenAPI,
						Description: "Tool for deletion test",
						OpenApi: &agentv1alpha1.OpenApiToolSpec{
							URL: "https://api.example.com",
							OpenApiSchema: agentv1alpha1.OpenApiSchema{
								EndpointPath: "/openapi.json",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling to create ConfigMap")
				controllerReconciler := &AgentToolReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					HTTPClient: mockHTTPClient(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(BeNumerically(">", 0))

				// Second reconcile creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap exists")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				By("Deleting the AgentTool")
				Expect(k8sClient.Delete(ctx, agentTool)).To(Succeed())

				By("Reconciling the deletion")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap was deleted")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())

				By("Verifying AgentTool was deleted")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					return errors.IsNotFound(err)
				}, timeout, interval).Should(BeTrue())
			})
		})
	})
})
