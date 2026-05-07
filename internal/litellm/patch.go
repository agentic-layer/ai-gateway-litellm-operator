/*
Copyright 2026 Agentic Layer.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package litellm

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConfigPatchAnnotation names the gateway annotation pointing at the
// same-namespace ConfigMap whose patch.yaml is layered onto the
// operator-generated LiteLLM config.
const ConfigPatchAnnotation = "ai-gateway-litellm.agentic-layer.ai/config-patch"

// ApplyPatch deep-merges patch onto base using RFC 7396 semantics:
//   - Maps merge recursively, key by key.
//   - Scalars and lists in the patch fully replace the value at that path.
//   - A nil value in the patch deletes the key from the result.
//
// ApplyPatch returns a new top-level map and never mutates either input.
// The returned map may share inner sub-tree pointers with base for keys the
// patch did not touch — callers must treat the result as read-only after the
// call (the operator only marshals it to YAML, which never mutates).
func ApplyPatch(base, patch map[string]any) map[string]any {
	out := make(map[string]any, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, pv := range patch {
		if pv == nil {
			delete(out, k)
			continue
		}
		if pm, ok := pv.(map[string]any); ok {
			if bm, ok := out[k].(map[string]any); ok {
				out[k] = ApplyPatch(bm, pm)
				continue
			}
		}
		out[k] = pv
	}
	return out
}

// PatchYAMLKey is the ConfigMap data key whose value is parsed as the patch.
const PatchYAMLKey = "patch.yaml"

// LoadPatch fetches the ConfigMap named cmName in namespace ns and parses its
// "patch.yaml" entry into a generic map. When cmName is empty, returns
// (nil, nil) — the gateway has no patch annotation. Patches that parse to
// null or {} are also treated as no-ops and return (nil, nil).
//
// Errors are tagged with PhaseError{Phase: "ConfigPatch"} so callers can map
// them to a stable status reason.
func LoadPatch(ctx context.Context, c client.Client, ns, cmName string) (map[string]any, error) {
	if cmName == "" {
		return nil, nil
	}
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: ns, Name: cmName}, cm); err != nil {
		return nil, &PhaseError{Phase: "ConfigPatch", Err: fmt.Errorf("configmap %q: %w", cmName, err)}
	}
	body, ok := cm.Data[PatchYAMLKey]
	if !ok {
		return nil, &PhaseError{Phase: "ConfigPatch", Err: fmt.Errorf("key %q not found in configmap %q", PatchYAMLKey, cmName)}
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil {
		return nil, &PhaseError{Phase: "ConfigPatch", Err: fmt.Errorf("failed to parse %s: %w", PatchYAMLKey, err)}
	}
	if len(parsed) == 0 {
		return nil, nil
	}
	return parsed, nil
}
