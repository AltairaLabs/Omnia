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

package facade

import (
	"testing"
)

func TestEnvPolicyProvider_WithLimit(t *testing.T) {
	p := NewEnvPolicyProvider(50)
	limits := p.GetLimits("default", "my-agent")

	if limits == nil {
		t.Fatal("expected non-nil limits")
	}
	if limits.MaxToolCallsPerSession != 50 {
		t.Errorf("MaxToolCallsPerSession = %d, want 50", limits.MaxToolCallsPerSession)
	}
}

func TestEnvPolicyProvider_ZeroLimit(t *testing.T) {
	p := NewEnvPolicyProvider(0)
	limits := p.GetLimits("default", "my-agent")

	if limits != nil {
		t.Errorf("expected nil limits for zero max, got %+v", limits)
	}
}

func TestEnvPolicyProvider_NegativeLimit(t *testing.T) {
	p := NewEnvPolicyProvider(-1)
	limits := p.GetLimits("default", "my-agent")

	if limits != nil {
		t.Errorf("expected nil limits for negative max, got %+v", limits)
	}
}

func TestEnvPolicyProvider_IgnoresNamespaceAndAgent(t *testing.T) {
	p := NewEnvPolicyProvider(10)

	// Should return the same limits regardless of namespace/agent
	l1 := p.GetLimits("ns-a", "agent-1")
	l2 := p.GetLimits("ns-b", "agent-2")

	if l1 == nil || l2 == nil {
		t.Fatal("expected non-nil limits")
	}
	if l1.MaxToolCallsPerSession != l2.MaxToolCallsPerSession {
		t.Errorf("limits differ: %d vs %d", l1.MaxToolCallsPerSession, l2.MaxToolCallsPerSession)
	}
}

func TestAgentPolicyLimits_Struct(t *testing.T) {
	limits := &AgentPolicyLimits{MaxToolCallsPerSession: 100}
	if limits.MaxToolCallsPerSession != 100 {
		t.Errorf("MaxToolCallsPerSession = %d, want 100", limits.MaxToolCallsPerSession)
	}
}
