/*
Copyright 2025.

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

// mcpPort returns the mcp facade's configured port, defaulting to DefaultMCPPort
// when unset. Caller has already verified isMCPEnabled (see facades.go).
func mcpPort(ar *omniav1alpha1.AgentRuntime) int32 {
	if f := facadeOfType(ar, omniav1alpha1.FacadeTypeMCP); f != nil && f.MCP != nil && f.MCP.Port != nil {
		return *f.MCP.Port
	}
	return DefaultMCPPort
}

// applyMCPFacadeOptions appends the MCP container port + env vars to
// the facade container when an mcp facade is present. CEL enforces
// function-mode; callers don't need to re-check.
func applyMCPFacadeOptions(facadeContainer *corev1.Container, ar *omniav1alpha1.AgentRuntime) {
	if !isMCPEnabled(ar) {
		return
	}
	port := mcpPort(ar)
	facadeContainer.Ports = append(facadeContainer.Ports, corev1.ContainerPort{
		Name:          portNameMCP,
		ContainerPort: port,
		Protocol:      corev1.ProtocolTCP,
	})
	facadeContainer.Env = append(facadeContainer.Env,
		corev1.EnvVar{Name: "OMNIA_MCP_ENABLED", Value: envValueTrue},
		corev1.EnvVar{Name: "OMNIA_MCP_PORT", Value: fmt.Sprintf("%d", port)},
	)
}

// appendMCPServicePort returns ports with an "mcp" entry appended when
// an mcp facade is present, otherwise returns the input slice
// unchanged. Mirrors applyMCPFacadeOptions but for the Service side.
func appendMCPServicePort(ports []corev1.ServicePort, ar *omniav1alpha1.AgentRuntime) []corev1.ServicePort {
	if !isMCPEnabled(ar) {
		return ports
	}
	return append(ports, corev1.ServicePort{
		Name:       portNameMCP,
		Port:       mcpPort(ar),
		TargetPort: intstr.FromString(portNameMCP),
		Protocol:   corev1.ProtocolTCP,
	})
}
