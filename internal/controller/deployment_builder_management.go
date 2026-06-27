/*
Copyright 2026.

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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// primaryManagementEnabled reports whether the primary facade (the one bound to
// the facade port) serves its internal management-plane twin. Gating is
// per-facade now (facades[].managementPlane, default true); the facade derives
// the matching internal ports from the same field (see internal/agent
// applyManagementPlanePorts), so both sides stay in sync.
func primaryManagementEnabled(ar *omniav1alpha1.AgentRuntime) bool {
	return facadeManagementEnabled(primaryFacade(ar))
}

// appendManagementServicePorts appends the internal management-plane Service
// ports for each facade with managementPlane enabled (the default). The primary
// facade (websocket / standalone-a2a / rest) maps to the facade-mgmt port; a
// dual-protocol secondary a2a to a2a-mgmt; an mcp facade to mcp-mgmt. TargetPort
// is numeric (the facade listens on the internal port directly), and the Service
// stays ClusterIP — the internal ports are never exposed via an external
// Gateway/HTTPRoute.
func appendManagementServicePorts(ports []corev1.ServicePort, ar *omniav1alpha1.AgentRuntime) []corev1.ServicePort {
	if primaryManagementEnabled(ar) {
		ports = append(ports, corev1.ServicePort{
			Name:       portNameFacadeMgmt,
			Port:       DefaultInternalFacadePort,
			TargetPort: intstr.FromInt32(DefaultInternalFacadePort),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	if isDualProtocol(ar) && facadeManagementEnabled(facadeOfType(ar, omniav1alpha1.FacadeTypeA2A)) {
		ports = append(ports, corev1.ServicePort{
			Name:       portNameA2AMgmt,
			Port:       DefaultInternalA2APort,
			TargetPort: intstr.FromInt32(DefaultInternalA2APort),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	if facadeManagementEnabled(facadeOfType(ar, omniav1alpha1.FacadeTypeMCP)) {
		ports = append(ports, corev1.ServicePort{
			Name:       portNameMCPMgmt,
			Port:       DefaultInternalMCPPort,
			TargetPort: intstr.FromInt32(DefaultInternalMCPPort),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	return ports
}

// setAgentPortAppProtocols stamps appProtocol on every agent Service port so an
// Istio waypoint/sidecar classifies them for L7 routing (required for mode=mesh
// weighted routing and the facade WebSocket upgrade). Every facade protocol
// (websocket/a2a/rest/mcp) is HTTP, so all ports get appProtocolHTTP.
func setAgentPortAppProtocols(ports []corev1.ServicePort) {
	for i := range ports {
		ap := appProtocolHTTP
		ports[i].AppProtocol = &ap
	}
}

// managementEndpointsStatus returns the status.managementEndpoints for the
// agent, or nil when no facade serves a management-plane twin. Callers (the
// dashboard, in-cluster service principals) read this to dial the agent over the
// management plane rather than computing the port from the external port. The WS
// field carries the primary facade-mgmt port (websocket, standalone-a2a, or
// rest); A2A the dual-protocol secondary a2a twin; MCP the mcp twin.
func managementEndpointsStatus(ar *omniav1alpha1.AgentRuntime) *omniav1alpha1.ManagementEndpoints {
	me := &omniav1alpha1.ManagementEndpoints{}
	if primaryManagementEnabled(ar) {
		me.WS = ptr.To(int32(DefaultInternalFacadePort))
	}
	if isDualProtocol(ar) && facadeManagementEnabled(facadeOfType(ar, omniav1alpha1.FacadeTypeA2A)) {
		me.A2A = ptr.To(int32(DefaultInternalA2APort))
	}
	if facadeManagementEnabled(facadeOfType(ar, omniav1alpha1.FacadeTypeMCP)) {
		me.MCP = ptr.To(int32(DefaultInternalMCPPort))
	}
	if me.WS == nil && me.A2A == nil && me.MCP == nil {
		return nil
	}
	return me
}
