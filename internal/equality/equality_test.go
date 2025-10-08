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

package equality_test

import (
	"testing"

	"github.com/agentic-layer/ai-gateway-litellm/internal/equality"
	"github.com/agentic-layer/ai-gateway-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

func TestAiModelsEqual(t *testing.T) {
	testCases := []struct {
		name string
		a    []v1alpha1.AiModel
		b    []v1alpha1.AiModel
		want bool
	}{
		{
			name: "should be equal for identical slices",
			a:    []v1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}},
			b:    []v1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}},
			want: true,
		},
		{
			name: "should be equal for slices in different order",
			a: []v1alpha1.AiModel{
				{Name: "gpt-4", Provider: "openai"},
				{Name: "gemini-1.5-pro", Provider: "gemini"},
			},
			b: []v1alpha1.AiModel{
				{Name: "gemini-1.5-pro", Provider: "gemini"},
				{Name: "gpt-4", Provider: "openai"},
			},
			want: true,
		},
		{
			name: "should be equal for empty slices",
			a:    []v1alpha1.AiModel{},
			b:    []v1alpha1.AiModel{},
			want: true,
		},
		{
			name: "should be equal for nil slices",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "should not be equal for different lengths",
			a:    []v1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}},
			b:    []v1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}, {Name: "gemini-1.5-pro", Provider: "gemini"}},
			want: false,
		},
		{
			name: "should not be equal for different names",
			a:    []v1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}},
			b:    []v1alpha1.AiModel{{Name: "claude-3-opus", Provider: "anthropic"}},
			want: false,
		},
		{
			name: "should not be equal for different providers",
			a:    []v1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}},
			b:    []v1alpha1.AiModel{{Name: "gpt-4", Provider: "azure"}},
			want: false,
		},
		{
			name: "should not be equal for nil vs empty slice",
			a:    nil,
			b:    []v1alpha1.AiModel{},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := equality.AiModelsEqual(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("AiModelsEqual() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLabelsEqual(t *testing.T) {
	testCases := []struct {
		name string
		a    map[string]string
		b    map[string]string
		want bool
	}{
		{
			name: "should be equal for identical maps",
			a:    map[string]string{"app": "test", "type": "litellm"},
			b:    map[string]string{"app": "test", "type": "litellm"},
			want: true,
		},
		{
			name: "should be equal for empty maps",
			a:    map[string]string{},
			b:    map[string]string{},
			want: true,
		},
		{
			name: "should be equal for nil maps",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "should not be equal for different values",
			a:    map[string]string{"app": "test"},
			b:    map[string]string{"app": "different"},
			want: false,
		},
		{
			name: "should not be equal for different keys",
			a:    map[string]string{"app": "test"},
			b:    map[string]string{"name": "test"},
			want: false,
		},
		{
			name: "should not be equal for nil vs empty map",
			a:    nil,
			b:    map[string]string{},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := equality.LabelsEqual(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("LabelsEqual() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEnvVarsEqual(t *testing.T) {
	testCases := []struct {
		name string
		a    []corev1.EnvVar
		b    []corev1.EnvVar
		want bool
	}{
		{
			name: "should be equal for identical slices",
			a:    []corev1.EnvVar{{Name: "LITELLM_LOG", Value: "INFO"}},
			b:    []corev1.EnvVar{{Name: "LITELLM_LOG", Value: "INFO"}},
			want: true,
		},
		{
			name: "should be equal for slices in different order",
			a: []corev1.EnvVar{
				{Name: "LITELLM_LOG", Value: "INFO"},
				{Name: "OPENAI_API_KEY", Value: "secret"},
			},
			b: []corev1.EnvVar{
				{Name: "OPENAI_API_KEY", Value: "secret"},
				{Name: "LITELLM_LOG", Value: "INFO"},
			},
			want: true,
		},
		{
			name: "should be equal for empty slices",
			a:    []corev1.EnvVar{},
			b:    []corev1.EnvVar{},
			want: true,
		},
		{
			name: "should be equal for nil slices",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "should not be equal for different values",
			a:    []corev1.EnvVar{{Name: "LITELLM_LOG", Value: "INFO"}},
			b:    []corev1.EnvVar{{Name: "LITELLM_LOG", Value: "DEBUG"}},
			want: false,
		},
		{
			name: "should not be equal for different names",
			a:    []corev1.EnvVar{{Name: "LITELLM_LOG", Value: "INFO"}},
			b:    []corev1.EnvVar{{Name: "DIFFERENT_VAR", Value: "INFO"}},
			want: false,
		},
		{
			name: "should not be equal for nil vs empty slice",
			a:    nil,
			b:    []corev1.EnvVar{},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := equality.EnvVarsEqual(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("EnvVarsEqual() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRequiredLabelsPresent(t *testing.T) {
	testCases := []struct {
		name     string
		existing map[string]string
		required map[string]string
		want     bool
	}{
		{
			name:     "should return true when all required labels are present with correct values",
			existing: map[string]string{"app": "test", "version": "1.0", "other": "value"},
			required: map[string]string{"app": "test", "version": "1.0"},
			want:     true,
		},
		{
			name:     "should return true when required labels are subset of existing",
			existing: map[string]string{"app": "test", "managed-by": "other-operator", "version": "1.0"},
			required: map[string]string{"app": "test"},
			want:     true,
		},
		{
			name:     "should return true when maps are identical",
			existing: map[string]string{"app": "test", "type": "service"},
			required: map[string]string{"app": "test", "type": "service"},
			want:     true,
		},
		{
			name:     "should return true when required is empty",
			existing: map[string]string{"app": "test", "version": "1.0"},
			required: map[string]string{},
			want:     true,
		},
		{
			name:     "should return true when both are empty",
			existing: map[string]string{},
			required: map[string]string{},
			want:     true,
		},
		{
			name:     "should return true when both are nil",
			existing: nil,
			required: nil,
			want:     true,
		},
		{
			name:     "should return false when required label is missing",
			existing: map[string]string{"version": "1.0"},
			required: map[string]string{"app": "test"},
			want:     false,
		},
		{
			name:     "should return false when required label has wrong value",
			existing: map[string]string{"app": "wrong-value", "version": "1.0"},
			required: map[string]string{"app": "test"},
			want:     false,
		},
		{
			name:     "should return false when existing is nil but required is not empty",
			existing: nil,
			required: map[string]string{"app": "test"},
			want:     false,
		},
		{
			name:     "should return false when existing is empty but required is not",
			existing: map[string]string{},
			required: map[string]string{"app": "test"},
			want:     false,
		},
		{
			name:     "should return false when some required labels are missing",
			existing: map[string]string{"app": "test"},
			required: map[string]string{"app": "test", "version": "1.0"},
			want:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := equality.RequiredLabelsPresent(tc.existing, tc.required)
			if got != tc.want {
				t.Errorf("RequiredLabelsPresent() = %v, want %v", got, tc.want)
			}
		})
	}
}
