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

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/litellm"
)

const (
	aiGatewayClassName = "litellm"
)

var _ = Describe("AiGateway Controller", func() {
	const (
		testPort      int32 = 8000
		aiGatewayName       = "test-aigateway"
		testNamespace       = "default"
	)

	ctx := context.Background()

	Context("When reconciling a valid AiGateway resource", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var gatewayNamespacedName types.NamespacedName
		var classNamespacedName types.NamespacedName

		BeforeEach(func() {
			gatewayNamespacedName = types.NamespacedName{Name: aiGatewayName, Namespace: testNamespace}
			classNamespacedName = types.NamespacedName{Name: aiGatewayClassName}

			createDefaultClass(classNamespacedName)

			By("Creating a valid AiGateway resource")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      aiGatewayName,
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
			cleanupAiGateway(gatewayNamespacedName)
			cleanupAiGatewayClass(classNamespacedName)
		})

		It("should successfully reconcile and create all required resources", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: gatewayNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Check all resources were created successfully
			checkConfigMapReconciled(ctx, gatewayNamespacedName)
			checkDeploymentReconciled(ctx, gatewayNamespacedName)
			checkServiceReconciled(ctx, gatewayNamespacedName)

			By("simulating deployment rollout and reconciling again so Ready flips True")
			markDeploymentRolledOut(gatewayNamespacedName.Name, gatewayNamespacedName.Namespace)
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayNamespacedName})
			Expect(err).NotTo(HaveOccurred())
			checkStatusConditions(ctx, gatewayNamespacedName, true)
		})
	})

	Context("When reconciling an AiGateway with env variables", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var gatewayNamespacedName types.NamespacedName
		var classNamespacedName types.NamespacedName

		BeforeEach(func() {
			gatewayNamespacedName = types.NamespacedName{
				Name:      "test-gateway-with-env",
				Namespace: testNamespace,
			}
			classNamespacedName = types.NamespacedName{Name: aiGatewayClassName}

			createDefaultClass(classNamespacedName)

			By("Creating an AiGateway with env variables")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo", Provider: "openai"},
					},
					Env: []corev1.EnvVar{
						{Name: "FAVORITE_COLOR", Value: "blue"},
						{Name: "FOO", Value: "bar"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayNamespacedName)
			cleanupAiGatewayClass(classNamespacedName)
		})

		It("should pass environment variables to the Deployment", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: gatewayNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying Deployment contains environment variables")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, gatewayNamespacedName, deployment)
			Expect(err).NotTo(HaveOccurred())

			container := deployment.Spec.Template.Spec.Containers[0]
			envVars := container.Env

			By("Checking FAVORITE_COLOR environment variable")
			favoriteColorFound := false
			for _, env := range envVars {
				if env.Name == "FAVORITE_COLOR" {
					Expect(env.Value).To(Equal("blue"))
					favoriteColorFound = true
					break
				}
			}
			Expect(favoriteColorFound).To(BeTrue(), "FAVORITE_COLOR environment variable should be present")

			By("Checking FOO environment variable")
			fooFound := false
			for _, env := range envVars {
				if env.Name == "FOO" {
					Expect(env.Value).To(Equal("bar"))
					fooFound = true
					break
				}
			}
			Expect(fooFound).To(BeTrue(), "FOO environment variable should be present")
		})
	})

	Context("When reconciling an AiGateway with envFrom", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var gatewayNamespacedName types.NamespacedName
		var classNamespacedName types.NamespacedName
		var configMapName types.NamespacedName

		BeforeEach(func() {
			gatewayNamespacedName = types.NamespacedName{Name: "test-gateway-with-envfrom", Namespace: testNamespace}
			classNamespacedName = types.NamespacedName{Name: aiGatewayClassName}
			configMapName = types.NamespacedName{Name: "test-config", Namespace: testNamespace}

			createDefaultClass(classNamespacedName)

			By("Creating a ConfigMap for envFrom")
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName.Name,
					Namespace: testNamespace,
				},
				Data: map[string]string{
					"TEST_KEY": "test-value",
				},
			}
			Expect(k8sClient.Create(ctx, configMap)).To(Succeed())

			By("Creating an AiGateway with envFrom")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo", Provider: "openai"},
					},
					EnvFrom: []corev1.EnvFromSource{
						{
							ConfigMapRef: &corev1.ConfigMapEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: configMapName.Name,
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayNamespacedName)
			cleanupConfigMap(configMapName)
			cleanupAiGatewayClass(classNamespacedName)
		})

		It("should pass envFrom to the Deployment", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: gatewayNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying Deployment contains envFrom")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, gatewayNamespacedName, deployment)
			Expect(err).NotTo(HaveOccurred())

			container := deployment.Spec.Template.Spec.Containers[0]
			envFrom := container.EnvFrom

			By("Checking envFrom ConfigMapRef")
			configMapRefFound := false
			for _, envFromSource := range envFrom {
				if envFromSource.ConfigMapRef != nil && envFromSource.ConfigMapRef.Name == "test-config" {
					configMapRefFound = true
					break
				}
			}
			Expect(configMapRefFound).To(BeTrue(), "envFrom ConfigMapRef should be present")
		})
	})

	Context("When reconciling an AiGateway with commonMetadata", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var gatewayNamespacedName types.NamespacedName
		var classNamespacedName types.NamespacedName

		BeforeEach(func() {
			gatewayNamespacedName = types.NamespacedName{Name: "test-gateway-with-common-metadata", Namespace: testNamespace}
			classNamespacedName = types.NamespacedName{Name: aiGatewayClassName}

			createDefaultClass(classNamespacedName)

			By("Creating an AiGateway with commonMetadata")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo", Provider: "openai"},
					},
					CommonMetadata: &gatewayv1alpha1.EmbeddedMetadata{
						Labels: map[string]string{
							"team":        "platform",
							"environment": "prod",
						},
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayNamespacedName)
			cleanupAiGatewayClass(classNamespacedName)
		})

		It("should propagate commonMetadata labels and annotations to Deployment and Service", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: gatewayNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying Deployment has commonMetadata labels")
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, gatewayNamespacedName, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Labels).To(HaveKeyWithValue("team", "platform"))
			Expect(deployment.Labels).To(HaveKeyWithValue("environment", "prod"))
			Expect(deployment.Labels).To(HaveKeyWithValue("app", gatewayNamespacedName.Name))

			By("Verifying Deployment has commonMetadata annotations")
			Expect(deployment.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))

			By("Verifying pod template has commonMetadata labels")
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("team", "platform"))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("environment", "prod"))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app", gatewayNamespacedName.Name))

			By("Verifying pod template has commonMetadata annotations")
			Expect(deployment.Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
			Expect(deployment.Spec.Template.Annotations).To(HaveKey("gateway.agentic-layer.ai/config-hash"))

			By("Verifying Service has commonMetadata labels")
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, gatewayNamespacedName, service)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Labels).To(HaveKeyWithValue("team", "platform"))
			Expect(service.Labels).To(HaveKeyWithValue("environment", "prod"))
			Expect(service.Labels).To(HaveKeyWithValue("app", gatewayNamespacedName.Name))

			By("Verifying Service has commonMetadata annotations")
			Expect(service.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
		})
	})

	Context("When reconciling an AiGateway with podMetadata", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var gatewayNamespacedName types.NamespacedName
		var classNamespacedName types.NamespacedName

		BeforeEach(func() {
			gatewayNamespacedName = types.NamespacedName{Name: "test-gateway-with-pod-metadata", Namespace: testNamespace}
			classNamespacedName = types.NamespacedName{Name: aiGatewayClassName}

			createDefaultClass(classNamespacedName)

			By("Creating an AiGateway with commonMetadata and podMetadata")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo", Provider: "openai"},
					},
					CommonMetadata: &gatewayv1alpha1.EmbeddedMetadata{
						Labels: map[string]string{
							"team": "platform",
						},
						Annotations: map[string]string{
							"prometheus.io/scrape": "true",
						},
					},
					PodMetadata: &gatewayv1alpha1.EmbeddedMetadata{
						Labels: map[string]string{
							"sidecar": "enabled",
						},
						Annotations: map[string]string{
							"sidecar.istio.io/inject": "true",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayNamespacedName)
			cleanupAiGatewayClass(classNamespacedName)
		})

		It("should apply podMetadata only to pod template, not to Deployment or Service ObjectMeta", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: gatewayNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, gatewayNamespacedName, deployment)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying Deployment ObjectMeta does NOT have podMetadata labels")
			Expect(deployment.Labels).NotTo(HaveKey("sidecar"))

			By("Verifying Deployment ObjectMeta does NOT have podMetadata annotations")
			Expect(deployment.Annotations).NotTo(HaveKey("sidecar.istio.io/inject"))

			By("Verifying pod template has both commonMetadata and podMetadata labels")
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("team", "platform"))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("sidecar", "enabled"))
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app", gatewayNamespacedName.Name))

			By("Verifying pod template has both commonMetadata and podMetadata annotations")
			Expect(deployment.Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
			Expect(deployment.Spec.Template.Annotations).To(HaveKeyWithValue("sidecar.istio.io/inject", "true"))

			By("Verifying Service does NOT have podMetadata labels")
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, gatewayNamespacedName, service)
			Expect(err).NotTo(HaveOccurred())
			Expect(service.Labels).NotTo(HaveKey("sidecar"))

			By("Verifying Service does NOT have podMetadata annotations")
			Expect(service.Annotations).NotTo(HaveKey("sidecar.istio.io/inject"))
		})
	})

	Context("When reconciling an AiGateway with Presidio guardrails", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var gatewayNamespacedName types.NamespacedName
		var classNamespacedName types.NamespacedName
		var guardNamespacedName types.NamespacedName
		var providerNamespacedName types.NamespacedName

		BeforeEach(func() {
			gatewayNamespacedName = types.NamespacedName{Name: "test-gateway-guardrails", Namespace: testNamespace}
			classNamespacedName = types.NamespacedName{Name: aiGatewayClassName}
			guardNamespacedName = types.NamespacedName{Name: "pii-guard", Namespace: testNamespace}
			providerNamespacedName = types.NamespacedName{Name: "presidio-provider", Namespace: testNamespace}

			createDefaultClass(classNamespacedName)

			By("Creating a GuardrailProvider with presidio-api type")
			provider := &gatewayv1alpha1.GuardrailProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.GuardrailProviderSpec{
					Type: "presidio-api",
					Presidio: &gatewayv1alpha1.PresidioProviderConfig{
						BaseUrl: "http://presidio.example.com:80",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			By("Creating a Guard referencing the GuardrailProvider")
			guard := &gatewayv1alpha1.Guard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      guardNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.GuardSpec{
					Mode:        []gatewayv1alpha1.GuardMode{gatewayv1alpha1.GuardModePreCall},
					Description: "PII masking via Presidio",
					ProviderRef: corev1.ObjectReference{
						Name:      providerNamespacedName.Name,
						Namespace: testNamespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, guard)).To(Succeed())

			By("Creating an AiGateway referencing the Guard")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo", Provider: "openai"},
					},
					Guardrails: []corev1.ObjectReference{
						{Name: guardNamespacedName.Name, Namespace: testNamespace},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayNamespacedName)
			cleanupAiGatewayClass(classNamespacedName)

			By("Cleaning up the Guard resource")
			guard := &gatewayv1alpha1.Guard{}
			if err := k8sClient.Get(ctx, guardNamespacedName, guard); err == nil {
				Expect(k8sClient.Delete(ctx, guard)).To(Succeed())
			}

			By("Cleaning up the GuardrailProvider resource")
			provider := &gatewayv1alpha1.GuardrailProvider{}
			if err := k8sClient.Get(ctx, providerNamespacedName, provider); err == nil {
				Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
			}
		})

		It("should include presidio guardrail config in the generated LiteLLM configuration", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: gatewayNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying ConfigMap contains guardrail configuration")
			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      gatewayNamespacedName.Name + "-config",
				Namespace: testNamespace,
			}, configMap)).To(Succeed())

			configContent := configMap.Data["config.yaml"]
			Expect(configContent).To(ContainSubstring("guardrails"))
			Expect(configContent).To(ContainSubstring("pii-guard"))
			Expect(configContent).To(ContainSubstring("guardrail: presidio"))
			Expect(configContent).To(ContainSubstring("pre_call"))
			Expect(configContent).To(ContainSubstring("default_on: true"))
			Expect(configContent).To(ContainSubstring("presidio_analyzer_api_base: http://presidio.example.com:80"))
			Expect(configContent).To(ContainSubstring("presidio_anonymizer_api_base: http://presidio.example.com:80"))
			Expect(configContent).To(ContainSubstring("output_parse_pii: true"))
		})
	})

	Context("When reconciling an AiGateway with Presidio guardrails with multiple modes", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var gatewayNamespacedName types.NamespacedName
		var classNamespacedName types.NamespacedName
		var guardNamespacedName types.NamespacedName
		var providerNamespacedName types.NamespacedName

		BeforeEach(func() {
			gatewayNamespacedName = types.NamespacedName{Name: "test-gateway-guardrails-multi-mode", Namespace: testNamespace}
			classNamespacedName = types.NamespacedName{Name: aiGatewayClassName}
			guardNamespacedName = types.NamespacedName{Name: "pii-guard-multi-mode", Namespace: testNamespace}
			providerNamespacedName = types.NamespacedName{Name: "presidio-provider-multi-mode", Namespace: testNamespace}

			createDefaultClass(classNamespacedName)

			By("Creating a GuardrailProvider with presidio-api type")
			provider := &gatewayv1alpha1.GuardrailProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.GuardrailProviderSpec{
					Type: "presidio-api",
					Presidio: &gatewayv1alpha1.PresidioProviderConfig{
						BaseUrl: "http://presidio.example.com:3000",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			By("Creating a Guard with multiple modes referencing the GuardrailProvider")
			guard := &gatewayv1alpha1.Guard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      guardNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.GuardSpec{
					Mode: []gatewayv1alpha1.GuardMode{
						gatewayv1alpha1.GuardModePreCall,
						gatewayv1alpha1.GuardModePostCall,
					},
					ProviderRef: corev1.ObjectReference{
						Name:      providerNamespacedName.Name,
						Namespace: testNamespace,
					},
				},
			}
			Expect(k8sClient.Create(ctx, guard)).To(Succeed())

			By("Creating an AiGateway referencing the Guard")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo", Provider: "openai"},
					},
					Guardrails: []corev1.ObjectReference{
						{Name: guardNamespacedName.Name, Namespace: testNamespace},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayNamespacedName)
			cleanupAiGatewayClass(classNamespacedName)

			By("Cleaning up the Guard resource")
			guard := &gatewayv1alpha1.Guard{}
			if err := k8sClient.Get(ctx, guardNamespacedName, guard); err == nil {
				Expect(k8sClient.Delete(ctx, guard)).To(Succeed())
			}

			By("Cleaning up the GuardrailProvider resource")
			provider := &gatewayv1alpha1.GuardrailProvider{}
			if err := k8sClient.Get(ctx, providerNamespacedName, provider); err == nil {
				Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
			}
		})

		It("should include all modes in the guardrail config", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: gatewayNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying ConfigMap contains guardrail configuration with both modes")
			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      gatewayNamespacedName.Name + "-config",
				Namespace: testNamespace,
			}, configMap)).To(Succeed())

			configContent := configMap.Data["config.yaml"]
			Expect(configContent).To(ContainSubstring("guardrails"))
			Expect(configContent).To(ContainSubstring("pii-guard-multi-mode"))
			Expect(configContent).To(ContainSubstring("guardrail: presidio"))
			Expect(configContent).To(ContainSubstring("pre_call"))
			Expect(configContent).To(ContainSubstring("post_call"))
			Expect(configContent).To(ContainSubstring("default_on: true"))
			Expect(configContent).To(ContainSubstring("presidio_analyzer_api_base: http://presidio.example.com:3000"))
		})
	})

	Context("When reconciling an AiGateway with Presidio guard-level config", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		var gatewayNamespacedName types.NamespacedName
		var classNamespacedName types.NamespacedName
		var guardNamespacedName types.NamespacedName
		var providerNamespacedName types.NamespacedName

		BeforeEach(func() {
			gatewayNamespacedName = types.NamespacedName{Name: "test-gateway-guardrails-config", Namespace: testNamespace}
			classNamespacedName = types.NamespacedName{Name: aiGatewayClassName}
			guardNamespacedName = types.NamespacedName{Name: "pii-guard-config", Namespace: testNamespace}
			providerNamespacedName = types.NamespacedName{Name: "presidio-provider-config", Namespace: testNamespace}

			createDefaultClass(classNamespacedName)

			By("Creating a GuardrailProvider with presidio-api type")
			provider := &gatewayv1alpha1.GuardrailProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      providerNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.GuardrailProviderSpec{
					Type: "presidio-api",
					Presidio: &gatewayv1alpha1.PresidioProviderConfig{
						BaseUrl: "http://presidio.example.com:80",
					},
				},
			}
			Expect(k8sClient.Create(ctx, provider)).To(Succeed())

			By("Creating a Guard with presidio-specific config")
			guard := &gatewayv1alpha1.Guard{
				ObjectMeta: metav1.ObjectMeta{
					Name:      guardNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.GuardSpec{
					Mode:        []gatewayv1alpha1.GuardMode{gatewayv1alpha1.GuardModePreCall},
					Description: "PII masking with entities and threshold",
					ProviderRef: corev1.ObjectReference{
						Name:      providerNamespacedName.Name,
						Namespace: testNamespace,
					},
					Presidio: &gatewayv1alpha1.PresidioGuardConfig{
						EntityActions: map[string]string{
							"PHONE_NUMBER":  "MASK",
							"EMAIL_ADDRESS": "MASK",
						},
						Language: "de",
						ScoreThresholds: map[string]string{
							"ALL": "0.7",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, guard)).To(Succeed())

			By("Creating an AiGateway referencing the Guard")
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayNamespacedName.Name,
					Namespace: testNamespace,
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port: testPort,
					AiModels: []gatewayv1alpha1.AiModel{
						{Name: "gpt-3.5-turbo", Provider: "openai"},
					},
					Guardrails: []corev1.ObjectReference{
						{Name: guardNamespacedName.Name, Namespace: testNamespace},
					},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayNamespacedName)
			cleanupAiGatewayClass(classNamespacedName)

			By("Cleaning up the Guard resource")
			guard := &gatewayv1alpha1.Guard{}
			if err := k8sClient.Get(ctx, guardNamespacedName, guard); err == nil {
				Expect(k8sClient.Delete(ctx, guard)).To(Succeed())
			}

			By("Cleaning up the GuardrailProvider resource")
			provider := &gatewayv1alpha1.GuardrailProvider{}
			if err := k8sClient.Get(ctx, providerNamespacedName, provider); err == nil {
				Expect(k8sClient.Delete(ctx, provider)).To(Succeed())
			}
		})

		It("should map entities to pii_entities_config and score threshold to presidio_score_thresholds", func() {
			By("Reconciling the created resource")
			reconciler := &AiGatewayReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: gatewayNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			By("Verifying ConfigMap contains correct LiteLLM field names")
			configMap := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      gatewayNamespacedName.Name + "-config",
				Namespace: testNamespace,
			}, configMap)).To(Succeed())

			configContent := configMap.Data["config.yaml"]
			Expect(configContent).To(ContainSubstring("pii_entities_config"))
			Expect(configContent).To(ContainSubstring("PHONE_NUMBER: MASK"))
			Expect(configContent).To(ContainSubstring("EMAIL_ADDRESS: MASK"))
			Expect(configContent).To(ContainSubstring("presidio_score_thresholds"))
			Expect(configContent).To(ContainSubstring("ALL: \"0.7\""))
			Expect(configContent).To(ContainSubstring("presidio_language: de"))
			// Verify old field names are NOT present
			Expect(configContent).NotTo(ContainSubstring("presidio_entities:"))
			Expect(configContent).NotTo(ContainSubstring("presidio_score_threshold:"))
		})
	})

	Context("When reconciling an AiGateway with a config-patch annotation pointing at a missing ConfigMap", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		gatewayKey := types.NamespacedName{Name: "test-gateway-patch-missing", Namespace: testNamespace}
		classKey := types.NamespacedName{Name: aiGatewayClassName}

		BeforeEach(func() {
			createDefaultClass(classKey)
			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayKey.Name,
					Namespace: testNamespace,
					Annotations: map[string]string{
						litellm.ConfigPatchAnnotation: "missing-cm",
					},
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port:     testPort,
					AiModels: []gatewayv1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayKey)
			cleanupAiGatewayClass(classKey)
		})

		It("flips Configured/Ready to False with ConfigPatchInvalid", func() {
			rec := &AiGatewayReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayKey})
			Expect(err).NotTo(HaveOccurred())

			refreshed := &gatewayv1alpha1.AiGateway{}
			Expect(k8sClient.Get(ctx, gatewayKey, refreshed)).To(Succeed())
			cond := findCondition(refreshed.Status.Conditions, "AiGatewayConfigured")
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("ConfigPatchInvalid"))
			Expect(cond.Message).To(ContainSubstring("not found"))

			ready := findCondition(refreshed.Status.Conditions, "AiGatewayReady")
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionFalse))
			Expect(ready.Reason).To(Equal("ConfigPatchInvalid"))
		})
	})

	Context("When reconciling an AiGateway with a valid patch ConfigMap", func() {
		var aiGateway *gatewayv1alpha1.AiGateway
		gatewayKey := types.NamespacedName{Name: "test-gateway-patch-ok", Namespace: testNamespace}
		classKey := types.NamespacedName{Name: aiGatewayClassName}
		patchCMKey := types.NamespacedName{Name: "ai-patch", Namespace: testNamespace}

		BeforeEach(func() {
			createDefaultClass(classKey)

			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: patchCMKey.Name, Namespace: patchCMKey.Namespace},
				Data: map[string]string{
					"patch.yaml": "router_settings:\n  routing_strategy: usage-based-routing-v2\n",
				},
			})).To(Succeed())

			aiGateway = &gatewayv1alpha1.AiGateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayKey.Name,
					Namespace: testNamespace,
					Annotations: map[string]string{
						litellm.ConfigPatchAnnotation: patchCMKey.Name,
					},
				},
				Spec: gatewayv1alpha1.AiGatewaySpec{
					Port:     testPort,
					AiModels: []gatewayv1alpha1.AiModel{{Name: "gpt-4", Provider: "openai"}},
				},
			}
			Expect(k8sClient.Create(ctx, aiGateway)).To(Succeed())
		})

		AfterEach(func() {
			cleanupAiGateway(gatewayKey)
			cleanupAiGatewayClass(classKey)
			cleanupConfigMap(patchCMKey)
		})

		It("merges router_settings into the operator-owned ConfigMap", func() {
			rec := &AiGatewayReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayKey})
			Expect(err).NotTo(HaveOccurred())

			ownedCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: gatewayKey.Name + "-config", Namespace: testNamespace}, ownedCM)).To(Succeed())
			yamlStr := ownedCM.Data["config.yaml"]
			Expect(yamlStr).To(ContainSubstring("model_list"))
			Expect(yamlStr).To(ContainSubstring("openai/gpt-4"))
			Expect(yamlStr).To(ContainSubstring("router_settings"))
			Expect(yamlStr).To(ContainSubstring("routing_strategy: usage-based-routing-v2"))
		})

		It("rolls the deployment when the patch ConfigMap changes", func() {
			rec := &AiGatewayReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayKey})
			Expect(err).NotTo(HaveOccurred())

			before := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, gatewayKey, before)).To(Succeed())
			beforeHash := before.Spec.Template.Annotations["gateway.agentic-layer.ai/config-hash"]
			Expect(beforeHash).ToNot(BeEmpty())

			patchCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, patchCMKey, patchCM)).To(Succeed())
			patchCM.Data["patch.yaml"] = "router_settings:\n  routing_strategy: simple-shuffle\n"
			Expect(k8sClient.Update(ctx, patchCM)).To(Succeed())

			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayKey})
			Expect(err).NotTo(HaveOccurred())

			after := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, gatewayKey, after)).To(Succeed())
			afterHash := after.Spec.Template.Annotations["gateway.agentic-layer.ai/config-hash"]
			Expect(afterHash).ToNot(BeEmpty(), "config-hash annotation must be set after second reconcile")
			Expect(afterHash).ToNot(Equal(beforeHash), "config-hash should change when patch.yaml changes")

			afterCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: gatewayKey.Name + "-config", Namespace: testNamespace}, afterCM)).To(Succeed())
			Expect(afterCM.Data["config.yaml"]).To(ContainSubstring("simple-shuffle"))
		})

		It("drops the patch when the annotation is removed", func() {
			rec := &AiGatewayReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			_, err := rec.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayKey})
			Expect(err).NotTo(HaveOccurred())

			ownedCMBefore := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: gatewayKey.Name + "-config", Namespace: testNamespace}, ownedCMBefore)).To(Succeed())
			Expect(ownedCMBefore.Data["config.yaml"]).To(ContainSubstring("router_settings"), "patch should be present before annotation removal")

			refreshed := &gatewayv1alpha1.AiGateway{}
			Expect(k8sClient.Get(ctx, gatewayKey, refreshed)).To(Succeed())
			delete(refreshed.Annotations, litellm.ConfigPatchAnnotation)
			Expect(k8sClient.Update(ctx, refreshed)).To(Succeed())

			_, err = rec.Reconcile(ctx, reconcile.Request{NamespacedName: gatewayKey})
			Expect(err).NotTo(HaveOccurred())

			ownedCM := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: gatewayKey.Name + "-config", Namespace: testNamespace}, ownedCM)).To(Succeed())
			Expect(ownedCM.Data["config.yaml"]).NotTo(ContainSubstring("router_settings"))
		})
	})

})

