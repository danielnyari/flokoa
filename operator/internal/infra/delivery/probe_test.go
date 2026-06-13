package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const testNamespace = "flokoa-system"

func newProbe(c client.Client) *Probe {
	return &Probe{
		Client:       c,
		Namespace:    testNamespace,
		Image:        "ghcr.io/danielnyari/flokoa-runner:0.2.0",
		Timeout:      200 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
		Log:          logr.Discard(),
	}
}

// createWithPhase intercepts pod creation to stamp a terminal phase, standing
// in for the kubelet actually running the probe.
func createWithPhase(phase corev1.PodPhase) interceptor.Funcs {
	return interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if pod, ok := obj.(*corev1.Pod); ok {
				pod.Status.Phase = phase
			}
			return c.Create(ctx, obj, opts...)
		},
	}
}

func assertProbePodDeleted(t *testing.T, c client.Client) {
	t.Helper()
	err := c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: ProbePodName}, &corev1.Pod{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("probe pod must be deleted on every path, Get returned: %v", err)
	}
}

func TestProbeCreateRejected(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
			return errors.New("admission denied the pod")
		},
	}).Build()

	outcome := newProbe(c).ProbeImageVolume(context.Background())

	if outcome.Supported {
		t.Error("create rejection must classify as unsupported")
	}
	if !strings.Contains(outcome.Reason, "create rejected") {
		t.Errorf("reason = %q, want it to mention the create rejection", outcome.Reason)
	}
	if outcome.CompletedAt.IsZero() {
		t.Error("CompletedAt must be set")
	}
}

func TestProbeImageFieldStripped(t *testing.T) {
	// The apiserver with the ImageVolume gate off silently drops the image
	// volume field; simulate by stripping it during create.
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if pod, ok := obj.(*corev1.Pod); ok && len(pod.Spec.Volumes) > 0 {
				pod.Spec.Volumes[0].Image = nil
			}
			return cl.Create(ctx, obj, opts...)
		},
	}).Build()

	outcome := newProbe(c).ProbeImageVolume(context.Background())

	if outcome.Supported {
		t.Error("a stripped image field must classify as unsupported")
	}
	if !strings.Contains(outcome.Reason, "feature gate") {
		t.Errorf("reason = %q, want it to name the feature gate", outcome.Reason)
	}
	assertProbePodDeleted(t, c)
}

func TestProbeSucceeded(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).
		WithInterceptorFuncs(createWithPhase(corev1.PodSucceeded)).Build()

	outcome := newProbe(c).ProbeImageVolume(context.Background())

	if !outcome.Supported {
		t.Errorf("a succeeded pod must classify as supported (reason: %q)", outcome.Reason)
	}
	if outcome.Reason != "" {
		t.Errorf("reason must be empty when supported, got %q", outcome.Reason)
	}
	if outcome.CompletedAt.IsZero() {
		t.Error("CompletedAt must be set")
	}
	assertProbePodDeleted(t, c)
}

func TestProbeSucceededReplacesOrphan(t *testing.T) {
	orphan := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: ProbePodName, Namespace: testNamespace}}
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(orphan).
		WithInterceptorFuncs(createWithPhase(corev1.PodSucceeded)).Build()

	outcome := newProbe(c).ProbeImageVolume(context.Background())

	if !outcome.Supported {
		t.Errorf("orphan replacement must not break the probe (reason: %q)", outcome.Reason)
	}
	assertProbePodDeleted(t, c)
}

func TestProbeFailed(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).
		WithInterceptorFuncs(createWithPhase(corev1.PodFailed)).Build()

	outcome := newProbe(c).ProbeImageVolume(context.Background())

	if outcome.Supported {
		t.Error("a failed pod must classify as unsupported")
	}
	if !strings.Contains(outcome.Reason, "failed") {
		t.Errorf("reason = %q, want it to mention the failure", outcome.Reason)
	}
	assertProbePodDeleted(t, c)
}

