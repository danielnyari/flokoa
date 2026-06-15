package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/danielnyari/flokoa/api/v1alpha1"
	"github.com/danielnyari/flokoa/internal/app/agent/compiler"
	agentdomain "github.com/danielnyari/flokoa/internal/domain/agent"
	"github.com/danielnyari/flokoa/internal/domain/hash"
	flokoaerrors "github.com/danielnyari/flokoa/internal/errors"
	"github.com/danielnyari/flokoa/internal/infra/builder"
	"github.com/danielnyari/flokoa/internal/infra/repo"
)

// Deps holds the repository dependencies for the agent application service.
type Deps struct {
	AgentTools    repo.AgentToolReader
	Models        repo.ModelReader
	Providers     repo.ModelProviderReader
	Instructions  repo.InstructionReader
	Capabilities  repo.CapabilityReader
	ConfigMaps    repo.ConfigMapRepo
	Deployments   repo.DeploymentRepo
	Services      repo.ServiceRepo
	ServiceReader repo.ServiceReader
	Secrets       repo.SecretReader
	OwnerSetter   repo.OwnerSetter
}

// Config carries cluster-level policy into the agent service.
type Config struct {
	// DefaultRunnerVersion is the operator's pinned runner release.
	DefaultRunnerVersion string

	// RunnerImageRepository is the generic runner image without a tag; the
	// effective runner version selects the tag.
	RunnerImageRepository string

	// Injected platform capabilities appended to every compiled spec.
	Injected []compiler.InjectedCapability

	// OTLPEndpoint configures OTEL_EXPORTER_OTLP_ENDPOINT on runner pods
	// (empty disables export).
	OTLPEndpoint string

	// CapabilityDelivery selects how capability wheelhouse artifacts reach
	// runner pods (empty: builder.DeliveryInitContainer). Per-cluster policy,
	// resolved by the operator entrypoint — never a per-Agent knob.
	CapabilityDelivery builder.CapabilityDeliveryMode

	// RequireVerifiedCapabilities mirrors the requireVerified cluster policy
	// into the compiler: attached Capabilities whose Verified condition is
	// not True fail compilation with a requeue (roadmap 09).
	RequireVerifiedCapabilities bool
}

// ReconcileResult holds the result of a reconciliation.
type ReconcileResult struct {
	// Requeue indicates whether the reconciliation should be requeued.
	Requeue bool
	// Error is any error that occurred during reconciliation.
	Error error
}

// Service is the application-layer orchestrator for Agent reconciliation:
// it compiles the composition, emits the spec ConfigMap, and keeps the
// Deployment and Services in step. All I/O goes through repo interfaces.
type Service struct {
	deps     Deps
	config   Config
	compiler *compiler.Compiler
}

// NewService creates a new agent application service.
func NewService(deps Deps, config Config) *Service {
	return &Service{
		deps:   deps,
		config: config,
		compiler: compiler.New(compiler.Deps{
			Models:       deps.Models,
			Providers:    deps.Providers,
			Instructions: deps.Instructions,
			AgentTools:   deps.AgentTools,
			Capabilities: deps.Capabilities,
			Services:     deps.ServiceReader,
		}, compiler.Options{
			DefaultRunnerVersion: config.DefaultRunnerVersion,
			Injected:             config.Injected,
			RequireVerified:      config.RequireVerifiedCapabilities,
		}),
	}
}

