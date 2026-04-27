/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/altairalabs/omnia/internal/facade/auth"
)

// jwkFromKey returns a public JWK with kid = RFC 7638 thumbprint, in
// the same shape the dashboard's lib/jwks.js produces. Lets the facade
// test stand up a fake JWKS endpoint without dragging Node into the
// Go test path.
func jwkFromKey(t *testing.T, key *rsa.PublicKey) map[string]string {
	t.Helper()
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	eBuf := big.NewInt(int64(key.E)).Bytes()
	e := base64.RawURLEncoding.EncodeToString(eBuf)
	canonical := fmt.Sprintf(`{"e":%q,"kty":"RSA","n":%q}`, e, n)
	sum := sha256.Sum256([]byte(canonical))
	kid := base64.RawURLEncoding.EncodeToString(sum[:])
	return map[string]string{
		"kty": "RSA",
		"alg": "RS256",
		"use": "sig",
		"kid": kid,
		"n":   n,
		"e":   e,
	}
}

func newJWKSServer(t *testing.T, keys ...*rsa.PublicKey) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		jwks := []map[string]string{}
		for _, k := range keys {
			jwks = append(jwks, jwkFromKey(t, k))
		}
		w.Header().Set("Content-Type", "application/jwk-set+json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": jwks})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestJWKSResolver_FetchesKey(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	srv, hits := newJWKSServer(t, &key.PublicKey)

	r := auth.NewJWKSResolver(srv.URL + "/jwks")
	jwk := jwkFromKey(t, &key.PublicKey)

	got, err := r.Resolve(context.Background(), jwk["kid"])
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.N.Cmp(key.N) != 0 {
		t.Errorf("modulus mismatch")
	}
	if got.E != key.E {
		t.Errorf("exponent mismatch: got %d want %d", got.E, key.E)
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 JWKS fetch, got %d", hits.Load())
	}
}

func TestJWKSResolver_CachesByKid(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv, hits := newJWKSServer(t, &key.PublicKey)
	r := auth.NewJWKSResolver(srv.URL + "/jwks")
	kid := jwkFromKey(t, &key.PublicKey)["kid"]

	for range 5 {
		if _, err := r.Resolve(context.Background(), kid); err != nil {
			t.Fatalf("Resolve: %v", err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("expected 1 fetch (cached), got %d", hits.Load())
	}
}

func TestJWKSResolver_RefreshesOnUnknownKid(t *testing.T) {
	t.Parallel()
	// Server initially returns key A; after the first resolve we swap it
	// to key B. The resolver must re-fetch when asked for B's kid.
	keyA, _ := rsa.GenerateKey(rand.Reader, 2048)
	keyB, _ := rsa.GenerateKey(rand.Reader, 2048)
	current := []*rsa.PublicKey{&keyA.PublicKey}
	mux := http.NewServeMux()
	var hits atomic.Int32
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		jwks := []map[string]string{}
		for _, k := range current {
			jwks = append(jwks, jwkFromKey(t, k))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": jwks})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	r := auth.NewJWKSResolver(srv.URL + "/jwks")
	kidA := jwkFromKey(t, &keyA.PublicKey)["kid"]
	kidB := jwkFromKey(t, &keyB.PublicKey)["kid"]

	// First resolve fills the cache from key A.
	if _, err := r.Resolve(context.Background(), kidA); err != nil {
		t.Fatalf("Resolve A: %v", err)
	}
	// Swap server to key B.
	current = []*rsa.PublicKey{&keyB.PublicKey}

	// Asking for kid B forces a refresh.
	got, err := r.Resolve(context.Background(), kidB)
	if err != nil {
		t.Fatalf("Resolve B: %v", err)
	}
	if got.N.Cmp(keyB.N) != 0 {
		t.Errorf("modulus mismatch on rotated key")
	}
	if hits.Load() != 2 {
		t.Errorf("expected 2 fetches (initial + refresh), got %d", hits.Load())
	}
}

func TestJWKSResolver_ErrorOnNon200(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	r := auth.NewJWKSResolver(srv.URL + "/jwks")
	_, err := r.Resolve(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestJWKSResolver_ErrorOnUnknownKidAfterRefresh(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	srv, _ := newJWKSServer(t, &key.PublicKey)
	r := auth.NewJWKSResolver(srv.URL + "/jwks")
	_, err := r.Resolve(context.Background(), "no-such-kid")
	if !errors.Is(err, auth.ErrUnknownKid) {
		t.Errorf("err = %v, want ErrUnknownKid", err)
	}
}

func TestStaticKeyResolver(t *testing.T) {
	t.Parallel()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	r := &auth.StaticKeyResolver{Keys: map[string]*rsa.PublicKey{"abc": &key.PublicKey}}

	got, err := r.Resolve(context.Background(), "abc")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != &key.PublicKey {
		t.Errorf("got different key pointer")
	}
	if _, err := r.Resolve(context.Background(), "missing"); !errors.Is(err, auth.ErrUnknownKid) {
		t.Errorf("err = %v, want ErrUnknownKid", err)
	}
}
