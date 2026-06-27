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
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// wsOnlyAR is the simplest valid agent: a single websocket facade with the
// management plane on by default.
func wsOnlyAR() *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{{Type: omniav1alpha1.FacadeTypeWebSocket}},
		},
	}
}

func dualProtocolAR() *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{
				{Type: omniav1alpha1.FacadeTypeWebSocket},
				{Type: omniav1alpha1.FacadeTypeA2A, A2A: &omniav1alpha1.A2AConfig{}},
			},
		},
	}
}

func mcpAR() *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facades: []omniav1alpha1.FacadeConfig{
				{Type: omniav1alpha1.FacadeTypeREST},
				{Type: omniav1alpha1.FacadeTypeMCP, MCP: &omniav1alpha1.MCPConfig{Enabled: true}},
			},
		},
	}
}

func portNames(ports []corev1.ServicePort) map[string]int32 {
	out := make(map[string]int32, len(ports))
	for _, p := range ports {
		out[p.Name] = p.Port
	}
	return out
}

func TestAppendManagementServicePorts(t *testing.T) {
	t.Run("WS-only agent gets facade-mgmt when allowed (default)", func(t *testing.T) {
		ports := appendManagementServicePorts(nil, wsOnlyAR())
		got := portNames(ports)
		if got[portNameFacadeMgmt] != DefaultInternalFacadePort {
			t.Errorf("facade-mgmt port = %d, want %d", got[portNameFacadeMgmt], DefaultInternalFacadePort)
		}
		if _, ok := got[portNameA2AMgmt]; ok {
			t.Error("a2a-mgmt should be absent for a non-dual-protocol agent")
		}
	})

	t.Run("dual-protocol agent also gets a2a-mgmt", func(t *testing.T) {
		got := portNames(appendManagementServicePorts(nil, dualProtocolAR()))
		if got[portNameA2AMgmt] != DefaultInternalA2APort {
			t.Errorf("a2a-mgmt port = %d, want %d", got[portNameA2AMgmt], DefaultInternalA2APort)
		}
	})

	t.Run("mcp agent also gets mcp-mgmt", func(t *testing.T) {
		got := portNames(appendManagementServicePorts(nil, mcpAR()))
		if got[portNameMCPMgmt] != DefaultInternalMCPPort {
			t.Errorf("mcp-mgmt port = %d, want %d", got[portNameMCPMgmt], DefaultInternalMCPPort)
		}
	})

	t.Run("every facade managementPlane=false adds no ports", func(t *testing.T) {
		ar := dualProtocolAR()
		for i := range ar.Spec.Facades {
			ar.Spec.Facades[i].ManagementPlane = ptr.To(false)
		}
		ports := appendManagementServicePorts(nil, ar)
		if len(ports) != 0 {
			t.Errorf("got %d ports, want 0 when management plane disabled", len(ports))
		}
	})
}

func TestManagementEndpointsStatus(t *testing.T) {
	t.Run("WS-only agent advertises ws only", func(t *testing.T) {
		me := managementEndpointsStatus(wsOnlyAR())
		if me == nil || me.WS == nil || *me.WS != DefaultInternalFacadePort {
			t.Fatalf("WS = %v, want %d", me, DefaultInternalFacadePort)
		}
		if me.A2A != nil || me.MCP != nil {
			t.Errorf("A2A/MCP should be nil for a WS-only agent: %+v", me)
		}
	})

	t.Run("dual-protocol advertises ws+a2a", func(t *testing.T) {
		me := managementEndpointsStatus(dualProtocolAR())
		if me == nil || me.A2A == nil || *me.A2A != DefaultInternalA2APort {
			t.Fatalf("A2A = %v, want %d", me, DefaultInternalA2APort)
		}
	})

	t.Run("mcp advertises ws+mcp", func(t *testing.T) {
		me := managementEndpointsStatus(mcpAR())
		if me == nil || me.MCP == nil || *me.MCP != DefaultInternalMCPPort {
			t.Fatalf("MCP = %v, want %d", me, DefaultInternalMCPPort)
		}
	})

	t.Run("disabled returns nil", func(t *testing.T) {
		ar := wsOnlyAR()
		ar.Spec.Facades[0].ManagementPlane = ptr.To(false)
		if me := managementEndpointsStatus(ar); me != nil {
			t.Errorf("got %+v, want nil when management plane disabled", me)
		}
	})
}
