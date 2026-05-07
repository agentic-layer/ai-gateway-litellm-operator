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
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/ai-gateway-litellm/test/utils"
)

var _ = Describe("ToolGateway with guardrails", Ordered, func() {
	const sample = "config/samples/toolgateway_guarded.yaml"

	gatewayTarget := utils.ServiceTarget{
		Namespace:   "tool-gateway-guarded",
		ServiceName: "tool-gateway-guarded",
		Port:        80,
	}

	BeforeAll(func() {
		By("applying the sample")
		_, err := utils.Run(exec.Command("kubectl", "apply", "-f", sample))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply guarded ToolGateway sample")

		By("waiting for all deployments to be ready")
		Expect(utils.WaitForAllDeploymentsReady(3 * time.Minute)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up the sample")
		_, _ = utils.Run(exec.Command("kubectl", "delete", "-f", sample, "--ignore-not-found=true"))
	})

	It("reports Ready=True after resolving the cross-namespace Guard", func() {
		By("checking the ToolGateway Ready condition")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "toolgateway",
				"tool-gateway-guarded", "-n", "tool-gateway-guarded",
				"-o", `jsonpath={.status.conditions[?(@.type=="ToolGatewayReady")].status}`))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal("True"))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})

	const mcpPath = "/mcp/tool_gateway_guarded_routes__echo_guarded"

	It("serves MCP tools/list through the guarded gateway", func() {
		By("listing MCP tools")
		Eventually(func(g Gomega) {
			tools := utils.FetchTools(g, gatewayTarget, mcpPath)
			g.Expect(tools).To(ContainElement("tool_gateway_guarded_routes__echo_guarded-echo_message"))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("forwards tools/call through the guarded gateway", func() {
		// Calling the echo tool through the guarded path proves the gateway
		// applies the Guard at request time without breaking the call.
		//
		// We deliberately do NOT assert that PII in the arguments is masked.
		// Our operator translates the Guard's pre_call mode to LiteLLM's
		// pre_mcp_call mode (see internal/litellm/guardrails.go), and LiteLLM
		// records the guardrail as applied — but the Presidio guardrail class
		// in upstream LiteLLM has no async_pre_mcp_call_hook method (verified
		// against tags v1.83.14-stable and main). The mode is accepted at
		// config-load but the Presidio backend is never invoked for MCP
		// traffic, so the arguments pass through unmasked. Re-arm the masking
		// assertion once upstream wires Presidio into the MCP dispatcher.
		const message = "Hello from the guarded gateway"

		By("calling the echo tool through the guarded path")
		Eventually(func(g Gomega) {
			result, err := utils.CallTool(g, gatewayTarget, mcpPath,
				"tool_gateway_guarded_routes__echo_guarded-echo",
				map[string]interface{}{"message": message})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(ContainSubstring(message))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})
})
