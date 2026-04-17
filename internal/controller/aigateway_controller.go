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
	"sort"
	"strconv"
	"strings"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"github.com/agentic-layer/ai-gateway-litellm/internal/controller/utils"
	"gopkg.in/yaml.v3"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	litellmImage = "ghcr.io/berriai/litellm:v1.83.3-stable.patch.1"
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

	// prometheusMultiprocVolumeName is the name of the emptyDir volume for prometheus multiprocess mode
	prometheusMultiprocVolumeName = "prometheus-multiproc"

	// prometheusMultiprocDir is the mount path for the prometheus multiprocess directory
	prometheusMultiprocDir = "/prometheus_multiproc"
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
	ModelList       []ModelConfig     `yaml:"model_list"`
	LiteLLMSettings LiteLLMSettings   `yaml:"litellm_settings,omitempty"`
	Guardrails      []GuardrailConfig `yaml:"guardrails,omitempty"`
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

// GuardrailConfig represents a single guardrail entry in the LiteLLM configuration.
type GuardrailConfig struct {
	GuardrailName string                 `yaml:"guardrail_name"`
	LiteLLMParams GuardrailLiteLLMParams `yaml:"litellm_params"`
}

// GuardrailLiteLLMParams holds the LiteLLM-specific parameters for a guardrail.
type GuardrailLiteLLMParams struct {
	// Guardrail is the LiteLLM guardrail type identifier (e.g. "presidio").
	Guardrail string `yaml:"guardrail"`
	// Mode defines when the guardrail is applied. Multiple modes can be specified.
	Mode []string `yaml:"mode"`
	// DefaultOn ensures the guardrail is applied to every request without requiring
	// explicit opt-in per call.
	DefaultOn bool `yaml:"default_on"`
	// OutputParsePii enables automatic unmasking of PII tokens in LLM responses.
	// When true, masked tokens (e.g. <PERSON_1>) are replaced with original values.
	// Only used when Guardrail is "presidio".
	OutputParsePii bool `yaml:"output_parse_pii,omitempty"`
	// PresidioAnalyzerApiBase is the URL of the Presidio Analyzer service.
	// Only used when Guardrail is "presidio".
	PresidioAnalyzerApiBase string `yaml:"presidio_analyzer_api_base,omitempty"`
	// PresidioAnonymizerApiBase is the URL of the Presidio Anonymizer service.
	// Only used when Guardrail is "presidio".
	PresidioAnonymizerApiBase string `yaml:"presidio_anonymizer_api_base,omitempty"`
	// PresidioLanguage is the language code for PII detection (e.g. "en", "de").
	// Only used when Guardrail is "presidio".
	PresidioLanguage string `yaml:"presidio_language,omitempty"`
	// PresidioScoreThresholds maps entity types to minimum confidence scores (0.0 to 1.0).
	// Use "ALL" as key to set a default threshold for all entity types.
	// Only used when Guardrail is "presidio".
	PresidioScoreThresholds map[string]string `yaml:"presidio_score_thresholds,omitempty"`
	// PiiEntitiesConfig maps PII entity types to actions ("MASK" or "BLOCK").
	// Only used when Guardrail is "presidio".
	PiiEntitiesConfig map[string]string `yaml:"pii_entities_config,omitempty"`
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

	// Step 4: Compute secret hash to detect API key changes
	secretHash, err := r.computeSecretHash(ctx, &aiGateway)
	if err != nil {
		log.Error(err, "Failed to compute secret hash")
		r.updateCondition(&aiGateway, AiGatewayConfigured, metav1.ConditionFalse,
			"SecretHashFailed", fmt.Sprintf("Failed to compute secret hash: %v", err))
		if err := r.updateStatus(ctx, &aiGateway); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	// Step 5: Create/update Deployment
	if err := r.reconcileDeployment(ctx, &aiGateway, configHash, secretHash); err != nil {
		log.Error(err, "Failed to reconcile Deployment")
		r.updateCondition(&aiGateway, AiGatewayReady, metav1.ConditionFalse,
			"DeploymentFailed", fmt.Sprintf("Failed to create/update Deployment: %v", err))
		if err := r.updateStatus(ctx, &aiGateway); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Step 6: Create/update Service
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
		"aiModels", len(aiGateway.Spec.AiModels), "configHash", configHash[:8], "secretHash", secretHash[:8])

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

	// Resolve guardrails from referenced Guard and GuardrailProvider resources
	guardrails, err := r.resolveGuardrails(ctx, aiGateway)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve guardrails: %w", err)
	}

	// Build complete configuration with settings
	config := LiteLLMConfig{
		ModelList: modelList,
		LiteLLMSettings: LiteLLMSettings{
			RequestTimeout: DefaultRequestTimeout,
			// 'callbacks: ["otel"]' is required to send traces to otel after handling incoming requests
			// (see https://docs.litellm.ai/docs/proxy/logging#opentelemetry)
			Callbacks: []string{"otel", "prometheus"},
		},
		Guardrails: guardrails,
	}

	// Generate YAML configuration
	configYAML, err := yaml.Marshal(config)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal LiteLLM config: %w", err)
	}

	// Generate configuration hash
	hash := sha256.Sum256(configYAML)
	configHash := fmt.Sprintf("%x", hash)[:16]

	log.Info("Generated LiteLLM configuration", "aiGateway", aiGateway.Name, "models", len(aiGateway.Spec.AiModels), "guardrails", len(guardrails), "configHash", configHash[:8])

	return string(configYAML), configHash, nil
}

