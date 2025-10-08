# AI Gateway LiteLLM Operator - Developer Guide

@README.md
@docs/modules/ROOT/partials/how-to-guides.adoc

## Overview

The AI Gateway LiteLLM is a Kubernetes operator built with the Operator SDK framework that manages [LiteLLM](https://www.litellm.ai/) deployments. This operator uses the `AiGateway` custom resource (provided by [ai-gateway-operator](https://github.com/agentic-layer/ai-gateway-operator)) to route requests to multiple AI model providers (OpenAI, Anthropic, Gemini, Azure, AWS Bedrock, etc.) through a single unified interface.

**Important**: This operator requires the [AI Gateway Operator](https://github.com/agentic-layer/ai-gateway-operator) to be installed first, as it provides the `AiGateway` and `AiGatewayClass` CRDs.

**Technology Stack:**
- **Framework**: Operator SDK v1.28+ (based on controller-runtime)
- **Language**: Go 1.24+
- **Kubernetes**: 1.28+ (uses admission webhooks, CRDs)
- **AI Gateway**: LiteLLM proxy for multi-provider AI model routing

## Architecture Overview

### Core Components

1. **AiGateway Controller** (`internal/controller/aigateway_controller.go`)
   - Reconciles AiGateway custom resources from the ai-gateway-operator
   - Creates ConfigMap, Deployment, and Service resources for LiteLLM
   - Manages LiteLLM container lifecycle
   - Uses config-hash annotations to trigger pod restarts on configuration changes
   - Implements `shouldProcessAiGateway()` to check for matching AiGatewayClass

2. **Configuration Generation** (in controller)
   - LiteLLM-specific configuration generation
   - Handles provider-specific model mapping with `Provider` and `Name` fields
   - Generates YAML configuration for LiteLLM proxy
   - Maps AI models to environment variables for API keys

3. **Equality Utilities** (`internal/equality/`)
   - Semantic comparison for Kubernetes resources
   - Supports comparing environment variables, labels, AI model lists (including provider field)
   - Uses `RequiredLabelsPresent()` to respect labels from other operators

### Project Structure
```
├── config/                 # Kustomize configurations
│   ├── crd/               # CRD test manifests (actual CRDs from ai-gateway-operator)
│   ├── samples/           # Sample AiGateway resources
│   └── default/           # Default deployment configuration
├── internal/
│   ├── controller/        # Controller logic (AiGateway reconciliation)
│   ├── equality/          # Resource comparison utilities
│   └── constants/         # Shared constants
└── test/e2e/              # End-to-end tests
```

**Note**: This operator does NOT define its own CRDs. It uses `AiGateway` and `AiGatewayClass` from the ai-gateway-operator.

## Advanced AiGateway Examples

### Multi-Provider Enterprise Setup
```yaml
apiVersion: agentic-layer.ai/v1alpha1
kind: AiGatewayClass
metadata:
  name: litellm
  annotations:
    aigatewayclass.kubernetes.io/is-default-class: "true"
spec:
  controller: aigateway.agentic-layer.ai/ai-gateway-litellm-controller
---
apiVersion: agentic-layer.ai/v1alpha1
kind: AiGateway
metadata:
  name: enterprise-ai-gateway
spec:
  aiGatewayClassName: litellm
  port: 8080
  aiModels:
    - name: gpt-4-turbo
      provider: openai
    - name: gpt-4
      provider: azure
    - name: claude-3-sonnet
      provider: bedrock
    - name: llama2
      provider: ollama  # Local models (no API key needed)
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

## Important Implementation Details

### Configuration Management
- **Config Hash Strategy**: Uses SHA-256 of LiteLLM config as pod template annotation
- **Rolling Updates**: Config changes trigger automatic pod restarts
- **Label Management**: Only manages `app` labels, preserves other operators' labels using `RequiredLabelsPresent()`
- **API Key Management**: Automatically injects environment variables from secrets based on provider names

### AiGateway Processing
- **Controller Selection**: Uses `AiGatewayClass` with controller name `aigateway.agentic-layer.ai/ai-gateway-litellm-controller`
- **Default Class**: Supports default AiGatewayClass via annotation `aigatewayclass.kubernetes.io/is-default-class: "true"`
- **Namespace Scoping**: Both AiGateway and AiGatewayClass are namespace-scoped resources
- **Provider/Name Fields**: AI models use separate `provider` and `name` fields (e.g., `provider: openai`, `name: gpt-4`)

### Webhook Implementation
- **No Webhooks**: This operator does NOT implement webhooks
- **Validation**: Handled by the ai-gateway-operator's webhooks
- **CRD Ownership**: All CRDs are owned and managed by ai-gateway-operator

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
  name: api-key-secrets
type: Opaque
data:
  OPENAI_API_KEY: <base64-encoded-key>
  ANTHROPIC_API_KEY: <base64-encoded-key>
  GEMINI_API_KEY: <base64-encoded-key>
```

## Code Architecture Notes

### Controller Responsibilities
- **Controller**: Processes AiGateway resources with matching AiGatewayClass, focuses on reconciliation
- **Configuration Generation**: Creates LiteLLM-specific configurations inline (no separate generator files)
- **Class Filtering**: Only processes AiGateways that reference this controller via AiGatewayClass

### String Conversion Best Practices
- Uses `strconv.Itoa(int(aiGateway.Spec.Port))` instead of `fmt.Sprintf("%d", ...)` for efficiency
- Port type is `int32` in Kubernetes but needs `int` for `strconv.Itoa`

### Label Management Strategy
- Controller only sets `app: <aigateway-name>` label
- Uses `RequiredLabelsPresent()` to check only our required labels
- Preserves labels set by other operators or users
- Config hash stored in annotation, not label

### Model Configuration
- AI models have separate `provider` and `name` fields
- Provider names map to environment variable API keys (e.g., `OPENAI_API_KEY`)
- Environment variables are pulled from secret named `api-key-secrets` (optional)

## Development Tips

1. **Local Development**: Use `make run` to run controller outside cluster for easier debugging
2. **Config Validation**: LiteLLM validates provider-specific settings at runtime
3. **Resource Management**: Consider adding resource requests/limits for production use
4. **Monitoring**: Prometheus metrics available but currently commented out (no prometheus-operator in local clusters)