package trigger

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	triggerdomain "github.com/danielnyari/flokoa/internal/domain/trigger"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/infra/repo"
)

// Deps holds repository interfaces for the trigger app service.
type Deps struct {
	Agents      repo.AgentReader
	ConfigMaps  repo.ConfigMapRepo
	Secrets     repo.SecretReader
	OwnerSetter repo.OwnerSetter
}

// Service orchestrates AgentTrigger reconciliation logic.
type Service struct {
	deps Deps
}

// NewService creates a new trigger app service.
func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// TriggerConfig is the materialized trigger configuration written to ConfigMap
// and consumed by flokoa-server at invocation time.
type TriggerConfig struct {
	AgentEndpoint          string            `json:"agentEndpoint"`
	SessionKeyFrom         string            `json:"sessionKeyFrom,omitempty"`
	PushNotificationConfig *PushConfig       `json:"pushNotificationConfig,omitempty"`
	Limits                 *LimitsConfig     `json:"limits,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
}

// PushConfig is the resolved push notification configuration.
type PushConfig struct {
	URL            string      `json:"url"`
	Authentication *AuthConfig `json:"authentication,omitempty"`
	Token          string      `json:"token,omitempty"`
}

// AuthConfig holds resolved authentication information for push notifications.
type AuthConfig struct {
	Schemes     []string `json:"schemes"`
	Credentials string   `json:"credentials,omitempty"`
}

// LimitsConfig is the serialized form of TriggerLimits for flokoa-server.
type LimitsConfig struct {
	MaxInvocationsPerHour *int32 `json:"maxInvocationsPerHour,omitempty"`
	MaxConcurrentTasks    *int32 `json:"maxConcurrentTasks,omitempty"`
	TokenBudgetPerEvent   *int32 `json:"tokenBudgetPerEvent,omitempty"`
	TokenBudgetPerHour    *int64 `json:"tokenBudgetPerHour,omitempty"`
}

// ReconcileResult holds the reconciliation outcome.
type ReconcileResult struct {
	AgentEndpoint string
	SensorName    string
}

// Reconcile performs the trigger reconciliation logic:
// 1. Validate spec
// 2. Resolve agent endpoint
// 3. Resolve push notification config (secrets)
// 4. Build and store trigger config ConfigMap
func (s *Service) Reconcile(ctx context.Context, trigger *agentv1alpha1.AgentTrigger) (*ReconcileResult, error) {
	logger := log.FromContext(ctx)

	// 1. Validate spec (pure, no I/O)
	if err := triggerdomain.ValidateSpec(trigger); err != nil {
		triggerdomain.SetCondition(trigger, triggerdomain.ConditionTypeReady, metav1.ConditionFalse, triggerdomain.ReasonValidationFailed, err.Error())
		return nil, flokoaerrors.NewPermanent(err)
	}

	// 2. Resolve agent
	agentNS := trigger.Spec.Agent.Namespace
	if agentNS == "" {
		agentNS = trigger.Namespace
	}
	agent, err := s.deps.Agents.GetAgent(ctx, types.NamespacedName{
		Name:      trigger.Spec.Agent.Name,
		Namespace: agentNS,
	})
	if err != nil {
		triggerdomain.SetCondition(trigger, triggerdomain.ConditionTypeAgentReady, metav1.ConditionFalse, triggerdomain.ReasonAgentNotFound,
			fmt.Sprintf("Agent %s/%s not found: %v", agentNS, trigger.Spec.Agent.Name, err))
		triggerdomain.SetCondition(trigger, triggerdomain.ConditionTypeReady, metav1.ConditionFalse, triggerdomain.ReasonAgentNotFound,
			fmt.Sprintf("Agent %s/%s not found", agentNS, trigger.Spec.Agent.Name))
		return nil, flokoaerrors.NewDependency(fmt.Errorf("agent %s/%s not found: %w", agentNS, trigger.Spec.Agent.Name, err))
	}

	// Validate agent is Running
	if agent.Status.Phase != agentv1alpha1.AgentPhaseRunning {
		triggerdomain.SetCondition(trigger, triggerdomain.ConditionTypeAgentReady, metav1.ConditionFalse, triggerdomain.ReasonAgentNotReady,
			fmt.Sprintf("Agent %s is in phase %s, expected Running", agent.Name, agent.Status.Phase))
		triggerdomain.SetCondition(trigger, triggerdomain.ConditionTypeReady, metav1.ConditionFalse, triggerdomain.ReasonAgentNotReady,
			fmt.Sprintf("Agent %s is not Running", agent.Name))
		return nil, flokoaerrors.NewDependency(fmt.Errorf("agent %s is in phase %s, expected Running", agent.Name, agent.Status.Phase))
	}

	agentEndpoint := agent.Status.URL
	if agentEndpoint == "" {
		triggerdomain.SetCondition(trigger, triggerdomain.ConditionTypeAgentReady, metav1.ConditionFalse, triggerdomain.ReasonAgentNotReady,
			fmt.Sprintf("Agent %s has no URL in status", agent.Name))
		return nil, flokoaerrors.NewDependency(fmt.Errorf("agent %s has no URL in status", agent.Name))
	}

	triggerdomain.SetCondition(trigger, triggerdomain.ConditionTypeAgentReady, metav1.ConditionTrue, triggerdomain.ReasonAgentResolved,
		fmt.Sprintf("Agent %s resolved: %s", agent.Name, agentEndpoint))

	// 3. Resolve push notification config
	var pushConfig *PushConfig
	if trigger.Spec.PushNotification != nil {
		var pushErr error
		pushConfig, pushErr = s.resolvePushConfig(ctx, trigger)
		if pushErr != nil {
			return nil, pushErr
		}
	}

	// 4. Build trigger config
	triggerConfig := &TriggerConfig{
		AgentEndpoint: agentEndpoint,
		Metadata: map[string]string{
			"trigger":   trigger.Name,
			"namespace": trigger.Namespace,
		},
	}

	if trigger.Spec.Task != nil {
		triggerConfig.SessionKeyFrom = trigger.Spec.Task.SessionKeyFrom
		// Merge task metadata
		for k, v := range trigger.Spec.Task.Metadata {
			triggerConfig.Metadata[k] = v
		}
	}

	if pushConfig != nil {
		triggerConfig.PushNotificationConfig = pushConfig
	}

	if trigger.Spec.Limits != nil {
		triggerConfig.Limits = &LimitsConfig{
			MaxInvocationsPerHour: trigger.Spec.Limits.MaxInvocationsPerHour,
			MaxConcurrentTasks:    trigger.Spec.Limits.MaxConcurrentTasks,
			TokenBudgetPerEvent:   trigger.Spec.Limits.TokenBudgetPerEvent,
			TokenBudgetPerHour:    trigger.Spec.Limits.TokenBudgetPerHour,
		}
	}

	// 5. Store config in ConfigMap
	configJSON, err := json.Marshal(triggerConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal trigger config: %w", err)
	}

	cmName := triggerdomain.ConfigMapName(trigger.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: trigger.Namespace,
			Labels:    triggerdomain.Labels(trigger.Name),
		},
		Data: map[string]string{
			"config.json": string(configJSON),
		},
	}

	if err := s.deps.OwnerSetter.SetOwner(trigger, cm); err != nil {
		return nil, fmt.Errorf("failed to set owner reference on ConfigMap: %w", err)
	}

	if err := s.deps.ConfigMaps.EnsureConfigMap(ctx, cm); err != nil {
		return nil, fmt.Errorf("failed to ensure trigger config ConfigMap: %w", err)
	}

	logger.Info("Trigger config stored", "configMap", cmName, "agentEndpoint", agentEndpoint)

	sensorName := triggerdomain.SensorName(trigger.Name)
	return &ReconcileResult{
		AgentEndpoint: agentEndpoint,
		SensorName:    sensorName,
	}, nil
}

// resolvePushConfig resolves the push notification configuration, reading secrets as needed.
func (s *Service) resolvePushConfig(ctx context.Context, trigger *agentv1alpha1.AgentTrigger) (*PushConfig, error) {
	push := trigger.Spec.PushNotification
	config := &PushConfig{}

	if push.AgentRef != nil {
		// Resolve target agent to construct gateway URL
		targetNS := push.AgentRef.Namespace
		if targetNS == "" {
			targetNS = trigger.Namespace
		}
		targetAgent, err := s.deps.Agents.GetAgent(ctx, types.NamespacedName{
			Name:      push.AgentRef.Name,
			Namespace: targetNS,
		})
		if err != nil {
			return nil, flokoaerrors.NewDependency(fmt.Errorf("push target agent %s/%s not found: %w", targetNS, push.AgentRef.Name, err))
		}
		_ = targetAgent // validated existence; gateway URL is constructed from name/namespace
		// Push URL points to flokoa-server gateway
		config.URL = fmt.Sprintf("http://flokoa-server.flokoa-system.svc.cluster.local/api/v1alpha1/namespaces/%s/agents/%s/push",
			targetNS, push.AgentRef.Name)
	} else if push.URL != "" {
		config.URL = push.URL
	}

	// Resolve authentication secret
	if push.Authentication != nil {
		config.Authentication = &AuthConfig{
			Schemes: push.Authentication.Schemes,
		}
		if push.Authentication.CredentialsRef != nil {
			secret, err := s.deps.Secrets.GetSecret(ctx, types.NamespacedName{
				Name:      push.Authentication.CredentialsRef.Name,
				Namespace: trigger.Namespace,
			})
			if err != nil {
				return nil, flokoaerrors.NewDependency(fmt.Errorf("push authentication secret %s not found: %w", push.Authentication.CredentialsRef.Name, err))
			}
			cred, ok := secret.Data[push.Authentication.CredentialsRef.Key]
			if !ok {
				return nil, flokoaerrors.NewPermanent(fmt.Errorf("key %s not found in secret %s", push.Authentication.CredentialsRef.Key, push.Authentication.CredentialsRef.Name))
			}
			config.Authentication.Credentials = string(cred)
		}
	}

	// Resolve token secret
	if push.TokenRef != nil {
		secret, err := s.deps.Secrets.GetSecret(ctx, types.NamespacedName{
			Name:      push.TokenRef.Name,
			Namespace: trigger.Namespace,
		})
		if err != nil {
			return nil, flokoaerrors.NewDependency(fmt.Errorf("push token secret %s not found: %w", push.TokenRef.Name, err))
		}
		tok, ok := secret.Data[push.TokenRef.Key]
		if !ok {
			return nil, flokoaerrors.NewPermanent(fmt.Errorf("key %s not found in secret %s", push.TokenRef.Key, push.TokenRef.Name))
		}
		config.Token = string(tok)
	}

	return config, nil
}
