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

var _ = Describe("AiGateway", Ordered, func() {
	const sample = "config/samples/aigateway.yaml"

	BeforeAll(func() {
		By("applying the sample")
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", sample))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply the sample")

		By("waiting for all deployments to be ready")
		Expect(utils.WaitForAllDeploymentsReady(3 * time.Minute)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning the sample")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", sample, "--ignore-not-found=true"))
	})

	It("should expose configured models via /models endpoint", func() {
		By("querying /models endpoint")
		target := utils.ServiceTarget{Namespace: "ai-gateway", ServiceName: "ai-gateway", Port: 80}
		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.GetRequest(baseURL + "/models")
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 1*time.Minute, 5*time.Second).Should(Succeed(), "Failed to query /models endpoint")

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
		payload := utils.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []utils.ChatMessage{
				{Role: "user", Content: "Hello!"},
			},
		}

		By("sending POST /v1/chat/completions through the AI gateway")
		target := utils.ServiceTarget{Namespace: "ai-gateway", ServiceName: "ai-gateway", Port: 80}
		var body []byte
		Eventually(func(g Gomega) {
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.PostRequest(baseURL+"/v1/chat/completions", payload, nil)
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 1*time.Minute, 5*time.Second).Should(Succeed(),
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
			"-n", "ai-gateway",
			"--type=json",
			"-p", `[{"op": "add", "path": "/spec/env", "value": [{"name": "NO_DOCS", "value": "true"}]}]`)
		_, err := utils.Run(patchCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch AiGateway")

		By("waiting for deployment to be updated and verifying patched response")
		target := utils.ServiceTarget{Namespace: "ai-gateway", ServiceName: "ai-gateway", Port: 80}
		Eventually(func(g Gomega) {
			body, statusCode, err := utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.GetRequest(baseURL + "/")
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))

			// Verify the patched response shows NO_DOCS is respected
			bodyStr := string(body)
			g.Expect(bodyStr).To(ContainSubstring("LiteLLM: RUNNING"), "Expected simple text response after NO_DOCS=true")
			g.Expect(bodyStr).NotTo(ContainSubstring("<!DOCTYPE HTML>"), "Should not serve Swagger docs after NO_DOCS=true")
		}, 1*time.Minute, 5*time.Second).Should(Succeed(), "Deployment should be updated with NO_DOCS env var")
	})
})

var _ = Describe("AiGateway with config patch", Ordered, func() {
	const sample = "config/samples/aigateway_with_patch.yaml"

	BeforeAll(func() {
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", sample))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply patched AiGateway sample")
		Expect(utils.WaitForAllDeploymentsReady(3 * time.Minute)).To(Succeed())
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", sample, "--ignore-not-found=true"))
	})

	It("rejects unauthenticated requests after master_key is patched in", func() {
		By("patching the patch ConfigMap to introduce general_settings.master_key")
		patchScript := `kubectl -n ai-gateway-patched patch configmap ai-gateway-patch --type merge -p '` +
			`{"data":{"patch.yaml":"router_settings:\n  routing_strategy: usage-based-routing-v2\nlitellm_settings:\n  drop_params: true\ngeneral_settings:\n  master_key: sk-e2e-test-1234\n"}}'`
		_, err := utils.Run(exec.Command("bash", "-c", patchScript))
		Expect(err).NotTo(HaveOccurred(), "Failed to patch ai-gateway-patch ConfigMap")

		Expect(utils.WaitForAllDeploymentsReady(3 * time.Minute)).To(Succeed())

		target := utils.ServiceTarget{Namespace: "ai-gateway-patched", ServiceName: "ai-gateway", Port: 80}

		By("expecting non-2xx for unauthenticated /v1/models")
		Eventually(func(g Gomega) {
			_, statusCode, err := utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.GetRequest(baseURL + "/v1/models")
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).ToNot(Equal(200), "expected non-2xx for unauthenticated request")
		}, 1*time.Minute, 5*time.Second).Should(Succeed())

		By("expecting 200 with the configured master key")
		Eventually(func(g Gomega) {
			_, statusCode, err := utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.GetRequestWithHeaders(baseURL+"/v1/models", map[string]string{
					"Authorization": "Bearer sk-e2e-test-1234",
				})
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("re-allows unauthenticated traffic after master_key is removed from the patch", func() {
		By("removing master_key from the patch ConfigMap")
		patchScript := `kubectl -n ai-gateway-patched patch configmap ai-gateway-patch --type merge -p '` +
			`{"data":{"patch.yaml":"router_settings:\n  routing_strategy: usage-based-routing-v2\nlitellm_settings:\n  drop_params: true\n"}}'`
		_, err := utils.Run(exec.Command("bash", "-c", patchScript))
		Expect(err).NotTo(HaveOccurred(), "Failed to remove master_key from patch ConfigMap")

		Expect(utils.WaitForAllDeploymentsReady(3 * time.Minute)).To(Succeed())

		target := utils.ServiceTarget{Namespace: "ai-gateway-patched", ServiceName: "ai-gateway", Port: 80}
		Eventually(func(g Gomega) {
			_, statusCode, err := utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.GetRequest(baseURL + "/v1/models")
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})
})
