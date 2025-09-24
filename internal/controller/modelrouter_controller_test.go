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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gatewayv1alpha1 "github.com/agentic-layer/ai-gateway-litellm/api/v1alpha1"
)

var _ = Describe("ModelRouter Controller", func() {
	const (
		resourceName        = "test-modelrouter"
		testNamespace       = "default"
		invalidType         = "some-invalid-type"
		validType           = "litellm"
		testPort      int32 = 8000
	)

	ctx := context.Background()

	Context("When reconciling a valid ModelRouter resource", func() {
		var modelRouter *gatewayv1alpha1.ModelRouter
		var namespacedName types.NamespacedName

		BeforeEach(func() {
			namespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: testNamespace,
			}

			By("Creating a valid ModelRouter resource")
			modelRouter = &gatewayv1alpha1.ModelRouter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.ModelRouterSpec{
					Type: validType,
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo"},
						{Name: "gpt-4"},
						{Name: "claude-3-opus"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, modelRouter)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up the ModelRouter resource")
			resource := &gatewayv1alpha1.ModelRouter{}
			if err := k8sClient.Get(ctx, namespacedName, resource); err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile and create all required resources", func() {
			By("Reconciling the created resource")
			reconciler := &ModelRouterReconciler{
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

	Context("When reconciling a ModelRouter with invalid configuration", func() {
		var modelRouter *gatewayv1alpha1.ModelRouter
		var namespacedName types.NamespacedName

		BeforeEach(func() {
			namespacedName = types.NamespacedName{
				Name:      resourceName + "-invalid",
				Namespace: testNamespace,
			}

			By("Creating a ModelRouter with invalid configuration (no AI models)")
			modelRouter = &gatewayv1alpha1.ModelRouter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName + "-invalid",
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.ModelRouterSpec{
					Type:     validType, // Use valid type but invalid config
					Port:     testPort,
					AiModels: []gatewayv1alpha1.AiModel{}, // Empty models - this should be invalid
				},
			}
			Expect(k8sClient.Create(ctx, modelRouter)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up the invalid ModelRouter resource")
			resource := &gatewayv1alpha1.ModelRouter{}
			if err := k8sClient.Get(ctx, namespacedName, resource); err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should handle invalid configuration and set appropriate conditions", func() {
			By("Reconciling the invalid resource")
			reconciler := &ModelRouterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})

			// Should handle the error gracefully (might requeue)
			Expect(err).ToNot(HaveOccurred())

			// Verify that resources were not created due to invalid config
			checkResourcesNotCreated(ctx, namespacedName)
			checkStatusConditions(ctx, namespacedName, false)
		})
	})

	Context("When reconciling a ModelRouter with no models", func() {
		var modelRouter *gatewayv1alpha1.ModelRouter
		var namespacedName types.NamespacedName

		BeforeEach(func() {
			namespacedName = types.NamespacedName{
				Name:      resourceName + "-nomodels",
				Namespace: testNamespace,
			}

			By("Creating a ModelRouter with no AI models")
			modelRouter = &gatewayv1alpha1.ModelRouter{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName + "-nomodels",
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.ModelRouterSpec{
					Type:     validType,
					Port:     testPort,
					AiModels: []gatewayv1alpha1.AiModel{}, // Empty models
				},
			}
			Expect(k8sClient.Create(ctx, modelRouter)).To(Succeed())
		})

		AfterEach(func() {
			By("Cleaning up the no-models ModelRouter resource")
			resource := &gatewayv1alpha1.ModelRouter{}
			if err := k8sClient.Get(ctx, namespacedName, resource); err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should fail with ModelRouterDiscoveryFailed condition", func() {
			By("Reconciling the resource with no models")
			reconciler := &ModelRouterReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: namespacedName,
			})

			// Should handle the error gracefully
			Expect(err).ToNot(HaveOccurred())

			// Check that the appropriate condition is set - should fail at config generation
			checkStatusConditions(ctx, namespacedName, false)
		})
	})
})

// checkConfigMapReconciled verifies that the ConfigMap was created with correct litellm configuration
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
	Expect(configMap.Labels).To(HaveKeyWithValue("type", "litellm"))

	By("Verifying ConfigMap contains litellm configuration")
	Expect(configMap.Data).To(HaveKey("config.yaml"))
	configContent := configMap.Data["config.yaml"]
	Expect(configContent).To(ContainSubstring("model_list"))
	Expect(configContent).To(ContainSubstring("gpt-3.5-turbo"))
	Expect(configContent).To(ContainSubstring("gpt-4"))
	Expect(configContent).To(ContainSubstring("claude-3-opus"))

	By("Verifying ConfigMap has owner reference")
	Expect(configMap.OwnerReferences).To(HaveLen(1))
	Expect(configMap.OwnerReferences[0].Kind).To(Equal("ModelRouter"))
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
	Expect(deployment.Labels).To(HaveKeyWithValue("type", "litellm"))

	By("Verifying Deployment has correct replicas")
	Expect(deployment.Spec.Replicas).NotTo(BeNil())
	Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))

	By("Verifying Deployment has correct selector")
	Expect(deployment.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app", namespacedName.Name))

	By("Verifying Deployment pod template")
	Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app", namespacedName.Name))
	Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("type", "litellm"))

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
	Expect(deployment.OwnerReferences[0].Kind).To(Equal("ModelRouter"))
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
	Expect(service.Labels).To(HaveKeyWithValue("type", "litellm"))

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
	Expect(service.OwnerReferences[0].Kind).To(Equal("ModelRouter"))
	Expect(service.OwnerReferences[0].Name).To(Equal(namespacedName.Name))
	Expect(*service.OwnerReferences[0].Controller).To(BeTrue())
}

// checkStatusConditions verifies that the appropriate status conditions are set
func checkStatusConditions(ctx context.Context, namespacedName types.NamespacedName, shouldBeReady bool) {
	By("Verifying ModelRouter status conditions")
	modelRouter := &gatewayv1alpha1.ModelRouter{}
	err := k8sClient.Get(ctx, namespacedName, modelRouter)
	Expect(err).NotTo(HaveOccurred())

	if shouldBeReady {
		// Should have successful conditions
		configuredCondition := findCondition(modelRouter.Status.Conditions, "ModelRouterConfigured")
		Expect(configuredCondition).NotTo(BeNil())
		Expect(configuredCondition.Status).To(Equal(metav1.ConditionTrue))

		readyCondition := findCondition(modelRouter.Status.Conditions, "ModelRouterReady")
		Expect(readyCondition).NotTo(BeNil())
		Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))
	} else {
		// Should have failure conditions
		conditions := modelRouter.Status.Conditions
		Expect(len(conditions)).To(BeNumerically(">", 0))

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

// checkResourcesNotCreated verifies that resources were not created for invalid config
func checkResourcesNotCreated(ctx context.Context, namespacedName types.NamespacedName) {
	By("Verifying Deployment was not created")
	deployment := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, namespacedName, deployment)
	Expect(errors.IsNotFound(err)).To(BeTrue())

	By("Verifying Service was not created")
	service := &corev1.Service{}
	err = k8sClient.Get(ctx, namespacedName, service)
	Expect(errors.IsNotFound(err)).To(BeTrue())

	By("Verifying ConfigMap was not created")
	configMap := &corev1.ConfigMap{}
	configMapName := namespacedName.Name + "-config"
	err = k8sClient.Get(ctx, types.NamespacedName{
		Name:      configMapName,
		Namespace: namespacedName.Namespace,
	}, configMap)
	Expect(errors.IsNotFound(err)).To(BeTrue())
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
