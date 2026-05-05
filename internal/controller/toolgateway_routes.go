/*
Copyright 2026 Agentic Layer.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/litellm"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	transportHTTP = "http"
	transportSSE  = "sse"
)

// upstreamResolution captures the URL and transport derived from a ToolRoute's
// upstream spec. Consumed by the mcp_servers renderer in the same package.
type upstreamResolution struct {
	url       string
	transport string
}

// resolveRouteUpstream maps a ToolRoute's upstream to a concrete URL + transport.
//
// ToolServerRef:
//
//	url       = http://<name>.<ns>.svc.cluster.local:<port><path>
//	transport = ToolServer.spec.transportType (must be "http" or "sse")
//	path      defaults to "/mcp" for http, "/sse" for sse
//
// External:
//
//	url       = external.url verbatim
//	transport = "http" by default, "sse" when the URL path ends in "/sse"
//
// Returns an error when the ToolServer does not exist, the URL fails to parse,
// the transport type is unrecognized, or neither upstream variant is set. The
// caller is responsible for translating these errors into a ToolRoute
// Ready=False/UpstreamUnresolved status.
func resolveRouteUpstream(ctx context.Context, c client.Reader, route *gatewayv1alpha1.ToolRoute) (upstreamResolution, error) {
	switch {
	case route.Spec.Upstream.ToolServerRef != nil:
		ref := route.Spec.Upstream.ToolServerRef
		ns := ref.Namespace
		if ns == "" {
			ns = route.Namespace
		}
		var ts gatewayv1alpha1.ToolServer
		if err := c.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: ns}, &ts); err != nil {
			return upstreamResolution{}, fmt.Errorf("failed to resolve ToolServer %s/%s: %w", ns, ref.Name, err)
		}
		transport, defaultPath, err := normalizeTransport(ts.Spec.TransportType)
		if err != nil {
			return upstreamResolution{}, fmt.Errorf("toolserver %s/%s: %w", ns, ref.Name, err)
		}
		path := ts.Spec.Path
		if path == "" {
			path = defaultPath
		}
		return upstreamResolution{
			url:       fmt.Sprintf("http://%s.%s.svc.cluster.local:%d%s", ts.Name, ts.Namespace, ts.Spec.Port, path),
			transport: transport,
		}, nil

	case route.Spec.Upstream.External != nil:
		raw := route.Spec.Upstream.External.Url
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return upstreamResolution{}, fmt.Errorf("invalid external.url %q: %w", raw, err)
		}
		transport := transportHTTP
		if strings.HasSuffix(u.Path, "/sse") {
			transport = transportSSE
		}
		return upstreamResolution{url: raw, transport: transport}, nil

	default:
		return upstreamResolution{}, fmt.Errorf("toolroute %s/%s has neither toolServerRef nor external set", route.Namespace, route.Name)
	}
}

// normalizeTransport maps a ToolServer.spec.transportType to the LiteLLM
// transport identifier and the default MCP path. Empty defaults to "http".
// Unknown values return an error rather than silently defaulting, so an upstream
// CRD typo surfaces as a clear status reason.
func normalizeTransport(t string) (transport, defaultPath string, err error) {
	switch t {
	case "", transportHTTP:
		return transportHTTP, "/mcp", nil
	case transportSSE:
		return transportSSE, "/sse", nil
	default:
		return "", "", fmt.Errorf("unsupported transportType %q (want http or sse)", t)
	}
}

// ToolRoute Ready-condition reasons. Kept package-private; the gateway reconciler
// is the only writer of ToolRoute status.
const (
	reasonRouteReconciled         = "Reconciled"
	reasonRouteUpstreamUnresolved = "UpstreamUnresolved"
	reasonRouteDuplicateName      = "DuplicateServerName"
	reasonRouteGatewayDegraded    = "GatewayDegraded"
)

// routeOutcome records the result of attempting to add one ToolRoute to a
// gateway's mcp_servers map. The reconciler uses outcomes to patch each route's
// status after rendering succeeds.
type routeOutcome struct {
	route         gatewayv1alpha1.ToolRoute
	included      bool
	failureReason string
	failureMsg    string
}

// mcpServerKey returns the key used under mcp_servers for a given route. The
// scheme — "<namespace>__<name>" with a double-underscore separator — also
// forms the gateway-side path (/mcp/<key>) and is therefore part of the
// public contract. Single underscores survive (replacing hyphens because MCP
// server names reject "-"); the doubled separator keeps "<ns>-<name>" and
// "<ns>_<name>" pairs from colliding on a single underscore boundary.
func mcpServerKey(route *gatewayv1alpha1.ToolRoute) string {
	ns := strings.ReplaceAll(route.Namespace, "-", "_")
	name := strings.ReplaceAll(route.Name, "-", "_")
	return ns + "__" + name
}

// routeAttachesToGateway reports whether route should be reconciled into gw's
// mcp_servers block. A route attaches iff its ToolGatewayRef matches gw by
// name and namespace (ref.Namespace defaulting to the route's own namespace
// when empty). Routes without a ref attach to no gateway — there is no
// implicit fallback, because multiple gateways in the same namespace would
// otherwise race on the route's status.
//
// Routes whose ref points to another gateway we own — or one belonging to
// another controller — are skipped silently here. Class ownership of gw itself
// is checked once in the Reconcile entry point, so we do not re-check it.
func routeAttachesToGateway(route *gatewayv1alpha1.ToolRoute, gw *gatewayv1alpha1.ToolGateway) bool {
	ref := route.Spec.ToolGatewayRef
	if ref == nil {
		return false
	}
	ns := ref.Namespace
	if ns == "" {
		ns = route.Namespace
	}
	return ref.Name == gw.Name && ns == gw.Namespace
}

// applyRouteOutcome patches the status of a single ToolRoute according to the
// outcome recorded during buildMcpServers. Successful outcomes set Ready=True
// and the public URL; failures set Ready=False with a phase-specific reason
// and clear Url so consumers cannot keep trusting a route that is no longer
// being served. The route is re-fetched immediately before patching so the
// merge base reflects the latest cluster state, not the list snapshot.
func applyRouteOutcome(ctx context.Context, c client.Client, gw *gatewayv1alpha1.ToolGateway, outcome routeOutcome) error {
	var fresh gatewayv1alpha1.ToolRoute
	key := types.NamespacedName{Name: outcome.route.Name, Namespace: outcome.route.Namespace}
	if err := c.Get(ctx, key, &fresh); err != nil {
		return err
	}
	original := fresh.DeepCopy()

	cond := metav1.Condition{
		Type:               "Ready",
		ObservedGeneration: fresh.Generation,
	}

	if outcome.included {
		routeUrl := fmt.Sprintf("http://%s.%s.svc.cluster.local/mcp/%s",
			gw.Name, gw.Namespace, mcpServerKey(&fresh))
		fresh.Status.Url = routeUrl
		cond.Status = metav1.ConditionTrue
		cond.Reason = reasonRouteReconciled
		cond.Message = fmt.Sprintf("ToolRoute is ready at %s", routeUrl)
	} else {
		fresh.Status.Url = ""
		cond.Status = metav1.ConditionFalse
		cond.Reason = outcome.failureReason
		cond.Message = outcome.failureMsg
	}

	apimeta.SetStatusCondition(&fresh.Status.Conditions, cond)
	return c.Status().Patch(ctx, &fresh, client.MergeFrom(original))
}

// buildMcpServers materialises the mcp_servers map for a single ToolGateway.
// Routes whose upstream cannot be resolved are omitted (partial-success) and
// their outcome is recorded with reasonRouteUpstreamUnresolved so the caller
// can patch ToolRoute.status. Duplicate keys cannot be produced by valid input
// (each (ns,name) is unique), but we defensively handle them with
// reasonRouteDuplicateName.
func buildMcpServers(ctx context.Context, c client.Reader, gw *gatewayv1alpha1.ToolGateway, routes []gatewayv1alpha1.ToolRoute) (map[string]litellm.McpServer, []routeOutcome) {
	log := logf.FromContext(ctx)
	servers := make(map[string]litellm.McpServer, len(routes))
	outcomes := make([]routeOutcome, 0, len(routes))

	for _, route := range routes {
		if !routeAttachesToGateway(&route, gw) {
			continue
		}

		out := routeOutcome{route: route}
		key := mcpServerKey(&route)

		if _, exists := servers[key]; exists {
			out.failureReason = reasonRouteDuplicateName
			out.failureMsg = fmt.Sprintf("mcp_servers key %q is already taken by another route", key)
			outcomes = append(outcomes, out)
			continue
		}

		up, err := resolveRouteUpstream(ctx, c, &route)
		if err != nil {
			log.Info("Route upstream unresolved; omitting from gateway config", "route", route.Name, "namespace", route.Namespace, "err", err)
			out.failureReason = reasonRouteUpstreamUnresolved
			out.failureMsg = err.Error()
			outcomes = append(outcomes, out)
			continue
		}

		entry := litellm.McpServer{
			Url:          up.url,
			Transport:    up.transport,
			AllowAllKeys: true,
		}
		if route.Spec.ToolFilter != nil {
			entry.AllowedTools = route.Spec.ToolFilter.Allow
			entry.DisallowedTools = route.Spec.ToolFilter.Deny
		}
		servers[key] = entry
		out.included = true
		outcomes = append(outcomes, out)
	}
	return servers, outcomes
}
