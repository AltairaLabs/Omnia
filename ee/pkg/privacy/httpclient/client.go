/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// Package httpclient provides a privacy.PreferencesStore and privacy.ConsentSource
// implementation backed by HTTP calls to the privacy-api service.
//
// All reads are fail-closed: a transport error or unexpected non-2xx response
// returns a non-nil error so callers can gate on privacy rather than silently
// permitting access.  A 404 (user has no preferences row) is the only
// non-error "empty" state and maps to privacy.ErrPreferencesNotFound.
package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/internal/serviceauth"
)

// tokenPathEnv overrides the ServiceAccount token file path used to authenticate
// requests to privacy-api.  Unset → serviceauth.DefaultTokenPath.
const tokenPathEnv = "PRIVACY_API_TOKEN_PATH"

const (
	defaultCacheTTL = 30 * time.Second
	defaultTimeout  = 10 * time.Second
)

// Option configures a Client.
type Option func(*Client)

// WithCacheTTL sets the per-userID preferences cache TTL. Default 30s.
func WithCacheTTL(d time.Duration) Option {
	return func(c *Client) { c.cacheTTL = d }
}

// WithHTTPClient replaces the default HTTP client (e.g. for tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithTokenSource sets a custom ServiceAccount token source.
func WithTokenSource(ts *serviceauth.TokenSource) Option {
	return func(c *Client) { c.tokenSource = ts }
}

// cacheEntry holds a cached Preferences value and its expiry time.
type cacheEntry struct {
	prefs     *privacy.Preferences
	expiresAt time.Time
}

// consentUsersCacheEntry holds a cached consent-user list and its expiry time.
type consentUsersCacheEntry struct {
	userIDs   []string
	expiresAt time.Time
}

// Client calls the privacy-api service. It is concurrency-safe.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	log         logr.Logger
	tokenSource *serviceauth.TokenSource
	cacheTTL    time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry

	cuMu    sync.RWMutex
	cuCache map[string]consentUsersCacheEntry
}

// Compile-time interface assertions.
var _ privacy.PreferencesStore = (*Client)(nil)
var _ privacy.ConsentSource = (*Client)(nil)

// New creates a Client pointing at baseURL.
func New(baseURL string, log logr.Logger, opts ...Option) *Client {
	transport := &http.Transport{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}
	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   defaultTimeout,
			Transport: otelhttp.NewTransport(transport),
		},
		log:         log.WithName("privacy-httpclient"),
		tokenSource: serviceauth.NewTokenSource(os.Getenv(tokenPathEnv), 0),
		cacheTTL:    defaultCacheTTL,
		cache:       make(map[string]cacheEntry),
		cuCache:     make(map[string]consentUsersCacheEntry),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetPreferences fetches privacy preferences for userID.
//   - 404 → privacy.ErrPreferencesNotFound (sentinel; errors.Is works).
//   - Other non-2xx or transport error → wrapped error (fail-closed signal).
//   - Successful responses are cached for cacheTTL.
func (c *Client) GetPreferences(ctx context.Context, userID string) (*privacy.Preferences, error) {
	if p := c.getFromCache(userID); p != nil {
		return p, nil
	}

	resp, err := c.doRequest(ctx, http.MethodGet, c.prefsURL(userID), nil)
	if err != nil {
		return nil, fmt.Errorf("privacy: get preferences: %w", err)
	}
	defer func() { _ = drainAndClose(resp.Body) }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("privacy: get preferences: %w", privacy.ErrPreferencesNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, readHTTPError(resp)
	}

	prefs, err := decodePreferences(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("privacy: decode preferences: %w", err)
	}
	c.putInCache(userID, prefs)
	return prefs, nil
}

// GetConsentGrants returns the consent grants for userID.
// When the user has no preferences row (ErrPreferencesNotFound) it returns an
// empty slice with no error, matching the Postgres store's contract so
// no-preferences users are not blocked on non-PII categories.
func (c *Client) GetConsentGrants(ctx context.Context, userID string) ([]privacy.ConsentCategory, error) {
	prefs, err := c.GetPreferences(ctx, userID)
	if errors.Is(err, privacy.ErrPreferencesNotFound) {
		return []privacy.ConsentCategory{}, nil
	}
	if err != nil {
		return nil, err
	}
	return prefs.ConsentGrants, nil
}

// SetOptOut sets an opt-out preference and invalidates the in-memory cache entry.
func (c *Client) SetOptOut(ctx context.Context, userID, scope, target string) error {
	body := privacy.OptOutRequest{UserID: userID, Scope: scope, Target: target}
	if err := c.doMutate(ctx, http.MethodPost, body); err != nil {
		return fmt.Errorf("privacy: set opt-out: %w", err)
	}
	c.evictFromCache(userID)
	return nil
}

