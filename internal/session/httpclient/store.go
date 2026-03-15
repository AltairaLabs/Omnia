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
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/sessionapi"
)

// ErrNotImplemented is returned for Store methods not needed by the facade.
var ErrNotImplemented = errors.New("not implemented by HTTP session client")

// Default timeout for HTTP requests to the session-api.
const DefaultHTTPTimeout = 30 * time.Second

// healthCheckTimeout is used for the uninstrumented Ping client.
const healthCheckTimeout = 5 * time.Second

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

// Write buffer defaults.
const (
	defaultBufferCapacity      = 1000            // max queued writes
	defaultBufferFlushInterval = 5 * time.Second // how often to try flushing
	defaultBufferMaxAge        = 5 * time.Minute // max age before dropping a buffered item
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

// WithBufferCapacity sets the write buffer capacity. Set to 0 to disable
// buffering entirely (failed writes return errors immediately). Defaults to 1000.
func WithBufferCapacity(n int) StoreOption {
	return func(s *Store) {
		s.bufCapacity = n
	}
}

// WithBufferFlushInterval sets how often the background goroutine attempts to
// flush buffered writes. Defaults to 5s.
func WithBufferFlushInterval(d time.Duration) StoreOption {
	return func(s *Store) {
		s.flushInterval = d
	}
}

// WithBufferMaxAge sets the maximum age of a buffered write before it is
// dropped without retrying. Defaults to 5m.
func WithBufferMaxAge(d time.Duration) StoreOption {
	return func(s *Store) {
		s.bufferMaxAge = d
	}
}

// Store implements session.Store by calling the session-api over HTTP.
// It is used by the facade's recordingResponseWriter for session persistence.
//
// Write operations (AppendMessage, UpdateSessionStats, RefreshTTL) are buffered
// on transient failure and retried automatically when session-api recovers.
type Store struct {
	baseURL      string
	httpClient   *http.Client
	healthClient *http.Client // uninstrumented client for Ping — avoids trace noise from K8s probes
	cb           *gobreaker.CircuitBreaker[*http.Response]
	log          logr.Logger

	// Write buffer for transient failures.
	buf           *writeBuffer
	bufCapacity   int
	flushInterval time.Duration
	bufferMaxAge  time.Duration
	flushNotify   chan struct{}
	stopCh        chan struct{}
	flushStopped  chan struct{}
}

// NewStore creates a new HTTP-backed session store.
func NewStore(baseURL string, log logr.Logger, opts ...StoreOption) *Store {
	s := &Store{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
		log:           log.WithName("session-httpclient"),
		bufCapacity:   defaultBufferCapacity,
		flushInterval: defaultBufferFlushInterval,
		bufferMaxAge:  defaultBufferMaxAge,
		flushNotify:   make(chan struct{}, 1),
	}
	for _, opt := range opts {
		opt(s)
	}
	// Keep an uninstrumented copy for health checks before wrapping with OTel.
	// K8s readiness probes call Ping() every 10s — tracing those creates noise.
	s.healthClient = &http.Client{
		Timeout:   healthCheckTimeout,
		Transport: s.httpClient.Transport,
	}
	// Wrap the HTTP transport for OTel trace context propagation.
	// This injects traceparent into outbound requests to session-api.
	s.httpClient.Transport = otelhttp.NewTransport(s.httpClient.Transport)
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
			s.notifyFlush(to)
		},
	})

	if s.bufCapacity > 0 {
		s.buf = newWriteBuffer(s.bufCapacity)
		s.stopCh = make(chan struct{})
		s.flushStopped = make(chan struct{})
		go s.flushLoop()
	}
	return s
}

// notifyFlush wakes the flush goroutine when the circuit breaker recovers.
func (s *Store) notifyFlush(state gobreaker.State) {
	if state != gobreaker.StateHalfOpen && state != gobreaker.StateClosed {
		return
	}
	select {
	case s.flushNotify <- struct{}{}:
	default:
	}
}

// CreateSession creates a new session via POST /api/v1/sessions.
// This is NOT buffered — callers need the session ID synchronously.
func (s *Store) CreateSession(ctx context.Context, opts session.CreateSessionOptions) (*session.Session, error) {
	id := uuid.New().String()
	reqBody := sessionapi.SessionToAPI(id, opts)

	resp, err := s.doJSON(ctx, http.MethodPost, "/api/v1/sessions", reqBody)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Treat 409 Conflict as success — the session already exists from a
	// previous attempt, making retries after network errors safe.
	if resp.StatusCode == http.StatusConflict {
		return s.GetSession(ctx, id)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, s.readError(resp)
	}

	var sr sessionapi.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode create session response: %w", err)
	}
	return sessionapi.SessionFromAPI(sr.Session), nil
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

	var sr sessionapi.SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode get session response: %w", err)
	}
	return sessionapi.SessionFromAPI(sr.Session), nil
}

