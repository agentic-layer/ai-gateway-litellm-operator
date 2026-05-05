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

// Package litellm provides shared building blocks for reconcilers that
// materialize LiteLLM proxy workloads (Deployment + Service + ConfigMap)
// from CRDs in this operator.
package litellm

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"

	gatewayv1alpha1 "github.com/agentic-layer/agent-runtime-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	Image                         = "ghcr.io/berriai/litellm:v1.83.14-stable.patch.2"
	ApiKeySecretName              = "api-key-secrets"
	DefaultRequestTimeout         = 600
	ContainerName                 = "litellm"
	PrometheusMultiprocVolumeName = "prometheus-multiproc"
	PrometheusMultiprocDir        = "/prometheus_multiproc"
)

// GatewayWorkload is the input both reconcilers fill before calling ReconcileWorkload.
// Env carries the caller's already-resolved env vars (user spec.env plus any
// CRD-specific generated entries such as API-key references); ReconcileWorkload
// passes it through MergeEnv before mounting on the container.
type GatewayWorkload struct {
	Name, Namespace string
	Owner           client.Object
	ContainerPort   int32
	ServicePort     int32
	Env             []corev1.EnvVar
	EnvFrom         []corev1.EnvFromSource
	CommonMetadata  *gatewayv1alpha1.EmbeddedMetadata
	PodMetadata     *gatewayv1alpha1.EmbeddedMetadata
	ConfigYAML      string
}

// PhaseError tags a workload-reconcile failure with which step failed.
// Callers can errors.As on this to map back to phase-specific status conditions.
type PhaseError struct {
	Phase string // "ConfigMap", "Deployment", "Service", "Secret"
	Err   error
}

func (e *PhaseError) Error() string { return fmt.Sprintf("%s reconcile failed: %v", e.Phase, e.Err) }
func (e *PhaseError) Unwrap() error { return e.Err }

// IsDeploymentRolledOut reports whether the deployment-controller has applied
// the latest spec and all desired replicas are available. The second return is
// a human-readable reason callers can use as a status-condition message when
// the rollout is still in progress. Callers should use this to gate a Ready=True
// condition so consumers do not see Ready before pods are actually serving.
func IsDeploymentRolledOut(d *appsv1.Deployment) (bool, string) {
	if d.Generation > d.Status.ObservedGeneration {
		return false, fmt.Sprintf("Deployment generation %d not yet observed (last observed: %d)",
			d.Generation, d.Status.ObservedGeneration)
	}
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	if d.Status.AvailableReplicas < desired {
		return false, fmt.Sprintf("Deployment rollout in progress: %d/%d replicas available",
			d.Status.AvailableReplicas, desired)
	}
	return true, ""
}

// ReconcileWorkload creates or updates the ConfigMap, Deployment, and Service that
// run a LiteLLM proxy for a single gateway CR (the Owner). All three are reconciled
// idempotently using controllerutil.CreateOrUpdate. The pod template carries
// config-hash and secret-hash annotations so any change to ConfigYAML or the
// api-keys secret triggers a rolling restart.
//
// On failure, the returned error is a *PhaseError tagged with which step failed.
func ReconcileWorkload(ctx context.Context, c client.Client, scheme *runtime.Scheme, w GatewayWorkload) error {
	configHash := hashYAML(w.ConfigYAML)

	if err := reconcileConfigMap(ctx, c, scheme, w); err != nil {
		return &PhaseError{Phase: "ConfigMap", Err: err}
	}

	secretHash, err := computeSecretHash(ctx, c, w.Namespace)
	if err != nil {
		return &PhaseError{Phase: "Secret", Err: err}
	}

	if err := reconcileDeployment(ctx, c, scheme, w, configHash, secretHash); err != nil {
		return &PhaseError{Phase: "Deployment", Err: err}
	}
	if err := reconcileService(ctx, c, scheme, w); err != nil {
		return &PhaseError{Phase: "Service", Err: err}
	}
	return nil
}

func hashYAML(yaml string) string {
	h := sha256.Sum256([]byte(yaml))
	return fmt.Sprintf("%x", h)[:16]
}

// findContainer returns a pointer to the named container in containers, or nil.
func findContainer(containers []corev1.Container, name string) *corev1.Container {
	for i := range containers {
		if containers[i].Name == name {
			return &containers[i]
		}
	}
	return nil
}

func reconcileConfigMap(ctx context.Context, c client.Client, scheme *runtime.Scheme, w GatewayWorkload) error {
	log := logf.FromContext(ctx)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", w.Name),
			Namespace: w.Namespace,
		},
	}
	result, err := controllerutil.CreateOrUpdate(ctx, c, cm, func() error {
		if err := controllerutil.SetControllerReference(w.Owner, cm, scheme); err != nil {
			return err
		}
		if cm.Labels == nil {
			cm.Labels = make(map[string]string)
		}
		cm.Labels["app"] = w.Name
		cm.Data = map[string]string{"config.yaml": w.ConfigYAML}
		return nil
	})
	if err != nil {
		return err
	}
	if result != controllerutil.OperationResultNone {
		log.Info("ConfigMap reconciled", "name", cm.Name, "operation", result)
	}
	return nil
}

