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
	"context"
	"fmt"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// GuardrailTarget selects the LiteLLM mode dialect to render. The Guard CRD
// exposes a single `pre_call`/`post_call`/`during_call` vocabulary, but LiteLLM
// distinguishes LLM-call guardrails (chat completions, etc.) from MCP-call
// guardrails (tools/call). The mapping is applied per-call site so the Guard
// resource itself stays portable across gateway kinds.
type GuardrailTarget int

const (
	// GuardrailTargetLLM applies guardrails to chat-completion-style requests.
	// Modes are passed to LiteLLM verbatim.
	GuardrailTargetLLM GuardrailTarget = iota
	// GuardrailTargetMCP applies guardrails to MCP tools/call requests.
	// LiteLLM uses dedicated `pre_mcp_call` / `during_mcp_call` modes here;
	// `post_call` has no MCP equivalent and is dropped — LiteLLM has no
	// `post_mcp_call` hook (documented limitation in
	// https://docs.litellm.ai/docs/proxy/guardrails/panw_prisma_airs).
	GuardrailTargetMCP
)

// ResolveGuardrails fetches the Guard and GuardrailProvider resources referenced by
// guardrailRefs (relative to defaultNamespace) and maps them to LiteLLM GuardrailConfig
// entries. Guard modes are translated to the dialect appropriate for target.
// Returns an error if any referenced Guard or Provider cannot be fetched.
// Unsupported provider types and Guards whose modes are all unsupported for the
// target are skipped with a log line.
func ResolveGuardrails(
	ctx context.Context,
	c client.Reader,
	defaultNamespace string,
	guardrailRefs []corev1.ObjectReference,
	target GuardrailTarget,
) ([]GuardrailConfig, error) {
	log := logf.FromContext(ctx)

	if len(guardrailRefs) == 0 {
		return nil, nil
	}

	guardrails := make([]GuardrailConfig, 0, len(guardrailRefs))
	for _, ref := range guardrailRefs {
		namespace := ref.Namespace
		if namespace == "" {
			namespace = defaultNamespace
		}

		var guard gatewayv1alpha1.Guard
		if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, &guard); err != nil {
			return nil, fmt.Errorf("failed to get Guard %s/%s: %w", namespace, ref.Name, err)
		}

		providerNamespace := guard.Spec.ProviderRef.Namespace
		if providerNamespace == "" {
			providerNamespace = guard.Namespace
		}
		providerName := guard.Spec.ProviderRef.Name

		var provider gatewayv1alpha1.GuardrailProvider
		if err := c.Get(ctx, types.NamespacedName{Name: providerName, Namespace: providerNamespace}, &provider); err != nil {
			return nil, fmt.Errorf(
				"failed to get GuardrailProvider %s/%s referenced by Guard %s: %w",
				providerNamespace, providerName, guard.Name, err,
			)
		}

		cfg, err := buildGuardrailConfig(&guard, &provider, target)
		if err != nil {
			log.Error(err, "Skipping guardrail", "guard", guard.Name, "type", provider.Spec.Type, "target", target)
			continue
		}
		guardrails = append(guardrails, cfg)
	}

	return guardrails, nil
}

// buildGuardrailConfig maps a Guard and its GuardrailProvider to a LiteLLM GuardrailConfig.
func buildGuardrailConfig(guard *gatewayv1alpha1.Guard, provider *gatewayv1alpha1.GuardrailProvider, target GuardrailTarget) (GuardrailConfig, error) {
	modes := make([]string, 0, len(guard.Spec.Mode))
	for _, m := range guard.Spec.Mode {
		mapped, ok := mapGuardMode(m, target)
		if !ok {
			continue
		}
		modes = append(modes, mapped)
	}
	if len(modes) == 0 {
		return GuardrailConfig{}, fmt.Errorf("guard %s has no modes compatible with target %v", guard.Name, target)
	}

	params := GuardrailLiteLLMParams{
		Mode:      modes,
		DefaultOn: true,
	}

	switch provider.Spec.Type {
	case "presidio-api":
		if provider.Spec.Presidio == nil {
			return GuardrailConfig{}, fmt.Errorf("GuardrailProvider %s has type presidio-api but no presidio config", provider.Name)
		}
		// The CRD type is "presidio-api" but LiteLLM expects "presidio" as the guardrail identifier.
		params.Guardrail = "presidio"
		// Presidio requires both an Analyzer and an Anonymizer endpoint. The CRD provides a
		// single baseUrl for the Presidio service, which is used for both.
		params.PresidioAnalyzerApiBase = provider.Spec.Presidio.BaseUrl
		params.PresidioAnonymizerApiBase = provider.Spec.Presidio.BaseUrl
		// Enable output parsing by default so masked PII tokens in LLM responses
		// (e.g. <PERSON_1>) are replaced with original values before returning to the user.
		params.OutputParsePii = true
		if guard.Spec.Presidio != nil {
			params.PresidioLanguage = guard.Spec.Presidio.Language
			if len(guard.Spec.Presidio.ScoreThresholds) > 0 {
				params.PresidioScoreThresholds = guard.Spec.Presidio.ScoreThresholds
			}
			if len(guard.Spec.Presidio.EntityActions) > 0 {
				params.PiiEntitiesConfig = guard.Spec.Presidio.EntityActions
			}
		}
	default:
		return GuardrailConfig{}, fmt.Errorf("unsupported guardrail provider type %q for guard %s", provider.Spec.Type, guard.Name)
	}

	return GuardrailConfig{
		GuardrailName: guard.Name,
		LiteLLMParams: params,
	}, nil
}

// mapGuardMode translates a Guard CRD mode to the LiteLLM proxy mode for the
// given target. Returns (mapped, true) when the mode is supported, or
// ("", false) when the target has no equivalent and the mode should be dropped.
func mapGuardMode(mode gatewayv1alpha1.GuardMode, target GuardrailTarget) (string, bool) {
	if target == GuardrailTargetLLM {
		return string(mode), true
	}
	switch mode {
	case gatewayv1alpha1.GuardModePreCall:
		return "pre_mcp_call", true
	case gatewayv1alpha1.GuardModeDuringCall:
		return "during_mcp_call", true
	default:
		// post_call has no LiteLLM MCP equivalent (no post_mcp_call hook).
		return "", false
	}
}
