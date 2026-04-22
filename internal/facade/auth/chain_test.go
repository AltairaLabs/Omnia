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

// stubValidator is a programmable Validator for chain tests. Each test
// composes a chain by listing what each step should return.
type stubValidator struct {
	id   *policy.AuthenticatedIdentity
	err  error
	name string
	// called records how many times Validate was invoked — lets us prove
	// short-circuit semantics (no validator runs after a non-fall-through
	// result).
	called int
}

func (s *stubValidator) Validate(_ context.Context, _ *http.Request) (*policy.AuthenticatedIdentity, error) {
	s.called++
	return s.id, s.err
}

func newReq() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/ws", nil)
}

func TestChainRun_EmptyChainReturnsErrNoCredential(t *testing.T) {
	t.Parallel()
	chain := auth.Chain{}
	_, err := chain.Run(context.Background(), newReq())
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestChainRun_FirstValidatorAdmits(t *testing.T) {
	t.Parallel()
	wantID := &policy.AuthenticatedIdentity{Origin: policy.OriginSharedToken}
	first := &stubValidator{name: "shared", id: wantID}
	second := &stubValidator{name: "mgmt", err: auth.ErrNoCredential}

	chain := auth.Chain{first, second}
	id, err := chain.Run(context.Background(), newReq())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if id != wantID {
		t.Errorf("identity pointer mismatch: got %p, want %p", id, wantID)
	}
	if second.called != 0 {
		t.Errorf("second validator should not run after first admits, called=%d", second.called)
	}
}

func TestChainRun_FallsThroughOnNoCredential(t *testing.T) {
	t.Parallel()
	wantID := &policy.AuthenticatedIdentity{Origin: policy.OriginManagementPlane}
	first := &stubValidator{name: "shared", err: auth.ErrNoCredential}
	second := &stubValidator{name: "mgmt", id: wantID}

	chain := auth.Chain{first, second}
	id, err := chain.Run(context.Background(), newReq())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if id != wantID {
		t.Errorf("expected fall-through to second validator, got %v", id)
	}
	if first.called != 1 || second.called != 1 {
		t.Errorf("call counts: first=%d second=%d, want 1 1", first.called, second.called)
	}
}

func TestChainRun_AllFallThroughReturnsErrNoCredential(t *testing.T) {
	t.Parallel()
	first := &stubValidator{err: auth.ErrNoCredential}
	second := &stubValidator{err: auth.ErrNoCredential}

	chain := auth.Chain{first, second}
	_, err := chain.Run(context.Background(), newReq())
	if !errors.Is(err, auth.ErrNoCredential) {
		t.Errorf("err = %v, want ErrNoCredential", err)
	}
}

func TestChainRun_InvalidCredentialShortCircuits(t *testing.T) {
	// A validator returning ErrInvalidCredential means "I recognise this
	// credential style and reject it" — falling through to a downstream
	// validator that might admit the same request would be a security
	// hole (an OIDC token rejected for wrong issuer can't suddenly admit
	// via shared-token comparison).
	t.Parallel()
	first := &stubValidator{err: auth.ErrInvalidCredential}
	second := &stubValidator{id: &policy.AuthenticatedIdentity{Origin: "would-admit-incorrectly"}}

	chain := auth.Chain{first, second}
	_, err := chain.Run(context.Background(), newReq())
	if !errors.Is(err, auth.ErrInvalidCredential) {
		t.Errorf("err = %v, want ErrInvalidCredential", err)
	}
	if second.called != 0 {
		t.Error("second validator must not run after ErrInvalidCredential")
	}
}

func TestChainRun_ExpiredShortCircuits(t *testing.T) {
	t.Parallel()
	first := &stubValidator{err: auth.ErrExpired}
	second := &stubValidator{id: &policy.AuthenticatedIdentity{Origin: "later-validator"}}

	chain := auth.Chain{first, second}
	_, err := chain.Run(context.Background(), newReq())
	if !errors.Is(err, auth.ErrExpired) {
		t.Errorf("err = %v, want ErrExpired", err)
	}
	if second.called != 0 {
		t.Error("second validator must not run after ErrExpired")
	}
}

func TestChainRun_UnknownErrorShortCircuits(t *testing.T) {
	// An unexpected runtime error (e.g., k8s informer not yet primed)
	// shouldn't accidentally admit via a downstream validator either.
	t.Parallel()
	custom := errors.New("informer not synced yet")
	first := &stubValidator{err: custom}
	second := &stubValidator{id: &policy.AuthenticatedIdentity{Origin: "later-validator"}}

	chain := auth.Chain{first, second}
	_, err := chain.Run(context.Background(), newReq())
	if !errors.Is(err, custom) {
		t.Errorf("err = %v, want %v", err, custom)
	}
	if second.called != 0 {
		t.Error("second validator must not run after unknown error")
	}
}

func TestChainRun_NilIdentityNilErrTreatedAsNoCredential(t *testing.T) {
	// Defensive: a buggy validator returning (nil, nil) should not crash
	// or short-circuit — log-and-continue is the safer default.
	t.Parallel()
	wantID := &policy.AuthenticatedIdentity{Origin: policy.OriginSharedToken}
	first := &stubValidator{} // nil id, nil err
	second := &stubValidator{id: wantID}

	chain := auth.Chain{first, second}
	id, err := chain.Run(context.Background(), newReq())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if id != wantID {
		t.Errorf("expected fall-through past buggy validator")
	}
}
