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

	"github.com/agentic-layer/ai-gateway-litellm/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/equality"
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
			a:    []v1alpha1.AiModel{{Name: "openai/gpt-4"}},
			b:    []v1alpha1.AiModel{{Name: "openai/gpt-4"}},
			want: true,
		},
		{
			name: "should be equal for slices in different order",
			a: []v1alpha1.AiModel{
				{Name: "openai/gpt-4"},
				{Name: "gemini/gemini-1.5-pro"},
			},
			b: []v1alpha1.AiModel{
				{Name: "gemini/gemini-1.5-pro"},
				{Name: "openai/gpt-4"},
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
			a:    []v1alpha1.AiModel{{Name: "openai/gpt-4"}},
			b:    []v1alpha1.AiModel{{Name: "openai/gpt-4"}, {Name: "gemini/gemini-1.5-pro"}},
			want: false,
		},
		{
			name: "should not be equal for different names",
			a:    []v1alpha1.AiModel{{Name: "openai/gpt-4"}},
			b:    []v1alpha1.AiModel{{Name: "anthropic/claude-3-opus"}},
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