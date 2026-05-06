/*
Copyright 2026 Agentic Layer.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/ai-gateway-litellm/test/utils"
)

const aiGatewayGuardrailSample = "config/samples/aigateway_guarded.yaml"

var _ = Describe("AiGateway with guardrails", Ordered, func() {

	BeforeAll(func() {
		By("applying the sample")
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", aiGatewayGuardrailSample))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply sample")

		By("waiting for all deployments to be ready")
		Expect(utils.WaitForAllDeploymentsReady(3 * time.Minute)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up the sample")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", aiGatewayGuardrailSample, "--ignore-not-found=true"))
	})

	It("should anonymize PII in chat completion requests", func() {
		payload := utils.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []utils.ChatMessage{
				{Role: "user", Content: "My name is John Smith and my email is john.smith@example.com. What is 2+2?"},
			},
		}

		By("sending POST /v1/chat/completions with PII content through the guardrail gateway")
		target := utils.ServiceTarget{Namespace: "ai-gateway-guarded", ServiceName: "ai-gateway-pii", Port: 80}
		Eventually(func(g Gomega) {
			_, statusCode, err := utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.PostRequest(baseURL+"/v1/chat/completions", payload, nil)
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200), "guardrail gateway should return 200")
		}, 1*time.Minute, 10*time.Second).Should(Succeed(),
			"AI gateway with Presidio guardrail did not return a successful response")

		By("verifying WireMock received the request with PII replaced by Presidio placeholders")
		wiremockTarget := utils.ServiceTarget{Namespace: "default", ServiceName: "wiremock", Port: 8080}
		var forwardedBody string
		Eventually(func(g Gomega) {
			var err error
			forwardedBody, err = utils.GetWireMockRequestBody(wiremockTarget, "chat/completions")
			g.Expect(err).NotTo(HaveOccurred())
		}, 30*time.Second, 5*time.Second).Should(Succeed(),
			"Failed to retrieve forwarded request from WireMock journal")

		_, _ = fmt.Fprintf(GinkgoWriter, "Forwarded request body to LLM:\n%s\n", forwardedBody)

		// Parse the forwarded body and verify PII was replaced with Presidio placeholders.
		var forwarded utils.ChatCompletionRequest
		Expect(json.Unmarshal([]byte(forwardedBody), &forwarded)).To(Succeed(),
			"Forwarded body should be valid JSON")
		Expect(forwarded.Messages).To(HaveLen(1))

		content := forwarded.Messages[0].Content
		Expect(content).To(MatchRegexp(`<PERSON[^>]*>`),
			"Name should be replaced with <PERSON> or <PERSON_N> placeholder")
		Expect(content).To(MatchRegexp(`<EMAIL_ADDRESS[^>]*>`),
			"Email should be replaced with <EMAIL_ADDRESS> or <EMAIL_ADDRESS_N> placeholder")
		Expect(content).NotTo(ContainSubstring("John Smith"),
			"Original name must not be present")
		Expect(content).NotTo(ContainSubstring("john.smith@example.com"),
			"Original email must not be present")
	})
})
