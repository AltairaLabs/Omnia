/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session/api"
)

// newTestLogger creates a silent logger for tests.
func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// makeResults creates a slice of EvalResult with the given pass/fail pattern.
// true = passed, false = failed.
func makeResults(evalID string, pattern []bool) []api.EvalResult {
	results := make([]api.EvalResult, len(pattern))
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, passed := range pattern {
		results[i] = api.EvalResult{
			EvalID:    evalID,
			SessionID: "session-" + string(rune('A'+i)),
			MessageID: "msg-" + string(rune('A'+i)),
			Passed:    passed,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
	}
	return results
}

func TestCheckAndFire_BelowThreshold(t *testing.T) {
	var received atomic.Int32
	var payload WebhookPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.8,
		WindowSize: 5,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())

	// 2 out of 5 passed = 40% pass rate, below 80% threshold.
	results := makeResults("eval-1", []bool{true, false, false, false, true})

	err := d.CheckAndFire(context.Background(), "eval-1", "my-agent", "default", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Load() != 1 {
		t.Fatalf("expected 1 webhook call, got %d", received.Load())
	}

	if payload.AgentName != "my-agent" {
		t.Errorf("expected agentName 'my-agent', got %q", payload.AgentName)
	}
	if payload.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", payload.Namespace)
	}
	if payload.EvalID != "eval-1" {
		t.Errorf("expected evalId 'eval-1', got %q", payload.EvalID)
	}
	if payload.Threshold != 0.8 {
		t.Errorf("expected threshold 0.8, got %f", payload.Threshold)
	}
	if len(payload.RecentFailures) != 3 {
		t.Errorf("expected 3 recent failures, got %d", len(payload.RecentFailures))
	}
}

func TestCheckAndFire_AcceptablePassRate(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.5,
		WindowSize: 4,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())

	// 3 out of 4 passed = 75% pass rate, above 50% threshold.
	results := makeResults("eval-1", []bool{true, true, false, true})

	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Load() != 0 {
		t.Fatalf("expected 0 webhook calls, got %d", received.Load())
	}
}

func TestCheckAndFire_ConsecutiveFailures(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:              srv.URL,
		Threshold:        0.0, // pass rate threshold won't trigger
		ConsecutiveFails: 3,
		WindowSize:       10,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())

	// Last 3 are failures (consecutive).
	results := makeResults("eval-1", []bool{true, true, true, false, false, false})

	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Load() != 1 {
		t.Fatalf("expected 1 webhook call, got %d", received.Load())
	}
}

func TestCheckAndFire_ConsecutiveFailsNotReached(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:              srv.URL,
		Threshold:        0.0,
		ConsecutiveFails: 5,
		WindowSize:       10,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())

	// Only 2 consecutive failures, threshold is 5.
	results := makeResults("eval-1", []bool{true, true, false, false})

	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Load() != 0 {
		t.Fatalf("expected 0 webhook calls, got %d", received.Load())
	}
}

func TestCheckAndFire_RateLimiting(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.9,
		WindowSize: 5,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())

	// All failures — will always trigger.
	results := makeResults("eval-1", []bool{false, false, false, false, false})

	// First call should fire.
	_ = d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if received.Load() != 1 {
		t.Fatalf("expected 1 webhook call after first check, got %d", received.Load())
	}

	// Second call should be rate limited.
	_ = d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if received.Load() != 1 {
		t.Fatalf("expected still 1 webhook call after rate limiting, got %d", received.Load())
	}
}

func TestCheckAndFire_RetryOnHTTPError(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.9,
		WindowSize: 5,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())
	results := makeResults("eval-1", []bool{false, false, false, false, false})

	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if attempts.Load() != 3 {
		t.Fatalf("expected 3 HTTP attempts, got %d", attempts.Load())
	}
}

func TestCheckAndFire_AllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.9,
		WindowSize: 5,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())
	results := makeResults("eval-1", []bool{false, false, false, false, false})

	// CheckAndFire logs errors but does not return them per config.
	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("CheckAndFire should not propagate errors, got: %v", err)
	}
}

func TestCheckAndFire_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders = r.Header.Clone()
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.9,
		WindowSize: 5,
		Headers: map[string]string{
			"Authorization": "Bearer secret-token",
			"X-Custom":      "custom-value",
		},
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())
	results := makeResults("eval-1", []bool{false, false, false, false, false})

	_ = d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)

	mu.Lock()
	defer mu.Unlock()

	if receivedHeaders.Get("Authorization") != "Bearer secret-token" {
		t.Errorf("expected Authorization header, got %q", receivedHeaders.Get("Authorization"))
	}
	if receivedHeaders.Get("X-Custom") != "custom-value" {
		t.Errorf("expected X-Custom header, got %q", receivedHeaders.Get("X-Custom"))
	}
	if receivedHeaders.Get(contentTypeHeader) != contentTypeJSON {
		t.Errorf("expected Content-Type %q, got %q", contentTypeJSON, receivedHeaders.Get(contentTypeHeader))
	}
}

