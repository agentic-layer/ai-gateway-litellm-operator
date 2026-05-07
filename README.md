# LiteLLM Gateway Operator

The LiteLLM Gateway Operator is a Kubernetes operator that creates and manages [LiteLLM](https://www.litellm.ai/) deployments fronting both LLM and MCP tool traffic.

It reconciles two kinds of gateway:

- `AiGateway` — a LiteLLM proxy in front of one or more LLM providers.
- `ToolGateway` — a LiteLLM proxy in front of one or more MCP tool servers, aggregated via `ToolRoute` resources.

The operator is based on the [Operator SDK](https://sdk.operatorframework.io/) framework. 

## Table of Contents

- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Development](#development)
- [Configuration](#configuration)
- [End-to-End (E2E) Testing](#end-to-end-e2e-testing)
- [Testing Tools and Configuration](#testing-tools-and-configuration)
- [Sample Data](#sample-data)
- [Contributing](#contribution)

----
## Prerequisites

Before working with this project, ensure you have the following tools installed on your system:

* **Go**: version 1.26.0 or higher
* **Docker**: version 20.10+ (or a compatible alternative like Podman)
* **kubectl**: The Kubernetes command-line tool
* **kind**: For running Kubernetes locally in Docker
* **make**: The build automation tool

----

## Getting Started

📖 **For detailed setup instructions**, see our [Getting Started guide](https://docs.agentic-layer.ai/ai-gateway-litellm/how-to-guides.html) in the documentation.

**Quick Start:**

> **Note:** This operator requires the [AI Gateway Operator](https://github.com/agentic-layer/agent-runtime-operator) to be installed first, as it provides the required CRDs (`AiGateway`, `AiGatewayClass`, `ToolGateway`, `ToolGatewayClass`, `ToolRoute`, `ToolServer`).

```shell
# Create local cluster and install cert-manager
kind create cluster
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml

# Install the AI Gateway Operator (provides CRDs)
kubectl apply -f https://github.com/agentic-layer/agent-runtime-operator/releases/latest/download/install.yaml

# Install the LiteLLM operator
kubectl apply -f https://github.com/agentic-layer/ai-gateway-litellm-operator/releases/latest/download/install.yaml
```

## Development

Follow the prerequisites above to set up your local environment.
Then follow these steps to build and deploy the operator locally:

```shell
# Install CRDs into the cluster
make install
# Build docker image
make docker-build
# Load image into kind cluster (not needed if using local registry)
make kind-load
# Deploy the operator to the cluster
make deploy
```

## Configuration


### AI Gateway

To deploy a LiteLLM AI Gateway instance, you define an `AiGateway` resource. Here is an example configuration:

```yaml
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: AiGateway
metadata:
  name: my-litellm
spec:
  aiGatewayClassName: litellm
  aiModels:
    - provider: openai
      name: gpt-3.5-turbo
    - provider: gemini
      name: gemini-1.5-pro
  # see https://docs.litellm.ai/docs/proxy/config_settings#environment-variables---reference for a reference
  env:
   - name: DEBUG
     value: "true"
```

### Tool Gateway

To deploy a LiteLLM-backed Tool Gateway, define a `ToolGateway` and one or more `ToolRoute` resources. The gateway aggregates each route's upstream MCP server into a single endpoint.

```yaml
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: ToolGateway
metadata:
  name: my-tool-gateway
  namespace: tools
spec:
  toolGatewayClassName: litellm
---
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: ToolRoute
metadata:
  name: echo
  namespace: tools
spec:
  toolGatewayRef:
    name: my-tool-gateway
    namespace: tools
  upstream:
    external:
      url: http://echo.default/mcp
```

`ToolRoute.spec.upstream` accepts either:

- `external.url` — an arbitrary HTTP(S) URL. Transport is auto-detected as `sse` when the path ends in `/sse`, otherwise `http`.
- `toolServerRef` — a reference to a `ToolServer` resource managed in the cluster. The operator builds the in-cluster URL and uses `ToolServer.spec.transportType` (`http` or `sse`) for the transport.

Each successfully-attached `ToolRoute` is published at `http://<gateway>.<namespace>.svc.cluster.local/mcp/<route-namespace>__<route-name>` and its `status.url` is updated accordingly.

Both gateway kinds support `spec.guardrails` to attach `Guard` resources for traffic inspection (e.g. PII masking via Presidio). See `config/samples/aigateway_guarded.yaml` and `config/samples/toolgateway_guarded.yaml`.


## End-to-End (E2E) Testing

### Prerequisites for E2E Tests

- **kind** must be installed and available in PATH
- **Docker** running and accessible
- **kubectl** configured and working

### Running E2E Tests

The E2E tests automatically create an isolated Kind cluster, deploy the operator, run comprehensive tests, and clean up afterwards.

```bash
# Run complete E2E test suite
make test-e2e
```

The E2E test suite includes:
- Operator deployment verification
- CRD installation testing
- Webhook functionality testing
- Certificate management verification

### Manual E2E Test Setup

If you need to run E2E tests manually or inspect the test environment:

```bash
# Set up test cluster (will create 'ai-gateway-litellm-test-e2e' cluster)
make setup-test-e2e
```
```bash
# Run E2E tests against the existing cluster
KIND_CLUSTER=ai-gateway-litellm-test-e2e go test ./test/e2e/ -v -ginkgo.v
```
```bash
# Clean up test cluster when done
make cleanup-test-e2e
```

## Testing Tools and Configuration

## Sample Data

The project includes sample `AiGateway` and `ToolGateway` custom resources to help you get started.

* **Where to find sample data?**
  Sample manifests are located in the `config/samples/` directory:

  - `aigateway.yaml` — minimal `AiGateway` wired to a WireMock LLM stub.
  - `aigateway_guarded.yaml` — `AiGateway` with a Presidio-backed PII `Guard`.
  - `toolgateway.yaml` — minimal `ToolGateway` with an `external` `ToolRoute`.
  - `toolgateway_guarded.yaml` — `ToolGateway` with a Presidio-backed PII `Guard` applied to MCP traffic.
  - `backends/` — backing services used by the samples (WireMock LLM, Presidio, an MCP tool server).

* **How to deploy the samples?**

  ```bash
  # Apply the backing services first
  kubectl apply -k config/samples/backends/
  # Then apply the gateway samples
  kubectl apply -k config/samples/
  ```

* **How to verify the samples?**

  ```bash
  # Check gateway status
  kubectl -n ai-gateway get aigateway ai-gateway -o yaml
  kubectl -n tool-gateway get toolgateway tool-gateway -o yaml

  # Check route status (status.url is set when the route is attached)
  kubectl -n tool-gateway-routes get toolroute echo -o yaml

  # Check the deployments created by the operator
  kubectl -n ai-gateway get deployment ai-gateway
  kubectl -n tool-gateway get deployment tool-gateway
  ```

## Contribution

See [Contribution Guide](https://github.com/agentic-layer/ai-gateway-litellm-operator?tab=contributing-ov-file) for details on contribution, and the process for submitting pull requests.
