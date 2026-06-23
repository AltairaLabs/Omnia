package controller

import (
	"sort"
	"strings"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
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

// GatewayResolver resolves the parent Gateway for an HTTPRoute parentRef.
// Returns false when the Gateway cannot be found/read (endpoint is then skipped).
type GatewayResolver func(parent gatewayv1.ParentReference, routeNamespace string) (*gatewayv1.Gateway, bool)

// BuildFacadeEndpoints derives external endpoints for an agent from observed
// HTTPRoutes. Pure function: all cluster reads are done by the caller and
// passed in (routes + resolve). Deterministically sorted.
func BuildFacadeEndpoints(agent *omniav1alpha1.AgentRuntime, routes []gatewayv1.HTTPRoute, resolve GatewayResolver) []omniav1alpha1.FacadeEndpoint {
	ports := facadePortProtocols(agent)
	var out []omniav1alpha1.FacadeEndpoint
	for i := range routes {
		out = append(out, endpointsForRoute(agent, &routes[i], ports, resolve)...)
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Host != out[b].Host {
			return out[a].Host < out[b].Host
		}
		if out[a].Path != out[b].Path {
			return out[a].Path < out[b].Path
		}
		return out[a].Protocol < out[b].Protocol
	})
	return out
}

func endpointsForRoute(agent *omniav1alpha1.AgentRuntime, route *gatewayv1.HTTPRoute, ports map[int32]string, resolve GatewayResolver) []omniav1alpha1.FacadeEndpoint {
	secure, ok := routeScheme(route, resolve)
	if !ok {
		return nil // parent Gateway unresolvable -> skip
	}
	hosts := routeHosts(route)
	var out []omniav1alpha1.FacadeEndpoint
	for ri := range route.Spec.Rules {
		rule := &route.Spec.Rules[ri]
		out = append(out, endpointsForRule(route, rule, agent.Name, ports, hosts, secure)...)
	}
	return out
}

// endpointsForRule emits endpoints for each matched protocol × path × hostname in one rule.
func endpointsForRule(route *gatewayv1.HTTPRoute, rule *gatewayv1.HTTPRouteRule, agentName string, ports map[int32]string, hosts []string, secure bool) []omniav1alpha1.FacadeEndpoint {
	var out []omniav1alpha1.FacadeEndpoint
	for _, proto := range matchedProtocols(rule, agentName, ports) {
		for _, m := range matchPaths(rule, proto.port) {
			for _, host := range hosts {
				ep := makeEndpoint(route, proto.protocol, host, m.prefix, m.port, secure, m.valid, m.reason)
				if ep != nil {
					out = append(out, *ep)
				}
			}
		}
	}
	return out
}

// routeScheme inspects the route's parentRefs to determine TLS termination.
// Returns (secure, true) when at least one parent resolves; (false, false) when none resolve.
func routeScheme(route *gatewayv1.HTTPRoute, resolve GatewayResolver) (secure bool, ok bool) {
	for _, ref := range route.Spec.ParentRefs {
		gw, found := resolve(ref, route.Namespace)
		if !found {
			continue
		}
		for _, l := range gw.Spec.Listeners {
			if listenerIsSecure(l) {
				return true, true
			}
		}
		return false, true
	}
	return false, false
}

// listenerIsSecure reports whether a Gateway listener terminates TLS.
func listenerIsSecure(l gatewayv1.Listener) bool {
	return l.Protocol == gatewayv1.HTTPSProtocolType ||
		l.Protocol == gatewayv1.TLSProtocolType ||
		l.TLS != nil
}

// routeHosts returns route hostnames as strings. Falls back to [""] (no host) when empty.
func routeHosts(route *gatewayv1.HTTPRoute) []string {
	if len(route.Spec.Hostnames) == 0 {
		return []string{""}
	}
	hosts := make([]string, len(route.Spec.Hostnames))
	for i, h := range route.Spec.Hostnames {
		hosts[i] = string(h)
	}
	return hosts
}

