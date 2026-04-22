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

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

const validAPIKey = "omk_test_raw_value_for_unit_tests"

func newAPIKeyStore(t *testing.T, opts ...func(*auth.APIKey)) *auth.StaticKeyStore {
	t.Helper()
	hashHex := auth.HashToken(validAPIKey)
	k := auth.APIKey{
		ID:      "key-001",
		HashHex: hashHex,
		Role:    policy.RoleEditor,
	}
	for _, opt := range opts {
		opt(&k)
	}
	return auth.NewStaticKeyStore(map[string]auth.APIKey{hashHex: k})
}

func reqWithBearer(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
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

func TestAPIKeyValidator_AdmitsKnownKey(t *testing.T) {
	t.Parallel()
	store := newAPIKeyStore(t)
	v := auth.NewAPIKeyValidator(store)

	id, err := v.Validate(context.Background(), reqWithBearer(validAPIKey))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if id == nil {
		t.Fatal("nil identity on admit")
	}
	if got, want := id.Origin, policy.OriginAPIKey; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
	if got, want := id.Subject, "key-001"; got != want {
		t.Errorf("Subject = %q, want %q (key ID)", got, want)
	}
	if got, want := id.Role, policy.RoleEditor; got != want {
		t.Errorf("Role = %q, want %q", got, want)
	}
	if id.EndUser != id.Subject {
		t.Errorf("EndUser = %q, want %q (no trustEndUserHeader)", id.EndUser, id.Subject)
	}
}

func TestAPIKeyValidator_RejectsUnknownKey(t *testing.T) {
	t.Parallel()
	store := newAPIKeyStore(t)
	v := auth.NewAPIKeyValidator(store)
	_, err := v.Validate(context.Background(), reqWithBearer("unknown-key-value"))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestAPIKeyValidator_NoBearerFallsThrough(t *testing.T) {
	t.Parallel()
	store := newAPIKeyStore(t)
	v := auth.NewAPIKeyValidator(store)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestAPIKeyValidator_NonBearerFallsThrough(t *testing.T) {
	t.Parallel()
	store := newAPIKeyStore(t)
	v := auth.NewAPIKeyValidator(store)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Basic xxx")
	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestAPIKeyValidator_ExpiredKeyReturnsErrExpired(t *testing.T) {
	t.Parallel()
	past := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
	store := newAPIKeyStore(t, func(k *auth.APIKey) { k.ExpiresAt = past })
	v := auth.NewAPIKeyValidator(store, auth.WithAPIKeyClock(func() time.Time {
		return time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	}))
	_, err := v.Validate(context.Background(), reqWithBearer(validAPIKey))
	if !errors.Is(err, auth.ErrExpired) {
		t.Errorf("err = %v, want ErrExpired", err)
	}
}

func TestAPIKeyValidator_NotYetExpiredAdmits(t *testing.T) {
	t.Parallel()
	future := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	store := newAPIKeyStore(t, func(k *auth.APIKey) { k.ExpiresAt = future })
	v := auth.NewAPIKeyValidator(store, auth.WithAPIKeyClock(func() time.Time {
		return time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	}))
	id, err := v.Validate(context.Background(), reqWithBearer(validAPIKey))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !id.ExpiresAt.Equal(future) {
		t.Errorf("Identity.ExpiresAt = %v, want %v", id.ExpiresAt, future)
	}
}

func TestAPIKeyValidator_NoExpiryNeverExpires(t *testing.T) {
	t.Parallel()
	// Zero ExpiresAt means "no expiry" — admit indefinitely.
	store := newAPIKeyStore(t)
	v := auth.NewAPIKeyValidator(store, auth.WithAPIKeyClock(func() time.Time {
		return time.Date(9999, time.January, 1, 0, 0, 0, 0, time.UTC)
	}))
	if _, err := v.Validate(context.Background(), reqWithBearer(validAPIKey)); err != nil {
		t.Errorf("err = %v, want nil for no-expiry key", err)
	}
}

func TestAPIKeyValidator_DefaultRoleAppliedWhenSecretMissingRole(t *testing.T) {
	t.Parallel()
	store := newAPIKeyStore(t, func(k *auth.APIKey) { k.Role = "" })
	v := auth.NewAPIKeyValidator(store, auth.WithAPIKeyDefaultRole(policy.RoleViewer))
	id, err := v.Validate(context.Background(), reqWithBearer(validAPIKey))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.Role, policy.RoleViewer; got != want {
		t.Errorf("Role = %q, want %q (default applied)", got, want)
	}
}

func TestAPIKeyValidator_TrustEndUserHeader(t *testing.T) {
	t.Parallel()
	store := newAPIKeyStore(t)
	v := auth.NewAPIKeyValidator(store, auth.WithAPIKeyTrustEndUserHeader(true))
	r := reqWithBearer(validAPIKey)
	r.Header.Set(auth.EndUserHeader, "alice@example.com")

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got, want := id.EndUser, "alice@example.com"; got != want {
		t.Errorf("EndUser = %q, want %q (header should propagate)", got, want)
	}
	if id.Subject == id.EndUser {
		t.Error("Subject should remain the key ID, not equal EndUser")
	}
}

func TestAPIKeyValidator_TrustEndUserHeaderDefaultsOff(t *testing.T) {
	t.Parallel()
	store := newAPIKeyStore(t)
	v := auth.NewAPIKeyValidator(store)
	r := reqWithBearer(validAPIKey)
	r.Header.Set(auth.EndUserHeader, "alice@example.com") // ignored

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