// AppendMessage appends a message via POST /api/v1/sessions/{sessionID}/messages.
// On transient failure, the write is buffered and retried automatically.
func (s *Store) AppendMessage(ctx context.Context, sessionID string, msg session.Message) error {
	path := fmt.Sprintf("/api/v1/sessions/%s/messages", sessionID)
	body, err := json.Marshal(&msg)
	if err != nil {
		return fmt.Errorf("append message: encode: %w", err)
	}

	resp, err := s.doWithRetry(ctx, http.MethodPost, path, body)
	if err != nil {
		return s.bufferWrite(err, http.MethodPost, path, body)
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
// On transient failure, the write is buffered and retried automatically.
func (s *Store) UpdateSessionStats(ctx context.Context, sessionID string, update session.SessionStatsUpdate) error {
	path := fmt.Sprintf("/api/v1/sessions/%s/stats", sessionID)
	body, err := json.Marshal(&update)
	if err != nil {
		return fmt.Errorf("update session stats: encode: %w", err)
	}

	resp, err := s.doWithRetry(ctx, http.MethodPatch, path, body)
	if err != nil {
		return s.bufferWrite(err, http.MethodPatch, path, body)
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
// On transient failure, the write is buffered and retried automatically.
func (s *Store) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	path := fmt.Sprintf("/api/v1/sessions/%s/ttl", sessionID)
	body, err := json.Marshal(sessionapi.RefreshTTLRequest{TtlSeconds: int(ttl.Seconds())})
	if err != nil {
		return fmt.Errorf("refresh TTL: encode: %w", err)
	}

	resp, err := s.doWithRetry(ctx, http.MethodPost, path, body)
	if err != nil {
		return s.bufferWrite(err, http.MethodPost, path, body)
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

// RecordToolCall sends a tool call record via POST /api/v1/sessions/{sessionID}/tool-calls.
// On transient failure, the write is buffered and retried automatically.
func (s *Store) RecordToolCall(ctx context.Context, sessionID string, tc session.ToolCall) error {
	path := fmt.Sprintf("/api/v1/sessions/%s/tool-calls", sessionID)
	body, err := json.Marshal(&tc)
	if err != nil {
		return fmt.Errorf("record tool call: encode: %w", err)
	}

	resp, err := s.doWithRetry(ctx, http.MethodPost, path, body)
	if err != nil {
		return s.bufferWrite(err, http.MethodPost, path, body)
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

// RecordProviderCall sends a provider call record via POST /api/v1/sessions/{sessionID}/provider-calls.
// On transient failure, the write is buffered and retried automatically.
func (s *Store) RecordProviderCall(ctx context.Context, sessionID string, pc session.ProviderCall) error {
	path := fmt.Sprintf("/api/v1/sessions/%s/provider-calls", sessionID)
	body, err := json.Marshal(&pc)
	if err != nil {
		return fmt.Errorf("record provider call: encode: %w", err)
	}

	resp, err := s.doWithRetry(ctx, http.MethodPost, path, body)
	if err != nil {
		return s.bufferWrite(err, http.MethodPost, path, body)
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

// Ping checks session-api connectivity via a lightweight /healthz endpoint.
// Uses an uninstrumented HTTP client to avoid generating trace spans from
// K8s readiness probes (which call Ping every 10s per agent).
func (s *Store) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("httpclient: ping: %w", err)
	}
	resp, err := s.healthClient.Do(req)
	if err != nil {
		return fmt.Errorf("httpclient: ping: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("httpclient: ping: status %d", resp.StatusCode)
	}
	return nil
}

// Close stops the buffer flush goroutine and logs any remaining buffered writes.
func (s *Store) Close() error {
	if s.buf != nil {
		close(s.stopCh)
		<-s.flushStopped
		remaining := s.buf.len()
		if remaining > 0 {
			s.log.Info("closing with buffered writes pending",
				"remaining", remaining, "dropped", s.buf.dropped.Load())
		}
	}
	return nil
}

// Buffered returns the number of writes currently queued in the buffer.
func (s *Store) Buffered() int {
	if s.buf == nil {
		return 0
	}
	return s.buf.len()
}

// Dropped returns the total number of buffered writes dropped due to capacity.
func (s *Store) Dropped() int64 {
	if s.buf == nil {
		return 0
	}
	return s.buf.dropped.Load()
}

// --- Methods not used by the facade — return ErrNotImplemented ---

// DeleteSession is not used by the facade.
func (s *Store) DeleteSession(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// GetMessages is not used by the facade.
func (s *Store) GetMessages(_ context.Context, _ string) ([]session.Message, error) {
	return nil, ErrNotImplemented
}

// GetToolCalls retrieves tool calls via GET /api/v1/sessions/{sessionID}/tool-calls.
func (s *Store) GetToolCalls(ctx context.Context, sessionID string) ([]session.ToolCall, error) {
	resp, err := s.doWithRetry(ctx, http.MethodGet, fmt.Sprintf("/api/v1/sessions/%s/tool-calls", sessionID), nil)
	if err != nil {
		return nil, fmt.Errorf("get tool calls: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, session.ErrSessionNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, s.readError(resp)
	}

	var calls []session.ToolCall
	if err := json.NewDecoder(resp.Body).Decode(&calls); err != nil {
		return nil, fmt.Errorf("decode tool calls: %w", err)
	}
	return calls, nil
}

// GetProviderCalls retrieves provider calls via GET /api/v1/sessions/{sessionID}/provider-calls.
func (s *Store) GetProviderCalls(ctx context.Context, sessionID string) ([]session.ProviderCall, error) {
	resp, err := s.doWithRetry(ctx, http.MethodGet, fmt.Sprintf("/api/v1/sessions/%s/provider-calls", sessionID), nil)
	if err != nil {
		return nil, fmt.Errorf("get provider calls: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, session.ErrSessionNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, s.readError(resp)
	}

	var calls []session.ProviderCall
	if err := json.NewDecoder(resp.Body).Decode(&calls); err != nil {
		return nil, fmt.Errorf("decode provider calls: %w", err)
	}
	return calls, nil
}

// SetState is not used by the facade.
func (s *Store) SetState(_ context.Context, _, _, _ string) error {
	return ErrNotImplemented
}

// GetState is not used by the facade.
func (s *Store) GetState(_ context.Context, _, _ string) (string, error) {
	return "", ErrNotImplemented
}

// --- Write buffer ---

// bufferWrite enqueues a failed write for later retry. Returns nil so callers
// (which typically swallow errors) don't need to handle the failure.
// If buffering is disabled, returns the original error.
func (s *Store) bufferWrite(origErr error, method, path string, body []byte) error {
	if s.buf == nil {
		return origErr
	}
	dropped := s.buf.enqueue(bufferedRequest{
		method: method,
		path:   path,
		body:   body,
		queued: time.Now(),
	})
	s.log.V(1).Info("write buffered",
		"method", method, "path", path,
		"bufferLen", s.buf.len(), "dropped", dropped)
	return nil
}

// flushLoop periodically drains the write buffer. It also wakes when the
// circuit breaker transitions to half-open or closed.
func (s *Store) flushLoop() {
	defer close(s.flushStopped)
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			s.drainBuffer()
			return
		case <-s.flushNotify:
			s.drainBuffer()
		case <-ticker.C:
			s.drainBuffer()
		}
	}
}

// drainBuffer replays buffered writes until the buffer is empty or a write fails.
func (s *Store) drainBuffer() {
	for {
		item, ok := s.buf.peek()
		if !ok {
			return
		}
		if time.Since(item.queued) > s.bufferMaxAge {
			s.buf.dequeue()
			s.log.V(1).Info("buffer item expired",
				"method", item.method, "path", item.path,
				"age", time.Since(item.queued).Round(time.Second).String())
			continue
		}
		if !s.tryFlushItem(item) {
			return
		}
		s.buf.dequeue()
	}
}

// tryFlushItem attempts to replay a single buffered write. Returns true if the
// item was successfully sent (or permanently rejected), false if the service is
// still unavailable and flushing should stop.
func (s *Store) tryFlushItem(item bufferedRequest) bool {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultHTTPTimeout)
	defer cancel()

	resp, err := s.doWithRetry(ctx, item.method, item.path, item.body)
	if err != nil {
		s.log.V(1).Info("buffer flush failed",
			"method", item.method, "path", item.path, "error", err.Error())
		return false
	}
	status := resp.StatusCode
	_ = drainAndClose(resp.Body)
	s.log.V(1).Info("buffer item flushed",
		"method", item.method, "path", item.path, "status", status)
	return true
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

func (s *Store) readError(resp *http.Response) error {
	var errResp sessionapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, errResp.Error)
}

// Interface assertion.
var _ session.Store = (*Store)(nil)
