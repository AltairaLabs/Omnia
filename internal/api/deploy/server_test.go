package deploy

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/altairalabs/omnia/internal/api/authz"
	"github.com/altairalabs/omnia/pkg/facade/auth"
	"github.com/altairalabs/omnia/pkg/workspaceauth"
)

type fakeWS struct{ ns string }

func (f fakeWS) Resolve(_ context.Context, _ string) (authz.ResolvedWorkspace, error) {
	return authz.ResolvedWorkspace{
		Namespace: f.ns,
		Inputs: workspaceauth.Inputs{
			RoleBindings: []workspaceauth.RoleBinding{{Groups: []string{"editors"}, Role: workspaceauth.RoleEditor}},
		},
	}, nil
}

func mintToken(t *testing.T, key *rsa.PrivateKey, kid, workspace string, groups []string) string {
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
		Identity: "u@x.io", Groups: groups, Workspace: workspace,
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
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	srv := NewServer("127.0.0.1:0", NewHandler(NewApplier(c, logr.Discard()), logr.Discard()), authorizer, logr.Discard())
	return srv, key
}

func TestServer_RejectsWithoutToken(t *testing.T) {
	srv, _ := newWiredServer(t)
	ts := httptest.NewServer(srv.server.Handler)
	defer ts.Close()
	r, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/workspaces/ws/deployments", bytes.NewReader([]byte("{}")))
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", resp.StatusCode)
	}
}

func TestServer_AppliesWithValidToken(t *testing.T) {
	srv, key := newWiredServer(t)
	ts := httptest.NewServer(srv.server.Handler)
	defer ts.Close()

	body, _ := json.Marshal(testIntent())
	r, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/workspaces/ws/deployments", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+mintToken(t, key, "k1", "ws", []string{"editors"}))
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("code = %d, want 200", resp.StatusCode)
	}
	var out DeployResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !out.Succeeded || len(out.Results) == 0 {
		t.Errorf("result = %+v", out)
	}
}

func TestServer_RejectsUnknownAPIVersion(t *testing.T) {
	srv, key := newWiredServer(t)
	ts := httptest.NewServer(srv.server.Handler)
	defer ts.Close()
	bad := testIntent()
	bad.APIVersion = "deploy.omnia.altairalabs.ai/v2"
	body, _ := json.Marshal(bad)
	r, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/workspaces/ws/deployments", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+mintToken(t, key, "k1", "ws", []string{"editors"}))
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", resp.StatusCode)
	}
}

func TestServer_Healthz(t *testing.T) {
	srv, _ := newWiredServer(t)
	ts := httptest.NewServer(srv.server.Handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("code = %d, want 200", resp.StatusCode)
	}
}

// TestServer_StartStopsOnContextCancel exercises Start's ctx.Done() path: an
// already-cancelled context makes Start return nil without waiting on a real
// listen failure. Shutdown then stops the listener Start spun up in the
// background so the test doesn't leak a bound port.
func TestServer_StartStopsOnContextCancel(t *testing.T) {
	srv, _ := newWiredServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start() = %v, want nil", err)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v", err)
	}
}

// TestServer_StartReturnsListenError exercises Start's errCh path: binding to
// an address already held by another listener fails immediately.
func TestServer_StartReturnsListenError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = ln.Close() }()

	srv, _ := newWiredServer(t)
	srv.addr = ln.Addr().String()
	srv.server.Addr = ln.Addr().String()

	if err := srv.Start(context.Background()); err == nil {
		t.Fatal("Start() = nil, want listen error")
	}
}
