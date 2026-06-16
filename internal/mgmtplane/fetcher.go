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

// Package mgmtplane provides a shared client for obtaining mgmt-plane JWTs
// from the dashboard's /api/auth/service-token endpoint. In-cluster services
// (Doctor, the Arena loadtest worker) present their kubelet-projected
// ServiceAccount token; the dashboard validates it via TokenReview, authorizes
// the caller, and mints a short-lived RS256 mgmt-plane JWT signed by the only
// private-key copy in the cluster. Callers attach the returned JWT as
// Authorization: Bearer when dialing an agent facade whose mgmt-plane validator
// is active.
package mgmtplane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// TokenSource mints a fresh mgmt-plane JWT for the supplied agent + workspace
// pair. Implementations are expected to cache when safe — callers may invoke
// this on every WS dial.
type TokenSource interface {
	Token(agent, workspace string) (string, error)
}

// TokenFetcher trades the kubelet-mounted service-account token for a fresh
// mgmt-plane JWT minted by the dashboard. Keeping the dashboard the sole minter
// means no service has to mount the signing keypair — the JWKS architecture is
// designed to keep the private key in a single pod.
//
// The fetcher caches the returned JWT until just before its expiry
// (reuseSafetyMargin) so a run reuses one signature across many WS dials without
// re-hitting the dashboard. On expiry or agent/workspace change, a fresh token
// is fetched.
type TokenFetcher struct {
	endpoint    string
	saTokenPath string
	httpClient  *http.Client
	now         func() time.Time

	mu     sync.Mutex
	cached cachedFetchedToken
}

// compile-time assertion that TokenFetcher satisfies TokenSource.
var _ TokenSource = (*TokenFetcher)(nil)

type cachedFetchedToken struct {
	token   string
	agent   string
	worksp  string
	expires time.Time
}

// FetcherOptions configures the fetcher.
type FetcherOptions struct {
	// Endpoint is the dashboard's service-token URL, e.g.
	// https://omnia-dashboard.omnia-system.svc.cluster.local/api/auth/service-token.
	// Required.
	Endpoint string
	// ServiceAccountTokenPath overrides the default kubelet mount path.
	// Tests inject a temp file; production leaves this empty.
	ServiceAccountTokenPath string
	// HTTPClient overrides the default 5-second-timeout client.
	HTTPClient *http.Client
	// AllowInsecureHTTP allows http:// endpoints (for local/dev/test only).
	// Production should keep this false so SA tokens are never sent over plaintext.
	AllowInsecureHTTP bool
}

// DefaultServiceAccountTokenPath is the kubelet-projected path inside every pod
// with a ServiceAccount mount. Always present in-cluster; missing only in tests,
// where the caller injects an alternative.
const DefaultServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// fetcherRequestTimeout caps a single dashboard call. The endpoint does a
// TokenReview round-trip + an RS256 sign, neither of which should take more than
// a couple of seconds; 5s leaves headroom for busy clusters.
const fetcherRequestTimeout = 5 * time.Second

// reuseSafetyMargin is the slack subtracted from a cached token's expiry before
// it's considered "still good to reuse". Without it the fetcher could hand out a
// token that's about to expire mid-handshake.
const reuseSafetyMargin = 30 * time.Second

// defaultMgmtPlaneTTL is the fallback TTL applied when the dashboard response
// omits expires_at — in practice it always sets it, but we don't trust the cap
// to "no expiry" if it doesn't.
const defaultMgmtPlaneTTL = 5 * time.Minute

// NewTokenFetcher builds a fetcher. Returns an error when Endpoint is empty —
// there's no useful default since service URLs vary per cluster.
//
// The fetcher does NOT make a request at construction; the first Token() call
// exercises the full path.
func NewTokenFetcher(opts FetcherOptions) (*TokenFetcher, error) {
	if opts.Endpoint == "" {
		return nil, errors.New("mgmt-plane fetcher: Endpoint required")
	}
	parsed, err := url.Parse(opts.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("mgmt-plane fetcher: invalid Endpoint %q: %w", opts.Endpoint, err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" {
		// http is acceptable only inside the trusted cluster/local network —
		// the same network on which Kubernetes itself ships projected SA
		// tokens. http to an external host would put the pod's SA token on the
		// wire in plaintext across an untrusted network, so it stays refused
		// unless the caller explicitly opts in (AllowInsecureHTTP).
		httpAllowed := scheme == "http" &&
			(opts.AllowInsecureHTTP || isClusterInternalHost(parsed.Hostname()))
		if !httpAllowed {
			return nil, fmt.Errorf(
				"mgmt-plane fetcher: insecure Endpoint scheme %q for %q; use https:// for external endpoints "+
					"(http is allowed only for cluster-internal hosts, or set AllowInsecureHTTP for local/dev)",
				parsed.Scheme,
				opts.Endpoint,
			)
		}
	}
	saTokenPath := opts.ServiceAccountTokenPath
	if saTokenPath == "" {
		saTokenPath = DefaultServiceAccountTokenPath
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: fetcherRequestTimeout}
	}
	return &TokenFetcher{
		endpoint:    opts.Endpoint,
		saTokenPath: saTokenPath,
		httpClient:  httpClient,
		now:         time.Now,
	}, nil
}

// isClusterInternalHost reports whether host is reachable only inside the
// cluster / local network, where plaintext http is acceptable for sending the
// pod's ServiceAccount token. The dashboard is served over http on a ClusterIP
// Service in every Omnia install, so requiring https for that endpoint would
// break fleet auth everywhere; this scopes the http allowance to names/IPs that
// never leave the cluster. External/public hosts return false so SA tokens are
// never sent over plaintext across an untrusted network.
//
// Recognised as internal:
//   - loopback ("localhost", 127.0.0.0/8, ::1) and link-local
//   - RFC1918 / unique-local private IPs
//   - bare single-label hostnames (e.g. "omnia-dashboard" — same-namespace svc)
//   - the ".svc" suffix (e.g. "svc.ns.svc")
//   - the ".local" suffix, which covers the conventional ".svc.cluster.local"
//     Service FQDN and ".cluster.local" cluster domain
//
// Matching uses suffixes, NOT a ".svc." substring: a substring check would
// treat an attacker-controlled "anything.svc.evil.com" as internal and leak the
// SA token. Non-".local" custom cluster domains must opt in via AllowInsecureHTTP.
func isClusterInternalHost(host string) bool {
	if host == "" {
		return false
	}
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
	}
	// Bare single-label hostname → a same-namespace Service short name.
	if !strings.Contains(h, ".") {
		return true
	}
	if strings.HasSuffix(h, ".local") {
		return true
	}
	return strings.HasSuffix(h, ".svc")
}

// Token implements TokenSource. Returns a cached token when the cache is bound
// to the same (agent, workspace) and still has >= reuseSafetyMargin until
// expiry; otherwise fetches a fresh one.
func (f *TokenFetcher) Token(agent, workspace string) (string, error) {
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
		// Read up to 1 KB of body so the operator sees the dashboard's reason
		// text (allowlist mismatch, TokenReview failure, etc.) in logs.
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
		// Defensive default: dashboard should always set expires_at, but a
		// missing value should still cap the cache lifetime.
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
