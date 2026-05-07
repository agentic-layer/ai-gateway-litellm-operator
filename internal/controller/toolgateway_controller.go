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
	stderrors "errors"
	"fmt"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/litellm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ToolGatewayControllerName is the reverse-DNS controller identifier matched against
// ToolGatewayClass.spec.controller. Must match the value in
// config/install/toolgatewayclass.yaml.
const ToolGatewayControllerName = "litellm.agentic-layer.ai/tool-gateway-litellm-controller"

// Status condition types
const (
	ToolGatewayConfigured = "ToolGatewayConfigured"
	ToolGatewayReady      = "ToolGatewayReady"
)

// Status condition reasons
const (
	ReasonToolGatewayConfigurationApplied = "ConfigurationApplied"
	ReasonToolGatewayReady                = "ToolGatewayReady"
	ReasonToolGatewayRollingOut           = "DeploymentRollingOut"
	ReasonToolGatewayConfigGenFailed      = "ConfigGenerationFailed"
	ReasonToolGatewayGuardrails           = "GuardrailsResolutionFailed"
	ReasonToolGatewayConfigMap            = "ConfigMapFailed"
	ReasonToolGatewaySecret               = "SecretFailed"
	ReasonToolGatewayDeployment           = "DeploymentFailed"
	ReasonToolGatewayService              = "ServiceFailed"
	ReasonToolGatewayWorkload             = "WorkloadFailed"
)

// PhaseError phase names for reconcile steps that translate user input.
const (
	phaseConfigRender = "ConfigRender"
	phaseGuardrails   = "Guardrails"
)

// ToolGatewayReconciler reconciles a ToolGateway object. It is the sole writer
// of both ToolGateway and ToolRoute status — every ToolRoute that targets this
// gateway gets its status patched from the same reconcile pass.
type ToolGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=toolgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=toolgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=toolgateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=toolgatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=toolroutes,verbs=get;list;watch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=toolroutes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=toolservers,verbs=get;list;watch

