package trigger_test

import (
	"context"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/app/trigger"
	triggerdomain "github.com/danielnyari/flokoa/internal/domain/trigger"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/infra/repo/fakes"
)

// helpers -----------------------------------------------------------------

func int32Ptr(v int32) *int32 { return &v }
func int64Ptr(v int64) *int64 { return &v }

// newRunningAgent returns an Agent in Running phase with a URL.
func newRunningAgent(name, namespace, url string) *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: agentv1alpha1.AgentStatus{
			Phase: agentv1alpha1.AgentPhaseRunning,
			URL:   url,
		},
	}
}

// newBaseTrigger returns a minimal valid AgentTrigger.
func newBaseTrigger(name, namespace, agentName string) *agentv1alpha1.AgentTrigger {
	return &agentv1alpha1.AgentTrigger{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: agentv1alpha1.AgentTriggerSpec{
			EventSource: agentv1alpha1.EventSourceRef{
				Name:      "my-eventsource",
				EventName: "my-event",
			},
			Agent: agentv1alpha1.AgentRef{
				Name: agentName,
			},
		},
	}
}

// makeDeps constructs trigger.Deps from the provided fakes.
func makeDeps(agents *fakes.FakeAgentRepo, cms *fakes.FakeConfigMapRepo, secrets *fakes.FakeSecretRepo, owner *fakes.FakeOwnerSetter) trigger.Deps {
	return trigger.Deps{
		Agents:      agents,
		ConfigMaps:  cms,
		Secrets:     secrets,
		OwnerSetter: owner,
	}
}

// parseTriggerConfig is a test helper that extracts TriggerConfig from the
// ConfigMap stored by the fake repo.
func parseTriggerConfig(t *testing.T, cms *fakes.FakeConfigMapRepo, triggerName, namespace string) trigger.TriggerConfig {
	t.Helper()
	cmName := triggerdomain.ConfigMapName(triggerName)
	cm, err := cms.GetConfigMap(context.Background(), types.NamespacedName{
		Name:      cmName,
		Namespace: namespace,
	})
	if err != nil {
		t.Fatalf("expected ConfigMap %s to exist: %v", cmName, err)
	}
	raw, ok := cm.Data["config.json"]
	if !ok {
		t.Fatalf("ConfigMap %s missing config.json key", cmName)
	}
	var cfg trigger.TriggerConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("failed to unmarshal config.json: %v", err)
	}
	return cfg
}

// findCondition returns the condition with the given type, or nil.
func findCondition(tr *agentv1alpha1.AgentTrigger, condType string) *metav1.Condition {
	return meta.FindStatusCondition(tr.Status.Conditions, condType)
}

// tests -------------------------------------------------------------------

