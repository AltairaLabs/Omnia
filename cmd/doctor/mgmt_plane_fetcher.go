/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// MgmtPlaneTokenFetcher trades the kubelet-mounted service-account
// token for a fresh mgmt-plane JWT minted by the dashboard. Replaces
// the previous local minter, which had to mount the dashboard's
// signing keypair into Doctor — duplicating private key material that
// the JWKS architecture is designed to keep in a single pod.
//
// The fetcher caches the returned JWT until just before its expiry
// (reuseSafetyMargin) so a single Doctor run reuses one signature
// across ~6 WS dials without re-hitting the dashboard. On expiry or
// agent/workspace change, a fresh token is fetched.
//
// Implements MgmtPlaneTokenSource so AgentChecker keeps the same
// wiring shape — only the constructor changes at the cmd/doctor
// boot site.
type MgmtPlaneTokenFetcher struct {
	endpoint    string
	saTokenPath string
	httpClient  *http.Client
	now         func() time.Time

	mu     sync.Mutex
	cached cachedFetchedToken
}

type cachedFetchedToken struct {
	token   string
	agent   string
	worksp  string
	expires time.Time
}

// MgmtPlaneTokenFetcherOptions configures the fetcher.
type MgmtPlaneTokenFetcherOptions struct {
	// Endpoint is the dashboard's service-token URL, e.g.
	// http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/service-token.
	// Required.
	Endpoint string
	// ServiceAccountTokenPath overrides the default kubelet mount
	// path. Tests inject a temp file; production leaves this empty.
	ServiceAccountTokenPath string
	// HTTPClient overrides the default 5-second-timeout client.
	HTTPClient *http.Client
}

// defaultServiceAccountTokenPath is the kubelet-projected path inside
// every pod with a ServiceAccount mount. Always present in-cluster;
// missing only in tests, where the caller injects an alternative.
const defaultServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// fetcherRequestTimeout caps a single dashboard call. The endpoint
// does a TokenReview round-trip + an RS256 sign, neither of which
// should take more than a couple of seconds; 5s leaves headroom for
// busy clusters without holding up Doctor's per-test timeouts.
const fetcherRequestTimeout = 5 * time.Second

// reuseSafetyMargin is the slack subtracted from a cached token's
// expiry before it's considered "still good to reuse". Without it the
// fetcher could hand out a token that's about to expire mid-handshake.
const reuseSafetyMargin = 30 * time.Second

// defaultMgmtPlaneTTL is the fallback TTL applied when the dashboard
// response omits expires_at — in practice it always sets it, but we
// don't trust the cap to "no expiry" if it doesn't.
const defaultMgmtPlaneTTL = 5 * time.Minute

// NewMgmtPlaneTokenFetcher builds a fetcher. Returns an error when
// Endpoint is empty — there's no useful default since service URLs
// vary per cluster.
//
// The fetcher does NOT make a request at construction; the first
// Token() call exercises the full path. Boot-time validation of the
// dashboard reachability happens via the operator's existing health
// checks — failing here would conflate "no dashboard" (which is OK
// for installs without service principals) with "Doctor misconfigured"
// (which is the case we want loud).
func NewMgmtPlaneTokenFetcher(opts MgmtPlaneTokenFetcherOptions) (*MgmtPlaneTokenFetcher, error) {
	if opts.Endpoint == "" {
		return nil, errors.New("mgmt-plane fetcher: Endpoint required")
	}
	saTokenPath := opts.ServiceAccountTokenPath
	if saTokenPath == "" {
		saTokenPath = defaultServiceAccountTokenPath
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: fetcherRequestTimeout}
	}
	return &MgmtPlaneTokenFetcher{
		endpoint:    opts.Endpoint,
		saTokenPath: saTokenPath,
		httpClient:  httpClient,
		now:         time.Now,
	}, nil
}

// Token implements MgmtPlaneTokenSource. Returns a cached token when
// the cache is bound to the same (agent, workspace) and still has
// >= reuseSafetyMargin until expiry; otherwise fetches a fresh one.
func (f *MgmtPlaneTokenFetcher) Token(agent, workspace string) (string, error) {
	if f == nil {
		return "", errors.New("mgmt-plane fetcher: nil receiver")
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.now()
	if f.cached.token != "" &&
		f.cached.agent == agent &&
		f.cached.worksp == workspace &&
		now.Add(reuseSafetyMargin).Before(f.cached.expires) {
		return f.cached.token, nil
	}

	saToken, err := os.ReadFile(f.saTokenPath)
	if err != nil {
		return "", fmt.Errorf("mgmt-plane fetcher: read SA token %q: %w", f.saTokenPath, err)
	}

	body, _ := json.Marshal(map[string]string{
		"agent":     agent,
		"workspace": workspace,
	})

	ctx, cancel := context.WithTimeout(context.Background(), fetcherRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("mgmt-plane fetcher: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+string(bytes.TrimSpace(saToken)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("mgmt-plane fetcher: POST %s: %w", f.endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Read up to 1 KB of body so the operator sees the dashboard's
		// reason text (allowlist mismatch, TokenReview failure, etc.)
		// in Doctor's logs without grepping the dashboard pod.
		excerpt, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf(
			"mgmt-plane fetcher: dashboard returned %d: %s",
			resp.StatusCode,
			bytes.TrimSpace(excerpt),
		)
	}

	var out struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("mgmt-plane fetcher: decode response: %w", err)
	}
	if out.Token == "" {
		return "", errors.New("mgmt-plane fetcher: dashboard returned empty token")
	}

	expires := time.Unix(out.ExpiresAt, 0)
	if out.ExpiresAt <= 0 {
		// Defensive default: dashboard should always set expires_at,
		// but a missing value should still cap the cache lifetime so
		// we don't reuse the token forever.
		expires = now.Add(defaultMgmtPlaneTTL)
	}
	f.cached = cachedFetchedToken{
		token:   out.Token,
		agent:   agent,
		worksp:  workspace,
		expires: expires,
	}
	return out.Token, nil
}
