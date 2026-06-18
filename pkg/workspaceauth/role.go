// Package workspaceauth computes a user's workspace role from RoleBindings /
// DirectGrants / anonymous-access config. It mirrors the dashboard's
// workspace-authz.ts (TS↔Go parity is enforced by parity_test.go) and is
// decoupled from the Workspace CRD so any service can reuse it.
package workspaceauth

// Role is a workspace role. Empty string ("") means "no access".
type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleOwner  Role = "owner"
)

// roleRank mirrors ROLE_HIERARCHY in dashboard/src/types/workspace.ts.
var roleRank = map[Role]int{RoleViewer: 1, RoleEditor: 2, RoleOwner: 3}

// MeetsRequiredRole reports whether granted is at least required. A zero/empty
// granted role never meets any requirement.
func MeetsRequiredRole(granted, required Role) bool {
	g, ok := roleRank[granted]
	if !ok {
		return false
	}
	return g >= roleRank[required]
}

// maxRole returns the higher-privilege of two roles ("" treated as lowest).
func maxRole(a, b Role) Role {
	if roleRank[a] >= roleRank[b] {
		if roleRank[a] == 0 {
			return b
		}
		return a
	}
	return b
}
