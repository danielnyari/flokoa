package builder

import (
	"fmt"
	"hash/fnv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// CapabilityDeliveryMode selects how capability wheelhouse artifacts reach
// runner pods (roadmap 09). The mode is a per-cluster operator setting, never
// a per-Agent knob.
type CapabilityDeliveryMode string

const (
	// DeliveryInitContainer copies each artifact's wheelhouse into a shared
	// emptyDir via one initContainer per capability. Default; works on every
	// cluster.
	DeliveryInitContainer CapabilityDeliveryMode = "initContainer"

	// DeliveryImageVolume mounts each artifact directly as a read-only image
	// volume (no copy, no initContainers; kubelet-cached layers shared across
	// pods). Requires the ImageVolume feature gate (beta) and containerd 2.x;
	// selected per-cluster by the operator's delivery resolver
	// (internal/infra/delivery), never assumed.
	DeliveryImageVolume CapabilityDeliveryMode = "imageVolume"
)

const (
	// CapabilitiesVolumeName names the shared emptyDir that carries every
	// attached capability's wheelhouse (initContainer mode).
	CapabilitiesVolumeName = "flokoa-capabilities"

	// CapabilitiesMountPath is where the runner reads wheelhouses from,
	// matching DEFAULT_CAPABILITIES_ROOT in flokoa_runner/capabilities.py
	// (runtime contract §4).
	CapabilitiesMountPath = "/opt/flokoa/capabilities"

	// CapabilityDeliveryAnnotation records the effective delivery mode on
	// the pod template for at-a-glance debugging.
	CapabilityDeliveryAnnotation = "flokoa.ai/capability-delivery"

	// capInitContainerDst is where each copy initContainer mounts its own
	// subPath-isolated slice of the shared volume.
	capInitContainerDst = "/dst"

	// capNamePrefix prefixes per-capability resource names: initContainers
	// (initContainer mode) and image volumes (imageVolume mode).
	capNamePrefix = "cap-"

	// capImageVolumeSubPath descends into the artifact image's /wheelhouse
	// directory (imageVolume mode), so the runner sees the identical layout
	// in both delivery modes and install_capabilities is unchanged.
	capImageVolumeSubPath = "wheelhouse"

	// maxContainerNameLen is the Kubernetes container-name limit (DNS label).
	maxContainerNameLen = 63

	// runnerUID is the runner image's non-root uid/gid; copy initContainers
	// run as it explicitly (busybox defaults to root) so the wheelhouse files
	// land readable by the runner.
	runnerUID int64 = 65532
)

// Copy initContainer resources: copying a wheelhouse is tiny, bounded work.
const (
	capInitCPURequest    = "10m"
	capInitMemoryRequest = "16Mi"
	capInitCPULimit      = "100m"
	capInitMemoryLimit   = "64Mi"
)

// CapabilityMount is one attached Capability's delivery input.
type CapabilityMount struct {
	// Name is the Capability CR name, which is also the wheelhouse directory
	// name under CapabilitiesMountPath.
	Name string
	// Artifact is the digest-pinned OCI reference, passed verbatim from the
	// Capability CR (the kubelet pulls by digest; content addressing does the
	// integrity work).
	Artifact string
}

// effectiveCapabilityDelivery resolves the delivery mode. Anything that is
// not exactly DeliveryImageVolume (including empty) resolves to
// DeliveryInitContainer: the builder fails safe to the path that works on
// every cluster, and because every emission helper and the pod annotation
// key off this one function, the annotation always reports what was
// actually emitted.
func effectiveCapabilityDelivery(params DeploymentParams) CapabilityDeliveryMode {
	if params.CapabilityDelivery == DeliveryImageVolume {
		return DeliveryImageVolume
	}
	return DeliveryInitContainer
}

// capabilityInitContainers emits one wheelhouse-copy initContainer per
// attached capability (initContainer mode; other modes emit none).
func capabilityInitContainers(params DeploymentParams) []corev1.Container {
	if len(params.Capabilities) == 0 || effectiveCapabilityDelivery(params) != DeliveryInitContainer {
		return nil
	}

	containers := make([]corev1.Container, 0, len(params.Capabilities))
	for _, mount := range params.Capabilities {
		containers = append(containers, corev1.Container{
			Name:  capResourceName(mount.Name),
			Image: mount.Artifact,
			// The trailing "/." copies directory contents (including
			// dotfiles) rather than nesting /wheelhouse under /dst.
			Command: []string{"/bin/sh", "-c", "cp -r /wheelhouse/. /dst/"},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      CapabilitiesVolumeName,
				MountPath: capInitContainerDst,
				// subPath confines each copy to its own subdirectory: one
				// capability's copy step cannot touch a sibling's directory.
				SubPath: mount.Name,
			}},
			Resources:       capInitContainerResources(),
			SecurityContext: capInitContainerSecurityContext(),
		})
	}
	return containers
}

