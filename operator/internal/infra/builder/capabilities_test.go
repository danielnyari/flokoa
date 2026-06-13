package builder

import (
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// baseParams is the minimal capability-free input shared by every table row.
func baseParams() DeploymentParams {
	return DeploymentParams{
		AgentName:         "test-agent",
		AgentNamespace:    "default",
		Labels:            map[string]string{"app.kubernetes.io/name": "test-agent"},
		RunnerVersion:     "0.2.0",
		SchemaDigest:      "sha256:abc",
		SpecConfigMapName: "test-agent-agent-spec",
		SpecHash:          "deadbeef",
		PublishedURL:      "http://test-agent.default.svc.cluster.local:80/",
	}
}

func echoMount() CapabilityMount {
	return CapabilityMount{
		Name:     "echo",
		Artifact: "ghcr.io/danielnyari/capabilities/echo@sha256:" + strings.Repeat("a", 64),
	}
}

func upperMount() CapabilityMount {
	return CapabilityMount{
		Name:     "upper",
		Artifact: "ghcr.io/danielnyari/capabilities/upper@sha256:" + strings.Repeat("b", 64),
	}
}

func TestCapResourceName(t *testing.T) {
	longName := strings.Repeat("a", 80)

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "short name passes through",
			input: "echo",
			want:  "cap-echo",
		},
		{
			name:  "exactly 59 chars fits the 63-char limit",
			input: strings.Repeat("x", 59),
			want:  "cap-" + strings.Repeat("x", 59),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := capResourceName(tt.input); got != tt.want {
				t.Errorf("capResourceName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	t.Run("long name truncated to 63 with deterministic fnv suffix", func(t *testing.T) {
		got := capResourceName(longName)
		if len(got) != 63 {
			t.Fatalf("len = %d, want 63 (got %q)", len(got), got)
		}
		if !strings.HasPrefix(got, "cap-") {
			t.Errorf("name %q lost the cap- prefix", got)
		}
		// 54 chars of prefix + "-" + 8 hex.
		if got[54] != '-' {
			t.Errorf("name %q missing the hash separator at index 54", got)
		}
		if again := capResourceName(longName); again != got {
			t.Errorf("not deterministic: %q vs %q", got, again)
		}
	})

	t.Run("distinct long names sharing a truncation prefix do not collide", func(t *testing.T) {
		a := capResourceName(longName + "-one")
		b := capResourceName(longName + "-two")
		if a == b {
			t.Errorf("collision: both names map to %q", a)
		}
	})
}

func TestBuildDeploymentNoCapabilitiesGoldenEquivalence(t *testing.T) {
	plain := BuildDeployment(baseParams())

	withFields := baseParams()
	withFields.Capabilities = nil
	withFields.CapabilityDelivery = DeliveryInitContainer
	explicit := BuildDeployment(withFields)

	if !reflect.DeepEqual(plain, explicit) {
		t.Errorf("zero capabilities must be a zero behavioral change:\nplain   = %+v\nexplicit = %+v", plain, explicit)
	}

	pod := plain.Spec.Template.Spec
	if len(pod.InitContainers) != 0 {
		t.Errorf("unexpected initContainers: %v", pod.InitContainers)
	}
	for _, v := range pod.Volumes {
		if v.Name == CapabilitiesVolumeName {
			t.Errorf("unexpected %s volume without capabilities", CapabilitiesVolumeName)
		}
	}
	for _, m := range pod.Containers[0].VolumeMounts {
		if m.Name == CapabilitiesVolumeName {
			t.Errorf("unexpected %s mount without capabilities", CapabilitiesVolumeName)
		}
	}
	if _, ok := plain.Spec.Template.Annotations[CapabilityDeliveryAnnotation]; ok {
		t.Errorf("unexpected %s annotation without capabilities", CapabilityDeliveryAnnotation)
	}
}

func TestBuildDeploymentCapabilityInitContainers(t *testing.T) {
	tests := []struct {
		name     string
		mounts   []CapabilityMount
		delivery CapabilityDeliveryMode
	}{
		{
			name:     "one capability, explicit initContainer mode",
			mounts:   []CapabilityMount{echoMount()},
			delivery: DeliveryInitContainer,
		},
		{
			name:     "one capability, empty mode defaults to initContainer",
			mounts:   []CapabilityMount{echoMount()},
			delivery: "",
		},
		{
			name:     "two capabilities, declaration order preserved",
			mounts:   []CapabilityMount{echoMount(), upperMount()},
			delivery: DeliveryInitContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := baseParams()
			params.Capabilities = tt.mounts
			params.CapabilityDelivery = tt.delivery

			deployment := BuildDeployment(params)
			pod := deployment.Spec.Template.Spec

			if got := deployment.Spec.Template.Annotations[CapabilityDeliveryAnnotation]; got != string(DeliveryInitContainer) {
				t.Errorf("annotation %s = %q, want %q", CapabilityDeliveryAnnotation, got, DeliveryInitContainer)
			}

			if len(pod.InitContainers) != len(tt.mounts) {
				t.Fatalf("got %d initContainers, want %d", len(pod.InitContainers), len(tt.mounts))
			}
			for i, mount := range tt.mounts {
				assertCapInitContainer(t, pod.InitContainers[i], mount)
			}

			assertSharedCapabilityVolume(t, pod)
			assertRunnerCapabilityMount(t, pod.Containers[0])
		})
	}
}

