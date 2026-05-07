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
	stderrors "errors"
	"fmt"
	"strings"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/litellm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Status condition types
const (
	// AiGatewayConfigured indicates if the AiGateway configuration is valid
	AiGatewayConfigured = "AiGatewayConfigured"

	// AiGatewayReady indicates if the AiGateway is ready to serve traffic
	AiGatewayReady = "AiGatewayReady"
)

// Condition reasons
const (
	// ReasonConfigurationApplied indicates successful configuration application
	ReasonConfigurationApplied = "ConfigurationApplied"

	// ReasonAiGatewayReady indicates AiGateway is ready
	ReasonAiGatewayReady = "AiGatewayReady"

	// ReasonAiGatewayRollingOut indicates the Deployment has not yet finished its rollout.
	ReasonAiGatewayRollingOut = "DeploymentRollingOut"

	// ReasonConfigGenerationFailed is the default reason for failures inside generateAiGatewayConfig.
	ReasonConfigGenerationFailed = "ConfigGenerationFailed"

	// ReasonGuardrailsResolutionFailed indicates a Guard / GuardrailProvider could not be resolved.
	ReasonGuardrailsResolutionFailed = "GuardrailsResolutionFailed"

	// ReasonConfigPatchInvalid indicates the config-patch annotation referenced a missing or
	// malformed ConfigMap.
	ReasonConfigPatchInvalid = "ConfigPatchInvalid"
)

const ControllerName = "aigateway.agentic-layer.ai/ai-gateway-litellm-controller"

// AiGatewayReconciler reconciles an AiGateway object
type AiGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=aigateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=aigateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=aigateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=aigatewayclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=guards,verbs=get;list;watch
// +kubebuilder:rbac:groups=runtime.agentic-layer.ai,resources=guardrailproviders,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

