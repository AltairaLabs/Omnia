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

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

const validToken = "supersecret-bearer-token"

func newRequestWithBearer(token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func TestNewSharedTokenValidator_RejectsEmptyToken(t *testing.T) {
	t.Parallel()
	// An empty configured token would constant-time-equal an empty bearer
	// presented by an attacker, silently always-admitting. Refuse to
	// construct rather than allow this footgun.
	if _, err := auth.NewSharedTokenValidator(""); err == nil {
		t.Error("expected error from NewSharedTokenValidator(\"\")")
	}
}

func TestSharedTokenValidator_AdmitsCorrectToken(t *testing.T) {
	t.Parallel()
	v, err := auth.NewSharedTokenValidator(validToken)
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	id, err := v.Validate(context.Background(), newRequestWithBearer(validToken))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if id == nil {
		t.Fatal("nil identity on admit")
	}
	if got, want := id.Origin, policy.OriginSharedToken; got != want {
		t.Errorf("Origin = %q, want %q", got, want)
	}
	if got, want := id.Role, policy.RoleEditor; got != want {
		t.Errorf("Role = %q, want %q (default)", got, want)
	}
	if got, want := id.Subject, auth.DefaultSharedTokenSubject; got != want {
		t.Errorf("Subject = %q, want %q (default)", got, want)
	}
	if got, want := id.EndUser, id.Subject; got != want {
		t.Errorf("EndUser = %q, want %q (no trustEndUserHeader → equals Subject)", got, want)
	}
}

func TestSharedTokenValidator_RejectsWrongToken(t *testing.T) {
	t.Parallel()
	v, _ := auth.NewSharedTokenValidator(validToken)
	_, err := v.Validate(context.Background(), newRequestWithBearer("other-token"))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestSharedTokenValidator_RejectsLengthMismatch(t *testing.T) {
	t.Parallel()
	v, _ := auth.NewSharedTokenValidator(validToken)
	// Different length should also be rejected without leaking which
	// candidate is longer.
	_, err := v.Validate(context.Background(), newRequestWithBearer("short"))
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
}

func TestSharedTokenValidator_NoBearerHeaderFallsThrough(t *testing.T) {
	t.Parallel()
	v, _ := auth.NewSharedTokenValidator(validToken)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil) // no Authorization
	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential (chain must fall through)", err)
	}
}

func TestSharedTokenValidator_NonBearerSchemeFallsThrough(t *testing.T) {
	t.Parallel()
	v, _ := auth.NewSharedTokenValidator(validToken)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("non-Bearer scheme: err = %v, want ErrNoCredential", err)
	}
}

func TestSharedTokenValidator_EmptyBearerRejected(t *testing.T) {
	t.Parallel()
	v, _ := auth.NewSharedTokenValidator(validToken)
	r := httptest.NewRequest(http.MethodGet, "/ws", nil)
	r.Header.Set("Authorization", "Bearer ")
	_, err := v.Validate(context.Background(), r)
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("empty bearer: err = %v, want ErrInvalidCredential", err)
	}
}

func TestSharedTokenValidator_TrustEndUserHeader(t *testing.T) {
	t.Parallel()
	v, _ := auth.NewSharedTokenValidator(
		validToken,
		auth.WithSharedTokenTrustEndUserHeader(true),
	)
	r := newRequestWithBearer(validToken)
	r.Header.Set(auth.EndUserHeader, "alice@example.com")

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.EndUser, "alice@example.com"; got != want {
		t.Errorf("EndUser = %q, want %q", got, want)
	}
	// Subject MUST stay the token-holder identifier — only EndUser shifts.
	if id.Subject == id.EndUser {
		t.Error("Subject should remain the token-holder when trustEndUserHeader is on, not equal EndUser")
	}
}

func TestSharedTokenValidator_TrustEndUserHeaderDefaultsOff(t *testing.T) {
	t.Parallel()
	v, _ := auth.NewSharedTokenValidator(validToken)
	r := newRequestWithBearer(validToken)
	r.Header.Set(auth.EndUserHeader, "alice@example.com") // ignored

	id, err := v.Validate(context.Background(), r)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.EndUser, id.Subject; got != want {
		t.Errorf("EndUser = %q, want %q (header ignored when trust flag off)", got, want)
	}
}

func TestSharedTokenValidator_OptionOverrides(t *testing.T) {
	t.Parallel()
	v, _ := auth.NewSharedTokenValidator(
		validToken,
		auth.WithSharedTokenSubject("custom-sub"),
		auth.WithSharedTokenRole(policy.RoleAdmin),
	)
	id, err := v.Validate(context.Background(), newRequestWithBearer(validToken))
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got, want := id.Subject, "custom-sub"; got != want {
		t.Errorf("Subject override: got %q, want %q", got, want)
	}
	if got, want := id.Role, policy.RoleAdmin; got != want {
		t.Errorf("Role override: got %q, want %q", got, want)
	}
}
