/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strconv"
	"time"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// Default HTTP client timeout for session-api requests.
const defaultHTTPTimeout = 10 * time.Second

// SessionAPIClient is the interface for communicating with the session-api service.
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

// HTTPSessionAPIClient implements SessionAPIClient using HTTP calls to session-api.
type HTTPSessionAPIClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPSessionAPIClient creates a new HTTP client for session-api.
func NewHTTPSessionAPIClient(baseURL string) *HTTPSessionAPIClient {
	return &HTTPSessionAPIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

// doGet performs a GET request to the given URL and decodes the JSON response into dest.
func (c *HTTPSessionAPIClient) doGet(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s returned status %d", url, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode response from %s: %w", url, err)
	}

	return nil
}

// GetSession retrieves session metadata by ID from the session-api.
func (c *HTTPSessionAPIClient) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s", c.baseURL, sessionID)

	var result sessionResponse
	if err := c.doGet(ctx, url, &result); err != nil {
		return nil, err
	}

	return result.Session, nil
}

// GetSessionMessages retrieves all messages for a session from the session-api.
func (c *HTTPSessionAPIClient) GetSessionMessages(ctx context.Context, sessionID string) ([]session.Message, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/messages", c.baseURL, sessionID)

	var result messagesResponse
	if err := c.doGet(ctx, url, &result); err != nil {
		return nil, err
	}

	// Convert pointer slice to value slice.
	messages := make([]session.Message, 0, len(result.Messages))
	for _, m := range result.Messages {
		if m != nil {
			messages = append(messages, *m)
		}
	}

	return messages, nil
}

// WriteEvalResults sends eval results to the session-api for persistence.
func (c *HTTPSessionAPIClient) WriteEvalResults(ctx context.Context, results []*api.EvalResult) error {
	url := fmt.Sprintf("%s/api/v1/eval-results", c.baseURL)

	body, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("marshal eval results: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("POST %s returned status %d", url, resp.StatusCode)
	}

	return nil
}

// ListEvalResults retrieves eval results matching the given filters from the session-api.
func (c *HTTPSessionAPIClient) ListEvalResults(ctx context.Context, opts api.EvalResultListOpts) ([]*api.EvalResult, error) {
	url := fmt.Sprintf("%s/api/v1/eval-results?%s", c.baseURL, encodeEvalListQuery(opts))

	var result api.EvalResultListResponse
	if err := c.doGet(ctx, url, &result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// GetSessionEvalResults retrieves eval results for a specific session from the session-api.
func (c *HTTPSessionAPIClient) GetSessionEvalResults(ctx context.Context, sessionID string) ([]*api.EvalResult, error) {
	url := fmt.Sprintf("%s/api/v1/sessions/%s/eval-results", c.baseURL, sessionID)

	var result api.EvalResultSessionResponse
	if err := c.doGet(ctx, url, &result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// encodeEvalListQuery builds query parameters for the eval results list endpoint.
func encodeEvalListQuery(opts api.EvalResultListOpts) string {
	q := make(neturl.Values)
	if opts.Passed != nil {
		q.Set("passed", strconv.FormatBool(*opts.Passed))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Offset > 0 {
		q.Set("offset", strconv.Itoa(opts.Offset))
	}
	if opts.AgentName != "" {
		q.Set("agent_name", opts.AgentName)
	}
	if opts.Namespace != "" {
		q.Set("namespace", opts.Namespace)
	}
	if opts.EvalID != "" {
		q.Set("eval_id", opts.EvalID)
	}
	return q.Encode()
}

// sessionResponse mirrors the session-api GET /sessions/{id} JSON response.
type sessionResponse struct {
	Session  *session.Session  `json:"session"`
	Messages []session.Message `json:"messages,omitempty"`
}

// messagesResponse mirrors the session-api GET /sessions/{id}/messages JSON response.
type messagesResponse struct {
	Messages []*session.Message `json:"messages"`
	HasMore  bool               `json:"hasMore"`
}
