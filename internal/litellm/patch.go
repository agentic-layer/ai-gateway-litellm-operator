/*
Copyright 2026 Agentic Layer.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package litellm

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
