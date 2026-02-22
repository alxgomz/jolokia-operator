//go:build e2e
// +build e2e

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

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/alxgomz/jolokia-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "jolokia-operator-system"

// testNamespace is where webhook test Pods are created (must not be excluded by namespaceSelector)
const testNamespace = "default"

// serviceAccountName created for the project
const serviceAccountName = "jolokia-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "jolokia-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "jolokia-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace)
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace)
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=jolokia-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			verifyControllerPodReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", controllerPodName, "-n", namespace,
					"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"), "Controller pod not ready")
			}
			Eventually(verifyControllerPodReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			By("waiting for the webhook service endpoints to be ready")
			verifyWebhookEndpointsReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpointslices.discovery.k8s.io", "-n", namespace,
					"-l", "kubernetes.io/service-name=jolokia-operator-webhook-service",
					"-o", "jsonpath={range .items[*]}{range .endpoints[*]}{.addresses[*]}{end}{end}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Webhook endpoints should exist")
				g.Expect(output).ShouldNot(BeEmpty(), "Webhook endpoints not yet ready")
			}
			Eventually(verifyWebhookEndpointsReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying the mutating webhook server is ready")
			verifyMutatingWebhookReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mutatingwebhookconfigurations.admissionregistration.k8s.io",
					"jolokia-operator-mutating-webhook-configuration",
					"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "MutatingWebhookConfiguration should exist")
				g.Expect(output).ShouldNot(BeEmpty(), "Mutating webhook CA bundle not yet injected")
			}
			Eventually(verifyMutatingWebhookReady, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying the validating webhook server is ready")
			verifyValidatingWebhookReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "validatingwebhookconfigurations.admissionregistration.k8s.io",
					"jolokia-operator-validating-webhook-configuration",
					"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "ValidatingWebhookConfiguration should exist")
				g.Expect(output).ShouldNot(BeEmpty(), "Validating webhook CA bundle not yet injected")
			}
			Eventually(verifyValidatingWebhookReady, 3*time.Minute, time.Second).Should(Succeed())

			By("waiting additional time for webhook server to stabilize")
			time.Sleep(5 * time.Second)

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": [
								"for i in $(seq 1 30); do curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics && exit 0 || sleep 2; done; exit 1"
							],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, namespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		It("should provisioned cert-manager", func() {
			By("validating that cert-manager has the certificate Secret")
			verifyCertManager := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertManager).Should(Succeed())
		})

		It("should have CA injection for mutating webhooks", func() {
			By("checking CA injection for mutating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"mutatingwebhookconfigurations.admissionregistration.k8s.io",
					"jolokia-operator-mutating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				mwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(mwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		It("should have CA injection for validating webhooks", func() {
			By("checking CA injection for validating webhooks")
			verifyCAInjection := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"validatingwebhookconfigurations.admissionregistration.k8s.io",
					"jolokia-operator-validating-webhook-configuration",
					"-o", "go-template={{ range .webhooks }}{{ .clientConfig.caBundle }}{{ end }}")
				vwhOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(vwhOutput)).To(BeNumerically(">", 10))
			}
			Eventually(verifyCAInjection).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks
	})

	Context("Webhook", func() {
		const sidecarName = "jolokia-agent"
		const volumeName = "jolokia-tmp"

		AfterEach(func() {
			// Clean up any test pods left in the test namespace
			for _, name := range []string{
				"test-basic-injection",
				"test-invalid-annotation",
				"test-no-annotations",
				"test-full-config",
			} {
				cmd := exec.Command("kubectl", "delete", "pod", name,
					"-n", testNamespace, "--ignore-not-found", "--grace-period=0", "--force")
				_, _ = utils.Run(cmd)
			}
		})

		It("should inject sidecar into Pod with valid annotations", func() {
			podName := "test-basic-injection"

			By("creating a Pod with valid Jolokia annotations")
			cmd := exec.Command("kubectl", "run", podName,
				"--namespace", testNamespace,
				"--image=busybox:latest",
				"--restart=Never",
				"--overrides", `{
					"metadata": {
						"annotations": {
							"jolokia.horoa.net/port": "8778",
							"jolokia.horoa.net/host": "0.0.0.0"
						}
					},
					"spec": {
						"containers": [{
							"name": "app",
							"image": "busybox:latest",
							"command": ["sleep", "3600"]
						}]
					}
				}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create test Pod")

			By("verifying the sidecar init container was injected")
			verifySidecar := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName,
					"-n", testNamespace,
					"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].name}", sidecarName))
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(sidecarName), "Sidecar container not found")
			}
			Eventually(verifySidecar).Should(Succeed())

			By("verifying shareProcessNamespace is set to true")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", "jsonpath={.spec.shareProcessNamespace}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"), "shareProcessNamespace not set")

			By("verifying the emptyDir volume was added")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.volumes[?(@.name==\"%s\")].name}", volumeName))
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(volumeName), "Volume not found")

			By("verifying the sidecar has SYS_PTRACE capability")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].securityContext.capabilities.add}", sidecarName))
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("SYS_PTRACE"), "SYS_PTRACE capability not found")

			By("verifying the sidecar args contain the Jolokia options")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].args}", sidecarName))
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("host=0.0.0.0"), "host arg not found")
			Expect(output).To(ContainSubstring("port=8778"), "port arg not found")
		})

		It("should reject Pod with invalid Jolokia annotation", func() {
			podName := "test-invalid-annotation"

			By("creating a Pod with an invalid Jolokia annotation key")
			cmd := exec.Command("kubectl", "run", podName,
				"--namespace", testNamespace,
				"--image=busybox:latest",
				"--restart=Never",
				"--overrides", `{
					"metadata": {
						"annotations": {
							"jolokia.horoa.net/invalidOption": "value"
						}
					},
					"spec": {
						"containers": [{
							"name": "app",
							"image": "busybox:latest",
							"command": ["sleep", "3600"]
						}]
					}
				}`)
			_, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Pod with invalid annotation should be rejected")
		})

		It("should not inject sidecar into Pod without Jolokia annotations", func() {
			podName := "test-no-annotations"

			By("creating a Pod without Jolokia annotations")
			cmd := exec.Command("kubectl", "run", podName,
				"--namespace", testNamespace,
				"--image=busybox:latest",
				"--restart=Never",
				"--command", "--", "sleep", "3600")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Pod without annotations")

			By("verifying no sidecar init container was injected")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", "jsonpath={.spec.initContainers}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(ContainSubstring(sidecarName),
				"Sidecar should not be injected into Pod without annotations")

			By("verifying shareProcessNamespace is not set")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", "jsonpath={.spec.shareProcessNamespace}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "shareProcessNamespace should not be set")
		})

		It("should inject fully configured sidecar with resource, mount, and target-process annotations", func() {
			podName := "test-full-config"

			By("creating a Pod with all Jolokia annotations")
			cmd := exec.Command("kubectl", "run", podName,
				"--namespace", testNamespace,
				"--image=busybox:latest",
				"--restart=Never",
				"--overrides", `{
					"metadata": {
						"annotations": {
							"jolokia.horoa.net/port": "8778",
							"jolokia.horoa.net/host": "0.0.0.0",
							"jolokia.horoa.net/rsc-limits-cpu": "200m",
							"jolokia.horoa.net/rsc-limits-memory": "128Mi",
							"jolokia.horoa.net/rsc-requests-cpu": "50m",
							"jolokia.horoa.net/rsc-requests-memory": "64Mi",
							"jolokia.horoa.net/mnt": "/opt/jolokia",
							"jolokia.horoa.net/target-process": "com.example.MainApp"
						}
					},
					"spec": {
						"containers": [{
							"name": "app",
							"image": "busybox:latest",
							"command": ["sleep", "3600"]
						}]
					}
				}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create test Pod")

			By("verifying the sidecar init container was injected")
			verifySidecar := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod", podName,
					"-n", testNamespace,
					"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].name}", sidecarName))
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(sidecarName), "Sidecar container not found")
			}
			Eventually(verifySidecar).Should(Succeed())

			By("verifying the custom mount path")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].volumeMounts[?(@.name==\"%s\")].mountPath}",
					sidecarName, volumeName))
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("/opt/jolokia"), "Custom mount path not applied")

			By("verifying the sidecar resource limits")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].resources.limits.cpu}", sidecarName))
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("200m"), "CPU limit not applied")

			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].resources.limits.memory}", sidecarName))
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("128Mi"), "Memory limit not applied")

			By("verifying the sidecar resource requests")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].resources.requests.cpu}", sidecarName))
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("50m"), "CPU request not applied")

			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].resources.requests.memory}", sidecarName))
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("64Mi"), "Memory request not applied")

			By("verifying the sidecar args contain Jolokia options and not operator-specific ones")
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", testNamespace,
				"-o", fmt.Sprintf("jsonpath={.spec.initContainers[?(@.name==\"%s\")].args}", sidecarName))
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("host=0.0.0.0"), "host arg not found")
			Expect(output).To(ContainSubstring("port=8778"), "port arg not found")
			// Operator-specific annotations must NOT appear in args
			Expect(output).NotTo(ContainSubstring("rsc-limits"), "Resource annotation leaked into args")
			Expect(output).NotTo(ContainSubstring("mnt="), "Mount annotation leaked into args")
			Expect(output).NotTo(ContainSubstring("target-process="), "Target process annotation leaked into args")
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			namespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}
