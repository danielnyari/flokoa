package delivery

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/danielnyari/flokoa/internal/infra/builder"
)

const (
	// ProbePodName is the fixed name of the probe pod. Fixed on purpose:
	// delete-if-exists-then-create makes the next startup collect orphans
	// left by a crashed operator.
	ProbePodName = "flokoa-imagevolume-probe"

	probeVolumeName    = "probe"
	probeContainerName = "probe"
	probeMountPath     = "/probe"

	// probeSubPath descends into the probe image's /etc/flokoa directory —
	// the same subPath-on-image-volume shape the builder emits for capability
	// artifacts (subPath "wheelhouse"), validated against a file guaranteed
	// to exist in the runner image: the Dockerfile bakes
	// /etc/flokoa/runner-manifest.json (runtime contract §1).
	probeSubPath = "etc/flokoa"

	// probeCommand exits 0 only if the mounted file is actually readable:
	// one shot validates the image volume, subPath semantics, and content.
	probeCommand = "import os,sys; sys.exit(0 if os.path.exists('/probe/runner-manifest.json') else 1)"

	defaultPollInterval = 2 * time.Second
)

// ProbeOutcome is the classified result of one ImageVolume support probe.
type ProbeOutcome struct {
	// Supported is true when the probe pod ran to completion reading a file
	// through an image volume.
	Supported bool
	// Reason explains why the cluster was classified unsupported (empty when
	// Supported).
	Reason string
	// CompletedAt is when the classification was made.
	CompletedAt time.Time
}

// Probe determines ImageVolume support by attempting it: it runs a pod that
// mounts an image volume with a subPath and reads a known file (roadmap 09 —
// "attempt is the only reliable probe"). Any failure classifies the cluster
// as unsupported; the pod is deleted on every path.
type Probe struct {
	Client    client.Client
	Namespace string
	// Image backs both the probe pod's container and its image volume.
	// Defaults to the operator's pinned runner image upstream — guaranteed
	// pullable, since it is what agent pods pull anyway.
	Image   string
	Timeout time.Duration
	// PollInterval between pod status checks (default 2s; tests shorten it).
	PollInterval time.Duration
	Log          logr.Logger
}

// ProbeImageVolume runs the probe. It never returns an error: every failure
// mode — create rejected, feature-gated field stripped, pod failed, stuck,
// or timed out — is an "unsupported" classification with a reason.
func (p *Probe) ProbeImageVolume(ctx context.Context) ProbeOutcome {
	interval := p.PollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	if err := p.deleteAndWaitGone(ctx, interval); err != nil {
		return unsupported(fmt.Sprintf("could not replace an existing probe pod: %v", err))
	}

	pod := p.buildProbePod()
	if err := p.Client.Create(ctx, pod); err != nil {
		// The apiserver refusing the pod outright (e.g. validation rejecting
		// the volume once the feature-gated field is stripped) means the
		// gate is off — classified immediately, no waiting.
		return unsupported(fmt.Sprintf("probe pod create rejected: %v", err))
	}
	defer p.cleanup(pod)

	// With the ImageVolume feature gate disabled the apiserver may instead
	// silently strip the image field; detect that on the returned object
	// rather than waiting for an unschedulable pod to time out.
	if len(pod.Spec.Volumes) == 0 || pod.Spec.Volumes[0].Image == nil {
		return unsupported("image volume field stripped by the API server (ImageVolume feature gate disabled)")
	}

	return p.waitForCompletion(ctx, interval)
}

// deleteAndWaitGone removes a leftover probe pod (fixed name) and waits for
// it to disappear so the subsequent Create cannot race a terminating pod.
func (p *Probe) deleteAndWaitGone(ctx context.Context, interval time.Duration) error {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: ProbePodName, Namespace: p.Namespace}}
	if err := p.Client.Delete(ctx, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	key := client.ObjectKey{Namespace: p.Namespace, Name: ProbePodName}
	for {
		err := p.Client.Get(ctx, key, &corev1.Pod{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

// waitForCompletion polls the probe pod until it succeeds, fails, or the
// timeout expires. A pod stuck Pending (kubelet gate off, containerd < 2.x —
// surfaces as CreateContainerError / FailedMount) lands in the timeout path.
func (p *Probe) waitForCompletion(ctx context.Context, interval time.Duration) ProbeOutcome {
	key := client.ObjectKey{Namespace: p.Namespace, Name: ProbePodName}
	var lastPhase corev1.PodPhase
	for {
		pod := &corev1.Pod{}
		if err := p.Client.Get(ctx, key, pod); err != nil {
			return unsupported(fmt.Sprintf("probe pod status check failed: %v", err))
		}
		lastPhase = pod.Status.Phase
		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			return ProbeOutcome{Supported: true, CompletedAt: time.Now()}
		case corev1.PodFailed:
			return unsupported("probe pod failed: image volume mounted but the probe command did not succeed")
		}
		select {
		case <-ctx.Done():
			return unsupported(fmt.Sprintf(
				"probe timed out after %s (last pod phase: %s — a kubelet feature gate that is off or containerd < 2.x typically leaves the pod stuck)",
				p.Timeout, lastPhase))
		case <-time.After(interval):
		}
	}
}

// cleanup deletes the probe pod on a fresh context — the probe context is
// usually expired by the time the timeout path gets here.
func (p *Probe) cleanup(pod *corev1.Pod) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p.Client.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
		p.Log.Error(err, "failed to delete the image volume probe pod",
			"pod", ProbePodName, "namespace", p.Namespace)
	}
}

// buildProbePod assembles the probe pod: restricted security profile (the
// builder's, same as agent pods), an image volume over the probe image, and
// a one-shot command that reads a baked-in file through the subPath mount.
func (p *Probe) buildProbePod() *corev1.Pod {
	grace := int64(5)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ProbePodName,
			Namespace: p.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "flokoa-operator",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                 corev1.RestartPolicyNever,
			TerminationGracePeriodSeconds: &grace,
			SecurityContext:               builder.RestrictedPodSecurityContext(),
			Volumes: []corev1.Volume{{
				Name: probeVolumeName,
				VolumeSource: corev1.VolumeSource{
					Image: &corev1.ImageVolumeSource{
						Reference:  p.Image,
						PullPolicy: corev1.PullIfNotPresent,
					},
				},
			}},
			Containers: []corev1.Container{{
				Name:    probeContainerName,
				Image:   p.Image,
				Command: []string{"python", "-c", probeCommand},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      probeVolumeName,
					MountPath: probeMountPath,
					SubPath:   probeSubPath,
					ReadOnly:  true,
				}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10m"),
						corev1.ResourceMemory: resource.MustParse("16Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
				SecurityContext: builder.RestrictedContainerSecurityContext(),
			}},
		},
	}
}

func unsupported(reason string) ProbeOutcome {
	return ProbeOutcome{Supported: false, Reason: reason, CompletedAt: time.Now()}
}
