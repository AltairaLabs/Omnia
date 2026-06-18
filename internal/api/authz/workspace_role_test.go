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

package authz

import (
	"context"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/workspaceauth"
)

type fakeResolver struct {
	policies map[string]workspaceauth.Inputs
}

func (f *fakeResolver) Resolve(_ context.Context, ws string) (workspaceauth.Inputs, error) {
	in, ok := f.policies[ws]
	if !ok {
		return workspaceauth.Inputs{}, ErrWorkspaceNotFound
	}
	return in, nil
}

func newTestAuthorizer(t *testing.T) (*Authorizer, *rsa.PrivateKey) {
	t.Helper()
	v, key := testVerifier(t)
	fr := &fakeResolver{policies: map[string]workspaceauth.Inputs{
		"team-a": {
			RoleBindings: []workspaceauth.RoleBinding{
				{Groups: []string{"editors"}, Role: workspaceauth.RoleEditor},
				{Groups: []string{"viewers"}, Role: workspaceauth.RoleViewer},
			},
		},
		"anon-ws": {
			AnonymousAccess: &workspaceauth.AnonymousAccess{Enabled: true},
		},
	}}
	return NewAuthorizer(v, fr), key
}

func tokenFor(t *testing.T, key *rsa.PrivateKey, ws, identity string, groups []string, anon bool) string {
	t.Helper()
	c := validClaims(time.Now())
	c.Workspace = ws
	c.Groups = groups
	c.Anonymous = anon
	if anon {
		c.Identity = ""
		c.Subject = "anonymous"
	} else {
		c.Identity = identity
		c.Subject = identity
	}
	return mintToken(t, key, testKid, c)
}

