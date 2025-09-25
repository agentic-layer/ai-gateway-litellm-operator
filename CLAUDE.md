# AI Gateway LiteLLM Operator - Developer Guide

@README.md

## Overview

The AI Gateway LiteLLM is a Kubernetes operator built with the Operator SDK framework that manages [LiteLLM](https://www.litellm.ai/) deployments. This operator implements a generic `ModelRouter` custom resource that can route requests to multiple AI model providers (OpenAI, Anthropic, Gemini, Azure, AWS Bedrock, etc.) through a single unified interface.

**Technology Stack:**
- **Framework**: Operator SDK v1.28+ (based on controller-runtime)
- **Language**: Go 1.24+
- **Kubernetes**: 1.28+ (uses admission webhooks, CRDs)
- **AI Gateway**: LiteLLM proxy for multi-provider AI model routing

## Architecture Overview

### Core Components

1. **ModelRouter Controller** (`internal/controller/modelrouter_controller.go`)
   - Reconciles ModelRouter custom resources
   - Creates ConfigMap, Deployment, and Service resources
   - Manages LiteLLM container lifecycle
   - Uses config-hash annotations to trigger pod restarts on configuration changes

2. **Validation Webhook** (`internal/webhook/v1alpha1/modelrouter_webhook.go`)
   - Validates ModelRouter specs at admission time
   - Ensures AI models follow `provider/model-name` format
   - Sets default port (4000) if not specified
   - Prevents invalid configurations from being stored

3. **Configuration Generators** (`internal/controller/modelrouter_generators.go`)
   - LiteLLM-specific configuration generation
   - Handles provider-specific model mapping
   - Generates YAML configuration for LiteLLM proxy

4. **Equality Utilities** (`internal/equality/`)
   - Semantic comparison for Kubernetes resources
   - Supports comparing environment variables, labels, AI model lists
   - Uses `RequiredLabelsPresent()` to respect labels from other operators

### Project Structure
```
├── api/v1alpha1/           # CRD definitions and types
├── config/                 # Kustomize configurations
│   ├── crd/               # Generated CRD manifests
│   ├── webhook/           # Webhook configurations
│   ├── samples/           # Sample ModelRouter resources
│   └── default/           # Default deployment configuration
├── internal/
│   ├── controller/        # Controller logic and generators
│   ├── webhook/           # Admission webhook handlers
│   ├── equality/          # Resource comparison utilities
│   └── constants/         # Shared constants
└── test/e2e/              # End-to-end tests
```

## Advanced ModelRouter Examples

### Multi-Provider Enterprise Setup
```yaml
apiVersion: gateway.agentic-layer.ai/v1alpha1
kind: ModelRouter
metadata:
  name: enterprise-ai-gateway
spec:
  type: litellm
  port: 8080
  aiModels:
    - name: openai/gpt-4-turbo
    - name: azure/gpt-4
    - name: bedrock/claude-3-sonnet
    - name: ollama/llama2  # Local models (no API key needed)
    - name: huggingface/codellama-7b-instruct
```

## Development Commands

### Controller Development
```bash
# Run controller locally (useful for debugging)
make run

# Check operator deployment status
kubectl get pods -n ai-gateway-litellm-system

# View controller logs
kubectl logs -n ai-gateway-litellm-system deployment/ai-gateway-litellm-controller-manager -f
```

### Webhook Development
```bash
# Check webhook certificate status
kubectl get certificates -n ai-gateway-litellm-system

# Test webhook validation locally
kubectl apply -f config/samples/gateway_v1alpha1_modelrouter.yaml --dry-run=server
```

## Important Implementation Details

### Configuration Management
- **Config Hash Strategy**: Uses SHA-256 of LiteLLM config as pod template annotation
- **Rolling Updates**: Config changes trigger automatic pod restarts
- **Label Management**: Only manages `app` labels, preserves other operators' labels using `RequiredLabelsPresent()`
- **API Key Management**: Automatically injects environment variables from secrets

### Webhook Implementation
- **Validation Only**: No mutation logic (defaulting handled separately)
- **Provider Format**: Enforces `provider/model-name` format for all AI models
- **Type Flexibility**: Allows any `type` value (not just "litellm") for future extensibility
- **Port Validation**: Ensures positive port numbers

### Resource Reconciliation
- **Owner References**: All created resources have proper controller references
- **Update Detection**: Uses semantic equality checking to avoid unnecessary updates
- **Error Handling**: Proper status conditions and event logging
- **Cleanup**: Automatic garbage collection via owner references

### Security Configuration
```yaml
# Required secret for API keys (optional keys won't fail deployment)
apiVersion: v1
kind: Secret
metadata:
  name: litellm-secrets
type: Opaque
data:
  OPENAI_API_KEY: <base64-encoded-key>
  ANTHROPIC_API_KEY: <base64-encoded-key>
  GEMINI_API_KEY: <base64-encoded-key>
```

## Code Architecture Notes

### Controller Separation of Concerns
- **Webhook**: Validates at admission time, rejects invalid resources
- **Controller**: Only processes valid resources, focuses on reconciliation
- **Generators**: Create provider-specific configurations (currently LiteLLM only)

### String Conversion Best Practices
- Uses `strconv.Itoa(int(modelRouter.Spec.Port))` instead of `fmt.Sprintf("%d", ...)` for efficiency
- Port type is `int32` in Kubernetes but needs `int` for `strconv.Itoa`

### Label Management Strategy
- Controller only sets `app: <modelrouter-name>` label
- Uses `RequiredLabelsPresent()` to check only our required labels
- Preserves labels set by other operators or users
- No longer uses `type` or `config-hash` labels (removed for simplicity)

## Development Tips

1. **Local Development**: Use `make run` to run controller outside cluster for easier debugging
2. **Config Validation**: LiteLLM validates provider-specific settings at runtime
3. **Resource Management**: Consider adding resource requests/limits for production use
4. **Monitoring**: Prometheus metrics available but currently commented out (no prometheus-operator in local clusters)