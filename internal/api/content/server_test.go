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

package content

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/internal/api/authz"
	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/pkg/workspaceauth"
)

type fakeWS struct{ ns string }

func (f fakeWS) Resolve(_ context.Context, _ string) (authz.ResolvedWorkspace, error) {
	return authz.ResolvedWorkspace{
		Namespace: f.ns,
		Inputs: workspaceauth.Inputs{
			RoleBindings: []workspaceauth.RoleBinding{
				{Groups: []string{"editors"}, Role: workspaceauth.RoleEditor},
			},
		},
	}, nil
}

func mintIdentityToken(t *testing.T, key *rsa.PrivateKey, kid, workspace string, groups []string) string {
	t.Helper()
	now := time.Now()
	claims := authz.IdentityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    authz.IssuerDashboard,
			Audience:  jwt.ClaimStrings{authz.AudienceContentAPI},
			Subject:   "u@x.io",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
		Identity:  "u@x.io",
		Groups:    groups,
		Workspace: workspace,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func newWiredServer(t *testing.T) (*Server, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	resolver := &auth.StaticKeyResolver{Keys: map[string]*rsa.PublicKey{"k1": &key.PublicKey}}
	verifier := authz.NewIdentityVerifier(resolver)
	authorizer := authz.NewAuthorizer(verifier, fakeWS{ns: "ns"})
	srv := NewServer("127.0.0.1:0", NewHandler(t.TempDir(), logr.Discard()), authorizer, logr.Discard())
	return srv, key
}

func TestServer_RejectsRequestWithoutToken(t *testing.T) {
	srv, _ := newWiredServer(t)
	ts := httptest.NewServer(srv.server.Handler)
	defer ts.Close()

	// Every content verb must be behind the authz middleware.
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/workspaces/ws/content"},
		{http.MethodGet, "/api/v1/workspaces/ws/content/a/b.txt"},
		{http.MethodPut, "/api/v1/workspaces/ws/content/a/b.txt"},
		{http.MethodPost, "/api/v1/workspaces/ws/content/a"},
		{http.MethodPatch, "/api/v1/workspaces/ws/content/a/b.txt"},
		{http.MethodDelete, "/api/v1/workspaces/ws/content/a/b.txt"},
	} {
		r, err := http.NewRequest(tc.method, ts.URL+tc.path, nil)
		if err != nil {
			t.Fatalf("build req: %v", err)
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s: code = %d, want 401", tc.method, tc.path, resp.StatusCode)
		}
	}
}

func TestServer_AdmitsValidToken(t *testing.T) {
	srv, key := newWiredServer(t)
	ts := httptest.NewServer(srv.server.Handler)
	defer ts.Close()

	tok := mintIdentityToken(t, key, "k1", "ws", []string{"editors"})
	r, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/workspaces/ws/content", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("valid token GET root: code = %d, want 200", resp.StatusCode)
	}
}
