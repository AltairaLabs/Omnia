package controller

import (
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ptrI32(v int32) *int32 { return &v }

func TestFacadePortProtocols(t *testing.T) {
	ws := omniav1alpha1.FacadeType("websocket")
	agent := &omniav1alpha1.AgentRuntime{}
	agent.Spec.Facade = omniav1alpha1.FacadeConfig{
		Type: ws, Port: ptrI32(8080),
		A2A: &omniav1alpha1.A2AConfig{Enabled: true, Port: ptrI32(9999)},
		MCP: &omniav1alpha1.MCPConfig{Enabled: true, Port: ptrI32(9998)},
	}
	got := facadePortProtocols(agent)
	want := map[int32]string{8080: "websocket", 9999: "a2a", 9998: "mcp"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for p, proto := range want {
		if got[p] != proto {
			t.Errorf("port %d: got %q want %q", p, got[p], proto)
		}
	}
}

func TestFacadePortProtocolsGRPCSkipped(t *testing.T) {
	agent := &omniav1alpha1.AgentRuntime{}
	agent.Spec.Facade = omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeType("grpc"), Port: ptrI32(8080)}
	if got := facadePortProtocols(agent); len(got) != 0 {
		t.Fatalf("grpc primary should be skipped, got %v", got)
	}
}

func TestCanonicalFacadePath(t *testing.T) {
	cases := []struct {
		protocol string
		want     string
	}{
		{omniav1alpha1.FacadeProtocolWebSocket, "/ws"},
		{omniav1alpha1.FacadeProtocolA2A, "/a2a"},
		{omniav1alpha1.FacadeProtocolMCP, "/mcp"},
		{omniav1alpha1.FacadeProtocolREST, ""},
	}
	for _, c := range cases {
		if got := canonicalFacadePath(c.protocol); got != c.want {
			t.Errorf("canonicalFacadePath(%q) = %q, want %q", c.protocol, got, c.want)
		}
	}
}

func TestFacadePortProtocolsDefaultPorts(t *testing.T) {
	agent := &omniav1alpha1.AgentRuntime{}
	agent.Spec.Facade = omniav1alpha1.FacadeConfig{
		Type: omniav1alpha1.FacadeType(omniav1alpha1.FacadeProtocolWebSocket),
		// Port, A2A.Port, MCP.Port all nil — defaults should apply.
		A2A: &omniav1alpha1.A2AConfig{Enabled: true},
		MCP: &omniav1alpha1.MCPConfig{Enabled: true},
	}
	got := facadePortProtocols(agent)
	want := map[int32]string{8080: "websocket", 9999: "a2a", 9998: "mcp"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for p, proto := range want {
		if got[p] != proto {
			t.Errorf("port %d: got %q want %q", p, got[p], proto)
		}
	}
}

func TestJoinExternalPath(t *testing.T) {
	cases := []struct{ prefix, suffix, want string }{
		{"/", "/ws", "/ws"},
		{"/my-agent", "/ws", "/my-agent/ws"},
		{"/my-agent/", "/ws", "/my-agent/ws"},
		{"/my-agent", "", "/my-agent"},
		{"", "/mcp", "/mcp"},
		{"", "", "/"},                       // both empty → root
		{"no-slash", "/ws", "/no-slash/ws"}, // no leading slash → adds one
	}
	for _, c := range cases {
		if got := joinExternalPath(c.prefix, c.suffix); got != c.want {
			t.Errorf("joinExternalPath(%q,%q)=%q want %q", c.prefix, c.suffix, got, c.want)
		}
	}
}

// --- BuildFacadeEndpoints test helpers ---

func gw(secure bool) *gatewayv1.Gateway {
	proto := gatewayv1.HTTPProtocolType
	g := &gatewayv1.Gateway{}
	if secure {
		proto = gatewayv1.HTTPSProtocolType
	}
	g.Spec.Listeners = []gatewayv1.Listener{{Name: "l", Port: 443, Protocol: proto}}
	return g
}

func hostRoute(name, ns, host, svc string, port int32, prefix string, rewrite bool) gatewayv1.HTTPRoute {
	pt := gatewayv1.PathMatchPathPrefix
	pn := gatewayv1.PortNumber(port)
	rule := gatewayv1.HTTPRouteRule{
		Matches: []gatewayv1.HTTPRouteMatch{{Path: &gatewayv1.HTTPPathMatch{Type: &pt, Value: &prefix}}},
		BackendRefs: []gatewayv1.HTTPBackendRef{{BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName(svc), Port: &pn,
			}}}},
	}
	if rewrite {
		rp := "/"
		mt := gatewayv1.PrefixMatchHTTPPathModifier
		rule.Filters = []gatewayv1.HTTPRouteFilter{{
			Type: gatewayv1.HTTPRouteFilterURLRewrite,
			URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
				Path: &gatewayv1.HTTPPathModifier{Type: mt, ReplacePrefixMatch: &rp},
			}}}
	}
	r := gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	r.Spec.Hostnames = []gatewayv1.Hostname{gatewayv1.Hostname(host)}
	r.Spec.Rules = []gatewayv1.HTTPRouteRule{rule}
	// Add a parentRef so routeScheme can attempt resolution.
	r.Spec.ParentRefs = []gatewayv1.ParentReference{{Name: "gw"}}
	return r
}

