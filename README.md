# LiteLLM Gateway Operator

The LiteLLM Gateway Operator is a Kubernetes operator that creates and manages [LiteLLM](https://www.litellm.ai/) deployments fronting both LLM and MCP tool traffic. It reconciles two kinds of gateway:

- `AiGateway` — a LiteLLM proxy in front of one or more LLM providers.
- `ToolGateway` — a LiteLLM proxy in front of one or more MCP tool servers, aggregated via `ToolRoute` resources.

📖 **Documentation:** https://docs.agentic-layer.ai/ai-gateway-litellm-operator/

## Development

### Prerequisites

- **Go** 1.26+
- **Docker**
- **kubectl**
- **kind** (used for local development and E2E tests)
- **make**

### Build and deploy locally

```shell
# Create a local cluster
kind create cluster

# Install Cert Manager for webhook TLS
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml

# Install the Agent Runtime Operator (provides the AiGateway/ToolGateway CRDs)
kubectl apply -f https://github.com/agentic-layer/agent-runtime-operator/releases/latest/download/install.yaml

# Install CRDs into the cluster
make install
# Build docker image
make docker-build
# Load image into kind cluster (not needed if using local registry)
make kind-load
# Deploy the operator to the cluster
make deploy
```

### Test

```shell
make lint       # linting
make test       # unit + integration tests
make test-e2e   # E2E tests in a Kind cluster
```

For manual E2E setup against an existing cluster, see the `Makefile` targets `setup-test-e2e` and `cleanup-test-e2e`.

### Verify the local deploy

Apply the bundled backing services and gateway samples, then check the resources the operator reconciles:

```shell
kubectl apply -k config/samples/backends/
kubectl apply -k config/samples/

kubectl -n ai-gateway get aigateway ai-gateway -o yaml
kubectl -n tool-gateway get toolgateway tool-gateway -o yaml
kubectl -n tool-gateway-routes get toolroute echo -o yaml
```

### Create or Update API and Webhooks

The operator-sdk CLI can be used to create or update APIs and webhooks.
This is the preferred way to add new APIs and webhooks to the operator.
If the operator-sdk CLI is updated, you may need to re-run these commands to update the generated code.

```shell
# Create API for Agent CRD
operator-sdk create api --group runtime --version v1alpha1 --kind Agent

# Create webhook for Agent CRD
operator-sdk create webhook --group runtime --version v1alpha1 --kind Agent --defaulting --programmatic-validation
```

## Contributing

See the [Contribution Guide](https://github.com/agentic-layer/ai-gateway-litellm-operator?tab=contributing-ov-file).
