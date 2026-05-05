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
	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
)

// Annotation keys written by ReconcileWorkload onto the pod template so a
// ConfigMap or Secret data change triggers a rolling restart.
const (
	configHashAnnotation = "gateway.agentic-layer.ai/config-hash"
	secretHashAnnotation = "gateway.agentic-layer.ai/secret-hash"
)

// BuildResourceLabels builds labels for Deployment/Service ObjectMeta:
// commonMetadata.Labels first, then the managed selector label ("app") which
// always wins.
func BuildResourceLabels(name string, common *gatewayv1alpha1.EmbeddedMetadata) map[string]string {
	labels := make(map[string]string)
	if common != nil {
		for k, v := range common.Labels {
			labels[k] = v
		}
	}
	labels["app"] = name
	return labels
}

// BuildResourceAnnotations builds annotations for Deployment/Service ObjectMeta from
// commonMetadata. Returns nil when nothing is configured.
func BuildResourceAnnotations(common *gatewayv1alpha1.EmbeddedMetadata) map[string]string {
	if common == nil || len(common.Annotations) == 0 {
		return nil
	}
	annotations := make(map[string]string)
	for k, v := range common.Annotations {
		annotations[k] = v
	}
	return annotations
}

// BuildPodTemplateLabels builds pod template labels: common.Labels + pod.Labels
// (pod overrides), then the selector label ("app") which always wins.
func BuildPodTemplateLabels(name string, common, pod *gatewayv1alpha1.EmbeddedMetadata) map[string]string {
	labels := make(map[string]string)
	if common != nil {
		for k, v := range common.Labels {
			labels[k] = v
		}
	}
	if pod != nil {
		for k, v := range pod.Labels {
			labels[k] = v
		}
	}
	labels["app"] = name
	return labels
}

// BuildPodTemplateAnnotations builds pod template annotations: common.Annotations +
// pod.Annotations (pod overrides), then the operator-managed config-hash and
// secret-hash annotations which always win.
func BuildPodTemplateAnnotations(common, pod *gatewayv1alpha1.EmbeddedMetadata, configHash, secretHash string) map[string]string {
	annotations := make(map[string]string)
	if common != nil {
		for k, v := range common.Annotations {
			annotations[k] = v
		}
	}
	if pod != nil {
		for k, v := range pod.Annotations {
			annotations[k] = v
		}
	}
	annotations[configHashAnnotation] = configHash
	annotations[secretHashAnnotation] = secretHash
	return annotations
}
