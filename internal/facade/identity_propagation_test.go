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

package facade

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altairalabs/omnia/pkg/policy"
)

// buildOutboundMetadata is the facade-side #1769 wiring assertion: it runs the
// real buildConnectionContext and returns the gRPC metadata the facade would
// forward to the runtime, so a test can prove origin/workspace are actually on
// the wire (not merely settable on the struct).
func buildOutboundMetadata(
	t *testing.T,
	agentCtx requestAgentContext,
	authIdentity *policy.AuthenticatedIdentity,
) map[string]string {
	t.Helper()
	server, _ := newTestServer(t, nil)
	r := httptest.NewRequest(http.MethodGet, "/agent?agent=agent-1", nil)
	ctx := server.buildConnectionContext(r, agentCtx, requestUserContext{userID: "user-1"}, authIdentity)
	return policy.ToGRPCMetadata(ctx)
}

// TestBuildConnectionContext_PropagatesOriginFromValidator proves the origin
// set by the admitting validator reaches the outbound gRPC metadata. Before
// #1769 there was no header for it, so identity.origin was empty at the broker.
func TestBuildConnectionContext_PropagatesOriginFromValidator(t *testing.T) {
	md := buildOutboundMetadata(t,
		requestAgentContext{agentName: "agent-1", namespace: "ns-1", workspaceName: "acme"},
		&policy.AuthenticatedIdentity{Origin: policy.OriginAPIKey},
	)
	assert.Equal(t, policy.OriginAPIKey, md[policy.HeaderOrigin])
}

// TestBuildConnectionContext_WorkspaceFromTokenWins proves a workspace-scoped
// token (e.g. the management plane) propagates its own workspace claim, not the
// agent's deployed workspace.
func TestBuildConnectionContext_WorkspaceFromTokenWins(t *testing.T) {
	md := buildOutboundMetadata(t,
		requestAgentContext{agentName: "agent-1", namespace: "ns-1", workspaceName: "deployed-ws"},
		&policy.AuthenticatedIdentity{Origin: policy.OriginManagementPlane, Workspace: "token-ws"},
	)
	assert.Equal(t, "token-ws", md[policy.HeaderWorkspace])
}

// TestBuildConnectionContext_WorkspaceFallsBackToAgent proves that when the
// validator carries no workspace scope (shared-token, api-key, oidc, edge),
// identity.workspace falls back to the agent's deployed workspace so the field
// is non-empty for every validator style (the #1769 acceptance criterion).
func TestBuildConnectionContext_WorkspaceFallsBackToAgent(t *testing.T) {
	md := buildOutboundMetadata(t,
		requestAgentContext{agentName: "agent-1", namespace: "ns-1", workspaceName: "acme"},
		&policy.AuthenticatedIdentity{Origin: policy.OriginSharedToken}, // no Workspace
	)
	assert.Equal(t, "acme", md[policy.HeaderWorkspace],
		"workspace must fall back to the agent's deployed workspace when the token has no scope")
}

// TestIdentityScope covers the resolution helper directly, including the
// no-identity (unauthenticated) path.
func TestIdentityScope(t *testing.T) {
	t.Run("nil identity falls back to agent workspace", func(t *testing.T) {
		origin, workspace := identityScope(nil, "acme")
		assert.Empty(t, origin)
		assert.Equal(t, "acme", workspace)
	})
	t.Run("token workspace wins over agent workspace", func(t *testing.T) {
		origin, workspace := identityScope(
			&policy.AuthenticatedIdentity{Origin: policy.OriginOIDC, Workspace: "token-ws"},
			"deployed-ws",
		)
		assert.Equal(t, policy.OriginOIDC, origin)
		assert.Equal(t, "token-ws", workspace)
	})
}