func TestReconcile(t *testing.T) {
	const (
		ns        = "test-ns"
		agentName = "my-agent"
		agentURL  = "http://my-agent.test-ns.svc.cluster.local:8080"
		trgName   = "my-trigger"
	)

	tests := []struct {
		name string
		// setup populates fake repos and returns the trigger to reconcile
		setup func(agents *fakes.FakeAgentRepo, cms *fakes.FakeConfigMapRepo, secrets *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger
		// ownerErr optionally injects an error from the FakeOwnerSetter
		ownerErr error
		// assertions
		wantErr        bool
		wantPermanent  bool
		wantDependency bool
		// verify is called after Reconcile to inspect result, trigger, and stored resources.
		verify func(t *testing.T, result *trigger.ReconcileResult, tr *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo)
	}{
		{
			name: "happy path: agent running with URL, creates ConfigMap",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				return newBaseTrigger(trgName, ns, agentName)
			},
			verify: func(t *testing.T, result *trigger.ReconcileResult, tr *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				if result == nil {
					t.Fatal("expected non-nil result")
				}
				if result.AgentEndpoint != agentURL {
					t.Errorf("AgentEndpoint = %q, want %q", result.AgentEndpoint, agentURL)
				}
				if result.SensorName != triggerdomain.SensorName(trgName) {
					t.Errorf("SensorName = %q, want %q", result.SensorName, triggerdomain.SensorName(trgName))
				}

				cfg := parseTriggerConfig(t, cms, trgName, ns)
				if cfg.AgentEndpoint != agentURL {
					t.Errorf("config.AgentEndpoint = %q, want %q", cfg.AgentEndpoint, agentURL)
				}
				if cfg.Metadata["trigger"] != trgName {
					t.Errorf("config.Metadata[trigger] = %q, want %q", cfg.Metadata["trigger"], trgName)
				}
				if cfg.Metadata["namespace"] != ns {
					t.Errorf("config.Metadata[namespace] = %q, want %q", cfg.Metadata["namespace"], ns)
				}
				if cfg.Limits != nil {
					t.Error("expected nil Limits")
				}
				if cfg.PushNotificationConfig != nil {
					t.Error("expected nil PushNotificationConfig")
				}

				// AgentReady condition should be True
				cond := findCondition(tr, triggerdomain.ConditionTypeAgentReady)
				if cond == nil {
					t.Fatal("expected AgentReady condition")
				}
				if cond.Status != metav1.ConditionTrue {
					t.Errorf("AgentReady status = %s, want True", cond.Status)
				}
			},
		},
		{
			name: "agent not found: returns DependencyError",
			setup: func(_ *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				// No agent in the repo
				return newBaseTrigger(trgName, ns, agentName)
			},
			wantErr:        true,
			wantDependency: true,
			verify: func(t *testing.T, result *trigger.ReconcileResult, tr *agentv1alpha1.AgentTrigger, _ *fakes.FakeConfigMapRepo) {
				if result != nil {
					t.Error("expected nil result")
				}

				cond := findCondition(tr, triggerdomain.ConditionTypeAgentReady)
				if cond == nil {
					t.Fatal("expected AgentReady condition")
				}
				if cond.Status != metav1.ConditionFalse {
					t.Errorf("AgentReady status = %s, want False", cond.Status)
				}
				if cond.Reason != triggerdomain.ReasonAgentNotFound {
					t.Errorf("AgentReady reason = %s, want %s", cond.Reason, triggerdomain.ReasonAgentNotFound)
				}

				readyCond := findCondition(tr, triggerdomain.ConditionTypeReady)
				if readyCond == nil {
					t.Fatal("expected Ready condition")
				}
				if readyCond.Status != metav1.ConditionFalse {
					t.Errorf("Ready status = %s, want False", readyCond.Status)
				}
			},
		},
		{
			name: "agent not running: returns DependencyError",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				pendingAgent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: ns},
					Status:     agentv1alpha1.AgentStatus{Phase: agentv1alpha1.AgentPhasePending},
				}
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = pendingAgent
				return newBaseTrigger(trgName, ns, agentName)
			},
			wantErr:        true,
			wantDependency: true,
			verify: func(t *testing.T, _ *trigger.ReconcileResult, tr *agentv1alpha1.AgentTrigger, _ *fakes.FakeConfigMapRepo) {
				cond := findCondition(tr, triggerdomain.ConditionTypeAgentReady)
				if cond == nil {
					t.Fatal("expected AgentReady condition")
				}
				if cond.Reason != triggerdomain.ReasonAgentNotReady {
					t.Errorf("AgentReady reason = %s, want %s", cond.Reason, triggerdomain.ReasonAgentNotReady)
				}
			},
		},
		{
			name: "agent has no URL: returns DependencyError",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				noURLAgent := &agentv1alpha1.Agent{
					ObjectMeta: metav1.ObjectMeta{Name: agentName, Namespace: ns},
					Status: agentv1alpha1.AgentStatus{
						Phase: agentv1alpha1.AgentPhaseRunning,
						URL:   "", // no URL
					},
				}
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = noURLAgent
				return newBaseTrigger(trgName, ns, agentName)
			},
			wantErr:        true,
			wantDependency: true,
			verify: func(t *testing.T, _ *trigger.ReconcileResult, tr *agentv1alpha1.AgentTrigger, _ *fakes.FakeConfigMapRepo) {
				cond := findCondition(tr, triggerdomain.ConditionTypeAgentReady)
				if cond == nil {
					t.Fatal("expected AgentReady condition")
				}
				if cond.Status != metav1.ConditionFalse {
					t.Errorf("AgentReady status = %s, want False", cond.Status)
				}
				if cond.Reason != triggerdomain.ReasonAgentNotReady {
					t.Errorf("AgentReady reason = %s, want %s", cond.Reason, triggerdomain.ReasonAgentNotReady)
				}
			},
		},
		{
			name: "invalid spec: missing eventSource name returns PermanentError",
			setup: func(_ *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.EventSource.Name = "" // make invalid
				return tr
			},
			wantErr:       true,
			wantPermanent: true,
			verify: func(t *testing.T, _ *trigger.ReconcileResult, tr *agentv1alpha1.AgentTrigger, _ *fakes.FakeConfigMapRepo) {
				cond := findCondition(tr, triggerdomain.ConditionTypeReady)
				if cond == nil {
					t.Fatal("expected Ready condition")
				}
				if cond.Status != metav1.ConditionFalse {
					t.Errorf("Ready status = %s, want False", cond.Status)
				}
				if cond.Reason != triggerdomain.ReasonValidationFailed {
					t.Errorf("Ready reason = %s, want %s", cond.Reason, triggerdomain.ReasonValidationFailed)
				}
			},
		},
		{
			name: "invalid spec: missing eventSource eventName returns PermanentError",
			setup: func(_ *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.EventSource.EventName = "" // make invalid
				return tr
			},
			wantErr:       true,
			wantPermanent: true,
		},
		{
			name: "invalid spec: missing agent name returns PermanentError",
			setup: func(_ *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.Agent.Name = "" // make invalid
				return tr
			},
			wantErr:       true,
			wantPermanent: true,
		},
		{
			name: "with push notification agentRef: resolves push config",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				// Source agent (the one being triggered)
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				// Target agent for push notification
				agents.Agents[types.NamespacedName{Name: "target-agent", Namespace: ns}] = newRunningAgent("target-agent", ns, "http://target-agent.test-ns.svc:8080")

				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					AgentRef: &agentv1alpha1.AgentRef{
						Name: "target-agent",
					},
				}
				return tr
			},
			verify: func(t *testing.T, result *trigger.ReconcileResult, _ *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				if result == nil {
					t.Fatal("expected non-nil result")
				}

				cfg := parseTriggerConfig(t, cms, trgName, ns)
				if cfg.PushNotificationConfig == nil {
					t.Fatal("expected non-nil PushNotificationConfig")
				}
				expectedURL := "http://flokoa-server.flokoa-system.svc.cluster.local/api/v1alpha1/namespaces/test-ns/agents/target-agent/push"
				if cfg.PushNotificationConfig.URL != expectedURL {
					t.Errorf("push URL = %q, want %q", cfg.PushNotificationConfig.URL, expectedURL)
				}
			},
		},
		{
			name: "with push notification URL: stores URL in push config",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)

				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					URL: "https://external.example.com/webhook",
				}
				return tr
			},
			verify: func(t *testing.T, _ *trigger.ReconcileResult, _ *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				cfg := parseTriggerConfig(t, cms, trgName, ns)
				if cfg.PushNotificationConfig == nil {
					t.Fatal("expected non-nil PushNotificationConfig")
				}
				if cfg.PushNotificationConfig.URL != "https://external.example.com/webhook" {
					t.Errorf("push URL = %q, want %q", cfg.PushNotificationConfig.URL, "https://external.example.com/webhook")
				}
			},
		},
		{
			name: "with push notification authentication and token: resolves secrets",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, secrets *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)

				// Auth credentials secret
				secrets.Secrets[types.NamespacedName{Name: "auth-secret", Namespace: ns}] = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "auth-secret", Namespace: ns},
					Data:       map[string][]byte{"token": []byte("my-bearer-token")},
				}
				// Push token secret
				secrets.Secrets[types.NamespacedName{Name: "push-token-secret", Namespace: ns}] = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "push-token-secret", Namespace: ns},
					Data:       map[string][]byte{"tok": []byte("opaque-push-token")},
				}

				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					URL: "https://external.example.com/webhook",
					Authentication: &agentv1alpha1.TriggerPushAuth{
						Schemes: []string{"Bearer"},
						CredentialsRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"},
							Key:                  "token",
						},
					},
					TokenRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "push-token-secret"},
						Key:                  "tok",
					},
				}
				return tr
			},
			verify: func(t *testing.T, _ *trigger.ReconcileResult, _ *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				cfg := parseTriggerConfig(t, cms, trgName, ns)
				if cfg.PushNotificationConfig == nil {
					t.Fatal("expected non-nil PushNotificationConfig")
				}
				pc := cfg.PushNotificationConfig
				if pc.Authentication == nil {
					t.Fatal("expected non-nil Authentication")
				}
				if len(pc.Authentication.Schemes) != 1 || pc.Authentication.Schemes[0] != "Bearer" {
					t.Errorf("schemes = %v, want [Bearer]", pc.Authentication.Schemes)
				}
				if pc.Authentication.Credentials != "my-bearer-token" {
					t.Errorf("credentials = %q, want %q", pc.Authentication.Credentials, "my-bearer-token")
				}
				if pc.Token != "opaque-push-token" {
					t.Errorf("token = %q, want %q", pc.Token, "opaque-push-token")
				}
			},
		},
		{
			name: "push notification: missing auth secret returns DependencyError",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					URL: "https://external.example.com/webhook",
					Authentication: &agentv1alpha1.TriggerPushAuth{
						Schemes: []string{"Bearer"},
						CredentialsRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "missing-secret"},
							Key:                  "token",
						},
					},
				}
				return tr
			},
			wantErr:        true,
			wantDependency: true,
		},
		{
			name: "push notification: missing secret key returns PermanentError",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, secrets *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				secrets.Secrets[types.NamespacedName{Name: "auth-secret", Namespace: ns}] = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "auth-secret", Namespace: ns},
					Data:       map[string][]byte{"wrong-key": []byte("value")},
				}
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					URL: "https://external.example.com/webhook",
					Authentication: &agentv1alpha1.TriggerPushAuth{
						Schemes: []string{"Bearer"},
						CredentialsRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "auth-secret"},
							Key:                  "token",
						},
					},
				}
				return tr
			},
			wantErr:       true,
			wantPermanent: true,
		},
		{
			name: "push notification: missing target agentRef returns DependencyError",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				// target-agent is NOT in the repo
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					AgentRef: &agentv1alpha1.AgentRef{Name: "target-agent"},
				}
				return tr
			},
			wantErr:        true,
			wantDependency: true,
		},
		{
			name: "with limits: limits are stored in ConfigMap config",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.Limits = &agentv1alpha1.TriggerLimits{
					MaxInvocationsPerHour: int32Ptr(100),
					MaxConcurrentTasks:    int32Ptr(5),
					TokenBudgetPerEvent:   int32Ptr(4096),
					TokenBudgetPerHour:    int64Ptr(1000000),
				}
				return tr
			},
			verify: func(t *testing.T, _ *trigger.ReconcileResult, _ *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				cfg := parseTriggerConfig(t, cms, trgName, ns)
				if cfg.Limits == nil {
					t.Fatal("expected non-nil Limits")
				}
				if cfg.Limits.MaxInvocationsPerHour == nil || *cfg.Limits.MaxInvocationsPerHour != 100 {
					t.Errorf("MaxInvocationsPerHour = %v, want 100", cfg.Limits.MaxInvocationsPerHour)
				}
				if cfg.Limits.MaxConcurrentTasks == nil || *cfg.Limits.MaxConcurrentTasks != 5 {
					t.Errorf("MaxConcurrentTasks = %v, want 5", cfg.Limits.MaxConcurrentTasks)
				}
				if cfg.Limits.TokenBudgetPerEvent == nil || *cfg.Limits.TokenBudgetPerEvent != 4096 {
					t.Errorf("TokenBudgetPerEvent = %v, want 4096", cfg.Limits.TokenBudgetPerEvent)
				}
				if cfg.Limits.TokenBudgetPerHour == nil || *cfg.Limits.TokenBudgetPerHour != 1000000 {
					t.Errorf("TokenBudgetPerHour = %v, want 1000000", cfg.Limits.TokenBudgetPerHour)
				}
			},
		},
		{
			name: "with task config: sessionKeyFrom and metadata stored",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.Task = &agentv1alpha1.TriggerTaskConfig{
					SessionKeyFrom: "$.customer_id",
					Metadata: map[string]string{
						"tenant": "acme",
						"env":    "prod",
					},
				}
				return tr
			},
			verify: func(t *testing.T, _ *trigger.ReconcileResult, _ *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				cfg := parseTriggerConfig(t, cms, trgName, ns)
				if cfg.SessionKeyFrom != "$.customer_id" {
					t.Errorf("SessionKeyFrom = %q, want %q", cfg.SessionKeyFrom, "$.customer_id")
				}
				// Task metadata merged with default metadata
				if cfg.Metadata["tenant"] != "acme" {
					t.Errorf("Metadata[tenant] = %q, want %q", cfg.Metadata["tenant"], "acme")
				}
				if cfg.Metadata["env"] != "prod" {
					t.Errorf("Metadata[env] = %q, want %q", cfg.Metadata["env"], "prod")
				}
				// Default metadata should still be present
				if cfg.Metadata["trigger"] != trgName {
					t.Errorf("Metadata[trigger] = %q, want %q", cfg.Metadata["trigger"], trgName)
				}
				if cfg.Metadata["namespace"] != ns {
					t.Errorf("Metadata[namespace] = %q, want %q", cfg.Metadata["namespace"], ns)
				}
			},
		},
		{
			name: "agent in different namespace: resolves cross-namespace agent",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: "other-ns"}] = newRunningAgent(agentName, "other-ns", "http://my-agent.other-ns.svc:8080")
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.Agent.Namespace = "other-ns"
				return tr
			},
			verify: func(t *testing.T, result *trigger.ReconcileResult, _ *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				if result == nil {
					t.Fatal("expected non-nil result")
				}
				if result.AgentEndpoint != "http://my-agent.other-ns.svc:8080" {
					t.Errorf("AgentEndpoint = %q, want %q", result.AgentEndpoint, "http://my-agent.other-ns.svc:8080")
				}

				cfg := parseTriggerConfig(t, cms, trgName, ns)
				if cfg.AgentEndpoint != "http://my-agent.other-ns.svc:8080" {
					t.Errorf("config.AgentEndpoint = %q, want %q", cfg.AgentEndpoint, "http://my-agent.other-ns.svc:8080")
				}
			},
		},
		{
			name: "push notification agentRef in different namespace",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				agents.Agents[types.NamespacedName{Name: "target-agent", Namespace: "other-ns"}] = newRunningAgent("target-agent", "other-ns", "http://target.other-ns.svc:8080")

				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					AgentRef: &agentv1alpha1.AgentRef{
						Name:      "target-agent",
						Namespace: "other-ns",
					},
				}
				return tr
			},
			verify: func(t *testing.T, _ *trigger.ReconcileResult, _ *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				cfg := parseTriggerConfig(t, cms, trgName, ns)
				if cfg.PushNotificationConfig == nil {
					t.Fatal("expected non-nil PushNotificationConfig")
				}
				expectedURL := "http://flokoa-server.flokoa-system.svc.cluster.local/api/v1alpha1/namespaces/other-ns/agents/target-agent/push"
				if cfg.PushNotificationConfig.URL != expectedURL {
					t.Errorf("push URL = %q, want %q", cfg.PushNotificationConfig.URL, expectedURL)
				}
			},
		},
		{
			name: "push notification: missing token secret returns DependencyError",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					URL: "https://external.example.com/webhook",
					TokenRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "missing-token-secret"},
						Key:                  "tok",
					},
				}
				return tr
			},
			wantErr:        true,
			wantDependency: true,
		},
		{
			name: "push notification: token secret with wrong key returns PermanentError",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, secrets *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				secrets.Secrets[types.NamespacedName{Name: "token-secret", Namespace: ns}] = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "token-secret", Namespace: ns},
					Data:       map[string][]byte{"other-key": []byte("val")},
				}
				tr := newBaseTrigger(trgName, ns, agentName)
				tr.Spec.PushNotification = &agentv1alpha1.PushNotificationTarget{
					URL: "https://external.example.com/webhook",
					TokenRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "token-secret"},
						Key:                  "tok",
					},
				}
				return tr
			},
			wantErr:       true,
			wantPermanent: true,
		},
		{
			name: "ConfigMap labels match domain labels",
			setup: func(agents *fakes.FakeAgentRepo, _ *fakes.FakeConfigMapRepo, _ *fakes.FakeSecretRepo) *agentv1alpha1.AgentTrigger {
				agents.Agents[types.NamespacedName{Name: agentName, Namespace: ns}] = newRunningAgent(agentName, ns, agentURL)
				return newBaseTrigger(trgName, ns, agentName)
			},
			verify: func(t *testing.T, _ *trigger.ReconcileResult, _ *agentv1alpha1.AgentTrigger, cms *fakes.FakeConfigMapRepo) {
				cmName := triggerdomain.ConfigMapName(trgName)
				cm, err := cms.GetConfigMap(context.Background(), types.NamespacedName{
					Name:      cmName,
					Namespace: ns,
				})
				if err != nil {
					t.Fatalf("expected ConfigMap %s to exist: %v", cmName, err)
				}
				expectedLabels := triggerdomain.Labels(trgName)
				for k, v := range expectedLabels {
					got, ok := cm.Labels[k]
					if !ok {
						t.Errorf("ConfigMap missing label %s", k)
					} else if got != v {
						t.Errorf("ConfigMap label %s = %q, want %q", k, got, v)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agents := fakes.NewFakeAgentRepo()
			cms := fakes.NewFakeConfigMapRepo()
			secrets := fakes.NewFakeSecretRepo()
			owner := &fakes.FakeOwnerSetter{SetOwnerErr: tt.ownerErr}

			tr := tt.setup(agents, cms, secrets)

			svc := trigger.NewService(makeDeps(agents, cms, secrets, owner))
			result, err := svc.Reconcile(context.Background(), tr)

			// Check error type
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantPermanent && !flokoaerrors.IsPermanent(err) {
				t.Errorf("expected PermanentError, got %T: %v", err, err)
			}
			if tt.wantDependency && !flokoaerrors.IsDependency(err) {
				t.Errorf("expected DependencyError, got %T: %v", err, err)
			}
			// Sanity: permanent and dependency should be mutually exclusive
			if tt.wantPermanent && flokoaerrors.IsDependency(err) {
				t.Errorf("expected PermanentError but got DependencyError: %v", err)
			}
			if tt.wantDependency && flokoaerrors.IsPermanent(err) {
				t.Errorf("expected DependencyError but got PermanentError: %v", err)
			}

			if tt.verify != nil {
				tt.verify(t, result, tr, cms)
			}
		})
	}
}
