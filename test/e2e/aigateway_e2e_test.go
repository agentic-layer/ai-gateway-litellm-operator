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

	BeforeAll(func() {
		By("applying AiGateway sample")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/v1alpha1_aigateway.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply AiGateway sample")
	})

	AfterAll(func() {
		By("cleaning up test resources")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/v1alpha1_aigateway.yaml"))
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
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
			body, statusCode, err = utils.MakeServiceGet("default", "my-litellm", 4000, "/models")
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

	It("should redeploy when env vars change", func() {
		By("patching AiGateway to add NO_DOCS environment variable")
		patchCmd := exec.Command("kubectl", "patch", "aigateway", "my-litellm",
			"--type=json",
			"-p", `[{"op": "add", "path": "/spec/env", "value": [{"name": "NO_DOCS", "value": "true"}]}]`)
		_, err := utils.Run(patchCmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch AiGateway")

		By("waiting for deployment to be updated and verifying patched response")
		Eventually(func(g Gomega) {
			var body []byte
			var statusCode int
			var err error
			body, statusCode, err = utils.MakeServiceGet("default", "my-litellm", 4000, "/")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))

			// Verify the patched response shows NO_DOCS is respected
			bodyStr := string(body)
			g.Expect(bodyStr).To(ContainSubstring("LiteLLM: RUNNING"), "Expected simple text response after NO_DOCS=true")
			g.Expect(bodyStr).NotTo(ContainSubstring("<!DOCTYPE HTML>"), "Should not serve Swagger docs after NO_DOCS=true")
		}, 3*time.Minute, 5*time.Second).Should(Succeed(), "Deployment should be updated with NO_DOCS env var")
	})
})
