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
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
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
	testIssuer   = "omnia-dashboard"
	testAudience = "omnia-facade"
)

// thumbprintForKey computes the RFC 7638 JWK thumbprint for an RSA
// public key, matching what dashboard/lib/jwks.js produces. Used as
// the kid the validator sees on minted tokens.
func thumbprintForKey(t *testing.T, key *rsa.PublicKey) string {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
	canonical := fmt.Sprintf(`{"e":%q,"kty":"RSA","n":%q}`, e, n)
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// testMintOpts tunes a single token's claims. Missing fields get safe defaults.
type testMintOpts struct {
	issuer    string
	audience  string
	subject   string
	origin    string
	agent     string
	workspace string
	exp       time.Time
	nbf       time.Time
	iat       time.Time
	key       *rsa.PrivateKey // override signing key
	kid       string          // override JWT header kid (default: thumbprint of key)
	noKid     bool            // omit kid entirely (for negative tests)
	noClaims  bool            // mint a token with jwt.RegisteredClaims only, no origin
}

func defaultMintOpts(key *rsa.PrivateKey) testMintOpts {
	now := time.Now()
	return testMintOpts{
		issuer:    testIssuer,
		audience:  testAudience,
		subject:   "admin@example.com",
		origin:    policy.OriginManagementPlane,
		agent:     "test-agent",
		workspace: "default",
		exp:       now.Add(5 * time.Minute),
		nbf:       now.Add(-1 * time.Minute),
		iat:       now,
		key:       key,
	}
}

type testClaims struct {
	jwt.RegisteredClaims
	Origin    string `json:"origin,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Workspace string `json:"workspace,omitempty"`
}

func mintToken(t *testing.T, opts testMintOpts) string {
	t.Helper()
	claims := testClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    opts.issuer,
			Subject:   opts.subject,
			Audience:  jwt.ClaimStrings{opts.audience},
			ExpiresAt: jwt.NewNumericDate(opts.exp),
			NotBefore: jwt.NewNumericDate(opts.nbf),
			IssuedAt:  jwt.NewNumericDate(opts.iat),
		},
	}
	if !opts.noClaims {
		claims.Origin = opts.origin
		claims.Agent = opts.agent
		claims.Workspace = opts.workspace
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if !opts.noKid {
		kid := opts.kid
		if kid == "" {
			kid = thumbprintForKey(t, &opts.key.PublicKey)
		}
		token.Header["kid"] = kid
	}
	signed, err := token.SignedString(opts.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func newRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return k
}

func requestWithToken(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// newValidator constructs an MgmtPlaneValidator backed by a static
// resolver keyed by the kid the dashboard would emit (RFC 7638
// thumbprint of the public half). Returns the validator and the
// matching private key for the tests to sign tokens with.
func newValidator(t *testing.T) (*auth.MgmtPlaneValidator, *rsa.PrivateKey) {
	t.Helper()
	key := newRSAKey(t)
	kid := thumbprintForKey(t, &key.PublicKey)
	resolver := &auth.StaticKeyResolver{
		Keys: map[string]*rsa.PublicKey{kid: &key.PublicKey},
	}
	return auth.NewMgmtPlaneValidatorWithResolver(resolver), key
}

func TestMgmtPlaneValidator_ValidToken(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	token := mintToken(t, defaultMintOpts(key))

	id, err := v.Validate(context.Background(), requestWithToken(token))
	if err != nil {
		t.Fatalf("Validate returned err: %v", err)
	}
	if id == nil {
		t.Fatal("expected non-nil identity")
	}
	if got, want := id.Origin, policy.OriginManagementPlane; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
	if got, want := id.Subject, "admin@example.com"; got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
	if got, want := id.EndUser, id.Subject; got != want {
		t.Errorf("EndUser = %q, want %q (same as Subject for mgmt-plane)", got, want)
	}
	if got, want := id.Role, policy.RoleAdmin; got != want {
		t.Errorf("Role = %q, want %q", got, want)
	}
	if got, want := id.Agent, "test-agent"; got != want {
		t.Errorf("Agent = %q, want %q", got, want)
	}
	if got, want := id.Workspace, "default"; got != want {
		t.Errorf("Workspace = %q, want %q", got, want)
	}
	if id.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be populated from exp claim")
	}
}

func TestMgmtPlaneValidator_NoAuthorizationHeader(t *testing.T) {
	t.Parallel()
	v, _ := newValidator(t)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil) // no header

	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestMgmtPlaneValidator_NonBearerAuthorization(t *testing.T) {
	t.Parallel()
	v, _ := newValidator(t)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("non-Bearer should fall through: err = %v, want ErrNoCredential", err)
	}
}

func TestMgmtPlaneValidator_EmptyBearerToken(t *testing.T) {
	t.Parallel()
	v, _ := newValidator(t)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Bearer ")

	_, err := v.Validate(context.Background(), r)
	// An empty bearer token is a malformed credential, not an absence.
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_MalformedJWT(t *testing.T) {
	t.Parallel()
	v, _ := newValidator(t)
	r := requestWithToken("not.a.valid.jwt")

	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_BadSignature(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	otherKey := newRSAKey(t) // different key the validator has not registered
	opts := defaultMintOpts(otherKey)
	// Override kid to one the resolver KNOWS (key's), so the resolver
	// returns key's pubkey but the signature was made with otherKey —
	// classic kid spoof. Validator must reject on signature.
	opts.kid = thumbprintForKey(t, &key.PublicKey)
	token := mintToken(t, opts)

	_, err := v.Validate(context.Background(), requestWithToken(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_UnknownKid(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	opts := defaultMintOpts(key)
	opts.kid = "no-such-kid"
	token := mintToken(t, opts)

	_, err := v.Validate(context.Background(), requestWithToken(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_MissingKidHeader(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	opts := defaultMintOpts(key)
	opts.noKid = true
	token := mintToken(t, opts)

	_, err := v.Validate(context.Background(), requestWithToken(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_WrongIssuer(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	opts := defaultMintOpts(key)
	opts.issuer = "someone-else"
	token := mintToken(t, opts)

	_, err := v.Validate(context.Background(), requestWithToken(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_WrongAudience(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	opts := defaultMintOpts(key)
	opts.audience = "some-other-audience"
	token := mintToken(t, opts)

	_, err := v.Validate(context.Background(), requestWithToken(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_WrongOrigin(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	opts := defaultMintOpts(key)
	opts.origin = "data-plane" // reject — the mgmt-plane validator only admits mgmt-plane tokens
	token := mintToken(t, opts)

	_, err := v.Validate(context.Background(), requestWithToken(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_MissingOriginClaim(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	opts := defaultMintOpts(key)
	opts.noClaims = true // RegisteredClaims only, no origin
	token := mintToken(t, opts)

	_, err := v.Validate(context.Background(), requestWithToken(token))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_Expired(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	opts := defaultMintOpts(key)
	opts.exp = time.Now().Add(-1 * time.Minute)
	opts.iat = time.Now().Add(-10 * time.Minute)
	opts.nbf = time.Now().Add(-10 * time.Minute)
	token := mintToken(t, opts)

	_, err := v.Validate(context.Background(), requestWithToken(token))
	if !errors.Is(err, auth.ErrExpired) {
		t.Errorf("err = %v, want ErrExpired", err)
	}
}

func TestMgmtPlaneValidator_WrongSigningMethod(t *testing.T) {
	// HMAC-signed token against an RSA validator must be rejected.
	t.Parallel()
	v, _ := newValidator(t)
	claims := jwt.RegisteredClaims{
		Issuer:    testIssuer,
		Subject:   "admin",
		Audience:  jwt.ClaimStrings{testAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
	}
	hmacToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := hmacToken.SignedString([]byte("shared-secret"))
	if err != nil {
		t.Fatalf("sign hmac token: %v", err)
	}

	_, err = v.Validate(context.Background(), requestWithToken(signed))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestMgmtPlaneValidator_ConstructorErrors(t *testing.T) {
	t.Parallel()

	t.Run("empty JWKS URL", func(t *testing.T) {
		t.Parallel()
		_, err := auth.NewMgmtPlaneValidator("")
		if err == nil {
			t.Fatal("expected error for empty JWKS URL")
		}
	})
}

func TestMgmtPlaneValidator_FetchesFromJWKSEndpoint(t *testing.T) {
	// End-to-end: spin up a real JWKS server, build a validator pointed
	// at it, and verify a token signed with the matching key admits.
	t.Parallel()
	key := newRSAKey(t)
	kid := thumbprintForKey(t, &key.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nB := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
		eB := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())
		_, _ = fmt.Fprintf(w, `{"keys":[{"kty":"RSA","alg":"RS256","use":"sig","kid":%q,"n":%q,"e":%q}]}`, kid, nB, eB)
	}))
	t.Cleanup(srv.Close)

	v, err := auth.NewMgmtPlaneValidator(srv.URL + "/jwks")
	if err != nil {
		t.Fatalf("NewMgmtPlaneValidator: %v", err)
	}
	id, err := v.Validate(context.Background(), requestWithToken(mintToken(t, defaultMintOpts(key))))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if id == nil || id.Origin != policy.OriginManagementPlane {
		t.Errorf("expected mgmt-plane identity, got %+v", id)
	}
}

func TestMgmtPlaneValidator_CustomIssuerAudience(t *testing.T) {
	// Operators may run multiple dashboards / facades and want to pin
	// non-default issuer / audience values. Validator must honour the
	// options.
	t.Parallel()
	key := newRSAKey(t)
	kid := thumbprintForKey(t, &key.PublicKey)
	resolver := &auth.StaticKeyResolver{Keys: map[string]*rsa.PublicKey{kid: &key.PublicKey}}
	v := auth.NewMgmtPlaneValidatorWithResolver(
		resolver,
		auth.WithMgmtPlaneIssuer("custom-iss"),
		auth.WithMgmtPlaneAudience("custom-aud"),
	)

	opts := defaultMintOpts(key)
	opts.issuer = "custom-iss"
	opts.audience = "custom-aud"
	token := mintToken(t, opts)
	if _, err := v.Validate(context.Background(), requestWithToken(token)); err != nil {
		t.Errorf("custom-issuer token should admit: %v", err)
	}

	defaultsToken := mintToken(t, defaultMintOpts(key))
	if _, err := v.Validate(context.Background(), requestWithToken(defaultsToken)); err == nil {
		t.Error("default-issuer token should be rejected by a custom-issuer validator")
	}
}

func TestMgmtPlaneValidator_LeewayToleratesSmallDrift(t *testing.T) {
	// nbf/iat slightly in the future (within jwtLeeway = 30s) must still
	// admit. Without leeway a facade pod a second ahead of the dashboard
	// 401s freshly minted tokens — the C-3 finding.
	t.Parallel()
	v, key := newValidator(t)
	opts := defaultMintOpts(key)
	now := time.Now()
	opts.iat = now.Add(15 * time.Second) // future, but within leeway
	opts.nbf = now.Add(15 * time.Second)
	opts.exp = now.Add(5 * time.Minute)
	token := mintToken(t, opts)

	if _, err := v.Validate(context.Background(), requestWithToken(token)); err != nil {
		t.Errorf("token with iat/nbf=+15s should admit under 30s leeway: %v", err)
	}
}