func (r *AiGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AiGateway instance that triggered the reconciliation
	var aiGateway gatewayv1alpha1.AiGateway
	if err := r.Get(ctx, req.NamespacedName, &aiGateway); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AiGateway resource not found")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AiGateway")
		return ctrl.Result{}, err
	}
	original := aiGateway.DeepCopy()

	owned, err := litellm.IsAiGatewayOwnedByController(ctx, r, &aiGateway, ControllerName)
	if err != nil {
		// Surface so controller-runtime requeues with backoff. Swallowing the
		// error left spec edits unobserved during transient API outages.
		log.Error(err, "Failed to determine controller ownership")
		return ctrl.Result{}, err
	}
	if !owned {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling AiGateway", "name", aiGateway.Name, "namespace", aiGateway.Namespace)

	// Initialize status if needed
	if aiGateway.Status.Conditions == nil {
		aiGateway.Status.Conditions = []metav1.Condition{}
	}

	// Step 1: Generate configuration
	configData, err := r.generateAiGatewayConfig(ctx, &aiGateway)
	if err != nil {
		reason := ReasonConfigGenerationFailed
		if pe, ok := stderrors.AsType[*litellm.PhaseError](err); ok {
			switch pe.Phase {
			case "Guardrails":
				reason = ReasonGuardrailsResolutionFailed
			case "ConfigPatch":
				reason = ReasonConfigPatchInvalid
			}
		}
		log.Error(err, "Failed to generate configuration")
		r.updateCondition(&aiGateway, AiGatewayConfigured, metav1.ConditionFalse, reason, err.Error())
		r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionFalse, reason, err.Error())
		if err := r.patchStatus(ctx, original, &aiGateway); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 2: Reconcile ConfigMap, Deployment, and Service
	workload := litellm.GatewayWorkload{
		Name:           aiGateway.Name,
		Namespace:      aiGateway.Namespace,
		Owner:          &aiGateway,
		ContainerPort:  aiGateway.Spec.Port,
		ServicePort:    aiGateway.Spec.Port,
		Env:            r.buildEnvironmentVariables(&aiGateway),
		EnvFrom:        aiGateway.Spec.EnvFrom,
		CommonMetadata: aiGateway.Spec.CommonMetadata,
		PodMetadata:    aiGateway.Spec.PodMetadata,
		ConfigYAML:     configData,
	}

	if err := litellm.ReconcileWorkload(ctx, r.Client, r.Scheme, workload); err != nil {
		// Add a case here whenever a new PhaseError.Phase is introduced in
		// internal/litellm. Unrecognized phases fall through to "WorkloadFailed"
		// — degraded but never silent.
		reason := "WorkloadFailed"
		if pe, ok := stderrors.AsType[*litellm.PhaseError](err); ok {
			switch pe.Phase {
			case "ConfigMap":
				reason = "ConfigMapFailed"
			case "Secret":
				reason = "SecretFailed"
			case "Deployment":
				reason = "DeploymentFailed"
			case "Service":
				reason = "ServiceFailed"
			}
		}
		log.Error(err, "Failed to reconcile workload")
		r.updateCondition(&aiGateway, AiGatewayConfigured, metav1.ConditionFalse, reason, err.Error())
		r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionFalse, reason, err.Error())
		if e := r.patchStatus(ctx, original, &aiGateway); e != nil {
			return ctrl.Result{}, e
		}
		// All ReconcileWorkload phases (ConfigMap / Secret / Deployment / Service)
		// are apiserver calls — surface the error so controller-runtime requeues
		// with exponential backoff. Permanent config-generation errors are handled
		// in the generateAiGatewayConfig branch above.
		return ctrl.Result{}, err
	}

	r.updateCondition(&aiGateway, AiGatewayConfigured, metav1.ConditionTrue,
		ReasonConfigurationApplied, "AiGateway configuration successfully applied")

	// Ready reflects pod-level availability, not just "we created the API objects".
	// The Owns(&appsv1.Deployment{}) watch re-fires Reconcile when the deployment-
	// controller publishes status changes, so we don't need a manual requeue.
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: aiGateway.Namespace, Name: aiGateway.Name}, deployment); err != nil {
		log.Error(err, "Failed to get Deployment for rollout check")
		return ctrl.Result{}, err
	}
	if rolledOut, msg := litellm.IsDeploymentRolledOut(deployment); rolledOut {
		r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionTrue,
			ReasonAiGatewayReady, "AiGateway is ready and serving traffic")
	} else {
		r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionFalse,
			ReasonAiGatewayRollingOut, msg)
	}

	log.Info("Successfully reconciled AiGateway", "name", aiGateway.Name,
		"aiModels", len(aiGateway.Spec.AiModels))

	if err := r.patchStatus(ctx, original, &aiGateway); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// generateAiGatewayConfig renders the LiteLLM config for aiGateway, optionally
