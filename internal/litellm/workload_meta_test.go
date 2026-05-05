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
	"testing"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
)

func TestBuildResourceLabels_NilCommonMetadata(t *testing.T) {
	got := BuildResourceLabels("gw", nil)
	if got["app"] != "gw" {
		t.Errorf("missing app=gw selector label: %v", got)
	}
	if len(got) != 1 {
		t.Errorf("unexpected extra labels: %v", got)
	}
}

func TestBuildResourceLabels_AppAlwaysWins(t *testing.T) {
	common := &gatewayv1alpha1.EmbeddedMetadata{
		Labels: map[string]string{"app": "user-supplied", "team": "core"},
	}
	got := BuildResourceLabels("gw", common)
	if got["app"] != "gw" {
		t.Errorf("app label must equal name; got %v", got)
	}
	if got["team"] != "core" {
		t.Errorf("user-supplied label dropped: %v", got)
	}
}

func TestBuildResourceAnnotations_NilReturnsNil(t *testing.T) {
	if got := BuildResourceAnnotations(nil); got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestBuildPodTemplateLabels_PodOverridesCommonExceptApp(t *testing.T) {
	common := &gatewayv1alpha1.EmbeddedMetadata{
		Labels: map[string]string{"app": "ignored", "team": "platform", "env": "dev"},
	}
	pod := &gatewayv1alpha1.EmbeddedMetadata{
		Labels: map[string]string{"team": "edge", "extra": "yes"},
	}
	got := BuildPodTemplateLabels("gw", common, pod)
	if got["app"] != "gw" {
		t.Errorf("app must be selector name")
	}
	if got["team"] != "edge" {
		t.Errorf("pod must override common; got %v", got)
	}
	if got["env"] != "dev" {
		t.Errorf("common label kept when pod doesn't override: %v", got)
	}
	if got["extra"] != "yes" {
		t.Errorf("pod-only label missing")
	}
}

func TestBuildPodTemplateAnnotations_HashesAlwaysWin(t *testing.T) {
	common := &gatewayv1alpha1.EmbeddedMetadata{
		Annotations: map[string]string{"gateway.agentic-layer.ai/config-hash": "user", "owner": "alice"},
	}
	got := BuildPodTemplateAnnotations(common, nil, "real-hash", "secret-hash")
	if got["gateway.agentic-layer.ai/config-hash"] != "real-hash" {
		t.Errorf("user-supplied config-hash must be overridden")
	}
	if got["gateway.agentic-layer.ai/secret-hash"] != "secret-hash" {
		t.Errorf("missing secret-hash")
	}
	if got["owner"] != "alice" {
		t.Errorf("user-supplied annotation dropped")
	}
}