func wsAgent(name, ns string) *omniav1alpha1.AgentRuntime {
	a := &omniav1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	a.Spec.Facade = omniav1alpha1.FacadeConfig{Type: omniav1alpha1.FacadeType("websocket"), Port: ptrI32(8080)}
	return a
}

func resolverFor(g *gatewayv1.Gateway) GatewayResolver {
	return func(_ gatewayv1.ParentReference, _ string) (*gatewayv1.Gateway, bool) { return g, true }
}

// --- BuildFacadeEndpoints tests ---

func TestBuildFacadeEndpoints_HostBasedSecure(t *testing.T) {
	agent := wsAgent("my-agent", "default")
	routes := []gatewayv1.HTTPRoute{hostRoute("r", "default", "agents.example.com", "my-agent", 8080, "/", false)}
	eps := BuildFacadeEndpoints(agent, routes, resolverFor(gw(true)))
	if len(eps) != 1 {
		t.Fatalf("want 1 endpoint, got %d", len(eps))
	}
	e := eps[0]
	if e.URL != "wss://agents.example.com/ws" || e.Scheme != "wss" || e.Protocol != "websocket" || !e.Valid {
		t.Fatalf("unexpected endpoint: %+v", e)
	}
}

func TestBuildFacadeEndpoints_PathPrefixWithoutRewriteInvalid(t *testing.T) {
	agent := wsAgent("my-agent", "default")
	routes := []gatewayv1.HTTPRoute{hostRoute("r", "default", "h", "my-agent", 8080, "/my-agent", false)}
	eps := BuildFacadeEndpoints(agent, routes, resolverFor(gw(false)))
	if len(eps) != 1 || eps[0].Valid {
		t.Fatalf("want 1 invalid endpoint, got %+v", eps)
	}
	if eps[0].URL != "ws://h/my-agent/ws" || eps[0].Reason == "" {
		t.Fatalf("unexpected: %+v", eps[0])
	}
}

func TestBuildFacadeEndpoints_PathPrefixWithRewriteValid(t *testing.T) {
	agent := wsAgent("my-agent", "default")
	routes := []gatewayv1.HTTPRoute{hostRoute("r", "default", "h", "my-agent", 8080, "/my-agent", true)}
	eps := BuildFacadeEndpoints(agent, routes, resolverFor(gw(true)))
	if len(eps) != 1 || !eps[0].Valid {
		t.Fatalf("want 1 valid endpoint, got %+v", eps)
	}
}

func TestBuildFacadeEndpoints_NoMatchingBackend(t *testing.T) {
	agent := wsAgent("my-agent", "default")
	routes := []gatewayv1.HTTPRoute{hostRoute("r", "default", "h", "other-svc", 8080, "/", false)}
	if eps := BuildFacadeEndpoints(agent, routes, resolverFor(gw(false))); len(eps) != 0 {
		t.Fatalf("want 0 endpoints, got %d", len(eps))
	}
}

func TestBuildFacadeEndpoints_GatewayUnresolvable(t *testing.T) {
	agent := wsAgent("my-agent", "default")
	routes := []gatewayv1.HTTPRoute{hostRoute("r", "default", "h", "my-agent", 8080, "/", false)}
	noResolve := GatewayResolver(func(_ gatewayv1.ParentReference, _ string) (*gatewayv1.Gateway, bool) { return nil, false })
	if eps := BuildFacadeEndpoints(agent, routes, noResolve); len(eps) != 0 {
		t.Fatalf("unresolvable gateway should skip, got %d", len(eps))
	}
}