func serve(t *testing.T, a *Authorizer, method, urlWS, token string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle("/api/v1/workspaces/{workspace}/content", a.Middleware(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			id, ok := IdentityFromContext(r.Context())
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("X-Role", string(id.Role))
			w.WriteHeader(http.StatusOK)
		},
	)))
	req := httptest.NewRequest(method, "/api/v1/workspaces/"+urlWS+"/content", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestAuthorizer_ViewerCanReadNotWrite(t *testing.T) {
	a, key := newTestAuthorizer(t)
	tok := tokenFor(t, key, "team-a", "v@x.io", []string{"viewers"}, false)

	if rec := serve(t, a, http.MethodGet, "team-a", tok); rec.Code != http.StatusOK {
		t.Errorf("viewer GET: code = %d, want 200", rec.Code)
	} else if rec.Header().Get("X-Role") != "viewer" {
		t.Errorf("viewer GET: role = %q, want viewer", rec.Header().Get("X-Role"))
	}
	if rec := serve(t, a, http.MethodPost, "team-a", tok); rec.Code != http.StatusForbidden {
		t.Errorf("viewer POST: code = %d, want 403", rec.Code)
	}
}

func TestAuthorizer_EditorCanWrite(t *testing.T) {
	a, key := newTestAuthorizer(t)
	tok := tokenFor(t, key, "team-a", "e@x.io", []string{"editors"}, false)

	if rec := serve(t, a, http.MethodPost, "team-a", tok); rec.Code != http.StatusOK {
		t.Errorf("editor POST: code = %d, want 200", rec.Code)
	} else if rec.Header().Get("X-Role") != "editor" {
		t.Errorf("editor POST: role = %q, want editor", rec.Header().Get("X-Role"))
	}
}

func TestAuthorizer_NonMemberForbidden(t *testing.T) {
	a, key := newTestAuthorizer(t)
	tok := tokenFor(t, key, "team-a", "x@x.io", []string{"randos"}, false)

	if rec := serve(t, a, http.MethodGet, "team-a", tok); rec.Code != http.StatusForbidden {
		t.Errorf("non-member GET: code = %d, want 403", rec.Code)
	}
}

func TestAuthorizer_CrossWorkspaceTokenForbidden(t *testing.T) {
	a, key := newTestAuthorizer(t)
	// Token scoped to "other" but request targets "team-a".
	tok := tokenFor(t, key, "other", "e@x.io", []string{"editors"}, false)

	if rec := serve(t, a, http.MethodGet, "team-a", tok); rec.Code != http.StatusForbidden {
		t.Errorf("cross-workspace GET: code = %d, want 403", rec.Code)
	}
}

func TestAuthorizer_NoToken(t *testing.T) {
	a, _ := newTestAuthorizer(t)
	if rec := serve(t, a, http.MethodGet, "team-a", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: code = %d, want 401", rec.Code)
	}
}

func TestAuthorizer_ExpiredToken(t *testing.T) {
	a, key := newTestAuthorizer(t)
	c := validClaims(time.Now().Add(-time.Hour))
	c.Workspace = "team-a"
	c.Groups = []string{"editors"}
	tok := mintToken(t, key, testKid, c)

	if rec := serve(t, a, http.MethodGet, "team-a", tok); rec.Code != http.StatusUnauthorized {
		t.Errorf("expired token: code = %d, want 401", rec.Code)
	}
}

func TestAuthorizer_AnonymousAccess(t *testing.T) {
	a, key := newTestAuthorizer(t)
	tok := tokenFor(t, key, "anon-ws", "", nil, true)

	if rec := serve(t, a, http.MethodGet, "anon-ws", tok); rec.Code != http.StatusOK {
		t.Errorf("anonymous GET: code = %d, want 200", rec.Code)
	}
	if rec := serve(t, a, http.MethodPost, "anon-ws", tok); rec.Code != http.StatusForbidden {
		t.Errorf("anonymous POST: code = %d, want 403", rec.Code)
	}
}

func TestAuthorizer_UnknownWorkspace(t *testing.T) {
	a, key := newTestAuthorizer(t)
	tok := tokenFor(t, key, "ghost", "e@x.io", []string{"editors"}, false)

	if rec := serve(t, a, http.MethodGet, "ghost", tok); rec.Code != http.StatusNotFound {
		t.Errorf("unknown workspace: code = %d, want 404", rec.Code)
	}
}

// --- ClientWorkspaceResolver (Workspace CR -> Inputs mapping) ---

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := omniav1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func TestClientWorkspaceResolver_MapsSpec(t *testing.T) {
	expires := &metav1.Time{Time: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)}
	ws := &omniav1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "team-a"},
		Spec: omniav1alpha1.WorkspaceSpec{
			RoleBindings: []omniav1alpha1.RoleBinding{
				{Groups: []string{"editors"}, Role: omniav1alpha1.WorkspaceRoleEditor},
			},
			DirectGrants: []omniav1alpha1.DirectGrant{
				{User: "u@x.io", Role: omniav1alpha1.WorkspaceRoleOwner, Expires: expires},
			},
			AnonymousAccess: &omniav1alpha1.AnonymousAccess{Enabled: true, Role: omniav1alpha1.WorkspaceRoleViewer},
		},
	}
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).WithObjects(ws).Build()
	r := NewClientWorkspaceResolver(c)

	in, err := r.Resolve(context.Background(), "team-a")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(in.RoleBindings) != 1 || in.RoleBindings[0].Role != workspaceauth.RoleEditor ||
		in.RoleBindings[0].Groups[0] != "editors" {
		t.Errorf("RoleBindings mapped wrong: %+v", in.RoleBindings)
	}
	if len(in.DirectGrants) != 1 || in.DirectGrants[0].User != "u@x.io" ||
		in.DirectGrants[0].Role != workspaceauth.RoleOwner ||
		in.DirectGrants[0].Expires != "2027-01-01T00:00:00Z" {
		t.Errorf("DirectGrants mapped wrong: %+v", in.DirectGrants)
	}
	if in.AnonymousAccess == nil || !in.AnonymousAccess.Enabled ||
		in.AnonymousAccess.Role != workspaceauth.RoleViewer {
		t.Errorf("AnonymousAccess mapped wrong: %+v", in.AnonymousAccess)
	}
}

func TestClientWorkspaceResolver_NotFound(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(newScheme(t)).Build()
	r := NewClientWorkspaceResolver(c)

	if _, err := r.Resolve(context.Background(), "ghost"); err != ErrWorkspaceNotFound {
		t.Errorf("missing workspace: err = %v, want ErrWorkspaceNotFound", err)
	}
}
