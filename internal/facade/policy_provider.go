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

// AgentPolicyLimits holds the resolved limits from AgentPolicy for use in the facade.
type AgentPolicyLimits struct {
	// MaxToolCallsPerSession is the maximum number of tool calls allowed per session.
	// Zero means no limit.
	MaxToolCallsPerSession int32
}

// PolicyProvider supplies policy limits for agents.
// Implementations may read from K8s, a local cache, or environment variables.
type PolicyProvider interface {
	// GetLimits returns the effective limits for the given agent in a namespace.
	// Returns nil if no policy applies.
	GetLimits(namespace, agentName string) *AgentPolicyLimits
}

// WithPolicyProvider sets the policy provider for the server.
func WithPolicyProvider(p PolicyProvider) ServerOption {
	return func(s *Server) {
		s.policyProvider = p
	}
}

// EnvPolicyProvider reads policy limits from environment variables.
// This is the simplest implementation: each facade pod is for a single agent,
// so the operator can inject the limit as an env var.
type EnvPolicyProvider struct {
	maxToolCalls int32
}

// NewEnvPolicyProvider creates a provider with a fixed limit.
// If maxToolCalls <= 0, no limit is enforced.
func NewEnvPolicyProvider(maxToolCalls int32) *EnvPolicyProvider {
	return &EnvPolicyProvider{maxToolCalls: maxToolCalls}
}

// GetLimits returns the configured limits regardless of namespace/agent
// (since this facade pod serves exactly one agent).
func (p *EnvPolicyProvider) GetLimits(_, _ string) *AgentPolicyLimits {
	if p.maxToolCalls <= 0 {
		return nil
	}
	return &AgentPolicyLimits{MaxToolCallsPerSession: p.maxToolCalls}
}
