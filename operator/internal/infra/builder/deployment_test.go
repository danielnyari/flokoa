package builder

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TestBuildDeploymentRunnerProbes pins the readiness/startup gating on the
// runner container. Without it the kubelet marks a still-bootstrapping (or
// crash-looping) runner pod Ready the instant the container is "running" —
// the Deployment's availableReplicas flaps to 1, the Agent controller
// reports Ready=True for an agent that can never serve, and the published
// Service routes traffic to a pod that hasn't finished bootstrapping. The
// probes gate on the runner's A2A health endpoint, which only answers once
// the FastAPI app is serving, i.e. after a successful bootstrap.
func TestBuildDeploymentRunnerProbes(t *testing.T) {
	deployment := BuildDeployment(baseParams())
	runner := deployment.Spec.Template.Spec.Containers[0]

	if runner.ReadinessProbe == nil {
		t.Fatal("runner container has no readiness probe; a bootstrap-failed pod would flap Ready")
	}
	assertHealthHTTPGet(t, "readiness", runner.ReadinessProbe)

	if runner.StartupProbe == nil {
		t.Fatal("runner container has no startup probe; slow capability installs need a startup budget")
	}
	assertHealthHTTPGet(t, "startup", runner.StartupProbe)

	// The startup budget (period × threshold) must comfortably exceed a
	// worst-case wheelhouse install so a slow-but-legitimate boot is not
	// killed before it can serve.
	budget := runner.StartupProbe.PeriodSeconds * runner.StartupProbe.FailureThreshold
	if budget < 120 {
		t.Errorf("startup budget = %ds (period %d × threshold %d), want >= 120s for slow capability installs",
			budget, runner.StartupProbe.PeriodSeconds, runner.StartupProbe.FailureThreshold)
	}
}

func assertHealthHTTPGet(t *testing.T, which string, probe *corev1.Probe) {
	t.Helper()
	if probe.HTTPGet == nil {
		t.Fatalf("%s probe is not an HTTP GET probe: %+v", which, probe.ProbeHandler)
	}
	if probe.HTTPGet.Path != RuntimeHealthPath {
		t.Errorf("%s probe path = %q, want %q", which, probe.HTTPGet.Path, RuntimeHealthPath)
	}
	if want := intstr.FromInt32(RuntimePort); probe.HTTPGet.Port != want {
		t.Errorf("%s probe port = %v, want the runtime port %d", which, probe.HTTPGet.Port, RuntimePort)
	}
	if probe.SuccessThreshold != 0 && probe.SuccessThreshold != 1 {
		t.Errorf("%s probe successThreshold = %d, want 1", which, probe.SuccessThreshold)
	}
}
