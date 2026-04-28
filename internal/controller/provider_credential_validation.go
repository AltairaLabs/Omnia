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

package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// CredentialValidator runs an authenticated probe against a provider
// to verify that a credential is actually accepted, not just present.
//
// Issue #1037 part 3. The CredentialConfigured condition only proves
// "we found a value in the secret"; #1037 part 1 added the
// PlaceholderCredential check for known dev-sample markers, but a
// typo, a revoked key, or a key for the wrong account still slips
// through. The validator hits the cheapest authenticated endpoint each
// provider exposes (typically a models-list call) and surfaces the
// outcome via the CredentialValid Provider status condition. Pre-flight
// surfaces "your key is wrong" instead of "the agent crashed
// mid-conversation with INVALID_API_KEY."
//
// Implementations MUST be safe to call concurrently — the controller
// reconciles Providers in parallel and may probe several at once.
type CredentialValidator interface {
	// Validate returns nil when the credential is accepted by the
	// provider. ErrCredentialInvalid wraps "the provider rejected
	// this key" (401/403/400-with-API_KEY_INVALID-shaped); other
	// errors mean we couldn't tell (network, 5xx, timeout) and the
	// caller leaves CredentialValid as Unknown rather than False —
	// false positives are worse than no signal.
	Validate(ctx context.Context, credential string) error
}

// ErrCredentialInvalid sentinels a "provider rejected this credential"
// outcome. Other errors from a Validator mean "we couldn't tell" and
// the controller leaves the condition Unknown rather than False.
var ErrCredentialInvalid = errors.New("credential rejected by provider")

// validatorTimeout caps the per-probe HTTP call. The validator runs
// during Provider reconcile so a hung provider should not block the
// reconciler indefinitely.
const validatorTimeout = 10 * time.Second

// validatorCacheTTL bounds how long a validation result is reused.
// Cache keys include the Secret resourceVersion so a key rotation
// invalidates immediately; the TTL only matters for "the provider
// changed its mind" (key revoked outside k8s, account suspended).
const validatorCacheTTL = 30 * time.Minute

// httpCredentialValidator probes a provider via an HTTP GET.
// addAuth prepares the request — different providers carry the
// credential in different headers.
type httpCredentialValidator struct {
	url     string
	addAuth func(req *http.Request, credential string)
	client  *http.Client
}

