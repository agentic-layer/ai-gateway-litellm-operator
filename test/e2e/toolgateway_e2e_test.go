/*
Copyright 2026 Agentic Layer.
*/

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-layer/ai-gateway-litellm/test/utils"
)

const (
	toolGatewayName       = "tool-gateway"
	toolGatewayNs         = "tool-gateway"
	mcpServerName         = "echo"
	mcpServerNs           = "default"
	mcpRouteName          = "echo"
	mcpRouteNs            = "default"
	expectedRouteEndpoint = "/mcp/default-echo"
	expectedToolName      = "echo_message" // matches ECHO_TOOL_NAME in the sample yaml
)

// jsonRpcRequest is a minimal JSON-RPC 2.0 envelope. Sufficient for tools/list.
type jsonRpcRequest struct {
	JsonRpc string                 `json:"jsonrpc"`
	Id      int                    `json:"id"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

var _ = Describe("ToolGateway", Ordered, func() {

	BeforeAll(func() {
		By("applying the ToolGateway sample (gateway + server + route)")
		_, err := utils.Run(exec.Command("kubectl", "apply",
			"-f", "config/samples/v1alpha1_toolgateway.yaml"))
		Expect(err).NotTo(HaveOccurred(), "Failed to apply ToolGateway sample")

		By("waiting for the echo MCP server Deployment to be ready")
		// Hand-rolled by the sample yaml (Task 11). Image pull is the bottleneck
		// on a fresh cluster, so the timeout is generous.
		Expect(utils.VerifyDeploymentReady(mcpServerName, mcpServerNs, 5*time.Minute)).To(Succeed(),
			"echo MCP server did not become ready")

		By("waiting for the LiteLLM ToolGateway Deployment to be ready")
		Expect(utils.VerifyDeploymentReady(toolGatewayName, toolGatewayNs, 5*time.Minute)).To(Succeed(),
			"ToolGateway deployment did not become ready")
	})

	AfterAll(func() {
		By("cleaning up the ToolGateway sample")
		_, _ = utils.Run(exec.Command("kubectl", "delete",
			"-f", "config/samples/v1alpha1_toolgateway.yaml", "--ignore-not-found=true"))
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			fetchControllerManagerPodLogs()
			fetchKubernetesEvents()
			fetchPodLogs("app="+toolGatewayName, toolGatewayNs, 100)
		}
	})

	It("serves MCP tools/list at /mcp/<route-ns>-<route-name>", func() {
		body, err := callJsonRpc("tools/list", nil)
		Expect(err).NotTo(HaveOccurred())

		// Assert on the configured ECHO_TOOL_NAME so the test is fully
		// deterministic — it cannot drift with image releases.
		Expect(string(body)).To(ContainSubstring(`"`+expectedToolName+`"`),
			"tools/list should advertise %q; got %s", expectedToolName, string(body))
	})

	It("marks the ToolRoute Ready=True with the gateway-relative URL", func() {
		out, err := utils.Run(exec.Command("kubectl", "get", "toolroute",
			mcpRouteName, "-n", mcpRouteNs, "-o", "jsonpath={.status.url}"))
		Expect(err).NotTo(HaveOccurred())
		want := fmt.Sprintf("http://%s.%s.svc.cluster.local%s", toolGatewayName, toolGatewayNs, expectedRouteEndpoint)
		Expect(strings.TrimSpace(out)).To(Equal(want),
			"ToolRoute.status.url mismatch")
	})

	It("redeploys the gateway when a second ToolRoute is added", func() {
		secondRouteYaml := `apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: ToolRoute
metadata:
  name: echo-second
  namespace: default
spec:
  upstream:
    toolServerRef:
      name: echo
`
		By("applying a second ToolRoute")
		Expect(utils.RunStdin(exec.Command("kubectl", "apply", "-f", "-"), secondRouteYaml)).To(Succeed())
		DeferCleanup(func() {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "toolroute",
				"echo-second", "-n", "default", "--ignore-not-found=true"))
		})

		By("waiting for the second route to become Ready=True at /mcp/default-echo-second")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "toolroute",
				"echo-second", "-n", "default", "-o", "jsonpath={.status.url}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(HaveSuffix("/mcp/default-echo-second"))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())

		By("calling tools/list via the second route's path")
		body, status, err := utils.MakeServicePost(toolGatewayNs, toolGatewayName, 80,
			"/mcp/default-echo-second",
			jsonRpcRequest{JsonRpc: "2.0", Id: 2, Method: "tools/list"})
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(200), "second-route /mcp endpoint returned %d, body: %s", status, string(body))
		Expect(string(body)).To(ContainSubstring(`"` + expectedToolName + `"`))
	})

	It("marks a route NotReady when its upstream is unresolved and keeps the gateway serving other routes", func() {
		brokenRouteYaml := `apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: ToolRoute
metadata:
  name: ghost-route
  namespace: default
spec:
  upstream:
    toolServerRef:
      name: ghost-server
`
		By("applying a route with a missing ToolServer")
		Expect(utils.RunStdin(exec.Command("kubectl", "apply", "-f", "-"), brokenRouteYaml)).To(Succeed())
		DeferCleanup(func() {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "toolroute",
				"ghost-route", "-n", "default", "--ignore-not-found=true"))
		})

		By("waiting for the route's Ready condition to flip to False/UpstreamUnresolved")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "toolroute",
				"ghost-route", "-n", "default",
				"-o", `jsonpath={.status.conditions[?(@.type=="Ready")].reason}`))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(out)).To(Equal("UpstreamUnresolved"))
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying the original route is still served")
		body, status, err := utils.MakeServicePost(toolGatewayNs, toolGatewayName, 80,
			expectedRouteEndpoint, jsonRpcRequest{JsonRpc: "2.0", Id: 3, Method: "tools/list"})
		Expect(err).NotTo(HaveOccurred())
		Expect(status).To(Equal(200),
			"Existing route should keep serving despite a broken sibling; got %d, body: %s", status, string(body))
	})

	It("forwards to an external upstream", func() {
		// We point at the same echo Service as an external URL so we don't
		// need a second image pull. The route URL exercises the External branch of
		// resolveRouteUpstream.
		externalUrl := fmt.Sprintf("http://%s.%s.svc.cluster.local:8000/mcp", mcpServerName, mcpServerNs)
		externalRouteYaml := fmt.Sprintf(`apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: ToolRoute
metadata:
  name: external-route
  namespace: default
spec:
  upstream:
    external:
      url: %q
`, externalUrl)

		By("applying a ToolRoute with external.url")
		Expect(utils.RunStdin(exec.Command("kubectl", "apply", "-f", "-"), externalRouteYaml)).To(Succeed())
		DeferCleanup(func() {
			_, _ = utils.Run(exec.Command("kubectl", "delete", "toolroute",
				"external-route", "-n", "default", "--ignore-not-found=true"))
		})

		By("calling tools/list via /mcp/default-external-route")
		Eventually(func(g Gomega) {
			body, status, err := utils.MakeServicePost(toolGatewayNs, toolGatewayName, 80,
				"/mcp/default-external-route",
				jsonRpcRequest{JsonRpc: "2.0", Id: 4, Method: "tools/list"})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status).To(Equal(200), "external route returned %d, body: %s", status, string(body))
			g.Expect(string(body)).To(ContainSubstring(`"` + expectedToolName + `"`))
		}, 3*time.Minute, 5*time.Second).Should(Succeed())
	})

	Context("with Presidio guardrail attached", func() {

		BeforeAll(func() {
			By("applying the Presidio guardrail sample")
			_, err := utils.Run(exec.Command("kubectl", "apply",
				"-f", "config/samples/v1alpha1_guardrail_presidio.yaml"))
			Expect(err).NotTo(HaveOccurred(), "Failed to apply Presidio guardrail sample")
			// Re-apply the toolgateway sample so the guarded gateway is created.
			_, err = utils.Run(exec.Command("kubectl", "apply",
				"-f", "config/samples/v1alpha1_toolgateway.yaml"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("brings the guarded ToolGateway Ready=True", func() {
			Expect(utils.VerifyDeploymentReady("tool-gateway-guarded", toolGatewayNs, 5*time.Minute)).
				To(Succeed(), "guarded ToolGateway deployment did not become ready")

			By("verifying ToolGatewayConfigured=True / ToolGatewayReady=True")
			out, err := utils.Run(exec.Command("kubectl", "get", "toolgateway",
				"tool-gateway-guarded", "-n", toolGatewayNs,
				"-o", `jsonpath={.status.conditions[?(@.type=="ToolGatewayReady")].status}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(out)).To(Equal("True"))
		})

		It("renders a guardrails block into the ConfigMap", func() {
			body, err := utils.Run(exec.Command("kubectl", "get", "configmap",
				"tool-gateway-guarded-config", "-n", toolGatewayNs,
				"-o", `jsonpath={.data.config\.yaml}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(ContainSubstring("guardrails:"),
				"ConfigMap should contain guardrails block")
			Expect(body).To(ContainSubstring("guardrail: presidio"))
		})
	})
})

	It("serves MCP tools/list", func() {
		By("listing MCP tools")
		Eventually(func(g Gomega) {
			tools := utils.FetchTools(g, gatewayTarget, "/mcp/default__echo")
			g.Expect(tools).To(ContainElement("default__echo-echo_message"))
		}, 1*time.Minute, 5*time.Second).Should(Succeed())
	})
})
