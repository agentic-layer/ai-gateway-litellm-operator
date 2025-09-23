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

package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"gopkg.in/yaml.v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"strings"

	gatewayv1alpha1 "github.com/agentic-layer/ai-gateway-litellm/api/v1alpha1"
)

// ModelRouterConfigGenerator defines the interface for generating model router configurations
type ModelRouterConfigGenerator interface {
	// Generate creates the configuration for the model router provider
	Generate(ctx context.Context, modelRouter *gatewayv1alpha1.ModelRouter) (configData string, configHash string, err error)

	// Validate checks if the model router configuration is valid for this provider
	Validate(modelRouter *gatewayv1alpha1.ModelRouter) error

	// GetDefaultConfig returns the default configuration template
	GetDefaultConfig() map[string]interface{}
}

// NewModelRouterGenerator creates a model router configuration generator
// Currently only LiteLLM is supported
func NewModelRouterGenerator(provider string) (ModelRouterConfigGenerator, error) {
	switch provider {
	case "litellm", "": // Default to LiteLLM
		return &LiteLLMGenerator{}, nil
	default:
		return nil, fmt.Errorf("unsupported model router provider '%s': only 'litellm' is currently supported", provider)
	}
}

// LiteLLMGenerator implements LiteLLM configuration generation
type LiteLLMGenerator struct{}

// LiteLLM configuration structs
type LiteLLMConfig struct {
	ModelList       []ModelConfig   `yaml:"model_list"`
	LiteLLMSettings LiteLLMSettings `yaml:"litellm_settings,omitempty"`
}

type ModelConfig struct {
	ModelName     string        `yaml:"model_name"`
	LiteLLMParams LiteLLMParams `yaml:"litellm_params"`
}

type LiteLLMParams struct {
	Model  string `yaml:"model"`
	ApiKey string `yaml:"api_key,omitempty"`
}

type LiteLLMSettings struct {
	RequestTimeout int `yaml:"request_timeout,omitempty"`
}

type GeneralSettings struct {
	MasterKey string `yaml:"master_key"`
	Port      int32  `yaml:"port"`
}

// GetDefaultConfig returns LiteLLM default configuration template
func (g *LiteLLMGenerator) GetDefaultConfig() map[string]interface{} {
	return map[string]interface{}{
		"general_settings": map[string]interface{}{
			"master_key": "your-master-key-here",
			"port":       8000,
		},
		"model_list": []interface{}{},
	}
}

// Validate checks LiteLLM-specific configuration
func (g *LiteLLMGenerator) Validate(modelRouter *gatewayv1alpha1.ModelRouter) error {
	if modelRouter.Spec.Type == "" {
		return fmt.Errorf("modelRouter type is required")
	}

	if modelRouter.Spec.Type != "litellm" {
		return fmt.Errorf("unsupported modelRouter type: %s, only 'litellm' is supported", modelRouter.Spec.Type)
	}

	if modelRouter.Spec.Port <= 0 {
		return fmt.Errorf("modelRouter port must be positive, got: %d", modelRouter.Spec.Port)
	}

	if len(modelRouter.Spec.AiModels) == 0 {
		return fmt.Errorf("no AI models specified in ModelRouter")
	}

	// Validate AI model names
	for _, model := range modelRouter.Spec.AiModels {
		if model.Name == "" {
			return fmt.Errorf("AI model name cannot be empty")
		}
	}

	return nil
}

// Generate creates LiteLLM YAML configuration
func (g *LiteLLMGenerator) Generate(ctx context.Context, modelRouter *gatewayv1alpha1.ModelRouter) (string, string, error) {
	log := logf.FromContext(ctx)

	// Build model list with proper provider prefixes and environment variable API keys
	modelList := make([]ModelConfig, len(modelRouter.Spec.AiModels))
	for i, model := range modelRouter.Spec.AiModels {
		// Map model names to proper LiteLLM format with provider prefix
		litellmModel, apiKeyEnvVar := g.mapModelToLiteLLMFormat(model.Name)

		modelConfig := ModelConfig{
			ModelName: model.Name,
			LiteLLMParams: LiteLLMParams{
				Model: litellmModel,
			},
		}

		// Add API key environment variable reference if needed
		if apiKeyEnvVar != "" {
			modelConfig.LiteLLMParams.ApiKey = fmt.Sprintf("os.environ/%s", apiKeyEnvVar)
		}

		modelList[i] = modelConfig
	}

	// Build complete configuration with settings
	config := LiteLLMConfig{
		ModelList: modelList,
		LiteLLMSettings: LiteLLMSettings{
			RequestTimeout: 600, // 10 minutes default timeout
		},
	}

	// Generate YAML configuration
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal LiteLLM config: %w", err)
	}

	// Generate configuration hash
	hash := sha256.Sum256(configYAML)
	configHash := fmt.Sprintf("%x", hash)[:16]

	log.Info("Generated LiteLLM configuration", "modelRouter", modelRouter.Name, "models", len(modelRouter.Spec.AiModels), "configHash", configHash[:8])

	return string(configYAML), configHash, nil
}

// mapModelToLiteLLMFormat uses AI model names directly as configured by users
// and derives the appropriate environment variables for API keys based on the provider prefix
func (g *LiteLLMGenerator) mapModelToLiteLLMFormat(modelName string) (litellmModel string, apiKeyEnvVar string) {
	// Use the model name directly as provided by the user
	litellmModel = modelName

	// Derive environment variable based on provider prefix
	if len(modelName) > 0 {
		// Extract provider from model name (everything before the first '/')
		providerEnd := len(modelName)
		if slashIndex := strings.Index(modelName, "/"); slashIndex != -1 {
			providerEnd = slashIndex
		}

		provider := strings.ToLower(modelName[:providerEnd])

		switch provider {
		case "openai":
			apiKeyEnvVar = "OPENAI_API_KEY"
		case "azure":
			apiKeyEnvVar = "AZURE_API_KEY"
		case "gemini":
			apiKeyEnvVar = "GEMINI_API_KEY"
		case "anthropic":
			apiKeyEnvVar = "ANTHROPIC_API_KEY"
		case "bedrock":
			apiKeyEnvVar = "AWS_ACCESS_KEY_ID" // Bedrock uses AWS credentials
		case "ollama":
			// Ollama typically doesn't require API keys for local deployments
			apiKeyEnvVar = ""
		case "huggingface":
			apiKeyEnvVar = "HUGGINGFACE_API_KEY"
		default:
			// For unknown providers, generate a standardized env var name
			apiKeyEnvVar = strings.ToUpper(provider) + "_API_KEY"
		}
	}

	return litellmModel, apiKeyEnvVar
}
