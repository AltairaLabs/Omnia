package workspaceauth

import (
	"testing"
	"time"
)

const testUser = "u@x.io"

func TestComputeRole(t *testing.T) {
	now := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour).Format(time.RFC3339)
	past := now.Add(-time.Hour).Format(time.RFC3339)

	in := Inputs{
		RoleBindings: []RoleBinding{
			{Groups: []string{"eng"}, Role: RoleViewer},
			{Groups: []string{"admins"}, Role: RoleOwner},
		},
		UserGroups:   []string{"eng", "admins"},
		UserIdentity: testUser,
	}
	if got := ComputeRole(in, now); got != RoleOwner {
		t.Fatalf("highest binding role: got %q want owner", got)
	}

	noMatch := Inputs{RoleBindings: in.RoleBindings, UserGroups: []string{"other"}, UserIdentity: testUser}
	if got := ComputeRole(noMatch, now); got != "" {
		t.Fatalf("no group match: got %q want empty", got)
	}

	grant := Inputs{
		DirectGrants: []DirectGrant{{User: "U@X.io", Role: RoleEditor, Expires: future}},
		UserGroups:   []string{}, UserIdentity: testUser,
	}
	if got := ComputeRole(grant, now); got != RoleEditor {
		t.Fatalf("direct grant (case-insensitive): got %q want editor", got)
	}

	expired := Inputs{
		DirectGrants: []DirectGrant{{User: testUser, Role: RoleOwner, Expires: past}},
		UserIdentity: testUser,
	}
	if got := ComputeRole(expired, now); got != "" {
		t.Fatalf("expired grant ignored: got %q want empty", got)
	}

	anon := Inputs{Anonymous: true, AnonymousAccess: &AnonymousAccess{Enabled: true}}
	if got := ComputeRole(anon, now); got != RoleViewer {
		t.Fatalf("anonymous default: got %q want viewer", got)
	}
	anonOff := Inputs{Anonymous: true, AnonymousAccess: &AnonymousAccess{Enabled: false}}
	if got := ComputeRole(anonOff, now); got != "" {
		t.Fatalf("anonymous disabled: got %q want empty", got)
	}
}