func assertCapInitContainer(t *testing.T, c corev1.Container, mount CapabilityMount) {
	t.Helper()

	if want := "cap-" + mount.Name; c.Name != want {
		t.Errorf("initContainer name = %q, want %q", c.Name, want)
	}
	if c.Image != mount.Artifact {
		t.Errorf("image = %q, want the artifact ref verbatim %q", c.Image, mount.Artifact)
	}
	wantCmd := []string{"/bin/sh", "-c", "cp -r /wheelhouse/. /dst/"}
	if !reflect.DeepEqual(c.Command, wantCmd) {
		t.Errorf("command = %v, want %v", c.Command, wantCmd)
	}

	// subPath isolation: each capability writes only its own subdirectory.
	wantMounts := []corev1.VolumeMount{{
		Name:      CapabilitiesVolumeName,
		MountPath: "/dst",
		SubPath:   mount.Name,
	}}
	if !reflect.DeepEqual(c.VolumeMounts, wantMounts) {
		t.Errorf("volumeMounts = %+v, want %+v", c.VolumeMounts, wantMounts)
	}

	assertQuantity(t, "cpu request", c.Resources.Requests[corev1.ResourceCPU], "10m")
	assertQuantity(t, "memory request", c.Resources.Requests[corev1.ResourceMemory], "16Mi")
	assertQuantity(t, "cpu limit", c.Resources.Limits[corev1.ResourceCPU], "100m")
	assertQuantity(t, "memory limit", c.Resources.Limits[corev1.ResourceMemory], "64Mi")

	sc := c.SecurityContext
	if sc == nil {
		t.Fatal("securityContext missing")
	}
	if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		t.Error("ReadOnlyRootFilesystem must be true")
	}
	if sc.RunAsUser == nil || *sc.RunAsUser != 65532 {
		t.Errorf("RunAsUser = %v, want 65532", sc.RunAsUser)
	}
	if sc.RunAsGroup == nil || *sc.RunAsGroup != 65532 {
		t.Errorf("RunAsGroup = %v, want 65532", sc.RunAsGroup)
	}
	// The restricted base profile must be retained.
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("RunAsNonRoot must be true")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("AllowPrivilegeEscalation must be false")
	}
	if sc.Capabilities == nil || !reflect.DeepEqual(sc.Capabilities.Drop, []corev1.Capability{"ALL"}) {
		t.Errorf("capabilities drop = %+v, want [ALL]", sc.Capabilities)
	}
	if sc.SeccompProfile == nil || sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("seccompProfile = %+v, want RuntimeDefault", sc.SeccompProfile)
	}
}

func assertQuantity(t *testing.T, what string, got resource.Quantity, want string) {
	t.Helper()
	if got.Cmp(resource.MustParse(want)) != 0 {
		t.Errorf("%s = %s, want %s", what, got.String(), want)
	}
}

// assertSharedCapabilityVolume checks the emptyDir appears exactly once no
// matter how many capabilities are attached.
func assertSharedCapabilityVolume(t *testing.T, pod corev1.PodSpec) {
	t.Helper()
	count := 0
	for _, v := range pod.Volumes {
		if v.Name != CapabilitiesVolumeName {
			continue
		}
		count++
		if v.EmptyDir == nil {
			t.Errorf("volume %s is not an emptyDir: %+v", CapabilitiesVolumeName, v.VolumeSource)
		}
	}
	if count != 1 {
		t.Errorf("found %d %s volumes, want exactly 1", count, CapabilitiesVolumeName)
	}
}

