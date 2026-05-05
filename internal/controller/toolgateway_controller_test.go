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
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/litellm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	toolGatewayClassName = "litellm"
	toolGatewayNamespace = "tool-gateway"
)

func createDefaultToolGatewayClass() {
	cls := &gatewayv1alpha1.ToolGatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: toolGatewayClassName,
			Annotations: map[string]string{
				"toolgatewayclass.kubernetes.io/is-default-class": "true",
			},
		},
		Spec: gatewayv1alpha1.ToolGatewayClassSpec{Controller: ToolGatewayControllerName},
	}
	Expect(k8sClient.Create(ctx, cls)).To(Succeed())
}

func ensureNamespace(name string) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := k8sClient.Create(ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

// markDeploymentRolledOut simulates the deployment-controller's rollout result
// in envtest, where no kube-controller-manager runs to populate the status.
func markDeploymentRolledOut(name, namespace string) {
	d := &appsv1.Deployment{}
	Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, d)).To(Succeed())
	d.Status.ObservedGeneration = d.Generation
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	d.Status.Replicas = desired
	d.Status.ReadyReplicas = desired
	d.Status.AvailableReplicas = desired
	Expect(k8sClient.Status().Update(ctx, d)).To(Succeed())
}

var _ = Describe("ToolGateway Controller", Ordered, func() {

	BeforeAll(func() {
		ensureNamespace(toolGatewayNamespace)
		createDefaultToolGatewayClass()
	})

	AfterAll(func() {
		cls := &gatewayv1alpha1.ToolGatewayClass{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: toolGatewayClassName}, cls); err == nil {
			Expect(k8sClient.Delete(ctx, cls)).To(Succeed())
		}
	})

	Context("when reconciling a ToolGateway with two ToolRoutes", func() {
		var gw *gatewayv1alpha1.ToolGateway
		var ts *gatewayv1alpha1.ToolServer
		var routeOK *gatewayv1alpha1.ToolRoute
		var routeMissing *gatewayv1alpha1.ToolRoute

		gwKey := types.NamespacedName{Name: "gw1", Namespace: toolGatewayNamespace}
		routeOKKey := types.NamespacedName{Name: "ok-route", Namespace: "default"}
		routeMissingKey := types.NamespacedName{Name: "broken-route", Namespace: "default"}
		tsKey := types.NamespacedName{Name: "everything", Namespace: "default"}

		BeforeEach(func() {
			gw = &gatewayv1alpha1.ToolGateway{
				ObjectMeta: metav1.ObjectMeta{Name: gwKey.Name, Namespace: gwKey.Namespace},
				Spec:       gatewayv1alpha1.ToolGatewaySpec{},
			}
			Expect(k8sClient.Create(ctx, gw)).To(Succeed())

			ts = &gatewayv1alpha1.ToolServer{
				ObjectMeta: metav1.ObjectMeta{Name: tsKey.Name, Namespace: tsKey.Namespace},
				Spec: gatewayv1alpha1.ToolServerSpec{
					Protocol:      "mcp",
					TransportType: "http",
					Image:         "ignored",
					Port:          8080,
				},
			}
			Expect(k8sClient.Create(ctx, ts)).To(Succeed())

			gwRef := &corev1.ObjectReference{Name: gwKey.Name, Namespace: gwKey.Namespace}

			routeOK = &gatewayv1alpha1.ToolRoute{
				ObjectMeta: metav1.ObjectMeta{Name: routeOKKey.Name, Namespace: routeOKKey.Namespace},
				Spec: gatewayv1alpha1.ToolRouteSpec{
					ToolGatewayRef: gwRef,
					Upstream: gatewayv1alpha1.ToolRouteUpstream{
						ToolServerRef: &corev1.ObjectReference{Name: tsKey.Name},
					},
					ToolFilter: &gatewayv1alpha1.ToolFilter{Allow: []string{"echo"}},
				},
			}
			Expect(k8sClient.Create(ctx, routeOK)).To(Succeed())

			routeMissing = &gatewayv1alpha1.ToolRoute{
				ObjectMeta: metav1.ObjectMeta{Name: routeMissingKey.Name, Namespace: routeMissingKey.Namespace},
				Spec: gatewayv1alpha1.ToolRouteSpec{
					ToolGatewayRef: gwRef,
					Upstream: gatewayv1alpha1.ToolRouteUpstream{
						ToolServerRef: &corev1.ObjectReference{Name: "does-not-exist"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, routeMissing)).To(Succeed())
		})

		AfterEach(func() {
			for _, obj := range []client.Object{gw, routeOK, routeMissing, ts} {
				_ = k8sClient.Delete(ctx, obj)
			}
		})

		It("renders both routes' status outcomes", func() {
			rec := &ToolGatewayReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			By("first reconcile creates the workload but Ready stays False until rollout")
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: gwKey})
			Expect(err).NotTo(HaveOccurred())

			refreshedGw := &gatewayv1alpha1.ToolGateway{}
			Expect(k8sClient.Get(ctx, gwKey, refreshedGw)).To(Succeed())
			ready := findRouteCondition(refreshedGw.Status.Conditions, ToolGatewayReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal(ReasonToolGatewayRollingOut))
			Expect(refreshedGw.Status.Url).To(Equal(""))

			By("simulating deployment rollout and reconciling again")
			markDeploymentRolledOut(gwKey.Name, gwKey.Namespace)
			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: gwKey})
			Expect(err).NotTo(HaveOccurred())

			By("reading ok-route status")
			refreshed := &gatewayv1alpha1.ToolRoute{}
			Expect(k8sClient.Get(ctx, routeOKKey, refreshed)).To(Succeed())
			ready = findRouteCondition(refreshed.Status.Conditions, "Ready")
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal("Reconciled"))
			Expect(refreshed.Status.Url).To(Equal("http://gw1.tool-gateway.svc.cluster.local/mcp/default__ok_route"))

			By("reading broken-route status")
			refreshed = &gatewayv1alpha1.ToolRoute{}
			Expect(k8sClient.Get(ctx, routeMissingKey, refreshed)).To(Succeed())
			ready = findRouteCondition(refreshed.Status.Conditions, "Ready")
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal("UpstreamUnresolved"))
			Expect(refreshed.Status.Url).To(Equal(""))

			By("reading gateway status")
			Expect(k8sClient.Get(ctx, gwKey, refreshedGw)).To(Succeed())
			ready = findRouteCondition(refreshedGw.Status.Conditions, ToolGatewayReady)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(refreshedGw.Status.Url).To(Equal("http://gw1.tool-gateway.svc.cluster.local"))
		})
	})
})

