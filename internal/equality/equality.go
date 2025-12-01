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

package equality

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
)

// AiModelsEqual compares AI model slices for equality, ignoring order.
func AiModelsEqual(existing, desired []gatewayv1alpha1.AiModel) bool {
	sortFunc := func(a, b gatewayv1alpha1.AiModel) bool {
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.Name < b.Name
	}
	return cmp.Equal(existing, desired, cmpopts.SortSlices(sortFunc))
}

// LabelsEqual compares label maps for equality.
func LabelsEqual(existing, desired map[string]string) bool {
	return cmp.Equal(existing, desired)
}

// RequiredLabelsPresent checks if all required labels are present with correct values in existing labels.
// This allows other operators to add additional labels without causing updates.
func RequiredLabelsPresent(existing, required map[string]string) bool {
	for key, value := range required {
		if existingValue, exists := existing[key]; !exists || existingValue != value {
			return false
		}
	}
	return true
}

// EnvVarsEqual compares environment variable slices for equality, ignoring order.
func EnvVarsEqual(existing, desired []corev1.EnvVar) bool {
	sortFunc := func(a, b corev1.EnvVar) bool {
		return a.Name < b.Name
	}
	return cmp.Equal(existing, desired, cmpopts.SortSlices(sortFunc))
}

// EnvFromEqual compares environment variable source slices for equality, ignoring order.
func EnvFromEqual(existing, desired []corev1.EnvFromSource) bool {
	sortFunc := func(a, b corev1.EnvFromSource) bool {
		// Sort by ConfigMapRef name first, then SecretRef name
		aName := ""
		bName := ""
		if a.ConfigMapRef != nil {
			aName = "cm:" + a.ConfigMapRef.Name
		} else if a.SecretRef != nil {
			aName = "secret:" + a.SecretRef.Name
		}
		if b.ConfigMapRef != nil {
			bName = "cm:" + b.ConfigMapRef.Name
		} else if b.SecretRef != nil {
			bName = "secret:" + b.SecretRef.Name
		}
		return aName < bName
	}
	return cmp.Equal(existing, desired, cmpopts.SortSlices(sortFunc))
}
