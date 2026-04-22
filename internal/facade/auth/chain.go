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

package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/altairalabs/omnia/pkg/policy"
)

// Chain runs an ordered slice of Validators against each request and
// returns the first one that admits.
//
// Order matters because the data-plane validators each look at the
// Authorization header in slightly different ways and a token meant for
// one validator can syntactically resemble another (e.g., a JWT also
// looks like an opaque bearer to sharedToken). The conventional order
// shipped by cmd/agent is:
//
//	sharedToken → apiKeys → oidc → edgeTrust → mgmtPlane
//
// — narrowest match first, broadest last. ErrInvalidCredential from any
// validator short-circuits the chain (a credential of that style was
// presented and rejected; we don't fall through to a different style),
// while ErrNoCredential moves to the next validator.
//
// An empty chain admits nothing — Run returns ErrNoCredential so the
// caller can decide whether that means "fall through to no-auth path"
// (PR 1a/c default) or "401" (PR 3 default).
type Chain []Validator

// Run walks the chain in order. The semantic contract:
//
//   - First validator returning a non-nil identity wins. Identity is returned.
//   - ErrNoCredential continues to the next validator.
//   - Any other error short-circuits — no later validator runs, the error
//     is returned to the caller (which translates to 401 at the HTTP layer).
//   - Empty chain or all-fall-through returns ErrNoCredential.
//
// The runner does not log — that is the caller's job, where the request
// scope lives and the structured logger is bound. Returning typed
// sentinels lets the caller decide whether to surface the underlying
// reason or aggregate.
func (c Chain) Run(ctx context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error) {
	for _, v := range c {
		id, err := v.Validate(ctx, r)
		switch {
		case err == nil && id != nil:
			return id, nil
		case errors.Is(err, ErrNoCredential):
			continue
		case err != nil:
			// Either ErrInvalidCredential, ErrExpired, or an unexpected
			// runtime error from the validator. Any of those means a
			// credential of *some* style was presented and we've decided
			// to reject it — short-circuit so a different validator
			// can't accidentally admit the same request.
			return nil, err
		default:
			// nil identity + nil error is a buggy validator. Treat it
			// as "no credential here" rather than panicking — log-and-
			// continue is more defensible than crashing on a bad plug-in.
			continue
		}
	}
	return nil, ErrNoCredential
}