func (r *ToolGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var toolGateway gatewayv1alpha1.ToolGateway
	if err := r.Get(ctx, req.NamespacedName, &toolGateway); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ToolGateway")
		return ctrl.Result{}, err
	}
	original := toolGateway.DeepCopy()

	owned, err := litellm.IsToolGatewayOwnedByController(ctx, r, &toolGateway, ToolGatewayControllerName)
	if err != nil {
		// Surface so controller-runtime requeues with backoff. Swallowing the
		// error left spec edits unobserved during transient API outages.
		log.Error(err, "Failed to determine controller ownership")
		return ctrl.Result{}, err
	}
	if !owned {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling ToolGateway", "name", toolGateway.Name, "namespace", toolGateway.Namespace)

	if toolGateway.Status.Conditions == nil {
		toolGateway.Status.Conditions = []metav1.Condition{}
	}

	outcomes, err := r.reconcile(ctx, &toolGateway)
	if err != nil {
		r.applyWorkloadError(&toolGateway, err)
		if e := r.patchStatus(ctx, original, &toolGateway); e != nil {
			return ctrl.Result{}, e
		}
		// Surface the failure on every attached route so consumers do not keep
		// trusting a stale Status.Url after the gateway breaks.
		r.markAttachedRoutesDegraded(ctx, &toolGateway, err)
		// Transient API failures (ListRoutes / ConfigMap / Secret / Deployment /
		// Service) get requeued by controller-runtime's exponential backoff.
		// Permanent failures (ConfigRender / Guardrails) wait for a spec edit —
		// the resulting watch event re-reconciles us.
		if isTransientPhaseError(err) {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	for _, outcome := range outcomes {
		if err := applyRouteOutcome(ctx, r.Client, &toolGateway, outcome); err != nil {
			log.Error(err, "Failed to patch ToolRoute status",
				"route", outcome.route.Name, "namespace", outcome.route.Namespace)
			// Non-fatal: gateway-level reconcile already succeeded; route status
			// will be retried on next pass.
		}
	}

	r.updateCondition(&toolGateway, ToolGatewayConfigured, metav1.ConditionTrue,
		ReasonToolGatewayConfigurationApplied, "ToolGateway configuration successfully applied")

	// Ready reflects pod-level availability, not just "we created the API objects".
	// The Owns(&appsv1.Deployment{}) watch re-fires Reconcile when the deployment-
	// controller publishes status changes, so we don't need a manual requeue.
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: toolGateway.Namespace, Name: toolGateway.Name}, deployment); err != nil {
		log.Error(err, "Failed to get Deployment for rollout check")
		return ctrl.Result{}, err
	}
	if rolledOut, msg := litellm.IsDeploymentRolledOut(deployment); rolledOut {
		r.updateCondition(&toolGateway, ToolGatewayReady, metav1.ConditionTrue,
			ReasonToolGatewayReady, "ToolGateway is ready and serving traffic")
		toolGateway.Status.Url = fmt.Sprintf("http://%s.%s.svc.cluster.local", toolGateway.Name, toolGateway.Namespace)
	} else {
		r.updateCondition(&toolGateway, ToolGatewayReady, metav1.ConditionFalse,
			ReasonToolGatewayRollingOut, msg)
		toolGateway.Status.Url = ""
	}

	if err := r.patchStatus(ctx, original, &toolGateway); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

const (
	toolGatewayContainerPort int32 = 4000
	toolGatewayServicePort   int32 = 80
)

// reconcile renders mcp_servers + guardrails into a LiteLLMConfig and applies
// the workload (ConfigMap + Deployment + Service) via the shared litellm
// package. Returns the per-route outcomes so the caller can patch route
// statuses after the gateway-level reconcile succeeds. Every failure path
// returns a *litellm.PhaseError so applyWorkloadError can map it to a stable
// status reason.
func (r *ToolGatewayReconciler) reconcile(ctx context.Context, gw *gatewayv1alpha1.ToolGateway) ([]routeOutcome, error) {
	var routeList gatewayv1alpha1.ToolRouteList
	if err := r.List(ctx, &routeList); err != nil {
		return nil, &litellm.PhaseError{Phase: "ListRoutes", Err: err}
	}

	servers, outcomes := buildMcpServers(ctx, r, gw, routeList.Items)

	guardrails, err := litellm.ResolveGuardrails(ctx, r, gw.Namespace, gw.Spec.Guardrails, litellm.GuardrailTargetMCP)
	if err != nil {
		return nil, &litellm.PhaseError{Phase: phaseGuardrails, Err: err}
	}

	cfg := litellm.LiteLLMConfig{
		McpServers: servers,
		LiteLLMSettings: litellm.LiteLLMSettings{
			RequestTimeout: litellm.DefaultRequestTimeout,
			Callbacks:      []string{"otel", "prometheus"},
		},
		Guardrails: guardrails,
	}

	configYAML, err := litellm.RenderConfig(cfg)
	if err != nil {
		return nil, &litellm.PhaseError{Phase: phaseConfigRender, Err: err}
	}

	workload := litellm.GatewayWorkload{
		Name:           gw.Name,
		Namespace:      gw.Namespace,
		Owner:          gw,
		ContainerPort:  toolGatewayContainerPort,
		ServicePort:    toolGatewayServicePort,
		Env:            gw.Spec.Env,
		EnvFrom:        gw.Spec.EnvFrom,
		CommonMetadata: gw.Spec.CommonMetadata,
		PodMetadata:    gw.Spec.PodMetadata,
		ConfigYAML:     configYAML,
	}
	if err := litellm.ReconcileWorkload(ctx, r.Client, r.Scheme, workload); err != nil {
		return nil, err
	}

	return outcomes, nil
}

// applyWorkloadError maps a reconcile error to phase-specific status conditions.
// Both ToolGatewayConfigured and ToolGatewayReady are flipped to False on every
// failure — leaving Ready=True after a config-side failure would be a contradiction.
// Status.Url is also cleared so consumers cannot keep trusting the previous URL
// while the gateway is broken.
func (r *ToolGatewayReconciler) applyWorkloadError(gw *gatewayv1alpha1.ToolGateway, err error) {
	reason := ReasonToolGatewayWorkload

	if pe, ok := stderrors.AsType[*litellm.PhaseError](err); ok {
		switch pe.Phase {
		case "ListRoutes", phaseConfigRender:
			reason = ReasonToolGatewayConfigGenFailed
		case phaseGuardrails:
			reason = ReasonToolGatewayGuardrails
		case "ConfigMap":
			reason = ReasonToolGatewayConfigMap
		case "Secret":
			reason = ReasonToolGatewaySecret
		case "Deployment":
			reason = ReasonToolGatewayDeployment
		case "Service":
			reason = ReasonToolGatewayService
		}
	}

	r.updateCondition(gw, ToolGatewayConfigured, metav1.ConditionFalse, reason, err.Error())
	r.updateCondition(gw, ToolGatewayReady, metav1.ConditionFalse, reason, err.Error())
	gw.Status.Url = ""
}

// updateCondition stamps a condition on the gateway via apimeta.SetStatusCondition,
// which preserves LastTransitionTime when status/reason/message are unchanged.
func (r *ToolGatewayReconciler) updateCondition(gw *gatewayv1alpha1.ToolGateway, conditionType string, status metav1.ConditionStatus, reason, message string) {
	apimeta.SetStatusCondition(&gw.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: gw.Generation,
	})
}

