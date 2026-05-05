/*
Copyright 2026 Agentic Layer.

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

package litellm

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// LiteLLMConfig is the top-level config rendered to config.yaml inside the proxy pod.
type LiteLLMConfig struct {
	ModelList       []ModelConfig        `yaml:"model_list,omitempty"`
	McpServers      map[string]McpServer `yaml:"mcp_servers,omitempty"`
	LiteLLMSettings LiteLLMSettings      `yaml:"litellm_settings,omitempty"`
	Guardrails      []GuardrailConfig    `yaml:"guardrails,omitempty"`
}

// ModelConfig is one entry under model_list.
type ModelConfig struct {
	ModelName     string        `yaml:"model_name"`
	LiteLLMParams LiteLLMParams `yaml:"litellm_params"`
}

// LiteLLMParams holds the litellm_params for a single model entry.
type LiteLLMParams struct {
	Model  string `yaml:"model"`
	ApiKey string `yaml:"api_key,omitempty"`
}

// McpServer is one entry under mcp_servers, keyed by the controller-side
// mcpServerKey helper. LiteLLM exposes each entry at /mcp/<key> on the proxy.
//
// AllowedTools / DisallowedTools are passed through verbatim from
// ToolRoute.spec.toolFilter.{allow,deny}; LiteLLM owns the glob semantics.
type McpServer struct {
	Url             string   `yaml:"url"`
	Transport       string   `yaml:"transport,omitempty"`
	AllowedTools    []string `yaml:"allowed_tools,omitempty"`
	DisallowedTools []string `yaml:"disallowed_tools,omitempty"`
	AllowAllKeys    bool     `yaml:"allow_all_keys,omitempty"`
}

// LiteLLMSettings is the litellm_settings block.
type LiteLLMSettings struct {
	RequestTimeout int      `yaml:"request_timeout,omitempty"`
	Callbacks      []string `yaml:"callbacks,omitempty"`
}

// GuardrailConfig is one entry under the top-level guardrails list.
type GuardrailConfig struct {
	GuardrailName string                 `yaml:"guardrail_name"`
	LiteLLMParams GuardrailLiteLLMParams `yaml:"litellm_params"`
}

// GuardrailLiteLLMParams holds the LiteLLM-specific parameters for a guardrail.
type GuardrailLiteLLMParams struct {
	// Guardrail is the LiteLLM guardrail type identifier (e.g. "presidio").
	Guardrail string `yaml:"guardrail"`
	// Mode defines when the guardrail is applied. Multiple modes can be specified.
	Mode []string `yaml:"mode"`
	// DefaultOn ensures the guardrail is applied to every request without requiring
	// explicit opt-in per call.
	DefaultOn bool `yaml:"default_on"`
	// OutputParsePii enables automatic unmasking of PII tokens in LLM responses.
	// When true, masked tokens (e.g. <PERSON_1>) are replaced with original values.
	// Only used when Guardrail is "presidio".
	OutputParsePii bool `yaml:"output_parse_pii,omitempty"`
	// PresidioAnalyzerApiBase is the URL of the Presidio Analyzer service.
	// Only used when Guardrail is "presidio".
	PresidioAnalyzerApiBase string `yaml:"presidio_analyzer_api_base,omitempty"`
	// PresidioAnonymizerApiBase is the URL of the Presidio Anonymizer service.
	// Only used when Guardrail is "presidio".
	PresidioAnonymizerApiBase string `yaml:"presidio_anonymizer_api_base,omitempty"`
	// PresidioLanguage is the language code for PII detection (e.g. "en", "de").
	// Only used when Guardrail is "presidio".
	PresidioLanguage string `yaml:"presidio_language,omitempty"`
	// PresidioScoreThresholds maps entity types to minimum confidence scores (0.0 to 1.0).
	// Use "ALL" as key to set a default threshold for all entity types.
	// Only used when Guardrail is "presidio".
	PresidioScoreThresholds map[string]string `yaml:"presidio_score_thresholds,omitempty"`
	// PiiEntitiesConfig maps PII entity types to actions ("MASK" or "BLOCK").
	// Only used when Guardrail is "presidio".
	PiiEntitiesConfig map[string]string `yaml:"pii_entities_config,omitempty"`
}

// RenderConfig marshals the LiteLLMConfig to YAML.
func RenderConfig(cfg LiteLLMConfig) (string, error) {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal LiteLLM config: %w", err)
	}
	return string(out), nil
}