// findRouteCondition returns a copy of the condition with the given type, or nil.
func findRouteCondition(conditions []metav1.Condition, t string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == t {
			cp := conditions[i]
			return &cp
		}
	}
	return nil
}

// ----- Plain-Go unit tests below this line. They use fake.NewClientBuilder
// and do not depend on envtest, so they run independently of the Ginkgo suite.

func toolGatewayScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := gatewayv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

// drainQueue pulls everything currently on the queue into a set so tests can
// assert membership without depending on insertion order.
func drainQueue(q workqueue.TypedRateLimitingInterface[reconcile.Request]) map[reconcile.Request]struct{} {
	got := map[reconcile.Request]struct{}{}
	for q.Len() > 0 {
		item, shutdown := q.Get()
		if shutdown {
			break
		}
		got[item] = struct{}{}
		q.Done(item)
	}
	return got
}

func TestRouteEventHandler_UpdateEnqueuesOldAndNewGateway(t *testing.T) {
	oldRoute := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: &corev1.ObjectReference{Name: "gw-A", Namespace: "ns-x"},
		},
	}
	newRoute := oldRoute.DeepCopy()
	newRoute.Spec.ToolGatewayRef = &corev1.ObjectReference{Name: "gw-B", Namespace: "ns-y"}

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()

	routeEventHandler().Update(context.Background(), event.UpdateEvent{
		ObjectOld: oldRoute,
		ObjectNew: newRoute,
	}, q)

	got := drainQueue(q)
	wantOld := reconcile.Request{NamespacedName: types.NamespacedName{Name: "gw-A", Namespace: "ns-x"}}
	wantNew := reconcile.Request{NamespacedName: types.NamespacedName{Name: "gw-B", Namespace: "ns-y"}}
	if _, ok := got[wantOld]; !ok {
		t.Errorf("missing previous gateway request %v in %v", wantOld, got)
	}
	if _, ok := got[wantNew]; !ok {
		t.Errorf("missing new gateway request %v in %v", wantNew, got)
	}
}