// resolveGuardrails fetches all Guard resources referenced in the AiGateway spec and maps
// them to LiteLLM guardrail configuration entries.
func (r *AiGatewayReconciler) resolveGuardrails(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway) ([]GuardrailConfig, error) {
	log := logf.FromContext(ctx)

	if len(aiGateway.Spec.Guardrails) == 0 {
		return nil, nil
	}

	guardrails := make([]GuardrailConfig, 0, len(aiGateway.Spec.Guardrails))
	for _, ref := range aiGateway.Spec.Guardrails {
		namespace := ref.Namespace
		if namespace == "" {
			namespace = aiGateway.Namespace
		}

		var guard gatewayv1alpha1.Guard
		if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, &guard); err != nil {
			return nil, fmt.Errorf("failed to get Guard %s/%s: %w", namespace, ref.Name, err)
		}

		providerNamespace := guard.Spec.ProviderRef.Namespace
		if providerNamespace == "" {
			providerNamespace = guard.Namespace
		}
		providerName := guard.Spec.ProviderRef.Name

		var provider gatewayv1alpha1.GuardrailProvider
		if err := r.Get(ctx, types.NamespacedName{Name: providerName, Namespace: providerNamespace}, &provider); err != nil {
			return nil, fmt.Errorf("failed to get GuardrailProvider %s/%s referenced by Guard %s: %w",
				providerNamespace, providerName, guard.Name, err)
		}

		guardrailCfg, err := r.buildGuardrailConfig(&guard, &provider)
		if err != nil {
			log.Error(err, "Skipping unsupported guardrail", "guard", guard.Name, "type", provider.Spec.Type)
			continue
		}
		guardrails = append(guardrails, guardrailCfg)
	}

	return guardrails, nil
}

