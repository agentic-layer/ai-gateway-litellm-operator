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
	"errors"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestApplyPatch(t *testing.T) {
	type tc struct {
		name  string
		base  map[string]any
		patch map[string]any
		want  map[string]any
	}
	cases := []tc{
		{
			name:  "empty patch leaves base unchanged",
			base:  map[string]any{"a": 1, "b": "x"},
			patch: map[string]any{},
			want:  map[string]any{"a": 1, "b": "x"},
		},
		{
			name:  "empty base, patch becomes result",
			base:  map[string]any{},
			patch: map[string]any{"router_settings": map[string]any{"redis_host": "r"}},
			want:  map[string]any{"router_settings": map[string]any{"redis_host": "r"}},
		},
		{
			name:  "scalar override at top level",
			base:  map[string]any{"litellm_settings": map[string]any{"request_timeout": 600}},
			patch: map[string]any{"litellm_settings": map[string]any{"request_timeout": 1200}},
			want:  map[string]any{"litellm_settings": map[string]any{"request_timeout": 1200}},
		},
		{
			name: "nested map deep-merges",
			base: map[string]any{
				"mcp_servers": map[string]any{
					"echo": map[string]any{"url": "http://echo", "transport": "http"},
				},
			},
			patch: map[string]any{
				"mcp_servers": map[string]any{
					"echo": map[string]any{
						"auth_type": "oauth2",
						"headers":   map[string]any{"Authorization": "os.environ/TOKEN"},
					},
				},
			},
			want: map[string]any{
				"mcp_servers": map[string]any{
					"echo": map[string]any{
						"url":       "http://echo",
						"transport": "http",
						"auth_type": "oauth2",
						"headers":   map[string]any{"Authorization": "os.environ/TOKEN"},
					},
				},
			},
		},
		{
			name: "list at any path replaces wholesale",
			base: map[string]any{
				"litellm_settings": map[string]any{"callbacks": []any{"otel", "prometheus"}},
			},
			patch: map[string]any{
				"litellm_settings": map[string]any{"callbacks": []any{"langfuse"}},
			},
			want: map[string]any{
				"litellm_settings": map[string]any{"callbacks": []any{"langfuse"}},
			},
		},
		{
			name:  "null in patch deletes the key",
			base:  map[string]any{"litellm_settings": map[string]any{"request_timeout": 600, "callbacks": []any{"otel"}}},
			patch: map[string]any{"litellm_settings": map[string]any{"request_timeout": nil}},
			want:  map[string]any{"litellm_settings": map[string]any{"callbacks": []any{"otel"}}},
		},
		{
			name:  "type mismatch: scalar replaces a map",
			base:  map[string]any{"mcp_servers": map[string]any{"echo": map[string]any{"url": "http://echo"}}},
			patch: map[string]any{"mcp_servers": "broken"},
			want:  map[string]any{"mcp_servers": "broken"},
		},
		{
			name:  "type mismatch: map replaces a scalar",
			base:  map[string]any{"foo": "bar"},
			patch: map[string]any{"foo": map[string]any{"x": 1}},
			want:  map[string]any{"foo": map[string]any{"x": 1}},
		},
		{
			name:  "patch introduces unmodeled top-level key",
			base:  map[string]any{"model_list": []any{}},
			patch: map[string]any{"router_settings": map[string]any{"routing_strategy": "usage-based-routing-v2"}},
			want: map[string]any{
				"model_list":      []any{},
				"router_settings": map[string]any{"routing_strategy": "usage-based-routing-v2"},
			},
		},
		{
			name:  "null in patch deletes a top-level key",
			base:  map[string]any{"model_list": []any{}, "litellm_settings": map[string]any{"timeout": 30}},
			patch: map[string]any{"litellm_settings": nil},
			want:  map[string]any{"model_list": []any{}},
		},
		{
			name:  "null in patch for absent key is a no-op",
			base:  map[string]any{"a": 1},
			patch: map[string]any{"b": nil},
			want:  map[string]any{"a": 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ApplyPatch(c.base, c.patch)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ApplyPatch mismatch\n got:  %#v\n want: %#v", got, c.want)
			}
		})
	}
}