// layering a user-supplied patch on top. Returns *litellm.PhaseError tagged
// with the failing phase. The Reconcile config-failure branch maps "Guardrails"
// and "ConfigPatch" to dedicated reasons; all other phases (e.g. "ConfigRender")
// fall through to ReasonConfigGenerationFailed.
func (r *AiGatewayReconciler) generateAiGatewayConfig(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway) (string, error) {

	log := logf.FromContext(ctx)

	// Build model list with proper provider prefixes and environment variable API keys
	modelList := make([]litellm.ModelConfig, len(aiGateway.Spec.AiModels))
	for i, model := range aiGateway.Spec.AiModels {
		modelList[i] = litellm.ModelConfig{
			ModelName: model.Name,
			LiteLLMParams: litellm.LiteLLMParams{
				Model:  fmt.Sprintf("%s/%s", model.Provider, model.Name),
				ApiKey: fmt.Sprintf("os.environ/%s", r.getProviderApiKeyEnvVar(model)),
			},
		}
	}

	// Resolve guardrails from referenced Guard and GuardrailProvider resources
	guardrails, err := litellm.ResolveGuardrails(ctx, r, aiGateway.Namespace, aiGateway.Spec.Guardrails, litellm.GuardrailTargetLLM)
	if err != nil {
		return "", &litellm.PhaseError{Phase: "Guardrails", Err: err}
	}

	config := litellm.LiteLLMConfig{
		ModelList: modelList,
		LiteLLMSettings: litellm.LiteLLMSettings{
			RequestTimeout: litellm.DefaultRequestTimeout,
			// 'callbacks: ["otel"]' is required to send traces to otel after handling incoming requests
			// (see https://docs.litellm.ai/docs/proxy/logging#opentelemetry)
			Callbacks: []string{"otel", "prometheus"},
		},
		Guardrails: guardrails,
	}

	patch, err := litellm.LoadPatch(ctx, r.Client, aiGateway.Namespace, aiGateway.Annotations[litellm.ConfigPatchAnnotation])
	if err != nil {
		return "", err
	}

	configYAML, err := litellm.RenderConfigWithPatch(config, patch)
	if err != nil {
		return "", &litellm.PhaseError{Phase: "ConfigRender", Err: err}
	}

	log.Info("Generated LiteLLM configuration",
		"aiGateway", aiGateway.Name,
		"models", len(aiGateway.Spec.AiModels),
		"guardrails", len(guardrails),
		"patched", patch != nil,
	)

	return configYAML, nil
}

func (r *AiGatewayReconciler) getProviderApiKeyEnvVar(model gatewayv1alpha1.AiModel) string {
	return strings.ToUpper(model.Provider) + "_API_KEY"
}

// buildEnvironmentVariables creates environment variables for the deployment
func (r *AiGatewayReconciler) buildEnvironmentVariables(aiGateway *gatewayv1alpha1.AiGateway) []corev1.EnvVar {
	envMap := make(map[string]corev1.EnvVar, len(aiGateway.Spec.Env)+len(aiGateway.Spec.AiModels))

	// Generated API-key env vars first; user spec.env wins on conflict.
	r.generateApiKeyEnvVars(aiGateway, envMap)
	for _, e := range aiGateway.Spec.Env {
		envMap[e.Name] = e
	}

	envs := make([]corev1.EnvVar, 0, len(envMap))
	for _, e := range envMap {
		envs = append(envs, e)
	}
	return envs
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
						Name: litellm.ApiKeySecretName,
					},
					Key:      envVarName,
					Optional: &[]bool{true}[0], // Make optional so deployment doesn't fail if secret missing
				},
			},
		}
	}
}

// updateCondition stamps a condition on the AiGateway via apimeta.SetStatusCondition,
// which preserves LastTransitionTime when status/reason/message are unchanged.
func (r *AiGatewayReconciler) updateCondition(aiGateway *gatewayv1alpha1.AiGateway, conditionType string, status metav1.ConditionStatus, reason, message string) {
	apimeta.SetStatusCondition(&aiGateway.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: aiGateway.Generation,
	})
}

