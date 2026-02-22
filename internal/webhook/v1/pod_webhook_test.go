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

package v1

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// testPodCounter is incremented to generate unique Pod names per test.
var testPodCounter int

// sidecarName is the expected name of the injected sidecar init container.
const sidecarName = "jolokia-agent"

// uniquePodName returns a unique pod name for each test invocation.
func uniquePodName(prefix string) string {
	testPodCounter++
	return fmt.Sprintf("%s-%d", prefix, testPodCounter)
}

// newTestPod returns a minimal Pod ready for creation via the envtest API server.
func newTestPod(name string, annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "default",
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "busybox:latest",
				},
			},
		},
	}
}

var _ = Describe("Pod Webhook", func() {
	var (
		obj       *corev1.Pod
		oldObj    *corev1.Pod
		validator PodCustomValidator
		defaulter PodCustomDefaulter
	)

	BeforeEach(func() {
		obj = &corev1.Pod{}
		oldObj = &corev1.Pod{}
		validator = PodCustomValidator{}
		Expect(validator).NotTo(BeNil(), "Expected validator to be initialized")
		defaulter = PodCustomDefaulter{DefaultImage: "ghcr.io/alxgomz/jolokia-agent:2"}
		Expect(defaulter).NotTo(BeNil(), "Expected defaulter to be initialized")
		Expect(oldObj).NotTo(BeNil(), "Expected oldObj to be initialized")
		Expect(obj).NotTo(BeNil(), "Expected obj to be initialized")
	})

	AfterEach(func() {
		// Clean up created pods — best-effort, ignore errors.
	})

	// ─── T013: Pod with jolokia annotation is mutated ────────────────────────
	Context("When creating Pod under Defaulting Webhook", func() {
		It("Should inject sidecar when Pod has jolokia annotations (T013)", func() {
			pod := newTestPod(uniquePodName("t013"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})

			By("creating the Pod through the API server (webhook intercepts)")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("verifying the jolokia-agent init container was injected")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil(), "Expected jolokia-agent init container to be injected")

			By("verifying native sidecar pattern: restartPolicy=Always")
			Expect(sidecar.RestartPolicy).NotTo(BeNil())
			Expect(*sidecar.RestartPolicy).To(Equal(corev1.ContainerRestartPolicyAlways))

			By("verifying SYS_PTRACE capability")
			Expect(sidecar.SecurityContext).NotTo(BeNil())
			Expect(sidecar.SecurityContext.Capabilities).NotTo(BeNil())
			Expect(sidecar.SecurityContext.Capabilities.Add).To(ContainElement(corev1.Capability("SYS_PTRACE")))
			Expect(sidecar.SecurityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("ALL")))

			By("verifying jolokia-tmp emptyDir volume was added")
			var foundVolume bool
			for _, v := range pod.Spec.Volumes {
				if v.Name == "jolokia-tmp" && v.EmptyDir != nil {
					foundVolume = true
					break
				}
			}
			Expect(foundVolume).To(BeTrue(), "Expected jolokia-tmp emptyDir volume")

			By("verifying shareProcessNamespace is set to true")
			Expect(pod.Spec.ShareProcessNamespace).NotTo(BeNil())
			Expect(*pod.Spec.ShareProcessNamespace).To(BeTrue())

			By("verifying sidecar has the volume mount for jolokia-tmp")
			Expect(sidecar.VolumeMounts).To(ContainElement(
				corev1.VolumeMount{Name: "jolokia-tmp", MountPath: "/tmp"},
			))

			By("verifying sidecar args contain port=8778")
			Expect(sidecar.Args).To(ContainElement("port=8778"))
		})

		// ─── T014: Pod without annotations passes through unmodified ─────────
		It("Should not inject sidecar when Pod has no jolokia annotations (T014)", func() {
			pod := newTestPod(uniquePodName("t014"), map[string]string{
				"app.kubernetes.io/name": "myapp",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("verifying no init containers were injected")
			Expect(pod.Spec.InitContainers).To(BeEmpty())

			By("verifying no jolokia-tmp volume was added")
			for _, v := range pod.Spec.Volumes {
				Expect(v.Name).NotTo(Equal("jolokia-tmp"))
			}

			By("verifying shareProcessNamespace was not modified")
			Expect(pod.Spec.ShareProcessNamespace).To(BeNil())
		})

		// ─── T015: shareProcessNamespace handling ────────────────────────────
		It("Should not conflict when shareProcessNamespace is already true (T015a)", func() {
			pod := newTestPod(uniquePodName("t015a"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})
			pod.Spec.ShareProcessNamespace = ptr.To(true)

			By("creating the Pod with shareProcessNamespace already true")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("verifying shareProcessNamespace remains true")
			Expect(pod.Spec.ShareProcessNamespace).NotTo(BeNil())
			Expect(*pod.Spec.ShareProcessNamespace).To(BeTrue())

			By("verifying sidecar was still injected")
			var found bool
			for _, c := range pod.Spec.InitContainers {
				if c.Name == sidecarName {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("Should override shareProcessNamespace from false to true (T015b)", func() {
			pod := newTestPod(uniquePodName("t015b"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})
			pod.Spec.ShareProcessNamespace = ptr.To(false)

			By("creating the Pod with shareProcessNamespace set to false")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("verifying shareProcessNamespace was overridden to true")
			Expect(pod.Spec.ShareProcessNamespace).NotTo(BeNil())
			Expect(*pod.Spec.ShareProcessNamespace).To(BeTrue())
		})
	})

	Context("When creating or updating Pod under Validating Webhook", func() {
		// ─── T018: Pod with invalid annotation key is rejected ─────────────
		It("Should reject Pod with invalid jolokia annotation key (T018)", func() {
			pod := newTestPod(uniquePodName("t018"), map[string]string{
				"jolokia.horoa.net/foobar": "value",
			})

			By("creating the Pod through the API server")
			err := k8sClient.Create(ctx, pod)

			By("verifying admission was denied")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("foobar"))
		})

		// ─── T019: Pod with mixed valid+invalid annotations is rejected ─────
		It("Should reject Pod with mixed valid and invalid annotation keys (T019)", func() {
			pod := newTestPod(uniquePodName("t019"), map[string]string{
				"jolokia.horoa.net/port":   "8778",
				"jolokia.horoa.net/badKey": "value",
			})

			By("creating the Pod through the API server")
			err := k8sClient.Create(ctx, pod)

			By("verifying admission was denied and error lists invalid keys")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("badKey"))
		})

		// ─── T020: Pod with only operator-specific annotations is accepted ──
		It("Should accept Pod with only operator-specific annotations (T020)", func() {
			pod := newTestPod(uniquePodName("t020"), map[string]string{
				"jolokia.horoa.net/mnt":                 "/data",
				"jolokia.horoa.net/rsc-limits-cpu":      "500m",
				"jolokia.horoa.net/rsc-limits-memory":   "256Mi",
				"jolokia.horoa.net/rsc-requests-cpu":    "100m",
				"jolokia.horoa.net/rsc-requests-memory": "128Mi",
				"jolokia.horoa.net/target-process":      "org.apache.catalina.startup.Bootstrap",
				"jolokia.horoa.net/sidecar-image":       "custom-jolokia:latest",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("verifying sidecar was injected (operator annotations trigger injection)")
			var found bool
			for _, c := range pod.Spec.InitContainers {
				if c.Name == sidecarName {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		})
	})

	// ─── Phase 5 (US3): Jolokia Agent Option Pass-Through ───────────────
	Context("When verifying Jolokia option pass-through (US3)", func() {
		// ─── T025: Args contain key=value from annotations ────────────────
		It("Should pass Jolokia options as sorted key=value args (T025)", func() {
			pod := newTestPod(uniquePodName("t025"), map[string]string{
				"jolokia.horoa.net/port": "9090",
				"jolokia.horoa.net/host": "0.0.0.0",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying sidecar args contain both options")
			Expect(sidecar.Args).To(ContainElement("host=0.0.0.0"))
			Expect(sidecar.Args).To(ContainElement("port=9090"))

			By("verifying args are sorted alphabetically")
			Expect(sidecar.Args[0]).To(Equal("host=0.0.0.0"))
			Expect(sidecar.Args[1]).To(Equal("port=9090"))
		})

		// ─── T026: Operator-only annotations produce no jolokia args ──────
		It("Should not include operator annotations in sidecar args (T026)", func() {
			pod := newTestPod(uniquePodName("t026"), map[string]string{
				"jolokia.horoa.net/mnt": "/data",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying sidecar args contain NO jolokia options")
			Expect(sidecar.Args).To(BeEmpty())
		})
	})

	// ─── Phase 6 (US4): Custom Mount Path ─────────────────────────────────
	Context("When verifying custom mount path (US4)", func() {
		It("Should use custom mount path from mnt annotation (T030a)", func() {
			pod := newTestPod(uniquePodName("t030a"), map[string]string{
				"jolokia.horoa.net/port": "8778",
				"jolokia.horoa.net/mnt":  "/opt/jolokia",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying volume mount path is /opt/jolokia")
			Expect(sidecar.VolumeMounts).To(ContainElement(
				corev1.VolumeMount{Name: "jolokia-tmp", MountPath: "/opt/jolokia"},
			))
		})

		It("Should default mount path to /tmp when mnt annotation is absent (T030b)", func() {
			pod := newTestPod(uniquePodName("t030b"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying volume mount path is /tmp (default)")
			Expect(sidecar.VolumeMounts).To(ContainElement(
				corev1.VolumeMount{Name: "jolokia-tmp", MountPath: "/tmp"},
			))
		})
	})

	// ─── Phase 7 (US5): Sidecar Resource Allocation ──────────────────────
	Context("When verifying sidecar resource allocation (US5)", func() {
		It("Should set sidecar resources from all 4 resource annotations (T035a)", func() {
			pod := newTestPod(uniquePodName("t035a"), map[string]string{
				"jolokia.horoa.net/port":                "8778",
				"jolokia.horoa.net/rsc-limits-cpu":      "500m",
				"jolokia.horoa.net/rsc-limits-memory":   "256Mi",
				"jolokia.horoa.net/rsc-requests-cpu":    "100m",
				"jolokia.horoa.net/rsc-requests-memory": "128Mi",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying resource limits")
			Expect(sidecar.Resources.Limits.Cpu().String()).To(Equal("500m"))
			Expect(sidecar.Resources.Limits.Memory().String()).To(Equal("256Mi"))

			By("verifying resource requests")
			Expect(sidecar.Resources.Requests.Cpu().String()).To(Equal("100m"))
			Expect(sidecar.Resources.Requests.Memory().String()).To(Equal("128Mi"))
		})

		It("Should set only limits when only limit annotations present (T035b)", func() {
			pod := newTestPod(uniquePodName("t035b"), map[string]string{
				"jolokia.horoa.net/port":              "8778",
				"jolokia.horoa.net/rsc-limits-cpu":    "1",
				"jolokia.horoa.net/rsc-limits-memory": "512Mi",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying limits are set")
			Expect(sidecar.Resources.Limits).NotTo(BeNil())
			Expect(sidecar.Resources.Limits.Cpu().String()).To(Equal("1"))
			Expect(sidecar.Resources.Limits.Memory().String()).To(Equal("512Mi"))

			By("verifying requests default to limits (K8s API server behavior)")
			Expect(sidecar.Resources.Requests).NotTo(BeNil())
			Expect(sidecar.Resources.Requests.Cpu().String()).To(Equal("1"))
			Expect(sidecar.Resources.Requests.Memory().String()).To(Equal("512Mi"))
		})

		It("Should leave resources empty when no resource annotations present (T035c)", func() {
			pod := newTestPod(uniquePodName("t035c"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying no resources are set")
			Expect(sidecar.Resources.Limits).To(BeNil())
			Expect(sidecar.Resources.Requests).To(BeNil())
		})
	})

	// ─── Phase 8 (US6): Target Process Selection ────────────────────────
	Context("When verifying target process selection (US6)", func() {
		It("Should pass target-process class name as env var (T039a)", func() {
			pod := newTestPod(uniquePodName("t039a"), map[string]string{
				"jolokia.horoa.net/port":           "8778",
				"jolokia.horoa.net/target-process": "org.apache.catalina.startup.Bootstrap",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying target-process is set as env var")
			var foundEnv bool
			for _, e := range sidecar.Env {
				if e.Name == "JOLOKIA_TARGET_PROCESS" && e.Value == "org.apache.catalina.startup.Bootstrap" {
					foundEnv = true
				}
			}
			Expect(foundEnv).To(BeTrue(), "expected JOLOKIA_TARGET_PROCESS env var with class name")

			By("verifying target-process is NOT in args")
			for _, arg := range sidecar.Args {
				Expect(arg).NotTo(HavePrefix("target-process="))
			}
		})

		It("Should pass target-process numeric PID as env var (T039b)", func() {
			pod := newTestPod(uniquePodName("t039b"), map[string]string{
				"jolokia.horoa.net/port":           "8778",
				"jolokia.horoa.net/target-process": "42",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying target-process is set as env var")
			var foundEnv bool
			for _, e := range sidecar.Env {
				if e.Name == "JOLOKIA_TARGET_PROCESS" && e.Value == "42" {
					foundEnv = true
				}
			}
			Expect(foundEnv).To(BeTrue(), "expected JOLOKIA_TARGET_PROCESS env var with PID")

			By("verifying target-process is NOT in args")
			for _, arg := range sidecar.Args {
				Expect(arg).NotTo(HavePrefix("target-process="))
			}
		})

		It("Should not include JOLOKIA_TARGET_PROCESS env var when annotation absent (T039c)", func() {
			pod := newTestPod(uniquePodName("t039c"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying no JOLOKIA_TARGET_PROCESS env var")
			for _, e := range sidecar.Env {
				Expect(e.Name).NotTo(Equal("JOLOKIA_TARGET_PROCESS"))
			}
		})
	})

	// ─── Phase 9 (US7): Idempotent Re-injection Prevention ───────────────
	Context("When verifying idempotent re-injection (US7)", func() {
		It("Should not inject a second sidecar when jolokia-agent already exists (T043)", func() {
			pod := newTestPod(uniquePodName("t043"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})
			// Pre-populate with an existing jolokia-agent init container
			pod.Spec.InitContainers = []corev1.Container{
				{
					Name:  sidecarName,
					Image: "existing-jolokia:v1",
				},
			}

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("verifying only one jolokia-agent init container exists (no duplicate)")
			count := 0
			for _, c := range pod.Spec.InitContainers {
				if c.Name == sidecarName {
					count++
				}
			}
			Expect(count).To(Equal(1))

			By("verifying the existing sidecar was NOT replaced")
			Expect(pod.Spec.InitContainers[0].Image).To(Equal("existing-jolokia:v1"))
		})
	})

	// ─── Phase 10: Sidecar Image Override (T050) ───────────────────────
	Context("When verifying sidecar image override (T050)", func() {
		It("Should use sidecar-image annotation when provided (T050a)", func() {
			pod := newTestPod(uniquePodName("t050a"), map[string]string{
				"jolokia.horoa.net/port":          "8778",
				"jolokia.horoa.net/sidecar-image": "custom-jolokia:v3",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying the sidecar image is from the annotation")
			Expect(sidecar.Image).To(Equal("custom-jolokia:v3"))
		})

		It("Should use operator default image when sidecar-image annotation is absent (T050b)", func() {
			pod := newTestPod(uniquePodName("t050b"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("finding the sidecar init container")
			var sidecar *corev1.Container
			for i := range pod.Spec.InitContainers {
				if pod.Spec.InitContainers[i].Name == sidecarName {
					sidecar = &pod.Spec.InitContainers[i]
					break
				}
			}
			Expect(sidecar).NotTo(BeNil())

			By("verifying the sidecar image is the operator default")
			Expect(sidecar.Image).To(Equal("ghcr.io/alxgomz/jolokia-agent:2"))
		})
	})

	// ─── Phase 10: Edge Cases (T051) ────────────────────────────────────
	Context("When verifying edge cases (T051)", func() {
		It("Should accept Pod with empty annotation value (T051a)", func() {
			pod := newTestPod(uniquePodName("t051a"), map[string]string{
				"jolokia.horoa.net/port": "",
			})

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("verifying sidecar was injected")
			var found bool
			for _, c := range pod.Spec.InitContainers {
				if c.Name == sidecarName {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("Should append sidecar without disrupting existing init containers (T051b)", func() {
			pod := newTestPod(uniquePodName("t051b"), map[string]string{
				"jolokia.horoa.net/port": "8778",
			})
			// Pre-populate with two unrelated init containers
			pod.Spec.InitContainers = []corev1.Container{
				{Name: "init-db", Image: "init-db:latest"},
				{Name: "init-cache", Image: "init-cache:latest"},
			}

			By("creating the Pod through the API server")
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			By("verifying all 3 init containers exist")
			Expect(pod.Spec.InitContainers).To(HaveLen(3))

			By("verifying original init containers are preserved in order")
			Expect(pod.Spec.InitContainers[0].Name).To(Equal("init-db"))
			Expect(pod.Spec.InitContainers[1].Name).To(Equal("init-cache"))

			By("verifying jolokia-agent was appended as the third")
			Expect(pod.Spec.InitContainers[2].Name).To(Equal(sidecarName))
		})

		It("Should accept agentContext (camelCase) but reject agentcontext (lowercase) (T051c)", func() {
			By("creating a Pod with correctly-cased agentContext")
			podGood := newTestPod(uniquePodName("t051c-good"), map[string]string{
				"jolokia.horoa.net/agentContext": "/my-jolokia",
			})
			Expect(k8sClient.Create(ctx, podGood)).To(Succeed())

			By("creating a Pod with incorrectly-cased agentcontext")
			podBad := newTestPod(uniquePodName("t051c-bad"), map[string]string{
				"jolokia.horoa.net/agentcontext": "/my-jolokia",
			})
			err := k8sClient.Create(ctx, podBad)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("agentcontext"))
		})
	})
})
