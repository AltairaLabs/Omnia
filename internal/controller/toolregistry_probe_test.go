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
	"context"
	"net"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestProbeAddress(t *testing.T) {
	cases := []struct {
		endpoint string
		want     string
		ok       bool
	}{
		{"http://svc.ns.svc.cluster.local:8080/x", "svc.ns.svc.cluster.local:8080", true},
		{"http://svc.ns/x", "svc.ns:80", true},
		{"https://petstore.example.com/v2/swagger.json", "petstore.example.com:443", true},
		{"grpc-svc.internal:50051", "grpc-svc.internal:50051", true},
		{"myhost", "", false}, // no scheme, no port — a plausible typo
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := probeAddress(tc.endpoint)
		if ok != tc.ok || got != tc.want {
			t.Errorf("probeAddress(%q) = (%q,%v), want (%q,%v)", tc.endpoint, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIsNetworkEndpoint(t *testing.T) {
	cases := map[string]bool{
		"http://svc:80/x":      true,
		"grpc-svc:50051":       true,
		"client://browser":     false,
		"stdio:///usr/bin/mcp": false,
		"":                     false,
	}
	for ep, want := range cases {
		if got := isNetworkEndpoint(ep); got != want {
			t.Errorf("isNetworkEndpoint(%q) = %v, want %v", ep, got, want)
		}
	}
}

func TestProbeRequeueAfter(t *testing.T) {
	if d := probeRequeueAfter(nil); d != 0 {
		t.Errorf("nil probe: got %v, want 0", d)
	}
	if d := probeRequeueAfter(&omniav1alpha1.ProbeConfig{Enabled: false}); d != 0 {
		t.Errorf("disabled: got %v, want 0", d)
	}
	if d := probeRequeueAfter(&omniav1alpha1.ProbeConfig{Enabled: true}); d != defaultProbeInterval {
		t.Errorf("enabled default: got %v, want %v", d, defaultProbeInterval)
	}
	custom := &metav1.Duration{Duration: 10 * time.Second}
	if d := probeRequeueAfter(&omniav1alpha1.ProbeConfig{Enabled: true, Interval: custom}); d != 10*time.Second {
		t.Errorf("enabled custom: got %v, want 10s", d)
	}
}

// closedAddr returns a 127.0.0.1:port that is guaranteed closed (a listener was
// opened to reserve the port, then closed).
func closedAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func TestProbeTools_ReachableAndUnreachable(t *testing.T) {
	// A live listener → reachable.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lis.Close() }()
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	r := &ToolRegistryReconciler{}
	tr := &omniav1alpha1.ToolRegistry{
		Spec: omniav1alpha1.ToolRegistrySpec{
			Probe: &omniav1alpha1.ProbeConfig{Enabled: true, Timeout: &metav1.Duration{Duration: time.Second}},
		},
	}
	tools := []omniav1alpha1.DiscoveredTool{
		{Name: "up", HandlerName: "up", Endpoint: "http://" + lis.Addr().String() + "/x", Status: omniav1alpha1.ToolStatusAvailable},
		{Name: "down", HandlerName: "down", Endpoint: closedAddr(t), Status: omniav1alpha1.ToolStatusAvailable},
		{Name: "browser", HandlerName: "browser", Endpoint: "client://browser", Status: omniav1alpha1.ToolStatusAvailable},
	}

	r.probeTools(context.Background(), tr, tools)

	if tools[0].Status != omniav1alpha1.ToolStatusAvailable {
		t.Errorf("reachable tool: got %q, want Available", tools[0].Status)
	}
	if tools[1].Status != omniav1alpha1.ToolStatusUnavailable {
		t.Errorf("unreachable tool: got %q, want Unavailable", tools[1].Status)
	}
	if tools[1].Error == nil {
		t.Error("unreachable tool should carry an error message")
	}
	// A non-network endpoint (client browser tool) is left untouched.
	if tools[2].Status != omniav1alpha1.ToolStatusAvailable || tools[2].LastChecked != nil {
		t.Errorf("client tool should not be probed; got status=%q lastChecked=%v", tools[2].Status, tools[2].LastChecked)
	}
}

func TestProbeTools_DisabledIsNoOp(t *testing.T) {
	r := &ToolRegistryReconciler{}
	tr := &omniav1alpha1.ToolRegistry{Spec: omniav1alpha1.ToolRegistrySpec{}} // no probe
	tools := []omniav1alpha1.DiscoveredTool{
		{Name: "t", HandlerName: "t", Endpoint: closedAddr(t), Status: omniav1alpha1.ToolStatusAvailable},
	}
	r.probeTools(context.Background(), tr, tools)
	if tools[0].Status != omniav1alpha1.ToolStatusAvailable || tools[0].LastChecked != nil {
		t.Errorf("disabled probe must be a no-op; got status=%q lastChecked=%v", tools[0].Status, tools[0].LastChecked)
	}
}

func TestProbeTools_MalformedEndpointUnavailable(t *testing.T) {
	r := &ToolRegistryReconciler{}
	tr := &omniav1alpha1.ToolRegistry{
		Spec: omniav1alpha1.ToolRegistrySpec{Probe: &omniav1alpha1.ProbeConfig{Enabled: true}},
	}
	tools := []omniav1alpha1.DiscoveredTool{
		{Name: "typo", HandlerName: "typo", Endpoint: "myhost", Status: omniav1alpha1.ToolStatusAvailable},
	}
	r.probeTools(context.Background(), tr, tools)
	if tools[0].Status != omniav1alpha1.ToolStatusUnavailable {
		t.Errorf("malformed endpoint should be Unavailable, got %q", tools[0].Status)
	}
	if tools[0].Error == nil {
		t.Error("malformed endpoint should carry an error")
	}
}

func TestProbeTools_DialTimeoutUnavailable(t *testing.T) {
	// Stub the dialer to simulate a slow endpoint that trips the probe timeout.
	orig := probeDialContext
	probeDialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	defer func() { probeDialContext = orig }()

	r := &ToolRegistryReconciler{}
	tr := &omniav1alpha1.ToolRegistry{
		Spec: omniav1alpha1.ToolRegistrySpec{Probe: &omniav1alpha1.ProbeConfig{
			Enabled: true, Timeout: &metav1.Duration{Duration: 20 * time.Millisecond},
		}},
	}
	tools := []omniav1alpha1.DiscoveredTool{
		{Name: "slow", HandlerName: "slow", Endpoint: "http://10.255.255.1:80/x", Status: omniav1alpha1.ToolStatusAvailable},
	}
	r.probeTools(context.Background(), tr, tools)
	if tools[0].Status != omniav1alpha1.ToolStatusUnavailable {
		t.Errorf("timed-out probe should be Unavailable, got %q", tools[0].Status)
	}
}
