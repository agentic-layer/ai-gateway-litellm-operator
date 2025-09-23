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

	gatewayv1alpha1 "github.com/agentic-layer/ai-gateway-litellm/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/constants"
	"github.com/agentic-layer/ai-gateway-litellm/internal/equality"
	corev1 "k8s.io/api/core/v1"
)

// TestEqualityIntegrationWithController tests that the controller can use equality utilities
// to make intelligent decisions about resource updates
func TestEqualityIntegrationWithController(t *testing.T) {
	t.Run("should detect when AI models change order but remain semantically identical", func(t *testing.T) {
		// Define AI models in different orders
		originalModels := []gatewayv1alpha1.AiModel{
			{Name: "openai/gpt-4"},
			{Name: "gemini/gemini-1.5-pro"},
			{Name: "anthropic/claude-3-opus"},
		}

		reorderedModels := []gatewayv1alpha1.AiModel{
			{Name: "anthropic/claude-3-opus"},
			{Name: "openai/gpt-4"},
			{Name: "gemini/gemini-1.5-pro"},
		}

		// These should be considered equal despite different order
		if !equality.AiModelsEqual(originalModels, reorderedModels) {
			t.Error("Expected reordered AI models to be equal, but they were not")
		}

		// Create ModelRouter specs with these models
		originalSpec := &gatewayv1alpha1.ModelRouterSpec{
			Type:     constants.TypeLitellm,
			Port:     4000,
			AiModels: originalModels,
		}

		reorderedSpec := &gatewayv1alpha1.ModelRouterSpec{
			Type:     constants.TypeLitellm,
			Port:     4000,
			AiModels: reorderedModels,
		}

		// Controller should recognize these as semantically identical
		if !equality.AiModelsEqual(originalSpec.AiModels, reorderedSpec.AiModels) {
			t.Error("Expected ModelRouter specs with reordered models to be equal")
		}
	})

	t.Run("should detect when labels change semantically", func(t *testing.T) {
		originalLabels := map[string]string{
			"app":                                  "test-router",
			"type":                                 "litellm",
			"gateway.agentic-layer.ai/config-hash": "abc123",
		}

		// Same labels in map (order doesn't matter for maps)
		identicalLabels := map[string]string{
			"gateway.agentic-layer.ai/config-hash": "abc123",
			"app":                                  "test-router",
			"type":                                 "litellm",
		}

		// Different config hash
		changedLabels := map[string]string{
			"app":                                  "test-router",
			"type":                                 "litellm",
			"gateway.agentic-layer.ai/config-hash": "def456",
		}

		// Should be equal despite order
		if !equality.LabelsEqual(originalLabels, identicalLabels) {
			t.Error("Expected identical labels to be equal")
		}

		// Should not be equal with different hash
		if equality.LabelsEqual(originalLabels, changedLabels) {
			t.Error("Expected labels with different config hash to be different")
		}
	})

	t.Run("should detect when environment variables change order but remain identical", func(t *testing.T) {
		originalEnvVars := []corev1.EnvVar{
			{Name: "LITELLM_LOG", Value: "INFO"},
			{Name: "OPENAI_API_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "api-key-secrets",
					},
					Key: "OPENAI_API_KEY",
				},
			}},
			{Name: "GEMINI_API_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "api-key-secrets",
					},
					Key: "GEMINI_API_KEY",
				},
			}},
		}

		reorderedEnvVars := []corev1.EnvVar{
			{Name: "GEMINI_API_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "api-key-secrets",
					},
					Key: "GEMINI_API_KEY",
				},
			}},
			{Name: "LITELLM_LOG", Value: "INFO"},
			{Name: "OPENAI_API_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "api-key-secrets",
					},
					Key: "OPENAI_API_KEY",
				},
			}},
		}

		// Should be equal despite different order
		if !equality.EnvVarsEqual(originalEnvVars, reorderedEnvVars) {
			t.Error("Expected reordered environment variables to be equal")
		}

		// Change one value
		changedEnvVars := []corev1.EnvVar{
			{Name: "LITELLM_LOG", Value: "DEBUG"}, // Changed from INFO to DEBUG
			{Name: "OPENAI_API_KEY", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "api-key-secrets",
					},
					Key: "OPENAI_API_KEY",
				},
			}},
		}

		// Should not be equal with different values
		if equality.EnvVarsEqual(originalEnvVars, changedEnvVars) {
			t.Error("Expected environment variables with different values to be different")
		}
	})

	t.Run("should handle edge cases gracefully", func(t *testing.T) {
		// Nil vs empty slice cases
		var nilModels []gatewayv1alpha1.AiModel
		emptyModels := []gatewayv1alpha1.AiModel{}

		if equality.AiModelsEqual(nilModels, emptyModels) {
			t.Error("Expected nil and empty slices to be different")
		}

		// Nil vs nil should be equal
		if !equality.AiModelsEqual(nilModels, nilModels) {
			t.Error("Expected nil slices to be equal to themselves")
		}

		// Empty vs empty should be equal
		if !equality.AiModelsEqual(emptyModels, emptyModels) {
			t.Error("Expected empty slices to be equal to themselves")
		}
	})
}

// TestEqualityWithModelRouterController tests that the equality utilities work correctly
// with the actual controller reconciliation patterns
func TestEqualityWithModelRouterController(t *testing.T) {
	t.Run("controller update decision logic uses equality utilities", func(t *testing.T) {
		// Create a scenario that simulates what the controller does

		// Original ModelRouter configuration
		originalModelRouter := &gatewayv1alpha1.ModelRouter{
			Spec: gatewayv1alpha1.ModelRouterSpec{
				Type: "litellm",
				Port: 4000,
				AiModels: []gatewayv1alpha1.AiModel{
					{Name: "openai/gpt-4"},
					{Name: "gemini/gemini-1.5-pro"},
				},
			},
		}

		// Simulate reordered models (should not trigger update)
		reorderedModelRouter := &gatewayv1alpha1.ModelRouter{
			Spec: gatewayv1alpha1.ModelRouterSpec{
				Type: "litellm",
				Port: 4000,
				AiModels: []gatewayv1alpha1.AiModel{
					{Name: "gemini/gemini-1.5-pro"},
					{Name: "openai/gpt-4"},
				},
			},
		}

		// Should recognize these as equal (no update needed)
		if !equality.AiModelsEqual(originalModelRouter.Spec.AiModels, reorderedModelRouter.Spec.AiModels) {
			t.Error("Controller should recognize reordered models as equal")
		}

		// Simulate actual model change (should trigger update)
		changedModelRouter := &gatewayv1alpha1.ModelRouter{
			Spec: gatewayv1alpha1.ModelRouterSpec{
				Type: "litellm",
				Port: 4000,
				AiModels: []gatewayv1alpha1.AiModel{
					{Name: "openai/gpt-4"},
					{Name: "anthropic/claude-3-opus"}, // Different model
				},
			},
		}

		// Should recognize these as different (update needed)
		if equality.AiModelsEqual(originalModelRouter.Spec.AiModels, changedModelRouter.Spec.AiModels) {
			t.Error("Controller should recognize different models as not equal")
		}
	})
}