func TestProbeTimeout(t *testing.T) {
	// No interceptor: the pod never leaves its zero phase, standing in for a
	// kubelet that cannot mount the image volume (gate off, containerd 1.x).
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()

	outcome := newProbe(c).ProbeImageVolume(context.Background())

	if outcome.Supported {
		t.Error("a stuck pod must classify as unsupported")
	}
	if !strings.Contains(outcome.Reason, "timed out") {
		t.Errorf("reason = %q, want it to mention the timeout", outcome.Reason)
	}
	assertProbePodDeleted(t, c)
}

// TestProbeOrphanDeleteError covers the path where the existing probe pod
// cannot be deleted (a non-NotFound API error in deleteAndWaitGone).
func TestProbeOrphanDeleteError(t *testing.T) {
	orphan := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: ProbePodName, Namespace: testNamespace}}
	deleteErr := errors.New("server-side error deleting pod")
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).WithObjects(orphan).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.DeleteOption) error {
				if pod, ok := obj.(*corev1.Pod); ok && pod.Name == ProbePodName {
					return deleteErr
				}
				return nil
			},
		}).Build()

	outcome := newProbe(c).ProbeImageVolume(context.Background())

	if outcome.Supported {
		t.Error("a delete error on the orphan must classify as unsupported")
	}
	if !strings.Contains(outcome.Reason, "could not replace") {
		t.Errorf("reason = %q, want it to mention replacing an existing probe pod", outcome.Reason)
	}
}

// TestProbePodShape pins the probe pod to exactly the mount shape the
// builder emits for capabilities: image volume + subPath + read-only mount,
// restricted security profile, fixed name.
func TestProbePodShape(t *testing.T) {
	probe := newProbe(nil)
	pod := probe.buildProbePod()

	if pod.Name != ProbePodName || pod.Namespace != testNamespace {
		t.Errorf("pod identity = %s/%s, want %s/%s", pod.Namespace, pod.Name, testNamespace, ProbePodName)
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy = %q, want Never", pod.Spec.RestartPolicy)
	}
	if pod.Spec.SecurityContext == nil || pod.Spec.SecurityContext.RunAsNonRoot == nil || !*pod.Spec.SecurityContext.RunAsNonRoot {
		t.Error("pod must run with the restricted profile (runAsNonRoot)")
	}

	if len(pod.Spec.Volumes) != 1 {
		t.Fatalf("got %d volumes, want 1", len(pod.Spec.Volumes))
	}
	volume := pod.Spec.Volumes[0]
	if volume.Image == nil {
		t.Fatalf("volume is not an image volume: %+v", volume.VolumeSource)
	}
	if volume.Image.Reference != probe.Image {
		t.Errorf("image volume reference = %q, want %q", volume.Image.Reference, probe.Image)
	}
	if volume.Image.PullPolicy != corev1.PullIfNotPresent {
		t.Errorf("pullPolicy = %q, want IfNotPresent", volume.Image.PullPolicy)
	}

	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("got %d containers, want 1", len(pod.Spec.Containers))
	}
	container := pod.Spec.Containers[0]
	if container.Image != probe.Image {
		t.Errorf("container image = %q, want %q", container.Image, probe.Image)
	}
	if len(container.VolumeMounts) != 1 {
		t.Fatalf("got %d mounts, want 1", len(container.VolumeMounts))
	}
	mount := container.VolumeMounts[0]
	if mount.MountPath != "/probe" || mount.SubPath != "etc/flokoa" || !mount.ReadOnly {
		t.Errorf("mount = %+v, want /probe with subPath etc/flokoa, read-only", mount)
	}
	// The command must read through the mount — the file the runner image
	// bakes at /etc/flokoa/runner-manifest.json.
	if len(container.Command) != 3 || !strings.Contains(container.Command[2], "/probe/runner-manifest.json") {
		t.Errorf("command = %v, want a check of /probe/runner-manifest.json", container.Command)
	}
	if container.SecurityContext == nil || container.SecurityContext.AllowPrivilegeEscalation == nil ||
		*container.SecurityContext.AllowPrivilegeEscalation {
		t.Error("container must use the restricted profile (no privilege escalation)")
	}
}
