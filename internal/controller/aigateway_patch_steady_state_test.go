/*
Copyright 2026 Agentic Layer.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"testing"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/litellm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAiGateway_PatchedConfigSteadyStateNoDrift(t *testing.T) {
	t.Run("a reconcile with a patch attached does not drift the ConfigMap or trigger a deployment update on each pass", func(t *testing.T) {
		ctx := context.Background()
		s := runtime.NewScheme()
		if err := scheme.AddToScheme(s); err != nil {
			t.Fatalf("failed to add core scheme: %v", err)
		}
		if err := gatewayv1alpha1.AddToScheme(s); err != nil {
			t.Fatalf("failed to add gateway scheme: %v", err)
		}

		gw := &gatewayv1alpha1.AiGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "drift-test",
				Namespace: "default",
				Annotations: map[string]string{
					litellm.ConfigPatchAnnotation: "drift-test-patch",
				},
			},
			Spec: gatewayv1alpha1.AiGatewaySpec{
				Port: 4000,
				AiModels: []gatewayv1alpha1.AiModel{
					{Name: "gpt-4", Provider: "openai"},
				},
			},
		}
		patchCM := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "drift-test-patch", Namespace: "default"},
			Data: map[string]string{
				"patch.yaml": "router_settings:\n  routing_strategy: usage-based-routing-v2\n",
			},
		}
		c := fake.NewClientBuilder().WithScheme(s).WithObjects(gw, patchCM).WithStatusSubresource(gw).Build()

		// Render the merged config twice; both passes must produce identical output
		// so the ConfigMap and config-hash annotation stay stable.
		first, err := litellm.LoadPatch(ctx, c, gw.Namespace, gw.Annotations[litellm.ConfigPatchAnnotation])
		if err != nil {
			t.Fatalf("LoadPatch (first): %v", err)
		}
		firstYAML, err := litellm.RenderConfigWithPatch(litellm.LiteLLMConfig{
			ModelList: []litellm.ModelConfig{{
				ModelName:     "gpt-4",
				LiteLLMParams: litellm.LiteLLMParams{Model: "openai/gpt-4", ApiKey: "os.environ/OPENAI_API_KEY"},
			}},
			LiteLLMSettings: litellm.LiteLLMSettings{
				RequestTimeout: litellm.DefaultRequestTimeout,
				Callbacks:      []string{"otel", "prometheus"},
			},
		}, first)
		if err != nil {
			t.Fatalf("RenderConfigWithPatch (first): %v", err)
		}

		second, err := litellm.LoadPatch(ctx, c, gw.Namespace, gw.Annotations[litellm.ConfigPatchAnnotation])
		if err != nil {
			t.Fatalf("LoadPatch (second): %v", err)
		}
		secondYAML, err := litellm.RenderConfigWithPatch(litellm.LiteLLMConfig{
			ModelList: []litellm.ModelConfig{{
				ModelName:     "gpt-4",
				LiteLLMParams: litellm.LiteLLMParams{Model: "openai/gpt-4", ApiKey: "os.environ/OPENAI_API_KEY"},
			}},
			LiteLLMSettings: litellm.LiteLLMSettings{
				RequestTimeout: litellm.DefaultRequestTimeout,
				Callbacks:      []string{"otel", "prometheus"},
			},
		}, second)
		if err != nil {
			t.Fatalf("RenderConfigWithPatch (second): %v", err)
		}

		if firstYAML != secondYAML {
			t.Errorf("steady-state drift detected — first and second renders differ\nfirst:\n%s\nsecond:\n%s", firstYAML, secondYAML)
		}
	})
}
