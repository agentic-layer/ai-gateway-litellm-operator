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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLiteLLMGenerator_mapModelToLiteLLMFormat(t *testing.T) {
	generator := &LiteLLMGenerator{}

	tests := []struct {
		name           string
		modelName      string
		expectedModel  string
		expectedEnvVar string
	}{
		{
			name:           "OpenAI model with provider prefix",
			modelName:      "openai/gpt-3.5-turbo",
			expectedModel:  "openai/gpt-3.5-turbo",
			expectedEnvVar: "OPENAI_API_KEY",
		},
		{
			name:           "OpenAI model with different variant",
			modelName:      "openai/gpt-4-turbo",
			expectedModel:  "openai/gpt-4-turbo",
			expectedEnvVar: "OPENAI_API_KEY",
		},
		{
			name:           "Azure model",
			modelName:      "azure/gpt-4",
			expectedModel:  "azure/gpt-4",
			expectedEnvVar: "AZURE_API_KEY",
		},
		{
			name:           "Gemini model",
			modelName:      "gemini/gemini-1.5-pro",
			expectedModel:  "gemini/gemini-1.5-pro",
			expectedEnvVar: "GEMINI_API_KEY",
		},
		{
			name:           "Anthropic model",
			modelName:      "anthropic/claude-3-opus",
			expectedModel:  "anthropic/claude-3-opus",
			expectedEnvVar: "ANTHROPIC_API_KEY",
		},
		{
			name:           "Bedrock model",
			modelName:      "bedrock/claude-3-sonnet",
			expectedModel:  "bedrock/claude-3-sonnet",
			expectedEnvVar: "AWS_ACCESS_KEY_ID",
		},
		{
			name:           "Ollama model (no API key needed)",
			modelName:      "ollama/llama2",
			expectedModel:  "ollama/llama2",
			expectedEnvVar: "",
		},
		{
			name:           "HuggingFace model",
			modelName:      "huggingface/microsoft/DialoGPT-large",
			expectedModel:  "huggingface/microsoft/DialoGPT-large",
			expectedEnvVar: "HUGGINGFACE_API_KEY",
		},
		{
			name:           "Unknown provider with slash",
			modelName:      "custom/my-model",
			expectedModel:  "custom/my-model",
			expectedEnvVar: "CUSTOM_API_KEY",
		},
		{
			name:           "Model name without provider prefix",
			modelName:      "gpt-3.5-turbo",
			expectedModel:  "gpt-3.5-turbo",
			expectedEnvVar: "GPT-3.5-TURBO_API_KEY",
		},
		{
			name:           "Empty model name",
			modelName:      "",
			expectedModel:  "",
			expectedEnvVar: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			litellmModel, apiKeyEnvVar := generator.mapModelToLiteLLMFormat(tt.modelName)

			assert.Equal(t, tt.expectedModel, litellmModel, "Model name should be used directly")
			assert.Equal(t, tt.expectedEnvVar, apiKeyEnvVar, "Environment variable should match expected value")
		})
	}
}
