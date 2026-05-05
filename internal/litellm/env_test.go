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

	corev1 "k8s.io/api/core/v1"
)

func TestMergeEnv_AppendsPrometheusMultiproc(t *testing.T) {
	got := MergeEnv(nil)
	if len(got) != 1 {
		t.Fatalf("want 1 env var, got %d", len(got))
	}
	if got[0].Name != "PROMETHEUS_MULTIPROC_DIR" {
		t.Errorf("missing PROMETHEUS_MULTIPROC_DIR")
	}
	if got[0].Value != PrometheusMultiprocDir {
		t.Errorf("value: want %q, got %q", PrometheusMultiprocDir, got[0].Value)
	}
}

func TestMergeEnv_UserCanOverridePrometheusMultiproc(t *testing.T) {
	caller := []corev1.EnvVar{{Name: "PROMETHEUS_MULTIPROC_DIR", Value: "/custom"}}
	got := MergeEnv(caller)
	if got[0].Value != "/custom" {
		t.Errorf("user override lost: %v", got)
	}
}

func TestMergeEnv_DeterministicSortByName(t *testing.T) {
	caller := []corev1.EnvVar{
		{Name: "ZED", Value: "z"},
		{Name: "ALPHA", Value: "a"},
		{Name: "MIDDLE", Value: "m"},
	}
	got := MergeEnv(caller)
	want := []string{"ALPHA", "MIDDLE", "PROMETHEUS_MULTIPROC_DIR", "ZED"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(want))
	}
	for i, e := range got {
		if e.Name != want[i] {
			t.Errorf("position %d: got %q, want %q", i, e.Name, want[i])
		}
	}
}

func TestMergeEnv_LastWriteWinsOnConflict(t *testing.T) {
	caller := []corev1.EnvVar{
		{Name: "FOO", Value: "first"},
		{Name: "FOO", Value: "second"},
	}
	got := MergeEnv(caller)
	for _, e := range got {
		if e.Name == "FOO" && e.Value != "second" {
			t.Errorf("want last-wins value 'second', got %q", e.Value)
		}
	}
}
