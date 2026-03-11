/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package a2a provides Omnia-specific implementations of the PromptKit A2A
// server interfaces (Authenticator, AgentCardProvider) backed by Kubernetes
// resources (Secrets, CRDs).
package a2a

import (
	"fmt"
	"net/http"
	"strings"
)

// BearerAuthenticator validates incoming A2A requests against a static bearer
// token loaded from a Kubernetes Secret. It implements a2aserver.Authenticator.
type BearerAuthenticator struct {
	token string
}

// NewBearerAuthenticator creates an authenticator that validates the
// Authorization header against the given token.
func NewBearerAuthenticator(token string) *BearerAuthenticator {
	return &BearerAuthenticator{token: token}
}

// Authenticate checks the Authorization header for a valid bearer token.
func (a *BearerAuthenticator) Authenticate(r *http.Request) error {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return fmt.Errorf("missing Authorization header")
	}

	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return fmt.Errorf("invalid Authorization scheme, expected Bearer")
	}

	if strings.TrimSpace(auth[len(prefix):]) != a.token {
		return fmt.Errorf("invalid bearer token")
	}

	return nil
}

// NoOpAuthenticator allows all requests without authentication.
type NoOpAuthenticator struct{}

// Authenticate always returns nil (no authentication required).
func (NoOpAuthenticator) Authenticate(*http.Request) error { return nil }
