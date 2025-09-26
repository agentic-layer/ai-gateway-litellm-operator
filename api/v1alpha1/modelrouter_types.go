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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ModelRouterSpec defines the desired state of ModelRouter.
type ModelRouterSpec struct {
	// NOTE: In the future, this will be a ModelRouterClass reference (similar to the AgentGatewayClass) instead.
	Type string `json:"type"`

	// Port on which the model router will be exposed.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=4000
	Port int32 `json:"port,omitempty"`

	// List of AI models to be made available through the router.
	AiModels []AiModel `json:"aiModels,omitempty"`
}

type AiModel struct {
	// Each model must specify a name in the format `provider/model-name`.
	// See https://docs.litellm.ai/docs/providers for a list of supported providers.
	Name string `json:"name"`
}

// ModelRouterStatus defines the observed state of ModelRouter.
type ModelRouterStatus struct {
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ModelRouter is the Schema for the modelrouters API.
type ModelRouter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ModelRouterSpec   `json:"spec,omitempty"`
	Status ModelRouterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ModelRouterList contains a list of ModelRouter.
type ModelRouterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ModelRouter `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ModelRouter{}, &ModelRouterList{})
}
