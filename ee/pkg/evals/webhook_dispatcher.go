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
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/altairalabs/omnia/internal/session/api"
)

// Constants for webhook dispatcher configuration.
const (
	webhookTimeout      = 10 * time.Second
	rateLimitInterval   = 1 * time.Minute
	maxRetries          = 3
	initialRetryBackoff = 1 * time.Second
	backoffMultiplier   = 2
	contentTypeJSON     = "application/json"
	contentTypeHeader   = "Content-Type"
)

// WebhookDispatcher evaluates recent eval results against configured
// thresholds and fires webhook alerts when pass rates drop too low.
type WebhookDispatcher struct {
	configs    []WebhookConfig
	httpClient *http.Client
	logger     *slog.Logger

	mu        sync.Mutex
	lastFired map[string]time.Time // key: evalID+configURL -> last fire time
}

// NewWebhookDispatcher creates a new dispatcher with the given configs.
func NewWebhookDispatcher(
	configs []WebhookConfig,
	httpClient *http.Client,
	logger *slog.Logger,
) *WebhookDispatcher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: webhookTimeout}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookDispatcher{
		configs:    configs,
		httpClient: httpClient,
		logger:     logger,
		lastFired:  make(map[string]time.Time),
	}
}

// CheckAndFire evaluates recent results against all configured webhooks
// and fires alerts when thresholds are breached.
func (d *WebhookDispatcher) CheckAndFire(
	ctx context.Context,
	evalID, agentName, namespace string,
	recentResults []api.EvalResult,
) error {
	for i := range d.configs {
		cfg := &d.configs[i]
		if err := d.checkConfig(ctx, cfg, evalID, agentName, namespace, recentResults); err != nil {
			d.logger.Error("webhook check failed",
				"evalID", evalID,
				"url", cfg.URL,
				"error", err,
			)
		}
	}
	return nil
}

// checkConfig evaluates a single webhook config and fires if triggered.
func (d *WebhookDispatcher) checkConfig(
	ctx context.Context,
	cfg *WebhookConfig,
	evalID, agentName, namespace string,
	results []api.EvalResult,
) error {
	if !d.shouldFire(cfg, evalID, results) {
		return nil
	}

	if d.isRateLimited(evalID, cfg.URL) {
		d.logger.Debug("webhook rate limited",
			"evalID", evalID,
			"url", cfg.URL,
		)
		return nil
	}

	payload := d.buildPayload(cfg, evalID, agentName, namespace, results)
	if err := d.sendWebhook(ctx, cfg, payload); err != nil {
		return err
	}

	d.recordFired(evalID, cfg.URL)
	return nil
}

// shouldFire determines whether the webhook should be triggered based on
// pass rate threshold or consecutive failure count.
func (d *WebhookDispatcher) shouldFire(
	cfg *WebhookConfig,
	evalID string,
	results []api.EvalResult,
) bool {
	filtered := filterByEvalID(results, evalID)
	if len(filtered) == 0 {
		return false
	}

	window := applyWindow(filtered, cfg.WindowSize)
	if len(window) == 0 {
		return false
	}

	if passRate(window) < cfg.Threshold {
		return true
	}

	if cfg.ConsecutiveFails > 0 && consecutiveFailCount(window) >= cfg.ConsecutiveFails {
		return true
	}

	return false
}

// buildPayload constructs the webhook payload from config and results.
func (d *WebhookDispatcher) buildPayload(
	cfg *WebhookConfig,
	evalID, agentName, namespace string,
	results []api.EvalResult,
) WebhookPayload {
	filtered := filterByEvalID(results, evalID)
	window := applyWindow(filtered, cfg.WindowSize)

	return WebhookPayload{
		AgentName:       agentName,
		Namespace:       namespace,
		EvalID:          evalID,
		CurrentPassRate: passRate(window),
		Threshold:       cfg.Threshold,
		WindowSize:      len(window),
		TriggeredAt:     time.Now(),
		RecentFailures:  collectFailureSamples(window),
	}
}

// sendWebhook posts the payload to the webhook URL with retries.
func (d *WebhookDispatcher) sendWebhook(
	ctx context.Context,
	cfg *WebhookConfig,
	payload WebhookPayload,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	backoff := initialRetryBackoff
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepWithContext(ctx, backoff); err != nil {
				return fmt.Errorf("retry wait interrupted: %w", err)
			}
			backoff *= backoffMultiplier
		}

		lastErr = d.doPost(ctx, cfg, body)
		if lastErr == nil {
			return nil
		}

		d.logger.Warn("webhook attempt failed",
			"attempt", attempt+1,
			"url", cfg.URL,
			"error", lastErr,
		)
	}

	return fmt.Errorf("webhook failed after %d attempts: %w", maxRetries, lastErr)
}

// doPost performs a single HTTP POST to the webhook URL.
func (d *WebhookDispatcher) doPost(
	ctx context.Context,
	cfg *WebhookConfig,
	body []byte,
) error {
	reqCtx, cancel := context.WithTimeout(ctx, webhookTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set(contentTypeHeader, contentTypeJSON)
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", cfg.URL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("POST %s returned status %d", cfg.URL, resp.StatusCode)
	}

	return nil
}

// isRateLimited checks if the webhook was fired too recently for the given eval.
func (d *WebhookDispatcher) isRateLimited(evalID, url string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := rateLimitKey(evalID, url)
	last, ok := d.lastFired[key]
	if !ok {
		return false
	}
	return time.Since(last) < rateLimitInterval
}

// recordFired records the current time as the last fire time for rate limiting.
func (d *WebhookDispatcher) recordFired(evalID, url string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastFired[rateLimitKey(evalID, url)] = time.Now()
}

// rateLimitKey builds the map key for rate limiting.
func rateLimitKey(evalID, url string) string {
	return evalID + "|" + url
}

// filterByEvalID returns only results matching the given eval ID.
func filterByEvalID(results []api.EvalResult, evalID string) []api.EvalResult {
	filtered := make([]api.EvalResult, 0, len(results))
	for i := range results {
		if results[i].EvalID == evalID {
			filtered = append(filtered, results[i])
		}
	}
	return filtered
}

// applyWindow truncates results to the most recent windowSize entries.
// Results are assumed to be ordered by creation time (newest first or oldest first).
func applyWindow(results []api.EvalResult, windowSize int) []api.EvalResult {
	if windowSize <= 0 || windowSize >= len(results) {
		return results
	}
	return results[len(results)-windowSize:]
}

// passRate calculates the fraction of results that passed.
func passRate(results []api.EvalResult) float64 {
	if len(results) == 0 {
		return 1.0
	}
	passed := 0
	for i := range results {
		if results[i].Passed {
			passed++
		}
	}
	return float64(passed) / float64(len(results))
}

// consecutiveFailCount returns the number of consecutive failures
// from the end of the results slice (most recent).
func consecutiveFailCount(results []api.EvalResult) int {
	count := 0
	for i := len(results) - 1; i >= 0; i-- {
		if results[i].Passed {
			break
		}
		count++
	}
	return count
}

// collectFailureSamples extracts failure details for the webhook payload.
func collectFailureSamples(results []api.EvalResult) []WebhookFailureSample {
	samples := make([]WebhookFailureSample, 0)
	for i := range results {
		if !results[i].Passed {
			samples = append(samples, WebhookFailureSample{
				SessionID: results[i].SessionID,
				MessageID: results[i].MessageID,
				CreatedAt: results[i].CreatedAt,
			})
		}
	}
	return samples
}

// sleepWithContext sleeps for the given duration or until the context is cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
