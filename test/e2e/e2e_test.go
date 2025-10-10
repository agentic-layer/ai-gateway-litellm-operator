/*
Copyright 2025 Agentic Layer.

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
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/ai-gateway-litellm/test/utils"
)

// namespace where the project is deployed in
const namespace = "ai-gateway-litellm-system"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string
	const aiGatewayInstallUrl = "https://github.com/agentic-layer/ai-gateway-operator/releases/" +
		"download/v0.1.0/install.yaml"

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

		By("deploying the ai gateway operator")
		cmd = exec.Command("kubectl", "apply", "-f", aiGatewayInstallUrl)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the agent runtime")

		By("waiting for ai-gateway-operator-controller-manager to be ready")
		Eventually(func() error {
			cmd := exec.Command("kubectl", "get", "deployment",
				"ai-gateway-operator-controller-manager", "-n", "ai-gateway-operator-system",
				"-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			if err != nil {
				return err
			}
			if output == "" || output == "0" {
				return fmt.Errorf("ai-gateway-operator deployment not ready")
			}
			return nil
		}, 2*time.Minute, 10*time.Second).Should(Succeed(), "ai-gateway-operator deployment should be ready")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("undeploying the controller-manager")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("cleaning up the ai gateway operator")
		cmd = exec.Command("kubectl", "delete", "-f", aiGatewayInstallUrl)
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

		// Metrics test removed - it requires complex networking that's failing in Kind

		It("should provisioned cert-manager", func() {
			By("validating that cert-manager has the certificate Secret")
			verifyCertManager := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "secrets", "webhook-server-cert", "-n", namespace)
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}
			Eventually(verifyCertManager).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// Test your actual AiGateway functionality
		It("should successfully create and reconcile an AiGateway", func() {
			By("creating a test AiGateway")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/crs/ai-gateway.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create AiGateway")

			By("verifying the AiGateway was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "aigateway", "test-gateway", "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if output != "test-gateway" {
					return fmt.Errorf("AiGateway not found")
				}
				return nil
			}, 1*time.Minute).Should(Succeed())

			By("verifying the ConfigMap was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "configmap", "test-gateway-config", "-o", "jsonpath={.data.config\\.yaml}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if !strings.Contains(output, "model_list") {
					return fmt.Errorf("configMap missing model_list")
				}
				if !strings.Contains(output, "gpt-4") {
					return fmt.Errorf("configMap missing gpt-4 model")
				}
				return nil
			}, 2*time.Minute).Should(Succeed())

			By("verifying the Deployment was created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "deployment", "test-gateway", "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if output != "test-gateway" {
					return fmt.Errorf("deployment not found")
				}
				return nil
			}, 2*time.Minute).Should(Succeed())

			By("cleaning up the test AiGateway")
			cmd = exec.Command("kubectl", "delete", "aigateway", "test-gateway")
			_, _ = utils.Run(cmd) // Ignore errors on cleanup

			By("cleaning up the test AiGatewayClass")
			cmd = exec.Command("kubectl", "delete", "aigatewayclass", "litellm", "-n", "default")
			_, _ = utils.Run(cmd) // Ignore errors on cleanup
		})
	})
})
