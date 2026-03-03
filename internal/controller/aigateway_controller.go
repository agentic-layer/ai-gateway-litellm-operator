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
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/controller/utils"
	"github.com/agentic-layer/ai-gateway-litellm/internal/equality"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	litellmImage = "ghcr.io/berriai/litellm:v1.81.12-stable.2"
)

// Status condition types
const (
	// AiGatewayConfigured indicates if the AiGateway configuration is valid
	AiGatewayConfigured = "AiGatewayConfigured"

	// AiGatewayReady indicates if the AiGateway is ready to serve traffic
	AiGatewayReady = "AiGatewayReady"
)

// Configuration constants
const (
	// DefaultRequestTimeout is the default timeout for LiteLLM requests in seconds
	DefaultRequestTimeout = 600

	// liteLLMContainerName is the name of the LiteLLM container in the deployment
	liteLLMContainerName = "litellm"
)

// Secret and API key constants
const (
	// DefaultSecretName is the default name for the API keys secret
	DefaultSecretName = "api-key-secrets"
)

// Condition reasons
const (
	// ReasonConfigurationApplied indicates successful configuration application
	ReasonConfigurationApplied = "ConfigurationApplied"

	// ReasonAiGatewayReady indicates AiGateway is ready
	ReasonAiGatewayReady = "AiGatewayReady"
)

const ControllerName = "aigateway.agentic-layer.ai/ai-gateway-litellm-controller"

// LiteLLMConfig configuration structs
type LiteLLMConfig struct {
	ModelList       []ModelConfig   `yaml:"model_list"`
	LiteLLMSettings LiteLLMSettings `yaml:"litellm_settings,omitempty"`
}

type ModelConfig struct {
	ModelName     string        `yaml:"model_name"`
	LiteLLMParams LiteLLMParams `yaml:"litellm_params"`
}

type LiteLLMParams struct {
	Model  string `yaml:"model"`
	ApiKey string `yaml:"api_key,omitempty"`
}

type LiteLLMSettings struct {
	RequestTimeout int      `yaml:"request_timeout,omitempty"`
	Callbacks      []string `yaml:"callbacks,omitempty"`
}

// AiGatewayReconciler reconciles an AiGateway object
type AiGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=aigateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=aigateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=aigateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=aigatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

