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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ModelRouterSpec defines the desired state of ModelRouter.
type ModelRouterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Type     string    `json:"type"`
	Port     int32     `json:"port,omitempty"`
	AiModels []AiModel `json:"aiModels,omitempty"`
}

type AiModel struct {
	Name string `json:"name"`
}

// ModelRouterStatus defines the observed state of ModelRouter.
type ModelRouterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ConfigHash represents the hash of the current configuration
	ConfigHash string `json:"configHash,omitempty"`

	// LastUpdated is the timestamp when the ModelRouter was last updated
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

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