// patchStatus issues an optimistic-merge patch against the snapshot captured
// at the start of Reconcile. Using Patch rather than Update keeps concurrent
// status writes from other workers / sub-resources from clobbering each other.
func (r *ToolGatewayReconciler) patchStatus(ctx context.Context, original, gw *gatewayv1alpha1.ToolGateway) error {
	if err := r.Status().Patch(ctx, gw, client.MergeFrom(original)); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to patch ToolGateway status")
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager. It wires watches so
// that the reconciler re-runs whenever a ToolRoute, ToolServer, Guard,
// GuardrailProvider, or the api-keys Secret changes.
func (r *ToolGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	enqueueViaToolRoute := routeEventHandler()

	// enqueueViaToolServer fans out via mapRouteToGateways for every route that
	// references the changed ToolServer, deduping target gateways.
	enqueueViaToolServer := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := logf.FromContext(ctx)
		ts, ok := obj.(*gatewayv1alpha1.ToolServer)
		if !ok {
			return nil
		}
		var routes gatewayv1alpha1.ToolRouteList
		if err := r.List(ctx, &routes); err != nil {
			log.Error(err, "Failed to list ToolRoutes for ToolServer watch")
			return nil
		}
		seen := map[types.NamespacedName]struct{}{}
		var requests []reconcile.Request
		for i := range routes.Items {
			route := &routes.Items[i]
			ref := route.Spec.Upstream.ToolServerRef
			if ref == nil {
				continue
			}
			ns := ref.Namespace
			if ns == "" {
				ns = route.Namespace
			}
			if ref.Name != ts.Name || ns != ts.Namespace {
				continue
			}
			for _, req := range mapRouteToGateways(route) {
				if _, dup := seen[req.NamespacedName]; dup {
					continue
				}
				seen[req.NamespacedName] = struct{}{}
				requests = append(requests, req)
			}
		}
		return requests
	})

	// enqueueGatewaysInNamespace enqueues all our ToolGateways in the namespace
	// of the triggering object. Used for Guard / GuardrailProvider / Secret
	// changes — same scoping as the AiGateway controller's existing watches.
	enqueueGatewaysInNamespace := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := logf.FromContext(ctx)
		var gwList gatewayv1alpha1.ToolGatewayList
		if err := r.List(ctx, &gwList, client.InNamespace(obj.GetNamespace())); err != nil {
			log.Error(err, "Failed to list ToolGateways for namespace watch", "namespace", obj.GetNamespace())
			return nil
		}
		requests := make([]reconcile.Request, 0, len(gwList.Items))
		for _, gw := range gwList.Items {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}})
		}
		return requests
	})

	// enqueueAllToolGateways fans out to every ToolGateway in the cluster.
	// Used for ToolGatewayClass changes (cluster-scoped) — the default-class
	// annotation toggling has to re-evaluate gateways that don't name a class
	// explicitly.
	enqueueAllToolGateways := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
		log := logf.FromContext(ctx)
		var gwList gatewayv1alpha1.ToolGatewayList
		if err := r.List(ctx, &gwList); err != nil {
			log.Error(err, "Failed to list ToolGateways for ToolGatewayClass watch")
			return nil
		}
		requests := make([]reconcile.Request, 0, len(gwList.Items))
		for _, gw := range gwList.Items {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}})
		}
		return requests
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.ToolGateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&gatewayv1alpha1.ToolRoute{}, enqueueViaToolRoute).
		Watches(&gatewayv1alpha1.ToolServer{}, enqueueViaToolServer).
		Watches(&gatewayv1alpha1.ToolGatewayClass{}, enqueueAllToolGateways).
		Watches(&gatewayv1alpha1.Guard{}, enqueueGatewaysInNamespace).
		Watches(&gatewayv1alpha1.GuardrailProvider{}, enqueueGatewaysInNamespace).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				if obj.GetName() != litellm.ApiKeySecretName {
					return nil
				}
				var gwList gatewayv1alpha1.ToolGatewayList
				if err := r.List(ctx, &gwList, client.InNamespace(obj.GetNamespace())); err != nil {
					return nil
				}
				requests := make([]reconcile.Request, 0, len(gwList.Items))
				for _, gw := range gwList.Items {
					requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}})
				}
				return requests
			}),
		).
		Named(ToolGatewayControllerName).
		Complete(r)
}