// patchStatus issues an optimistic-merge patch against the snapshot captured
// at the start of Reconcile. Using Patch rather than Update keeps concurrent
// status writes from other workers / sub-resources from clobbering each other.
func (r *AiGatewayReconciler) patchStatus(ctx context.Context, original, aiGateway *gatewayv1alpha1.AiGateway) error {
	if err := r.Status().Patch(ctx, aiGateway, client.MergeFrom(original)); err != nil {
		logf.FromContext(ctx).Error(err, "Failed to patch AiGateway status")
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AiGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Indexer key used to locate AiGateways by their config-patch annotation
	// value. Used by the ConfigMap watch below to enqueue only the gateways in
	// the namespace whose patch reference matches the changed ConfigMap.
	const aiGatewayConfigPatchIndex = "metadata.annotations.config-patch"

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &gatewayv1alpha1.AiGateway{}, aiGatewayConfigPatchIndex,
		func(obj client.Object) []string {
			gw, ok := obj.(*gatewayv1alpha1.AiGateway)
			if !ok {
				return nil
			}
			v, ok := gw.Annotations[litellm.ConfigPatchAnnotation]
			if !ok || v == "" {
				return nil
			}
			return []string{v}
		},
	); err != nil {
		return fmt.Errorf("failed to register AiGateway config-patch indexer: %w", err)
	}

	// enqueueAiGatewaysInNamespace enqueues reconcile requests for all AiGateway objects in
	// the namespace of the triggering object.
	enqueueAiGatewaysInNamespace := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := logf.FromContext(ctx)
		var aiGatewayList gatewayv1alpha1.AiGatewayList
		if err := r.List(ctx, &aiGatewayList, client.InNamespace(obj.GetNamespace())); err != nil {
			log.Error(err, "Failed to list AiGateways for re-queuing", "namespace", obj.GetNamespace(), "trigger", obj.GetName())
			return nil
		}
		requests := make([]reconcile.Request, len(aiGatewayList.Items))
		for i, gw := range aiGatewayList.Items {
			requests[i] = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      gw.Name,
					Namespace: gw.Namespace,
				},
			}
		}
		return requests
	})

	// enqueueAllAiGateways fans out to every AiGateway in the cluster. Used
	// for AiGatewayClass changes (cluster-scoped) — the default-class
	// annotation toggling has to re-evaluate gateways that don't name a class
	// explicitly.
	enqueueAllAiGateways := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []reconcile.Request {
		log := logf.FromContext(ctx)
		var aiGatewayList gatewayv1alpha1.AiGatewayList
		if err := r.List(ctx, &aiGatewayList); err != nil {
			log.Error(err, "Failed to list AiGateways for AiGatewayClass watch")
			return nil
		}
		requests := make([]reconcile.Request, len(aiGatewayList.Items))
		for i, gw := range aiGatewayList.Items {
			requests[i] = reconcile.Request{NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}}
		}
		return requests
	})

	enqueueAiGatewaysForPatchConfigMap := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
		log := logf.FromContext(ctx)
		var gwList gatewayv1alpha1.AiGatewayList
		if err := r.List(ctx, &gwList,
			client.InNamespace(obj.GetNamespace()),
			client.MatchingFields{aiGatewayConfigPatchIndex: obj.GetName()},
		); err != nil {
			log.Error(err, "Failed to list AiGateways for patch ConfigMap watch", "namespace", obj.GetNamespace(), "configmap", obj.GetName())
			return nil
		}
		requests := make([]reconcile.Request, len(gwList.Items))
		for i, gw := range gwList.Items {
			requests[i] = reconcile.Request{NamespacedName: types.NamespacedName{Name: gw.Name, Namespace: gw.Namespace}}
		}
		return requests
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.AiGateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(&gatewayv1alpha1.AiGatewayClass{}, enqueueAllAiGateways).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				// Only react to the default API-keys secret
				if obj.GetName() != litellm.ApiKeySecretName {
					return nil
				}
				// Enqueue reconcile requests for all AiGateway objects in the same namespace
				var aiGatewayList gatewayv1alpha1.AiGatewayList
				if err := r.List(ctx, &aiGatewayList, client.InNamespace(obj.GetNamespace())); err != nil {
					return nil
				}
				requests := make([]reconcile.Request, len(aiGatewayList.Items))
				for i, gw := range aiGatewayList.Items {
					requests[i] = reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      gw.Name,
							Namespace: gw.Namespace,
						},
					}
				}
				return requests
			}),
		).
		Watches(&corev1.ConfigMap{}, enqueueAiGatewaysForPatchConfigMap).
		// Watch Guard changes so that updates to a Guard trigger re-reconciliation of all
		// AiGateway resources in the same namespace that may reference it.
		Watches(&gatewayv1alpha1.Guard{}, enqueueAiGatewaysInNamespace).
		// Watch GuardrailProvider changes for the same reason.
		Watches(&gatewayv1alpha1.GuardrailProvider{}, enqueueAiGatewaysInNamespace).
		Named(ControllerName).
		Complete(r)
}
