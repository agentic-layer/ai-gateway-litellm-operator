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

// chatCompletionRequest is the minimal OpenAI chat completion request body.
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var _ = Describe("AiGateway Chat Completion", Ordered, func() {

	BeforeAll(func() {
		By("applying AiGateway sample (WireMock backend configured)")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/v1alpha1_aigateway.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply AiGateway sample")

		By("waiting for AiGateway deployment to be ready")
		Expect(utils.VerifyDeploymentReady("my-litellm", "default", 3*time.Minute)).
			To(Succeed(), "AiGateway deployment did not become ready")
	})

	AfterAll(func() {
		By("cleaning up AiGateway sample")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/v1alpha1_aigateway.yaml", "--ignore-not-found=true"))
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			fetchControllerManagerPodLogs()
			fetchKubernetesEvents()
		}
	})

	It("should forward a chat completion request to the mocked LLM provider and return a valid response", func() {
		payload := chatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []chatMessage{
				{Role: "user", Content: "Hello!"},
			},
		}

		By("sending POST /v1/chat/completions through the AI gateway")
		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServicePost("default", "my-litellm", 80,
				"/v1/chat/completions", payload)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 3*time.Minute, 5*time.Second).Should(Succeed(),
			"AI gateway did not return a successful chat completion response")

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
		Expect(message["content"]).To(Equal("I am a mock AI response from WireMock."),
			"Response message content should match the WireMock stub")
	})
})
