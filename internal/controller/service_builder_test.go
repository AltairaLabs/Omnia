/*
Copyright 2026 Altaira Labs.

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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestServiceGroup(name string) omniav1alpha1.WorkspaceServiceGroup {
	return omniav1alpha1.WorkspaceServiceGroup{
		Name: name,
		Mode: omniav1alpha1.ServiceModeManaged,
	}
}

func newTestServiceBuilder() ServiceBuilder {
	return ServiceBuilder{
		SessionImage:           "ghcr.io/altairalabs/omnia-session-api:test",
		SessionImagePullPolicy: corev1.PullIfNotPresent,
		MemoryImage:            "ghcr.io/altairalabs/omnia-memory-api:test",
		MemoryImagePullPolicy:  corev1.PullIfNotPresent,
	}
}

func TestBuildSessionDeployment(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("default")

	dep := sb.BuildSessionDeployment("my-workspace", "my-ns", sg)

	require.NotNil(t, dep)
	assert.Equal(t, "session-my-workspace-default", dep.Name)
	assert.Equal(t, "my-ns", dep.Namespace)

	// Labels
	labels := dep.Labels
	assert.Equal(t, "session-api", labels["app.kubernetes.io/component"])
	assert.Equal(t, "omnia-operator", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "my-workspace", labels["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "default", labels["omnia.altairalabs.ai/service-group"])

	// Replicas
	require.NotNil(t, dep.Spec.Replicas)
	assert.Equal(t, int32(1), *dep.Spec.Replicas)

	// Container
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, sb.SessionImage, container.Image)
	assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)

	// Args
	assert.Contains(t, container.Args, "--workspace=my-workspace")
	assert.Contains(t, container.Args, "--service-group=default")

	// Ports
	require.Len(t, container.Ports, 2)
	portNames := map[string]int32{}
	for _, p := range container.Ports {
		portNames[p.Name] = p.ContainerPort
	}
	assert.Equal(t, int32(servicePort), portNames["http"])
	assert.Equal(t, int32(healthPort), portNames["health"])

	// Probes
	require.NotNil(t, container.LivenessProbe)
	require.NotNil(t, container.LivenessProbe.HTTPGet)
	assert.Equal(t, "/healthz", container.LivenessProbe.HTTPGet.Path)
	assert.Equal(t, int32(healthPort), container.LivenessProbe.HTTPGet.Port.IntVal)

	require.NotNil(t, container.ReadinessProbe)
	require.NotNil(t, container.ReadinessProbe.HTTPGet)
	assert.Equal(t, "/readyz", container.ReadinessProbe.HTTPGet.Path)
	assert.Equal(t, int32(healthPort), container.ReadinessProbe.HTTPGet.Port.IntVal)

	// Security context
	sc := container.SecurityContext
	require.NotNil(t, sc)
	require.NotNil(t, sc.RunAsNonRoot)
	assert.True(t, *sc.RunAsNonRoot)
	require.NotNil(t, sc.AllowPrivilegeEscalation)
	assert.False(t, *sc.AllowPrivilegeEscalation)
	require.NotNil(t, sc.Capabilities)
	assert.Contains(t, sc.Capabilities.Drop, corev1.Capability("ALL"))
	require.NotNil(t, sc.SeccompProfile)
	assert.Equal(t, corev1.SeccompProfileTypeRuntimeDefault, sc.SeccompProfile.Type)
}

func TestBuildMemoryDeployment(t *testing.T) {
	sb := newTestServiceBuilder()
	sg := newTestServiceGroup("prod")

	dep := sb.BuildMemoryDeployment("acme", "acme-ns", sg)

	require.NotNil(t, dep)
	assert.Equal(t, "memory-acme-prod", dep.Name)
	assert.Equal(t, "acme-ns", dep.Namespace)

	// Labels
	labels := dep.Labels
	assert.Equal(t, "memory-api", labels["app.kubernetes.io/component"])
	assert.Equal(t, "omnia-operator", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "acme", labels["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "prod", labels["omnia.altairalabs.ai/service-group"])

	// Container
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	container := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, sb.MemoryImage, container.Image)
	assert.Equal(t, corev1.PullIfNotPresent, container.ImagePullPolicy)

	// Args
	assert.Contains(t, container.Args, "--workspace=acme")
	assert.Contains(t, container.Args, "--service-group=prod")
}

func TestBuildService(t *testing.T) {
	svc := BuildService("session-my-workspace-default", "my-ns", "session-api", "my-workspace", "default")

	require.NotNil(t, svc)
	assert.Equal(t, "session-my-workspace-default", svc.Name)
	assert.Equal(t, "my-ns", svc.Namespace)

	// Labels
	labels := svc.Labels
	assert.Equal(t, "session-api", labels["app.kubernetes.io/component"])
	assert.Equal(t, "omnia-operator", labels["app.kubernetes.io/managed-by"])
	assert.Equal(t, "my-workspace", labels["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "default", labels["omnia.altairalabs.ai/service-group"])

	// Spec
	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type)
	require.Len(t, svc.Spec.Ports, 1)
	port := svc.Spec.Ports[0]
	assert.Equal(t, int32(servicePort), port.Port)
	assert.Equal(t, "http", port.TargetPort.String())

	// Selector matches deployment labels
	assert.Equal(t, "session-api", svc.Spec.Selector["app.kubernetes.io/component"])
	assert.Equal(t, "my-workspace", svc.Spec.Selector["omnia.altairalabs.ai/workspace"])
	assert.Equal(t, "default", svc.Spec.Selector["omnia.altairalabs.ai/service-group"])
}

func TestServiceURL(t *testing.T) {
	url := ServiceURL("session-my-workspace-default", "my-ns")
	assert.Equal(t, "http://session-my-workspace-default.my-ns:8080", url)
}
