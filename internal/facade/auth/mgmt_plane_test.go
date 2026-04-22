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
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func writePubKeyPEM(t *testing.T, dir string, key *rsa.PublicKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		t.Fatalf("marshal pkix: %v", err)
	}
	path := filepath.Join(dir, "pub.pem")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create pub.pem: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := pem.Encode(f, &pem.Block{Type: "PUBLIC KEY", Bytes: der}); err != nil {
		t.Fatalf("encode pub.pem: %v", err)
	}
	return path
}

func writeSelfSignedCertPEM(t *testing.T, dir string, key *rsa.PrivateKey) string {
	t.Helper()
	tpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "omnia-dashboard-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	path := filepath.Join(dir, "pub.pem")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create pub.pem: %v", err)
	}
	defer func() { _ = f.Close() }()
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("encode cert: %v", err)
	}
	return path
}

func requestWithToken(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// newValidator constructs an MgmtPlaneValidator with a random RSA keypair
// written to disk as PKIX public-key PEM. Returns the validator and the
// matching private key for the tests to sign tokens with.
func newValidator(t *testing.T) (*auth.MgmtPlaneValidator, *rsa.PrivateKey) {
	t.Helper()
	key := newRSAKey(t)
	path := writePubKeyPEM(t, t.TempDir(), &key.PublicKey)
	v, err := auth.NewMgmtPlaneValidator(path)
	if err != nil {
		t.Fatalf("NewMgmtPlaneValidator: %v", err)
	}
	return v, key
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

func TestMgmtPlaneValidator_AcceptsCertificatePEM(t *testing.T) {
	// Helm's genSelfSigned emits a certificate (tls.crt). The validator
	// must accept that shape too, not just raw PKIX public-key PEM.
	t.Parallel()
	key := newRSAKey(t)
	path := writeSelfSignedCertPEM(t, t.TempDir(), key)
	v, err := auth.NewMgmtPlaneValidator(path)
	if err != nil {
		t.Fatalf("NewMgmtPlaneValidator with cert: %v", err)
	}
	token := mintToken(t, defaultMintOpts(key))
	id, err := v.Validate(context.Background(), requestWithToken(token))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if id == nil || id.Origin != policy.OriginManagementPlane {
		t.Errorf("expected mgmt-plane identity, got %+v", id)
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
	v, _ := newValidator(t)
	otherKey := newRSAKey(t) // different key than the validator trusts
	token := mintToken(t, defaultMintOpts(otherKey))

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
	// HMAC-signed token against an RSA validator must be rejected. If we
	// naively handed the validator's *rsa.PublicKey to jwt.Parse it would
	// panic or accept, depending on library version — guard explicitly.
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

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		_, err := auth.NewMgmtPlaneValidator(filepath.Join(t.TempDir(), "nonexistent.pem"))
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("bad pem", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.pem")
		if err := os.WriteFile(path, []byte("not pem"), 0o600); err != nil {
			t.Fatalf("write bad.pem: %v", err)
		}
		_, err := auth.NewMgmtPlaneValidator(path)
		if err == nil {
			t.Fatal("expected error for bad PEM")
		}
	})

	t.Run("non-rsa key", func(t *testing.T) {
		t.Parallel()
		// Write an EC key — PKIX-parseable but not RSA.
		dir := t.TempDir()
		ecPEM := []byte(`-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEhE0DGbdcbGP/ECD0W99dd6BlMaBL
Rum0+43T0SPpPJUdGuzc3rI80AJ+yAv3MZD3j6SS4Qh5ET7nFyGoiPkfbw==
-----END PUBLIC KEY-----
`)
		path := filepath.Join(dir, "ec.pem")
		if err := os.WriteFile(path, ecPEM, 0o600); err != nil {
			t.Fatalf("write ec.pem: %v", err)
		}
		_, err := auth.NewMgmtPlaneValidator(path)
		if err == nil {
			t.Fatal("expected error for non-RSA key")
		}
	})
}

func TestMgmtPlaneValidator_AcceptsPKCS1PublicKey(t *testing.T) {
	// Some operators may hand-roll a PKCS1 "RSA PUBLIC KEY" PEM — the
	// validator must accept it too.
	t.Parallel()
	key := newRSAKey(t)
	der := x509.MarshalPKCS1PublicKey(&key.PublicKey)
	dir := t.TempDir()
	path := filepath.Join(dir, "pkcs1.pem")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create pkcs1.pem: %v", err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "RSA PUBLIC KEY", Bytes: der}); err != nil {
		_ = f.Close()
		t.Fatalf("encode pkcs1.pem: %v", err)
	}
	_ = f.Close()

	v, err := auth.NewMgmtPlaneValidator(path)
	if err != nil {
		t.Fatalf("NewMgmtPlaneValidator: %v", err)
	}
	token := mintToken(t, defaultMintOpts(key))
	if _, err := v.Validate(context.Background(), requestWithToken(token)); err != nil {
		t.Errorf("PKCS1 pubkey should admit: %v", err)
	}
}

func TestMgmtPlaneValidator_UnsupportedPEMType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ec-priv.pem")
	data := []byte(`-----BEGIN EC PRIVATE KEY-----
MHcCAQEEIAP4HDzRgFNdYaqy5EZGDA4Gz7k7B1JjSoC3bM8XxBYboAoGCCqGSM49
AwEHoUQDQgAEhE0DGbdcbGP/ECD0W99dd6BlMaBLRum0+43T0SPpPJUdGuzc3rI8
0AJ+yAv3MZD3j6SS4Qh5ET7nFyGoiPkfbw==
-----END EC PRIVATE KEY-----
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write ec.pem: %v", err)
	}
	if _, err := auth.NewMgmtPlaneValidator(path); err == nil {
		t.Error("expected error for unsupported PEM block type")
	}
}

func TestMgmtPlaneValidator_CustomIssuerAudience(t *testing.T) {
	// Operators may run multiple dashboards / facades and want to pin
	// non-default issuer / audience values. Validator must honour the
	// options.
	t.Parallel()
	key := newRSAKey(t)
	path := writePubKeyPEM(t, t.TempDir(), &key.PublicKey)
	v, err := auth.NewMgmtPlaneValidator(
		path,
		auth.WithMgmtPlaneIssuer("custom-iss"),
		auth.WithMgmtPlaneAudience("custom-aud"),
	)
	if err != nil {
		t.Fatalf("NewMgmtPlaneValidator: %v", err)
	}

	// Default-minted token has issuer=omnia-dashboard — wrong for this validator.
	wrongToken := mintToken(t, defaultMintOpts(key))
	if _, err := v.Validate(context.Background(), requestWithToken(wrongToken)); !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("default-issuer token should be rejected: err = %v", err)
	}

	opts := defaultMintOpts(key)
	opts.issuer = "custom-iss"
	opts.audience = "custom-aud"
	rightToken := mintToken(t, opts)
	if _, err := v.Validate(context.Background(), requestWithToken(rightToken)); err != nil {
		t.Errorf("custom-issuer token should admit: %v", err)
	}
}

// TestMgmtPlaneValidator_LeewayToleratesSmallDrift proves T1 is fixed:
// tokens just-barely in the future (iat/nbf up to ~15s after the
// facade's clock) must still admit. Dashboards running on a slightly
// different clock than the facade's would otherwise 401 their own
// freshly-minted tokens.
//
// The inverse direction — that the leeway does NOT mask a genuinely
// expired token — is already covered by TestMgmtPlaneValidator_Expired
// (exp = -1 minute, comfortably outside the 30s leeway).
func TestMgmtPlaneValidator_LeewayToleratesSmallDrift(t *testing.T) {
	t.Parallel()
	v, key := newValidator(t)
	future := time.Now().Add(15 * time.Second)
	opts := defaultMintOpts(key)
	opts.iat = future
	opts.nbf = future
	opts.exp = future.Add(5 * time.Minute)
	token := mintToken(t, opts)

	if _, err := v.Validate(context.Background(), requestWithToken(token)); err != nil {
		t.Errorf("token with iat/nbf=+15s should admit under 30s leeway: %v", err)
	}
}
