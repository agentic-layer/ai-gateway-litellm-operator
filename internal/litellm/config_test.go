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
	"strings"
	"testing"
)

func TestRenderConfig_ModelsOnly(t *testing.T) {
	cfg := LiteLLMConfig{
		ModelList: []ModelConfig{
			{
				ModelName:     "gpt-4",
				LiteLLMParams: LiteLLMParams{Model: "openai/gpt-4", ApiKey: "os.environ/OPENAI_API_KEY"},
			},
		},
		LiteLLMSettings: LiteLLMSettings{
			RequestTimeout: DefaultRequestTimeout,
			Callbacks:      []string{"otel", "prometheus"},
		},
	}
	got, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	for _, want := range []string{
		"model_list:",
		"model_name: gpt-4",
		"model: openai/gpt-4",
		"api_key: os.environ/OPENAI_API_KEY",
		"request_timeout: 600",
		"- otel",
		"- prometheus",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderConfig output missing %q\nfull output:\n%s", want, got)
		}
	}
}

func TestRenderConfig_OmitsEmptyBlocks(t *testing.T) {
	cfg := LiteLLMConfig{
		LiteLLMSettings: LiteLLMSettings{RequestTimeout: 600},
	}
	got, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	if strings.Contains(got, "guardrails:") {
		t.Errorf("expected guardrails: to be omitted when empty\noutput:\n%s", got)
	}
}

func TestRenderConfig_GuardrailsBlock(t *testing.T) {
	cfg := LiteLLMConfig{
		Guardrails: []GuardrailConfig{
			{
				GuardrailName: "my-guard",
				LiteLLMParams: GuardrailLiteLLMParams{
					Guardrail:               "presidio",
					Mode:                    []string{"pre_call", "post_call"},
					DefaultOn:               true,
					OutputParsePii:          true,
					PresidioAnalyzerApiBase: "http://presidio.svc:5002",
				},
			},
		},
	}
	got, err := RenderConfig(cfg)
	if err != nil {
		t.Fatalf("RenderConfig: %v", err)
	}
	for _, want := range []string{
		"guardrails:",
		"guardrail_name: my-guard",
		"guardrail: presidio",
		"- pre_call",
		"- post_call",
		"default_on: true",
		"output_parse_pii: true",
		"presidio_analyzer_api_base: http://presidio.svc:5002",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderConfig output missing %q\nfull output:\n%s", want, got)
		}
	}
}
