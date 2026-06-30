/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import "testing"

func TestScopeShape(t *testing.T) {
	cases := []struct {
		name  string
		scope Scope
		want  ScopeShape
	}{
		{"institutional", Scope{WorkspaceID: "w"}, ScopeShapeInstitutional},
		{"agent-scoped", Scope{WorkspaceID: "w", AgentID: "a"}, ScopeShapeAgentScoped},
		{"user-scoped", Scope{WorkspaceID: "w", UserID: "u"}, ScopeShapeUserScoped},
		{"user-for-agent", Scope{WorkspaceID: "w", AgentID: "a", UserID: "u"}, ScopeShapeUserForAgent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scope.Shape(); got != tc.want {
				t.Errorf("Shape() = %v, want %v", got, tc.want)
			}
		})
	}
}
