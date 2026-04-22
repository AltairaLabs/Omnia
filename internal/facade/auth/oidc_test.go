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

package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

const (
	testOIDCIssuer   = "https://idp.example.com"
	testOIDCAudience = "omnia"
	testOIDCKid      = "test-key-1"
	// testAliceEmail is defined in edge_trust_test.go (PR 2e, already
	// on main); referenced here to avoid the goconst lint warning.
)

// newOIDCTestKey generates an RSA keypair and returns it plus a JWKS
// containing only the public half keyed by testOIDCKid.
func newOIDCTestKey(t *testing.T) (*rsa.PrivateKey, *auth.JWKS) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	jwks := &auth.JWKS{Keys: []auth.JSONWebKey{jwkFromRSA(&key.PublicKey, testOIDCKid)}}
	return key, jwks
}

func jwkFromRSA(pub *rsa.PublicKey, kid string) auth.JSONWebKey {
	return auth.JSONWebKey{
		Kty: "RSA",
		Kid: kid,
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

type oidcMintOpts struct {
	kid      string
	issuer   string
	audience string
	subject  string
	exp      time.Time
	extras   map[string]any
	key      *rsa.PrivateKey
	alg      jwt.SigningMethod
}

func mintOIDCToken(t *testing.T, opts oidcMintOpts) string {
	t.Helper()
	now := time.Now()
	if opts.exp.IsZero() {
		opts.exp = now.Add(5 * time.Minute)
	}
	claims := jwt.MapClaims{
		"iss": opts.issuer,
		"aud": opts.audience,
		"sub": opts.subject,
		"iat": jwt.NewNumericDate(now).Unix(),
		"nbf": jwt.NewNumericDate(now.Add(-time.Minute)).Unix(),
		"exp": jwt.NewNumericDate(opts.exp).Unix(),
	}
	for k, v := range opts.extras {
		claims[k] = v
	}
	alg := opts.alg
	if alg == nil {
		alg = jwt.SigningMethodRS256
	}
	token := jwt.NewWithClaims(alg, claims)
	if opts.kid != "" {
		token.Header["kid"] = opts.kid
	}
	signed, err := token.SignedString(opts.key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func oidcReq(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func newOIDCValidatorForTest(t *testing.T, opts ...auth.OIDCOption) (*auth.OIDCValidator, *rsa.PrivateKey) {
	t.Helper()
	key, jwks := newOIDCTestKey(t)
	set, err := auth.NewKeySet(jwks)
	if err != nil {
		t.Fatalf("NewKeySet: %v", err)
	}
	v, err := auth.NewOIDCValidator(testOIDCIssuer, testOIDCAudience, set, opts...)
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}
	return v, key
}

func TestNewOIDCValidator_RequiresArgs(t *testing.T) {
	t.Parallel()
	key, jwks := newOIDCTestKey(t)
	set, _ := auth.NewKeySet(jwks)
	_ = key

	if _, err := auth.NewOIDCValidator("", testOIDCAudience, set); err == nil {
		t.Error("expected error on empty issuer")
	}
	if _, err := auth.NewOIDCValidator(testOIDCIssuer, "", set); err == nil {
		t.Error("expected error on empty audience")
	}
	if _, err := auth.NewOIDCValidator(testOIDCIssuer, testOIDCAudience, nil); err == nil {
		t.Error("expected error on nil KeySupplier")
	}
}

func TestNewKeySet_RejectsEmptyJWKS(t *testing.T) {
	t.Parallel()
	if _, err := auth.NewKeySet(nil); err == nil {
		t.Error("expected error on nil JWKS")
	}
	empty := &auth.JWKS{Keys: []auth.JSONWebKey{}}
	if _, err := auth.NewKeySet(empty); err == nil {
		t.Error("expected error on zero-key JWKS")
	}
}

func TestNewKeySet_SkipsNonRSAKeys(t *testing.T) {
	// An EC key in the JWKS should be skipped without failing
	// construction — future-proofs against JWKS responses that mix
	// algorithms.
	t.Parallel()
	_, jwks := newOIDCTestKey(t)
	jwks.Keys = append(jwks.Keys, auth.JSONWebKey{Kty: "EC", Kid: "ec-key"})
	set, err := auth.NewKeySet(jwks)
	if err != nil {
		t.Fatalf("NewKeySet: %v", err)
	}
	if _, ok := set.Lookup("ec-key"); ok {
		t.Error("EC key should have been skipped")
	}
	if _, ok := set.Lookup(testOIDCKid); !ok {
		t.Error("RSA key should still be usable alongside skipped EC key")
	}
}

func TestNewKeySet_SkipsKidlessEntries(t *testing.T) {
	// A JWK with no kid is legal under RFC 7517 but useless to us —
	// skip rather than fail construction.
	t.Parallel()
	key, _ := newOIDCTestKey(t)
	jwkNoKid := jwkFromRSA(&key.PublicKey, "") // empty kid
	jwkWithKid := jwkFromRSA(&key.PublicKey, "other-kid")
	set, err := auth.NewKeySet(&auth.JWKS{Keys: []auth.JSONWebKey{jwkNoKid, jwkWithKid}})
	if err != nil {
		t.Fatalf("NewKeySet: %v", err)
	}
	if _, ok := set.Lookup(""); ok {
		t.Error("empty kid should not be indexable")
	}
	if _, ok := set.Lookup("other-kid"); !ok {
		t.Error("kidded key should be usable")
	}
}

func TestNewKeySetFromJSON_ParsesIssuerResponse(t *testing.T) {
	t.Parallel()
	_, jwks := newOIDCTestKey(t)
	blob, err := json.Marshal(jwks)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	set, err := auth.NewKeySetFromJSON(blob)
	if err != nil {
		t.Fatalf("NewKeySetFromJSON: %v", err)
	}
	if _, ok := set.Lookup(testOIDCKid); !ok {
		t.Error("key lookup failed after NewKeySetFromJSON")
	}
}

func TestNewKeySetFromJSON_RejectsGarbage(t *testing.T) {
	t.Parallel()
	if _, err := auth.NewKeySetFromJSON([]byte("not-json")); err == nil {
		t.Error("expected error on garbage JSON")
	}
}

func TestOIDCValidator_AdmitsValidToken(t *testing.T) {
	t.Parallel()
	v, key := newOIDCValidatorForTest(t)
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      testOIDCKid,
		issuer:   testOIDCIssuer,
		audience: testOIDCAudience,
		subject:  testAliceEmail,
		extras: map[string]any{
			"omnia.role": policy.RoleEditor,
			"email":      testAliceEmail,
			"groups":     "finance",
		},
		key: key,
	})

	id, err := v.Validate(context.Background(), oidcReq(token))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if id == nil {
		t.Fatal("nil identity on admit")
	}
	if got, want := id.Origin, policy.OriginOIDC; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
	if got, want := id.Subject, testAliceEmail; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if got, want := id.Role, policy.RoleEditor; got != want {
		t.Errorf("Role = %q, want %q", got, want)
	}
	if got, want := id.EndUser, id.Subject; got != want {
		t.Errorf("EndUser = %q, want %q (default mapping → Subject)", got, want)
	}
	if got, want := id.Claims["email"], testAliceEmail; got != want {
		t.Errorf("Claims[email] = %q, want %q", got, want)
	}
	if got, want := id.Claims["groups"], "finance"; got != want {
		t.Errorf("Claims[groups] = %q, want %q", got, want)
	}
	if id.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be populated from exp claim")
	}
}

func TestOIDCValidator_RejectsWrongIssuer(t *testing.T) {
	t.Parallel()
	v, key := newOIDCValidatorForTest(t)
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      testOIDCKid,
		issuer:   "https://other-idp.example.com",
		audience: testOIDCAudience,
		subject:  "alice",
		key:      key,
	})

	_, err := v.Validate(context.Background(), oidcReq(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestOIDCValidator_RejectsWrongAudience(t *testing.T) {
	t.Parallel()
	v, key := newOIDCValidatorForTest(t)
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      testOIDCKid,
		issuer:   testOIDCIssuer,
		audience: "not-omnia",
		subject:  "alice",
		key:      key,
	})

	_, err := v.Validate(context.Background(), oidcReq(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestOIDCValidator_RejectsExpiredToken(t *testing.T) {
	t.Parallel()
	v, key := newOIDCValidatorForTest(t)
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      testOIDCKid,
		issuer:   testOIDCIssuer,
		audience: testOIDCAudience,
		subject:  "alice",
		exp:      time.Now().Add(-time.Minute),
		key:      key,
	})

	_, err := v.Validate(context.Background(), oidcReq(token))
	if !errors.Is(err, auth.ErrExpired) {
		t.Errorf("err = %v, want ErrExpired", err)
	}
}

func TestOIDCValidator_RejectsUnknownKid(t *testing.T) {
	// A JWT signed by a key whose kid isn't in the JWKS cache should be
	// rejected — this is the kid-rotation-lag case the design doc
	// flagged. Facade annotates AgentRuntime for refresh in PR 2d-2;
	// for MVP we just reject.
	t.Parallel()
	v, key := newOIDCValidatorForTest(t)
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      "unknown-kid",
		issuer:   testOIDCIssuer,
		audience: testOIDCAudience,
		subject:  "alice",
		key:      key,
	})

	_, err := v.Validate(context.Background(), oidcReq(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestOIDCValidator_RejectsTokenWithoutKid(t *testing.T) {
	t.Parallel()
	v, key := newOIDCValidatorForTest(t)
	token := mintOIDCToken(t, oidcMintOpts{
		// no kid
		issuer:   testOIDCIssuer,
		audience: testOIDCAudience,
		subject:  "alice",
		key:      key,
	})

	_, err := v.Validate(context.Background(), oidcReq(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestOIDCValidator_RejectsWrongSigningKey(t *testing.T) {
	t.Parallel()
	v, _ := newOIDCValidatorForTest(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      testOIDCKid,
		issuer:   testOIDCIssuer,
		audience: testOIDCAudience,
		subject:  "alice",
		key:      otherKey,
	})

	_, e := v.Validate(context.Background(), oidcReq(token))
	if !errors.Is(e, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", e)
	}
}

func TestOIDCValidator_RejectsHMACSigningMethod(t *testing.T) {
	// A token signed with HS256 against the validator's RSA key material
	// must be rejected even if the bytes accidentally verify — the
	// WithValidMethods option should short-circuit.
	t.Parallel()
	v, _ := newOIDCValidatorForTest(t)
	hmac := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": testOIDCIssuer,
		"aud": testOIDCAudience,
		"sub": "alice",
		"exp": jwt.NewNumericDate(time.Now().Add(5 * time.Minute)).Unix(),
	})
	hmac.Header["kid"] = testOIDCKid
	signed, err := hmac.SignedString([]byte("shared"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := v.Validate(context.Background(), oidcReq(signed)); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("HMAC token: err = %v, want ErrInvalidCredential", err)
	}
}

func TestOIDCValidator_NoBearerFallsThrough(t *testing.T) {
	t.Parallel()
	v, _ := newOIDCValidatorForTest(t)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestOIDCValidator_CustomClaimMapping(t *testing.T) {
	t.Parallel()
	mapping := auth.OIDCClaimMapping{
		Subject: "user_id",
		Role:    "tier",
		EndUser: "actor",
	}
	v, key := newOIDCValidatorForTest(t, auth.WithOIDCClaimMapping(mapping))
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      testOIDCKid,
		issuer:   testOIDCIssuer,
		audience: testOIDCAudience,
		subject:  "service-token",
		extras: map[string]any{
			"user_id": "svc-payroll",
			"tier":    policy.RoleAdmin,
			"actor":   testAliceEmail,
		},
		key: key,
	})

	id, err := v.Validate(context.Background(), oidcReq(token))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Default sub claim goes into jwt's Subject but NOT our Identity
	// because we mapped Subject to "user_id".
	if got, want := id.Subject, "svc-payroll"; got != want {
		t.Errorf("Subject = %q, want %q (custom mapping user_id → Subject)", got, want)
	}
	if got, want := id.Role, policy.RoleAdmin; got != want {
		t.Errorf("Role = %q, want %q", got, want)
	}
	if got, want := id.EndUser, testAliceEmail; got != want {
		t.Errorf("EndUser = %q, want %q (service token → actor)", got, want)
	}
}

func TestOIDCValidator_EndUserFallsBackToSubjectWhenMappedClaimMissing(t *testing.T) {
	// The CRD says "If the named claim is missing from a given token,
	// the validator falls back to Subject." Test that explicitly.
	t.Parallel()
	v, key := newOIDCValidatorForTest(t, auth.WithOIDCClaimMapping(auth.OIDCClaimMapping{
		EndUser: "actor", // claim name that isn't present
	}))
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      testOIDCKid,
		issuer:   testOIDCIssuer,
		audience: testOIDCAudience,
		subject:  testAliceEmail,
		key:      key,
	})

	id, err := v.Validate(context.Background(), oidcReq(token))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if id.EndUser != testAliceEmail {
		t.Errorf("EndUser = %q, want %q (fallback to Subject when mapped claim missing)",
			id.EndUser, testAliceEmail)
	}
}

