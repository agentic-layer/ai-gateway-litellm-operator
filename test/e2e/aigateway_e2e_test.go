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
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/ai-gateway-litellm/test/utils"
)

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// getWireMockRequestBody retrieves the body of the most recent request matching urlFragment
// from WireMock's request journal.
func getWireMockRequestBody(urlFragment string) (string, error) {
	body, statusCode, err := utils.MakeServiceGet("default", "wiremock", 8080, "/__admin/requests")
	if err != nil {
		return "", fmt.Errorf("failed to fetch WireMock journal: %w", err)
	}
	if statusCode != 200 {
		return "", fmt.Errorf("WireMock journal returned status %d", statusCode)
	}

	var journal map[string]interface{}
	if err := json.Unmarshal(body, &journal); err != nil {
		return "", fmt.Errorf("failed to parse WireMock journal: %w", err)
	}

	requests, ok := journal["requests"].([]interface{})
	if !ok || len(requests) == 0 {
		return "", fmt.Errorf("WireMock journal contains no requests")
	}

	// WireMock 3.x nests fields under a "request" key.
	for _, entry := range requests {
		e := entry.(map[string]interface{})
		r, ok := e["request"].(map[string]interface{})
		if !ok {
			r = e
		}
		if url, ok := r["url"].(string); ok && strings.Contains(url, urlFragment) {
			if reqBody, ok := r["body"].(string); ok {
				return reqBody, nil
			}
		}
	}
	return "", fmt.Errorf("no request matching %q found in WireMock journal", urlFragment)
}

var _ = Describe("AiGateway", Ordered, func() {

	BeforeAll(func() {
		By("applying AiGateway sample")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/v1alpha1_aigateway.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply AiGateway sample")

		By("applying Presidio guardrail resources")
		_, err = utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/v1alpha1_guardrail_presidio.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply guardrail resources")
	})

	AfterAll(func() {
		By("cleaning up test resources")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/v1alpha1_guardrail_presidio.yaml", "--ignore-not-found=true"))
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/v1alpha1_aigateway.yaml", "--ignore-not-found=true"))
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			fetchControllerManagerPodLogs()
			fetchKubernetesEvents()
		}
	})

	It("should expose configured models via /models endpoint", func() {
		By("querying /models endpoint")
		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServiceGet("default", "ai-gateway", 80, "/models")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 2*time.Minute, 5*time.Second).Should(Succeed(), "Failed to query /models endpoint")

		By("verifying response contains configured model")
		var responseMap map[string]interface{}
		err := json.Unmarshal(body, &responseMap)
		Expect(err).NotTo(HaveOccurred(), "Failed to unmarshal /models response")
		Expect(responseMap["data"]).NotTo(BeNil(), "/models response should contain 'data' field")

		data, ok := responseMap["data"].([]interface{})
		Expect(ok).To(BeTrue(), "'data' field should be an array")
		Expect(data).ToNot(BeEmpty(), "Should have at least one model")

		Expect(data).To(ContainElement(HaveKeyWithValue("id", "gpt-3.5-turbo")),
			"the /models endpoint should contain the configured model gpt-3.5-turbo")
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
			body, statusCode, err = utils.MakeServicePost("default", "ai-gateway", 80,
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

	It("should redeploy when env vars change", func() {
		By("patching AiGateway to add NO_DOCS environment variable")
		patchCmd := exec.Command("kubectl", "patch", "aigateway", "ai-gateway",
			"--type=json",
			"-p", `[{"op": "add", "path": "/spec/env", "value": [{"name": "NO_DOCS", "value": "true"}]}]`)
		_, err := utils.Run(patchCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch AiGateway")

		By("waiting for deployment to be updated and verifying patched response")
		Eventually(func(g Gomega) {
			var body []byte
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServiceGet("default", "ai-gateway", 80, "/")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))

			// Verify the patched response shows NO_DOCS is respected
			bodyStr := string(body)
			g.Expect(bodyStr).To(ContainSubstring("LiteLLM: RUNNING"), "Expected simple text response after NO_DOCS=true")
			g.Expect(bodyStr).NotTo(ContainSubstring("<!DOCTYPE HTML>"), "Should not serve Swagger docs after NO_DOCS=true")
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "Deployment should be updated with NO_DOCS env var")
	})

	Context("with Presidio PII guardrail", func() {

		AfterEach(func() {
			if CurrentSpecReport().Failed() {
				By("fetching ai-gateway-pii pod status")
				describeOutput, err := utils.Run(exec.Command("kubectl", "describe", "pods",
					"-l", "app=ai-gateway-pii", "-n", "default"))
				if err == nil {
					_, _ = fmt.Fprintf(GinkgoWriter, "ai-gateway-pii pod describe:\n%s\n", describeOutput)
				}

				fetchPodLogs("app=presidio-analyzer", "default", 50)
				fetchPodLogs("app=presidio-anonymizer", "default", 50)
				fetchPodLogs("app=ai-gateway-pii", "default", 100)
			}
		})

		It("should anonymize PII in chat completion requests", func() {
			payload := chatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []chatMessage{
					{Role: "user", Content: "My name is John Smith and my email is john.smith@example.com. What is 2+2?"},
				},
			}

			By("sending POST /v1/chat/completions with PII content through the guardrail gateway")
			Eventually(func(g Gomega) {
				_, statusCode, err := utils.MakeServicePost("default", "ai-gateway-pii", 80,
					"/v1/chat/completions", payload)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusCode).To(Equal(200), "guardrail gateway should return 200")
			}, 5*time.Minute, 10*time.Second).Should(Succeed(),
				"AI gateway with Presidio guardrail did not return a successful response")

			By("verifying WireMock received the request with PII replaced by Presidio placeholders")
			var forwardedBody string
			Eventually(func(g Gomega) {
				var err error
				forwardedBody, err = getWireMockRequestBody("chat/completions")
				g.Expect(err).NotTo(HaveOccurred())
			}, 30*time.Second, 5*time.Second).Should(Succeed(),
				"Failed to retrieve forwarded request from WireMock journal")

			_, _ = fmt.Fprintf(GinkgoWriter, "Forwarded request body to LLM:\n%s\n", forwardedBody)

			// Parse the forwarded body and verify PII was replaced with Presidio placeholders.
			var forwarded chatCompletionRequest
			Expect(json.Unmarshal([]byte(forwardedBody), &forwarded)).To(Succeed(),
				"Forwarded body should be valid JSON")
			Expect(forwarded.Messages).To(HaveLen(1))

			content := forwarded.Messages[0].Content
			Expect(content).To(ContainSubstring("<PERSON>"),
				"Name should be replaced with <PERSON> placeholder")
			Expect(content).To(ContainSubstring("<EMAIL_ADDRESS>"),
				"Email should be replaced with <EMAIL_ADDRESS> placeholder")
			Expect(content).NotTo(ContainSubstring("John Smith"),
				"Original name must not be present")
			Expect(content).NotTo(ContainSubstring("john.smith@example.com"),
				"Original email must not be present")
		})
	})
})
