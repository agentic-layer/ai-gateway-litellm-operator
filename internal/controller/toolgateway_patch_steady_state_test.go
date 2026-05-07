/*
Copyright 2026 Agentic Layer.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package controller

import (
	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/litellm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("ToolGateway Controller — patch steady-state", Ordered, func() {
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

	Context("When reconciling a ToolGateway with a valid patch ConfigMap twice with no input changes", func() {
		gwKey := types.NamespacedName{Name: "tg-patch-steady", Namespace: toolGatewayNamespace}
		patchCMKey := types.NamespacedName{Name: "tg-patch-steady-cm", Namespace: toolGatewayNamespace}
		routeKey := types.NamespacedName{Name: "tg-patch-steady-route", Namespace: "default"}
		tsKey := types.NamespacedName{Name: "tg-patch-steady-ts", Namespace: "default"}
		ownedCMKey := types.NamespacedName{Name: gwKey.Name + "-config", Namespace: gwKey.Namespace}

		BeforeEach(func() {
			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: patchCMKey.Name, Namespace: patchCMKey.Namespace},
				Data: map[string]string{
					"patch.yaml": "mcp_servers:\n  default__tg_patch_steady_route:\n    auth_type: oauth2\n    headers:\n      Authorization: os.environ/TOKEN\n",
				},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &gatewayv1alpha1.ToolGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwKey.Name,
					Namespace: gwKey.Namespace,
					Annotations: map[string]string{
						litellm.ConfigPatchAnnotation: patchCMKey.Name,
					},
				},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &gatewayv1alpha1.ToolServer{
				ObjectMeta: metav1.ObjectMeta{Name: tsKey.Name, Namespace: tsKey.Namespace},
				Spec:       gatewayv1alpha1.ToolServerSpec{Protocol: "mcp", TransportType: "http", Image: "ignored", Port: 8080},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &gatewayv1alpha1.ToolRoute{
				ObjectMeta: metav1.ObjectMeta{Name: routeKey.Name, Namespace: routeKey.Namespace},
				Spec: gatewayv1alpha1.ToolRouteSpec{
					ToolGatewayRef: &corev1.ObjectReference{Name: gwKey.Name, Namespace: gwKey.Namespace},
					Upstream:       gatewayv1alpha1.ToolRouteUpstream{ToolServerRef: &corev1.ObjectReference{Name: tsKey.Name}},
				},
			})).To(Succeed())
		})

		AfterEach(func() {
			for _, obj := range []client.Object{
				&gatewayv1alpha1.ToolRoute{ObjectMeta: metav1.ObjectMeta{Name: routeKey.Name, Namespace: routeKey.Namespace}},
				&gatewayv1alpha1.ToolServer{ObjectMeta: metav1.ObjectMeta{Name: tsKey.Name, Namespace: tsKey.Namespace}},
				&gatewayv1alpha1.ToolGateway{ObjectMeta: metav1.ObjectMeta{Name: gwKey.Name, Namespace: gwKey.Namespace}},
				&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: patchCMKey.Name, Namespace: patchCMKey.Namespace}},
			} {
				_ = k8sClient.Delete(ctx, obj)
			}
		})

		It("does not drift the ConfigMap or change the deployment config-hash on a no-op pass", func() {
			rec := &ToolGatewayReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: gwKey})
			Expect(err).NotTo(HaveOccurred())

			firstCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, ownedCMKey, firstCM)).To(Succeed())
			firstYAML := firstCM.Data["config.yaml"]
			Expect(firstYAML).To(ContainSubstring("auth_type: oauth2"))
			firstResourceVersion := firstCM.ResourceVersion

			firstDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, gwKey, firstDeploy)).To(Succeed())
			firstHash := firstDeploy.Spec.Template.Annotations["gateway.agentic-layer.ai/config-hash"]
			Expect(firstHash).ToNot(BeEmpty())
			firstDeployResourceVersion := firstDeploy.ResourceVersion

			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: gwKey})
			Expect(err).NotTo(HaveOccurred())

			secondCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, ownedCMKey, secondCM)).To(Succeed())
			Expect(secondCM.Data["config.yaml"]).To(Equal(firstYAML), "config.yaml must be byte-identical across no-op reconciles")
			Expect(secondCM.ResourceVersion).To(Equal(firstResourceVersion), "ConfigMap must not be rewritten on a no-op reconcile")

			secondDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, gwKey, secondDeploy)).To(Succeed())
			Expect(secondDeploy.Spec.Template.Annotations["gateway.agentic-layer.ai/config-hash"]).To(Equal(firstHash), "config-hash annotation must be stable across no-op reconciles")
			Expect(secondDeploy.ResourceVersion).To(Equal(firstDeployResourceVersion), "Deployment must not be rewritten on a no-op reconcile")
		})
	})
})
