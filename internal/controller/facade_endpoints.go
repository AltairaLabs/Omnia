package controller

import (
	"strings"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	defaultFacadePort = int32(8080)
	defaultA2APort    = int32(9999)
	defaultMCPPort    = int32(9998)
)

// facadePortProtocols maps each externally-exposed Service port to its facade
// protocol. grpc primary facades are skipped (not routable via HTTPRoute).
func facadePortProtocols(agent *omniav1alpha1.AgentRuntime) map[int32]string {
	out := map[int32]string{}
	f := agent.Spec.Facade

	primaryPort := defaultFacadePort
	if f.Port != nil {
		primaryPort = *f.Port
	}
	switch string(f.Type) {
	case omniav1alpha1.FacadeProtocolWebSocket:
		out[primaryPort] = omniav1alpha1.FacadeProtocolWebSocket
	case omniav1alpha1.FacadeProtocolA2A:
		out[primaryPort] = omniav1alpha1.FacadeProtocolA2A
	case omniav1alpha1.FacadeProtocolREST:
		out[primaryPort] = omniav1alpha1.FacadeProtocolREST
		// grpc: intentionally not added (use GRPCRoute, out of scope).
	}

	if f.A2A != nil && f.A2A.Enabled && string(f.Type) != omniav1alpha1.FacadeProtocolA2A {
		p := defaultA2APort
		if f.A2A.Port != nil {
			p = *f.A2A.Port
		}
		out[p] = omniav1alpha1.FacadeProtocolA2A
	}
	if f.MCP != nil && f.MCP.Enabled {
		p := defaultMCPPort
		if f.MCP.Port != nil {
			p = *f.MCP.Port
		}
		out[p] = omniav1alpha1.FacadeProtocolMCP
	}
	return out
}

// canonicalFacadePath returns the path suffix a client appends for a protocol.
func canonicalFacadePath(protocol string) string {
	switch protocol {
	case omniav1alpha1.FacadeProtocolWebSocket:
		return "/ws"
	case omniav1alpha1.FacadeProtocolA2A:
		return "/a2a"
	case omniav1alpha1.FacadeProtocolMCP:
		return "/mcp"
	default: // rest: functions are POSTed under the route base
		return ""
	}
}

// joinExternalPath concatenates a route path prefix and a protocol suffix,
// collapsing duplicate slashes. Returns "/" only when both are empty.
func joinExternalPath(routePrefix, suffix string) string {
	combined := strings.TrimRight(routePrefix, "/") + suffix
	if combined == "" {
		return "/"
	}
	if !strings.HasPrefix(combined, "/") {
		combined = "/" + combined
	}
	for strings.Contains(combined, "//") {
		combined = strings.ReplaceAll(combined, "//", "/")
	}
	return combined
}
