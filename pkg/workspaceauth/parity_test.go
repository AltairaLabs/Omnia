package workspaceauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// parityCase mirrors one entry in testdata/parity_cases.json. The SAME file is
// consumed by the dashboard's parity.test.ts so the Go and TS implementations
// can't drift. An empty ExpectedRole means "no access".
type parityCase struct {
	Name            string           `json:"name"`
	RoleBindings    []RoleBinding    `json:"roleBindings"`
	DirectGrants    []DirectGrant    `json:"directGrants"`
	AnonymousAccess *AnonymousAccess `json:"anonymousAccess"`
	UserGroups      []string         `json:"userGroups"`
	UserIdentity    string           `json:"userIdentity"`
	Anonymous       bool             `json:"anonymous"`
	ExpectedRole    Role             `json:"expectedRole"`
}

func TestComputeRoleParity(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "parity_cases.json"))
	if err != nil {
		t.Fatalf("read parity fixture: %v", err)
	}

	var cases []parityCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("unmarshal parity fixture: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("parity fixture is empty")
	}

	now := time.Now()
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := ComputeRole(Inputs{
				RoleBindings:    tc.RoleBindings,
				DirectGrants:    tc.DirectGrants,
				AnonymousAccess: tc.AnonymousAccess,
				UserGroups:      tc.UserGroups,
				UserIdentity:    tc.UserIdentity,
				Anonymous:       tc.Anonymous,
			}, now)
			if got != tc.ExpectedRole {
				t.Errorf("ComputeRole = %q, want %q", got, tc.ExpectedRole)
			}
		})
	}
}
