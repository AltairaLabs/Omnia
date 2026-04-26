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

package facade

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/policy"
)

// kidForKey returns the RFC 7638 thumbprint of an RSA public key.
// Matches the kid the dashboard's lib/jwks.js produces, so JWTs minted
// here verify against an auth.StaticKeyResolver keyed by it.
func kidForKey(t *testing.T, pub *rsa.PublicKey) string {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	canonical := fmt.Sprintf(`{"e":%q,"kty":"RSA","n":%q}`, e, n)
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type authTestClaims struct {
	jwt.RegisteredClaims
	Origin    string `json:"origin"`
	Agent     string `json:"agent,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

// newAuthTestValidator returns a MgmtPlaneValidator backed by a static
// resolver and the RSA private key used to sign tokens for it. Tests
// must include the kid header (kidForKey) so the validator can find
// the matching public key.
func newAuthTestValidator(t *testing.T) (*auth.MgmtPlaneValidator, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kid := kidForKey(t, &key.PublicKey)
	resolver := &auth.StaticKeyResolver{
		Keys: map[string]*rsa.PublicKey{kid: &key.PublicKey},
	}
	return auth.NewMgmtPlaneValidatorWithResolver(resolver), key
}

func mintMgmtToken(t *testing.T, key *rsa.PrivateKey, override func(*authTestClaims)) string {
	t.Helper()
	now := time.Now()
	claims := authTestClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.DefaultMgmtPlaneIssuer,
			Audience:  jwt.ClaimStrings{auth.DefaultMgmtPlaneAudience},
			Subject:   "admin@example.com",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
		Origin:    policy.OriginManagementPlane,
		Agent:     "test-agent",
		Workspace: "default",
	}
	if override != nil {
		override(&claims)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kidForKey(t, &key.PublicKey)
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

// newAuthTestServer builds a Server with the given validator and a capturing
// handler that records the propagation fields observed for each WS message.
// The returned channel delivers one entry per HandleMessage call.
func newAuthTestServer(t *testing.T, v auth.Validator) (*httptest.Server, <-chan policy.PropagationFields) {
	t.Helper()

	observed := make(chan policy.PropagationFields, 1)
	handler := &mockHandler{
		handleFunc: func(ctx context.Context, _ string, msg *ClientMessage, w ResponseWriter) error {
			select {
			case observed <- policy.ExtractPropagationFields(ctx):
			default:
			}
			return w.WriteDone("echo: " + msg.Content)
		},
	}

	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond
	server := NewServer(cfg, store, handler, logr.Discard(), WithMgmtPlaneValidator(v))

	ts := httptest.NewServer(server)
	t.Cleanup(func() {
		ts.Close()
		_ = store.Close()
	})
	return ts, observed
}

func dialWS(t *testing.T, ts *httptest.Server, header http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	return websocket.DefaultDialer.Dial(wsURL(ts.URL)+"?agent=test-agent", header)
}

func TestServerAuth_NoValidator_DevModeAllowsUpgrade(t *testing.T) {
	// Empty chain is the dev/test escape hatch — allowUnauthenticated
	// defaults to true at the Server layer so a bare NewServer call
	// (no WithAuthChain) keeps working for standalone binaries that
	// have no k8s client or mgmt-plane key.
	_, ts := newTestServer(t, nil)
	ws, _, err := dialWS(t, ts, nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()
	readConnected(t, ws)
}

func TestServerAuth_ValidatorPresent_NoAuthHeader_Rejects401(t *testing.T) {
	// PR 3: with a chain configured, a request carrying no credential
	// must 401 before Upgrade. This is the default-flip that closes
	// pen-test C-3 — a customer app reaching the facade without
	// authentication must not get a WebSocket session.
	v, _ := newAuthTestValidator(t)
	ts, _ := newAuthTestServer(t, v)

	_, resp, err := dialWS(t, ts, nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"PR 3: missing credential with chain configured must 401")
}

func TestServerAuth_ValidToken_AttachesIdentity(t *testing.T) {
	v, key := newAuthTestValidator(t)
	ts, observed := newAuthTestServer(t, v)

	token := mintMgmtToken(t, key, nil)
	header := http.Header{"Authorization": []string{"Bearer " + token}}

	ws, _, err := dialWS(t, ts, header)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()
	readConnected(t, ws)
	require.NoError(t, ws.WriteJSON(ClientMessage{Type: "user_message", Content: "hi"}))

	select {
	case fields := <-observed:
		require.NotNil(t, fields.Identity, "expected Identity on PropagationFields")
		assert.Equal(t, policy.OriginManagementPlane, fields.Identity.Origin)
		assert.Equal(t, "admin@example.com", fields.Identity.Subject)
		assert.Equal(t, policy.RoleAdmin, fields.Identity.Role)
		assert.NotEmpty(t, fields.UserID, "UserID should be pseudonymised EndUser")
		assert.Equal(t, policy.RoleAdmin, fields.UserRoles)
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not run")
	}
}

func TestServerAuth_InvalidToken_Rejects(t *testing.T) {
	v, key := newAuthTestValidator(t)
	ts, _ := newAuthTestServer(t, v)

	// Wrong origin — the validator admits only management-plane tokens.
	badToken := mintMgmtToken(t, key, func(c *authTestClaims) { c.Origin = "data-plane" })
	header := http.Header{"Authorization": []string{"Bearer " + badToken}}

	_, resp, err := dialWS(t, ts, header)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServerAuth_ExpiredToken_Rejects(t *testing.T) {
	v, key := newAuthTestValidator(t)
	ts, _ := newAuthTestServer(t, v)

	badToken := mintMgmtToken(t, key, func(c *authTestClaims) {
		c.IssuedAt = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))
		c.NotBefore = jwt.NewNumericDate(time.Now().Add(-1 * time.Hour))
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-5 * time.Minute))
	})
	header := http.Header{"Authorization": []string{"Bearer " + badToken}}

	_, resp, err := dialWS(t, ts, header)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServerAuth_MalformedToken_Rejects(t *testing.T) {
	v, _ := newAuthTestValidator(t)
	ts, _ := newAuthTestServer(t, v)

	header := http.Header{"Authorization": []string{"Bearer not.a.jwt"}}
	_, resp, err := dialWS(t, ts, header)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestServerAuth_NonBearerScheme_Rejects401(t *testing.T) {
	// PR 3: a non-Bearer Authorization header (Basic, Negotiate, etc.)
	// is no credential any configured validator recognises. Under PR 3
	// the chain-wide ErrNoCredential result 401s instead of falling
	// through to the unauthenticated upgrade path. This closes the
	// "attacker sends Basic auth and gets a WS session" bypass.
	v, _ := newAuthTestValidator(t)
	ts, _ := newAuthTestServer(t, v)

	header := http.Header{"Authorization": []string{"Basic dXNlcjpwYXNz"}}
	_, resp, err := dialWS(t, ts, header)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"non-Bearer Authorization with chain configured must 401")
}

// TestServerAuth_SharedTokenChain_AdmitsBeforeMgmtPlane proves that PR
// 2b's chain wiring works end-to-end through the facade — a sharedToken
// validator placed before the mgmt-plane validator admits a presented-
// bearer request and tags the identity with origin=shared-token.
func TestServerAuth_SharedTokenChain_AdmitsBeforeMgmtPlane(t *testing.T) {
	mgmt, _ := newAuthTestValidator(t)
	const sharedToken = "shared-bearer-value"
	stv, err := auth.NewSharedTokenValidator(sharedToken)
	require.NoError(t, err)
	chain := auth.Chain{stv, mgmt}

	observed := make(chan policy.PropagationFields, 1)
	handler := &mockHandler{
		handleFunc: func(ctx context.Context, _ string, msg *ClientMessage, w ResponseWriter) error {
			select {
			case observed <- policy.ExtractPropagationFields(ctx):
			default:
			}
			return w.WriteDone("echo: " + msg.Content)
		},
	}
	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond
	server := NewServer(cfg, store, handler, logr.Discard(), WithAuthChain(chain))
	ts := httptest.NewServer(server)
	t.Cleanup(func() { ts.Close(); _ = store.Close() })

	header := http.Header{"Authorization": []string{"Bearer " + sharedToken}}
	ws, _, err := dialWS(t, ts, header)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()
	readConnected(t, ws)
	require.NoError(t, ws.WriteJSON(ClientMessage{Type: "user_message", Content: "hi"}))

	select {
	case fields := <-observed:
		require.NotNil(t, fields.Identity)
		assert.Equal(t, policy.OriginSharedToken, fields.Identity.Origin,
			"sharedToken must win over mgmt-plane when both could admit")
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not run")
	}
}

// TestServerAuth_SharedTokenChain_RejectsWrongToken proves that a
// presented bearer that fails the sharedToken compare short-circuits
// the chain — we must NOT fall through to mgmt-plane and accidentally
// admit a data-plane token via a different validator.
func TestServerAuth_SharedTokenChain_RejectsWrongToken(t *testing.T) {
	mgmt, _ := newAuthTestValidator(t)
	stv, err := auth.NewSharedTokenValidator("expected-token")
	require.NoError(t, err)
	chain := auth.Chain{stv, mgmt}

	cfg := DefaultServerConfig()
	store := session.NewMemoryStore()
	server := NewServer(cfg, store, &mockHandler{}, logr.Discard(), WithAuthChain(chain))
	ts := httptest.NewServer(server)
	t.Cleanup(func() { ts.Close(); _ = store.Close() })

	header := http.Header{"Authorization": []string{"Bearer wrong-token"}}
	_, resp, err := dialWS(t, ts, header)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"sharedToken's ErrInvalidCredential must reject; mgmt-plane must NOT be reached")
}

// stubClaimValidator is an auth.Validator that admits every request
// and returns a canned AuthenticatedIdentity with a populated Claims
// map. Good enough to prove the facade copies the map into
// PropagationFields regardless of which validator did the admit.
type stubClaimValidator struct {
	id *policy.AuthenticatedIdentity
}

func (s *stubClaimValidator) Validate(_ context.Context, _ *http.Request) (*policy.AuthenticatedIdentity, error) {
	return s.id, nil
}

// TestServerAuth_IdentityClaims_PropagatedToHeaders proves B3 is fixed:
// when a validator admits with non-empty Identity.Claims, those claims
// must reach PropagationFields.Claims so downstream ToOutboundHeaders
// emits X-Omnia-Claim-<name> headers regardless of which validator
// admitted.
func TestServerAuth_IdentityClaims_PropagatedToHeaders(t *testing.T) {
	want := &policy.AuthenticatedIdentity{
		Origin:  policy.OriginOIDC,
		Subject: "alice@example.com",
		EndUser: "alice@example.com",
		Role:    policy.RoleEditor,
		Claims: map[string]string{
			"team":   "finance",
			"region": "us-east",
			"email":  "alice@example.com",
		},
	}
	chain := auth.Chain{&stubClaimValidator{id: want}}

	observed := make(chan policy.PropagationFields, 1)
	handler := &mockHandler{
		handleFunc: func(ctx context.Context, _ string, msg *ClientMessage, w ResponseWriter) error {
			select {
			case observed <- policy.ExtractPropagationFields(ctx):
			default:
			}
			return w.WriteDone("echo: " + msg.Content)
		},
	}
	store := session.NewMemoryStore()
	cfg := DefaultServerConfig()
	cfg.PingInterval = 100 * time.Millisecond
	cfg.PongTimeout = 200 * time.Millisecond
	server := NewServer(cfg, store, handler, logr.Discard(), WithAuthChain(chain))
	ts := httptest.NewServer(server)
	t.Cleanup(func() { ts.Close(); _ = store.Close() })

	header := http.Header{"Authorization": []string{"Bearer anything"}}
	ws, _, err := dialWS(t, ts, header)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()
	readConnected(t, ws)
	require.NoError(t, ws.WriteJSON(ClientMessage{Type: "user_message", Content: "hi"}))

	select {
	case fields := <-observed:
		require.NotNil(t, fields.Identity, "Identity must still be attached")
		assert.Equal(t, "finance", fields.Claims["team"],
			"Identity.Claims must be copied into PropagationFields.Claims (B3)")
		assert.Equal(t, "us-east", fields.Claims["region"])
		assert.Equal(t, "alice@example.com", fields.Claims["email"])
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not run")
	}
}
