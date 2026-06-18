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
	"errors"
	"net/http"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/workspaceauth"
)

// pathVarWorkspace is the ServeMux path-pattern variable carrying the target
// workspace, e.g. /api/v1/workspaces/{workspace}/content.
const pathVarWorkspace = "workspace"

// bearerPrefix is the case-sensitive scheme tag on Authorization: Bearer <token>.
const bearerPrefix = "Bearer "

// ErrWorkspaceNotFound is returned by a WorkspaceResolver when the named
// workspace does not exist. The middleware maps it to 404.
var ErrWorkspaceNotFound = errors.New("authz: workspace not found")

// WorkspaceResolver resolves a workspace name to the workspace-derived portion
// of the authorization inputs (role bindings, direct grants, anonymous-access
// config). The principal portion (identity, groups, anonymous) is filled in by
// the middleware from the verified token.
type WorkspaceResolver interface {
	Resolve(ctx context.Context, workspace string) (workspaceauth.Inputs, error)
}

// RequestIdentity is the verified principal plus the role recomputed for the
// target workspace, stashed in the request context for downstream handlers.
type RequestIdentity struct {
	*VerifiedIdentity
	Role workspaceauth.Role
}

type contextKey int

const identityContextKey contextKey = iota

// IdentityFromContext returns the RequestIdentity attached by the authz
// middleware, or false if the request did not pass through it.
func IdentityFromContext(ctx context.Context) (*RequestIdentity, bool) {
	id, ok := ctx.Value(identityContextKey).(*RequestIdentity)
	return id, ok
}

// Authorizer is HTTP middleware that verifies the identity token, recomputes
// the caller's workspace role server-side, and enforces a minimum role per
// HTTP verb (>= viewer for reads, >= editor for writes).
type Authorizer struct {
	verifier *IdentityVerifier
	resolver WorkspaceResolver
	now      func() time.Time
}

// AuthorizerOption tunes an Authorizer.
type AuthorizerOption func(*Authorizer)

// WithClock injects the clock used for direct-grant expiry evaluation.
func WithClock(now func() time.Time) AuthorizerOption {
	return func(a *Authorizer) { a.now = now }
}

// NewAuthorizer constructs an Authorizer.
func NewAuthorizer(verifier *IdentityVerifier, resolver WorkspaceResolver, opts ...AuthorizerOption) *Authorizer {
	a := &Authorizer{verifier: verifier, resolver: resolver, now: time.Now}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Middleware wraps next, admitting only requests that carry a valid identity
// token granting at least the verb-required role on the path workspace.
func (a *Authorizer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, status, msg := a.authorize(r)
		if status != http.StatusOK {
			http.Error(w, msg, status)
			return
		}
		ctx := context.WithValue(r.Context(), identityContextKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// authorize runs the full check and returns the request identity on success,
// or (nil, status, message) describing the rejection.
func (a *Authorizer) authorize(r *http.Request) (*RequestIdentity, int, string) {
	token, ok := bearerToken(r)
	if !ok {
		return nil, http.StatusUnauthorized, "missing bearer token"
	}
	verified, err := a.verifier.Verify(r.Context(), token)
	if err != nil {
		return nil, http.StatusUnauthorized, "invalid token"
	}

	workspace := r.PathValue(pathVarWorkspace)
	if workspace == "" {
		return nil, http.StatusBadRequest, "missing workspace"
	}
	// The token is scoped to a single workspace; reject if it does not match
	// the request path so a token for workspace A cannot reach workspace B.
	if verified.Workspace != workspace {
		return nil, http.StatusForbidden, "token workspace mismatch"
	}

	inputs, err := a.resolver.Resolve(r.Context(), workspace)
	if err != nil {
		if errors.Is(err, ErrWorkspaceNotFound) {
			return nil, http.StatusNotFound, "workspace not found"
		}
		return nil, http.StatusInternalServerError, "workspace lookup failed"
	}

	inputs.UserIdentity = verified.Identity
	inputs.UserGroups = verified.Groups
	inputs.Anonymous = verified.Anonymous

	role := workspaceauth.ComputeRole(inputs, a.now())
	required := requiredRoleForMethod(r.Method)
	if !workspaceauth.MeetsRequiredRole(role, required) {
		return nil, http.StatusForbidden, "insufficient role"
	}

	return &RequestIdentity{VerifiedIdentity: verified, Role: role}, http.StatusOK, ""
}

// requiredRoleForMethod maps an HTTP verb to the minimum role required: reads
// (GET/HEAD) require viewer, all mutating verbs require editor.
func requiredRoleForMethod(method string) workspaceauth.Role {
	switch method {
	case http.MethodGet, http.MethodHead:
		return workspaceauth.RoleViewer
	default:
		return workspaceauth.RoleEditor
	}
}

// bearerToken returns the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) (string, bool) {
	raw := r.Header.Get("Authorization")
	if !strings.HasPrefix(raw, bearerPrefix) {
		return "", false
	}
	token := strings.TrimSpace(raw[len(bearerPrefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// ClientWorkspaceResolver loads a Workspace CR (cluster-scoped) and maps its
// authorization spec to workspaceauth.Inputs.
type ClientWorkspaceResolver struct {
	client client.Client
}

// NewClientWorkspaceResolver constructs a resolver backed by a Kubernetes client.
func NewClientWorkspaceResolver(c client.Client) *ClientWorkspaceResolver {
	return &ClientWorkspaceResolver{client: c}
}

// Resolve loads the named Workspace and maps its RoleBindings / DirectGrants /
// AnonymousAccess onto Inputs. Returns ErrWorkspaceNotFound when absent.
func (r *ClientWorkspaceResolver) Resolve(ctx context.Context, name string) (workspaceauth.Inputs, error) {
	ws := &omniav1alpha1.Workspace{}
	if err := r.client.Get(ctx, types.NamespacedName{Name: name}, ws); err != nil {
		if apierrors.IsNotFound(err) {
			return workspaceauth.Inputs{}, ErrWorkspaceNotFound
		}
		return workspaceauth.Inputs{}, err
	}
	return workspaceSpecToInputs(&ws.Spec), nil
}

// workspaceSpecToInputs maps a Workspace's authorization spec to the
// workspace-derived portion of Inputs. ServiceAccount bindings are ignored —
// they gate token minting, not a user's role.
func workspaceSpecToInputs(spec *omniav1alpha1.WorkspaceSpec) workspaceauth.Inputs {
	in := workspaceauth.Inputs{}
	for _, b := range spec.RoleBindings {
		if len(b.Groups) == 0 {
			continue
		}
		in.RoleBindings = append(in.RoleBindings, workspaceauth.RoleBinding{
			Groups: b.Groups,
			Role:   workspaceauth.Role(b.Role),
		})
	}
	for _, g := range spec.DirectGrants {
		grant := workspaceauth.DirectGrant{User: g.User, Role: workspaceauth.Role(g.Role)}
		if g.Expires != nil {
			grant.Expires = g.Expires.Format(time.RFC3339)
		}
		in.DirectGrants = append(in.DirectGrants, grant)
	}
	if spec.AnonymousAccess != nil {
		in.AnonymousAccess = &workspaceauth.AnonymousAccess{
			Enabled: spec.AnonymousAccess.Enabled,
			Role:    workspaceauth.Role(spec.AnonymousAccess.Role),
		}
	}
	return in
}
