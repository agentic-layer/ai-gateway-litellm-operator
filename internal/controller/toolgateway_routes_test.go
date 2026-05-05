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

package controller

import (
	"context"
	"strings"
	"testing"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func upstreamScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := gatewayv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func TestResolveRouteUpstream_ToolServerRefHttpDefaultPath(t *testing.T) {
	ts := &gatewayv1alpha1.ToolServer{
		ObjectMeta: metav1.ObjectMeta{Name: "search", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolServerSpec{
			Protocol:      "mcp",
			TransportType: "http",
			Image:         "ignored",
			Port:          8080,
		},
	}
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).WithObjects(ts).Build()
	route := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				ToolServerRef: &corev1.ObjectReference{Name: "search"},
			},
		},
	}
	got, err := resolveRouteUpstream(context.Background(), c, route)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.url != "http://search.default.svc.cluster.local:8080/mcp" {
		t.Errorf("url: got %q", got.url)
	}
	if got.transport != "http" {
		t.Errorf("transport: got %q, want http", got.transport)
	}
}

func TestResolveRouteUpstream_ToolServerRefSseDefaultPath(t *testing.T) {
	ts := &gatewayv1alpha1.ToolServer{
		ObjectMeta: metav1.ObjectMeta{Name: "files", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolServerSpec{
			TransportType: "sse",
			Port:          9000,
		},
	}
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).WithObjects(ts).Build()
	route := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				ToolServerRef: &corev1.ObjectReference{Name: "files"},
			},
		},
	}
	got, err := resolveRouteUpstream(context.Background(), c, route)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.url != "http://files.default.svc.cluster.local:9000/sse" {
		t.Errorf("url: got %q", got.url)
	}
	if got.transport != "sse" {
		t.Errorf("transport: got %q, want sse", got.transport)
	}
}

func TestResolveRouteUpstream_ToolServerRefExplicitPath(t *testing.T) {
	ts := &gatewayv1alpha1.ToolServer{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns2"},
		Spec: gatewayv1alpha1.ToolServerSpec{
			TransportType: "http",
			Port:          80,
			Path:          "/custom",
		},
	}
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).WithObjects(ts).Build()
	route := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				ToolServerRef: &corev1.ObjectReference{Name: "x", Namespace: "ns2"},
			},
		},
	}
	got, err := resolveRouteUpstream(context.Background(), c, route)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.url != "http://x.ns2.svc.cluster.local:80/custom" {
		t.Errorf("url: got %q", got.url)
	}
}

func TestResolveRouteUpstream_ToolServerNotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).Build()
	route := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				ToolServerRef: &corev1.ObjectReference{Name: "missing"},
			},
		},
	}
	_, err := resolveRouteUpstream(context.Background(), c, route)
	if err == nil {
		t.Fatal("expected error for missing ToolServer")
	}
}

func TestResolveRouteUpstream_ExternalHttp(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).Build()
	route := &gatewayv1alpha1.ToolRoute{
		Spec: gatewayv1alpha1.ToolRouteSpec{
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				External: &gatewayv1alpha1.ExternalUpstream{Url: "https://example.com/mcp"},
			},
		},
	}
	got, err := resolveRouteUpstream(context.Background(), c, route)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.url != "https://example.com/mcp" {
		t.Errorf("url: got %q", got.url)
	}
	if got.transport != "http" {
		t.Errorf("transport: got %q, want http", got.transport)
	}
}

func TestResolveRouteUpstream_ExternalSseSuffix(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).Build()
	route := &gatewayv1alpha1.ToolRoute{
		Spec: gatewayv1alpha1.ToolRouteSpec{
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				External: &gatewayv1alpha1.ExternalUpstream{Url: "https://example.com/sse"},
			},
		},
	}
	got, err := resolveRouteUpstream(context.Background(), c, route)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.transport != "sse" {
		t.Errorf("transport: got %q, want sse", got.transport)
	}
}

