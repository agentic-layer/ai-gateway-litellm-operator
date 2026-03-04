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
	"testing"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/equality"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestEqualityIntegrationWithController tests that the controller can use equality utilities
// to make intelligent decisions about resource updates
func TestEqualityIntegrationWithController(t *testing.T) {
	t.Run("should detect when AI models change order but remain semantically identical", func(t *testing.T) {
		// Define AI models in different orders
		originalModels := []gatewayv1alpha1.AiModel{
			{Name: "gpt-4", Provider: "openai"},
			{Name: "gemini-1.5-pro", Provider: "gemini"},
			{Name: "claude-3-opus", Provider: "anthropic"},
		}

		reorderedModels := []gatewayv1alpha1.AiModel{
			{Name: "claude-3-opus", Provider: "anthropic"},
			{Name: "gpt-4", Provider: "openai"},
			{Name: "gemini-1.5-pro", Provider: "gemini"},
		}

		// These should be considered equal despite different order
		if !equality.AiModelsEqual(originalModels, reorderedModels) {
			t.Error("Expected reordered AI models to be equal, but they were not")
		}

		// Create AiGateway specs with these models
		originalSpec := &gatewayv1alpha1.AiGatewaySpec{
			Port:     4000,
			AiModels: originalModels,
		}

		reorderedSpec := &gatewayv1alpha1.AiGatewaySpec{
			Port:     4000,
			AiModels: reorderedModels,
		}

		// Controller should recognize these as semantically identical
		if !equality.AiModelsEqual(originalSpec.AiModels, reorderedSpec.AiModels) {
			t.Error("Expected AiGateway specs with reordered models to be equal")
		}
	})

	t.Run("should detect when labels change semantically", func(t *testing.T) {
		originalLabels := map[string]string{
			"app":                                  "test-gateway",
			"type":                                 "litellm",
			"gateway.agentic-layer.ai/config-hash": "abc123",
		}

		// Same labels in map (order doesn't matter for maps)
		identicalLabels := map[string]string{
			"gateway.agentic-layer.ai/config-hash": "abc123",
			"app":                                  "test-gateway",
			"type":                                 "litellm",
		}

		// Different config hash
		changedLabels := map[string]string{
			"app":                                  "test-gateway",
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

// TestBuildPodTemplateAnnotationsWithSecretHash tests that buildPodTemplateAnnotations
// includes the secret-hash annotation in addition to the config-hash annotation.
func TestBuildPodTemplateAnnotationsWithSecretHash(t *testing.T) {
	aiGateway := &gatewayv1alpha1.AiGateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway",
			Namespace: "default",
		},
		Spec: gatewayv1alpha1.AiGatewaySpec{
			Port: 4000,
		},
	}

	configHash := "abcd1234efgh5678"
	secretHash := "1111222233334444"

	annotations := buildPodTemplateAnnotations(aiGateway, configHash, secretHash)

	if annotations["gateway.agentic-layer.ai/config-hash"] != configHash {
		t.Errorf("Expected config-hash %q, got %q", configHash, annotations["gateway.agentic-layer.ai/config-hash"])
	}
	if annotations["gateway.agentic-layer.ai/secret-hash"] != secretHash {
		t.Errorf("Expected secret-hash %q, got %q", secretHash, annotations["gateway.agentic-layer.ai/secret-hash"])
	}
}

// TestBuildPodTemplateAnnotationsSecretHashOverridesUserAnnotations tests that the operator-managed
// secret-hash annotation always takes precedence over user-provided annotations.
func TestBuildPodTemplateAnnotationsSecretHashOverridesUserAnnotations(t *testing.T) {
	aiGateway := &gatewayv1alpha1.AiGateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-gateway",
			Namespace: "default",
		},
		Spec: gatewayv1alpha1.AiGatewaySpec{
			Port: 4000,
			PodMetadata: &gatewayv1alpha1.EmbeddedMetadata{
				Annotations: map[string]string{
					"gateway.agentic-layer.ai/secret-hash": "user-provided-value",
					"custom-annotation":                    "custom-value",
				},
			},
		},
	}

	secretHash := "operator-managed-hash"
	annotations := buildPodTemplateAnnotations(aiGateway, "somehash", secretHash)

	// Operator-managed secret-hash must take precedence
	if annotations["gateway.agentic-layer.ai/secret-hash"] != secretHash {
		t.Errorf("Operator-managed secret-hash should override user annotation; got %q", annotations["gateway.agentic-layer.ai/secret-hash"])
	}
	// User annotation is preserved
	if annotations["custom-annotation"] != "custom-value" {
		t.Error("User-provided custom annotation should be preserved")
	}
}