func (r *AiGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AiGateway instance that triggered the reconciliation
	var aiGateway gatewayv1alpha1.AiGateway
	if err := r.Get(ctx, req.NamespacedName, &aiGateway); err != nil {
		if errors.IsNotFound(err) {
			log.Info("AiGateway resource not found")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AiGateway")
		return ctrl.Result{}, err
	}

	if !r.shouldProcessAiGateway(ctx, &aiGateway) {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling AiGateway", "name", aiGateway.Name, "namespace", aiGateway.Namespace)

	// Initialize status if needed
	if aiGateway.Status.Conditions == nil {
		aiGateway.Status.Conditions = []metav1.Condition{}
	}

	// Step 1: Generate configuration
	configData, configHash, err := r.generateAiGatewayConfig(ctx, &aiGateway)
	if err != nil {
		log.Error(err, "Failed to generate configuration")
		r.updateCondition(&aiGateway, AiGatewayConfigured, metav1.ConditionFalse,
			"ConfigGenerationFailed", fmt.Sprintf("Failed to generate config: %v", err))
		r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionFalse,
			"ConfigGenerationFailed", "AiGateway not ready due to config generation failure")
		if err := r.updateStatus(ctx, &aiGateway); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 3: Create/update ConfigMap with configuration
	if err := r.reconcileConfigMap(ctx, &aiGateway, configData); err != nil {
		log.Error(err, "Failed to reconcile ConfigMap")
		r.updateCondition(&aiGateway, AiGatewayConfigured, metav1.ConditionFalse,
			"ConfigMapFailed", fmt.Sprintf("Failed to create/update ConfigMap: %v", err))
		if err := r.updateStatus(ctx, &aiGateway); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 4: Create/update Deployment
	if err := r.reconcileDeployment(ctx, &aiGateway, configHash); err != nil {
		log.Error(err, "Failed to reconcile Deployment")
		r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionFalse,
			"DeploymentFailed", fmt.Sprintf("Failed to create/update Deployment: %v", err))
		if err := r.updateStatus(ctx, &aiGateway); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 5: Create/update Service
	if err := r.reconcileService(ctx, &aiGateway); err != nil {
		log.Error(err, "Failed to reconcile Service")
		r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionFalse,
			"ServiceFailed", fmt.Sprintf("Failed to create/update Service: %v", err))
		if err := r.updateStatus(ctx, &aiGateway); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	r.updateCondition(&aiGateway, AiGatewayConfigured, metav1.ConditionTrue,
		ReasonConfigurationApplied, "AiGateway configuration successfully applied")
	r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionTrue,
		ReasonAiGatewayReady, "AiGateway is ready and serving traffic")

	log.Info("Successfully reconciled AiGateway", "name", aiGateway.Name,
		"aiModels", len(aiGateway.Spec.AiModels), "configHash", configHash[:8])

	if err := r.updateStatus(ctx, &aiGateway); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// shouldProcessAiGateway determines if this controller is responsible for the given AiGateway
func (r *AiGatewayReconciler) shouldProcessAiGateway(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway) bool {
	log := logf.FromContext(ctx)

	// If no className specified, check for default AiGatewayClass
	var aiGatewayClassList gatewayv1alpha1.AiGatewayClassList
	if err := r.List(ctx, &aiGatewayClassList); err != nil {
		log.Info("If we can't list classes, don't process to avoid errors")
		return false
	}

	// Filter aiGatewayClassList to only contain classes with matching controller
	var litellmClasses []gatewayv1alpha1.AiGatewayClass
	for _, aiGatewayClass := range aiGatewayClassList.Items {
		if aiGatewayClass.Spec.Controller == ControllerName {
			litellmClasses = append(litellmClasses, aiGatewayClass)
		}
	}

	// If className is explicitly set, check if it matches any of our managed classes
	aiGatewayClassName := aiGateway.Spec.AiGatewayClassName
	if aiGatewayClassName != "" {
		for _, litellmClass := range litellmClasses {
			if litellmClass.Name == aiGatewayClassName {
				return true
			}
		}
	}

	// Look for AiGatewayClass with default annotation among filtered classes
	for _, litellmClass := range litellmClasses {
		if litellmClass.Annotations["aigatewayclass.kubernetes.io/is-default-class"] == "true" {
			log.Info("Using default AiGatewayClass", "className", litellmClass.Name)
			return true
		}
	}

	return false
}

// generateAiGatewayConfig generates the configuration using the appropriate generator
func (r *AiGatewayReconciler) generateAiGatewayConfig(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway) (string, string, error) {

	log := logf.FromContext(ctx)

	// Build model list with proper provider prefixes and environment variable API keys
	modelList := make([]ModelConfig, len(aiGateway.Spec.AiModels))
	for i, model := range aiGateway.Spec.AiModels {

		modelConfig := ModelConfig{
			ModelName: model.Name,
			LiteLLMParams: LiteLLMParams{
				Model:  fmt.Sprintf("%s/%s", model.Provider, model.Name),
				ApiKey: fmt.Sprintf("os.environ/%s", r.getProviderApiKeyEnvVar(model)),
			},
		}

		modelList[i] = modelConfig
	}

	// Build complete configuration with settings
	config := LiteLLMConfig{
		ModelList: modelList,
		LiteLLMSettings: LiteLLMSettings{
			RequestTimeout: DefaultRequestTimeout,
			// 'callbacks: ["otel"]' is required to send traces to otel after handling incoming requests
			// (see https://docs.litellm.ai/docs/proxy/logging#opentelemetry)
			Callbacks: []string{"otel"},
		},
	}

	// Generate YAML configuration
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal LiteLLM config: %w", err)
	}

	// Generate configuration hash
	hash := sha256.Sum256(configYAML)
	configHash := fmt.Sprintf("%x", hash)[:16]

	log.Info("Generated LiteLLM configuration", "aiGateway", aiGateway.Name, "models", len(aiGateway.Spec.AiModels), "configHash", configHash[:8])

	return string(configYAML), configHash, nil
}

func (r *AiGatewayReconciler) getProviderApiKeyEnvVar(model gatewayv1alpha1.AiModel) string {
	return strings.ToUpper(model.Provider) + "_API_KEY"
}

// reconcileConfigMap creates or updates the ConfigMap containing LiteLLM configuration
func (r *AiGatewayReconciler) reconcileConfigMap(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway, configData string) error {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", aiGateway.Name),
			Namespace: aiGateway.Namespace,
			Labels: map[string]string{
				"app": aiGateway.Name,
			},
		},
		Data: map[string]string{
			"config.yaml": configData,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(aiGateway, configMap, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Check if ConfigMap exists
	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, existing)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating ConfigMap", "name", configMap.Name)
		return r.Create(ctx, configMap)
	} else if err != nil {
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Update if needed using equality utilities
	needsUpdate := existing.Data["config.yaml"] != configData

	if !equality.RequiredLabelsPresent(existing.Labels, configMap.Labels) {
		needsUpdate = true
	}

	if needsUpdate {
		log.Info("Updating ConfigMap", "name", configMap.Name)
		existing.Data = configMap.Data
		// Only update our required labels, preserve others
		if existing.Labels == nil {
			existing.Labels = make(map[string]string)
		}
		for key, value := range configMap.Labels {
			existing.Labels[key] = value
		}
		return r.Update(ctx, existing)
	}

	return nil
}

// reconcileDeployment creates or updates the Deployment for LiteLLM
func (r *AiGatewayReconciler) reconcileDeployment(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway, configHash string) error {
	log := logf.FromContext(ctx)

	replicas := int32(1)
	deploymentLabels := buildResourceLabels(aiGateway)
	deploymentAnnotations := buildResourceAnnotations(aiGateway)
	podTemplateLabels := buildPodTemplateLabels(aiGateway)
	podTemplateAnnotations := buildPodTemplateAnnotations(aiGateway, configHash)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        aiGateway.Name,
			Namespace:   aiGateway.Namespace,
			Labels:      deploymentLabels,
			Annotations: deploymentAnnotations,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": aiGateway.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podTemplateLabels,
					Annotations: podTemplateAnnotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  liteLLMContainerName,
							Image: litellmImage,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: aiGateway.Spec.Port,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/app/config",
									ReadOnly:  true,
								},
							},
							Command: []string{
								"litellm",
								"--config",
								"/app/config/config.yaml",
								"--port",
								strconv.Itoa(int(aiGateway.Spec.Port)),
							},
							Env:     r.buildEnvironmentVariables(aiGateway),
							EnvFrom: aiGateway.Spec.EnvFrom,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("250M"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("400M"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							// LiteLLM health check endpoints: /health/liveliness and /health/readiness
							// Note: LiteLLM uses "liveliness" (not "liveness") in their API
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health/liveliness",
										Port: intstr.FromInt32(aiGateway.Spec.Port),
									},
								},
								InitialDelaySeconds: 15,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health/readiness",
										Port: intstr.FromInt32(aiGateway.Spec.Port),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", aiGateway.Name),
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(aiGateway, deployment, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Check if Deployment exists
	existing := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, existing)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Deployment", "name", deployment.Name)
		return r.Create(ctx, deployment)
	} else if err != nil {
		return fmt.Errorf("failed to get Deployment: %w", err)
	}

	// Update if needed using equality utilities
	needsUpdate := !equality.RequiredLabelsPresent(existing.Labels, deployment.Labels)

	// Check deployment annotations for changes
	if !equality.RequiredLabelsPresent(existing.Annotations, deployment.Annotations) {
		needsUpdate = true
	}

	// Check template labels for changes
	if !equality.RequiredLabelsPresent(existing.Spec.Template.Labels, deployment.Spec.Template.Labels) {
		needsUpdate = true
	}

	// Check template annotations for changes (includes config hash and user-provided annotations)
	if !equality.RequiredLabelsPresent(existing.Spec.Template.Annotations, deployment.Spec.Template.Annotations) {
		needsUpdate = true
	}

	// Check environment variables for changes
	existingContainer := utils.FindContainerByName(existing.Spec.Template.Spec.Containers, liteLLMContainerName)
	desiredContainer := utils.FindContainerByName(deployment.Spec.Template.Spec.Containers, liteLLMContainerName)

	if existingContainer != nil && desiredContainer != nil {
		if !equality.EnvVarsEqual(existingContainer.Env, desiredContainer.Env) {
			needsUpdate = true
		}

		// Check EnvFrom for changes
		if !equality.EnvFromEqual(existingContainer.EnvFrom, desiredContainer.EnvFrom) {
			needsUpdate = true
		}

		// Check image changes
		if existingContainer.Image != desiredContainer.Image {
			needsUpdate = true
		}

		// Check port configuration changes
		existingPorts := existingContainer.Ports
		newPorts := desiredContainer.Ports
		if len(existingPorts) != len(newPorts) {
			needsUpdate = true
		} else if len(existingPorts) > 0 && len(newPorts) > 0 {
			// Check if the main container port changed
			if existingPorts[0].ContainerPort != newPorts[0].ContainerPort {
				needsUpdate = true
			}
		}

		// Check resource requirements changes
		if !equality.ResourceRequirementsEqual(existingContainer.Resources, desiredContainer.Resources) {
			needsUpdate = true
		}

		// Check liveness probe changes
		if !equality.ProbesEqual(existingContainer.LivenessProbe, desiredContainer.LivenessProbe) {
			needsUpdate = true
		}

		// Check readiness probe changes
		if !equality.ProbesEqual(existingContainer.ReadinessProbe, desiredContainer.ReadinessProbe) {
			needsUpdate = true
		}
	}

	if needsUpdate {
		log.Info("Updating Deployment", "name", deployment.Name)
		// Only update our required labels, preserve others
		if existing.Labels == nil {
			existing.Labels = make(map[string]string)
		}
		for key, value := range deployment.Labels {
			existing.Labels[key] = value
		}
		// Update deployment annotations
		if deployment.Annotations != nil {
			if existing.Annotations == nil {
				existing.Annotations = make(map[string]string)
			}
			for key, value := range deployment.Annotations {
				existing.Annotations[key] = value
			}
		}
		// Update template labels and annotations
		if existing.Spec.Template.Labels == nil {
			existing.Spec.Template.Labels = make(map[string]string)
		}
		for key, value := range deployment.Spec.Template.Labels {
			existing.Spec.Template.Labels[key] = value
		}
		if existing.Spec.Template.Annotations == nil {
			existing.Spec.Template.Annotations = make(map[string]string)
		}
		for key, value := range deployment.Spec.Template.Annotations {
			existing.Spec.Template.Annotations[key] = value
		}
		// Update the rest of the spec
		existing.Spec.Replicas = deployment.Spec.Replicas
		existing.Spec.Selector = deployment.Spec.Selector
		existing.Spec.Template.Spec = deployment.Spec.Template.Spec
		return r.Update(ctx, existing)
	}

	return nil
}

