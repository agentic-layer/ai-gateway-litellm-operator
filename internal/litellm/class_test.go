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
	"testing"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testController = "test.agentic-layer.ai/test-controller"

func classScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := gatewayv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func newClass(controller string, isDefault bool) *gatewayv1alpha1.AiGatewayClass {
	c := &gatewayv1alpha1.AiGatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "litellm"},
		Spec:       gatewayv1alpha1.AiGatewayClassSpec{Controller: controller},
	}
	if isDefault {
		c.Annotations = map[string]string{AiGatewayClassDefaultAnnotation: "true"}
	}
	return c
}

func TestIsAiGatewayOwned_ExplicitMatch(t *testing.T) {
	s := classScheme(t)
	cls := newClass(testController, false)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cls).Build()

	gw := &gatewayv1alpha1.AiGateway{Spec: gatewayv1alpha1.AiGatewaySpec{AiGatewayClassName: "litellm"}}
	got, err := IsAiGatewayOwnedByController(context.Background(), c, gw, testController)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("expected owned=true")
	}
}

func TestIsAiGatewayOwned_ExplicitMismatchDoesNotFallBackToDefault(t *testing.T) {
	s := classScheme(t)
	defaultCls := newClass(testController, true)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(defaultCls).Build()

	gw := &gatewayv1alpha1.AiGateway{Spec: gatewayv1alpha1.AiGatewaySpec{AiGatewayClassName: "other"}}
	got, _ := IsAiGatewayOwnedByController(context.Background(), c, gw, testController)
	if got {
		t.Errorf("expected owned=false when explicit name does not match")
	}
}

func TestIsAiGatewayOwned_EmptyClassUsesDefault(t *testing.T) {
	s := classScheme(t)
	defaultCls := newClass(testController, true)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(defaultCls).Build()

	gw := &gatewayv1alpha1.AiGateway{}
	got, _ := IsAiGatewayOwnedByController(context.Background(), c, gw, testController)
	if !got {
		t.Errorf("expected owned=true via default class")
	}
}

func TestIsAiGatewayOwned_EmptyClassNoDefault(t *testing.T) {
	s := classScheme(t)
	cls := newClass(testController, false) // not default
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cls).Build()

	gw := &gatewayv1alpha1.AiGateway{}
	got, _ := IsAiGatewayOwnedByController(context.Background(), c, gw, testController)
	if got {
		t.Errorf("expected owned=false when no default class")
	}
}

func TestIsAiGatewayOwned_OtherController(t *testing.T) {
	s := classScheme(t)
	cls := newClass("someone-else/controller", true)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cls).Build()

	gw := &gatewayv1alpha1.AiGateway{}
	got, _ := IsAiGatewayOwnedByController(context.Background(), c, gw, testController)
	if got {
		t.Errorf("expected owned=false for class belonging to another controller")
	}
}

func newToolClass(controller string, isDefault bool) *gatewayv1alpha1.ToolGatewayClass {
	c := &gatewayv1alpha1.ToolGatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "litellm"},
		Spec:       gatewayv1alpha1.ToolGatewayClassSpec{Controller: controller},
	}
	if isDefault {
		c.Annotations = map[string]string{ToolGatewayClassDefaultAnnotation: "true"}
	}
	return c
}

func TestIsToolGatewayOwned_ExplicitMatch(t *testing.T) {
	s := classScheme(t)
	cls := newToolClass(testController, false)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cls).Build()

	gw := &gatewayv1alpha1.ToolGateway{Spec: gatewayv1alpha1.ToolGatewaySpec{ToolGatewayClassName: "litellm"}}
	got, err := IsToolGatewayOwnedByController(context.Background(), c, gw, testController)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("expected owned=true")
	}
}

func TestIsToolGatewayOwned_ExplicitMismatchDoesNotFallBackToDefault(t *testing.T) {
	s := classScheme(t)
	defaultCls := newToolClass(testController, true)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(defaultCls).Build()

	gw := &gatewayv1alpha1.ToolGateway{Spec: gatewayv1alpha1.ToolGatewaySpec{ToolGatewayClassName: "other"}}
	got, _ := IsToolGatewayOwnedByController(context.Background(), c, gw, testController)
	if got {
		t.Errorf("expected owned=false when explicit name does not match")
	}
}

func TestIsToolGatewayOwned_EmptyClassUsesDefault(t *testing.T) {
	s := classScheme(t)
	defaultCls := newToolClass(testController, true)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(defaultCls).Build()

	gw := &gatewayv1alpha1.ToolGateway{}
	got, _ := IsToolGatewayOwnedByController(context.Background(), c, gw, testController)
	if !got {
		t.Errorf("expected owned=true via default class")
	}
}

func TestIsToolGatewayOwned_EmptyClassNoDefault(t *testing.T) {
	s := classScheme(t)
	cls := newToolClass(testController, false)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cls).Build()

	gw := &gatewayv1alpha1.ToolGateway{}
	got, _ := IsToolGatewayOwnedByController(context.Background(), c, gw, testController)
	if got {
		t.Errorf("expected owned=false when no default class")
	}
}

func TestIsToolGatewayOwned_OtherController(t *testing.T) {
	s := classScheme(t)
	cls := newToolClass("someone-else/controller", true)
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(cls).Build()

	gw := &gatewayv1alpha1.ToolGateway{}
	got, _ := IsToolGatewayOwnedByController(context.Background(), c, gw, testController)
	if got {
		t.Errorf("expected owned=false for class belonging to another controller")
	}
}