func TestApplyPatch_DoesNotMutateInputs(t *testing.T) {
	base := map[string]any{
		"litellm_settings": map[string]any{"request_timeout": 600},
		"mcp_servers":      map[string]any{"echo": map[string]any{"url": "http://echo"}},
	}
	patch := map[string]any{
		"litellm_settings": map[string]any{"request_timeout": 1200, "drop_params": true},
		"mcp_servers":      map[string]any{"echo": map[string]any{"auth_type": "oauth2"}},
	}
	baseSnapshot := deepCopyMap(base)
	patchSnapshot := deepCopyMap(patch)

	_ = ApplyPatch(base, patch)

	if !reflect.DeepEqual(base, baseSnapshot) {
		t.Errorf("ApplyPatch mutated base:\n got:  %#v\n want: %#v", base, baseSnapshot)
	}
	if !reflect.DeepEqual(patch, patchSnapshot) {
		t.Errorf("ApplyPatch mutated patch:\n got:  %#v\n want: %#v", patch, patchSnapshot)
	}
}

func deepCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if nested, ok := v.(map[string]any); ok {
			out[k] = deepCopyMap(nested)
			continue
		}
		out[k] = v
	}
	return out
}

func TestLoadPatch_EmptyAnnotationReturnsNil(t *testing.T) {
	c := newFakeClient(t)
	got, err := LoadPatch(context.Background(), c, "ns", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %#v", got)
	}
}

func TestLoadPatch_ConfigMapNotFound(t *testing.T) {
	c := newFakeClient(t)
	_, err := LoadPatch(context.Background(), c, "ns", "missing-cm")
	var pe *PhaseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected *PhaseError, got %T: %v", err, err)
	}
	if pe.Phase != "ConfigPatch" {
		t.Errorf("expected Phase=ConfigPatch, got %q", pe.Phase)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to mention 'not found', got: %v", err)
	}
}

func TestLoadPatch_MissingPatchYAMLKey(t *testing.T) {
	c := newFakeClient(t, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"},
		Data:       map[string]string{"other.yaml": "foo: bar"},
	})
	_, err := LoadPatch(context.Background(), c, "ns", "cm")
	var pe *PhaseError
	if !errors.As(err, &pe) || pe.Phase != "ConfigPatch" {
		t.Fatalf("expected *PhaseError{Phase:ConfigPatch}, got %v", err)
	}
	if !strings.Contains(err.Error(), "patch.yaml") {
		t.Errorf("expected error to mention 'patch.yaml', got: %v", err)
	}
}

func TestLoadPatch_InvalidYAML(t *testing.T) {
	c := newFakeClient(t, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"},
		Data:       map[string]string{"patch.yaml": "this: : is: not: yaml"},
	})
	_, err := LoadPatch(context.Background(), c, "ns", "cm")
	var pe *PhaseError
	if !errors.As(err, &pe) || pe.Phase != "ConfigPatch" {
		t.Fatalf("expected *PhaseError{Phase:ConfigPatch}, got %v", err)
	}
	if !strings.Contains(err.Error(), "patch.yaml") {
		t.Errorf("expected error to mention 'patch.yaml', got: %v", err)
	}
}

func TestLoadPatch_NullPatchIsNoop(t *testing.T) {
	c := newFakeClient(t, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"},
		Data:       map[string]string{"patch.yaml": "null"},
	})
	got, err := LoadPatch(context.Background(), c, "ns", "cm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for null patch, got %#v", got)
	}
}

func TestLoadPatch_EmptyMapPatchIsNoop(t *testing.T) {
	c := newFakeClient(t, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"},
		Data:       map[string]string{"patch.yaml": "{}"},
	})
	got, err := LoadPatch(context.Background(), c, "ns", "cm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty-map patch, got %#v", got)
	}
}

func TestLoadPatch_HappyPath(t *testing.T) {
	yamlBody := "router_settings:\n  routing_strategy: usage-based-routing-v2\n"
	c := newFakeClient(t, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "ns"},
		Data:       map[string]string{"patch.yaml": yamlBody},
	})
	got, err := LoadPatch(context.Background(), c, "ns", "cm")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rs, ok := got["router_settings"].(map[string]any)
	if !ok {
		t.Fatalf("expected router_settings map, got %#v", got)
	}
	if rs["routing_strategy"] != "usage-based-routing-v2" {
		t.Errorf("unexpected routing_strategy: %v", rs["routing_strategy"])
	}
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	s := runtime.NewScheme()
	if err := scheme.AddToScheme(s); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
}
