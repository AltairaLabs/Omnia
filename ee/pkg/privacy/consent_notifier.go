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
	"net/url"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/serviceauth"
	"github.com/altairalabs/omnia/pkg/logging"
)

// ConsentNotifier is implemented by types that notify downstream services of
// a consent revocation for a specific user and category.
// delivered is true only if every configured target returned 2xx; it is also
// true when there are zero targets (vacuously delivered). err is always nil —
// delivery failures are logged per-target and never propagated to the caller.
type ConsentNotifier interface {
	NotifyRevocation(ctx context.Context, userID string, category ConsentCategory) (delivered bool, err error)
}

// NoopConsentNotifier is a nil-safe ConsentNotifier that does nothing.
// Use it when no memory URLs are configured and the notify call must be inert.
type NoopConsentNotifier struct{}

// NotifyRevocation implements ConsentNotifier by doing nothing.
// Nothing to deliver → vacuously delivered.
func (NoopConsentNotifier) NotifyRevocation(_ context.Context, _ string, _ ConsentCategory) (bool, error) {
	return true, nil
}

// consentEventBody is the JSON payload sent to the memory-api consent-events endpoint.
type consentEventBody struct {
	UserID   string `json:"userId"`
	Category string `json:"category"`
}

// MemoryAPINotifier fans out consent revocation events to one or more memory-api
// instances. It implements ConsentNotifier. Failures are per-target — one failed
// target never aborts others, and the overall return is always nil so callers
// never fail a consent write because of a push failure.
type MemoryAPINotifier struct {
	urls      []string
	workspace string
	ts        *serviceauth.TokenSource
	client    *http.Client
	log       logr.Logger
}

// NewMemoryAPINotifier creates a MemoryAPINotifier. memoryURLs is the set of
// memory-api base URLs to fan out to, resolved by the caller from the workspace
// — there is no env override, because the workspace is the only source of truth
// for a service endpoint. workspace is appended as a required ?workspace= query
// parameter on every POST — the
// memory-api consent-events endpoint returns 400 without it. ts is the SA token
// source used to attach an Authorization: Bearer header; pass nil to send requests
// unauthenticated (development / tests). An empty URL set is valid and causes
// NotifyRevocation to be a no-op.
func NewMemoryAPINotifier(
	memoryURLs []string,
	workspace string,
	ts *serviceauth.TokenSource,
	log logr.Logger,
) *MemoryAPINotifier {
	return &MemoryAPINotifier{
		urls:      memoryURLs,
		workspace: workspace,
		ts:        ts,
		client:    &http.Client{Timeout: 10 * time.Second},
		log:       log.WithName("consent-notifier"),
	}
}

// NotifyRevocation POSTs a consent-revocation event to every configured
// memory-api URL. Failures are logged per target; err is always nil so the
// caller's consent write is never rolled back due to a push failure.
// delivered is true only when every target returned 2xx (or there are none).
func (n *MemoryAPINotifier) NotifyRevocation(
	ctx context.Context, userID string, category ConsentCategory,
) (bool, error) {
	if len(n.urls) == 0 {
		n.log.V(1).Info("consent notify skipped", "reason", "no memory URLs configured")
		return true, nil
	}

	body, err := json.Marshal(consentEventBody{UserID: userID, Category: string(category)})
	if err != nil {
		// JSON marshal of two plain strings cannot fail in practice, but guard
		// defensively: treat as undelivered, keep err nil per contract.
		n.log.Error(err, "consent notifier: marshal body failed")
		return false, nil
	}

	allOK := true
	for _, baseURL := range n.urls {
		if pushErr := n.pushOne(ctx, baseURL, body, userID, category); pushErr != nil {
			n.log.Error(pushErr, "consent notify failed for target",
				"targetURL", baseURL,
				"userHash", logging.HashID(userID),
				"category", string(category),
			)
			// Best-effort: continue to remaining targets regardless of this failure.
			allOK = false
		}
	}
	return allOK, nil
}

// pushOne sends a single POST to one memory-api target.
func (n *MemoryAPINotifier) pushOne(
	ctx context.Context,
	baseURL string,
	body []byte,
	userID string,
	category ConsentCategory,
) error {
	target := baseURL + "/api/v1/memories/consent-events"
	if n.workspace != "" {
		target = target + "?" + url.Values{ScopeWorkspace: {n.workspace}}.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if n.ts != nil {
		if authErr := n.ts.Authorize(req); authErr != nil {
			return fmt.Errorf("set auth header: %w", authErr)
		}
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", target, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("POST %s returned status %d", target, resp.StatusCode)
	}

	n.log.V(1).Info("consent notify sent",
		"targetURL", baseURL,
		"userHash", logging.HashID(userID),
		"category", string(category),
	)
	return nil
}
