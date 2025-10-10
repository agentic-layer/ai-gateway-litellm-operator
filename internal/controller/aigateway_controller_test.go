/*
Copyright 2025 Agentic Layer.

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1alpha1 "github.com/agentic-layer/ai-gateway-operator/api/v1alpha1"
)

var _ = Describe("AiGateway Controller", func() {
	const (
		resourceName        = "test-aigateway"
		testNamespace       = "default"
		invalidType         = "some-invalid-type"
		validType           = "litellm"
		testPort      int32 = 8000
	)

	ctx := context.Background()

	Context("When reconciling a valid AiGateway resource", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var aiGatewayClass *gatewayv1alpha1.AiGatewayClass
		var namespacedName types.NamespacedName

		BeforeEach(func() {
			namespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: testNamespace,
			}

			By("Creating a default AiGatewayClass")
			aiGatewayClass = &gatewayv1alpha1.AiGatewayClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "litellm",
					Namespace: testNamespace,
					Annotations: map[string]string{
						"aigatewayclass.kubernetes.io/is-default-class": "true",
					},
				},
				Spec: gatewayv1alpha1.AiGatewayClassSpec{
					Controller: ControllerName,
				},
			}
			Expect(k8sClient.Create(ctx, aiGatewayClass)).To(Succeed())

			By("Creating a valid AiGateway resource")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo", Provider: "openai"},
						{Name: "gpt-4", Provider: "openai"},
						{Name: "claude-3-opus", Provider: "anthropic"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up the AiGateway resource")
			resource := &gatewayv1alpha1.AiGateway{}
			if err := k8sClient.Get(ctx, namespacedName, resource); err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			By("Cleaning up the AiGatewayClass")
			if aiGatewayClass != nil {
				classResource := &gatewayv1alpha1.AiGatewayClass{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: aiGatewayClass.Name, Namespace: aiGatewayClass.Namespace}, classResource); err == nil {
					Expect(k8sClient.Delete(ctx, classResource)).To(Succeed())
				}
			}
		})

		It("should successfully reconcile and create all required resources", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Check all resources were created successfully
			checkConfigMapReconciled(ctx, namespacedName)
			checkDeploymentReconciled(ctx, namespacedName)
			checkServiceReconciled(ctx, namespacedName)
			checkStatusConditions(ctx, namespacedName, true)
		})
	})

})

// checkConfigMapReconciled verifies that the ConfigMap was created with correct LiteLLM configuration
func checkConfigMapReconciled(ctx context.Context, namespacedName types.NamespacedName) {
	By("Verifying ConfigMap was created")
	configMap := &corev1.ConfigMap{}
	configMapName := namespacedName.Name + "-config"
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: namespacedName.Namespace,
	}, configMap)
	Expect(err).NotTo(HaveOccurred())

	By("Verifying ConfigMap has correct labels")
	Expect(configMap.Labels).To(HaveKeyWithValue("app", namespacedName.Name))

	By("Verifying ConfigMap contains litellm configuration")
	Expect(configMap.Data).To(HaveKey("config.yaml"))
	configContent := configMap.Data["config.yaml"]
	Expect(configContent).To(ContainSubstring("model_list"))
	Expect(configContent).To(ContainSubstring("gpt-3.5-turbo"))
	Expect(configContent).To(ContainSubstring("gpt-4"))
	Expect(configContent).To(ContainSubstring("claude-3-opus"))

	By("Verifying ConfigMap has owner reference")
	Expect(configMap.OwnerReferences).To(HaveLen(1))
	Expect(configMap.OwnerReferences[0].Kind).To(Equal("AiGateway"))
	Expect(configMap.OwnerReferences[0].Name).To(Equal(namespacedName.Name))
	Expect(*configMap.OwnerReferences[0].Controller).To(BeTrue())
}

// checkDeploymentReconciled verifies that the Deployment was created correctly
func checkDeploymentReconciled(ctx context.Context, namespacedName types.NamespacedName) {
	By("Verifying Deployment was created")
	deployment := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, namespacedName, deployment)
	Expect(err).NotTo(HaveOccurred())

	By("Verifying Deployment has correct labels")
	Expect(deployment.Labels).To(HaveKeyWithValue("app", namespacedName.Name))

	By("Verifying Deployment has correct replicas")
	Expect(deployment.Spec.Replicas).NotTo(BeNil())
	Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))

	By("Verifying Deployment has correct selector")
	Expect(deployment.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app", namespacedName.Name))

	By("Verifying Deployment pod template")
	Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app", namespacedName.Name))

	By("Verifying Deployment pod template has config-hash annotation")
	Expect(deployment.Spec.Template.Annotations).To(HaveKey("gateway.agentic-layer.ai/config-hash"))

	By("Verifying Deployment has litellm container")
	Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
	container := deployment.Spec.Template.Spec.Containers[0]
	Expect(container.Name).To(Equal("litellm"))
	Expect(container.Image).To(ContainSubstring("litellm"))

	By("Verifying Deployment container port")
	Expect(container.Ports).To(HaveLen(1))
	Expect(container.Ports[0].ContainerPort).To(Equal(int32(8000)))
	Expect(container.Ports[0].Name).To(Equal("http"))

	By("Verifying Deployment has ConfigMap volume mount")
	Expect(container.VolumeMounts).To(HaveLen(1))
	Expect(container.VolumeMounts[0].Name).To(Equal("config"))
	Expect(container.VolumeMounts[0].MountPath).To(Equal("/app/config"))

	By("Verifying Deployment has owner reference")
	Expect(deployment.OwnerReferences).To(HaveLen(1))
	Expect(deployment.OwnerReferences[0].Kind).To(Equal("AiGateway"))
	Expect(deployment.OwnerReferences[0].Name).To(Equal(namespacedName.Name))
	Expect(*deployment.OwnerReferences[0].Controller).To(BeTrue())
}

// checkServiceReconciled verifies that the Service was created correctly
func checkServiceReconciled(ctx context.Context, namespacedName types.NamespacedName) {
	By("Verifying Service was created")
	service := &corev1.Service{}
	err := k8sClient.Get(ctx, namespacedName, service)
	Expect(err).NotTo(HaveOccurred())

	By("Verifying Service has correct labels")
	Expect(service.Labels).To(HaveKeyWithValue("app", namespacedName.Name))

	By("Verifying Service has correct selector")
	Expect(service.Spec.Selector).To(HaveKeyWithValue("app", namespacedName.Name))

	By("Verifying Service has correct port")
	Expect(service.Spec.Ports).To(HaveLen(1))
	Expect(service.Spec.Ports[0].Port).To(Equal(int32(8000)))
	Expect(service.Spec.Ports[0].TargetPort.IntVal).To(Equal(int32(8000)))
	Expect(service.Spec.Ports[0].Name).To(Equal("http"))
	Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))

	By("Verifying Service has owner reference")
	Expect(service.OwnerReferences).To(HaveLen(1))
	Expect(service.OwnerReferences[0].Kind).To(Equal("AiGateway"))
	Expect(service.OwnerReferences[0].Name).To(Equal(namespacedName.Name))
	Expect(*service.OwnerReferences[0].Controller).To(BeTrue())
}

// checkStatusConditions verifies that the appropriate status conditions are set
func checkStatusConditions(ctx context.Context, namespacedName types.NamespacedName, shouldBeReady bool) {
	By("Verifying AiGateway status conditions")
	aiGateway := &gatewayv1alpha1.AiGateway{}
	err := k8sClient.Get(ctx, namespacedName, aiGateway)
	Expect(err).NotTo(HaveOccurred())

	if shouldBeReady {
		// Should have successful conditions
		configuredCondition := findCondition(aiGateway.Status.Conditions, "AiGatewayConfigured")
		Expect(configuredCondition).NotTo(BeNil())
		Expect(configuredCondition.Status).To(Equal(metav1.ConditionTrue))

		readyCondition := findCondition(aiGateway.Status.Conditions, "AiGatewayReady")
		Expect(readyCondition).NotTo(BeNil())
		Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
	} else {
		// Should have failure conditions
		conditions := aiGateway.Status.Conditions
		Expect(conditions).ToNot(BeEmpty())

		// At least one condition should be False
		hasFailureCondition := false
		for _, condition := range conditions {
			if condition.Status == metav1.ConditionFalse {
				hasFailureCondition = true
				break
			}
		}
		Expect(hasFailureCondition).To(BeTrue())
	}
}

// findCondition finds a condition by type in the conditions slice
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