// reconcileService creates or updates the Service for LiteLLM
func (r *AiGatewayReconciler) reconcileService(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway) error {
	log := logf.FromContext(ctx)

	serviceLabels := buildResourceLabels(aiGateway)
	serviceAnnotations := buildResourceAnnotations(aiGateway)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        aiGateway.Name,
			Namespace:   aiGateway.Namespace,
			Labels:      serviceLabels,
			Annotations: serviceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": aiGateway.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       aiGateway.Spec.Port,
					TargetPort: intstr.FromInt32(aiGateway.Spec.Port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(aiGateway, service, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Check if Service exists
	existing := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, existing)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Creating Service", "name", service.Name)
		return r.Create(ctx, service)
	} else if err != nil {
		return fmt.Errorf("failed to get Service: %w", err)
	}

	// Update if needed using equality utilities
	needsUpdate := !equality.RequiredLabelsPresent(existing.Labels, service.Labels)

	// Check service annotations for changes
	if !equality.RequiredLabelsPresent(existing.Annotations, service.Annotations) {
		needsUpdate = true
	}

	// Check ports for changes (safe checking)
	if len(existing.Spec.Ports) > 0 && len(service.Spec.Ports) > 0 &&
		existing.Spec.Ports[0].Port != service.Spec.Ports[0].Port {
		needsUpdate = true
	}

	if needsUpdate {
		log.Info("Updating Service", "name", service.Name)
		existing.Spec.Ports = service.Spec.Ports
		// Only update our required labels, preserve others
		if existing.Labels == nil {
			existing.Labels = make(map[string]string)
		}
		for key, value := range service.Labels {
			existing.Labels[key] = value
		}
		// Update service annotations
		if service.Annotations != nil {
			if existing.Annotations == nil {
				existing.Annotations = make(map[string]string)
			}
			for key, value := range service.Annotations {
				existing.Annotations[key] = value
			}
		}
		return r.Update(ctx, existing)
	}

	return nil
}