// Validate sends the probe and classifies the response. 401/403 → invalid;
// 200 → valid; everything else → "couldn't tell" so we don't trip
// CredentialValid=False on a transient 5xx.
func (v *httpCredentialValidator) Validate(ctx context.Context, credential string) error {
	if credential == "" {
		return ErrCredentialInvalid
	}
	probeCtx, cancel := context.WithTimeout(ctx, validatorTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, v.url, nil)
	if err != nil {
		return fmt.Errorf("build credential-validation request: %w", err)
	}
	v.addAuth(req, credential)

	client := v.client
	if client == nil {
		client = &http.Client{Timeout: validatorTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("credential-validation HTTP error: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return ErrCredentialInvalid
	case resp.StatusCode == http.StatusBadRequest:
		// Some providers (Gemini in particular) return 400 with an
		// API_KEY_INVALID body for bad keys instead of 401. Read a
		// bounded amount of the body and look for the marker; if
		// it's absent we treat the 400 as "something else went
		// wrong" and leave the condition Unknown.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		if containsAuthFailureMarker(body) {
			return ErrCredentialInvalid
		}
		return fmt.Errorf("credential-validation 400 from provider: %s", truncate(string(body), 200))
	default:
		return fmt.Errorf("credential-validation unexpected status %d", resp.StatusCode)
	}
}

// authFailureMarkers are body substrings that confirm a non-2xx is an
// auth failure rather than something else.
var authFailureMarkers = [][]byte{
	[]byte("API_KEY_INVALID"),
	[]byte("api key not valid"),
	[]byte("invalid_api_key"),
	[]byte("authentication_error"),
}

func containsAuthFailureMarker(body []byte) bool {
	for _, m := range authFailureMarkers {
		if containsCaseInsensitive(body, m) {
			return true
		}
	}
	return false
}

// truncate caps a string at n runes, appending "…" when cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// containsCaseInsensitive does a case-insensitive substring scan
// without allocating a fresh lowercased copy of the body — bytes.Contains
// is case-sensitive, and providers don't agree on case.
func containsCaseInsensitive(haystack, needle []byte) bool {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return len(needle) == 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			h, n := haystack[i+j], needle[j]
			if h >= 'A' && h <= 'Z' {
				h += 'a' - 'A'
			}
			if n >= 'A' && n <= 'Z' {
				n += 'a' - 'A'
			}
			if h != n {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// validatorForProvider returns a Validator for the given provider type
// or nil when validation isn't supported (mock, platform-hosted via
// cloud SDK auth, ollama with no auth, vllm self-hosted).
//
// Returning nil is the documented "skip" signal — the controller
// leaves CredentialValid as Unknown when no validator exists.
func validatorForProvider(p *omniav1alpha1.Provider, client *http.Client) CredentialValidator {
	if isPlatformHosted(p) {
		// Platform-hosted (vertex, bedrock, azure) authenticate via
		// the cloud SDK's credential chain — no API-key probe applies.
		return nil
	}
	switch p.Spec.Type {
	case omniav1alpha1.ProviderTypeOpenAI:
		return &httpCredentialValidator{
			url:     resolveProviderBaseURL(p) + "/v1/models",
			addAuth: func(req *http.Request, c string) { req.Header.Set("Authorization", "Bearer "+c) },
			client:  client,
		}
	case omniav1alpha1.ProviderTypeClaude:
		return &httpCredentialValidator{
			url: resolveProviderBaseURL(p) + "/v1/models",
			addAuth: func(req *http.Request, c string) {
				req.Header.Set("x-api-key", c)
				req.Header.Set("anthropic-version", "2023-06-01")
			},
			client: client,
		}
	case omniav1alpha1.ProviderTypeGemini:
		return &httpCredentialValidator{
			url:     resolveProviderBaseURL(p) + "/v1beta/models",
			addAuth: func(req *http.Request, c string) { req.Header.Set("x-goog-api-key", c) },
			client:  client,
		}
	default:
		return nil
	}
}

// resolveProviderBaseURL returns the base URL the validator should
// hit. Honors Provider.spec.baseURL (for proxies / OpenRouter) and
// falls back to the well-known endpoint for the type.
func resolveProviderBaseURL(p *omniav1alpha1.Provider) string {
	if p.Spec.BaseURL != "" {
		return p.Spec.BaseURL
	}
	return defaultProviderEndpoints[p.Spec.Type]
}

// credentialValidationCache memoises Validate results keyed by
// (provider namespace, provider name, secret name, secret
// resourceVersion). Entries expire after validatorCacheTTL; rotating
// the Secret invalidates immediately because the resourceVersion
// changes. Concurrent reconciles hit the same cache.
type credentialValidationCache struct {
	mu      sync.Mutex
	entries map[string]credentialValidationEntry
}

type credentialValidationEntry struct {
	err      error
	cachedAt time.Time
}

// newCredentialValidationCache constructs an empty cache. Tests
// inject their own; production wires one per ProviderReconciler.
func newCredentialValidationCache() *credentialValidationCache {
	return &credentialValidationCache{entries: map[string]credentialValidationEntry{}}
}

// get returns the cached validation result for the given key when
// it's still within the TTL window. The bool indicates a hit; the
// error mirrors what Validate returned (nil for "valid").
func (c *credentialValidationCache) get(key string) (error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Since(e.cachedAt) > validatorCacheTTL {
		delete(c.entries, key)
		return nil, false
	}
	return e.err, true
}

// put records the result for the given key with the current time.
func (c *credentialValidationCache) put(key string, result error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = credentialValidationEntry{err: result, cachedAt: time.Now()}
}

// validationCacheKey assembles the cache key for a Provider + Secret.
// Includes the Secret's resourceVersion so any modification (rotation,
// re-paste, label change) invalidates immediately.
func validationCacheKey(providerNS, providerName, secretName, secretResourceVersion string) string {
	return providerNS + "/" + providerName + "|" + secretName + "@" + secretResourceVersion
}
