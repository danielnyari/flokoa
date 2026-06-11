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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentapp "github.com/danielnyari/flokoa/internal/app/agent"
	"github.com/danielnyari/flokoa/internal/app/agent/compiler"
	"github.com/danielnyari/flokoa/internal/infra/builder"
	"github.com/danielnyari/flokoa/internal/infra/repo"
	"github.com/danielnyari/flokoa/internal/spec"
)

// Platform-injected capabilities (roadmap 07): operator-appended entries in
// every compiled spec, after user entries, surfaced in status — transparency
// without editability.
var _ = Describe("Platform capability injection", func() {
	ctx := context.Background()

	newReconcilerWithInjection := func(otlpEndpoint string) *AgentReconciler {
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
			Injected:              []compiler.InjectedCapability{{Name: "flokoa.platform/telemetry"}},
			OTLPEndpoint:          otlpEndpoint,
		})
		return &AgentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), AppService: appService}
	}

	It("appends the telemetry entry last and surfaces it in status", func() {
		agent := &agentv1alpha1.Agent{
			ObjectMeta: metav1.ObjectMeta{Name: "injected-agent", Namespace: "default"},
			Spec: agentv1alpha1.AgentSpec{
				Card: minimalCard(),
				Spec: &agentv1alpha1.AgentSpecFragment{
					Model:        "openai:gpt-5-mini",
					Capabilities: []agentv1alpha1.NativeCapabilityEntry{{Name: "Thinking"}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, agent)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, agent) })

		nn := types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}
		reconcileAgent(ctx, newReconcilerWithInjection("http://otel-collector:4317"), nn)

		fetched := getAgent(ctx, nn)
		Expect(fetched.Status.InjectedCapabilities).To(Equal([]string{"flokoa.platform/telemetry"}))
		specValid := findCondition(fetched.Status.Conditions, "SpecValid")
		Expect(specValid).NotTo(BeNil())
		Expect(specValid.Status).To(Equal(metav1.ConditionTrue))

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: builder.SpecConfigMapName(agent.Name), Namespace: agent.Namespace,
		}, cm)).To(Succeed())
		compiled := cm.Data[builder.AgentSpecConfigMapKey]
		Expect(compiled).To(ContainSubstring("flokoa.platform/telemetry"))
		// Ordering: the user's Thinking entry must precede the injected entry.
		Expect(strings.Index(compiled, "Thinking")).To(BeNumerically("<", strings.Index(compiled, "flokoa.platform/telemetry")))

		deployment := getDeployment(ctx, nn)
		env := deployment.Spec.Template.Spec.Containers[0].Env
		Expect(env).To(ContainElement(corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_ENDPOINT", Value: "http://otel-collector:4317"}))
		Expect(env).To(ContainElement(corev1.EnvVar{Name: "OTEL_SERVICE_NAME", Value: agent.Name}))
	})
})

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