// buildGuardrailConfig maps a Guard and its GuardrailProvider to a LiteLLM GuardrailConfig.
func (r *AiGatewayReconciler) buildGuardrailConfig(guard *gatewayv1alpha1.Guard, provider *gatewayv1alpha1.GuardrailProvider) (GuardrailConfig, error) {
	// Convert []GuardMode to []string for the LiteLLM YAML config.
	modes := make([]string, len(guard.Spec.Mode))
	for i, m := range guard.Spec.Mode {
		modes[i] = string(m)
	}

	params := GuardrailLiteLLMParams{
		Mode:      modes,
		DefaultOn: true,
	}

	switch provider.Spec.Type {
	case "presidio-api":
		if provider.Spec.Presidio == nil {
			return GuardrailConfig{}, fmt.Errorf("GuardrailProvider %s has type presidio-api but no presidio config", provider.Name)
		}
		// The CRD type is "presidio-api" but LiteLLM expects "presidio" as the guardrail identifier.
		params.Guardrail = "presidio"
		// Presidio requires both an Analyzer and an Anonymizer endpoint. The CRD provides a
		// single baseUrl for the Presidio service, which is used for both.
		params.PresidioAnalyzerApiBase = provider.Spec.Presidio.BaseUrl
		params.PresidioAnonymizerApiBase = provider.Spec.Presidio.BaseUrl
		// Enable output parsing by default so masked PII tokens in LLM responses
		// (e.g. <PERSON_1>) are replaced with original values before returning to the user.
		params.OutputParsePii = true
		if guard.Spec.Presidio != nil {
			params.PresidioLanguage = guard.Spec.Presidio.Language
			if len(guard.Spec.Presidio.ScoreThresholds) > 0 {
				params.PresidioScoreThresholds = guard.Spec.Presidio.ScoreThresholds
			}
			if len(guard.Spec.Presidio.EntityActions) > 0 {
				params.PiiEntitiesConfig = guard.Spec.Presidio.EntityActions
			}
		}
	default:
		return GuardrailConfig{}, fmt.Errorf("unsupported guardrail provider type %q for guard %s", provider.Spec.Type, guard.Name)
	}

	return GuardrailConfig{
		GuardrailName: guard.Name,
		LiteLLMParams: params,
	}, nil
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
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(aiGateway, configMap, r.Scheme); err != nil {
			return err
		}

		// Set labels
		if configMap.Labels == nil {
			configMap.Labels = make(map[string]string)
		}
		configMap.Labels["app"] = aiGateway.Name

		// Set data
		configMap.Data = map[string]string{
			"config.yaml": configData,
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile ConfigMap: %w", err)
	}

	if result != controllerutil.OperationResultNone {
		log.Info("ConfigMap reconciled", "name", configMap.Name, "operation", result)
	}

	return nil
}

// reconcileDeployment creates or updates the Deployment for LiteLLM
func (r *AiGatewayReconciler) reconcileDeployment(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway, configHash string, secretHash string) error {
	log := logf.FromContext(ctx)

	replicas := int32(1)
	deploymentLabels := buildResourceLabels(aiGateway)
	deploymentAnnotations := buildResourceAnnotations(aiGateway)
	podTemplateLabels := buildPodTemplateLabels(aiGateway)
	podTemplateAnnotations := buildPodTemplateAnnotations(aiGateway, configHash, secretHash)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      aiGateway.Name,
			Namespace: aiGateway.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(aiGateway, deployment, r.Scheme); err != nil {
			return err
		}

		// Set labels
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		for key, value := range deploymentLabels {
			deployment.Labels[key] = value
		}

		// Set annotations
		if deployment.Annotations == nil {
			deployment.Annotations = make(map[string]string)
		}
		for key, value := range deploymentAnnotations {
			deployment.Annotations[key] = value
		}

		// Set spec
		deployment.Spec.Replicas = &replicas
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": aiGateway.Name,
			},
		}

		// Set template labels
		if deployment.Spec.Template.Labels == nil {
			deployment.Spec.Template.Labels = make(map[string]string)
		}
		for key, value := range podTemplateLabels {
			deployment.Spec.Template.Labels[key] = value
		}

		// Set template annotations
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		for key, value := range podTemplateAnnotations {
			deployment.Spec.Template.Annotations[key] = value
		}

		// Find or create the litellm container
		container := utils.FindContainerByName(deployment.Spec.Template.Spec.Containers, liteLLMContainerName)
		if container == nil {
			deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, corev1.Container{
				Name: liteLLMContainerName,
			})
			container = &deployment.Spec.Template.Spec.Containers[len(deployment.Spec.Template.Spec.Containers)-1]
		}

		container.Image = litellmImage
		container.Ports = []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: aiGateway.Spec.Port,
				Protocol:      corev1.ProtocolTCP,
			},
		}
		container.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      "config",
				MountPath: "/app/config",
				ReadOnly:  true,
			},
			{
				Name:      prometheusMultiprocVolumeName,
				MountPath: prometheusMultiprocDir,
			},
		}
		container.Command = []string{
			"litellm",
			"--config",
			"/app/config/config.yaml",
			"--port",
			strconv.Itoa(int(aiGateway.Spec.Port)),
		}
		container.Env = r.buildEnvironmentVariables(aiGateway)
		container.EnvFrom = aiGateway.Spec.EnvFrom
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("250M"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1G"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		}
		// LiteLLM health check endpoints: /health/liveliness and /health/readiness
		// Note: LiteLLM uses "liveliness" (not "liveness") in their API
		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/health/liveliness",
					Port:   intstr.FromInt32(aiGateway.Spec.Port),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			SuccessThreshold:    1,
			FailureThreshold:    10,
		}
		container.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/health/readiness",
					Port:   intstr.FromInt32(aiGateway.Spec.Port),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		}

		// Set volumes
		deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
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
			{
				Name: prometheusMultiprocVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile Deployment: %w", err)
	}

	if result != controllerutil.OperationResultNone {
		log.Info("Deployment reconciled", "name", deployment.Name, "operation", result)
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
			Name:      aiGateway.Name,
			Namespace: aiGateway.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(aiGateway, service, r.Scheme); err != nil {
			return err
		}

		// Set labels
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		for key, value := range serviceLabels {
			service.Labels[key] = value
		}

		// Set annotations
		if service.Annotations == nil {
			service.Annotations = make(map[string]string)
		}
		for key, value := range serviceAnnotations {
			service.Annotations[key] = value
		}

		// Set spec
		service.Spec.Selector = map[string]string{
			"app": aiGateway.Name,
		}
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       aiGateway.Spec.Port,
				TargetPort: intstr.FromInt32(aiGateway.Spec.Port),
				Protocol:   corev1.ProtocolTCP,
			},
		}
		service.Spec.Type = corev1.ServiceTypeClusterIP

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}

	if result != controllerutil.OperationResultNone {
		log.Info("Service reconciled", "name", service.Name, "operation", result)
	}

	return nil
}