func cleanupAiGatewayClass(namespacedName types.NamespacedName) {
	By("Cleaning up the AiGatewayClass")
	classResource := &gatewayv1alpha1.AiGatewayClass{}

	if err := k8sClient.Get(ctx, namespacedName, classResource); err == nil {
		Expect(k8sClient.Delete(ctx, classResource)).To(Succeed())
	}
}

func cleanupAiGateway(namespacedName types.NamespacedName) {
	By("Cleaning up the AiGateway resource")
	resource := &gatewayv1alpha1.AiGateway{}

	if err := k8sClient.Get(ctx, namespacedName, resource); err == nil {
		Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
	}
}

func cleanupConfigMap(namespacedName types.NamespacedName) {
	By("Cleaning up the ConfigMap")
	configMap := &corev1.ConfigMap{}

	if err := k8sClient.Get(ctx, namespacedName, configMap); err == nil {
		Expect(k8sClient.Delete(ctx, configMap)).To(Succeed())
	}
}

func createDefaultClass(namespacedName types.NamespacedName) {
	By("Creating a default AiGatewayClass")
	var aiGatewayClass = &gatewayv1alpha1.AiGatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespacedName.Name,
			Annotations: map[string]string{
				"aigatewayclass.kubernetes.io/is-default-class": "true",
			},
		},
		Spec: gatewayv1alpha1.AiGatewayClassSpec{
			Controller: ControllerName,
		},
	}
	Expect(k8sClient.Create(ctx, aiGatewayClass)).To(Succeed())
}

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
	Expect(container.VolumeMounts).To(HaveLen(2))
	Expect(container.VolumeMounts[0].Name).To(Equal("config"))
	Expect(container.VolumeMounts[0].MountPath).To(Equal("/app/config"))

	By("Verifying Deployment has prometheus multiproc volume mount")
	Expect(container.VolumeMounts[1].Name).To(Equal("prometheus-multiproc"))
	Expect(container.VolumeMounts[1].MountPath).To(Equal("/prometheus_multiproc"))

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