func TestRouteEventHandler_UpdateWithoutPriorRefStillEnqueuesNew(t *testing.T) {
	oldRoute := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
	}
	newRoute := oldRoute.DeepCopy()
	newRoute.Spec.ToolGatewayRef = &corev1.ObjectReference{Name: "gw-B", Namespace: "ns-y"}

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()

	routeEventHandler().Update(context.Background(), event.UpdateEvent{
		ObjectOld: oldRoute,
		ObjectNew: newRoute,
	}, q)

	got := drainQueue(q)
	wantNew := reconcile.Request{NamespacedName: types.NamespacedName{Name: "gw-B", Namespace: "ns-y"}}
	if _, ok := got[wantNew]; !ok {
		t.Errorf("missing new gateway request %v in %v", wantNew, got)
	}
	if len(got) != 1 {
		t.Errorf("expected exactly one request, got %d: %v", len(got), got)
	}
}

func TestIsTransientPhaseError(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantTry bool
	}{
		{"plain error defaults to transient", errors.New("boom"), true},
		{"ConfigRender is permanent", &litellm.PhaseError{Phase: "ConfigRender", Err: errors.New("yaml")}, false},
		{"Guardrails is permanent", &litellm.PhaseError{Phase: "Guardrails", Err: errors.New("missing")}, false},
		{"ListRoutes is transient", &litellm.PhaseError{Phase: "ListRoutes", Err: errors.New("api")}, true},
		{"ConfigMap is transient", &litellm.PhaseError{Phase: "ConfigMap", Err: errors.New("api")}, true},
		{"Secret is transient", &litellm.PhaseError{Phase: "Secret", Err: errors.New("api")}, true},
		{"Deployment is transient", &litellm.PhaseError{Phase: "Deployment", Err: errors.New("api")}, true},
		{"Service is transient", &litellm.PhaseError{Phase: "Service", Err: errors.New("api")}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientPhaseError(tc.err); got != tc.wantTry {
				t.Errorf("isTransientPhaseError(%v) = %v, want %v", tc.err, got, tc.wantTry)
			}
		})
	}
}

func TestMarkAttachedRoutesDegraded_OnlyAttachedRoutesArePatched(t *testing.T) {
	gw := &gatewayv1alpha1.ToolGateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "tgw-ns"},
	}
	attached := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "attached", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: &corev1.ObjectReference{Name: "gw", Namespace: "tgw-ns"},
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				External: &gatewayv1alpha1.ExternalUpstream{Url: "http://up/mcp"},
			},
		},
		Status: gatewayv1alpha1.ToolRouteStatus{
			Url: "http://gw.tgw-ns.svc.cluster.local/mcp/default__attached",
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "ok"},
			},
		},
	}
	detached := &gatewayv1alpha1.ToolRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "detached", Namespace: "default"},
		Spec: gatewayv1alpha1.ToolRouteSpec{
			ToolGatewayRef: &corev1.ObjectReference{Name: "other", Namespace: "tgw-ns"},
			Upstream: gatewayv1alpha1.ToolRouteUpstream{
				External: &gatewayv1alpha1.ExternalUpstream{Url: "http://up/mcp"},
			},
		},
		Status: gatewayv1alpha1.ToolRouteStatus{
			Conditions: []metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue, Reason: "Reconciled", Message: "ok"},
			},
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(toolGatewayScheme(t)).
		WithObjects(gw, attached, detached).
		WithStatusSubresource(&gatewayv1alpha1.ToolRoute{}).
		Build()

	r := &ToolGatewayReconciler{Client: c, Scheme: c.Scheme()}
	r.markAttachedRoutesDegraded(context.Background(), gw, errors.New("workload broken"))

	var got gatewayv1alpha1.ToolRoute
	if err := c.Get(context.Background(), types.NamespacedName{Name: "attached", Namespace: "default"}, &got); err != nil {
		t.Fatalf("get attached: %v", err)
	}
	cond := findRouteCondition(got.Status.Conditions, "Ready")
	if cond == nil {
		t.Fatalf("attached route missing Ready condition")
	}
	if cond.Status != metav1.ConditionFalse || cond.Reason != reasonRouteGatewayDegraded {
		t.Errorf("attached: got %v/%v, want False/%s", cond.Status, cond.Reason, reasonRouteGatewayDegraded)
	}
	if got.Status.Url != "" {
		t.Errorf("attached.Status.Url should be cleared by failure outcome, got %q", got.Status.Url)
	}

	if err := c.Get(context.Background(), types.NamespacedName{Name: "detached", Namespace: "default"}, &got); err != nil {
		t.Fatalf("get detached: %v", err)
	}
	cond = findRouteCondition(got.Status.Conditions, "Ready")
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("detached route should be left alone, got %+v", cond)
	}
}
