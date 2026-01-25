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
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Test tool for fetching weather data",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:            "https://api.weather.com/v1/forecast",
							Method:         agentv1alpha1.HTTPMethodGet,
							TimeoutSeconds: &timeoutSeconds,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling the AgentTool to add finalizer")
				controllerReconciler := &AgentToolReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds the finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

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

				By("Verifying ConfigMap contains valid spec JSON")
				var spec agentv1alpha1.AgentToolSpec
				err = json.Unmarshal([]byte(cm.Data["spec.json"]), &spec)
				Expect(err).NotTo(HaveOccurred())
				Expect(spec.Type).To(Equal(agentv1alpha1.AgentToolTypeHTTPAPI))
				Expect(spec.Description).To(Equal("Test tool for fetching weather data"))
				Expect(spec.HTTPApi.URL).To(Equal("https://api.weather.com/v1/forecast"))
				Expect(spec.HTTPApi.Method).To(Equal(agentv1alpha1.HTTPMethodGet))
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

		Context("Schema validation", func() {
			It("should validate and accept valid inputSchema", func() {
				By("Creating an AgentTool with valid inputSchema")
				inputSchema := map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{
							"type":        "string",
							"description": "The city name",
						},
						"country": map[string]interface{}{
							"type":        "string",
							"description": "The country code",
						},
					},
					"required": []string{"city"},
				}
				inputSchemaBytes, _ := json.Marshal(inputSchema)

				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Weather API with input schema",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.weather.com/v1/forecast",
							Method: agentv1alpha1.HTTPMethodGet,
						},
						InputSchema: &runtime.RawExtension{Raw: inputSchemaBytes},
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
				Expect(result.Requeue).To(BeTrue())

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

			It("should validate and accept valid outputSchema", func() {
				By("Creating an AgentTool with valid outputSchema")
				outputSchema := map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"temperature": map[string]interface{}{
							"type":        "number",
							"description": "Temperature in Celsius",
						},
						"humidity": map[string]interface{}{
							"type":        "number",
							"description": "Humidity percentage",
						},
					},
				}
				outputSchemaBytes, _ := json.Marshal(outputSchema)

				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Weather API with output schema",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.weather.com/v1/forecast",
							Method: agentv1alpha1.HTTPMethodGet,
						},
						OutputSchema: &runtime.RawExtension{Raw: outputSchemaBytes},
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
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile validates schema
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
			})

			It("should fail validation for empty inputSchema", func() {
				By("Creating an AgentTool with empty inputSchema raw bytes")
				// Empty raw bytes represent invalid/empty schema that should fail validation
				emptySchema := &runtime.RawExtension{Raw: []byte{}}

				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Tool with empty schema",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
						InputSchema: emptySchema,
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
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile should fail validation due to empty schema
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
				Expect(condition.Reason).To(Equal(ReasonValidationFailed))
			})
		})

		Context("OpenAPI schema reference validation", func() {
			It("should validate OpenAPI schema from existing ConfigMap", func() {
				By("Creating a ConfigMap with OpenAPI spec")
				openApiSpec := `openapi: "3.0.0"
info:
  title: Weather API
  version: "1.0"
paths:
  /forecast:
    get:
      summary: Get weather forecast`

				openApiCM := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "weather-openapi",
						Namespace: agentToolNamespace,
					},
					Data: map[string]string{
						"openapi.yaml": openApiSpec,
					},
				}
				Expect(k8sClient.Create(ctx, openApiCM)).To(Succeed())

				defer func() {
					_ = k8sClient.Delete(ctx, openApiCM)
				}()

				By("Creating an AgentTool with OpenAPI ConfigMap reference")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Weather API from OpenAPI spec",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.weather.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
						OpenApiSchemaRef: &agentv1alpha1.OpenApiSchemaRef{
							ConfigMapRef: &agentv1alpha1.ConfigMapKeyRef{
								Name: "weather-openapi",
								Key:  "openapi.yaml",
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
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile validates ConfigMap reference
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
			})

			It("should fail validation when ConfigMap does not exist", func() {
				By("Creating an AgentTool referencing non-existent ConfigMap")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Tool with missing ConfigMap",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
						OpenApiSchemaRef: &agentv1alpha1.OpenApiSchemaRef{
							ConfigMapRef: &agentv1alpha1.ConfigMapKeyRef{
								Name: "non-existent-configmap",
								Key:  "openapi.yaml",
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
				Expect(result.Requeue).To(BeTrue())

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

			It("should fail validation when ConfigMap key does not exist", func() {
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
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Tool with wrong ConfigMap key",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
						OpenApiSchemaRef: &agentv1alpha1.OpenApiSchemaRef{
							ConfigMapRef: &agentv1alpha1.ConfigMapKeyRef{
								Name: "wrong-key-configmap",
								Key:  "openapi.yaml",
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
				Expect(result.Requeue).To(BeTrue())

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

			It("should accept OpenAPI URL reference", func() {
				By("Creating an AgentTool with OpenAPI URL reference")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Tool with OpenAPI URL",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
						OpenApiSchemaRef: &agentv1alpha1.OpenApiSchemaRef{
							URL: "https://api.example.com/openapi.json",
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
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile validates (URL validation is deferred to runtime)
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
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Test tool",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
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
				Expect(result.Requeue).To(BeTrue())

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
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Test tool for stored condition",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodPost,
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
				Expect(result.Requeue).To(BeTrue())

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
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Original description",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com/v1",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling to create initial ConfigMap")
				controllerReconciler := &AgentToolReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

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

				var initialSpec agentv1alpha1.AgentToolSpec
				err = json.Unmarshal([]byte(cm.Data["spec.json"]), &initialSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(initialSpec.Description).To(Equal("Original description"))

				By("Updating the AgentTool spec")
				err = k8sClient.Get(ctx, typeNamespacedName, agentTool)
				Expect(err).NotTo(HaveOccurred())
				agentTool.Spec.Description = "Updated description"
				agentTool.Spec.HTTPApi.URL = "https://api.example.com/v2"
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
					var spec agentv1alpha1.AgentToolSpec
					if err := json.Unmarshal([]byte(cm.Data["spec.json"]), &spec); err != nil {
						return ""
					}
					return spec.Description
				}, timeout, interval).Should(Equal("Updated description"))

				var updatedSpec agentv1alpha1.AgentToolSpec
				err = json.Unmarshal([]byte(cm.Data["spec.json"]), &updatedSpec)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedSpec.HTTPApi.URL).To(Equal("https://api.example.com/v2"))
			})

			It("should set owner reference on ConfigMap", func() {
				By("Creating a new AgentTool")
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Test tool for owner reference",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
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
				Expect(result.Requeue).To(BeTrue())

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

		Context("HTTP API configurations", func() {
			It("should handle ServiceRef configuration", func() {
				By("Creating an AgentTool with ServiceRef")
				port := int32(8080)
				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Tool with ServiceRef",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							ServiceRef: &agentv1alpha1.ServiceRef{
								Name:      "my-backend-service",
								Namespace: "backend",
								Port:      &port,
							},
							Path:   "/api/data",
							Method: agentv1alpha1.HTTPMethodPost,
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
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile creates ConfigMap
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying ConfigMap contains ServiceRef configuration")
				cmName := fmt.Sprintf("%s-spec", agentToolName)
				cm := &corev1.ConfigMap{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
				}, timeout, interval).Should(Succeed())

				var spec agentv1alpha1.AgentToolSpec
				err = json.Unmarshal([]byte(cm.Data["spec.json"]), &spec)
				Expect(err).NotTo(HaveOccurred())
				Expect(spec.HTTPApi.ServiceRef).NotTo(BeNil())
				Expect(spec.HTTPApi.ServiceRef.Name).To(Equal("my-backend-service"))
				Expect(spec.HTTPApi.ServiceRef.Namespace).To(Equal("backend"))
				Expect(*spec.HTTPApi.ServiceRef.Port).To(Equal(int32(8080)))
				Expect(spec.HTTPApi.Path).To(Equal("/api/data"))
			})

			It("should handle all HTTP methods", func() {
				methods := []agentv1alpha1.HTTPMethod{
					agentv1alpha1.HTTPMethodGet,
					agentv1alpha1.HTTPMethodPost,
					agentv1alpha1.HTTPMethodPut,
					agentv1alpha1.HTTPMethodPatch,
					agentv1alpha1.HTTPMethodDelete,
				}

				for _, method := range methods {
					// Use lowercase method name for valid Kubernetes resource name
					testName := fmt.Sprintf("test-tool-%s-%d", strings.ToLower(string(method)), time.Now().UnixNano())
					testNamespacedName := types.NamespacedName{
						Name:      testName,
						Namespace: agentToolNamespace,
					}

					By(fmt.Sprintf("Creating an AgentTool with %s method", method))
					agentTool := &agentv1alpha1.AgentTool{
						ObjectMeta: metav1.ObjectMeta{
							Name:      testName,
							Namespace: agentToolNamespace,
						},
						Spec: agentv1alpha1.AgentToolSpec{
							Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
							Description: fmt.Sprintf("Tool with %s method", method),
							HTTPApi: &agentv1alpha1.HTTPApiSpec{
								URL:    "https://api.example.com/resource",
								Method: method,
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
						NamespacedName: testNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(result.Requeue).To(BeTrue())

					// Second reconcile creates ConfigMap
					_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
						NamespacedName: testNamespacedName,
					})
					Expect(err).NotTo(HaveOccurred())

					By(fmt.Sprintf("Verifying ConfigMap contains %s method", method))
					cmName := fmt.Sprintf("%s-spec", testName)
					cm := &corev1.ConfigMap{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{Name: cmName, Namespace: agentToolNamespace}, cm)
					}, timeout, interval).Should(Succeed())

					var spec agentv1alpha1.AgentToolSpec
					err = json.Unmarshal([]byte(cm.Data["spec.json"]), &spec)
					Expect(err).NotTo(HaveOccurred())
					Expect(spec.HTTPApi.Method).To(Equal(method))

					// Cleanup
					if controllerutil.ContainsFinalizer(agentTool, agentToolFinalizer) {
						err = k8sClient.Get(ctx, testNamespacedName, agentTool)
						if err == nil {
							controllerutil.RemoveFinalizer(agentTool, agentToolFinalizer)
							_ = k8sClient.Update(ctx, agentTool)
						}
					}
					_ = k8sClient.Delete(ctx, agentTool)
					_ = k8sClient.Delete(ctx, cm)
				}
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
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Tool with custom headers",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:            "https://api.example.com",
							Method:         agentv1alpha1.HTTPMethodPost,
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
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

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

				var spec agentv1alpha1.AgentToolSpec
				err = json.Unmarshal([]byte(cm.Data["spec.json"]), &spec)
				Expect(err).NotTo(HaveOccurred())
				Expect(*spec.HTTPApi.TimeoutSeconds).To(Equal(int32(60)))
				Expect(spec.HTTPApi.Headers).To(HaveLen(3))
				Expect(spec.HTTPApi.Headers["Content-Type"]).To(Equal("application/json"))
				Expect(spec.HTTPApi.Headers["Authorization"]).To(Equal("Bearer ${API_KEY}"))
				Expect(spec.HTTPApi.Headers["X-Custom"]).To(Equal("custom-value"))
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
						Type:        agentv1alpha1.AgentToolTypeHTTPAPI,
						Description: "Tool for deletion test",
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.example.com",
							Method: agentv1alpha1.HTTPMethodGet,
						},
					},
				}
				Expect(k8sClient.Create(ctx, agentTool)).To(Succeed())

				By("Reconciling to create ConfigMap")
				controllerReconciler := &AgentToolReconciler{
					Client: k8sClient,
					Scheme: k8sClient.Scheme(),
				}

				// First reconcile adds finalizer
				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Requeue).To(BeTrue())

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

		Context("Combined input and output schemas", func() {
			It("should accept both input and output schemas together", func() {
				By("Creating an AgentTool with both schemas")
				inputSchema := map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type": "string",
						},
					},
					"required": []string{"query"},
				}
				outputSchema := map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"results": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
					},
				}
				inputSchemaBytes, _ := json.Marshal(inputSchema)
				outputSchemaBytes, _ := json.Marshal(outputSchema)

				agentTool := &agentv1alpha1.AgentTool{
					ObjectMeta: metav1.ObjectMeta{
						Name:      agentToolName,
						Namespace: agentToolNamespace,
					},
					Spec: agentv1alpha1.AgentToolSpec{
						Type:         agentv1alpha1.AgentToolTypeHTTPAPI,
						Description:  "Search API with both schemas",
						InputSchema:  &runtime.RawExtension{Raw: inputSchemaBytes},
						OutputSchema: &runtime.RawExtension{Raw: outputSchemaBytes},
						HTTPApi: &agentv1alpha1.HTTPApiSpec{
							URL:    "https://api.search.com/search",
							Method: agentv1alpha1.HTTPMethodPost,
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
				Expect(result.Requeue).To(BeTrue())

				// Second reconcile validates and stores
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying both conditions are True")
				Eventually(func() bool {
					err := k8sClient.Get(ctx, typeNamespacedName, agentTool)
					if err != nil {
						return false
					}
					validated := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeValidated)
					stored := meta.FindStatusCondition(agentTool.Status.Conditions, ConditionTypeStored)
					return validated != nil && validated.Status == metav1.ConditionTrue &&
						stored != nil && stored.Status == metav1.ConditionTrue
				}, timeout, interval).Should(BeTrue())
			})
		})
	})
})
