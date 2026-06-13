package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/danielnyari/flokoa/internal/infra/builder"
)

type fakeProber struct {
	outcome ProbeOutcome
	calls   int
}

func (f *fakeProber) ProbeImageVolume(_ context.Context) ProbeOutcome {
	f.calls++
	return f.outcome
}

func TestResolverModeMatrix(t *testing.T) {
	probedAt := time.Now()

	tests := []struct {
		name          string
		mode          string
		probe         ProbeOutcome
		wantEffective builder.CapabilityDeliveryMode
		wantProbed    bool
		wantSupported bool
		wantInMessage string
	}{
		{
			name:          "explicit initContainer skips the probe",
			mode:          ModeInitContainer,
			wantEffective: builder.DeliveryInitContainer,
			wantProbed:    false,
		},
		{
			name:          "explicit imageVolume trusts the admin, no probe",
			mode:          ModeImageVolume,
			wantEffective: builder.DeliveryImageVolume,
			wantProbed:    false,
			wantInMessage: "not probed",
		},
		{
			name:          "auto with supporting cluster lands on imageVolume",
			mode:          ModeAuto,
			probe:         ProbeOutcome{Supported: true, CompletedAt: probedAt},
			wantEffective: builder.DeliveryImageVolume,
			wantProbed:    true,
			wantSupported: true,
		},
		{
			name:          "auto with failing probe falls back to initContainer with the reason",
			mode:          ModeAuto,
			probe:         ProbeOutcome{Supported: false, Reason: "probe timed out after 60s", CompletedAt: probedAt},
			wantEffective: builder.DeliveryInitContainer,
			wantProbed:    true,
			wantSupported: false,
			wantInMessage: "probe timed out after 60s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prober := &fakeProber{outcome: tt.probe}
			resolver := &Resolver{Mode: tt.mode, Prober: prober}

			result, err := resolver.Resolve(context.Background())
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}

			if result.EffectiveMode != tt.wantEffective {
				t.Errorf("effective mode = %q, want %q", result.EffectiveMode, tt.wantEffective)
			}
			if result.ConfiguredMode != tt.mode {
				t.Errorf("configured mode = %q, want %q", result.ConfiguredMode, tt.mode)
			}
			if tt.wantProbed {
				if prober.calls != 1 {
					t.Errorf("probe ran %d times, want 1", prober.calls)
				}
				if result.ImageVolumeSupported == nil {
					t.Fatal("ImageVolumeSupported must be recorded after a probe")
				}
				if *result.ImageVolumeSupported != tt.wantSupported {
					t.Errorf("ImageVolumeSupported = %v, want %v", *result.ImageVolumeSupported, tt.wantSupported)
				}
				if !result.ProbedAt.Equal(probedAt) {
					t.Errorf("ProbedAt = %v, want the probe completion time %v", result.ProbedAt, probedAt)
				}
			} else {
				if prober.calls != 0 {
					t.Errorf("probe ran %d times, want 0 for explicit modes", prober.calls)
				}
				if result.ImageVolumeSupported != nil {
					t.Errorf("ImageVolumeSupported = %v, want nil without a probe", *result.ImageVolumeSupported)
				}
				if !result.ProbedAt.IsZero() {
					t.Errorf("ProbedAt = %v, want zero without a probe", result.ProbedAt)
				}
			}
			if tt.wantInMessage != "" && !strings.Contains(result.Message, tt.wantInMessage) {
				t.Errorf("message = %q, want it to contain %q", result.Message, tt.wantInMessage)
			}
		})
	}
}

func TestResolverInvalidMode(t *testing.T) {
	for _, mode := range []string{"bogus", "", "ImageVolume"} {
		t.Run("mode "+mode, func(t *testing.T) {
			resolver := &Resolver{Mode: mode, Prober: &fakeProber{}}
			if _, err := resolver.Resolve(context.Background()); err == nil {
				t.Errorf("Resolve(%q) must fail — startup treats it as fatal", mode)
			}
		})
	}
}

