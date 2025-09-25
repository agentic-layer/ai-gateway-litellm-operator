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
	"fmt"

	gatewayv1alpha1 "github.com/agentic-layer/ai-gateway-litellm/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/constants"
	"github.com/agentic-layer/ai-gateway-litellm/internal/equality"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ModelRouterReconciler reconciles a ModelRouter object
type ModelRouterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=gateway.agentic-layer.ai,resources=modelrouters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.agentic-layer.ai,resources=modelrouters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.agentic-layer.ai,resources=modelrouters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *ModelRouterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the ModelRouter instance that triggered the reconciliation
	var modelRouter gatewayv1alpha1.ModelRouter
	if err := r.Get(ctx, req.NamespacedName, &modelRouter); err != nil {
		if errors.IsNotFound(err) {
			log.Info("ModelRouter resource not found")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ModelRouter")
		return ctrl.Result{}, err
	}

	if !r.shouldProcessModelRouter(&modelRouter) {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling ModelRouter", "name", modelRouter.Name, "namespace", modelRouter.Namespace)

	// Initialize status if needed
	if modelRouter.Status.Conditions == nil {
		modelRouter.Status.Conditions = []metav1.Condition{}
	}

	// Step 1: Validate ModelRouter configuration
	if err := r.validateModelRouterConfig(&modelRouter); err != nil {
		log.Error(err, "Invalid ModelRouter configuration")
		r.updateCondition(&modelRouter, constants.ModelRouterConfigured, metav1.ConditionFalse,
			constants.ReasonConfigurationInvalid, fmt.Sprintf("ModelRouter configuration validation failed: %v", err))
		r.updateCondition(&modelRouter, constants.ModelRouterReady, metav1.ConditionFalse,
			constants.ReasonConfigurationInvalid, "ModelRouter not ready due to invalid configuration")
		if err := r.updateStatus(ctx, &modelRouter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	r.updateCondition(&modelRouter, constants.ModelRouterConfigured, metav1.ConditionTrue,
		constants.ReasonConfigurationValid, "ModelRouter configuration validation passed")

	// Step 2: Generate configuration
	configData, configHash, err := r.generateModelRouterConfig(ctx, &modelRouter)
	if err != nil {
		log.Error(err, "Failed to generate configuration")
		r.updateCondition(&modelRouter, constants.ModelRouterConfigured, metav1.ConditionFalse,
			"ConfigGenerationFailed", fmt.Sprintf("Failed to generate config: %v", err))
		r.updateCondition(&modelRouter, constants.ModelRouterReady, metav1.ConditionFalse,
			"ConfigGenerationFailed", "ModelRouter not ready due to config generation failure")
		if err := r.updateStatus(ctx, &modelRouter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 3: Create/update ConfigMap with configuration
	if err := r.reconcileConfigMap(ctx, &modelRouter, configData, configHash); err != nil {
		log.Error(err, "Failed to reconcile ConfigMap")
		r.updateCondition(&modelRouter, constants.ModelRouterConfigured, metav1.ConditionFalse,
			"ConfigMapFailed", fmt.Sprintf("Failed to create/update ConfigMap: %v", err))
		if err := r.updateStatus(ctx, &modelRouter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 4: Create/update Deployment
	if err := r.reconcileDeployment(ctx, &modelRouter, configHash); err != nil {
		log.Error(err, "Failed to reconcile Deployment")
		r.updateCondition(&modelRouter, constants.ModelRouterReady, metav1.ConditionFalse,
			"DeploymentFailed", fmt.Sprintf("Failed to create/update Deployment: %v", err))
		if err := r.updateStatus(ctx, &modelRouter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 5: Create/update Service
	if err := r.reconcileService(ctx, &modelRouter); err != nil {
		log.Error(err, "Failed to reconcile Service")
		r.updateCondition(&modelRouter, constants.ModelRouterReady, metav1.ConditionFalse,
			"ServiceFailed", fmt.Sprintf("Failed to create/update Service: %v", err))
		if err := r.updateStatus(ctx, &modelRouter); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Update successful status
	modelRouter.Status.ConfigHash = configHash
	now := metav1.Now()
	modelRouter.Status.LastUpdated = &now

	r.updateCondition(&modelRouter, constants.ModelRouterConfigured, metav1.ConditionTrue,
		constants.ReasonConfigurationApplied, "ModelRouter configuration successfully applied")
	r.updateCondition(&modelRouter, constants.ModelRouterReady, metav1.ConditionTrue,
		constants.ReasonModelRouterReady, "ModelRouter is ready and serving traffic")

	log.Info("Successfully reconciled ModelRouter", "name", modelRouter.Name,
		"aiModels", len(modelRouter.Spec.AiModels), "configHash", configHash[:8])

	if err := r.updateStatus(ctx, &modelRouter); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// validateModelRouterConfig validates the ModelRouter configuration
func (r *ModelRouterReconciler) validateModelRouterConfig(modelRouter *gatewayv1alpha1.ModelRouter) error {
	// Create generator and use it to validate
	generator, err := NewModelRouterGenerator(modelRouter.Spec.Type)
	if err != nil {
		return err
	}

	return generator.Validate(modelRouter)
}

// generateModelRouterConfig generates the configuration using the appropriate generator
func (r *ModelRouterReconciler) generateModelRouterConfig(ctx context.Context, modelRouter *gatewayv1alpha1.ModelRouter) (string, string, error) {
	// Create generator
	generator, err := NewModelRouterGenerator(modelRouter.Spec.Type)
	if err != nil {
		return "", "", err
	}

	return generator.Generate(ctx, modelRouter)
}

// reconcileConfigMap creates or updates the ConfigMap containing LiteLLM configuration
func (r *ModelRouterReconciler) reconcileConfigMap(ctx context.Context, modelRouter *gatewayv1alpha1.ModelRouter, configData, configHash string) error {
	log := logf.FromContext(ctx)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", modelRouter.Name),
			Namespace: modelRouter.Namespace,
			Labels: map[string]string{
				"app":                                  modelRouter.Name,
				"type":                                 "litellm",
				"gateway.agentic-layer.ai/config-hash": configHash,
			},
		},
		Data: map[string]string{
			"config.yaml": configData,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(modelRouter, configMap, r.Scheme); err != nil {
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

	if !equality.LabelsEqual(existing.Labels, configMap.Labels) {
		needsUpdate = true
	}

	if needsUpdate {
		log.Info("Updating ConfigMap", "name", configMap.Name)
		existing.Data = configMap.Data
		existing.Labels = configMap.Labels
		return r.Update(ctx, existing)
	}

	return nil
}

// reconcileDeployment creates or updates the Deployment for LiteLLM
func (r *ModelRouterReconciler) reconcileDeployment(ctx context.Context, modelRouter *gatewayv1alpha1.ModelRouter, configHash string) error {
	log := logf.FromContext(ctx)

	replicas := int32(1)
	labels := map[string]string{
		"app":                                  modelRouter.Name,
		"type":                                 "litellm",
		"gateway.agentic-layer.ai/config-hash": configHash,
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelRouter.Name,
			Namespace: modelRouter.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": modelRouter.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "litellm",
							Image: "ghcr.io/berriai/litellm:v1.77.2-stable",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: modelRouter.Spec.Port,
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
								fmt.Sprintf("%d", modelRouter.Spec.Port),
							},
							Env: r.buildEnvironmentVariables(modelRouter),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-config", modelRouter.Name),
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
	if err := controllerutil.SetControllerReference(modelRouter, deployment, r.Scheme); err != nil {
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
	needsUpdate := !equality.LabelsEqual(existing.Labels, deployment.Labels)

	// Check labels for changes

	// Check template labels for changes
	if !equality.LabelsEqual(existing.Spec.Template.Labels, deployment.Spec.Template.Labels) {
		needsUpdate = true
	}

	// Check environment variables for changes
	if len(existing.Spec.Template.Spec.Containers) > 0 && len(deployment.Spec.Template.Spec.Containers) > 0 {
		if !equality.EnvVarsEqual(existing.Spec.Template.Spec.Containers[0].Env, deployment.Spec.Template.Spec.Containers[0].Env) {
			needsUpdate = true
		}

		// Check image changes
		if existing.Spec.Template.Spec.Containers[0].Image != deployment.Spec.Template.Spec.Containers[0].Image {
			needsUpdate = true
		}

		// Check port configuration changes
		existingPorts := existing.Spec.Template.Spec.Containers[0].Ports
		newPorts := deployment.Spec.Template.Spec.Containers[0].Ports
		if len(existingPorts) != len(newPorts) {
			needsUpdate = true
		} else if len(existingPorts) > 0 && len(newPorts) > 0 {
			// Check if the main container port changed
			if existingPorts[0].ContainerPort != newPorts[0].ContainerPort {
				needsUpdate = true
			}
		}
	}

	if needsUpdate {
		log.Info("Updating Deployment", "name", deployment.Name)
		existing.Labels = deployment.Labels
		existing.Spec = deployment.Spec
		return r.Update(ctx, existing)
	}

	return nil
}

// reconcileService creates or updates the Service for LiteLLM
func (r *ModelRouterReconciler) reconcileService(ctx context.Context, modelRouter *gatewayv1alpha1.ModelRouter) error {
	log := logf.FromContext(ctx)

	labels := map[string]string{
		"app":  modelRouter.Name,
		"type": "litellm",
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      modelRouter.Name,
			Namespace: modelRouter.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": modelRouter.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       modelRouter.Spec.Port,
					TargetPort: intstr.FromInt32(modelRouter.Spec.Port),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(modelRouter, service, r.Scheme); err != nil {
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
	needsUpdate := !equality.LabelsEqual(existing.Labels, service.Labels)

	// Check labels for changes

	// Check ports for changes (safe checking)
	if len(existing.Spec.Ports) > 0 && len(service.Spec.Ports) > 0 &&
		existing.Spec.Ports[0].Port != service.Spec.Ports[0].Port {
		needsUpdate = true
	}

	if needsUpdate {
		log.Info("Updating Service", "name", service.Name)
		existing.Spec.Ports = service.Spec.Ports
		existing.Labels = service.Labels
		return r.Update(ctx, existing)
	}

	return nil
}

// buildEnvironmentVariables creates environment variables for the deployment
func (r *ModelRouterReconciler) buildEnvironmentVariables(modelRouter *gatewayv1alpha1.ModelRouter) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "LITELLM_LOG",
			Value: "INFO",
		},
	}

	// Add API key environment variables for each model
	// We need to determine what API keys are needed based on the models
	generator, err := NewModelRouterGenerator(modelRouter.Spec.Type)
	if err != nil {
		return envVars // Return basic env vars if generator fails
	}

	if litellmGen, ok := generator.(*LiteLLMGenerator); ok {
		apiKeyEnvVars := make(map[string]bool)

		// Collect unique API key environment variables needed
		for _, model := range modelRouter.Spec.AiModels {
			_, apiKeyEnvVar := litellmGen.mapModelToLiteLLMFormat(model.Name)
			if apiKeyEnvVar != "" {
				apiKeyEnvVars[apiKeyEnvVar] = true
			}
		}

		// Add environment variables from secret for each API key
		for envVarName := range apiKeyEnvVars {
			envVars = append(envVars, corev1.EnvVar{
				Name: envVarName,
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: constants.DefaultSecretName,
						},
						Key:      envVarName,
						Optional: &[]bool{true}[0], // Make optional so deployment doesn't fail if secret missing
					},
				},
			})
		}
	}

	return envVars
}

// updateCondition updates or adds a condition to the ModelRouter status
func (r *ModelRouterReconciler) updateCondition(modelRouter *gatewayv1alpha1.ModelRouter, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}

	// Find existing condition
	for i, existingCondition := range modelRouter.Status.Conditions {
		if existingCondition.Type == conditionType {
			// Update existing condition
			modelRouter.Status.Conditions[i] = condition
			modelRouter.Status.Conditions[i].LastTransitionTime = metav1.Now()
			return
		}
	}

	// Add new condition
	condition.LastTransitionTime = metav1.Now()
	modelRouter.Status.Conditions = append(modelRouter.Status.Conditions, condition)
}

// updateStatus updates the ModelRouter status
func (r *ModelRouterReconciler) updateStatus(ctx context.Context, modelRouter *gatewayv1alpha1.ModelRouter) error {
	if statusErr := r.Status().Update(ctx, modelRouter); statusErr != nil {
		logf.FromContext(ctx).Error(statusErr, "Failed to update ModelRouter status")
		return statusErr
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ModelRouterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.ModelRouter{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Named("modelrouter").
		Complete(r)
}

func (r *ModelRouterReconciler) shouldProcessModelRouter(modelRouter *gatewayv1alpha1.ModelRouter) bool {
	// NOTE: In the future, we will check the modelRouter.Spec.ModelRouterClassName field.
	// If the field is not set, but the litellm ModelRouterClass is the default, then we are also responsible and should process the router

	return modelRouter.Spec.Type == constants.TypeLitellm
}