func reconcileDeployment(ctx context.Context, c client.Client, scheme *runtime.Scheme, w GatewayWorkload, configHash, secretHash string) error {
	log := logf.FromContext(ctx)

	replicas := int32(1)
	deploymentLabels := BuildResourceLabels(w.Name, w.CommonMetadata)
	deploymentAnnotations := BuildResourceAnnotations(w.CommonMetadata)
	podTemplateLabels := BuildPodTemplateLabels(w.Name, w.CommonMetadata, w.PodMetadata)
	podTemplateAnnotations := BuildPodTemplateAnnotations(w.CommonMetadata, w.PodMetadata, configHash, secretHash)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.Name,
			Namespace: w.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, c, deployment, func() error {
		if err := controllerutil.SetControllerReference(w.Owner, deployment, scheme); err != nil {
			return err
		}
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		for k, v := range deploymentLabels {
			deployment.Labels[k] = v
		}
		if deployment.Annotations == nil {
			deployment.Annotations = make(map[string]string)
		}
		for k, v := range deploymentAnnotations {
			deployment.Annotations[k] = v
		}

		deployment.Spec.Replicas = &replicas
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{"app": w.Name},
		}

		if deployment.Spec.Template.Labels == nil {
			deployment.Spec.Template.Labels = make(map[string]string)
		}
		for k, v := range podTemplateLabels {
			deployment.Spec.Template.Labels[k] = v
		}
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		for k, v := range podTemplateAnnotations {
			deployment.Spec.Template.Annotations[k] = v
		}

		container := findContainer(deployment.Spec.Template.Spec.Containers, ContainerName)
		if container == nil {
			deployment.Spec.Template.Spec.Containers = append(deployment.Spec.Template.Spec.Containers, corev1.Container{
				Name: ContainerName,
			})
			container = &deployment.Spec.Template.Spec.Containers[len(deployment.Spec.Template.Spec.Containers)-1]
		}

		container.Image = Image
		container.Ports = []corev1.ContainerPort{
			{Name: "http", ContainerPort: w.ContainerPort, Protocol: corev1.ProtocolTCP},
		}
		container.VolumeMounts = []corev1.VolumeMount{
			{Name: "config", MountPath: "/app/config", ReadOnly: true},
			{Name: PrometheusMultiprocVolumeName, MountPath: PrometheusMultiprocDir},
		}
		container.Command = []string{
			"litellm", "--config", "/app/config/config.yaml",
			"--port", strconv.Itoa(int(w.ContainerPort)),
		}
		container.Env = MergeEnv(w.Env)
		container.EnvFrom = w.EnvFrom
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("250M"),
				corev1.ResourceCPU:    resource.MustParse("100m"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("2G"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		}
		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health/liveliness", Port: intstr.FromInt32(w.ContainerPort), Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 30, PeriodSeconds: 10, TimeoutSeconds: 5, SuccessThreshold: 1, FailureThreshold: 10,
		}
		container.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/health/readiness", Port: intstr.FromInt32(w.ContainerPort), Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 5, PeriodSeconds: 10, TimeoutSeconds: 5, SuccessThreshold: 1, FailureThreshold: 3,
		}

		deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: "config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: fmt.Sprintf("%s-config", w.Name)},
					},
				},
			},
			{
				Name:         PrometheusMultiprocVolumeName,
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
		}
		return nil
	})
	if err != nil {
		return err
	}
	if result != controllerutil.OperationResultNone {
		log.Info("Deployment reconciled", "name", deployment.Name, "operation", result)
	}
	return nil
}

func reconcileService(ctx context.Context, c client.Client, scheme *runtime.Scheme, w GatewayWorkload) error {
	log := logf.FromContext(ctx)

	serviceLabels := BuildResourceLabels(w.Name, w.CommonMetadata)
	serviceAnnotations := BuildResourceAnnotations(w.CommonMetadata)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace},
	}
	result, err := controllerutil.CreateOrUpdate(ctx, c, service, func() error {
		if err := controllerutil.SetControllerReference(w.Owner, service, scheme); err != nil {
			return err
		}
		if service.Labels == nil {
			service.Labels = make(map[string]string)
		}
		for k, v := range serviceLabels {
			service.Labels[k] = v
		}
		if service.Annotations == nil {
			service.Annotations = make(map[string]string)
		}
		for k, v := range serviceAnnotations {
			service.Annotations[k] = v
		}
		service.Spec.Selector = map[string]string{"app": w.Name}
		service.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "http",
				Port:       w.ServicePort,
				TargetPort: intstr.FromInt32(w.ContainerPort),
				Protocol:   corev1.ProtocolTCP,
			},
		}
		service.Spec.Type = corev1.ServiceTypeClusterIP
		return nil
	})
	if err != nil {
		return err
	}
	if result != controllerutil.OperationResultNone {
		log.Info("Service reconciled", "name", service.Name, "operation", result)
	}
	return nil
}
