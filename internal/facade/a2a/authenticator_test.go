/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBearerAuthenticator_ValidToken(t *testing.T) {
	auth := NewBearerAuthenticator("secret-token")
	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)
	req.Header.Set("Authorization", "Bearer secret-token")

	err := auth.Authenticate(req)
	require.NoError(t, err)
}

func TestBearerAuthenticator_InvalidToken(t *testing.T) {
	auth := NewBearerAuthenticator("secret-token")
	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")

	err := auth.Authenticate(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid bearer token")
}

func TestBearerAuthenticator_MissingHeader(t *testing.T) {
	auth := NewBearerAuthenticator("secret-token")
	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)

	err := auth.Authenticate(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing Authorization header")
}

func TestBearerAuthenticator_WrongScheme(t *testing.T) {
	auth := NewBearerAuthenticator("secret-token")
	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	err := auth.Authenticate(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid Authorization scheme")
}

func TestBearerAuthenticator_TokenWithWhitespace(t *testing.T) {
	auth := NewBearerAuthenticator("secret-token")
	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)
	req.Header.Set("Authorization", "Bearer  secret-token ")

	err := auth.Authenticate(req)
	require.NoError(t, err)
}

func TestNoOpAuthenticator(t *testing.T) {
	auth := NoOpAuthenticator{}
	req := httptest.NewRequest(http.MethodPost, "/a2a", nil)

	err := auth.Authenticate(req)
	require.NoError(t, err)
}
