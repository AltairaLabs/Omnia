/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import "testing"

func TestParseServiceAccount(t *testing.T) {
	tests := []struct {
		subject  string
		wantNS   string
		wantName string
		wantOK   bool
	}{
		{"system:serviceaccount:ws-ns:foo-facade", "ws-ns", "foo-facade", true},
		{"system:serviceaccount:omnia-system:omnia-session-api", "omnia-system", "omnia-session-api", true},
		// Missing name segment — must reject (this is the substring-attack case:
		// a bare namespace must NOT parse as a valid SA).
		{"system:serviceaccount:ws-ns", "", "", false},
		// Trailing colon, empty name.
		{"system:serviceaccount:ws-ns:", "", "", false},
		// Empty namespace.
		{"system:serviceaccount::foo", "", "", false},
		// Extra colon in the name segment — malformed.
		{"system:serviceaccount:ws-ns:foo:bar", "", "", false},
		// Not a ServiceAccount subject (a user).
		{"system:node:worker-1", "", "", false},
		{"alice@example.com", "", "", false},
		{"", "", "", false},
		// Almost-prefix.
		{"system:serviceaccount", "", "", false},
	}
	for _, tt := range tests {
		ns, name, ok := ParseServiceAccount(tt.subject)
		if ns != tt.wantNS || name != tt.wantName || ok != tt.wantOK {
			t.Errorf("ParseServiceAccount(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.subject, ns, name, ok, tt.wantNS, tt.wantName, tt.wantOK)
		}
	}
}

func TestAuthorizerAllowed(t *testing.T) {
	const exactSub = "system:serviceaccount:omnia-system:omnia-dashboard"
	a := newAuthorizer([]string{exactSub}, []string{"ws-ns"})

	cases := map[string]struct {
		subject string
		want    bool
	}{
		"exact subject match":                 {exactSub, true},
		"namespace match, not in subjects":    {"system:serviceaccount:ws-ns:foo-facade", true},
		"namespace not allowed":               {"system:serviceaccount:other-ns:foo-facade", false},
		"malformed subject with ns substring": {"system:serviceaccount:ws-ns", false},
		"non-SA subject":                      {"system:node:worker", false},
	}
	for name, tc := range cases {
		if got := a.allowed(tc.subject); got != tc.want {
			t.Errorf("%s: allowed(%q) = %v, want %v", name, tc.subject, got, tc.want)
		}
	}

	// No namespaces configured -> only exact-subject matches, never namespace.
	subjOnly := newAuthorizer([]string{exactSub}, nil)
	if subjOnly.allowed("system:serviceaccount:ws-ns:foo-facade") {
		t.Error("with no allowedNamespaces, a namespace-only caller must be rejected")
	}
	if !subjOnly.allowed(exactSub) {
		t.Error("exact subject must still be allowed with no namespaces")
	}
}
