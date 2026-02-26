/*
Copyright 2025.

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

// Package httpclient provides a session.Store implementation backed by HTTP
// calls to the session-api service.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
)

// ErrNotImplemented is returned for Store methods not needed by the facade.
var ErrNotImplemented = errors.New("not implemented by HTTP session client")

// Default timeout for HTTP requests to the session-api.
const DefaultHTTPTimeout = 30 * time.Second

// StoreOption is a functional option for configuring the HTTP session store.
type StoreOption func(*Store)

// WithHTTPTimeout sets the timeout for HTTP requests. Defaults to 30s.
func WithHTTPTimeout(d time.Duration) StoreOption {
	return func(s *Store) {
		s.httpClient.Timeout = d
	}
}

// WithHTTPClient sets a custom HTTP client. When set, WithHTTPTimeout is ignored.
func WithHTTPClient(c *http.Client) StoreOption {
	return func(s *Store) {
		s.httpClient = c
	}
}

// Store implements session.Store by calling the session-api over HTTP.
// It is used by the facade's recordingResponseWriter for session persistence.
type Store struct {
	baseURL    string
	httpClient *http.Client
	log        logr.Logger
}

// NewStore creates a new HTTP-backed session store.
func NewStore(baseURL string, log logr.Logger, opts ...StoreOption) *Store {
	s := &Store{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
		log: log.WithName("session-httpclient"),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// createSessionRequest mirrors the session-api CreateSessionRequest.
type createSessionRequest struct {
	ID            string `json:"id"`
	AgentName     string `json:"agentName"`
	Namespace     string `json:"namespace"`
	WorkspaceName string `json:"workspaceName,omitempty"`
	TTLSeconds    int    `json:"ttlSeconds,omitempty"`
}

// sessionResponse mirrors the session-api SessionResponse.
type sessionResponse struct {
	Session *session.Session `json:"session"`
}

// refreshTTLRequest mirrors the session-api RefreshTTLRequest.
type refreshTTLRequest struct {
	TTLSeconds int `json:"ttlSeconds"`
}

// CreateSession creates a new session via POST /api/v1/sessions.
func (s *Store) CreateSession(ctx context.Context, opts session.CreateSessionOptions) (*session.Session, error) {
	id := uuid.New().String()
	reqBody := createSessionRequest{
		ID:            id,
		AgentName:     opts.AgentName,
		Namespace:     opts.Namespace,
		WorkspaceName: opts.WorkspaceName,
	}
	if opts.TTL > 0 {
		reqBody.TTLSeconds = int(opts.TTL.Seconds())
	}

	resp, err := s.doJSON(ctx, http.MethodPost, "/api/v1/sessions", reqBody)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return nil, s.readError(resp)
	}

	var sr sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode create session response: %w", err)
	}
	return sr.Session, nil
}

// GetSession retrieves a session via GET /api/v1/sessions/{sessionID}.
func (s *Store) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	resp, err := s.doRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v1/sessions/%s", sessionID), nil)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, session.ErrSessionNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, s.readError(resp)
	}

	var sr sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode get session response: %w", err)
	}
	return sr.Session, nil
}

// AppendMessage appends a message via POST /api/v1/sessions/{sessionID}/messages.
func (s *Store) AppendMessage(ctx context.Context, sessionID string, msg session.Message) error {
	resp, err := s.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/sessions/%s/messages", sessionID), &msg)
	if err != nil {
		return fmt.Errorf("append message: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return session.ErrSessionNotFound
	}
	if resp.StatusCode != http.StatusCreated {
		return s.readError(resp)
	}
	return nil
}

// UpdateSessionStats sends incremental updates via PATCH /api/v1/sessions/{sessionID}/stats.
func (s *Store) UpdateSessionStats(ctx context.Context, sessionID string, update session.SessionStatsUpdate) error {
	resp, err := s.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/sessions/%s/stats", sessionID), &update)
	if err != nil {
		return fmt.Errorf("update session stats: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return session.ErrSessionNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return s.readError(resp)
	}
	return nil
}

// RefreshTTL extends session expiry via POST /api/v1/sessions/{sessionID}/ttl.
func (s *Store) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	reqBody := refreshTTLRequest{TTLSeconds: int(ttl.Seconds())}
	resp, err := s.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/sessions/%s/ttl", sessionID), reqBody)
	if err != nil {
		return fmt.Errorf("refresh TTL: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return session.ErrSessionNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return s.readError(resp)
	}
	return nil
}

// Close is a no-op for the HTTP client store.
func (s *Store) Close() error { return nil }

// --- Methods not used by the facade — return ErrNotImplemented ---

// DeleteSession is not used by the facade.
func (s *Store) DeleteSession(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// GetMessages is not used by the facade.
func (s *Store) GetMessages(_ context.Context, _ string) ([]session.Message, error) {
	return nil, ErrNotImplemented
}

// SetState is not used by the facade.
func (s *Store) SetState(_ context.Context, _, _, _ string) error {
	return ErrNotImplemented
}

// GetState is not used by the facade.
func (s *Store) GetState(_ context.Context, _, _ string) (string, error) {
	return "", ErrNotImplemented
}

// --- HTTP helpers ---

func (s *Store) doJSON(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, fmt.Errorf("encode request body: %w", err)
	}
	return s.doRequest(ctx, method, path, &buf)
}

func (s *Store) doRequest(ctx context.Context, method, path string, body *bytes.Buffer) (*http.Response, error) {
	url := s.baseURL + path
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return s.httpClient.Do(req)
}

// errorResponse mirrors the session-api ErrorResponse.
type errorResponse struct {
	Error string `json:"error"`
}

func (s *Store) readError(resp *http.Response) error {
	var errResp errorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Error)
}

// Interface assertion.
var _ session.Store = (*Store)(nil)
