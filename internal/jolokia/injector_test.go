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
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

// basePod returns a minimal Pod suitable for injection tests.
func basePod() *corev1.Pod {
	return &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "myapp:latest"},
			},
		},
	}
}

// baseParsed returns a minimal ParsedAnnotations with defaults.
func baseParsed() ParsedAnnotations {
	return ParsedAnnotations{
		JolokiaOptions: make(map[string]string),
		MountPath:      DefaultMountPath,
	}
}

// targetProcessEnvVar is the expected env var name for target-process.
const targetProcessEnvVar = "JOLOKIA_TARGET_PROCESS"

// findInitContainer returns the jolokia-agent init container, or nil.
func findInitContainer(pod *corev1.Pod) *corev1.Container {
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == sidecarContainerName {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

// findVolume returns the volume with the given name, or nil.
func findVolume(pod *corev1.Pod, name string) *corev1.Volume {
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == name {
			return &pod.Spec.Volumes[i]
		}
	}
	return nil
}

func TestInjectSidecar(t *testing.T) { //nolint:gocyclo
	tests := []struct {
		name         string
		pod          *corev1.Pod
		parsed       ParsedAnnotations
		defaultImage string
		verify       func(t *testing.T, pod *corev1.Pod, result InjectionResult)
	}{
		{
			name: "basic injection",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{"port": "8778"},
				MountPath:      DefaultMountPath,
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				if !result.Injected {
					t.Fatal("expected Injected=true")
				}
				if result.Skipped {
					t.Fatal("expected Skipped=false")
				}

				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}

				if sidecar.RestartPolicy == nil || *sidecar.RestartPolicy != corev1.ContainerRestartPolicyAlways {
					t.Errorf("expected RestartPolicy=Always, got %v", sidecar.RestartPolicy)
				}

				if sidecar.Image != HardcodedDefaultImage {
					t.Errorf("expected image %q, got %q", HardcodedDefaultImage, sidecar.Image)
				}

				foundArg := false
				for _, arg := range sidecar.Args {
					if arg == "port=8778" {
						foundArg = true
					}
				}
				if !foundArg {
					t.Errorf("expected args to contain port=8778, got %v", sidecar.Args)
				}

				vol := findVolume(pod, volumeName)
				if vol == nil {
					t.Fatal("expected volume jolokia-tmp")
				}
				if vol.EmptyDir == nil {
					t.Fatal("expected EmptyDir volume source")
				}

				if pod.Spec.ShareProcessNamespace == nil || !*pod.Spec.ShareProcessNamespace {
					t.Error("expected ShareProcessNamespace=true")
				}
			},
		},
		{
			name: "idempotent skip",
			pod: func() *corev1.Pod {
				p := basePod()
				p.Spec.InitContainers = []corev1.Container{
					{Name: sidecarContainerName, Image: "existing:v1"},
				}
				return p
			}(),
			parsed:       baseParsed(),
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				if result.Injected {
					t.Fatal("expected Injected=false for idempotent skip")
				}
				if !result.Skipped {
					t.Fatal("expected Skipped=true")
				}
				if result.SkipReason == "" {
					t.Fatal("expected non-empty SkipReason")
				}
				if !strings.Contains(result.SkipReason, "already present") {
					t.Errorf("expected SkipReason to contain 'already present', got %q", result.SkipReason)
				}
				if len(pod.Spec.InitContainers) != 1 {
					t.Errorf("expected 1 init container (no duplicate), got %d", len(pod.Spec.InitContainers))
				}
			},
		},
		{
			name: "security context correctness",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{"port": "8778"},
				MountPath:      DefaultMountPath,
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}

				sc := sidecar.SecurityContext
				if sc == nil {
					t.Fatal("expected non-nil SecurityContext")
				}

				if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
					t.Error("expected ReadOnlyRootFilesystem=true")
				}
				if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
					t.Error("expected AllowPrivilegeEscalation=false")
				}
				if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
					t.Error("expected RunAsNonRoot=true")
				}

				if sc.Capabilities == nil {
					t.Fatal("expected non-nil Capabilities")
				}

				foundDropAll := false
				for _, cap := range sc.Capabilities.Drop {
					if cap == "ALL" {
						foundDropAll = true
					}
				}
				if !foundDropAll {
					t.Errorf("expected Capabilities.Drop to contain ALL, got %v", sc.Capabilities.Drop)
				}

				foundAddPtrace := false
				for _, cap := range sc.Capabilities.Add {
					if cap == "SYS_PTRACE" {
						foundAddPtrace = true
					}
				}
				if !foundAddPtrace {
					t.Errorf("expected Capabilities.Add to contain SYS_PTRACE, got %v", sc.Capabilities.Add)
				}

				if sc.SeccompProfile == nil {
					t.Fatal("expected non-nil SeccompProfile")
				}
				if sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
					t.Errorf("expected SeccompProfile.Type=RuntimeDefault, got %v", sc.SeccompProfile.Type)
				}
			},
		},
		{
			name: "volume mount path default /tmp",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				found := false
				for _, vm := range sidecar.VolumeMounts {
					if vm.Name == volumeName {
						found = true
						if vm.MountPath != DefaultMountPath {
							t.Errorf("expected mount path %s, got %q", DefaultMountPath, vm.MountPath)
						}
					}
				}
				if !found {
					t.Fatal("expected volume mount for jolokia-tmp")
				}
			},
		},
		{
			name: "custom mount path",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      "/opt/jolokia",
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				for _, vm := range sidecar.VolumeMounts {
					if vm.Name == volumeName {
						if vm.MountPath != "/opt/jolokia" {
							t.Errorf("expected mount path /opt/jolokia, got %q", vm.MountPath)
						}
						return
					}
				}
				t.Fatal("expected volume mount for jolokia-tmp")
			},
		},
		{
			name: "image resolution - annotation overrides all",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
				SidecarImage:   "custom:v3",
			},
			defaultImage: "flag-image:v1",
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				if sidecar.Image != "custom:v3" {
					t.Errorf("expected image custom:v3, got %q", sidecar.Image)
				}
			},
		},
		{
			name: "image resolution - flag overrides hardcoded",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
				SidecarImage:   "",
			},
			defaultImage: "flag-image:v1",
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				if sidecar.Image != "flag-image:v1" {
					t.Errorf("expected image flag-image:v1, got %q", sidecar.Image)
				}
			},
		},
		{
			name: "image resolution - hardcoded fallback",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
				SidecarImage:   "",
			},
			defaultImage: "",
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				if sidecar.Image != HardcodedDefaultImage {
					t.Errorf("expected image %q, got %q", HardcodedDefaultImage, sidecar.Image)
				}
			},
		},
		{
			name: "args sorted deterministically",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{
					"port":    "8778",
					"host":    "0.0.0.0",
					"agentId": "test",
				},
				MountPath: DefaultMountPath,
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				want := []string{"agentId=test", "host=0.0.0.0", "port=8778"}
				if !reflect.DeepEqual(result.OptionsApplied, want) {
					t.Errorf("expected args %v, got %v", want, result.OptionsApplied)
				}
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				if !reflect.DeepEqual(sidecar.Args, want) {
					t.Errorf("expected container args %v, got %v", want, sidecar.Args)
				}
			},
		},
		{
			name: "target process sets env var, not args",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{"port": "8778"},
				MountPath:      DefaultMountPath,
				TargetProcess:  "org.apache.catalina.startup.Bootstrap",
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				wantArgs := []string{"port=8778"}
				if !reflect.DeepEqual(result.OptionsApplied, wantArgs) {
					t.Errorf("expected args %v, got %v", wantArgs, result.OptionsApplied)
				}
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				var found bool
				for _, e := range sidecar.Env {
					if e.Name == targetProcessEnvVar && e.Value == "org.apache.catalina.startup.Bootstrap" {
						found = true
					}
				}
				if !found {
					t.Error("expected JOLOKIA_TARGET_PROCESS env var")
				}
			},
		},
		{
			name: "target process numeric PID sets env var",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
				TargetProcess:  "42",
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				if len(result.OptionsApplied) != 0 {
					t.Errorf("expected no args, got %v", result.OptionsApplied)
				}
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				var found bool
				for _, e := range sidecar.Env {
					if e.Name == targetProcessEnvVar && e.Value == "42" {
						found = true
					}
				}
				if !found {
					t.Error("expected JOLOKIA_TARGET_PROCESS env var with value 42")
				}
			},
		},
		{
			name: "no target process - no env var",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{"port": "8778"},
				MountPath:      DefaultMountPath,
				TargetProcess:  "",
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				want := []string{"port=8778"}
				if !reflect.DeepEqual(result.OptionsApplied, want) {
					t.Errorf("expected args %v, got %v", want, result.OptionsApplied)
				}
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				for _, e := range sidecar.Env {
					if e.Name == targetProcessEnvVar {
						t.Error("expected no JOLOKIA_TARGET_PROCESS env var")
					}
				}
			},
		},
		{
			name: "resource limits and requests",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
				Resources: SidecarResources{
					LimitsCPU:      "500m",
					LimitsMemory:   "256Mi",
					RequestsCPU:    "100m",
					RequestsMemory: "128Mi",
				},
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}

				wantLimitCPU := resource.MustParse("500m")
				gotLimitCPU := sidecar.Resources.Limits[corev1.ResourceCPU]
				if !wantLimitCPU.Equal(gotLimitCPU) {
					t.Errorf("expected limits CPU 500m, got %v", gotLimitCPU.String())
				}

				wantLimitMem := resource.MustParse("256Mi")
				gotLimitMem := sidecar.Resources.Limits[corev1.ResourceMemory]
				if !wantLimitMem.Equal(gotLimitMem) {
					t.Errorf("expected limits memory 256Mi, got %v", gotLimitMem.String())
				}

				wantReqCPU := resource.MustParse("100m")
				gotReqCPU := sidecar.Resources.Requests[corev1.ResourceCPU]
				if !wantReqCPU.Equal(gotReqCPU) {
					t.Errorf("expected requests CPU 100m, got %v", gotReqCPU.String())
				}

				wantReqMem := resource.MustParse("128Mi")
				gotReqMem := sidecar.Resources.Requests[corev1.ResourceMemory]
				if !wantReqMem.Equal(gotReqMem) {
					t.Errorf("expected requests memory 128Mi, got %v", gotReqMem.String())
				}
			},
		},
		{
			name: "partial resources - only limits",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
				Resources: SidecarResources{
					LimitsCPU:    "1",
					LimitsMemory: "512Mi",
				},
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}

				if sidecar.Resources.Limits == nil {
					t.Fatal("expected non-nil Limits")
				}
				wantCPU := resource.MustParse("1")
				gotCPU := sidecar.Resources.Limits[corev1.ResourceCPU]
				if !wantCPU.Equal(gotCPU) {
					t.Errorf("expected limits CPU 1, got %v", gotCPU.String())
				}

				if sidecar.Resources.Requests != nil {
					t.Errorf("expected nil Requests, got %v", sidecar.Resources.Requests)
				}
			},
		},
		{
			name: "no resources",
			pod:  basePod(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				sidecar := findInitContainer(pod)
				if sidecar == nil {
					t.Fatal("expected init container jolokia-agent")
				}
				if sidecar.Resources.Limits != nil {
					t.Errorf("expected nil Limits, got %v", sidecar.Resources.Limits)
				}
				if sidecar.Resources.Requests != nil {
					t.Errorf("expected nil Requests, got %v", sidecar.Resources.Requests)
				}
			},
		},
		{
			name: "existing init containers preserved",
			pod: func() *corev1.Pod {
				p := basePod()
				p.Spec.InitContainers = []corev1.Container{
					{Name: "init-db", Image: "db-init:latest"},
				}
				return p
			}(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{"port": "8778"},
				MountPath:      DefaultMountPath,
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				if len(pod.Spec.InitContainers) != 2 {
					t.Fatalf("expected 2 init containers, got %d", len(pod.Spec.InitContainers))
				}
				if pod.Spec.InitContainers[0].Name != "init-db" {
					t.Errorf("expected first init container to be init-db, got %q", pod.Spec.InitContainers[0].Name)
				}
				if pod.Spec.InitContainers[1].Name != sidecarContainerName {
					t.Errorf("expected second init container to be jolokia-agent, got %q", pod.Spec.InitContainers[1].Name)
				}
			},
		},
		{
			name: "existing volumes preserved",
			pod: func() *corev1.Pod {
				p := basePod()
				p.Spec.Volumes = []corev1.Volume{
					{
						Name: "config-vol",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"},
							},
						},
					},
				}
				return p
			}(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				if len(pod.Spec.Volumes) != 2 {
					t.Fatalf("expected 2 volumes, got %d", len(pod.Spec.Volumes))
				}
				if pod.Spec.Volumes[0].Name != "config-vol" {
					t.Errorf("expected first volume to be config-vol, got %q", pod.Spec.Volumes[0].Name)
				}
				if pod.Spec.Volumes[1].Name != volumeName {
					t.Errorf("expected second volume to be jolokia-tmp, got %q", pod.Spec.Volumes[1].Name)
				}
			},
		},
		{
			name: "volume deduplication",
			pod: func() *corev1.Pod {
				p := basePod()
				p.Spec.Volumes = []corev1.Volume{
					{
						Name: volumeName,
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				}
				return p
			}(),
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				MountPath:      DefaultMountPath,
			},
			defaultImage: HardcodedDefaultImage,
			verify: func(t *testing.T, pod *corev1.Pod, result InjectionResult) {
				count := 0
				for _, v := range pod.Spec.Volumes {
					if v.Name == volumeName {
						count++
					}
				}
				if count != 1 {
					t.Errorf("expected exactly 1 volume named %q, got %d", volumeName, count)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := InjectSidecar(tt.pod, tt.parsed, tt.defaultImage)
			tt.verify(t, tt.pod, result)
		})
	}
}

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name   string
		parsed ParsedAnnotations
		want   []string
	}{
		{
			name: "empty options no target",
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
			},
			want: []string{},
		},
		{
			name: "single option",
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{"port": "8778"},
			},
			want: []string{"port=8778"},
		},
		{
			name: "multiple options sorted",
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{
					"port":    "8778",
					"host":    "0.0.0.0",
					"agentId": "test",
				},
			},
			want: []string{"agentId=test", "host=0.0.0.0", "port=8778"},
		},
		{
			name: "target process only - not included in args",
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{},
				TargetProcess:  "org.Main",
			},
			want: []string{},
		},
		{
			name: "options with target process - only options in args",
			parsed: ParsedAnnotations{
				JolokiaOptions: map[string]string{"port": "9090"},
				TargetProcess:  "42",
			},
			want: []string{"port=9090"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildArgs(tt.parsed)
			if len(got) == 0 && len(tt.want) == 0 {
				return // both empty, pass
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyResources(t *testing.T) {
	tests := []struct {
		name      string
		resources SidecarResources
		verify    func(t *testing.T, container *corev1.Container)
	}{
		{
			name:      "all resources set",
			resources: SidecarResources{LimitsCPU: "500m", LimitsMemory: "256Mi", RequestsCPU: "100m", RequestsMemory: "128Mi"},
			verify: func(t *testing.T, c *corev1.Container) {
				if c.Resources.Limits == nil || c.Resources.Requests == nil {
					t.Fatal("expected both Limits and Requests to be set")
				}
				if !resource.MustParse("500m").Equal(c.Resources.Limits[corev1.ResourceCPU]) {
					t.Errorf("unexpected limits CPU: %v", c.Resources.Limits[corev1.ResourceCPU])
				}
				if !resource.MustParse("256Mi").Equal(c.Resources.Limits[corev1.ResourceMemory]) {
					t.Errorf("unexpected limits memory: %v", c.Resources.Limits[corev1.ResourceMemory])
				}
				if !resource.MustParse("100m").Equal(c.Resources.Requests[corev1.ResourceCPU]) {
					t.Errorf("unexpected requests CPU: %v", c.Resources.Requests[corev1.ResourceCPU])
				}
				if !resource.MustParse("128Mi").Equal(c.Resources.Requests[corev1.ResourceMemory]) {
					t.Errorf("unexpected requests memory: %v", c.Resources.Requests[corev1.ResourceMemory])
				}
			},
		},
		{
			name:      "no resources",
			resources: SidecarResources{},
			verify: func(t *testing.T, c *corev1.Container) {
				if c.Resources.Limits != nil {
					t.Errorf("expected nil Limits, got %v", c.Resources.Limits)
				}
				if c.Resources.Requests != nil {
					t.Errorf("expected nil Requests, got %v", c.Resources.Requests)
				}
			},
		},
		{
			name:      "only CPU limit",
			resources: SidecarResources{LimitsCPU: "2"},
			verify: func(t *testing.T, c *corev1.Container) {
				if c.Resources.Limits == nil {
					t.Fatal("expected non-nil Limits")
				}
				if !resource.MustParse("2").Equal(c.Resources.Limits[corev1.ResourceCPU]) {
					t.Errorf("unexpected limits CPU: %v", c.Resources.Limits[corev1.ResourceCPU])
				}
				if _, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
					t.Error("expected no memory limit")
				}
				if c.Resources.Requests != nil {
					t.Errorf("expected nil Requests, got %v", c.Resources.Requests)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := &corev1.Container{Name: "test"}
			applyResources(container, tt.resources)
			tt.verify(t, container)
		})
	}
}

func TestHasVolume(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{Name: "existing-vol"},
			},
		},
	}

	if !hasVolume(pod, "existing-vol") {
		t.Error("expected hasVolume=true for existing-vol")
	}
	if hasVolume(pod, "nonexistent") {
		t.Error("expected hasVolume=false for nonexistent")
	}

	emptyPod := &corev1.Pod{}
	if hasVolume(emptyPod, "any") {
		t.Error("expected hasVolume=false for empty pod")
	}
}

// Compile-time assertion that ptr.To is used properly in the injector.
var _ = ptr.To(true)
