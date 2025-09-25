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

package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	gatewayv1alpha1 "github.com/agentic-layer/ai-gateway-litellm/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var modelrouterlog = logf.Log.WithName("modelrouter-resource")

// SetupModelRouterWebhookWithManager registers the webhook for ModelRouter in the manager.
func SetupModelRouterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&gatewayv1alpha1.ModelRouter{}).
		WithValidator(&ModelRouterCustomValidator{}).
		WithDefaulter(&ModelRouterCustomDefaulter{}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-gateway-agentic-layer-ai-v1alpha1-modelrouter,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.agentic-layer.ai,resources=modelrouters,verbs=create;update,versions=v1alpha1,name=modelrouter-v1alpha1.kb.io,admissionReviewVersions=v1

// ModelRouterCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind ModelRouter when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type ModelRouterCustomDefaulter struct {
}

var _ webhook.CustomDefaulter = &ModelRouterCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind ModelRouter.
func (d *ModelRouterCustomDefaulter) Default(_ context.Context, obj runtime.Object) error {
	modelrouter, ok := obj.(*gatewayv1alpha1.ModelRouter)

	if !ok {
		return fmt.Errorf("expected an ModelRouter object but got %T", obj)
	}
	modelrouterlog.Info("Defaulting for ModelRouter", "name", modelrouter.GetName())

	const DefaultPort = 4000
	if modelrouter.Spec.Port == 0 {
		modelrouter.Spec.Port = DefaultPort
	}

	return nil
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-gateway-agentic-layer-ai-v1alpha1-modelrouter,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.agentic-layer.ai,resources=modelrouters,verbs=create;update,versions=v1alpha1,name=vmodelrouter-v1alpha1.kb.io,admissionReviewVersions=v1

// ModelRouterCustomValidator struct is responsible for validating the ModelRouter resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ModelRouterCustomValidator struct {
}

var _ webhook.CustomValidator = &ModelRouterCustomValidator{}

// Use a constant for the separator to avoid magic strings.
const providerModelSeparator = "/"

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ModelRouter.
func (v *ModelRouterCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	modelRouter, ok := obj.(*gatewayv1alpha1.ModelRouter)
	if !ok {
		// This error is for the webhook runtime, not the user.
		return nil, fmt.Errorf("expected a ModelRouter object but got %T", obj)
	}
	modelrouterlog.Info("Validation for ModelRouter upon creation", "name", modelRouter.GetName())
	return v.validateModelRouterSpec(modelRouter)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ModelRouter.
func (v *ModelRouterCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	modelRouter, ok := newObj.(*gatewayv1alpha1.ModelRouter)
	if !ok {
		return nil, fmt.Errorf("expected a ModelRouter object for the newObj but got %T", newObj)
	}
	modelrouterlog.Info("Validation for ModelRouter upon update", "name", modelRouter.GetName())
	return v.validateModelRouterSpec(modelRouter)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ModelRouter.
func (v *ModelRouterCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	modelrouter, ok := obj.(*gatewayv1alpha1.ModelRouter)
	if !ok {
		return nil, fmt.Errorf("expected a ModelRouter object but got %T", obj)
	}
	modelrouterlog.Info("Validation for ModelRouter upon deletion", "name", modelrouter.GetName())

	return nil, nil
}

// validateModelRouterSpec contains the core validation logic for the ModelRouter spec.
// It's called by both ValidateCreate and ValidateUpdate.
func (v *ModelRouterCustomValidator) validateModelRouterSpec(modelRouter *gatewayv1alpha1.ModelRouter) (admission.Warnings, error) {
	// Validate type is specified (allow any non-empty string - other operators may implement different types)
	if modelRouter.Spec.Type == "" {
		return nil, errors.New("model router must specify a type")
	}

	// Validate port is positive
	if modelRouter.Spec.Port <= 0 {
		return nil, fmt.Errorf("modelRouter port must be positive, got: %d", modelRouter.Spec.Port)
	}

	// Validate at least one AI model is specified
	if len(modelRouter.Spec.AiModels) == 0 {
		return nil, errors.New("no AI models specified in ModelRouter")
	}

	// Validate AI model names
	for _, model := range modelRouter.Spec.AiModels {
		if model.Name == "" {
			return nil, errors.New("AI model name cannot be empty")
		}

		provider, modelName, found := strings.Cut(model.Name, providerModelSeparator)

		// Handle malformed names like "gpt-4" (no separator) or "/gpt-4" (empty provider).
		if !found || provider == "" || modelName == "" {
			return nil, fmt.Errorf("model %q is malformed; format must be 'provider/model-name'", model.Name)
		}

		// LiteLLM supports a vast number of providers, so we don't validate against a specific list.
		// The controller will handle provider-specific configuration and the LiteLLM proxy will
		// validate the actual model availability at runtime.
	}

	return nil, nil
}
