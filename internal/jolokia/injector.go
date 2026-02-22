/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package jolokia

import (
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

// HardcodedDefaultImage is the fallback sidecar image used when neither the
// annotation nor the CLI flag provides an image reference.
const HardcodedDefaultImage = "ghcr.io/alxgomz/jolokia-agent:2"

// sidecarContainerName is the well-known name for the injected Jolokia sidecar.
const sidecarContainerName = "jolokia-agent"

// volumeName is the well-known name for the emptyDir volume shared between
// the sidecar and the main container.
const volumeName = "jolokia-tmp"

// InjectionResult describes what the injector did to a Pod.
type InjectionResult struct {
	// Injected is true if the sidecar was added.
	Injected bool

	// Skipped is true if injection was skipped (already injected).
	Skipped bool

	// SkipReason explains why injection was skipped.
	SkipReason string

	// OptionsApplied lists the args passed to the sidecar container.
	OptionsApplied []string
}

// InjectSidecar mutates a Pod in-place by appending a Jolokia native sidecar
// init container, an emptyDir volume, and setting shareProcessNamespace.
// If the Pod already contains an init container named "jolokia-agent", injection
// is skipped to ensure idempotency.
func InjectSidecar(pod *corev1.Pod, parsed ParsedAnnotations, defaultImage string) InjectionResult {
	// Idempotency: skip if sidecar already present.
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == sidecarContainerName {
			return InjectionResult{
				Skipped:    true,
				SkipReason: "sidecar already present",
			}
		}
	}

	// Resolve image: annotation > CLI flag > hardcoded default.
	image := HardcodedDefaultImage
	if defaultImage != "" {
		image = defaultImage
	}
	if parsed.SidecarImage != "" {
		image = parsed.SidecarImage
	}

	// Build Jolokia args from options, sorted alphabetically for determinism.
	args := buildArgs(parsed)

	// Build the sidecar container.
	sidecar := buildSidecarContainer(parsed, image, args)

	// Apply resource requests/limits if specified.
	applyResources(&sidecar, parsed.Resources)

	// Append sidecar init container.
	pod.Spec.InitContainers = append(pod.Spec.InitContainers, sidecar)

	// Add emptyDir volume if not already present.
	if !hasVolume(pod, volumeName) {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	// Enable shared PID namespace.
	pod.Spec.ShareProcessNamespace = ptr.To(true)

	return InjectionResult{
		Injected:       true,
		OptionsApplied: args,
	}
}

// buildArgs constructs the sorted list of container args from parsed annotations.
// Only Jolokia JVM agent options are included, formatted as "key=value".
// Operator-specific annotations (target-process, mnt, resources, sidecar-image)
// are handled separately and never appear in args.
func buildArgs(parsed ParsedAnnotations) []string {
	keys := make([]string, 0, len(parsed.JolokiaOptions))
	for k := range parsed.JolokiaOptions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	args := make([]string, 0, len(keys))
	for _, k := range keys {
		args = append(args, fmt.Sprintf("%s=%s", k, parsed.JolokiaOptions[k]))
	}
	return args
}

// buildSidecarContainer creates the Jolokia native sidecar container spec
// following KEP-753 (restartPolicy: Always) with hardened security context.
// If a target process is specified, it is passed as the environment variable
// JOLOKIA_TARGET_PROCESS rather than as a container arg.
func buildSidecarContainer(parsed ParsedAnnotations, image string, args []string) corev1.Container {
	var env []corev1.EnvVar
	if parsed.TargetProcess != "" {
		env = append(env, corev1.EnvVar{
			Name:  "JOLOKIA_TARGET_PROCESS",
			Value: parsed.TargetProcess,
		})
	}
	return corev1.Container{
		Name:          sidecarContainerName,
		Image:         image,
		Args:          args,
		Env:           env,
		RestartPolicy: ptr.To(corev1.ContainerRestartPolicyAlways),
		SecurityContext: &corev1.SecurityContext{
			ReadOnlyRootFilesystem:   ptr.To(true),
			AllowPrivilegeEscalation: ptr.To(false),
			RunAsNonRoot:             ptr.To(true),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
				Add:  []corev1.Capability{"SYS_PTRACE"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      volumeName,
				MountPath: parsed.MountPath,
			},
		},
	}
}

// applyResources sets resource requests and limits on the container
// from the parsed annotation values. Only non-empty values are applied.
func applyResources(container *corev1.Container, res SidecarResources) {
	limits := corev1.ResourceList{}
	requests := corev1.ResourceList{}

	if res.LimitsCPU != "" {
		limits[corev1.ResourceCPU] = resource.MustParse(res.LimitsCPU)
	}
	if res.LimitsMemory != "" {
		limits[corev1.ResourceMemory] = resource.MustParse(res.LimitsMemory)
	}
	if res.RequestsCPU != "" {
		requests[corev1.ResourceCPU] = resource.MustParse(res.RequestsCPU)
	}
	if res.RequestsMemory != "" {
		requests[corev1.ResourceMemory] = resource.MustParse(res.RequestsMemory)
	}

	if len(limits) > 0 || len(requests) > 0 {
		container.Resources = corev1.ResourceRequirements{}
		if len(limits) > 0 {
			container.Resources.Limits = limits
		}
		if len(requests) > 0 {
			container.Resources.Requests = requests
		}
	}
}

// hasVolume checks if a Pod already has a volume with the given name.
func hasVolume(pod *corev1.Pod, name string) bool {
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == name {
			return true
		}
	}
	return false
}
