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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	servicePort = 8080
	healthPort  = 8081
)

// Label key constants for service groups.
const (
	labelComponent    = "app.kubernetes.io/component"
	labelServiceGroup = "omnia.altairalabs.ai/service-group"
	// labelWorkspace is already defined in workspace_controller.go
)

// ServiceBuilder builds Deployment and Service objects for per-workspace
// session-api and memory-api instances.
type ServiceBuilder struct {
	SessionImage           string
	SessionImagePullPolicy corev1.PullPolicy
	MemoryImage            string
	MemoryImagePullPolicy  corev1.PullPolicy
}

// BuildSessionDeployment builds a Deployment for the session-api service group.
func (sb *ServiceBuilder) BuildSessionDeployment(workspaceName, namespace string, sg omniav1alpha1.WorkspaceServiceGroup) *appsv1.Deployment {
	name := fmt.Sprintf("session-%s-%s", workspaceName, sg.Name)
	labels := serviceLabels("session-api", workspaceName, sg.Name)
	args := []string{
		fmt.Sprintf("--workspace=%s", workspaceName),
		fmt.Sprintf("--service-group=%s", sg.Name),
	}
	var overrides *omniav1alpha1.PodOverrides
	if sg.Session != nil {
		overrides = sg.Session.PodOverrides
	}
	return buildServiceDeployment(name, namespace, sb.SessionImage, sb.SessionImagePullPolicy, args, labels, overrides)
}

// BuildMemoryDeployment builds a Deployment for the memory-api service group.
func (sb *ServiceBuilder) BuildMemoryDeployment(workspaceName, namespace string, sg omniav1alpha1.WorkspaceServiceGroup) *appsv1.Deployment {
	name := fmt.Sprintf("memory-%s-%s", workspaceName, sg.Name)
	labels := serviceLabels("memory-api", workspaceName, sg.Name)
	args := []string{
		fmt.Sprintf("--workspace=%s", workspaceName),
		fmt.Sprintf("--service-group=%s", sg.Name),
	}
	var overrides *omniav1alpha1.PodOverrides
	if sg.Memory != nil {
		overrides = sg.Memory.PodOverrides
	}
	return buildServiceDeployment(name, namespace, sb.MemoryImage, sb.MemoryImagePullPolicy, args, labels, overrides)
}

// BuildService builds a ClusterIP Service for the given component.
func BuildService(name, namespace, component, workspaceName, groupName string) *corev1.Service {
	labels := serviceLabels(component, workspaceName, groupName)
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       servicePort,
					TargetPort: intstr.FromString("http"),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// serviceAccountName returns the ServiceAccount name for a per-workspace service deployment.
func serviceAccountName(deploymentName string) string {
	return deploymentName
}

// ServiceURL returns the in-cluster HTTP URL for the given service.
func ServiceURL(serviceName, namespace string) string {
	return fmt.Sprintf("http://%s.%s:%d", serviceName, namespace, servicePort)
}

// buildServiceDeployment constructs a Deployment with restricted security context,
// standard health probes, and the given image and args. If overrides is non-nil,
// pod-level and container-level fields from PodOverrides are merged in.
func buildServiceDeployment(
	name, namespace, image string,
	pullPolicy corev1.PullPolicy,
	args []string,
	labels map[string]string,
	overrides *omniav1alpha1.PodOverrides,
) *appsv1.Deployment {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: serviceAccountName(name),
					Containers: []corev1.Container{
						{
							Name:            name,
							Image:           image,
							ImagePullPolicy: pullPolicy,
							Args:            args,
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: servicePort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "health",
									ContainerPort: healthPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe:   httpProbe(healthzPath, healthPort),
							ReadinessProbe:  httpProbe("/readyz", healthPort),
							SecurityContext: restrictedSecurityContext(),
						},
					},
				},
			},
		},
	}

	if overrides != nil {
		ApplyPodOverrides(&dep.Spec.Template.Spec, &dep.Spec.Template.ObjectMeta, overrides)
		for i := range dep.Spec.Template.Spec.Containers {
			ApplyContainerOverrides(&dep.Spec.Template.Spec.Containers[i], overrides)
		}
	}

	return dep
}

// serviceLabels returns the standard label set for a service group component.
func serviceLabels(component, workspaceName, groupName string) map[string]string {
	return map[string]string{
		labelComponent:    component,
		labelAppManagedBy: labelValueOmniaOperator,
		labelWorkspace:    workspaceName,
		labelServiceGroup: groupName,
	}
}

// httpProbe returns an HTTP GET probe for the given path and port.
func httpProbe(path string, port int) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt32(int32(port)),
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       10,
	}
}

// restrictedSecurityContext returns a PodSecurity restricted-compliant SecurityContext.
func restrictedSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		RunAsNonRoot:             ptr.To(true),
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}
