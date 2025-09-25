# AI Gateway Litellm

"AI Gateway Litellm" is a Kubernetes operator that creates and manages [Litellm](https://www.litellm.ai/) deployments. This operator serves as one possible implementation of an "AI Gateway" operator. It defines a generic `ModelRouter` custom resource.  

The operator is based on the [Operator SDK](https://sdk.operatorframework.io/) framework. 

## Table of Contents

- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Configuration](#configuration)
- [End-to-End (E2E) Testing](#end-to-end-e2e-testing)
- [Testing Tools and Configuration](#testing-tools-and-configuration)
- [Sample Data](#sample-data)
- [Contributing](#contributing)

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

Follow these steps to get the operator up and running on a local Kubernetes cluster.

### Prerequisites
```shell
# Create a local Kubernetes cluster using kind
kind create cluster
```

```bash
# Install cert-manager for webhook support (update the version to the latest stable if needed)
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.18.2/cert-manager.yaml
# Wait for cert-manager to be ready
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=cert-manager -n cert-manager --timeout=60s
```

### Installation
```bash
# Install the AI Gateway Litellm operator (update the version to the latest stable if needed)
kubectl apply -f https://github.com/agentic-layer/ai-gateway-litellm/releases/download/v0.0.1/install.yaml
# Wait for the operator to be ready
kubectl wait --for=condition=Available --timeout=60s -n ai-gateway-litellm-system deployment/ai-gateway-litellm-controller-manager
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

To deploy a Litellm ModelRouter instance, you define a `ModelRouter` resource. Here is an example configuration:

```yaml
apiVersion: gateway.agentic-layer.ai/v1alpha1
kind: ModelRouter
metadata:
  labels:
    app.kubernetes.io/name: ai-gateway-litellm
    app.kubernetes.io/managed-by: kustomize
  name: my-litellm
spec:
  type: litellm
  aiModels:
    - name: openai/gpt-3.5-turbo
    - name: gemini/gemini-1.5-pro

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

The project includes sample `Agent` custom resources to help you get started.

* **Where to find sample data?**
  Sample manifests are located in the `config/samples/` directory.

* **How to deploy a sample agent?**
  You can deploy the sample "weather-agent" with the following `kubectl` command:

  ```bash
  kubectl apply -k config/samples/
  ```

* **How to verify the sample agent?**
  After applying the sample, you can check the status of the created resources:

  ```bash
  # Check the modelrouter's status
  kubectl get modelrouters my-litellm -o yaml
  ```
  ```bash
  # Check the deployment created by the operator
  kubectl get deployments -l app.kubernetes.io/name=my-litellm
  ```
## Contributing

We welcome contributions to the Agentic Layer! Please follow these guidelines:

### Setup for Contributors

1. **Fork and clone the repository**
2. **Install pre-commit hooks** (mandatory for all contributors):
   ```bash
   brew bundle
   ```
   ```bash
   # Install hooks for this repository
   pre-commit install
   ```

3. **Verify your development environment**:
   ```bash
   # Run all checks that pre-commit will run
   make fmt vet lint test
   ```

### Code Style and Standards

- **Go Style**: We follow standard Go conventions and use `gofmt` for formatting
- **Linting**: Code must pass golangci-lint checks (see `.golangci.yml`)
- **Testing**: All new features must include appropriate unit tests
- **Documentation**: Update relevant documentation for new features

### Development Workflow

1. **Create a feature branch** from `main`:
   ```bash
   git checkout -b feature/PAAL-1234-your-feature-name
   ```

2. **Make your changes** following the code style guidelines

3. **Run development checks**:
   ```bash
   # Format code
   make fmt

   # Run static analysis
   make vet

   # Run linting
   make lint

   # Run unit tests
   make test

   # Generate updated manifests if needed
   make manifests generate
   ```

4. **Test your changes**:
   ```bash
   # Run E2E tests to ensure everything works
   make test-e2e
   ```
5. **Update Documentation**:
   Documentation is located in the [`/docs`](/docs) directory. We use the **[Diátaxis framework](https://diataxis.fr/)** for structure and **Antora** to build the site. Please adhere to these conventions when making updates.

6. **Commit your changes** with a descriptive commit message

7**Submit a pull request** with:
- Clear description of the changes
- Reference to any related issues
- Screenshots/logs if applicable

Thank you for contributing to the Agentic Layer!