func TestOIDCValidator_KeySetReplaceAllowsHotReload(t *testing.T) {
	// KeySet.Replace atomically swaps the map — proves the cmd/agent
	// periodic-refresh pattern works without reconstructing the validator.
	t.Parallel()
	key1, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key1: %v", err)
	}
	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key2: %v", err)
	}
	jwks1 := &auth.JWKS{Keys: []auth.JSONWebKey{jwkFromRSA(&key1.PublicKey, "k1")}}
	set, err := auth.NewKeySet(jwks1)
	if err != nil {
		t.Fatalf("NewKeySet: %v", err)
	}
	v, err := auth.NewOIDCValidator(testOIDCIssuer, testOIDCAudience, set)
	if err != nil {
		t.Fatalf("NewOIDCValidator: %v", err)
	}

	// Token signed with key1 — admits
	t1 := mintOIDCToken(t, oidcMintOpts{
		kid: "k1", issuer: testOIDCIssuer, audience: testOIDCAudience,
		subject: "alice", key: key1,
	})
	if _, err := v.Validate(context.Background(), oidcReq(t1)); err != nil {
		t.Fatalf("before rotate: %v", err)
	}

	// Rotate: swap in key2 under kid=k2. set now knows only k2.
	set.Replace(map[string]*rsa.PublicKey{"k2": &key2.PublicKey})

	// Old kid is gone.
	if _, err := v.Validate(context.Background(), oidcReq(t1)); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("after rotate: old kid should be unknown, got err = %v", err)
	}

	// New kid + new key admits.
	t2 := mintOIDCToken(t, oidcMintOpts{
		kid: "k2", issuer: testOIDCIssuer, audience: testOIDCAudience,
		subject: "alice", key: key2,
	})
	if _, err := v.Validate(context.Background(), oidcReq(t2)); err != nil {
		t.Errorf("after rotate, new key: %v", err)
	}
}

// TestOIDCValidator_LeewayToleratesSmallDrift proves T1 is fixed for
// the OIDC path: a token with nbf/iat a few seconds after the
// validator's clock must still admit. Cross-cloud IdPs commonly drift
// ~1-5s; tokens freshly minted by the IdP would otherwise 401.
//
// The inverse — leeway doesn't mask genuine expiry — is already
// covered by TestOIDCValidator_RejectsExpiredToken (exp = -1 minute,
// beyond the 30s leeway).
func TestOIDCValidator_LeewayToleratesSmallDrift(t *testing.T) {
	t.Parallel()
	v, key := newOIDCValidatorForTest(t)
	future := time.Now().Add(15 * time.Second)
	token := mintOIDCToken(t, oidcMintOpts{
		kid:      testOIDCKid,
		issuer:   testOIDCIssuer,
		audience: testOIDCAudience,
		subject:  "alice",
		key:      key,
		exp:      future.Add(5 * time.Minute),
		extras: map[string]any{
			"iat": future.Unix(),
			"nbf": future.Unix(),
		},
	})

	if _, err := v.Validate(context.Background(), oidcReq(token)); err != nil {
		t.Errorf("token with iat/nbf=+15s should admit under 30s leeway: %v", err)
	}
}
