package workspaceauth

import (
	"strings"
	"time"
)

// RoleBinding mirrors workspace.spec.roleBindings[]: a role granted to groups.
type RoleBinding struct {
	Groups []string `json:"groups"`
	Role   Role     `json:"role"`
}

// DirectGrant mirrors workspace.spec.directGrants[]: a role granted to a user,
// optionally expiring (RFC3339). Empty Expires = never expires.
type DirectGrant struct {
	User    string `json:"user"`
	Role    Role   `json:"role"`
	Expires string `json:"expires,omitempty"`
}

// AnonymousAccess mirrors workspace.spec.anonymousAccess.
type AnonymousAccess struct {
	Enabled bool `json:"enabled"`
	Role    Role `json:"role,omitempty"` // "" defaults to viewer when Enabled
}

// Inputs is the decoupled authorization input (caller maps the Workspace CR +
// the authenticated principal onto this).
type Inputs struct {
	RoleBindings    []RoleBinding
	DirectGrants    []DirectGrant
	AnonymousAccess *AnonymousAccess
	UserGroups      []string
	UserIdentity    string // email-or-username; "" for identity-less principals
	Anonymous       bool   // principal authenticated as anonymous
}

// ComputeRole returns the highest role granted to the principal, or "" for no
// access. now is injected for deterministic expiry evaluation.
func ComputeRole(in Inputs, now time.Time) Role {
	if in.Anonymous || in.UserIdentity == "" {
		if in.AnonymousAccess != nil && in.AnonymousAccess.Enabled {
			if in.AnonymousAccess.Role == "" {
				return RoleViewer
			}
			return in.AnonymousAccess.Role
		}
		return ""
	}
	role := roleFromBindings(in.RoleBindings, in.UserGroups)
	role = maxRole(role, directGrant(in.DirectGrants, in.UserIdentity, now))
	return role
}

func roleFromBindings(bindings []RoleBinding, userGroups []string) Role {
	groupSet := make(map[string]struct{}, len(userGroups))
	for _, g := range userGroups {
		groupSet[g] = struct{}{}
	}
	var highest Role
	for _, b := range bindings {
		for _, g := range b.Groups {
			if _, ok := groupSet[g]; ok {
				highest = maxRole(highest, b.Role)
				break
			}
		}
	}
	return highest
}

func directGrant(grants []DirectGrant, identity string, now time.Time) Role {
	if identity == "" {
		return ""
	}
	id := strings.ToLower(identity)
	for _, g := range grants {
		if strings.ToLower(g.User) != id {
			continue
		}
		if g.Expires != "" {
			exp, err := time.Parse(time.RFC3339, g.Expires)
			if err == nil && now.After(exp) {
				continue
			}
		}
		return g.Role
	}
	return ""
}
