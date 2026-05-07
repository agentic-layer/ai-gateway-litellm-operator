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
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/ai-gateway-litellm/test/utils"
)

var _ = Describe("ToolGateway", Ordered, func() {
	const sample = "config/samples/toolgateway.yaml"

	gatewayTarget := utils.ServiceTarget{
		Namespace:   "tool-gateway",
		ServiceName: "tool-gateway",
		Port:        80,
	}

	BeforeAll(func() {
		By("applying the sample")
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", sample))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ToolGateway sample")

		By("waiting for all deployments to be ready")
		Expect(utils.WaitForAllDeploymentsReady(3 * time.Minute)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up the sample")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", sample, "--ignore-not-found=true"))
	})

	const mcpPath = "/mcp/tool_gateway_routes__echo"

	It("serves MCP tools/list", func() {
		By("listing MCP tools")
		Eventually(func(g Gomega) {
			tools := utils.FetchTools(g, gatewayTarget, mcpPath)
			g.Expect(tools).To(ContainElement("tool_gateway_routes__echo-echo_message"))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("forwards MCP tools/call to the upstream echo server", func() {
		By("calling the echo tool")
		const message = "Hello, MCP!"
		Eventually(func(g Gomega) {
			result, err := utils.CallTool(g, gatewayTarget, mcpPath,
				"tool_gateway_routes__echo-echo",
				map[string]interface{}{"message": message})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(ContainSubstring(message))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})
})

var _ = Describe("ToolGateway with config patch", Ordered, func() {
	const sample = "config/samples/toolgateway_with_patch.yaml"

	BeforeAll(func() {
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", sample))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply patched ToolGateway sample")
		Expect(utils.WaitForAllDeploymentsReady(3 * time.Minute)).To(Succeed())
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", sample, "--ignore-not-found=true"))
	})

	It("enforces the master_key carried by the patch ConfigMap", func() {
		target := utils.ServiceTarget{Namespace: "tool-gateway-patched", ServiceName: "tool-gateway", Port: 80}

		By("expecting non-2xx for unauthenticated /v1/models on the ToolGateway port")
		Eventually(func(g Gomega) {
			_, statusCode, err := utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.GetRequest(baseURL + "/v1/models")
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).ToNot(Equal(200))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())

		By("expecting 200 with the configured master key")
		Eventually(func(g Gomega) {
			_, statusCode, err := utils.MakeServiceRequest(target, func(baseURL string) ([]byte, int, error) {
				b, _, status, err := utils.GetRequestWithHeaders(baseURL+"/v1/models", map[string]string{
					"Authorization": "Bearer sk-sample-tool-1234",
				})
				return b, status, err
			})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(statusCode).To(Equal(200))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})
})
