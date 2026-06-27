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

// managementPlaneEnabled reports whether the agent serves the internal
// management-plane twin listeners (externalAuth.allowManagementPlane, default
// true). The facade derives the matching internal ports from the same CRD field
// (see internal/agent applyManagementPlanePorts), so both sides stay in sync.
func managementPlaneEnabled(ar *omniav1alpha1.AgentRuntime) bool {
	return ar.Spec.ExternalAuth.ManagementPlaneAllowed()
}

// appendManagementServicePorts appends the internal management-plane Service
// ports for each enabled surface when the management plane is allowed. WS is
// always present; A2A on dual-protocol agents; MCP on MCP-enabled agents. The
// TargetPort is numeric (the facade listens on the internal port directly), and
// the Service stays ClusterIP — the internal ports are never exposed via an
// external Gateway/HTTPRoute.
func appendManagementServicePorts(ports []corev1.ServicePort, ar *omniav1alpha1.AgentRuntime) []corev1.ServicePort {
	if !managementPlaneEnabled(ar) {
		return ports
	}
	ports = append(ports, corev1.ServicePort{
		Name:       portNameFacadeMgmt,
		Port:       DefaultInternalFacadePort,
		TargetPort: intstr.FromInt32(DefaultInternalFacadePort),
		Protocol:   corev1.ProtocolTCP,
	})
	if isDualProtocol(ar) {
		ports = append(ports, corev1.ServicePort{
			Name:       portNameA2AMgmt,
			Port:       DefaultInternalA2APort,
			TargetPort: intstr.FromInt32(DefaultInternalA2APort),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	if isMCPEnabled(ar) {
		ports = append(ports, corev1.ServicePort{
			Name:       portNameMCPMgmt,
			Port:       DefaultInternalMCPPort,
			TargetPort: intstr.FromInt32(DefaultInternalMCPPort),
			Protocol:   corev1.ProtocolTCP,
		})
	}
	return ports
}

// facadeTypeOrDefault returns the agent's facade type, defaulting to websocket
// (the implicit default when spec.facade.type is unset).
func facadeTypeOrDefault(ar *omniav1alpha1.AgentRuntime) omniav1alpha1.FacadeType {
	if ar.Spec.Facade.Type != "" {
		return ar.Spec.Facade.Type
	}
	return omniav1alpha1.FacadeTypeWebSocket
}

// agentPortAppProtocol is the single source of truth mapping an agent Service
// port to its Istio appProtocol. Facades added over time get classified here in
// one place; anything unrecognised falls back to http (HTTP/1.1) so a waypoint
// still does L7 rather than silently treating a new port as opaque TCP. Only the
// primary "facade" port varies by facade type (gRPC is HTTP/2); a2a (JSON-RPC)
// and mcp (HTTP/SSE) are HTTP.
func agentPortAppProtocol(portName string, facadeType omniav1alpha1.FacadeType) string {
	if portName == portNameFacade && facadeType == omniav1alpha1.FacadeTypeGRPC {
		return appProtocolGRPC
	}
	return appProtocolHTTP
}

// setAgentPortAppProtocols stamps appProtocol on every agent Service port so an
// Istio waypoint/sidecar classifies them for L7 routing (required for mode=mesh
// weighted routing and the facade WebSocket upgrade). One pass over the
// assembled ports keeps new facades from being missed.
func setAgentPortAppProtocols(ports []corev1.ServicePort, facadeType omniav1alpha1.FacadeType) {
	for i := range ports {
		ap := agentPortAppProtocol(ports[i].Name, facadeType)
		ports[i].AppProtocol = &ap
	}
}

// managementEndpointsStatus returns the status.managementEndpoints for the
// agent, or nil when the management plane is disabled. Callers (the dashboard,
// in-cluster service principals) read this to dial the agent over the
// management plane rather than computing the port from the external port.
func managementEndpointsStatus(ar *omniav1alpha1.AgentRuntime) *omniav1alpha1.ManagementEndpoints {
	if !managementPlaneEnabled(ar) {
		return nil
	}
	me := &omniav1alpha1.ManagementEndpoints{WS: ptr.To(int32(DefaultInternalFacadePort))}
	if isDualProtocol(ar) {
		me.A2A = ptr.To(int32(DefaultInternalA2APort))
	}
	if isMCPEnabled(ar) {
		me.MCP = ptr.To(int32(DefaultInternalMCPPort))
	}
	return me
}
