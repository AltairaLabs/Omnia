/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/pkg/sessionapi"
)

// Default HTTP client timeout for session-api requests.
const defaultHTTPTimeout = 10 * time.Second

// SessionAPIClient is the interface for communicating with the session-api service.
// It provides both read and write operations for sessions, messages, and eval results.
// The eval worker uses the narrower EvalResultWriter for writes only.
type SessionAPIClient interface {
	// GetSession retrieves session metadata by ID.
	GetSession(ctx context.Context, sessionID string) (*session.Session, error)
	// GetSessionMessages retrieves all messages for a session.
	GetSessionMessages(ctx context.Context, sessionID string) ([]session.Message, error)
	// WriteEvalResults persists eval results via the session-api.
	WriteEvalResults(ctx context.Context, results []*api.EvalResult) error
	// ListEvalResults retrieves eval results matching the given filters.
	ListEvalResults(ctx context.Context, opts api.EvalResultListOpts) ([]*api.EvalResult, error)
	// GetSessionEvalResults retrieves eval results for a specific session.
	GetSessionEvalResults(ctx context.Context, sessionID string) ([]*api.EvalResult, error)
}

// EvalResultWriter is the subset of SessionAPIClient needed by the eval worker
// for persisting eval results. Session/message reads use the Redis hot tier.
type EvalResultWriter interface {
	WriteEvalResults(ctx context.Context, results []*api.EvalResult) error
}

// HTTPSessionAPIClient implements SessionAPIClient using the generated session-api client.
type HTTPSessionAPIClient struct {
	client *sessionapi.ClientWithResponses
}

// NewHTTPSessionAPIClient creates a new HTTP client for session-api.
// The client's transport is wrapped with otelhttp to propagate trace context.
func NewHTTPSessionAPIClient(baseURL string) (*HTTPSessionAPIClient, error) {
	httpClient := &http.Client{
		Timeout:   defaultHTTPTimeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
	client, err := sessionapi.NewClientWithResponses(baseURL,
		sessionapi.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("create session-api client: %w", err)
	}
	return &HTTPSessionAPIClient{client: client}, nil
}

// GetSession retrieves session metadata by ID from the session-api.
func (c *HTTPSessionAPIClient) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	id, err := parseSessionID(sessionID)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.GetSessionWithResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get session %s: %w", sessionID, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get session %s: status %d", sessionID, resp.StatusCode())
	}
	if resp.JSON200 == nil || resp.JSON200.Session == nil {
		return nil, fmt.Errorf("get session %s: empty response", sessionID)
	}

	return sessionapi.SessionFromAPI(resp.JSON200.Session), nil
}

// GetSessionMessages retrieves all messages for a session from the session-api.
func (c *HTTPSessionAPIClient) GetSessionMessages(ctx context.Context, sessionID string) ([]session.Message, error) {
	id, err := parseSessionID(sessionID)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.GetMessagesWithResponse(ctx, id, nil)
	if err != nil {
		return nil, fmt.Errorf("get messages for %s: %w", sessionID, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get messages for %s: status %d", sessionID, resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("get messages for %s: empty response", sessionID)
	}

	return sessionapi.MessagesFromAPI(resp.JSON200.Messages), nil
}

// WriteEvalResults sends eval results to the session-api for persistence.
func (c *HTTPSessionAPIClient) WriteEvalResults(ctx context.Context, results []*api.EvalResult) error {
	body := sessionapi.EvalResultsToAPI(results)

	resp, err := c.client.CreateEvalResultsWithResponse(ctx, body)
	if err != nil {
		return fmt.Errorf("write eval results: %w", err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return fmt.Errorf("write eval results: status %d", resp.StatusCode())
	}

	return nil
}

// ListEvalResults retrieves eval results matching the given filters from the session-api.
func (c *HTTPSessionAPIClient) ListEvalResults(ctx context.Context, opts api.EvalResultListOpts) ([]*api.EvalResult, error) {
	params := sessionapi.EvalListOptsToParams(opts)

	resp, err := c.client.ListEvalResultsWithResponse(ctx, &params)
	if err != nil {
		return nil, fmt.Errorf("list eval results: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("list eval results: status %d", resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("list eval results: empty response")
	}

	return sessionapi.EvalResultsFromAPI(resp.JSON200.Results), nil
}

// GetSessionEvalResults retrieves eval results for a specific session from the session-api.
func (c *HTTPSessionAPIClient) GetSessionEvalResults(ctx context.Context, sessionID string) ([]*api.EvalResult, error) {
	id, err := parseSessionID(sessionID)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.GetSessionEvalResultsWithResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get session eval results for %s: %w", sessionID, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("get session eval results for %s: status %d", sessionID, resp.StatusCode())
	}
	if resp.JSON200 == nil {
		return nil, fmt.Errorf("get session eval results for %s: empty response", sessionID)
	}

	return sessionapi.EvalResultsFromAPI(resp.JSON200.Results), nil
}

// parseSessionID parses a string session ID into the UUID type expected by the generated client.
func parseSessionID(sessionID string) (sessionapi.SessionID, error) {
	var id sessionapi.SessionID
	if err := id.UnmarshalText([]byte(sessionID)); err != nil {
		return id, fmt.Errorf("invalid session ID %q: %w", sessionID, err)
	}
	return id, nil
}
