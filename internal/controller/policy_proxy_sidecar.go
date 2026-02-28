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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// Policy proxy sidecar constants.
const (
	// PolicyProxyContainerName is the name of the policy proxy sidecar container.
	PolicyProxyContainerName = "policy-proxy"
	// DefaultPolicyProxyImage is the default image for the policy proxy sidecar.
	DefaultPolicyProxyImage = "ghcr.io/altairalabs/omnia-policy-proxy:latest"
	// DefaultPolicyProxyPort is the listen port for the policy proxy.
	DefaultPolicyProxyPort = 8082
	// DefaultPolicyProxyHealthPort is the health port for the policy proxy.
	DefaultPolicyProxyHealthPort = 8083
)

// extractToolRegistryName returns the tool registry name from the agent runtime spec.
func extractToolRegistryName(agentRuntime *omniav1alpha1.AgentRuntime) string {
	if agentRuntime.Spec.ToolRegistryRef == nil {
		return ""
	}
	return agentRuntime.Spec.ToolRegistryRef.Name
}

// filterPoliciesByRegistry returns policies whose selector matches the given registry.
func filterPoliciesByRegistry(policies []eev1alpha1.ToolPolicy, registry string) []eev1alpha1.ToolPolicy {
	var matched []eev1alpha1.ToolPolicy
	for i := range policies {
		if policies[i].Spec.Selector.Registry == registry {
			matched = append(matched, policies[i])
		}
	}
	return matched
}

// buildPolicyProxyContainer creates the policy proxy sidecar container spec.
func buildPolicyProxyContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	proxyImage string,
) corev1.Container {
	if proxyImage == "" {
		proxyImage = DefaultPolicyProxyImage
	}

	return corev1.Container{
		Name:            PolicyProxyContainerName,
		Image:           proxyImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "policy-proxy",
				ContainerPort: DefaultPolicyProxyPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "proxy-health",
				ContainerPort: DefaultPolicyProxyHealthPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: buildPolicyProxyEnvVars(agentRuntime),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt32(DefaultPolicyProxyHealthPort),
				},
			},
			InitialDelaySeconds: 3,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: healthzPath,
					Port: intstr.FromInt32(DefaultPolicyProxyHealthPort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       20,
		},
	}
}

// buildPolicyProxyEnvVars creates environment variables for the policy proxy container.
func buildPolicyProxyEnvVars(_ *omniav1alpha1.AgentRuntime) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "OMNIA_AGENT_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.labels['app.kubernetes.io/instance']",
				},
			},
		},
		{
			Name: "OMNIA_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name:  "POLICY_PROXY_LISTEN_ADDR",
			Value: fmt.Sprintf(":%d", DefaultPolicyProxyPort),
		},
		{
			Name:  "POLICY_PROXY_HEALTH_ADDR",
			Value: fmt.Sprintf(":%d", DefaultPolicyProxyHealthPort),
		},
		{
			Name:  "POLICY_PROXY_UPSTREAM_URL",
			Value: fmt.Sprintf("http://localhost:%d", DefaultRuntimeGRPCPort),
		},
	}
}
