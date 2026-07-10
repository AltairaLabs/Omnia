/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package facade_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/examples/custom-facade/facade"
)

// staticResolver is an in-memory facade.KeyResolver for tests.
type staticResolver struct {
	keys map[string]*rsa.PublicKey
}

func (s staticResolver) Resolve(_ context.Context, kid string) (*rsa.PublicKey, error) {
	if k, ok := s.keys[kid]; ok {
		return k, nil
	}
	return nil, jwt.ErrTokenUnverifiable
}

// signRS256 mints an RS256 JWT with the given kid, key and expiry.
func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, exp time.Time) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.RegisteredClaims{
		Subject:   "dashboard-user",
		ExpiresAt: jwt.NewNumericDate(exp),
	})
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func newVerifier(t *testing.T) (*facade.MgmtVerifier, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	res := staticResolver{keys: map[string]*rsa.PublicKey{"k1": &key.PublicKey}}
	return facade.NewMgmtVerifier(res), key
}

// TestMgmtVerify_ValidToken is the happy path: a correctly signed, unexpired
// RS256 token verifies.
func TestMgmtVerify_ValidToken(t *testing.T) {
	v, key := newVerifier(t)
	token := signRS256(t, key, "k1", time.Now().Add(time.Hour))
	if _, err := v.Verify(context.Background(), "Bearer "+token); err != nil {
		t.Fatalf("Verify(valid) error = %v, want nil", err)
	}
}

// TestMgmtVerify_FailsClosed covers every rejection path the mgmt twin must
// close on: absent token, malformed token, expired token, and a token signed
// by a key the resolver does not know.
func TestMgmtVerify_FailsClosed(t *testing.T) {
	v, key := newVerifier(t)

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}

	cases := map[string]string{
		"absent":       "",
		"malformed":    "Bearer not.a.jwt",
		"expired":      "Bearer " + signRS256(t, key, "k1", time.Now().Add(-time.Hour)),
		"unknown-kid":  "Bearer " + signRS256(t, key, "k-unknown", time.Now().Add(time.Hour)),
		"wrong-signer": "Bearer " + signRS256(t, otherKey, "k1", time.Now().Add(time.Hour)),
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := v.Verify(context.Background(), header); err == nil {
				t.Errorf("Verify(%s) error = nil, want rejection (fail closed)", name)
			}
		})
	}
}

// TestMgmtVerify_RejectsNoneAlg guards against alg-confusion: a token using the
// "none" algorithm must be rejected even though it has a valid structure.
func TestMgmtVerify_RejectsNoneAlg(t *testing.T) {
	v, _ := newVerifier(t)
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{Subject: "x"})
	tok.Header["kid"] = "k1"
	unsigned, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := v.Verify(context.Background(), "Bearer "+unsigned); err == nil {
		t.Error("Verify(none-alg) error = nil, want rejection")
	}
}

// TestMgmtMiddleware_FailsClosed asserts the HTTP middleware returns 401 for an
// unauthenticated request and passes a valid one through.
func TestMgmtMiddleware_FailsClosed(t *testing.T) {
	v, key := newVerifier(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := v.Middleware(next)

	t.Run("no token → 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/chat", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("valid token → 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/chat", nil)
		req.Header.Set("Authorization", "Bearer "+signRS256(t, key, "k1", time.Now().Add(time.Hour)))
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", rec.Code)
		}
	})
}

// TestJWKSResolver_FetchesAndVerifies exercises the real JWKS resolver against
// an httptest JWKS endpoint, then verifies a token signed by the served key —
// the production wiring path (OMNIA_MGMT_PLANE_JWKS_URL).
func TestJWKSResolver_FetchesAndVerifies(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pub := key.PublicKey
		doc := map[string]any{"keys": []map[string]string{{
			"kty": "RSA",
			"kid": "k1",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(bigEndianExp(pub.E)),
		}}}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer srv.Close()

	v := facade.NewMgmtVerifier(facade.NewJWKSResolver(srv.URL))
	token := signRS256(t, key, "k1", time.Now().Add(time.Hour))
	if _, err := v.Verify(context.Background(), "Bearer "+token); err != nil {
		t.Fatalf("Verify via JWKS error = %v, want nil", err)
	}
}

// TestJWKSResolver_UnknownKidFailsClosed asserts the resolver errors (not falls
// open) when the served keyset lacks the token's kid.
func TestJWKSResolver_UnknownKidFailsClosed(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer srv.Close()

	v := facade.NewMgmtVerifier(facade.NewJWKSResolver(srv.URL))
	token := signRS256(t, key, "k1", time.Now().Add(time.Hour))
	if _, err := v.Verify(context.Background(), "Bearer "+token); err == nil {
		t.Error("Verify with empty keyset error = nil, want rejection")
	}
}

// bigEndianExp packs an RSA exponent into its minimal big-endian byte form.
func bigEndianExp(e int) []byte {
	var b []byte
	for e > 0 {
		b = append([]byte{byte(e & 0xff)}, b...)
		e >>= 8
	}
	return b
}