// capabilityVolumes returns the delivery volumes: the shared emptyDir every
// initContainer copies into (initContainer mode), or one image volume per
// capability referencing the digest-pinned artifact (imageVolume mode).
func capabilityVolumes(params DeploymentParams) []corev1.Volume {
	if len(params.Capabilities) == 0 {
		return nil
	}
	if effectiveCapabilityDelivery(params) == DeliveryImageVolume {
		volumes := make([]corev1.Volume, 0, len(params.Capabilities))
		for _, mount := range params.Capabilities {
			volumes = append(volumes, corev1.Volume{
				Name: capResourceName(mount.Name),
				VolumeSource: corev1.VolumeSource{
					Image: &corev1.ImageVolumeSource{
						Reference:  mount.Artifact,
						PullPolicy: corev1.PullIfNotPresent,
					},
				},
			})
		}
		return volumes
	}
	return []corev1.Volume{{
		Name: CapabilitiesVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}}
}

// capabilityRunnerMounts returns the runner container's read-only view of the
// delivered wheelhouses: one mount of the shared emptyDir (initContainer
// mode), or one mount per image volume at CapabilitiesMountPath/<name> with
// subPath "wheelhouse" (imageVolume mode) — the same on-disk layout either
// way.
func capabilityRunnerMounts(params DeploymentParams) []corev1.VolumeMount {
	if len(params.Capabilities) == 0 {
		return nil
	}
	if effectiveCapabilityDelivery(params) == DeliveryImageVolume {
		mounts := make([]corev1.VolumeMount, 0, len(params.Capabilities))
		for _, mount := range params.Capabilities {
			mounts = append(mounts, corev1.VolumeMount{
				Name:      capResourceName(mount.Name),
				MountPath: CapabilitiesMountPath + "/" + mount.Name,
				SubPath:   capImageVolumeSubPath,
				ReadOnly:  true,
			})
		}
		return mounts
	}
	return []corev1.VolumeMount{{
		Name:      CapabilitiesVolumeName,
		MountPath: CapabilitiesMountPath,
		ReadOnly:  true,
	}}
}

// capResourceName returns "cap-" + name, truncated to the 63-char DNS-label
// limit (shared by container and volume names) with a deterministic 8-hex
// fnv suffix when it would overflow, so distinct long names cannot collide
// after truncation. Used for initContainer names (initContainer mode) and
// volume names (imageVolume mode).
func capResourceName(name string) string {
	full := capNamePrefix + name
	if len(full) <= maxContainerNameLen {
		return full
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(full)) // fnv writes never fail
	suffix := fmt.Sprintf("%08x", h.Sum32())
	return full[:maxContainerNameLen-len(suffix)-1] + "-" + suffix
}

func capInitContainerResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(capInitCPURequest),
			corev1.ResourceMemory: resource.MustParse(capInitMemoryRequest),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(capInitCPULimit),
			corev1.ResourceMemory: resource.MustParse(capInitMemoryLimit),
		},
	}
}

// capInitContainerSecurityContext is the runner profile hardened further:
// read-only rootfs (the only write target is the volume mount) and explicit
// non-root uid/gid — busybox runs as root by default, so RunAsNonRoot alone
// would fail the pod.
func capInitContainerSecurityContext() *corev1.SecurityContext {
	sc := RestrictedContainerSecurityContext()
	sc.ReadOnlyRootFilesystem = boolPtr(true)
	sc.RunAsUser = int64Ptr(runnerUID)
	sc.RunAsGroup = int64Ptr(runnerUID)
	return sc
}

func int64Ptr(v int64) *int64 {
	return &v
}
