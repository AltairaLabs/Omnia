/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package license

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
)

// LicensePath is the operator/arena-controller endpoint that serves the
// current license for backend services to gate on.
const LicensePath = "/api/v1/license"

// defaultClientTTL is how often the background refresher re-reads the license.
// It matches the dashboard's license refresh interval so all consumers observe
// entitlement changes on the same cadence.
const defaultClientTTL = 5 * time.Minute

// defaultClientTimeout bounds a single license fetch. Fetches run on the
// background refresher, never on a request path.
const defaultClientTimeout = 5 * time.Second

// Client fetches the license from the operator/arena-controller over HTTP and
// caches it. Backend services (memory-api, privacy-api, policy-proxy) use it to
// gate enterprise capabilities on the real license instead of a bare infra
// flag, without needing cross-namespace Secret access. It mirrors how the
// dashboard resolves the license for white-label enforcement.
//
// Reads are served from cache and never block on network I/O — call Start to
// keep the cache warm in the background, or Refresh for a one-shot update. The
// zero value is not usable; construct with NewClient.
type Client struct {
	baseURL string
	httpC   *http.Client
	ttl     time.Duration
	log     logr.Logger

	fetchMu sync.Mutex // serializes fetches so an outage can't cause a stampede

	mu     sync.RWMutex
	cached *License
}

// ClientOption customizes a Client.
type ClientOption func(*Client)

// WithClientTTL overrides the background refresh interval.
func WithClientTTL(ttl time.Duration) ClientOption {
	return func(c *Client) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

// WithClientHTTP overrides the HTTP client used for license fetches.
func WithClientHTTP(h *http.Client) ClientOption {
	return func(c *Client) {
		if h != nil {
			c.httpC = h
		}
	}
}

// WithClientLogger sets the logger used to report fetch failures.
func WithClientLogger(log logr.Logger) ClientOption {
	return func(c *Client) { c.log = log }
}

// NewClient returns a license Client that reads from baseURL (e.g.
// http://omnia-arena-controller.omnia-system:8082). A trailing slash is
// tolerated; LicensePath is appended per request.
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpC:   &http.Client{Timeout: defaultClientTimeout},
		ttl:     defaultClientTTL,
		log:     logr.Discard(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// License returns the currently cached license as a copy, without ever making
// a network call. Before the first successful fetch it returns an open-core
// license, so an uninitialized or never-reachable operator degrades to
// open-core rather than failing open. The returned pointer is the caller's own
// copy — mutating it cannot corrupt the shared cache.
func (c *Client) License() *License {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.cached == nil {
		return OpenCoreLicense()
	}
	cp := *c.cached
	return &cp
}

// Refresh fetches the license once and, on success, updates the cache. It
// returns the fetch error (if any) so a caller that needs a definitive answer
// — e.g. a startup decision — can distinguish "operator unreachable" from
// "operator says open-core". On failure the previously cached license is left
// intact (last-good survives transient outages). Concurrent Refresh calls are
// serialized so an outage cannot spawn a fetch storm.
func (c *Client) Refresh(ctx context.Context) (*License, error) {
	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()

	lic, err := c.fetch(ctx)
	if err != nil {
		c.log.Error(err, "license fetch failed; keeping last-known license", "url", c.baseURL)
		return nil, err
	}

	c.mu.Lock()
	c.cached = lic
	c.mu.Unlock()

	cp := *lic
	return &cp, nil
}

// Start launches a background goroutine that refreshes the cache every TTL
// until ctx is cancelled. Fetch failures are logged and leave the last-good
// license in place. Call it once after construction; License() then serves the
// warm cache without blocking request paths.
func (c *Client) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(c.ttl)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = c.Refresh(ctx)
			}
		}
	}()
}

// fetch performs a single license request and decodes it.
func (c *Client) fetch(ctx context.Context) (*License, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+LicensePath, nil)
	if err != nil {
		return nil, fmt.Errorf("build license request: %w", err)
	}

	resp, err := c.httpC.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch license: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("license endpoint returned HTTP %d", resp.StatusCode)
	}

	// The endpoint serves the canonical license.License JSON — the same struct
	// the validator produces from the signed JWT — so we decode straight into
	// it. One struct, one parser, regardless of source.
	var lic License
	if err := json.NewDecoder(resp.Body).Decode(&lic); err != nil {
		return nil, fmt.Errorf("decode license response: %w", err)
	}
	return &lic, nil
}
