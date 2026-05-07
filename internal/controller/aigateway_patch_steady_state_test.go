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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("AiGateway Controller — patch steady-state", func() {
	const (
		testNS       = "default"
		testPort     = int32(8000)
	)

	Context("When reconciling an AiGateway with a valid patch ConfigMap twice with no input changes", func() {
		gatewayKey := types.NamespacedName{Name: "ai-patch-steady", Namespace: testNS}
		classKey := types.NamespacedName{Name: aiGatewayClassName}
		patchCMKey := types.NamespacedName{Name: "ai-patch-steady-cm", Namespace: testNS}
		ownedCMKey := types.NamespacedName{Name: gatewayKey.Name + "-config", Namespace: testNS}

		BeforeEach(func() {
			createDefaultClass(classKey)

			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: patchCMKey.Name, Namespace: patchCMKey.Namespace},
				Data: map[string]string{
					"patch.yaml": "router_settings:\n  routing_strategy: usage-based-routing-v2\n",
				},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayKey.Name,
					Namespace: testNS,
					Annotations: map[string]string{
						litellm.ConfigPatchAnnotation: patchCMKey.Name,
					},
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port:     testPort,
					AiModels: []gatewayv1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}},
				},
			})).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayKey)
			cleanupAiGatewayClass(classKey)
			cleanupConfigMap(patchCMKey)
		})

		It("does not drift the ConfigMap or change the deployment config-hash on a no-op pass", func() {
			rec := &AiGatewayReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}

			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayKey})
			Expect(err).NotTo(HaveOccurred())

			firstCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, ownedCMKey, firstCM)).To(Succeed())
			firstYAML := firstCM.Data["config.yaml"]
			Expect(firstYAML).To(ContainSubstring("router_settings"))
			firstResourceVersion := firstCM.ResourceVersion

			firstDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, gatewayKey, firstDeploy)).To(Succeed())
			firstHash := firstDeploy.Spec.Template.Annotations["gateway.agentic-layer.ai/config-hash"]
			Expect(firstHash).ToNot(BeEmpty())
			firstDeployResourceVersion := firstDeploy.ResourceVersion

			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayKey})
			Expect(err).NotTo(HaveOccurred())

			secondCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, ownedCMKey, secondCM)).To(Succeed())
			Expect(secondCM.Data["config.yaml"]).To(Equal(firstYAML), "config.yaml must be byte-identical across no-op reconciles")
			Expect(secondCM.ResourceVersion).To(Equal(firstResourceVersion), "ConfigMap must not be rewritten on a no-op reconcile")

			secondDeploy := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, gatewayKey, secondDeploy)).To(Succeed())
			Expect(secondDeploy.Spec.Template.Annotations["gateway.agentic-layer.ai/config-hash"]).To(Equal(firstHash), "config-hash annotation must be stable across no-op reconciles")
			Expect(secondDeploy.ResourceVersion).To(Equal(firstDeployResourceVersion), "Deployment must not be rewritten on a no-op reconcile")
		})
	})
})