// buildEnvironmentVariables creates environment variables for the deployment
func (r *AiGatewayReconciler) buildEnvironmentVariables(aiGateway *gatewayv1alpha1.AiGateway) []corev1.EnvVar {
	envMap := make(map[string]corev1.EnvVar)

	r.generateApiKeyEnvVars(aiGateway, envMap)

	// Required for prometheus callback to work with multiple workers
	// (see https://docs.litellm.ai/docs/proxy/prometheus)
	envMap["PROMETHEUS_MULTIPROC_DIR"] = corev1.EnvVar{
		Name:  "PROMETHEUS_MULTIPROC_DIR",
		Value: prometheusMultiprocDir,
	}

	// Add environment variables from AiGateway spec
	// User-provided Env Vars override generated Env Vars
	for _, env := range aiGateway.Spec.Env {
		envMap[env.Name] = env
	}

	// Convert map to sorted slice for deterministic ordering
	envVars := make([]corev1.EnvVar, 0, len(envMap))
	for _, env := range envMap {
		envVars = append(envVars, env)
	}
	sort.Slice(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})
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

// computeSecretHash fetches the API keys secret and returns a hash of its data.
// If the secret does not exist, an empty hash is returned so the deployment can still be created.
func (r *AiGatewayReconciler) computeSecretHash(ctx context.Context, aiGateway *gatewayv1alpha1.AiGateway) (string, error) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: DefaultSecretName, Namespace: aiGateway.Namespace}, secret)
	if err != nil {
		if errors.IsNotFound(err) {
			// Secret is optional; return a stable empty hash so the annotation is still set
			empty := sha256.Sum256([]byte{})
			return fmt.Sprintf("%x", empty)[:16], nil
		}
		return "", fmt.Errorf("failed to get secret %s: %w", DefaultSecretName, err)
	}

	// Sort keys for a deterministic hash
	keys := make([]string, 0, len(secret.Data))
	for k := range secret.Data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write(secret.Data[k])
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
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
// operator-managed config-hash and secret-hash annotations are always set last.
func buildPodTemplateAnnotations(aiGateway *gatewayv1alpha1.AiGateway, configHash string, secretHash string) map[string]string {
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
	annotations["gateway.agentic-layer.ai/secret-hash"] = secretHash
	return annotations
}

// SetupWithManager sets up the controller with the Manager.
func (r *AiGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.AiGateway{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				// Only react to the default API-keys secret
				if obj.GetName() != DefaultSecretName {
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
		// Watch Guard changes so that updates to a Guard trigger re-reconciliation of all
		// AiGateway resources in the same namespace that may reference it.
		Watches(&gatewayv1alpha1.Guard{}, enqueueAiGatewaysInNamespace).
		// Watch GuardrailProvider changes for the same reason.
		Watches(&gatewayv1alpha1.GuardrailProvider{}, enqueueAiGatewaysInNamespace).
		Named(ControllerName).
		Complete(r)
}
