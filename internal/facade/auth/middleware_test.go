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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/altairalabs/omnia/internal/facade/auth"
	"github.com/altairalabs/omnia/pkg/policy"
)

// observingHandler is an http.Handler that records what it saw — used
// by the middleware tests to prove next.ServeHTTP was (or wasn't)
// called, and what identity was attached.
type observingHandler struct {
	called  int
	saw     *policy.AuthenticatedIdentity
	respond func(w http.ResponseWriter)
}

func (h *observingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called++
	h.saw = policy.IdentityFromContext(r.Context())
	if h.respond != nil {
		h.respond(w)
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

// stubMwValidator: programmable Validator for middleware tests. Mirrors
// chain_test.go's stubValidator but in a form the middleware package
// can use without re-importing test helpers.
type stubMwValidator struct {
	id  *policy.AuthenticatedIdentity
	err error
}

func (s *stubMwValidator) Validate(_ context.Context, _ *http.Request) (*policy.AuthenticatedIdentity, error) {
	return s.id, s.err
}

func newMwRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/", nil)
}

func TestMiddleware_EmptyChainPassesThrough(t *testing.T) {
	// Empty chain should call next without attaching identity. PR 1
	// preserves this for back-compat.
	t.Parallel()
	next := &observingHandler{}
	mw := auth.Middleware(auth.Chain{}, next)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, newMwRequest())

	if next.called != 1 {
		t.Errorf("next called %d times, want 1", next.called)
	}
	if next.saw != nil {
		t.Errorf("expected no identity attached, got %+v", next.saw)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddleware_AdmitAttachesIdentity(t *testing.T) {
	t.Parallel()
	want := &policy.AuthenticatedIdentity{
		Origin:  policy.OriginSharedToken,
		Subject: "caller-1",
	}
	chain := auth.Chain{&stubMwValidator{id: want}}
	next := &observingHandler{}
	mw := auth.Middleware(chain, next)

	mw.ServeHTTP(httptest.NewRecorder(), newMwRequest())

	if next.called != 1 {
		t.Fatalf("next called %d, want 1", next.called)
	}
	if next.saw == nil {
		t.Fatal("expected identity on downstream context")
	}
	if next.saw != want {
		t.Errorf("saw %p, want %p", next.saw, want)
	}
}

func TestMiddleware_NoCredentialFallsThrough(t *testing.T) {
	// ErrNoCredential from all validators → no identity, but next still runs.
	// This is PR 1a/c's unauthenticated-upgrade default; PR 3 flips it
	// by configuring a chain that ends with an always-reject validator.
	t.Parallel()
	chain := auth.Chain{&stubMwValidator{err: auth.ErrNoCredential}}
	next := &observingHandler{}
	mw := auth.Middleware(chain, next)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, newMwRequest())

	if next.called != 1 {
		t.Errorf("next called %d, want 1 (fall-through)", next.called)
	}
	if next.saw != nil {
		t.Errorf("expected no identity attached, got %+v", next.saw)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestMiddleware_InvalidCredentialReturns401(t *testing.T) {
	t.Parallel()
	chain := auth.Chain{&stubMwValidator{err: auth.ErrInvalidCredential}}
	next := &observingHandler{}
	mw := auth.Middleware(chain, next)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, newMwRequest())

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if next.called != 0 {
		t.Errorf("next called %d, want 0 (must short-circuit on reject)", next.called)
	}
}

func TestMiddleware_ExpiredReturns401(t *testing.T) {
	t.Parallel()
	chain := auth.Chain{&stubMwValidator{err: auth.ErrExpired}}
	next := &observingHandler{}
	mw := auth.Middleware(chain, next)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, newMwRequest())

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if next.called != 0 {
		t.Error("next must not run after ErrExpired")
	}
}

func TestMiddleware_ChainOrderAdmitsFirstMatch(t *testing.T) {
	// A shared-token chain followed by mgmt-plane: the shared-token
	// admit should arrive at next.ServeHTTP even if a later validator
	// would have ALSO admitted.
	t.Parallel()
	shared := &policy.AuthenticatedIdentity{Origin: policy.OriginSharedToken}
	mgmt := &policy.AuthenticatedIdentity{Origin: policy.OriginManagementPlane}

	chain := auth.Chain{
		&stubMwValidator{id: shared},
		&stubMwValidator{id: mgmt}, // would also admit, but shouldn't run
	}
	next := &observingHandler{}
	mw := auth.Middleware(chain, next)
	mw.ServeHTTP(httptest.NewRecorder(), newMwRequest())

	if next.saw != shared {
		t.Errorf("expected sharedToken identity, got %+v", next.saw)
	}
}

func TestMiddleware_FallThroughMultipleValidators(t *testing.T) {
	// Every validator returns ErrNoCredential; chain ends up returning
	// ErrNoCredential; middleware falls through to next.
	t.Parallel()
	chain := auth.Chain{
		&stubMwValidator{err: auth.ErrNoCredential},
		&stubMwValidator{err: auth.ErrNoCredential},
		&stubMwValidator{err: auth.ErrNoCredential},
	}
	next := &observingHandler{}
	mw := auth.Middleware(chain, next)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, newMwRequest())

	if next.called != 1 {
		t.Errorf("next called %d, want 1", next.called)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
