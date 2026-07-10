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
	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// facadeOfType returns the facade entry of the given protocol type, or nil when
// absent. CEL guarantees at most one facade per type.
func facadeOfType(ar *omniav1alpha1.AgentRuntime, t omniav1alpha1.FacadeType) *omniav1alpha1.FacadeConfig {
	for i := range ar.Spec.Facades {
		if ar.Spec.Facades[i].Type == t {
			return &ar.Spec.Facades[i]
		}
	}
	return nil
}

// primaryFacade returns the facade the agent pod dispatches to: websocket
// (agent mode) > rest (function mode) > a2a (standalone agent mode) > custom
// (bring-your-own-container agent mode). A custom facade replaces the built-in
// facade container with a third-party image, so it is the primary surface when
// no built-in agent/function facade is present. Returns nil only when
// spec.facades is empty, which CEL forbids.
func primaryFacade(ar *omniav1alpha1.AgentRuntime) *omniav1alpha1.FacadeConfig {
	if f := facadeOfType(ar, omniav1alpha1.FacadeTypeWebSocket); f != nil {
		return f
	}
	if f := facadeOfType(ar, omniav1alpha1.FacadeTypeREST); f != nil {
		return f
	}
	if f := facadeOfType(ar, omniav1alpha1.FacadeTypeA2A); f != nil {
		return f
	}
	return facadeOfType(ar, omniav1alpha1.FacadeTypeCustom)
}

// primaryFacadePort returns the primary facade's listen port, defaulting to
// DefaultFacadePort when unset.
func primaryFacadePort(ar *omniav1alpha1.AgentRuntime) int32 {
	f := primaryFacade(ar)
	if f != nil && f.Port != nil {
		return *f.Port
	}
	return int32(DefaultFacadePort)
}

// a2aConfig returns the A2A sub-config from the a2a facade entry, or nil when no
// a2a facade is present.
func a2aConfig(ar *omniav1alpha1.AgentRuntime) *omniav1alpha1.A2AConfig {
	f := facadeOfType(ar, omniav1alpha1.FacadeTypeA2A)
	if f == nil {
		return nil
	}
	return f.A2A
}

// a2aSecondaryPort returns the port the dual-protocol secondary A2A listener
// binds, defaulting to DefaultA2APort.
func a2aSecondaryPort(ar *omniav1alpha1.AgentRuntime) int32 {
	if a2a := a2aConfig(ar); a2a != nil && a2a.Port != nil {
		return *a2a.Port
	}
	return int32(DefaultA2APort)
}

// isDualProtocol reports whether the agent runs A2A as a secondary listener
// alongside a websocket primary.
func isDualProtocol(ar *omniav1alpha1.AgentRuntime) bool {
	return facadeOfType(ar, omniav1alpha1.FacadeTypeWebSocket) != nil &&
		facadeOfType(ar, omniav1alpha1.FacadeTypeA2A) != nil
}

// isStandaloneA2A reports whether A2A is the primary facade (no websocket).
func isStandaloneA2A(ar *omniav1alpha1.AgentRuntime) bool {
	return facadeOfType(ar, omniav1alpha1.FacadeTypeWebSocket) == nil &&
		facadeOfType(ar, omniav1alpha1.FacadeTypeA2A) != nil
}

// isMCPEnabled reports whether the agent has an mcp facade.
func isMCPEnabled(ar *omniav1alpha1.AgentRuntime) bool {
	return facadeOfType(ar, omniav1alpha1.FacadeTypeMCP) != nil
}

// facadeManagementEnabled reports whether the given facade serves its internal
// management-plane twin (nil facade => false).
func facadeManagementEnabled(f *omniav1alpha1.FacadeConfig) bool {
	return f != nil && f.ManagementPlaneEnabled()
}

// anyManagementPlaneEnabled reports whether at least one facade serves a
// management-plane twin — i.e. whether the agent is reachable over the
// management plane at all.
func anyManagementPlaneEnabled(ar *omniav1alpha1.AgentRuntime) bool {
	for i := range ar.Spec.Facades {
		if ar.Spec.Facades[i].ManagementPlaneEnabled() {
			return true
		}
	}
	return false
}
