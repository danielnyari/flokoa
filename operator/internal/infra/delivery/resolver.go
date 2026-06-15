// Package delivery resolves how capability wheelhouse artifacts reach runner
// pods (roadmap 09): initContainer copy (default; works on every cluster) or
// ImageVolume mounts (fast path; beta feature gate + containerd 2.x). The
// resolution runs once, synchronously, at operator startup so every reconcile
// sees a settled per-cluster mode; each restart re-probes, because cluster
// upgrades change the answer. The result is surfaced as Prometheus gauges,
// the flokoa-capability-delivery state ConfigMap, and a structured log line.
package delivery

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/danielnyari/flokoa/internal/infra/builder"
)

// Mode strings accepted by --capability-delivery-mode. ModeInitContainer and
// ModeImageVolume mirror the builder constants; ModeAuto is resolver-only —
// it always settles on one of the other two before anything reconciles.
const (
	ModeInitContainer = string(builder.DeliveryInitContainer)
	ModeImageVolume   = string(builder.DeliveryImageVolume)
	ModeAuto          = "auto"
)

// StateConfigMapName names the kubectl-inspectable record of the resolution,
// written to the operator's own namespace.
const StateConfigMapName = "flokoa-capability-delivery"

// serviceAccountNamespaceFile is the in-cluster namespace fallback when the
// downward-API POD_NAMESPACE env is not set.
const serviceAccountNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// Result is the settled delivery resolution for this operator process.
// It is immutable for the process lifetime.
type Result struct {
	// ConfiguredMode is the operator flag value (initContainer, imageVolume,
	// or auto).
	ConfiguredMode string
	// EffectiveMode is what the builder will emit: always initContainer or
	// imageVolume, never auto.
	EffectiveMode builder.CapabilityDeliveryMode
	// ImageVolumeSupported records the probe verdict; nil when no probe ran
	// (explicitly configured modes are trusted, not probed).
	ImageVolumeSupported *bool
	// ProbedAt is the probe completion time (zero when no probe ran).
	ProbedAt time.Time
	// Message is the human-readable explanation of how the mode was chosen.
	Message string
}

// ImageVolumeProber abstracts the probe so the resolver mode matrix is
// testable without a cluster.
type ImageVolumeProber interface {
	ProbeImageVolume(ctx context.Context) ProbeOutcome
}

// The probe pod and the state ConfigMap live only in the operator's own
// namespace, so these markers generate namespaced Role rules (bound by
// config/rbac/manager_namespaced_role_binding.yaml and mirrored by the Helm
// chart's capability-delivery Role) rather than widening the ClusterRole.

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete,namespace=flokoa-system
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;create;update,namespace=flokoa-system

// Resolver picks the effective capability delivery mode from the configured
// one.
type Resolver struct {
	// Mode is the configured delivery mode (the flag value).
	Mode string
	// Prober runs the support probe; only consulted in ModeAuto.
	Prober ImageVolumeProber
}

// Resolve maps the configured mode to an effective one:
//
//   - initContainer: returned as-is, no probe.
//   - imageVolume: returned as-is, no probe — the operator trusts the admin;
//     misconfiguration surfaces as pod failures (documented).
//   - auto: probe; success means imageVolume, any failure falls back to
//     initContainer silently (no Agent-visible error — but logged, metered,
//     and recorded in the state ConfigMap by Publish).
//
// An unknown mode is a configuration error: the caller treats it as fatal.
func (r *Resolver) Resolve(ctx context.Context) (Result, error) {
	switch r.Mode {
	case ModeInitContainer:
		return Result{
			ConfiguredMode: r.Mode,
			EffectiveMode:  builder.DeliveryInitContainer,
			Message:        "initContainer delivery configured explicitly; no probe",
		}, nil
	case ModeImageVolume:
		return Result{
			ConfiguredMode: r.Mode,
			EffectiveMode:  builder.DeliveryImageVolume,
			Message:        "imageVolume delivery configured explicitly; not probed — misconfiguration surfaces as agent pod failures",
		}, nil
	case ModeAuto:
		outcome := r.Prober.ProbeImageVolume(ctx)
		supported := outcome.Supported
		result := Result{
			ConfiguredMode:       ModeAuto,
			ImageVolumeSupported: &supported,
			ProbedAt:             outcome.CompletedAt,
		}
		if supported {
			result.EffectiveMode = builder.DeliveryImageVolume
			result.Message = "auto: image volume probe succeeded; using imageVolume delivery"
		} else {
			result.EffectiveMode = builder.DeliveryInitContainer
			result.Message = "auto: falling back to initContainer delivery: " + outcome.Reason
		}
		return result, nil
	default:
		return Result{}, fmt.Errorf("invalid capability delivery mode %q (valid: %s, %s, %s)",
			r.Mode, ModeInitContainer, ModeImageVolume, ModeAuto)
	}
}

