# AI Gateway LiteLLM

"AI Gateway LiteLLM" is a Kubernetes operator that creates and manages [LiteLLM](https://www.litellm.ai/) deployments.

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

* **Go**: version 1.24.0 or higher
* **Docker**: version 20.10+ (or a compatible alternative like Podman)
* **kubectl**: The Kubernetes command-line tool
* **kind**: For running Kubernetes locally in Docker
* **make**: The build automation tool

----

## Getting Started

📖 **For detailed setup instructions**, see our [Getting Started guide](https://docs.agentic-layer.ai/ai-gateway-litellm/how-to-guides.html) in the documentation.

**Quick Start:**

> **Note:** This operator requires the [AI Gateway Operator](https://github.com/agentic-layer/agent-runtime-operator) to be installed first, as it provides the required CRDs (`AiGateway` and `AiGatewayClass`).

```shell
# Create local cluster and install cert-manager
kind create cluster
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.19.1/cert-manager.yaml

# Install the AI Gateway Operator (provides CRDs)
kubectl apply -f https://github.com/agentic-layer/agent-runtime-operator/releases/download/v0.9.0/install.yaml

# Install the LiteLLM operator
kubectl apply -f https://github.com/agentic-layer/ai-gateway-litellm-operator/releases/download/v0.2.0/install.yaml
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


### Custom Resource Configuration

To deploy a LiteLLM AI Gateway instance, you define an `AiGateway` resource. Here is an example configuration:

```yaml
apiVersion: runtime.agentic-layer.ai/v1alpha1
kind: AiGateway
metadata:
  name: my-litellm
spec:
  AiGatewayClassName: litellm
  aiModels:
    - provider: openai
      name: gpt-3.5-turbo
    - provider: gemini
      name: gemini-1.5-pro

```


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

The project includes sample `AiGateway` custom resources to help you get started.

* **Where to find sample data?**
  Sample manifests are located in the `config/samples/` directory.

* **How to deploy a sample AI Gateway?**
  You can deploy the sample "my-litellm" with the following `kubectl` command:

  ```bash
  kubectl apply -k config/samples/
  ```

* **How to verify the sample AI Gateway?**
  After applying the sample, you can check the status of the created resources:

  ```bash
  # Check the aigateway's status
  kubectl get aigateways my-litellm -o yaml
  ```
  ```bash
  # Check the deployment created by the operator
  kubectl get deployments -l app.kubernetes.io/name=my-litellm
  ```

## Contribution

See [Contribution Guide](https://github.com/agentic-layer/ai-gateway-litellm-operator?tab=contributing-ov-file) for details on contribution, and the process for submitting pull requests.