// buildEnvironmentVariables creates environment variables for the deployment
func (r *AiGatewayReconciler) buildEnvironmentVariables(aiGateway *gatewayv1alpha1.AiGateway) []corev1.EnvVar {
	envMap := make(map[string]corev1.EnvVar)

	r.generateApiKeyEnvVars(aiGateway, envMap)

	// Add environment variables from AiGateway spec
	// User-provided Env Vars override generated Env Vars
	for _, env := range aiGateway.Spec.Env {
		envMap[env.Name] = env
	}

	// Convert map to slice
	envVars := make([]corev1.EnvVar, 0, len(envMap))
	for _, env := range envMap {
		envVars = append(envVars, env)
	}
	return envVars
}

func (r *AiGatewayReconciler) generateApiKeyEnvVars(aiGateway *gatewayv1alpha1.AiGateway, envMap map[string]corev1.EnvVar) {
	// Add API key environment variables for each model
	// We need to determine what API keys are needed based on the models
	apiKeyEnvVars := make(map[string]bool)

	// Collect unique API key environment variables needed
	for _, model := range aiGateway.Spec.AiModels {
		apiKeyEnvVar := r.getProviderApiKeyEnvVar(model)
		if apiKeyEnvVar != "" {
			apiKeyEnvVars[apiKeyEnvVar] = true
		}
	}

	// Add environment variables from secret for each API key
	for envVarName := range apiKeyEnvVars {
		envMap[envVarName] = corev1.EnvVar{
			Name: envVarName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: DefaultSecretName,
					},
					Key:      envVarName,
					Optional: &[]bool{true}[0], // Make optional so deployment doesn't fail if secret missing
				},
			},
		}
	}
}