// --- multi-protocol + multi-host coverage ---

func multiProtoAgent(name, ns string) *omniav1alpha1.AgentRuntime {
	a := &omniav1alpha1.AgentRuntime{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	a.Spec.Facade = omniav1alpha1.FacadeConfig{
		Type: omniav1alpha1.FacadeType("websocket"),
		Port: ptrI32(8080),
		A2A:  &omniav1alpha1.A2AConfig{Enabled: true, Port: ptrI32(9999)},
		MCP:  &omniav1alpha1.MCPConfig{Enabled: true, Port: ptrI32(9998)},
	}
	return a
}

func makeFacadeRoute(name, ns, host, svc string, port int32, prefix string) gatewayv1.HTTPRoute {
	return hostRoute(name, ns, host, svc, port, prefix, false)
}

func TestBuildFacadeEndpoints_MultiProtocol(t *testing.T) {
	agent := multiProtoAgent("my-agent", "default")
	routes := []gatewayv1.HTTPRoute{
		makeFacadeRoute("r-ws", "default", "agents.example.com", "my-agent", 8080, "/"),
		makeFacadeRoute("r-a2a", "default", "agents.example.com", "my-agent", 9999, "/"),
		makeFacadeRoute("r-mcp", "default", "agents.example.com", "my-agent", 9998, "/"),
	}
	eps := BuildFacadeEndpoints(agent, routes, resolverFor(gw(true)))
	if len(eps) != 3 {
		t.Fatalf("want 3 endpoints (ws, a2a, mcp), got %d: %+v", len(eps), eps)
	}
	protos := map[string]bool{}
	for _, e := range eps {
		protos[e.Protocol] = true
		if !e.Valid {
			t.Errorf("endpoint %s should be valid: %+v", e.Protocol, e)
		}
	}
	for _, p := range []string{"websocket", "a2a", "mcp"} {
		if !protos[p] {
			t.Errorf("missing protocol %s", p)
		}
	}
}

func TestBuildFacadeEndpoints_MultiHostname(t *testing.T) {
	agent := wsAgent("my-agent", "default")

	pt := gatewayv1.PathMatchPathPrefix
	prefix := "/"
	port := gatewayv1.PortNumber(int32(8080))
	rule := gatewayv1.HTTPRouteRule{
		Matches: []gatewayv1.HTTPRouteMatch{{Path: &gatewayv1.HTTPPathMatch{Type: &pt, Value: &prefix}}},
		BackendRefs: []gatewayv1.HTTPBackendRef{{BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: gatewayv1.ObjectName("my-agent"), Port: &port,
			}}}},
	}
	r := gatewayv1.HTTPRoute{ObjectMeta: metav1.ObjectMeta{Name: "r", Namespace: "default"}}
	r.Spec.Hostnames = []gatewayv1.Hostname{"alpha.example.com", "beta.example.com"}
	r.Spec.Rules = []gatewayv1.HTTPRouteRule{rule}
	r.Spec.ParentRefs = []gatewayv1.ParentReference{{Name: "gw"}}

	eps := BuildFacadeEndpoints(agent, []gatewayv1.HTTPRoute{r}, resolverFor(gw(true)))
	if len(eps) != 2 {
		t.Fatalf("want 2 endpoints (one per hostname), got %d: %+v", len(eps), eps)
	}
	// Verify deterministic order (alpha < beta).
	if eps[0].Host != "alpha.example.com" || eps[1].Host != "beta.example.com" {
		t.Errorf("unexpected order: %v, %v", eps[0].Host, eps[1].Host)
	}
}

func TestBuildFacadeEndpoints_DeterministicOrder(t *testing.T) {
	agent := wsAgent("my-agent", "default")
	routes := []gatewayv1.HTTPRoute{
		makeFacadeRoute("r-z", "default", "z.example.com", "my-agent", 8080, "/"),
		makeFacadeRoute("r-a", "default", "a.example.com", "my-agent", 8080, "/"),
	}
	eps := BuildFacadeEndpoints(agent, routes, resolverFor(gw(true)))
	if len(eps) != 2 {
		t.Fatalf("want 2, got %d", len(eps))
	}
	if eps[0].Host != "a.example.com" || eps[1].Host != "z.example.com" {
		t.Errorf("not sorted: %v, %v", eps[0].Host, eps[1].Host)
	}
}