// routeEventHandler builds the event handler for ToolRoute watches. On Update
// it enqueues both the previous and current ToolGatewayRef so a reparented
// route triggers the *previous* gateway to drop the route from its mcp_servers
// in addition to the new gateway picking it up. q.Add is idempotent at the
// workqueue level, so no manual deduplication is needed.
func routeEventHandler() handler.Funcs {
	return handler.Funcs{
		CreateFunc: func(_ context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if route, ok := e.Object.(*gatewayv1alpha1.ToolRoute); ok {
				enqueueRouteRequests(route, q)
			}
		},
		UpdateFunc: func(_ context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if route, ok := e.ObjectOld.(*gatewayv1alpha1.ToolRoute); ok {
				enqueueRouteRequests(route, q)
			}
			if route, ok := e.ObjectNew.(*gatewayv1alpha1.ToolRoute); ok {
				enqueueRouteRequests(route, q)
			}
		},
		DeleteFunc: func(_ context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if route, ok := e.Object.(*gatewayv1alpha1.ToolRoute); ok {
				enqueueRouteRequests(route, q)
			}
		},
		GenericFunc: func(_ context.Context, e event.GenericEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if route, ok := e.Object.(*gatewayv1alpha1.ToolRoute); ok {
				enqueueRouteRequests(route, q)
			}
		},
	}
}

// mapRouteToGateways returns the single ToolGateway whose Reconcile loop should
// re-evaluate this route. Routes without an explicit ToolGatewayRef fan out to
// nothing — see routeAttachesToGateway.
func mapRouteToGateways(route *gatewayv1alpha1.ToolRoute) []reconcile.Request {
	ref := route.Spec.ToolGatewayRef
	if ref == nil {
		return nil
	}
	ns := ref.Namespace
	if ns == "" {
		ns = route.Namespace
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: ref.Name, Namespace: ns}}}
}

// enqueueRouteRequests adds the gateway requests for the given route to the
// workqueue. Used by the ToolRoute event handler so Create / Update / Delete
// share one fan-out path.
func enqueueRouteRequests(route *gatewayv1alpha1.ToolRoute, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	for _, req := range mapRouteToGateways(route) {
		q.Add(req)
	}
}

// isTransientPhaseError reports whether a workload reconcile error should be
// requeued by controller-runtime. Phases that hit the apiserver are transient —
// exponential backoff is the right recovery. Phases that translate user input
// (ConfigRender, Guardrails) are permanent: they will not heal until the user
// edits the spec, which fires its own watch event.
func isTransientPhaseError(err error) bool {
	pe, ok := stderrors.AsType[*litellm.PhaseError](err)
	if !ok {
		return true
	}
	switch pe.Phase {
	case phaseConfigRender, phaseGuardrails:
		return false
	default:
		return true
	}
}

// markAttachedRoutesDegraded marks every ToolRoute that would attach to gw as
// Ready=False/GatewayDegraded so consumers do not keep trusting a Status.Url
// rendered before the gateway broke. List failures are logged and ignored —
// the gateway's own status reflects the underlying issue, and the next
// successful reconcile overwrites each route's condition.
func (r *ToolGatewayReconciler) markAttachedRoutesDegraded(ctx context.Context, gw *gatewayv1alpha1.ToolGateway, cause error) {
	log := logf.FromContext(ctx)
	var routes gatewayv1alpha1.ToolRouteList
	if err := r.List(ctx, &routes); err != nil {
		log.Error(err, "Failed to list ToolRoutes for degraded-status update")
		return
	}
	for i := range routes.Items {
		route := &routes.Items[i]
		if !routeAttachesToGateway(route, gw) {
			continue
		}
		outcome := routeOutcome{
			route:         *route,
			failureReason: reasonRouteGatewayDegraded,
			failureMsg:    fmt.Sprintf("ToolGateway reconcile failed: %v", cause),
		}
		if err := applyRouteOutcome(ctx, r.Client, gw, outcome); err != nil {
			log.Error(err, "Failed to patch degraded ToolRoute status",
				"route", route.Name, "namespace", route.Namespace)
		}
	}
}
