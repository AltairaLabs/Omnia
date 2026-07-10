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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/altairalabs/omnia/pkg/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

const validClientKey = "omk_test_raw_value_for_unit_tests"

func newClientKeyStore(t *testing.T, opts ...func(*auth.ClientKey)) *auth.StaticKeyStore {
	t.Helper()
	hashHex := auth.HashToken(validClientKey)
	k := auth.ClientKey{
		ID:      "key-001",
		HashHex: hashHex,
		Claims:  map[string]string{"role": policy.RoleEditor},
	}
	for _, opt := range opts {
		opt(&k)
	}
	return auth.NewStaticKeyStore(map[string]auth.ClientKey{hashHex: k})
}

func reqWithBearer(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

// mismatchStore returns a key whose HashHex never matches the looked-up
// candidate, exercising the validator's defence-in-depth constant-time compare
// (a store that returned a key under the wrong hash must still be rejected).
type mismatchStore struct{}

func (mismatchStore) Lookup(string) (auth.ClientKey, bool) {
	return auth.ClientKey{ID: "k1", HashHex: "0000000000deadbeef"}, true
}

func TestClientKeyValidator_HashMismatchFallsThrough(t *testing.T) {
	t.Parallel()
	v := auth.NewClientKeyValidator(mismatchStore{})
	_, err := v.Validate(context.Background(), reqWithBearer("some-token"))
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Fatalf("Validate err = %v, want ErrNoCredential (hash-mismatch defence)", err)
	}
}

func TestHashToken_StableAndDeterministic(t *testing.T) {
	t.Parallel()
	a := auth.HashToken("alpha")
	b := auth.HashToken("alpha")
	c := auth.HashToken("alpha-different")
	if a != b {
		t.Errorf("HashToken not deterministic: %q vs %q", a, b)
	}
	if a == c {
		t.Error("HashToken collision on different inputs")
	}
	// sha256 hex is 64 chars.
	if len(a) != 64 {
		t.Errorf("HashToken length = %d, want 64 (hex sha256)", len(a))
	}
}

func TestClientKeyValidator_AdmitsKnownKey(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t)
	v := auth.NewClientKeyValidator(store)

	id, err := v.Validate(context.Background(), reqWithBearer(validClientKey))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if id == nil {
		t.Fatal("nil identity on admit")
	}
	if got, want := id.Origin, policy.OriginClientKey; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
	if got, want := id.Subject, "key-001"; got != want {
		t.Errorf("Subject = %q, want %q (key ID)", got, want)
	}
	if id.EndUser != id.Subject {
		t.Errorf("EndUser = %q, want %q (no trustEndUserHeader)", id.EndUser, id.Subject)
	}
}

func TestClientKeyValidator_UnknownKeyFallsThrough(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t)
	v := auth.NewClientKeyValidator(store)
	_, err := v.Validate(context.Background(), reqWithBearer("unknown-key-value"))
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential (unknown bearer is not a client-key-style credential)", err)
	}
}

func TestClientKeyValidator_ChainFallsThroughToLaterValidator(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t) // contains a known key, but we present a different bearer
	wantSubject := "mgmt-admin"
	admit := &stubValidator{id: &policy.AuthenticatedIdentity{Origin: policy.OriginManagementPlane, Subject: wantSubject}}
	chain := auth.Chain{auth.NewClientKeyValidator(store), admit}

	id, err := chain.Run(context.Background(), reqWithBearer("a-mgmt-plane-jwt-shaped-bearer"))
	if err != nil {
		t.Fatalf("Run err = %v, want nil (clientKeys must fall through to the admitting validator)", err)
	}
	if id == nil || id.Subject != wantSubject {
		t.Errorf("identity = %+v, want admitted by the later validator", id)
	}
	if admit.called != 1 {
		t.Errorf("later validator called %d times, want 1 (clientKeys short-circuited the chain)", admit.called)
	}
}

func TestClientKeyValidator_NoBearerFallsThrough(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t)
	v := auth.NewClientKeyValidator(store)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestClientKeyValidator_NonBearerFallsThrough(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t)
	v := auth.NewClientKeyValidator(store)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Basic xxx")
	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestClientKeyValidator_ExpiredKeyReturnsErrExpired(t *testing.T) {
	t.Parallel()
	past := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	store := newClientKeyStore(t, func(k *auth.ClientKey) { k.ExpiresAt = past })
	v := auth.NewClientKeyValidator(store, auth.WithClientKeyClock(func() time.Time {
		return time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	}))
	_, err := v.Validate(context.Background(), reqWithBearer(validClientKey))
	if !errors.Is(err, auth.ErrExpired) {
		t.Errorf("err = %v, want ErrExpired", err)
	}
}

