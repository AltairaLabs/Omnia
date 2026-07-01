/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/serviceauth"
)

// sessionEraseClientRequest is the JSON body for the session-api delete-by-user
// endpoint. Field tags MUST match the server-side sessionEraseRequest.
type sessionEraseClientRequest struct {
	VirtualUserID string     `json:"virtual_user_id"`
	Workspace     string     `json:"workspace,omitempty"`
	DateFrom      *time.Time `json:"date_from,omitempty"`
	DateTo        *time.Time `json:"date_to,omitempty"`
}

// SessionGroupEraser calls one session-api's delete-by-user endpoint (Slice A,
// #1676) with SA-bearer auth and returns that group's erase result. privacy-api
// uses it to erase each service-group's sessions without holding warm-store or
// object-storage credentials itself.
type SessionGroupEraser struct {
	client *http.Client
	ts     *serviceauth.TokenSource
	log    logr.Logger
}

// NewSessionGroupEraser builds a SessionGroupEraser. ts may be nil (requests are
// sent unauthenticated — session-api's delete-by-user is behind SA auth, so a nil
// ts only makes sense in tests).
func NewSessionGroupEraser(ts *serviceauth.TokenSource, log logr.Logger) *SessionGroupEraser {
	return &SessionGroupEraser{
		client: &http.Client{Timeout: 60 * time.Second},
		ts:     ts,
		log:    log.WithName("session-group-eraser"),
	}
}

// Erase POSTs the scope to sessionURL's delete-by-user endpoint and returns the
// decoded result. A non-2xx response or transport/decode failure is an error.
func (e *SessionGroupEraser) Erase(ctx context.Context, sessionURL string, scope EraseScope) (EraseResult, error) {
	body, err := json.Marshal(sessionEraseClientRequest(scope))
	if err != nil {
		return EraseResult{}, fmt.Errorf("marshal erase request: %w", err)
	}

	target := sessionURL + "/api/v1/privacy/sessions/delete-by-user"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return EraseResult{}, fmt.Errorf("build erase request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if e.ts != nil {
		if authErr := e.ts.Authorize(req); authErr != nil {
			return EraseResult{}, fmt.Errorf("set auth header: %w", authErr)
		}
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return EraseResult{}, fmt.Errorf("POST %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return EraseResult{}, fmt.Errorf("POST %s returned status %d", target, resp.StatusCode)
	}

	var result EraseResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return EraseResult{}, fmt.Errorf("decode erase response: %w", err)
	}
	return result, nil
}
