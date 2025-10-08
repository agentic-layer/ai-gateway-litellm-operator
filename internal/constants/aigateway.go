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

package constants

// AiGateway provider types
const (
	// TypeLitellm is the LiteLLM provider type
	TypeLitellm = "litellm"
)

// Configuration constants
const (
	// DefaultRequestTimeout is the default timeout for LiteLLM requests in seconds
	DefaultRequestTimeout = 600
)

// Secret and API key constants
const (
	// DefaultSecretName is the default name for the API keys secret
	DefaultSecretName = "api-key-secrets"
)

// Status condition types
const (
	// AiGatewayConfigured indicates if the AiGateway configuration is valid
	AiGatewayConfigured = "AiGatewayConfigured"

	// AiGatewayReady indicates if the AiGateway is ready to serve traffic
	AiGatewayReady = "AiGatewayReady"
)

// Condition reasons
const (
	// ReasonConfigurationApplied indicates successful configuration application
	ReasonConfigurationApplied = "ConfigurationApplied"

	// ReasonAiGatewayReady indicates AiGateway is ready
	ReasonAiGatewayReady = "AiGatewayReady"
)
