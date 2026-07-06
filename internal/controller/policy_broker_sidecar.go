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
)

// Policy broker sidecar constants.
const (
	// PolicyBrokerContainerName is the name of the policy broker sidecar container.
	PolicyBrokerContainerName = "policy-broker"
	// DefaultPolicyBrokerImage is the default image for the policy broker sidecar.
	DefaultPolicyBrokerImage = "ghcr.io/altairalabs/omnia-policy-broker:latest"
	// DefaultPolicyBrokerPort is the decision-request listen port for the policy broker.
	DefaultPolicyBrokerPort = 8090
	// DefaultPolicyBrokerHealthPort is the health port for the policy broker.
	DefaultPolicyBrokerHealthPort = 8091
)

// buildPolicyBrokerContainer creates the policy broker sidecar container spec.
// Unlike the (retired in P2.4) policy-proxy sidecar, which would have sat
// inline in the tool-call path and proxied to the runtime, the broker has no
// upstream URL — the runtime calls it directly for a decision (see
// internal/runtime/tools/policy_broker_client.go)
// and the broker only watches ToolPolicy CRDs in the agent's namespace.
func buildPolicyBrokerContainer(
	agentRuntime *omniav1alpha1.AgentRuntime,
	brokerImage string,
	licenseAPIURL string,
) corev1.Container {
	if brokerImage == "" {
		brokerImage = DefaultPolicyBrokerImage
	}

	return corev1.Container{
		Name:            PolicyBrokerContainerName,
		Image:           brokerImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{
			{
				Name:          "policy-broker",
				ContainerPort: DefaultPolicyBrokerPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				// Named "metrics" (not "broker-health") so the broker's health
				// port is picked up by the same port-NAME contract the facade
				// and runtime use — the omnia-agents scrape job and the
				// PodMonitor both select every container port named "metrics"
				// (see deployment_builder.go), so this sidecar is scraped with
				// no scrape-config changes. The broker serves /metrics on this
				// same port (see ee/cmd/policy-broker buildHealthMux).
				Name:          metricsPortName,
				ContainerPort: DefaultPolicyBrokerHealthPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: buildPolicyBrokerEnvVars(agentRuntime, licenseAPIURL),
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz",
					Port: intstr.FromInt32(DefaultPolicyBrokerHealthPort),
				},
			},
			InitialDelaySeconds: 3,
			PeriodSeconds:       10,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: healthzPath,
					Port: intstr.FromInt32(DefaultPolicyBrokerHealthPort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       20,
		},
	}
}

// buildPolicyBrokerEnvVars creates environment variables for the policy broker
// container. OMNIA_NAMESPACE scopes the broker's ToolPolicy watch to the
// agent's namespace. OMNIA_AGENT_NAME (downward API) gives the broker's
// Prometheus metrics the same "agent" identity the facade and runtime
// containers use (internal/agent.NewMetrics), so all three sidecars' series
// join on {agent, namespace}. licenseAPIURL, when set, is passed as
// OPERATOR_API_URL so the sidecar logs a startup license nag when unlicensed
// (#1682).
func buildPolicyBrokerEnvVars(agentRuntime *omniav1alpha1.AgentRuntime, licenseAPIURL string) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{
			Name: envOmniaAgentName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPathInstanceLabel,
				},
			},
		},
		{
			Name:  "OMNIA_NAMESPACE",
			Value: agentRuntime.Namespace,
		},
		{
			Name:  "POLICY_BROKER_LISTEN_ADDR",
			Value: fmt.Sprintf(":%d", DefaultPolicyBrokerPort),
		},
		{
			Name:  "POLICY_BROKER_HEALTH_ADDR",
			Value: fmt.Sprintf(":%d", DefaultPolicyBrokerHealthPort),
		},
	}
	if licenseAPIURL != "" {
		env = append(env, corev1.EnvVar{Name: "OPERATOR_API_URL", Value: licenseAPIURL})
	}
	return env
}
