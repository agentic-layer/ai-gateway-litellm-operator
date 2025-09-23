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

// ModelRouter provider types
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
	// ModelRouterConfigured indicates if the ModelRouter configuration is valid
	ModelRouterConfigured = "ModelRouterConfigured"

	// ModelRouterReady indicates if the ModelRouter is ready to serve traffic
	ModelRouterReady = "ModelRouterReady"

	// ModelRouterDiscoveryFailed indicates discovery failures
	ModelRouterDiscoveryFailed = "ModelRouterDiscoveryFailed"
)

// Condition reasons
const (
	// ReasonConfigurationValid indicates valid configuration
	ReasonConfigurationValid = "ConfigurationValid"

	// ReasonConfigurationInvalid indicates invalid configuration
	ReasonConfigurationInvalid = "ConfigurationInvalid"

	// ReasonConfigurationApplied indicates successful configuration application
	ReasonConfigurationApplied = "ConfigurationApplied"

	// ReasonModelRouterReady indicates ModelRouter is ready
	ReasonModelRouterReady = "ModelRouterReady"
)

// ModelRouterKind is the kind name for ModelRouter resources
const ModelRouterKind = "ModelRouter"