func TestResolveRouteUpstream_ExternalInvalidUrl(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).Build()
	route := &gatewayv1alpha1.ToolRoute{
		Spec: gatewayv1alpha1.ToolRouteSpec{
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				External: &gatewayv1alpha1.ExternalUpstream{Url: "://broken"},
			},
		},
	}
	_, err := resolveRouteUpstream(context.Background(), c, route)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "external") {
		t.Errorf("error should mention external, got %v", err)
	}
}

func TestResolveRouteUpstream_RejectsUnknownTransport(t *testing.T) {
	ts := &gatewayv1alpha1.ToolServer{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "default"},
		Spec:       gatewayv1alpha1.ToolServerSpec{TransportType: "websocket", Port: 80},
	}
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).WithObjects(ts).Build()
	route := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				ToolServerRef: &corev1.ObjectReference{Name: "x"},
			},
		},
	}
	_, err := resolveRouteUpstream(context.Background(), c, route)
	if err == nil {
		t.Fatal("expected error for unknown transportType")
	}
	if !strings.Contains(err.Error(), "websocket") {
		t.Errorf("error should mention the bad transport, got %v", err)
	}
}

func TestResolveRouteUpstream_NeitherSet(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).Build()
	route := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
	}
	_, err := resolveRouteUpstream(context.Background(), c, route)
	if err == nil {
		t.Fatal("expected error when neither toolServerRef nor external is set")
	}
}

func TestMcpServerKey(t *testing.T) {
	r := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "search", Namespace: "default"},
	}
	if got := mcpServerKey(r); got != "default__search" {
		t.Errorf("got %q, want default__search", got)
	}
}

func TestMcpServerKey_DistinguishesHyphenBoundaries(t *testing.T) {
	// Regression for an earlier collapsing-separator scheme that mapped
	// (foo-bar, baz) and (foo, bar-baz) onto the same key.
	a := mcpServerKey(&gatewayv1alpha1.ToolRoute{ObjectMeta: metav1.ObjectMeta{Name: "baz", Namespace: "foo-bar"}})
	b := mcpServerKey(&gatewayv1alpha1.ToolRoute{ObjectMeta: metav1.ObjectMeta{Name: "bar-baz", Namespace: "foo"}})
	if a == b {
		t.Errorf("expected distinct keys; both = %q", a)
	}
}

func TestRouteAttachesToGateway_ExplicitMatch(t *testing.T) {
	gw := &gatewayv1alpha1.ToolGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns1"},
	}
	route := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "ns2"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: &corev1.ObjectReference{Name: "gw", Namespace: "ns1"},
		},
	}
	if !routeAttachesToGateway(route, gw) {
		t.Error("expected attach=true on explicit match")
	}
}

func TestRouteAttachesToGateway_ExplicitMismatch(t *testing.T) {
	gw := &gatewayv1alpha1.ToolGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "ns1"},
	}
	route := &gatewayv1alpha1.ToolRoute{
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: &corev1.ObjectReference{Name: "other", Namespace: "ns1"},
		},
	}
	if routeAttachesToGateway(route, gw) {
		t.Error("expected attach=false on explicit mismatch")
	}
}

func TestRouteAttachesToGateway_RefNamespaceDefaultsToRouteNs(t *testing.T) {
	gw := &gatewayv1alpha1.ToolGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
	}
	route := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: &corev1.ObjectReference{Name: "gw"}, // namespace empty
		},
	}
	if !routeAttachesToGateway(route, gw) {
		t.Error("expected attach=true; ref.Namespace empty should default to route.Namespace")
	}
}

func TestRouteAttachesToGateway_UnsetRefAttachesToNothing(t *testing.T) {
	// Two gateways in the same namespace would otherwise both claim a
	// ref-less route and race on its status — see routeAttachesToGateway.
	gw1 := &gatewayv1alpha1.ToolGateway{ObjectMeta: metav1.ObjectMeta{Name: "gw1", Namespace: "tool-gateway"}}
	gw2 := &gatewayv1alpha1.ToolGateway{ObjectMeta: metav1.ObjectMeta{Name: "gw2", Namespace: "tool-gateway"}}
	route := &gatewayv1alpha1.ToolRoute{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "anywhere"}}
	if routeAttachesToGateway(route, gw1) || routeAttachesToGateway(route, gw2) {
		t.Error("expected attach=false for ref-less route on any gateway")
	}
}

