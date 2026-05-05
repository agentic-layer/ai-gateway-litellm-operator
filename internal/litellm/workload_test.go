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
	"errors"
	"strings"
	"testing"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func workloadScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := gatewayv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatalf("corev1: %v", err)
	}
	if err := appsv1.AddToScheme(s); err != nil {
		t.Fatalf("appsv1: %v", err)
	}
	return s
}

func newOwner(name, ns string) *gatewayv1alpha1.AiGateway {
	return &gatewayv1alpha1.AiGateway{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, UID: "owner-uid-123"},
	}
}

func TestReconcileWorkload_CreatesAllThree_AiGatewayShape(t *testing.T) {
	s := workloadScheme(t)
	owner := newOwner("gw", "default")
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(owner).Build()

	w := GatewayWorkload{
		Name:          "gw",
		Namespace:     "default",
		Owner:         owner,
		ContainerPort: 80,
		ServicePort:   80,
		Env:           []corev1.EnvVar{{Name: "FOO", Value: "bar"}},
		ConfigYAML:    "model_list: []\n",
	}
	if err := ReconcileWorkload(context.Background(), c, s, w); err != nil {
		t.Fatalf("ReconcileWorkload: %v", err)
	}

	var cm corev1.ConfigMap
	if err := c.Get(context.Background(), types.NamespacedName{Name: "gw-config", Namespace: "default"}, &cm); err != nil {
		t.Fatalf("ConfigMap not found: %v", err)
	}
	if !strings.Contains(cm.Data["config.yaml"], "model_list") {
		t.Errorf("ConfigMap content missing")
	}

	var dep appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Name: "gw", Namespace: "default"}, &dep); err != nil {
		t.Fatalf("Deployment not found: %v", err)
	}
	container := dep.Spec.Template.Spec.Containers[0]
	if container.Ports[0].ContainerPort != 80 {
		t.Errorf("ContainerPort: want 80, got %d", container.Ports[0].ContainerPort)
	}
	if dep.Spec.Template.Annotations[configHashAnnotation] == "" {
		t.Errorf("config-hash annotation not set")
	}

	var svc corev1.Service
	if err := c.Get(context.Background(), types.NamespacedName{Name: "gw", Namespace: "default"}, &svc); err != nil {
		t.Fatalf("Service not found: %v", err)
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("Service.Port: want 80, got %d", svc.Spec.Ports[0].Port)
	}
	if svc.Spec.Ports[0].TargetPort.IntVal != 80 {
		t.Errorf("Service.TargetPort: want 80, got %d", svc.Spec.Ports[0].TargetPort.IntVal)
	}
}

func TestReconcileWorkload_ToolGatewayShape_DecoupledPorts(t *testing.T) {
	s := workloadScheme(t)
	owner := newOwner("tg", "tool-gateway")
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(owner).Build()

	w := GatewayWorkload{
		Name:          "tg",
		Namespace:     "tool-gateway",
		Owner:         owner,
		ContainerPort: 4000,
		ServicePort:   80,
		ConfigYAML:    "mcp_servers: {}\n",
	}
	if err := ReconcileWorkload(context.Background(), c, s, w); err != nil {
		t.Fatalf("ReconcileWorkload: %v", err)
	}

	var dep appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Name: "tg", Namespace: "tool-gateway"}, &dep); err != nil {
		t.Fatalf("Deployment not found: %v", err)
	}
	if dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort != 4000 {
		t.Errorf("ContainerPort: want 4000, got %d", dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort)
	}

	var svc corev1.Service
	if err := c.Get(context.Background(), types.NamespacedName{Name: "tg", Namespace: "tool-gateway"}, &svc); err != nil {
		t.Fatalf("Service not found: %v", err)
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("Service.Port: want 80, got %d", svc.Spec.Ports[0].Port)
	}
	if svc.Spec.Ports[0].TargetPort.IntVal != 4000 {
		t.Errorf("Service.TargetPort: want 4000, got %d", svc.Spec.Ports[0].TargetPort.IntVal)
	}
}

// TestReconcileWorkload_InjectsPrometheusMultiprocDir guarantees that any caller
// of ReconcileWorkload — even one that passes nil/no Env — gets the
// PROMETHEUS_MULTIPROC_DIR env var the prometheus_client multi-process exporter
// requires. The container always mounts /prometheus_multiproc, so a missing env
// var would crash the pod or silently lose metrics.
func TestReconcileWorkload_InjectsPrometheusMultiprocDir(t *testing.T) {
	s := workloadScheme(t)
	owner := newOwner("tg", "tool-gateway")
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(owner).Build()

	w := GatewayWorkload{
		Name: "tg", Namespace: "tool-gateway", Owner: owner,
		ContainerPort: 4000, ServicePort: 80,
		// Env intentionally nil to model a ToolGateway whose user spec has no env.
		ConfigYAML: "mcp_servers: {}\n",
	}
	if err := ReconcileWorkload(context.Background(), c, s, w); err != nil {
		t.Fatalf("ReconcileWorkload: %v", err)
	}
	var dep appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Name: "tg", Namespace: "tool-gateway"}, &dep); err != nil {
		t.Fatalf("Deployment not found: %v", err)
	}
	var found bool
	for _, e := range dep.Spec.Template.Spec.Containers[0].Env {
		if e.Name == "PROMETHEUS_MULTIPROC_DIR" {
			if e.Value != PrometheusMultiprocDir {
				t.Errorf("PROMETHEUS_MULTIPROC_DIR = %q; want %q", e.Value, PrometheusMultiprocDir)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("PROMETHEUS_MULTIPROC_DIR not injected into container env: %v", dep.Spec.Template.Spec.Containers[0].Env)
	}
}

func TestReconcileWorkload_PhaseErrorTagsConfigMapFailure(t *testing.T) {
	// Use a scheme without corev1 to force the ConfigMap apply to fail with
	// a "no kind is registered" error.
	s := runtime.NewScheme()
	if err := gatewayv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	owner := newOwner("gw", "default")
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(owner).Build()

	w := GatewayWorkload{
		Name: "gw", Namespace: "default", Owner: owner,
		ContainerPort: 80, ServicePort: 80, ConfigYAML: "",
	}
	err := ReconcileWorkload(context.Background(), c, s, w)
	if err == nil {
		t.Fatalf("expected error")
	}
	var pe *PhaseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected PhaseError; got %T: %v", err, err)
	}
	if pe.Phase != "ConfigMap" {
		t.Errorf("Phase: want ConfigMap, got %q", pe.Phase)
	}
}
