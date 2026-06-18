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
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

const testKid = "test-key-1"

func testVerifier(t *testing.T) (*IdentityVerifier, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	resolver := &auth.StaticKeyResolver{Keys: map[string]*rsa.PublicKey{testKid: &key.PublicKey}}
	return NewIdentityVerifier(resolver), key
}

func mintToken(t *testing.T, key *rsa.PrivateKey, kid string, claims IdentityClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func validClaims(now time.Time) IdentityClaims {
	return IdentityClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    IssuerDashboard,
			Audience:  jwt.ClaimStrings{AudienceContentAPI},
			Subject:   "u@x.io",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
		Identity:  "u@x.io",
		Groups:    []string{"eng", "admins"},
		Workspace: "team-a",
	}
}

func TestIdentityVerifier_Valid(t *testing.T) {
	v, key := testVerifier(t)
	token := mintToken(t, key, testKid, validClaims(time.Now()))

	id, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify valid token: %v", err)
	}
	if id.Identity != "u@x.io" {
		t.Errorf("Identity = %q, want u@x.io", id.Identity)
	}
	if id.Workspace != "team-a" {
		t.Errorf("Workspace = %q, want team-a", id.Workspace)
	}
	if len(id.Groups) != 2 || id.Groups[0] != "eng" || id.Groups[1] != "admins" {
		t.Errorf("Groups = %v, want [eng admins]", id.Groups)
	}
	if id.Anonymous {
		t.Errorf("Anonymous = true, want false")
	}
}

func TestIdentityVerifier_Anonymous(t *testing.T) {
	v, key := testVerifier(t)
	claims := validClaims(time.Now())
	claims.Identity = ""
	claims.Subject = "anonymous"
	claims.Groups = nil
	claims.Anonymous = true
	token := mintToken(t, key, testKid, claims)

	id, err := v.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify anonymous token: %v", err)
	}
	if !id.Anonymous {
		t.Errorf("Anonymous = false, want true")
	}
	if id.Identity != "" {
		t.Errorf("Identity = %q, want empty", id.Identity)
	}
}

func TestIdentityVerifier_Expired(t *testing.T) {
	v, key := testVerifier(t)
	claims := validClaims(time.Now().Add(-time.Hour))
	token := mintToken(t, key, testKid, claims)

	_, err := v.Verify(context.Background(), token)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("expired token: err = %v, want ErrExpired", err)
	}
}

func TestIdentityVerifier_Tampered(t *testing.T) {
	v, key := testVerifier(t)
	token := mintToken(t, key, testKid, validClaims(time.Now()))
	// Flip the last character of the signature.
	tampered := token[:len(token)-1] + string(rune(token[len(token)-1])^0x01)

	_, err := v.Verify(context.Background(), tampered)
	if err == nil {
		t.Fatalf("tampered token: want error, got nil")
	}
	if errors.Is(err, ErrExpired) {
		t.Fatalf("tampered token: got ErrExpired, want invalid")
	}
}

func TestIdentityVerifier_WrongAudience(t *testing.T) {
	v, key := testVerifier(t)
	claims := validClaims(time.Now())
	claims.Audience = jwt.ClaimStrings{"omnia-facade"}
	token := mintToken(t, key, testKid, claims)

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatalf("wrong audience: want error, got nil")
	}
}

func TestIdentityVerifier_WrongIssuer(t *testing.T) {
	v, key := testVerifier(t)
	claims := validClaims(time.Now())
	claims.Issuer = "someone-else"
	token := mintToken(t, key, testKid, claims)

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatalf("wrong issuer: want error, got nil")
	}
}

func TestIdentityVerifier_MissingKid(t *testing.T) {
	v, key := testVerifier(t)
	token := mintToken(t, key, "", validClaims(time.Now()))

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatalf("missing kid: want error, got nil")
	}
}

func TestIdentityVerifier_UnknownKid(t *testing.T) {
	v, key := testVerifier(t)
	token := mintToken(t, key, "other-kid", validClaims(time.Now()))

	if _, err := v.Verify(context.Background(), token); err == nil {
		t.Fatalf("unknown kid: want error, got nil")
	}
}

func TestIdentityVerifier_EmptyToken(t *testing.T) {
	v, _ := testVerifier(t)
	if _, err := v.Verify(context.Background(), ""); err == nil {
		t.Fatalf("empty token: want error, got nil")
	}
}