func TestPublishWritesStateConfigMapAndMetrics(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
	ctx := context.Background()
	supported := true
	probedAt := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	Publish(ctx, c, testNamespace, Result{
		ConfiguredMode:       ModeAuto,
		EffectiveMode:        builder.DeliveryImageVolume,
		ImageVolumeSupported: &supported,
		ProbedAt:             probedAt,
		Message:              "auto: image volume probe succeeded; using imageVolume delivery",
	}, logr.Discard())

	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: StateConfigMapName}, cm); err != nil {
		t.Fatalf("state ConfigMap not created: %v", err)
	}
	want := map[string]string{
		"configuredMode":       "auto",
		"effectiveMode":        "imageVolume",
		"imageVolumeSupported": "true",
		"probedAt":             "2026-06-12T10:00:00Z",
		"message":              "auto: image volume probe succeeded; using imageVolume delivery",
	}
	for key, value := range want {
		if cm.Data[key] != value {
			t.Errorf("data[%s] = %q, want %q", key, cm.Data[key], value)
		}
	}

	if got := testutil.ToFloat64(deliveryModeGauge.WithLabelValues("imageVolume")); got != 1 {
		t.Errorf("flokoa_capability_delivery_mode{mode=imageVolume} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(imageVolumeSupportedGauge); got != 1 {
		t.Errorf("flokoa_capability_imagevolume_supported = %v, want 1", got)
	}

	// A second Publish (same process would never do this, but the write must
	// be idempotent for tests and crash-loops) takes the update path.
	Publish(ctx, c, testNamespace, Result{
		ConfiguredMode: ModeInitContainer,
		EffectiveMode:  builder.DeliveryInitContainer,
		Message:        "initContainer delivery configured explicitly; no probe",
	}, logr.Discard())

	if err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: StateConfigMapName}, cm); err != nil {
		t.Fatalf("state ConfigMap lost on update: %v", err)
	}
	if cm.Data["effectiveMode"] != "initContainer" {
		t.Errorf("data[effectiveMode] = %q, want initContainer after update", cm.Data["effectiveMode"])
	}
	if cm.Data["imageVolumeSupported"] != "" || cm.Data["probedAt"] != "" {
		t.Errorf("probe fields must clear when no probe ran: supported=%q probedAt=%q",
			cm.Data["imageVolumeSupported"], cm.Data["probedAt"])
	}
	if got := testutil.ToFloat64(deliveryModeGauge.WithLabelValues("initContainer")); got != 1 {
		t.Errorf("flokoa_capability_delivery_mode{mode=initContainer} = %v, want 1", got)
	}
	// Reset clears the stale label.
	if got := testutil.ToFloat64(deliveryModeGauge.WithLabelValues("imageVolume")); got != 0 {
		t.Errorf("stale flokoa_capability_delivery_mode{mode=imageVolume} = %v, want 0", got)
	}
}

func TestWriteStateConfigMapRetriesOnConflict(t *testing.T) {
	ctx := context.Background()
	seed := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: StateConfigMapName},
		Data:       map[string]string{"effectiveMode": "stale"},
	}

	conflicts := 0
	updates := 0
	c := fake.NewClientBuilder().
		WithScheme(clientgoscheme.Scheme).
		WithObjects(seed).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				if conflicts == 0 {
					conflicts++
					return apierrors.NewConflict(
						corev1.Resource("configmaps"), StateConfigMapName, errors.New("simulated concurrent write"))
				}
				return cl.Update(ctx, obj, opts...)
			},
		}).
		Build()

	err := writeStateConfigMap(ctx, c, testNamespace, Result{
		ConfiguredMode: ModeInitContainer,
		EffectiveMode:  builder.DeliveryInitContainer,
		Message:        "initContainer delivery configured explicitly; no probe",
	})
	if err != nil {
		t.Fatalf("a single conflict must be retried away, got %v", err)
	}
	if updates != 2 {
		t.Errorf("update attempts = %d, want 2 (one conflict, one success)", updates)
	}

	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: StateConfigMapName}, cm); err != nil {
		t.Fatalf("state ConfigMap lost: %v", err)
	}
	if cm.Data["effectiveMode"] != "initContainer" {
		t.Errorf("data[effectiveMode] = %q, want initContainer after the retried write", cm.Data["effectiveMode"])
	}
}

func TestOperatorNamespaceEnvOverride(t *testing.T) {
	t.Setenv("POD_NAMESPACE", "my-operator-ns")
	if got := OperatorNamespace(); got != "my-operator-ns" {
		t.Errorf("OperatorNamespace() = %q, want my-operator-ns", got)
	}
}