// RemoveOptOut removes an opt-out preference and invalidates the in-memory cache entry.
func (c *Client) RemoveOptOut(ctx context.Context, userID, scope, target string) error {
	body := privacy.OptOutRequest{UserID: userID, Scope: scope, Target: target}
	if err := c.doMutate(ctx, http.MethodDelete, body); err != nil {
		return fmt.Errorf("privacy: remove opt-out: %w", err)
	}
	c.evictFromCache(userID)
	return nil
}

// --- cache helpers ---

func (c *Client) getFromCache(userID string) *privacy.Preferences {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.cache[userID]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.prefs
}

func (c *Client) putInCache(userID string, prefs *privacy.Preferences) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[userID] = cacheEntry{prefs: prefs, expiresAt: time.Now().Add(c.cacheTTL)}
}

func (c *Client) evictFromCache(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, userID)
}

// GetConsentStats fetches workspace-wide consent statistics from the privacy-api.
func (c *Client) GetConsentStats(ctx context.Context) (*privacy.ConsentStats, error) {
	url := c.baseURL + "/api/v1/privacy/consent/stats?workspace=default"
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("privacy: get consent stats: %w", err)
	}
	defer func() { _ = drainAndClose(resp.Body) }()

	if !isSuccessStatus(resp.StatusCode) {
		return nil, readHTTPError(resp)
	}

	var stats privacy.ConsentStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("privacy: decode consent stats: %w", err)
	}
	return &stats, nil
}

// ListConsentUsers returns user IDs (pseudonyms) whose consent for the given
// category matches granted. Results are cached for cacheTTL to reduce load
// under frequent per-aggregate calls (keyed by category|granted).
func (c *Client) ListConsentUsers(
	ctx context.Context, category privacy.ConsentCategory, granted bool,
) ([]string, error) {
	key := string(category) + "|" + strconv.FormatBool(granted)

	if ids := c.getConsentUsersFromCache(key); ids != nil {
		return ids, nil
	}

	url := fmt.Sprintf("%s/api/v1/privacy/consent/users?category=%s&granted=%s",
		c.baseURL, string(category), strconv.FormatBool(granted))
	resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("privacy: list consent users: %w", err)
	}
	defer func() { _ = drainAndClose(resp.Body) }()

	if !isSuccessStatus(resp.StatusCode) {
		return nil, readHTTPError(resp)
	}

	var body struct {
		UserIDs []string `json:"userIds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("privacy: decode consent users: %w", err)
	}

	ids := body.UserIDs
	if ids == nil {
		ids = []string{}
	}
	c.putConsentUsersInCache(key, ids)
	return ids, nil
}

// --- consent users cache helpers ---

func (c *Client) getConsentUsersFromCache(key string) []string {
	c.cuMu.RLock()
	defer c.cuMu.RUnlock()
	entry, ok := c.cuCache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.userIDs
}

func (c *Client) putConsentUsersInCache(key string, userIDs []string) {
	c.cuMu.Lock()
	defer c.cuMu.Unlock()
	c.cuCache[key] = consentUsersCacheEntry{
		userIDs:   userIDs,
		expiresAt: time.Now().Add(c.cacheTTL),
	}
}

// --- HTTP helpers ---

// prefsURL builds the full URL for the preferences endpoint.
func (c *Client) prefsURL(userID string) string {
	return c.baseURL + "/api/v1/privacy/preferences/" + userID
}

// doRequest builds, authorizes, and executes a single HTTP request.
func (c *Client) doRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := c.tokenSource.Authorize(req); err != nil {
		return nil, fmt.Errorf("authorize: %w", err)
	}
	return c.httpClient.Do(req)
}

// doMutate encodes body as JSON and sends it as a POST or DELETE to the opt-out endpoint.
func (c *Client) doMutate(ctx context.Context, method string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	resp, err := c.doRequest(ctx, method, c.baseURL+"/api/v1/privacy/opt-out", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer func() { _ = drainAndClose(resp.Body) }()
	if !isSuccessStatus(resp.StatusCode) {
		return readHTTPError(resp)
	}
	return nil
}

// decodePreferences decodes a JSON Preferences value from r.
func decodePreferences(r io.Reader) (*privacy.Preferences, error) {
	var p privacy.Preferences
	if err := json.NewDecoder(r).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// readHTTPError reads the response body and returns a descriptive error.
func readHTTPError(resp *http.Response) error {
	var errResp struct {
		Error string `json:"error"`
	}
	body, _ := io.ReadAll(resp.Body)
	if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
		return fmt.Errorf("privacy: HTTP %d: %s", resp.StatusCode, errResp.Error)
	}
	return fmt.Errorf("privacy: HTTP %d", resp.StatusCode)
}

// drainAndClose discards remaining body bytes and closes the reader.
func drainAndClose(body io.ReadCloser) error {
	_, _ = io.Copy(io.Discard, body)
	return body.Close()
}

// isSuccessStatus reports whether code is in the 2xx range.
func isSuccessStatus(code int) bool {
	return code >= 200 && code < 300
}