// Publish records the resolution where operators can see it: Prometheus
// gauges, the state ConfigMap in the operator namespace, and one structured
// log line. A ConfigMap write failure is logged but not fatal — the resolved
// mode itself is unaffected. A Kubernetes Event on the operator Deployment
// was considered and deliberately skipped: it would need the Deployment's
// identity plumbed in for marginal benefit over log + metric + ConfigMap.
func Publish(ctx context.Context, c client.Client, namespace string, result Result, log logr.Logger) {
	recordMetrics(result)

	keysAndValues := []any{
		"configuredMode", result.ConfiguredMode,
		"effectiveMode", string(result.EffectiveMode),
		"message", result.Message,
	}
	if result.ImageVolumeSupported != nil {
		keysAndValues = append(keysAndValues, "imageVolumeSupported", *result.ImageVolumeSupported)
	}
	log.Info("capability delivery mode resolved", keysAndValues...)

	if err := writeStateConfigMap(ctx, c, namespace, result); err != nil {
		log.Error(err, "failed to write the capability delivery state ConfigMap",
			"configMap", StateConfigMapName, "namespace", namespace)
	}
}

// stateConfigMapRetry bounds the conflict retry for the state ConfigMap
// write: 3 attempts with a short backoff. The ConfigMap can be touched
// concurrently (kubectl edits, a second operator replica during rollout),
// and one resolution record is not worth more persistence than that.
var stateConfigMapRetry = wait.Backoff{Steps: 3, Duration: 10 * time.Millisecond, Factor: 2.0, Jitter: 0.1}

// writeStateConfigMap creates or updates the state ConfigMap, retrying the
// Get/Update pair on write conflicts.
func writeStateConfigMap(ctx context.Context, c client.Client, namespace string, result Result) error {
	data := map[string]string{
		"configuredMode":       result.ConfiguredMode,
		"effectiveMode":        string(result.EffectiveMode),
		"imageVolumeSupported": "",
		"probedAt":             "",
		"message":              result.Message,
	}
	if result.ImageVolumeSupported != nil {
		data["imageVolumeSupported"] = strconv.FormatBool(*result.ImageVolumeSupported)
	}
	if !result.ProbedAt.IsZero() {
		data["probedAt"] = result.ProbedAt.UTC().Format(time.RFC3339)
	}

	return retry.RetryOnConflict(stateConfigMapRetry, func() error {
		existing := &corev1.ConfigMap{}
		err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: StateConfigMapName}, existing)
		if apierrors.IsNotFound(err) {
			return c.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      StateConfigMapName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "flokoa-operator",
					},
				},
				Data: data,
			})
		}
		if err != nil {
			return err
		}
		existing.Data = data
		return c.Update(ctx, existing)
	})
}

// OperatorNamespace returns the namespace the operator runs in: the
// downward-API POD_NAMESPACE env first, then the in-cluster serviceaccount
// namespace file, then "default" (local runs against a kubeconfig).
func OperatorNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}
	if data, err := os.ReadFile(serviceAccountNamespaceFile); err == nil {
		if ns := strings.TrimSpace(string(data)); ns != "" {
			return ns
		}
	}
	return "default"
}
