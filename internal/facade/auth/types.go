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

// Package auth validates credentials presented to the agent facade.
//
// The package exposes a small Validator interface and concrete validator
// implementations. The facade middleware runs the configured validators
// against every upgrade / request and, on first admit, attaches an
// AuthenticatedIdentity (defined in pkg/policy) to the request context.
//
// The identity type and its origin/role constants live in pkg/policy so
// that PropagationFields can reference them without pkg/policy gaining a
// reverse dependency on internal/facade/auth.
package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/altairalabs/omnia/pkg/policy"
)

// Validator validates a single credential style. Implementations inspect
// the request and return an AuthenticatedIdentity on admit, or one of the
// typed errors below on reject / absence.
type Validator interface {
	// Validate returns an AuthenticatedIdentity when the request carries a
	// credential this validator admits. It returns ErrNoCredential when no
	// credential of this validator's style is present (the chain falls
	// through to the next validator). Any other error rejects the request
	// — the chain runner translates it to 401.
	Validate(ctx context.Context, r *http.Request) (*policy.AuthenticatedIdentity, error)
}

// Typed errors returned by validators. Chain logic inspects these.
var (
	// ErrNoCredential signals the request carries no credential of this
	// validator's style. The chain runner falls through to the next
	// validator on this error.
	ErrNoCredential = errors.New("auth: no credential present")

	// ErrInvalidCredential signals a credential of this validator's style
	// was presented but failed validation (bad signature, wrong issuer,
	// unknown key id, malformed payload, etc.). The chain runner rejects
	// the request with 401 on this error.
	ErrInvalidCredential = errors.New("auth: invalid credential")

	// ErrExpired signals the presented credential parsed and verified but
	// has expired. The chain runner rejects with 401.
	ErrExpired = errors.New("auth: credential expired")
)
