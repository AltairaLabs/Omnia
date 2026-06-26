package v1alpha1

import "testing"

func TestFacadeStatusDeepCopy(t *testing.T) {
	in := &FacadeStatus{Endpoints: []FacadeEndpoint{{
		Protocol: FacadeProtocolWebSocket, URL: "wss://h/my-agent/ws", Scheme: "wss",
		Host: "h", Path: "/my-agent/ws", Port: 8080,
		RouteName: "r", RouteNamespace: "ns", Valid: true,
	}}}
	out := in.DeepCopy()
	out.Endpoints[0].URL = "changed"
	if in.Endpoints[0].URL == "changed" {
		t.Fatal("DeepCopy did not deep-copy Endpoints slice")
	}
}
