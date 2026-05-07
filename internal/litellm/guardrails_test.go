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
	"context"
	"reflect"
	"testing"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := gatewayv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1 AddToScheme: %v", err)
	}
	return s
}

func TestResolveGuardrails_NoRefs(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	got, err := ResolveGuardrails(context.Background(), c, "default", nil, GuardrailTargetLLM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestResolveGuardrails_PresidioGuard(t *testing.T) {
	guard := &gatewayv1alpha1.Guard{
		ObjectMeta: metav1.ObjectMeta{Name: "pii", Namespace: "default"},
		Spec: gatewayv1alpha1.GuardSpec{
			Mode:        []gatewayv1alpha1.GuardMode{"pre_call"},
			ProviderRef: corev1.ObjectReference{Name: "presidio"},
			Presidio: &gatewayv1alpha1.PresidioGuardConfig{
				Language:        "en",
				ScoreThresholds: map[string]string{"PERSON": "0.5"},
				EntityActions:   map[string]string{"EMAIL_ADDRESS": "MASK"},
			},
		},
	}
	provider := &gatewayv1alpha1.GuardrailProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "presidio", Namespace: "default"},
		Spec: gatewayv1alpha1.GuardrailProviderSpec{
			Type: "presidio-api",
			Presidio: &gatewayv1alpha1.PresidioProviderConfig{
				BaseUrl: "http://presidio.svc:5002",
			},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(guard, provider).
		Build()

	got, err := ResolveGuardrails(context.Background(), c, "default", []corev1.ObjectReference{{Name: "pii"}}, GuardrailTargetLLM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 guardrail, got %d", len(got))
	}
	g := got[0]
	if g.GuardrailName != "pii" {
		t.Errorf("GuardrailName: want pii, got %q", g.GuardrailName)
	}
	if g.LiteLLMParams.Guardrail != "presidio" {
		t.Errorf("Guardrail: want presidio, got %q", g.LiteLLMParams.Guardrail)
	}
	if len(g.LiteLLMParams.Mode) != 1 || g.LiteLLMParams.Mode[0] != "pre_call" {
		t.Errorf("Mode: want [pre_call], got %v", g.LiteLLMParams.Mode)
	}
	if g.LiteLLMParams.PresidioAnalyzerApiBase != "http://presidio.svc:5002" {
		t.Errorf("PresidioAnalyzerApiBase: want %q, got %q", "http://presidio.svc:5002", g.LiteLLMParams.PresidioAnalyzerApiBase)
	}
	if !g.LiteLLMParams.OutputParsePii {
		t.Errorf("OutputParsePii: want true")
	}
	if g.LiteLLMParams.PresidioLanguage != "en" {
		t.Errorf("PresidioLanguage: want en, got %q", g.LiteLLMParams.PresidioLanguage)
	}
}

func TestResolveGuardrails_MissingGuard(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	_, err := ResolveGuardrails(context.Background(), c, "default", []corev1.ObjectReference{{Name: "ghost"}}, GuardrailTargetLLM)
	if err == nil {
		t.Errorf("expected error for missing Guard")
	}
}

func TestResolveGuardrails_UnsupportedProviderTypeSkipped(t *testing.T) {
	guard := &gatewayv1alpha1.Guard{
		ObjectMeta: metav1.ObjectMeta{Name: "g", Namespace: "default"},
		Spec: gatewayv1alpha1.GuardSpec{
			Mode:        []gatewayv1alpha1.GuardMode{"pre_call"},
			ProviderRef: corev1.ObjectReference{Name: "p"},
		},
	}
	provider := &gatewayv1alpha1.GuardrailProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "default"},
		Spec:       gatewayv1alpha1.GuardrailProviderSpec{Type: "unknown-type"},
	}
	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(guard, provider).
		Build()

	got, err := ResolveGuardrails(context.Background(), c, "default", []corev1.ObjectReference{{Name: "g"}}, GuardrailTargetLLM)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected unsupported provider type to be skipped, got %d guardrails", len(got))
	}
}

func TestResolveGuardrails_MissingProvider(t *testing.T) {
	guard := &gatewayv1alpha1.Guard{
		ObjectMeta: metav1.ObjectMeta{Name: "g", Namespace: "default"},
		Spec: gatewayv1alpha1.GuardSpec{
			Mode:        []gatewayv1alpha1.GuardMode{"pre_call"},
			ProviderRef: corev1.ObjectReference{Name: "ghost-provider"},
		},
	}
	// Guard exists, but no provider is registered with that name.
	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(guard).
		Build()

	_, err := ResolveGuardrails(context.Background(), c, "default", []corev1.ObjectReference{{Name: "g"}}, GuardrailTargetLLM)
	if err == nil {
		t.Errorf("expected error for missing GuardrailProvider")
	}
}

func TestResolveGuardrails_MCPMapsModes(t *testing.T) {
	guard := &gatewayv1alpha1.Guard{
		ObjectMeta: metav1.ObjectMeta{Name: "pii", Namespace: "default"},
		Spec: gatewayv1alpha1.GuardSpec{
			Mode:        []gatewayv1alpha1.GuardMode{"pre_call", "during_call", "post_call"},
			ProviderRef: corev1.ObjectReference{Name: "presidio"},
		},
	}
	provider := &gatewayv1alpha1.GuardrailProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "presidio", Namespace: "default"},
		Spec: gatewayv1alpha1.GuardrailProviderSpec{
			Type: "presidio-api",
			Presidio: &gatewayv1alpha1.PresidioProviderConfig{
				BaseUrl: "http://presidio.svc:5002",
			},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(guard, provider).
		Build()

	got, err := ResolveGuardrails(context.Background(), c, "default", []corev1.ObjectReference{{Name: "pii"}}, GuardrailTargetMCP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 guardrail, got %d", len(got))
	}
	want := []string{"pre_mcp_call", "during_mcp_call"}
	if !reflect.DeepEqual(got[0].LiteLLMParams.Mode, want) {
		t.Errorf("Mode: want %v, got %v", want, got[0].LiteLLMParams.Mode)
	}
}

func TestResolveGuardrails_MCPSkipsGuardWithOnlyPostCall(t *testing.T) {
	// post_call has no MCP equivalent in LiteLLM. A Guard whose modes all map
	// to nothing for the MCP target should be dropped entirely so we do not
	// emit a malformed empty-mode entry into config.yaml.
	guard := &gatewayv1alpha1.Guard{
		ObjectMeta: metav1.ObjectMeta{Name: "audit", Namespace: "default"},
		Spec: gatewayv1alpha1.GuardSpec{
			Mode:        []gatewayv1alpha1.GuardMode{"post_call"},
			ProviderRef: corev1.ObjectReference{Name: "presidio"},
		},
	}
	provider := &gatewayv1alpha1.GuardrailProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "presidio", Namespace: "default"},
		Spec: gatewayv1alpha1.GuardrailProviderSpec{
			Type:     "presidio-api",
			Presidio: &gatewayv1alpha1.PresidioProviderConfig{BaseUrl: "http://presidio.svc:5002"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(guard, provider).
		Build()

	got, err := ResolveGuardrails(context.Background(), c, "default", []corev1.ObjectReference{{Name: "audit"}}, GuardrailTargetMCP)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected guard with only post_call to be skipped for MCP target, got %d guardrails", len(got))
	}
}