func TestCheckAndFire_TimeoutHandling(t *testing.T) {
	// Server that blocks longer than the timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.9,
		WindowSize: 5,
	}}

	// Use a very short timeout client.
	client := &http.Client{Timeout: 50 * time.Millisecond}
	d := NewWebhookDispatcher(configs, client, newTestLogger())
	results := makeResults("eval-1", []bool{false, false, false, false, false})

	// Should not block forever; CheckAndFire swallows errors.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := d.CheckAndFire(ctx, "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckAndFire_EmptyResults(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.5,
		WindowSize: 5,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())

	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Load() != 0 {
		t.Fatalf("expected 0 webhook calls for empty results, got %d", received.Load())
	}
}

func TestCheckAndFire_WindowSizeLargerThanResults(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.8,
		WindowSize: 100, // much larger than available results
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())

	// 1 out of 2 = 50% — below 80%.
	results := makeResults("eval-1", []bool{true, false})

	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Load() != 1 {
		t.Fatalf("expected 1 webhook call, got %d", received.Load())
	}
}

func TestCheckAndFire_DifferentEvalIDFiltered(t *testing.T) {
	var received atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.8,
		WindowSize: 5,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())

	// Results are for "eval-2" but we check "eval-1".
	results := makeResults("eval-2", []bool{false, false, false})

	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received.Load() != 0 {
		t.Fatalf("expected 0 webhook calls for mismatched eval ID, got %d", received.Load())
	}
}

func TestCheckAndFire_MultipleConfigs(t *testing.T) {
	var received1, received2 atomic.Int32

	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received1.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		received2.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv2.Close()

	configs := []WebhookConfig{
		{URL: srv1.URL, Threshold: 0.9, WindowSize: 5},
		{URL: srv2.URL, Threshold: 0.5, WindowSize: 5},
	}

	d := NewWebhookDispatcher(configs, nil, newTestLogger())

	// 60% pass rate: triggers srv1 (threshold 0.9) but not srv2 (threshold 0.5).
	results := makeResults("eval-1", []bool{true, true, true, false, false})

	err := d.CheckAndFire(context.Background(), "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if received1.Load() != 1 {
		t.Errorf("expected srv1 to receive 1 call, got %d", received1.Load())
	}
	if received2.Load() != 0 {
		t.Errorf("expected srv2 to receive 0 calls, got %d", received2.Load())
	}
}

func TestPassRate(t *testing.T) {
	tests := []struct {
		name     string
		pattern  []bool
		expected float64
	}{
		{"all passed", []bool{true, true, true}, 1.0},
		{"all failed", []bool{false, false, false}, 0.0},
		{"mixed", []bool{true, false, true, false}, 0.5},
		{"empty", []bool{}, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := makeResults("eval-1", tt.pattern)
			rate := passRate(results)
			if rate != tt.expected {
				t.Errorf("expected %f, got %f", tt.expected, rate)
			}
		})
	}
}

func TestConsecutiveFailCount(t *testing.T) {
	tests := []struct {
		name     string
		pattern  []bool
		expected int
	}{
		{"trailing failures", []bool{true, true, false, false, false}, 3},
		{"no failures", []bool{true, true, true}, 0},
		{"all failures", []bool{false, false, false}, 3},
		{"pass then fail then pass", []bool{true, false, true}, 0},
		{"empty", []bool{}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := makeResults("eval-1", tt.pattern)
			count := consecutiveFailCount(results)
			if count != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, count)
			}
		})
	}
}

func TestNewWebhookDispatcher_Defaults(t *testing.T) {
	d := NewWebhookDispatcher(nil, nil, nil)
	if d.httpClient == nil {
		t.Error("expected default HTTP client to be set")
	}
	if d.logger == nil {
		t.Error("expected default logger to be set")
	}
	if d.lastFired == nil {
		t.Error("expected lastFired map to be initialized")
	}
}

func TestContextCancellation(t *testing.T) {
	// Server that blocks.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	configs := []WebhookConfig{{
		URL:        srv.URL,
		Threshold:  0.9,
		WindowSize: 5,
	}}

	d := NewWebhookDispatcher(configs, srv.Client(), newTestLogger())
	results := makeResults("eval-1", []bool{false, false, false, false, false})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := d.CheckAndFire(ctx, "eval-1", "agent", "ns", results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