func TestClientKeyValidator_NotYetExpiredAdmits(t *testing.T) {
	t.Parallel()
	future := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	store := newClientKeyStore(t, func(k *auth.ClientKey) { k.ExpiresAt = future })
	v := auth.NewClientKeyValidator(store, auth.WithClientKeyClock(func() time.Time {
		return time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	}))
	id, err := v.Validate(context.Background(), reqWithBearer(validClientKey))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !id.ExpiresAt.Equal(future) {
		t.Errorf("Identity.ExpiresAt = %v, want %v", id.ExpiresAt, future)
	}
}

func TestClientKeyValidator_NoExpiryNeverExpires(t *testing.T) {
	t.Parallel()
	// Zero ExpiresAt means "no expiry" — admit indefinitely.
	store := newClientKeyStore(t)
	v := auth.NewClientKeyValidator(store, auth.WithClientKeyClock(func() time.Time {
		return time.Date(9999, time.January, 1, 0, 0, 0, 0, time.UTC)
	}))
	if _, err := v.Validate(context.Background(), reqWithBearer(validClientKey)); err != nil {
		t.Errorf("err = %v, want nil for no-expiry key", err)
	}
}

func TestClientKeyValidator_DefaultRoleAppliedWhenSecretMissingClaims(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t, func(k *auth.ClientKey) { k.Claims = nil })
	v := auth.NewClientKeyValidator(store, auth.WithClientKeyDefaultRole(policy.RoleViewer))
	id, err := v.Validate(context.Background(), reqWithBearer(validClientKey))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.Claims["role"], policy.RoleViewer; got != want {
		t.Errorf("Claims[role] = %q, want %q (default applied)", got, want)
	}
}

func TestClientKeyValidator_TrustEndUserHeader(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t)
	v := auth.NewClientKeyValidator(store, auth.WithClientKeyTrustEndUserHeader(true))
	r := reqWithBearer(validClientKey)
	r.Header.Set(auth.EndUserHeader, testAliceEmail)

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.EndUser, testAliceEmail; got != want {
		t.Errorf("EndUser = %q, want %q (header should propagate)", got, want)
	}
	if id.Subject == id.EndUser {
		t.Error("Subject should remain the key ID, not equal EndUser")
	}
}

func TestClientKeyValidator_TrustEndUserHeaderDefaultsOff(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t)
	v := auth.NewClientKeyValidator(store)
	r := reqWithBearer(validClientKey)
	r.Header.Set(auth.EndUserHeader, testAliceEmail) // ignored

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if id.EndUser != id.Subject {
		t.Errorf("EndUser = %q, want %q (header ignored when flag off)", id.EndUser, id.Subject)
	}
}

func TestStaticKeyStore_NilMapIsSafe(t *testing.T) {
	t.Parallel()
	s := auth.NewStaticKeyStore(nil)
	if _, ok := s.Lookup("anything"); ok {
		t.Error("expected miss on empty store")
	}
}

func TestClientKeyValidator_RoleSurfacesAsClaim(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t)
	v := auth.NewClientKeyValidator(store)

	id, err := v.Validate(context.Background(), reqWithBearer(validClientKey))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.Claims["role"], policy.RoleEditor; got != want {
		t.Fatalf("Claims[role] = %q, want %q", got, want)
	}
}

func TestClientKeyValidator_ArbitraryClaimsSurfaceVerbatim(t *testing.T) {
	t.Parallel()
	store := newClientKeyStore(t, func(k *auth.ClientKey) {
		k.Claims = map[string]string{"role": policy.RoleAdmin, "team": "growth", "region": "eu"}
	})
	v := auth.NewClientKeyValidator(store)

	id, err := v.Validate(context.Background(), reqWithBearer(validClientKey))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.Claims["team"], "growth"; got != want {
		t.Errorf("Claims[team] = %q, want %q", got, want)
	}
	if got, want := id.Claims["region"], "eu"; got != want {
		t.Errorf("Claims[region] = %q, want %q", got, want)
	}
	if got, want := id.Claims["role"], policy.RoleAdmin; got != want {
		t.Errorf("Claims[role] = %q, want %q", got, want)
	}
}
