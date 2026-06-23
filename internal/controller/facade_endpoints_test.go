package controller

import (
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
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

func TestJoinExternalPath(t *testing.T) {
	cases := []struct{ prefix, suffix, want string }{
		{"/", "/ws", "/ws"},
		{"/my-agent", "/ws", "/my-agent/ws"},
		{"/my-agent/", "/ws", "/my-agent/ws"},
		{"/my-agent", "", "/my-agent"},
		{"", "/mcp", "/mcp"},
	}
	for _, c := range cases {
		if got := joinExternalPath(c.prefix, c.suffix); got != c.want {
			t.Errorf("joinExternalPath(%q,%q)=%q want %q", c.prefix, c.suffix, got, c.want)
		}
	}
}
