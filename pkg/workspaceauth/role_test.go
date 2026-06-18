package workspaceauth

import "testing"

func TestMeetsRequiredRole(t *testing.T) {
	cases := []struct {
		granted, required Role
		want              bool
	}{
		{RoleOwner, RoleEditor, true},
		{RoleEditor, RoleEditor, true},
		{RoleViewer, RoleEditor, false},
		{RoleEditor, RoleViewer, true},
		{"", RoleViewer, false},
	}
	for _, c := range cases {
		if got := MeetsRequiredRole(c.granted, c.required); got != c.want {
			t.Errorf("MeetsRequiredRole(%q,%q)=%v want %v", c.granted, c.required, got, c.want)
		}
	}
}
