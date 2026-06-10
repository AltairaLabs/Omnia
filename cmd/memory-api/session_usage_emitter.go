/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/memory"
)

// sessionUsageEmitter forwards workspace-scoped provider spend to session-api's
// POST /api/v1/provider-usage endpoint. It is deliberately thin and
// best-effort: emits happen on a background goroutine with their own timeout so
// they never block or fail the embedding path, and excess emits are dropped
// (with a debug log) rather than queued unboundedly.
type sessionUsageEmitter struct {
	url    string // fully-qualified POST URL
	client *http.Client
	sem    chan struct{} // bounds concurrent in-flight emits
	log    logr.Logger
}

// sessionUsageEmitTimeout bounds a single emit's HTTP round trip.
const sessionUsageEmitTimeout = 5 * time.Second

// sessionUsageEmitConcurrency caps concurrent in-flight emits; excess is dropped.
const sessionUsageEmitConcurrency = 32

// providerUsagePayload mirrors session-api's ProviderUsage JSON contract. Kept
// local so memory-api does not import the session-api package.
type providerUsagePayload struct {
	Namespace     string  `json:"namespace"`
	WorkspaceName string  `json:"workspaceName,omitempty"`
	Provider      string  `json:"provider"`
	ProviderName  string  `json:"providerName,omitempty"`
	Model         string  `json:"model,omitempty"`
	Source        string  `json:"source"`
	InputTokens   int64   `json:"inputTokens,omitempty"`
	OutputTokens  int64   `json:"outputTokens,omitempty"`
	CachedTokens  int64   `json:"cachedTokens,omitempty"`
	CostUSD       float64 `json:"costUsd,omitempty"`
	CallCount     int32   `json:"callCount,omitempty"`
}

// newSessionUsageEmitter returns an emitter that POSTs to baseURL, or nil when
// baseURL is empty (emit disabled; only the Prometheus counter is updated).
func newSessionUsageEmitter(baseURL string, log logr.Logger) memory.ProviderUsageEmitter {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &sessionUsageEmitter{
		url:    strings.TrimRight(baseURL, "/") + "/api/v1/provider-usage",
		client: &http.Client{Timeout: sessionUsageEmitTimeout},
		sem:    make(chan struct{}, sessionUsageEmitConcurrency),
		log:    log.WithName("session-usage-emitter"),
	}
}

// EmitProviderUsage sends one usage record on a background goroutine. Drops the
// record (logging at V(1)) when the concurrency cap is saturated.
func (e *sessionUsageEmitter) EmitProviderUsage(_ context.Context, rec memory.ProviderUsageRecord) {
	select {
	case e.sem <- struct{}{}:
	default:
		e.log.V(1).Info("dropping provider usage emit (concurrency cap reached)",
			"namespace", rec.Namespace, "source", rec.Source)
		return
	}

	go func() {
		defer func() { <-e.sem }()
		ctx, cancel := context.WithTimeout(context.Background(), sessionUsageEmitTimeout)
		defer cancel()
		if err := e.post(ctx, rec); err != nil {
			e.log.V(1).Info("provider usage emit failed", "error", err.Error(),
				"namespace", rec.Namespace, "source", rec.Source)
		}
	}()
}

func (e *sessionUsageEmitter) post(ctx context.Context, rec memory.ProviderUsageRecord) error {
	body, err := json.Marshal([]providerUsagePayload{{
		Namespace:     rec.Namespace,
		WorkspaceName: rec.WorkspaceName,
		Provider:      rec.Provider,
		ProviderName:  rec.ProviderName,
		Model:         rec.Model,
		Source:        rec.Source,
		InputTokens:   rec.InputTokens,
		CallCount:     rec.CallCount,
	}})
	if err != nil {
		return fmt.Errorf("marshal provider usage: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("post provider usage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("session-api returned %d", resp.StatusCode)
	}
	return nil
}
