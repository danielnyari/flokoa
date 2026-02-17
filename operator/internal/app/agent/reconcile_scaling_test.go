package agent

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
	"github.com/danielnyari/flokoa/internal/infra/builder"
	"github.com/danielnyari/flokoa/internal/infra/repo/fakes"
)

func int32Ptr(i int32) *int32 { return &i }

func newTestServiceWithScaling(scaledObjectRepo *fakes.FakeScaledObjectRepo) *Service {
	return NewService(Deps{
		AgentTools:    fakes.NewFakeAgentToolRepo(),
		AgentToolW:    fakes.NewFakeAgentToolRepo(),
		Models:        fakes.NewFakeModelRepo(),
		Providers:     fakes.NewFakeModelProviderRepo(),
		Instructions:  fakes.NewFakeInstructionRepo(),
		InstructionW:  fakes.NewFakeInstructionRepo(),
		ConfigMaps:    fakes.NewFakeConfigMapRepo(),
		Deployments:   fakes.NewFakeDeploymentRepo(),
		Services:      fakes.NewFakeServiceRepo(),
		Secrets:       fakes.NewFakeSecretRepo(),
		ScaledObjects: scaledObjectRepo,
		OwnerSetter:   &fakes.FakeOwnerSetter{},
	})
}

func TestReconcileScaledObject_CreatesWhenScalingConfigured(t *testing.T) {
	scaledObjectRepo := fakes.NewFakeScaledObjectRepo()
	svc := newTestServiceWithScaling(scaledObjectRepo)

	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: agentv1alpha1.AgentSpec{
			Scaling: &agentv1alpha1.ScalingSpec{
				MinReplicaCount: int32Ptr(0),
				MaxReplicaCount: int32Ptr(5),
				CooldownPeriod:  int32Ptr(300),
				PollingInterval: int32Ptr(15),
				Triggers: []agentv1alpha1.ScalingTrigger{
					{
						Type:     "prometheus",
						Metadata: map[string]string{"threshold": "100"},
					},
				},
			},
		},
	}

	err := svc.reconcileScaledObject(context.Background(), agent)
	if err != nil {
		t.Fatalf("reconcileScaledObject() error = %v", err)
	}

	// Verify ScaledObject was created
	key := types.NamespacedName{
		Name:      builder.ScaledObjectName("test-agent"),
		Namespace: "default",
	}
	if _, ok := scaledObjectRepo.ScaledObjects[key]; !ok {
		t.Error("ScaledObject was not created")
	}

	// Verify status updated
	if agent.Status.ScaledObjectName != "test-agent-scaler" {
		t.Errorf("ScaledObjectName = %q, want test-agent-scaler", agent.Status.ScaledObjectName)
	}

	// Verify condition set
	cond := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeScalingReady)
	if cond == nil {
		t.Fatal("ScalingReady condition not set")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("ScalingReady status = %q, want True", cond.Status)
	}
	if cond.Reason != agentdomain.ReasonScaledObjectReady {
		t.Errorf("ScalingReady reason = %q, want %q", cond.Reason, agentdomain.ReasonScaledObjectReady)
	}
}

func TestReconcileScaledObject_DeletesWhenScalingRemoved(t *testing.T) {
	scaledObjectRepo := fakes.NewFakeScaledObjectRepo()
	svc := newTestServiceWithScaling(scaledObjectRepo)

	// Pre-populate a ScaledObject
	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: agentv1alpha1.AgentSpec{
			Scaling: &agentv1alpha1.ScalingSpec{
				Triggers: []agentv1alpha1.ScalingTrigger{
					{Type: "cpu", Metadata: map[string]string{"value": "50"}},
				},
			},
		},
	}

	// First reconcile: create the ScaledObject
	err := svc.reconcileScaledObject(context.Background(), agent)
	if err != nil {
		t.Fatalf("reconcileScaledObject() create error = %v", err)
	}

	key := types.NamespacedName{
		Name:      builder.ScaledObjectName("test-agent"),
		Namespace: "default",
	}
	if _, ok := scaledObjectRepo.ScaledObjects[key]; !ok {
		t.Fatal("ScaledObject was not created in setup")
	}

	// Remove scaling and reconcile again
	agent.Spec.Scaling = nil
	err = svc.reconcileScaledObject(context.Background(), agent)
	if err != nil {
		t.Fatalf("reconcileScaledObject() delete error = %v", err)
	}

	// Verify ScaledObject was deleted
	if _, ok := scaledObjectRepo.ScaledObjects[key]; ok {
		t.Error("ScaledObject was not deleted")
	}

	// Verify status cleared
	if agent.Status.ScaledObjectName != "" {
		t.Errorf("ScaledObjectName = %q, want empty", agent.Status.ScaledObjectName)
	}

	// Verify condition set
	cond := meta.FindStatusCondition(agent.Status.Conditions, agentdomain.ConditionTypeScalingReady)
	if cond == nil {
		t.Fatal("ScalingReady condition not set")
	}
	if cond.Reason != agentdomain.ReasonScaledObjectRemoved {
		t.Errorf("ScalingReady reason = %q, want %q", cond.Reason, agentdomain.ReasonScaledObjectRemoved)
	}
}

func TestReconcileScaledObject_SkipsWhenRepoNil(t *testing.T) {
	svc := NewService(Deps{
		AgentTools:   fakes.NewFakeAgentToolRepo(),
		AgentToolW:   fakes.NewFakeAgentToolRepo(),
		Models:       fakes.NewFakeModelRepo(),
		Providers:    fakes.NewFakeModelProviderRepo(),
		Instructions: fakes.NewFakeInstructionRepo(),
		InstructionW: fakes.NewFakeInstructionRepo(),
		ConfigMaps:   fakes.NewFakeConfigMapRepo(),
		Deployments:  fakes.NewFakeDeploymentRepo(),
		Services:     fakes.NewFakeServiceRepo(),
		Secrets:      fakes.NewFakeSecretRepo(),
		// ScaledObjects is nil
		OwnerSetter: &fakes.FakeOwnerSetter{},
	})

	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: agentv1alpha1.AgentSpec{
			Scaling: &agentv1alpha1.ScalingSpec{
				Triggers: []agentv1alpha1.ScalingTrigger{
					{Type: "cpu", Metadata: map[string]string{"value": "50"}},
				},
			},
		},
	}

	err := svc.reconcileScaledObject(context.Background(), agent)
	if err != nil {
		t.Fatalf("reconcileScaledObject() error = %v, should skip when repo is nil", err)
	}
}

func TestReconcileScaledObject_NoopWhenNoScalingAndNoExisting(t *testing.T) {
	scaledObjectRepo := fakes.NewFakeScaledObjectRepo()
	svc := newTestServiceWithScaling(scaledObjectRepo)

	agent := &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: agentv1alpha1.AgentSpec{
			// No scaling configured
		},
	}

	err := svc.reconcileScaledObject(context.Background(), agent)
	if err != nil {
		t.Fatalf("reconcileScaledObject() error = %v", err)
	}

	// No ScaledObject should exist
	if len(scaledObjectRepo.ScaledObjects) != 0 {
		t.Errorf("expected 0 ScaledObjects, got %d", len(scaledObjectRepo.ScaledObjects))
	}
}