func TestBuildMcpServers_Success(t *testing.T) {
	gw := &gatewayv1alpha1.ToolGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "tool-gateway"},
	}
	ts := &gatewayv1alpha1.ToolServer{
		ObjectMeta: metav1.ObjectMeta{Name: "search", Namespace: "default"},
		Spec:       gatewayv1alpha1.ToolServerSpec{TransportType: "http", Port: 8080},
	}
	route := gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "r1", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: &corev1.ObjectReference{Name: "gw", Namespace: "tool-gateway"},
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				ToolServerRef: &corev1.ObjectReference{Name: "search"},
			},
			ToolFilter: &gatewayv1alpha1.ToolFilter{
				Allow: []string{"search", "fetch_*"},
				Deny:  []string{"delete_*"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).WithObjects(ts).Build()

	servers, outcomes := buildMcpServers(context.Background(), c, gw, []gatewayv1alpha1.ToolRoute{route})
	if len(servers) != 1 {
		t.Fatalf("want 1 server, got %d", len(servers))
	}
	got := servers["default__r1"]
	if got.Url != "http://search.default.svc.cluster.local:8080/mcp" {
		t.Errorf("url: %q", got.Url)
	}
	if got.Transport != "http" {
		t.Errorf("transport: %q", got.Transport)
	}
	if len(got.AllowedTools) != 2 || got.AllowedTools[0] != "search" || got.AllowedTools[1] != "fetch_*" {
		t.Errorf("AllowedTools: %v", got.AllowedTools)
	}
	if len(got.DisallowedTools) != 1 || got.DisallowedTools[0] != "delete_*" {
		t.Errorf("DisallowedTools: %v", got.DisallowedTools)
	}
	if len(outcomes) != 1 || !outcomes[0].included || outcomes[0].failureReason != "" {
		t.Errorf("expected single included outcome, got %+v", outcomes)
	}
}

func TestBuildMcpServers_PartialSuccessOnUpstreamFailure(t *testing.T) {
	gw := &gatewayv1alpha1.ToolGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "tool-gateway"},
	}
	ref := &corev1.ObjectReference{Name: "gw", Namespace: "tool-gateway"}
	ts := &gatewayv1alpha1.ToolServer{
		ObjectMeta: metav1.ObjectMeta{Name: "ok", Namespace: "default"},
		Spec:       gatewayv1alpha1.ToolServerSpec{TransportType: "http", Port: 8080},
	}
	good := gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "good", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: ref,
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				ToolServerRef: &corev1.ObjectReference{Name: "ok"},
			},
		},
	}
	bad := gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "bad", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: ref,
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				ToolServerRef: &corev1.ObjectReference{Name: "missing"},
			},
		},
	}
	c := fake.NewClientBuilder().WithScheme(upstreamScheme(t)).WithObjects(ts).Build()

	servers, outcomes := buildMcpServers(context.Background(), c, gw, []gatewayv1alpha1.ToolRoute{good, bad})
	if _, ok := servers["default__good"]; !ok {
		t.Errorf("good route missing from servers")
	}
	if _, ok := servers["default__bad"]; ok {
		t.Errorf("bad route should be omitted on upstream failure")
	}
	var goodOutcome, badOutcome *routeOutcome
	for i := range outcomes {
		switch outcomes[i].route.Name {
		case "good":
			goodOutcome = &outcomes[i]
		case "bad":
			badOutcome = &outcomes[i]
		}
	}
	if goodOutcome == nil || !goodOutcome.included {
		t.Errorf("good outcome: %+v", goodOutcome)
	}
	if badOutcome == nil || badOutcome.included || badOutcome.failureReason != reasonRouteUpstreamUnresolved {
		t.Errorf("bad outcome: %+v", badOutcome)
	}
}