// updateCondition updates or adds a condition to the AiGateway status
func (r *AiGatewayReconciler) updateCondition(aiGateway *gatewayv1alpha1.AiGateway, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}

	// Find existing condition
	for i, existingCondition := range aiGateway.Status.Conditions {
		if existingCondition.Type == conditionType {
			// Update existing condition
			aiGateway.Status.Conditions[i] = condition
			aiGateway.Status.Conditions[i].LastTransitionTime = metav1.Now()
			return
		}
	}

	// Add new condition
	condition.LastTransitionTime = metav1.Now()
	aiGateway.Status.Conditions = append(aiGateway.Status.Conditions, condition)
}

// updateStatus updates the AiGateway status
func (r *AiGatewayReconciler) updateStatus(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway) error {
	if statusErr := r.Status().Update(ctx, aiGateway); statusErr != nil {
		logf.FromContext(ctx).Error(statusErr, "Failed to update AiGateway status")
		return statusErr
	}
	return nil
}

// buildResourceLabels builds labels for Deployment/Service ObjectMeta:
// user commonMetadata.Labels first, then the managed label ("app") always takes precedence.
func buildResourceLabels(aiGateway *gatewayv1alpha1.AiGateway) map[string]string {
	labels := make(map[string]string)
	if aiGateway.Spec.CommonMetadata != nil {
		for k, v := range aiGateway.Spec.CommonMetadata.Labels {
			labels[k] = v
		}
	}
	labels["app"] = aiGateway.Name
	return labels
}

// buildResourceAnnotations builds annotations for Deployment/Service ObjectMeta from commonMetadata.
// Returns nil if no annotations are configured.
func buildResourceAnnotations(aiGateway *gatewayv1alpha1.AiGateway) map[string]string {
	if aiGateway.Spec.CommonMetadata == nil || len(aiGateway.Spec.CommonMetadata.Annotations) == 0 {
		return nil
	}
	annotations := make(map[string]string)
	for k, v := range aiGateway.Spec.CommonMetadata.Annotations {
		annotations[k] = v
	}
	return annotations
}

// buildPodTemplateLabels builds pod template labels:
// commonMetadata.Labels + podMetadata.Labels (podMetadata overrides), then the selector label
// ("app") always takes precedence.
func buildPodTemplateLabels(aiGateway *gatewayv1alpha1.AiGateway) map[string]string {
	labels := make(map[string]string)
	if aiGateway.Spec.CommonMetadata != nil {
		for k, v := range aiGateway.Spec.CommonMetadata.Labels {
			labels[k] = v
		}
	}
	if aiGateway.Spec.PodMetadata != nil {
		for k, v := range aiGateway.Spec.PodMetadata.Labels {
			labels[k] = v
		}
	}
	labels["app"] = aiGateway.Name
	return labels
}

// buildPodTemplateAnnotations builds pod template annotations:
// commonMetadata.Annotations + podMetadata.Annotations (podMetadata overrides), then the
// operator-managed config-hash annotation is always set last.
func buildPodTemplateAnnotations(aiGateway *gatewayv1alpha1.AiGateway, configHash string) map[string]string {
	annotations := make(map[string]string)
	if aiGateway.Spec.CommonMetadata != nil {
		for k, v := range aiGateway.Spec.CommonMetadata.Annotations {
			annotations[k] = v
		}
	}
	if aiGateway.Spec.PodMetadata != nil {
		for k, v := range aiGateway.Spec.PodMetadata.Annotations {
			annotations[k] = v
		}
	}
	annotations["gateway.agentic-layer.ai/config-hash"] = configHash
	return annotations
}

// SetupWithManager sets up the controller with the Manager.
func (r *AiGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.AiGateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Named(ControllerName).
		Complete(r)
}