// Reconcile performs the full agent reconciliation.
// The agent is already fetched and finalizers are handled by the controller.
// This method mutates agent.Status in place.
func (s *Service) Reconcile(ctx context.Context, agent *agentv1alpha1.Agent) ReconcileResult {
	logger := log.FromContext(ctx)

	if err := agentdomain.ValidateSpec(agent); err != nil {
		logger.Error(err, "Agent validation failed")
		agent.Status.Phase = agentv1alpha1.AgentPhaseFailed
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeSpecValid, metav1.ConditionFalse, agentdomain.ReasonValidationFailed, err.Error())
		return ReconcileResult{} // Don't requeue on validation errors
	}

	compiled, err := s.compiler.Compile(ctx, agent)
	if err != nil {
		var verr *compiler.ValidationError
		switch {
		case errors.As(err, &verr):
			// Bad composition: surface SpecValid=False and leave the last
			// good Deployment running. Watches re-trigger on edits.
			logger.Info("Compiled spec failed schema validation", "error", verr.Error())
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeSpecValid, metav1.ConditionFalse, agentdomain.ReasonSpecInvalid, verr.Error())
			return ReconcileResult{}
		case flokoaerrors.IsDependency(err):
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeSpecValid, metav1.ConditionFalse, agentdomain.ReasonDependencyMissing, err.Error())
			return ReconcileResult{Requeue: true, Error: err}
		case flokoaerrors.IsPermanent(err):
			agent.Status.Phase = agentv1alpha1.AgentPhaseFailed
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeSpecValid, metav1.ConditionFalse, agentdomain.ReasonValidationFailed, err.Error())
			return ReconcileResult{}
		default:
			agentdomain.SetCondition(agent, agentdomain.ConditionTypeSpecValid, metav1.ConditionFalse, agentdomain.ReasonReconcileError, err.Error())
			return ReconcileResult{Requeue: true, Error: err}
		}
	}

	agentdomain.SetCondition(agent, agentdomain.ConditionTypeSpecValid, metav1.ConditionTrue, agentdomain.ReasonSpecCompiled,
		fmt.Sprintf("Compiled spec %s for runner %s", compiled.Hash, compiled.RunnerVersion))
	agent.Status.SpecHash = compiled.Hash
	agent.Status.RunnerVersion = compiled.RunnerVersion
	agent.Status.InjectedCapabilities = compiled.Injected

	specConfigMap, err := s.reconcileSpecConfigMap(ctx, agent, compiled)
	if err != nil {
		logger.Error(err, "Failed to reconcile spec ConfigMap")
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonReconcileError, err.Error())
		return ReconcileResult{Requeue: true, Error: err}
	}

	secretsHash, missingSecrets := s.secretsState(ctx, agent.Namespace, compiled)
	if len(missingSecrets) > 0 {
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeSecretsReady, metav1.ConditionFalse, agentdomain.ReasonSecretsMissing,
			fmt.Sprintf("Missing secrets: %v", missingSecrets))
	} else {
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeSecretsReady, metav1.ConditionTrue, agentdomain.ReasonSecretsResolved, "All referenced secrets are present")
	}

	deployment, err := s.reconcileDeployment(ctx, agent, compiled, specConfigMap, secretsHash)
	if err != nil {
		logger.Error(err, "Failed to reconcile Deployment")
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonReconcileError, err.Error())
		return ReconcileResult{Requeue: true, Error: err}
	}

	service, err := s.reconcileServices(ctx, agent)
	if err != nil {
		logger.Error(err, "Failed to reconcile Services")
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonReconcileError, err.Error())
		return ReconcileResult{Requeue: true, Error: err}
	}

	agent.Status.Phase = agentdomain.CalculatePhase(deployment)
	agent.Status.URL = builder.PublishedURL(service.Name, service.Namespace)
	agent.Status.Replicas = deployment.Status.Replicas
	agent.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	agent.Status.ObservedGeneration = agent.Generation

	if deployment.Status.AvailableReplicas > 0 {
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionTrue, agentdomain.ReasonDeploymentReady, "Agent is ready")
	} else {
		agentdomain.SetCondition(agent, agentdomain.ConditionTypeReady, metav1.ConditionFalse, agentdomain.ReasonDeploymentNotReady, "Waiting for pods")
	}

	return ReconcileResult{}
}

// reconcileSpecConfigMap emits the single compiled-spec ConfigMap: the
// agent-spec.yaml document plus the A2A card (runtime contract §2).
func (s *Service) reconcileSpecConfigMap(ctx context.Context, agent *agentv1alpha1.Agent, compiled *compiler.Result) (string, error) {
	name := builder.SpecConfigMapName(agent.Name)

	cardJSON, err := json.Marshal(agent.Spec.Card)
	if err != nil {
		return "", fmt.Errorf("failed to marshal agent card to JSON: %w", err)
	}

	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: agent.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       agent.Name,
				"app.kubernetes.io/component":  "agent-spec",
				"app.kubernetes.io/managed-by": "flokoa-operator",
				"flokoa.ai/agent":              agent.Name,
			},
			Annotations: map[string]string{
				"flokoa.ai/spec-hash":      compiled.Hash,
				"flokoa.ai/runner-version": compiled.RunnerVersion,
				"flokoa.ai/schema-digest":  compiled.SchemaDigest,
			},
		},
		Data: map[string]string{
			builder.AgentSpecConfigMapKey: string(compiled.YAML),
			builder.AgentCardConfigMapKey: string(cardJSON),
		},
	}

	if err := s.deps.OwnerSetter.SetOwner(agent, desired); err != nil {
		return "", fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := s.deps.ConfigMaps.EnsureConfigMap(ctx, desired); err != nil {
		return "", err
	}
	return name, nil
}

