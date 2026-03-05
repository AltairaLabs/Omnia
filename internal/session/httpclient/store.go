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
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/sony/gobreaker/v2"

	"github.com/altairalabs/omnia/internal/session"
)

// ErrNotImplemented is returned for Store methods not needed by the facade.
var ErrNotImplemented = errors.New("not implemented by HTTP session client")

// Default timeout for HTTP requests to the session-api.
const DefaultHTTPTimeout = 30 * time.Second

// Retry configuration for transient failures.
const (
	maxRetries    = 3
	retryBaseWait = 100 * time.Millisecond
)

// Circuit breaker defaults.
const (
	cbMaxRequests = 5                // requests allowed in half-open state
	cbInterval    = 30 * time.Second // counters reset interval in closed state
	cbTimeout     = 10 * time.Second // time to wait before probing after open
	cbMinRequests = 10               // minimum requests before tripping
	cbFailRatio   = 0.6              // failure ratio to trip the breaker
)

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
	cb         *gobreaker.CircuitBreaker[*http.Response]
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
	s.cb = gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
		Name:        "session-api",
		MaxRequests: cbMaxRequests,
		Interval:    cbInterval,
		Timeout:     cbTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.Requests >= cbMinRequests &&
				float64(counts.TotalFailures)/float64(counts.Requests) >= cbFailRatio
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			s.log.Info("circuit breaker state change",
				"name", name, "from", from.String(), "to", to.String())
		},
	})
	return s
}

// createSessionRequest mirrors the session-api CreateSessionRequest.
type createSessionRequest struct {
	ID                string `json:"id"`
	AgentName         string `json:"agentName"`
	Namespace         string `json:"namespace"`
	WorkspaceName     string `json:"workspaceName,omitempty"`
	TTLSeconds        int    `json:"ttlSeconds,omitempty"`
	PromptPackName    string `json:"promptPackName,omitempty"`
	PromptPackVersion string `json:"promptPackVersion,omitempty"`
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
		ID:                id,
		AgentName:         opts.AgentName,
		Namespace:         opts.Namespace,
		WorkspaceName:     opts.WorkspaceName,
		PromptPackName:    opts.PromptPackName,
		PromptPackVersion: opts.PromptPackVersion,
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
	resp, err := s.doWithRetry(ctx, http.MethodGet, fmt.Sprintf("/api/v1/sessions/%s", sessionID), nil)
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
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request body: %w", err)
	}
	return s.doWithRetry(ctx, method, path, data)
}

// doWithRetry executes an HTTP request with retry for transient failures,
// wrapped in a circuit breaker. When the breaker is open, requests fail
// immediately without hitting session-api.
func (s *Store) doWithRetry(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	return s.cb.Execute(func() (*http.Response, error) {
		return s.doWithRetryInner(ctx, method, path, body)
	})
}

// doWithRetryInner contains the retry loop. Called within the circuit breaker.
func (s *Store) doWithRetryInner(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := range maxRetries {
		if attempt > 0 {
			wait := retryBaseWait << uint(attempt-1) // 100ms, 200ms, 400ms
			s.log.V(1).Info("retrying request",
				"method", method, "path", path,
				"attempt", attempt+1, "backoff", wait.String())
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		resp, err := s.doRequest(ctx, method, path, body)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, err
			}
			continue
		}
		if isRetryableStatus(resp.StatusCode) {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			_ = drainAndClose(resp.Body)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}

func (s *Store) doRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	url := s.baseURL + path
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	s.log.V(2).Info("session-api request",
		"method", method, "path", path)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.log.V(1).Info("session-api request failed",
			"method", method, "path", path, "error", err.Error())
		return nil, err
	}

	s.log.V(2).Info("session-api response",
		"method", method, "path", path, "status", resp.StatusCode)
	return resp, nil
}

// isRetryableStatus returns true for HTTP status codes that indicate a transient server issue.
func isRetryableStatus(code int) bool {
	return code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
}

// drainAndClose reads remaining body bytes and closes it.
func drainAndClose(body io.ReadCloser) error {
	_, _ = io.Copy(io.Discard, body)
	return body.Close()
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