func TestBuildDeploymentCapabilityImageVolumes(t *testing.T) {
	tests := []struct {
		name   string
		mounts []CapabilityMount
	}{
		{
			name:   "one capability",
			mounts: []CapabilityMount{echoMount()},
		},
		{
			name:   "two capabilities, declaration order preserved",
			mounts: []CapabilityMount{echoMount(), upperMount()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := baseParams()
			params.Capabilities = tt.mounts
			params.CapabilityDelivery = DeliveryImageVolume

			deployment := BuildDeployment(params)
			pod := deployment.Spec.Template.Spec

			// The annotation reports the mode actually emitted.
			if got := deployment.Spec.Template.Annotations[CapabilityDeliveryAnnotation]; got != string(DeliveryImageVolume) {
				t.Errorf("annotation %s = %q, want %q", CapabilityDeliveryAnnotation, got, DeliveryImageVolume)
			}

			// No copy machinery in this mode: no initContainers, no emptyDir.
			if len(pod.InitContainers) != 0 {
				t.Errorf("unexpected initContainers in imageVolume mode: %v", pod.InitContainers)
			}
			for _, v := range pod.Volumes {
				if v.Name == CapabilitiesVolumeName {
					t.Errorf("unexpected shared %s volume in imageVolume mode", CapabilitiesVolumeName)
				}
			}

			// One image volume per capability, digest ref verbatim.
			var imageVolumes []corev1.Volume
			for _, v := range pod.Volumes {
				if strings.HasPrefix(v.Name, "cap-") {
					imageVolumes = append(imageVolumes, v)
				}
			}
			if len(imageVolumes) != len(tt.mounts) {
				t.Fatalf("got %d image volumes, want %d", len(imageVolumes), len(tt.mounts))
			}
			for i, mount := range tt.mounts {
				v := imageVolumes[i]
				if want := "cap-" + mount.Name; v.Name != want {
					t.Errorf("volume name = %q, want %q", v.Name, want)
				}
				if v.Image == nil {
					t.Fatalf("volume %s is not an image volume: %+v", v.Name, v.VolumeSource)
				}
				if v.Image.Reference != mount.Artifact {
					t.Errorf("image reference = %q, want the artifact ref verbatim %q", v.Image.Reference, mount.Artifact)
				}
				if v.Image.PullPolicy != corev1.PullIfNotPresent {
					t.Errorf("pullPolicy = %q, want %q", v.Image.PullPolicy, corev1.PullIfNotPresent)
				}
			}

			// Runner mounts: per-capability, same on-disk layout as the
			// initContainer path (wheelhouse content at
			// /opt/flokoa/capabilities/<name>).
			var capMounts []corev1.VolumeMount
			for _, m := range pod.Containers[0].VolumeMounts {
				if strings.HasPrefix(m.Name, "cap-") {
					capMounts = append(capMounts, m)
				}
			}
			if len(capMounts) != len(tt.mounts) {
				t.Fatalf("got %d runner capability mounts, want %d", len(capMounts), len(tt.mounts))
			}
			for i, mount := range tt.mounts {
				want := corev1.VolumeMount{
					Name:      "cap-" + mount.Name,
					MountPath: CapabilitiesMountPath + "/" + mount.Name,
					SubPath:   "wheelhouse",
					ReadOnly:  true,
				}
				if !reflect.DeepEqual(capMounts[i], want) {
					t.Errorf("runner mount = %+v, want %+v", capMounts[i], want)
				}
			}

			// The shared emptyDir mount must not leak into this mode.
			for _, m := range pod.Containers[0].VolumeMounts {
				if m.Name == CapabilitiesVolumeName {
					t.Errorf("unexpected shared %s mount in imageVolume mode", CapabilitiesVolumeName)
				}
			}
		})
	}
}

// TestBuildDeploymentUnknownModeFailsSafe pins the lockstep property: an
// unknown mode value emits the initContainer machinery and the annotation
// reports initContainer — never a mode that was not actually emitted.
func TestBuildDeploymentUnknownModeFailsSafe(t *testing.T) {
	params := baseParams()
	params.Capabilities = []CapabilityMount{echoMount()}
	params.CapabilityDelivery = CapabilityDeliveryMode("bogus")

	deployment := BuildDeployment(params)
	pod := deployment.Spec.Template.Spec

	if got := deployment.Spec.Template.Annotations[CapabilityDeliveryAnnotation]; got != string(DeliveryInitContainer) {
		t.Errorf("annotation %s = %q, want fallback %q", CapabilityDeliveryAnnotation, got, DeliveryInitContainer)
	}
	if len(pod.InitContainers) != 1 {
		t.Fatalf("got %d initContainers, want 1 (initContainer fallback)", len(pod.InitContainers))
	}
	assertSharedCapabilityVolume(t, pod)
	assertRunnerCapabilityMount(t, pod.Containers[0])
}

// assertRunnerCapabilityMount checks the runner sees one read-only mount of
// the shared volume at the contract path.
func assertRunnerCapabilityMount(t *testing.T, runner corev1.Container) {
	t.Helper()
	var mounts []corev1.VolumeMount
	for _, m := range runner.VolumeMounts {
		if m.Name == CapabilitiesVolumeName {
			mounts = append(mounts, m)
		}
	}
	want := []corev1.VolumeMount{{
		Name:      CapabilitiesVolumeName,
		MountPath: CapabilitiesMountPath,
		ReadOnly:  true,
	}}
	if !reflect.DeepEqual(mounts, want) {
		t.Errorf("runner capability mounts = %+v, want %+v", mounts, want)
	}
}