// secretsState fetches every secret the compiled spec references and returns
// a deterministic hash of their resource versions (pods roll on rotation)
// plus the list of missing ones.
func (s *Service) secretsState(ctx context.Context, namespace string, compiled *compiler.Result) (string, []string) {
	versions := map[string]string{}
	var missing []string

	record := func(envs []corev1.EnvVar) {
		for _, env := range envs {
			if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
				continue
			}
			name := env.ValueFrom.SecretKeyRef.Name
			if _, seen := versions[name]; seen {
				continue
			}
			secret, err := s.deps.Secrets.GetSecret(ctx, types.NamespacedName{Name: name, Namespace: namespace})
			if err != nil {
				versions[name] = "missing"
				missing = append(missing, name)
				continue
			}
			versions[name] = secret.ResourceVersion
		}
	}
	record(compiled.SecretEnv)
	record(compiled.ProviderSecretEnv)

	if len(versions) == 0 {
		return "", nil
	}
	return hash.SecretVersions(versions), missing
}

func (s *Service) reconcileDeployment(ctx context.Context, agent *agentv1alpha1.Agent, compiled *compiler.Result, specConfigMap, secretsHash string) (*appsv1.Deployment, error) {
	// Map compiler delivery inputs to the builder's own type: infra/builder
	// must not import app/agent/compiler (layering), so the translation
	// happens here.
	capabilities := make([]builder.CapabilityMount, 0, len(compiled.CapabilityArtifacts))
	for _, artifact := range compiled.CapabilityArtifacts {
		// The capability webhook enforces DNS-label names at admission, but
		// the name flows into container names, volume subPaths, and mount
		// paths — the builder must not trust that invariant blindly
		// (pre-webhook objects, disabled webhooks, direct etcd writes).
		if errs := validation.IsDNS1123Label(artifact.Name); len(errs) > 0 {
			return nil, fmt.Errorf(
				"capability name %q cannot be used for artifact delivery (it becomes a container name and volume subPath): %s",
				artifact.Name, strings.Join(errs, "; "))
		}
		capabilities = append(capabilities, builder.CapabilityMount{
			Name:     artifact.Name,
			Artifact: artifact.Artifact,
		})
	}
	delivery := s.config.CapabilityDelivery
	if delivery == "" {
		delivery = builder.DeliveryInitContainer
	}

	desired := builder.BuildDeployment(builder.DeploymentParams{
		AgentName:             agent.Name,
		AgentNamespace:        agent.Namespace,
		Labels:                agentdomain.Labels(agent),
		Runtime:               agent.Spec.Runtime,
		RunnerImageRepository: s.config.RunnerImageRepository,
		RunnerVersion:         compiled.RunnerVersion,
		SchemaDigest:          compiled.SchemaDigest,
		SpecConfigMapName:     specConfigMap,
		SpecHash:              compiled.Hash,
		SecretsHash:           secretsHash,
		SecretEnv:             compiled.SecretEnv,
		ProviderEnv:           compiled.ProviderEnv,
		ProviderSecretEnv:     compiled.ProviderSecretEnv,
		PublishedURL:          builder.PublishedURL(agent.Name, agent.Namespace),
		OTLPEndpoint:          s.config.OTLPEndpoint,
		Capabilities:          capabilities,
		CapabilityDelivery:    delivery,
	})

	if err := s.deps.OwnerSetter.SetOwner(agent, desired); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	return s.deps.Deployments.EnsureDeployment(ctx, desired)
}

// reconcileServices keeps the two operator-owned Services in step: the
// published endpoint ({agent}, the virtual endpoint behind status.url whose
// backend flokoa may change) and the internal workload Service
// ({agent}-runtime). The published Service's existing selector is what the
// session router flips for session-tier agents (P1) — EnsureService only
// rewrites ports here, never an externally-managed selector swap.
func (s *Service) reconcileServices(ctx context.Context, agent *agentv1alpha1.Agent) (*corev1.Service, error) {
	labels := agentdomain.Labels(agent)

	runtime := builder.BuildRuntimeService(agent.Name, agent.Namespace, labels)
	if err := s.deps.OwnerSetter.SetOwner(agent, runtime); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}
	if _, err := s.deps.Services.EnsureService(ctx, runtime); err != nil {
		return nil, err
	}

	published := builder.BuildPublishedService(agent.Name, agent.Namespace, labels)
	if err := s.deps.OwnerSetter.SetOwner(agent, published); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}
	return s.deps.Services.EnsureService(ctx, published)
}
