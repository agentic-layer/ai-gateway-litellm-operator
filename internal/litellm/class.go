/*
Copyright 2026 Agentic Layer.

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

package litellm

import (
	"context"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AiGatewayClassDefaultAnnotation marks an AiGatewayClass as the default class.
const AiGatewayClassDefaultAnnotation = "aigatewayclass.kubernetes.io/is-default-class"

// IsAiGatewayOwnedByController reports whether the given AiGateway is owned by the
// controller named controllerName. An explicit spec.aiGatewayClassName must match an
// AiGatewayClass with that controller; when empty, the gateway is claimed iff an
// AiGatewayClass owned by this controller carries the default-class annotation.
//
// Returns (false, nil) if the gateway is not ours; returns an error only when listing
// AiGatewayClasses fails so callers can requeue rather than silently drop the object.
func IsAiGatewayOwnedByController(ctx context.Context, c client.Reader, gw *gatewayv1alpha1.AiGateway, controllerName string) (bool, error) {
	var classList gatewayv1alpha1.AiGatewayClassList
	if err := c.List(ctx, &classList); err != nil {
		return false, err
	}

	owned := make([]gatewayv1alpha1.AiGatewayClass, 0, len(classList.Items))
	for _, cls := range classList.Items {
		if cls.Spec.Controller == controllerName {
			owned = append(owned, cls)
		}
	}

	if className := gw.Spec.AiGatewayClassName; className != "" {
		for _, cls := range owned {
			if cls.Name == className {
				return true, nil
			}
		}
		return false, nil
	}

	for _, cls := range owned {
		if cls.Annotations[AiGatewayClassDefaultAnnotation] == "true" {
			return true, nil
		}
	}
	return false, nil
}