// TestComputeSecretHashNoSecret tests that computeSecretHash returns a stable hash
// when the secret does not exist.
func TestComputeSecretHashNoSecret(t *testing.T) {
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("Failed to add core scheme: %v", err)
	}
	if err := gatewayv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("Failed to add gateway scheme: %v", err)
	}

	fakeClient := fake.NewClientBuilder().WithScheme(s).Build()
	reconciler := &AiGatewayReconciler{Client: fakeClient, Scheme: s}

	aiGateway := &gatewayv1alpha1.AiGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
	}

	hash1, err := reconciler.computeSecretHash(context.Background(), aiGateway)
	if err != nil {
		t.Fatalf("Expected no error when secret is missing, got: %v", err)
	}
	hash2, err := reconciler.computeSecretHash(context.Background(), aiGateway)
	if err != nil {
		t.Fatalf("Expected no error on second call, got: %v", err)
	}
	if hash1 != hash2 {
		t.Errorf("Expected stable hash for missing secret, got %q and %q", hash1, hash2)
	}
	if len(hash1) != 16 {
		t.Errorf("Expected hash length 16, got %d", len(hash1))
	}
}

// TestComputeSecretHashWithSecret tests that computeSecretHash changes when secret data changes.
func TestComputeSecretHashWithSecret(t *testing.T) {
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("Failed to add core scheme: %v", err)
	}
	if err := gatewayv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("Failed to add gateway scheme: %v", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultSecretName,
			Namespace: "default",
		},
		Data: map[string][]byte{
			"OPENAI_API_KEY": []byte("key-v1"),
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	reconciler := &AiGatewayReconciler{Client: fakeClient, Scheme: s}

	aiGateway := &gatewayv1alpha1.AiGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
	}

	hash1, err := reconciler.computeSecretHash(context.Background(), aiGateway)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Update the secret
	secret.Data["OPENAI_API_KEY"] = []byte("key-v2")
	fakeClient2 := fake.NewClientBuilder().WithScheme(s).WithObjects(secret).Build()
	reconciler2 := &AiGatewayReconciler{Client: fakeClient2, Scheme: s}

	hash2, err := reconciler2.computeSecretHash(context.Background(), aiGateway)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if hash1 == hash2 {
		t.Error("Expected hash to change when secret data changes")
	}
	if len(hash1) != 16 || len(hash2) != 16 {
		t.Errorf("Expected hash length 16, got %d and %d", len(hash1), len(hash2))
	}
}
func TestEqualityWithAiGatewayController(t *testing.T) {
	t.Run("controller update decision logic uses equality utilities", func(t *testing.T) {
		// Create a scenario that simulates what the controller does

		// Original AiGateway configuration
		originalAiGateway := &gatewayv1alpha1.AiGateway{
			Spec: gatewayv1alpha1.AiGatewaySpec{
				Port: 4000,
				AiModels: []gatewayv1alpha1.AiModel{
					{Name: "gpt-4", Provider: "openai"},
					{Name: "gemini-1.5-pro", Provider: "gemini"},
				},
			},
		}

		// Simulate reordered models (should not trigger update)
		reorderedAiGateway := &gatewayv1alpha1.AiGateway{
			Spec: gatewayv1alpha1.AiGatewaySpec{
				Port: 4000,
				AiModels: []gatewayv1alpha1.AiModel{
					{Name: "gemini-1.5-pro", Provider: "gemini"},
					{Name: "gpt-4", Provider: "openai"},
				},
			},
		}

		// Should recognize these as equal (no update needed)
		if !equality.AiModelsEqual(originalAiGateway.Spec.AiModels, reorderedAiGateway.Spec.AiModels) {
			t.Error("Controller should recognize reordered models as equal")
		}

		// Simulate actual model change (should trigger update)
		changedAiGateway := &gatewayv1alpha1.AiGateway{
			Spec: gatewayv1alpha1.AiGatewaySpec{
				Port: 4000,
				AiModels: []gatewayv1alpha1.AiModel{
					{Name: "gpt-4", Provider: "openai"},
					{Name: "claude-3-opus", Provider: "anthropic"}, // Different model
				},
			},
		}

		// Should recognize these as different (update needed)
		if equality.AiModelsEqual(originalAiGateway.Spec.AiModels, changedAiGateway.Spec.AiModels) {
			t.Error("Controller should recognize different models as not equal")
		}
	})
}
