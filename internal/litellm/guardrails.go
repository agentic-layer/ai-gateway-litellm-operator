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

// ResolveGuardrails fetches the Guard and GuardrailProvider resources referenced by
// guardrailRefs (relative to defaultNamespace) and maps them to LiteLLM GuardrailConfig
// entries. Returns an error if any referenced Guard or Provider cannot be fetched.
// Unsupported provider types are skipped with a log line.
func ResolveGuardrails(
	ctx context.Context,
	c client.Reader,
	defaultNamespace string,
	guardrailRefs []corev1.ObjectReference,
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

		cfg, err := buildGuardrailConfig(&guard, &provider)
		if err != nil {
			log.Error(err, "Skipping unsupported guardrail", "guard", guard.Name, "type", provider.Spec.Type)
			continue
		}
		guardrails = append(guardrails, cfg)
	}

	return guardrails, nil
}

// buildGuardrailConfig maps a Guard and its GuardrailProvider to a LiteLLM GuardrailConfig.
func buildGuardrailConfig(guard *gatewayv1alpha1.Guard, provider *gatewayv1alpha1.GuardrailProvider) (GuardrailConfig, error) {
	// Convert []GuardMode to []string for the LiteLLM YAML config.
	modes := make([]string, len(guard.Spec.Mode))
	for i, m := range guard.Spec.Mode {
		modes[i] = string(m)
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