// matchedProto pairs a protocol name with its Service port.
type matchedProto struct {
	protocol string
	port     int32
}

// matchedProtocols returns the protocols whose Service port appears in this rule's backendRefs
// and matches the agent's name.
func matchedProtocols(rule *gatewayv1.HTTPRouteRule, agentName string, ports map[int32]string) []matchedProto {
	seen := map[string]bool{}
	var out []matchedProto
	for _, br := range rule.BackendRefs {
		if string(br.Name) != agentName || br.Port == nil {
			continue
		}
		p := int32(*br.Port)
		proto, ok := ports[p]
		if !ok || seen[proto] {
			continue
		}
		seen[proto] = true
		out = append(out, matchedProto{protocol: proto, port: p})
	}
	return out
}

// pathMatch carries the computed prefix, port, validity, and reason for one path match.
type pathMatch struct {
	prefix string
	port   int32
	valid  bool
	reason string
}

// matchPaths returns one pathMatch per rule.Matches entry (or a single root match when empty).
func matchPaths(rule *gatewayv1.HTTPRouteRule, port int32) []pathMatch {
	if len(rule.Matches) == 0 {
		return []pathMatch{{prefix: "/", port: port, valid: true}}
	}
	hasPrefixRewrite := ruleHasPrefixRewrite(rule)
	var out []pathMatch
	for _, m := range rule.Matches {
		prefix, valid, reason := pathValidity(m.Path, hasPrefixRewrite)
		out = append(out, pathMatch{prefix: prefix, port: port, valid: valid, reason: reason})
	}
	return out
}

// ruleHasPrefixRewrite reports whether the rule has a URLRewrite filter with ReplacePrefixMatch.
func ruleHasPrefixRewrite(rule *gatewayv1.HTTPRouteRule) bool {
	for _, f := range rule.Filters {
		if f.Type == gatewayv1.HTTPRouteFilterURLRewrite &&
			f.URLRewrite != nil &&
			f.URLRewrite.Path != nil &&
			f.URLRewrite.Path.Type == gatewayv1.PrefixMatchHTTPPathModifier {
			return true
		}
	}
	return false
}

// pathValidity derives the prefix string, validity, and reason from a path match spec.
func pathValidity(pm *gatewayv1.HTTPPathMatch, hasPrefixRewrite bool) (prefix string, valid bool, reason string) {
	if pm == nil || pm.Type == nil || *pm.Type != gatewayv1.PathMatchPathPrefix {
		return "/", true, ""
	}
	p := "/"
	if pm.Value != nil {
		p = *pm.Value
	}
	if p != "/" && !hasPrefixRewrite {
		return p, false, omniav1alpha1.FacadeEndpointReasonPrefixNotStripped
	}
	return p, true, ""
}

// schemeFor maps a facade protocol + TLS flag to a URL scheme.
func schemeFor(protocol string, secure bool) string {
	switch protocol {
	case omniav1alpha1.FacadeProtocolWebSocket:
		if secure {
			return "wss"
		}
		return "ws"
	default:
		if secure {
			return "https"
		}
		return "http"
	}
}

// makeEndpoint builds a FacadeEndpoint. Returns nil when host is empty.
func makeEndpoint(route *gatewayv1.HTTPRoute, protocol, host, prefix string, port int32, secure, valid bool, reason string) *omniav1alpha1.FacadeEndpoint {
	if host == "" {
		return nil
	}
	scheme := schemeFor(protocol, secure)
	path := joinExternalPath(prefix, canonicalFacadePath(protocol))
	return &omniav1alpha1.FacadeEndpoint{
		Protocol:       protocol,
		URL:            scheme + "://" + host + path,
		Scheme:         scheme,
		Host:           host,
		Path:           path,
		Port:           port,
		RouteName:      route.Name,
		RouteNamespace: route.Namespace,
		Valid:          valid,
		Reason:         reason,
	}
}
