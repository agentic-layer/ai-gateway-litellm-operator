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
	"encoding/json"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/ai-gateway-litellm/test/utils"
)

var _ = Describe("GuardrailPresidio", Ordered, func() {

	BeforeAll(func() {
		By("deploying Presidio analyzer, anonymizer and nginx proxy")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/presidio/presidio.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy Presidio services")

		By("waiting for Presidio proxy to be ready")
		Expect(utils.VerifyDeploymentReady("presidio-proxy", "default", 2*time.Minute)).
			To(Succeed(), "presidio-proxy deployment did not become ready")

		By("waiting for Presidio anonymizer to be ready")
		Expect(utils.VerifyDeploymentReady("presidio-anonymizer", "default", 2*time.Minute)).
			To(Succeed(), "presidio-anonymizer deployment did not become ready")

		// The analyzer loads spaCy NLP models on startup and can take several minutes.
		By("waiting for Presidio analyzer to be ready")
		Expect(utils.VerifyDeploymentReady("presidio-analyzer", "default", 10*time.Minute)).
			To(Succeed(), "presidio-analyzer deployment did not become ready")

		By("applying Presidio guardrail resources (GuardrailProvider, Guard, AiGateway)")
		_, err = utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/presidio/guardrail_e2e.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply Presidio guardrail resources")
	})

	AfterAll(func() {
		By("cleaning up Presidio guardrail resources")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/presidio/guardrail_e2e.yaml", "--ignore-not-found=true"))

		By("removing Presidio services")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/presidio/presidio.yaml", "--ignore-not-found=true"))
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			fetchControllerManagerPodLogs()
			fetchKubernetesEvents()

			By("fetching Presidio analyzer logs")
			analyzerLogs, err := utils.Run(exec.Command("kubectl", "logs",
				"-l", "app=presidio-analyzer", "-n", "default", "--tail=50"))
			if err == nil {
				_, _ = GinkgoWriter.Write([]byte("Presidio analyzer logs:\n" + analyzerLogs))
			}

			By("fetching Presidio anonymizer logs")
			anonymizerLogs, err := utils.Run(exec.Command("kubectl", "logs",
				"-l", "app=presidio-anonymizer", "-n", "default", "--tail=50"))
			if err == nil {
				_, _ = GinkgoWriter.Write([]byte("Presidio anonymizer logs:\n" + anonymizerLogs))
			}

			By("fetching my-litellm-pii pod logs")
			gatewayLogs, err := utils.Run(exec.Command("kubectl", "logs",
				"-l", "app=my-litellm-pii", "-n", "default", "--tail=100"))
			if err == nil {
				_, _ = GinkgoWriter.Write([]byte("my-litellm-pii logs:\n" + gatewayLogs))
			}
		}
	})

	It("should expose a guardrail config in the generated ConfigMap", func() {
		By("waiting for the my-litellm-pii ConfigMap to be created")
		var configMapData string
		Eventually(func(g Gomega) {
			output, err := utils.Run(exec.Command("kubectl", "get", "configmap",
				"my-litellm-pii-config", "-n", "default",
				"-o", "jsonpath={.data.config\\.yaml}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("guardrails"))
			configMapData = output
		}, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"ConfigMap my-litellm-pii-config should contain guardrail config")

		By("verifying guardrail config references Presidio")
		Expect(configMapData).To(ContainSubstring("presidio-api"),
			"guardrail config should use presidio-api guardrail type")
		Expect(configMapData).To(ContainSubstring("presidio-service"),
			"guardrail config should reference the presidio-service URL")
		Expect(configMapData).To(ContainSubstring("pre_call"),
			"guardrail config should include pre_call mode")
	})

	It("should forward a chat completion with PII through the Presidio guardrail and return a valid response", func() {
		payload := chatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []chatMessage{
				{
					Role:    "user",
					Content: "My name is John Smith and my email is john.smith@example.com. What is 2+2?",
				},
			},
		}

		By("sending POST /v1/chat/completions with PII content through the my-litellm-pii AI gateway")
		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServicePost("default", "my-litellm-pii", 80,
				"/v1/chat/completions", payload)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 5*time.Minute, 10*time.Second).Should(Succeed(),
			"AI gateway with Presidio guardrail should return a successful chat completion response")

		By("verifying the response contains the expected fields")
		var response map[string]interface{}
		err := json.Unmarshal(body, &response)
		Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal chat completion response")

		Expect(response["object"]).To(Equal("chat.completion"),
			"Response 'object' field should be 'chat.completion'")

		choices, ok := response["choices"].([]interface{})
		Expect(ok).To(BeTrue(), "Response should contain a 'choices' array")
		Expect(choices).NotTo(BeEmpty(), "Response 'choices' should not be empty")

		firstChoice, ok := choices[0].(map[string]interface{})
		Expect(ok).To(BeTrue(), "First choice should be an object")

		message, ok := firstChoice["message"].(map[string]interface{})
		Expect(ok).To(BeTrue(), "First choice should contain a 'message' object")
		Expect(message["role"]).To(Equal("assistant"),
			"Response message role should be 'assistant'")
	})
})
