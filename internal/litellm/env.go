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
	"sort"

	corev1 "k8s.io/api/core/v1"
)

// MergeEnv produces the sorted, prometheus-augmented env list for the LiteLLM container.
// `caller` is the caller's already-merged env (user spec.env plus any CRD-specific
// generated entries such as API-key references). MergeEnv:
//   - dedupes by name (last-wins),
//   - appends PROMETHEUS_MULTIPROC_DIR if absent,
//   - returns a slice sorted by name for deterministic ordering.
func MergeEnv(caller []corev1.EnvVar) []corev1.EnvVar {
	envMap := make(map[string]corev1.EnvVar, len(caller)+1)
	for _, e := range caller {
		envMap[e.Name] = e
	}
	if _, ok := envMap["PROMETHEUS_MULTIPROC_DIR"]; !ok {
		envMap["PROMETHEUS_MULTIPROC_DIR"] = corev1.EnvVar{
			Name:  "PROMETHEUS_MULTIPROC_DIR",
			Value: PrometheusMultiprocDir,
		}
	}

	out := make([]corev1.EnvVar, 0, len(envMap))
	for _, e := range envMap {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
