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
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

type authTestClaims struct {
	jwt.RegisteredClaims
	Origin    string `json:"origin"`
	Agent     string `json:"agent,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

// newAuthTestValidator returns a configured MgmtPlaneValidator and the RSA
// private key used to sign tokens for it.
func newAuthTestValidator(t *testing.T) (*auth.MgmtPlaneValidator, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "pub.pem")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	require.NoError(t, f.Close())

	v, err := auth.NewMgmtPlaneValidator(path)
	require.NoError(t, err)
	return v, key
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

func TestServerAuth_NoValidator_AllowsUpgrade(t *testing.T) {
	// Behaviour-preserving default: with no validator configured, upgrade
	// proceeds even without Authorization header.
	_, ts := newTestServer(t, nil)
	ws, _, err := dialWS(t, ts, nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()
	readConnected(t, ws)
}

func TestServerAuth_ValidatorPresent_NoAuthHeader_AllowsUpgrade(t *testing.T) {
	// PR 1 preserves the unauthenticated upgrade path even when a validator
	// is configured. PR 3 flips this default.
	v, _ := newAuthTestValidator(t)
	ts, observed := newAuthTestServer(t, v)

	ws, _, err := dialWS(t, ts, nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()
	readConnected(t, ws)

	// Send a message so the handler captures propagation fields.
	require.NoError(t, ws.WriteJSON(ClientMessage{Type: "user_message", Content: "hi"}))

	select {
	case fields := <-observed:
		assert.Nil(t, fields.Identity, "no credential presented → no Identity attached")
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not run")
	}
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

func TestServerAuth_NonBearerScheme_FallsThrough(t *testing.T) {
	// Non-Bearer Authorization header (Basic, Negotiate, etc.) is not a
	// mgmt-plane credential — the chain falls through and the upgrade
	// proceeds unauthenticated (PR 1 behaviour).
	v, _ := newAuthTestValidator(t)
	ts, _ := newAuthTestServer(t, v)

	header := http.Header{"Authorization": []string{"Basic dXNlcjpwYXNz"}}
	ws, _, err := dialWS(t, ts, header)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()
	readConnected(t, ws)
}
