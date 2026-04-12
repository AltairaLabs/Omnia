# SessionPrivacyPolicy Recording Control & Encryption Implementation Plan (REVISED)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce SessionPrivacyPolicy recording flags at the facade and session-api layers, and wire the **existing** `ee/pkg/encryption/` package into session-api so session data (messages, tool call arguments/results, runtime event data) is encrypted at rest.

**Architecture:** Defense-in-depth recording control (facade skips early, session-api rejects as safety net). For encryption, reuse the existing `Encryptor`/`Provider` infrastructure in `ee/pkg/encryption/` — the AWS KMS, Azure Key Vault, GCP KMS, and Vault providers are already built and tested. This plan only adds the missing **wiring** from the SessionPrivacyPolicy CRD into session-api, and extends encryption to cover `ToolCall` and `RuntimeEvent` records.

**Spec:** `docs/superpowers/specs/2026-04-12-session-privacy-recording-encryption-design.md`

## Why this revision

The original plan was drafted without discovering that `ee/pkg/encryption/` already contains:
- `Encryptor` interface with `EncryptMessage`/`DecryptMessage` on `*session.Message`
- `Provider` interface and four KMS implementations (AWS, Azure, GCP, Vault)
- `MessageReEncryptor` for key rotation
- Postgres `ReEncryptionStore` implementation
- `KeyRotationReconciler` already running in arena-controller

This revision drops the "local AES key in Secret" approach and the new `Encryptor`/`EncryptingStore` abstractions. Instead it:

1. Extends `PolicyWatcher.EffectivePolicy` to carry the `Encryption` config
2. Wires session-api to build an `encryption.Provider` + `encryption.Encryptor` from the effective policy
3. Extends encryption to cover `ToolCall` and `RuntimeEvent` (Messages already work)
4. Wires the encryption in the session-api handler layer
5. Populates `KeyRotationReconciler.StoreFactory` in arena-controller

---

## Task 1: httpclient GetPrivacyPolicy Method

**Files:**
- Modify: `internal/session/httpclient/store.go`
- Modify: `internal/session/httpclient/store_test.go`

### Tests first

- [ ] **Step 1: Write the failing test**

Add to `internal/session/httpclient/store_test.go`:

```go
func TestStore_GetPrivacyPolicy_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/privacy-policy", r.URL.Path)
		assert.Equal(t, "default", r.URL.Query().Get("namespace"))
		assert.Equal(t, "my-agent", r.URL.Query().Get("agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"recording":{"enabled":true,"facadeData":true,"richData":false}}`))
	}))
	defer srv.Close()

	store := httpclient.NewStore(srv.URL, logr.Discard())
	defer func() { _ = store.Close() }()

	policy, err := store.GetPrivacyPolicy(context.Background(), "default", "my-agent")
	require.NoError(t, err)
	require.NotNil(t, policy)
	assert.True(t, policy.Recording.Enabled)
	assert.False(t, policy.Recording.RichData)
}

func TestStore_GetPrivacyPolicy_NoPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	store := httpclient.NewStore(srv.URL, logr.Discard())
	defer func() { _ = store.Close() }()

	policy, err := store.GetPrivacyPolicy(context.Background(), "default", "my-agent")
	require.NoError(t, err)
	assert.Nil(t, policy)
}

func TestStore_GetPrivacyPolicy_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	store := httpclient.NewStore(srv.URL, logr.Discard())
	defer func() { _ = store.Close() }()

	_, err := store.GetPrivacyPolicy(context.Background(), "default", "my-agent")
	assert.Error(t, err)
}
```

The response type should be a JSON-only representation to avoid the httpclient importing `ee/`:

```go
// PrivacyPolicyResponse is the shape of GET /api/v1/privacy-policy.
type PrivacyPolicyResponse struct {
	Recording struct {
		Enabled    bool `json:"enabled"`
		FacadeData bool `json:"facadeData"`
		RichData   bool `json:"richData"`
	} `json:"recording"`
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `env GOWORK=off go test ./internal/session/httpclient/... -count=1 -v -run TestStore_GetPrivacyPolicy`
Expected: compilation failure — `GetPrivacyPolicy` doesn't exist.

### Implementation

- [ ] **Step 3: Add GetPrivacyPolicy to the Store**

```go
// PrivacyPolicyResponse is the minimal shape of GET /api/v1/privacy-policy
// that the facade needs for recording decisions.
type PrivacyPolicyResponse struct {
	Recording struct {
		Enabled    bool `json:"enabled"`
		FacadeData bool `json:"facadeData"`
		RichData   bool `json:"richData"`
	} `json:"recording"`
}

// GetPrivacyPolicy fetches the effective privacy policy for a namespace/agent pair.
// Returns nil with no error if no policy applies (204 response).
// Bypasses the circuit breaker — this is a config read, not a session write.
func (s *Store) GetPrivacyPolicy(ctx context.Context, namespace, agent string) (*PrivacyPolicyResponse, error) {
	q := neturl.Values{}
	q.Set("namespace", namespace)
	q.Set("agent", agent)
	url := s.baseURL + "/api/v1/privacy-policy?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create privacy policy request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("privacy policy request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("privacy policy: unexpected status %d", resp.StatusCode)
	}

	var policy PrivacyPolicyResponse
	if err := json.NewDecoder(resp.Body).Decode(&policy); err != nil {
		return nil, fmt.Errorf("decode privacy policy: %w", err)
	}
	return &policy, nil
}
```

Use existing import alias `neturl "net/url"` that's already in the file (check existing imports).

- [ ] **Step 4: Run tests**

Run: `env GOWORK=off go test ./internal/session/httpclient/... -count=1 -v -run TestStore_GetPrivacyPolicy`
Expected: all 3 tests PASS.

- [ ] **Step 5: goimports**

Run: `goimports -w internal/session/httpclient/store.go`

- [ ] **Step 6: Commit**

```
feat(httpclient): add GetPrivacyPolicy method to session-api client

Ref #780
```

---

## Task 2: Privacy Policy Endpoint on Session-API

**Files:**
- Modify: `internal/session/api/handler.go`
- Modify: `internal/session/api/handler_test.go`

### Tests first

- [ ] **Step 1: Write the failing test**

```go
func TestHandleGetPrivacyPolicy_ReturnsEffective(t *testing.T) {
	resolver := api.PolicyResolverFunc(func(namespace, agent string) (json.RawMessage, bool) {
		return json.RawMessage(`{"recording":{"enabled":true,"facadeData":true,"richData":false}}`), true
	})

	handler := api.NewHandler(nil, logr.Discard(), api.DefaultMaxBodySize)
	handler.SetPolicyResolver(resolver)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"enabled":true`)
	assert.Contains(t, rec.Body.String(), `"richData":false`)
}

func TestHandleGetPrivacyPolicy_NoPolicyReturns204(t *testing.T) {
	resolver := api.PolicyResolverFunc(func(namespace, agent string) (json.RawMessage, bool) {
		return nil, false
	})

	handler := api.NewHandler(nil, logr.Discard(), api.DefaultMaxBodySize)
	handler.SetPolicyResolver(resolver)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleGetPrivacyPolicy_NoResolverReturns204(t *testing.T) {
	handler := api.NewHandler(nil, logr.Discard(), api.DefaultMaxBodySize)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `env GOWORK=off go test ./internal/session/api/... -count=1 -v -run TestHandleGetPrivacyPolicy`
Expected: compilation failure.

### Implementation

- [ ] **Step 3: Add PolicyResolver interface to handler.go**

```go
// PolicyResolver returns the effective privacy policy JSON for a namespace/agent pair.
// Returns (policyJSON, true) when a policy applies, or (nil, false) when none applies.
// Using json.RawMessage keeps this package unaware of ee/ types.
type PolicyResolver interface {
	ResolveEffectivePolicy(namespace, agentName string) (json.RawMessage, bool)
}

// PolicyResolverFunc adapts a function to the PolicyResolver interface.
type PolicyResolverFunc func(namespace, agentName string) (json.RawMessage, bool)

func (f PolicyResolverFunc) ResolveEffectivePolicy(namespace, agentName string) (json.RawMessage, bool) {
	return f(namespace, agentName)
}
```

Add `policyResolver PolicyResolver` field to `Handler`, a `SetPolicyResolver` setter, a `handleGetPrivacyPolicy` method that writes the JSON or 204, and register the route:

```go
mux.HandleFunc("GET /api/v1/privacy-policy", h.handleGetPrivacyPolicy)
```

- [ ] **Step 4: Run tests**

Run: `env GOWORK=off go test ./internal/session/api/... -count=1 -v -run TestHandleGetPrivacyPolicy`
Expected: all 3 tests PASS.

- [ ] **Step 5: goimports + commit**

```
feat(api): add GET /api/v1/privacy-policy endpoint

Ref #780
```

---

## Task 3: Extend EffectivePolicy with Encryption

**Files:**
- Modify: `ee/pkg/privacy/watcher.go`
- Modify: `ee/pkg/privacy/watcher_test.go`
- Verify: `ee/pkg/privacy/merge.go` already merges `Encryption` (it does — see `MergeEncryption` function)

### Tests first

- [ ] **Step 1: Write failing test**

```go
func TestEffectivePolicy_IncludesEncryption(t *testing.T) {
	w := &PolicyWatcher{}
	policy := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "global", Namespace: "omnia-system"},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelGlobal,
			Recording: omniav1alpha1.RecordingConfig{Enabled: true},
			Encryption: omniav1alpha1.EncryptionConfig{
				Enabled:     true,
				KMSProvider: omniav1alpha1.KMSProviderAWS,
				KeyID:       "arn:aws:kms:us-east-1:123:key/test",
			},
		},
	}
	w.policies.Store("omnia-system/global", policy)

	eff := w.GetEffectivePolicy("default", "my-agent")
	require.NotNil(t, eff)
	assert.True(t, eff.Encryption.Enabled)
	assert.Equal(t, "arn:aws:kms:us-east-1:123:key/test", eff.Encryption.KeyID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Expected: `EffectivePolicy.Encryption` doesn't exist.

### Implementation

- [ ] **Step 3: Add Encryption field**

```go
type EffectivePolicy struct {
	Recording  omniav1alpha1.RecordingConfig
	UserOptOut *omniav1alpha1.UserOptOutConfig
	Encryption omniav1alpha1.EncryptionConfig
}
```

Update `GetEffectivePolicy` (inspect its current implementation) to populate `Encryption` from the merged spec. If the merge helper already computes encryption, this is a one-line assignment.

- [ ] **Step 4: Run tests + goimports + commit**

```
feat(privacy): expose encryption config in EffectivePolicy

Ref #780
```

---

## Task 4: PolicyResolver Adapter on PolicyWatcher

**Files:**
- Create: `ee/pkg/privacy/resolver.go`
- Create: `ee/pkg/privacy/resolver_test.go`

### Tests first

- [ ] **Step 1: Write failing tests**

```go
func TestFacadePolicyJSON_OnlyIncludesFacadeFields(t *testing.T) {
	eff := &EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   false,
		},
		Encryption: omniav1alpha1.EncryptionConfig{
			Enabled: true,
			KeyID:   "secret-key-id",
		},
	}

	raw, err := facadePolicyJSON(eff)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	recording := decoded["recording"].(map[string]any)
	assert.Equal(t, true, recording["enabled"])
	assert.Equal(t, false, recording["richData"])

	_, hasEncryption := decoded["encryption"]
	assert.False(t, hasEncryption, "encryption config must not leak to facade")
}

func TestResolveEffectivePolicy_NilWhenNoPolicy(t *testing.T) {
	w := &PolicyWatcher{}
	raw, ok := w.ResolveEffectivePolicy("default", "my-agent")
	assert.False(t, ok)
	assert.Nil(t, raw)
}
```

### Implementation

- [ ] **Step 2: Write resolver.go**

```go
package privacy

import (
	"encoding/json"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// facadePolicy is the subset of EffectivePolicy exposed to the facade via
// GET /api/v1/privacy-policy. Encryption config stays server-side.
type facadePolicy struct {
	Recording omniav1alpha1.RecordingConfig `json:"recording"`
}

func facadePolicyJSON(p *EffectivePolicy) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	return json.Marshal(facadePolicy{Recording: p.Recording})
}

// ResolveEffectivePolicy adapts PolicyWatcher to api.PolicyResolver.
func (w *PolicyWatcher) ResolveEffectivePolicy(namespace, agentName string) (json.RawMessage, bool) {
	eff := w.GetEffectivePolicy(namespace, agentName)
	if eff == nil {
		return nil, false
	}
	raw, err := facadePolicyJSON(eff)
	if err != nil || len(raw) == 0 {
		return nil, false
	}
	return raw, true
}
```

- [ ] **Step 3: Run tests + goimports + commit**

```
feat(privacy): add PolicyResolver adapter for session-api handler

Ref #780
```

---

## Task 5: Facade Recording Policy Cache + Gate

**Files:**
- Create: `internal/facade/recording_policy.go`
- Create: `internal/facade/recording_policy_test.go`
- Modify: `internal/facade/recording_writer.go`
- Modify: `internal/facade/recording_writer_test.go`
- Modify: `internal/facade/session.go`, `internal/facade/connection.go`, `internal/facade/server.go`

### Tests first

- [ ] **Step 1: Write failing tests for policy cache**

Create `internal/facade/recording_policy_test.go` — see plan for full test code (TestRecordingPolicyCache_FetchesOnFirstCall, _ReturnsCachedWithinTTL, _FetchError_DefaultsToRecordingEnabled, _NilPolicy_DefaultsToRecordingEnabled). Use `httpclient.PrivacyPolicyResponse` as the cached type.

- [ ] **Step 2: Write failing tests for recording gate**

Add to `internal/facade/recording_writer_test.go`:

```go
func TestRecordingWriter_RecordingDisabled_SkipsAll(t *testing.T) {
	store := &mockRecordingStore{} // or existing helper
	policy := &httpclient.PrivacyPolicyResponse{}
	policy.Recording.Enabled = false

	writer := newRecordingWriter(context.Background(), &mockInnerWriter{}, store, "sess-1", logr.Discard(), nil)
	writer.setPolicy(policy)

	_ = writer.WriteDone("hello")
	_ = writer.WriteToolCall(&ToolCallInfo{ID: "tc-1", Name: "search", Arguments: map[string]any{"q": "test"}})
	_ = writer.WriteToolResult(&ToolResultInfo{ID: "tc-1", Result: "found it"})

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.messages)
}

func TestRecordingWriter_RichDataDisabled_SkipsContent(t *testing.T) {
	store := &mockRecordingStore{}
	policy := &httpclient.PrivacyPolicyResponse{}
	policy.Recording.Enabled = true
	policy.Recording.FacadeData = true
	policy.Recording.RichData = false

	writer := newRecordingWriter(context.Background(), &mockInnerWriter{}, store, "sess-1", logr.Discard(), nil)
	writer.setPolicy(policy)

	_ = writer.WriteDone("assistant response")
	_ = writer.WriteToolCall(&ToolCallInfo{ID: "tc-1", Name: "search", Arguments: map[string]any{"q": "test"}})
	_ = writer.WriteToolResult(&ToolResultInfo{ID: "tc-1", Result: "result"})

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.messages)
}

func TestRecordingWriter_DefaultPolicy_RecordsEverything(t *testing.T) {
	store := &mockRecordingStore{}
	writer := newRecordingWriter(context.Background(), &mockInnerWriter{}, store, "sess-1", logr.Discard(), nil)
	// no setPolicy — defaults to enabled

	_ = writer.WriteDone("hello")

	time.Sleep(50 * time.Millisecond)
	assert.Len(t, store.messages, 1)
}
```

Inspect the existing test file for existing mocks before writing new ones; reuse what exists.

- [ ] **Step 3: Run tests to verify they fail**

Expected: compilation failures.

### Implementation

- [ ] **Step 4: Write recording_policy.go**

```go
package facade

import (
	"context"
	"sync"
	"time"

	"github.com/altairalabs/omnia/internal/session/httpclient"
)

type PolicyFetcher interface {
	GetPrivacyPolicy(ctx context.Context, namespace, agent string) (*httpclient.PrivacyPolicyResponse, error)
}

func defaultPolicy() *httpclient.PrivacyPolicyResponse {
	p := &httpclient.PrivacyPolicyResponse{}
	p.Recording.Enabled = true
	p.Recording.FacadeData = true
	p.Recording.RichData = true
	return p
}

type RecordingPolicyCache struct {
	fetcher   PolicyFetcher
	namespace string
	agent     string
	ttl       time.Duration

	mu        sync.Mutex
	cached    *httpclient.PrivacyPolicyResponse
	fetchedAt time.Time
}

func NewRecordingPolicyCache(fetcher PolicyFetcher, namespace, agent string, ttl time.Duration) *RecordingPolicyCache {
	return &RecordingPolicyCache{fetcher: fetcher, namespace: namespace, agent: agent, ttl: ttl}
}

func (c *RecordingPolicyCache) Get(ctx context.Context) *httpclient.PrivacyPolicyResponse {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && time.Since(c.fetchedAt) < c.ttl {
		return c.cached
	}

	policy, err := c.fetcher.GetPrivacyPolicy(ctx, c.namespace, c.agent)
	if err != nil || policy == nil {
		c.cached = defaultPolicy()
	} else {
		c.cached = policy
	}
	c.fetchedAt = time.Now()
	return c.cached
}
```

- [ ] **Step 5: Add recording gate to recording_writer.go**

Add `policy *httpclient.PrivacyPolicyResponse` field to `recordingResponseWriter`, a `setPolicy` setter, and helpers:

```go
func (w *recordingResponseWriter) shouldRecord() bool {
	return w.policy == nil || w.policy.Recording.Enabled
}

func (w *recordingResponseWriter) shouldRecordRichData() bool {
	return w.policy == nil || w.policy.Recording.RichData
}
```

Gate each method:
- `WriteDone` / `WriteDoneWithParts`: call `recordDone` only when `shouldRecord() && shouldRecordRichData()`
- `WriteToolCall` / `WriteToolResult`: skip the `w.submit(...)` call when `!shouldRecord() || !shouldRecordRichData()`
- `WriteError`: skip `w.submit(...)` when `!shouldRecord()` (errors record regardless of RichData)

- [ ] **Step 6: Wire policy into Connection + session.go**

Add to `Connection`:
```go
policyCache *RecordingPolicyCache
```

Add a `PolicyFetcher` field on `Server` (or reuse the `session.Store` via type assertion). In `handleConnection`, once namespace/agent are known, create the cache. In `session.go` around line 195, set the policy on `recWriter`:

```go
recWriter := newRecordingWriter(ctx, writer, s.sessionStore, sessionID, log, s.recordingPool)
if c.policyCache != nil {
	recWriter.setPolicy(c.policyCache.Get(ctx))
}
```

The implementer should inspect `internal/facade/server.go` to determine the cleanest place to hold the `PolicyFetcher` (likely the `Server` struct, accepting it via `ServerConfig` or a new `WithPolicyFetcher` option). The `*httpclient.Store` that the facade already uses for `session.Store` operations has `GetPrivacyPolicy` and thus satisfies `PolicyFetcher`.

- [ ] **Step 7: Run tests + goimports + commit**

```
feat(facade): add recording policy cache and gate

Ref #780
```

---

## Task 6: Privacy Middleware Recording Flag Enforcement

**Files:**
- Modify: `ee/pkg/privacy/middleware.go`
- Modify: `ee/pkg/privacy/middleware_test.go`

### Tests first

- [ ] **Step 1: Write failing tests**

Add tests: `TestPrivacyMiddleware_RecordingDisabled_Returns204`, `_RichDataDisabled_DropsAssistantMessage`, `_RichDataDisabled_AllowsUserMessage`, `_RichDataDisabled_BlocksToolCallEndpoint`, `_RichDataDisabled_AllowsStatusUpdate`. Use existing `newMockWatcher`, `newMockSessionCache`, `newMockPrefStore` helpers.

### Implementation

- [ ] **Step 2: Add checkRecordingPolicy to middleware.go**

Add regexes and helpers:

```go
var (
	messageEndpointRe  = regexp.MustCompile(`/api/v1/sessions/[^/]+/messages$`)
	toolCallEndpointRe = regexp.MustCompile(`/api/v1/sessions/[^/]+/tool-calls$`)
	runtimeEventRe     = regexp.MustCompile(`/api/v1/sessions/[^/]+/events$`)
	providerCallRe     = regexp.MustCompile(`/api/v1/sessions/[^/]+/provider-calls$`)
)

func isRichDataEndpoint(path string) bool {
	return toolCallEndpointRe.MatchString(path) ||
		runtimeEventRe.MatchString(path) ||
		providerCallRe.MatchString(path)
}

// checkRecordingPolicy returns true if the request was blocked.
func (m *PrivacyMiddleware) checkRecordingPolicy(
	w http.ResponseWriter, r *http.Request, policy *EffectivePolicy,
) bool {
	if !policy.Recording.Enabled {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	if policy.Recording.RichData {
		return false
	}
	if isRichDataEndpoint(r.URL.Path) {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	if messageEndpointRe.MatchString(r.URL.Path) && isRichMessage(r) {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

func isRichMessage(r *http.Request) bool {
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return false
	}
	var msg struct {
		Role     string            `json:"role"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return false
	}
	if msg.Role == "user" {
		return false
	}
	if msg.Role == "assistant" || msg.Role == "system" {
		return true
	}
	t := msg.Metadata["type"]
	return t == "tool_call" || t == "tool_result"
}
```

Wire into `Wrap()` between policy fetch (line 89) and opt-out check (line 97):

```go
if m.checkRecordingPolicy(w, r, policy) {
	return
}
```

- [ ] **Step 3: Run tests + goimports + commit**

```
feat(ee): enforce recording flags in privacy middleware

Ref #780
```

---

## Task 7: Extend Encryptor for ToolCall and RuntimeEvent

**Files:**
- Modify: `ee/pkg/encryption/encryptor.go`
- Modify: `ee/pkg/encryption/encryption_test.go`

The existing `Encryptor` interface only handles `*session.Message`. Extend it to cover `*session.ToolCall` and `*session.RuntimeEvent`.

### Storage strategy

**ToolCall:**
- `Name` → plaintext
- `Arguments` (`map[string]any`) → JSON-marshal, encrypt, wrap in envelope map: `{"_encryption": {...meta...}, "_payload": "base64-ciphertext"}`. Stored back into `Arguments` field.
- `Result` (`any`) → same envelope pattern in `Result`
- `ErrorMessage` (`string`) → encrypted base64 string with a sentinel prefix, stored back in `ErrorMessage`
- On read: detect the `_encryption` sentinel, reverse the process

**RuntimeEvent:**
- `EventType` → plaintext
- `Data` (`map[string]any`) → envelope pattern in `Data`
- `ErrorMessage` → sentinel-prefixed ciphertext

No schema changes — JSONB columns hold the envelope maps fine.

### Tests first

- [ ] **Step 1: Write failing tests**

```go
func TestEncryptor_EncryptToolCall_EncryptsArgumentsAndResult(t *testing.T) {
	provider := newMockProvider(t)
	enc := NewEncryptor(provider)

	tc := &session.ToolCall{
		ID:           "tc-1",
		Name:         "web_search",
		Arguments:    map[string]any{"query": "sensitive search"},
		Result:       "sensitive result",
		ErrorMessage: "",
	}

	encrypted, events, err := enc.EncryptToolCall(context.Background(), tc)
	require.NoError(t, err)

	assert.Equal(t, "web_search", encrypted.Name)
	assert.NotEqual(t, tc.Arguments, encrypted.Arguments)
	assert.NotEqual(t, tc.Result, encrypted.Result)

	eventFieldSet := map[string]bool{}
	for _, e := range events {
		eventFieldSet[e.Field] = true
	}
	assert.True(t, eventFieldSet["arguments"])
	assert.True(t, eventFieldSet["result"])
}

func TestEncryptor_DecryptToolCall_RoundTrip(t *testing.T) {
	provider := newMockProvider(t)
	enc := NewEncryptor(provider)

	original := &session.ToolCall{
		ID:           "tc-1",
		Name:         "web_search",
		Arguments:    map[string]any{"query": "test"},
		Result:       "found",
		ErrorMessage: "no error",
	}

	encrypted, _, err := enc.EncryptToolCall(context.Background(), original)
	require.NoError(t, err)

	decrypted, err := enc.DecryptToolCall(context.Background(), encrypted)
	require.NoError(t, err)

	assert.Equal(t, original.Name, decrypted.Name)
	assert.Equal(t, original.Arguments, decrypted.Arguments)
	assert.Equal(t, original.Result, decrypted.Result)
	assert.Equal(t, original.ErrorMessage, decrypted.ErrorMessage)
}

func TestEncryptor_EncryptRuntimeEvent_EncryptsData(t *testing.T) {
	provider := newMockProvider(t)
	enc := NewEncryptor(provider)

	evt := &session.RuntimeEvent{
		ID:           "evt-1",
		EventType:    "pipeline.completed",
		Data:         map[string]any{"output": "sensitive"},
		ErrorMessage: "sensitive error",
	}

	encrypted, _, err := enc.EncryptRuntimeEvent(context.Background(), evt)
	require.NoError(t, err)
	assert.Equal(t, "pipeline.completed", encrypted.EventType)
	assert.NotEqual(t, evt.Data, encrypted.Data)
}

func TestEncryptor_DecryptRuntimeEvent_RoundTrip(t *testing.T) {
	provider := newMockProvider(t)
	enc := NewEncryptor(provider)

	original := &session.RuntimeEvent{
		ID:           "evt-1",
		EventType:    "pipeline.completed",
		Data:         map[string]any{"output": "hello"},
		ErrorMessage: "fail",
	}

	encrypted, _, err := enc.EncryptRuntimeEvent(context.Background(), original)
	require.NoError(t, err)

	decrypted, err := enc.DecryptRuntimeEvent(context.Background(), encrypted)
	require.NoError(t, err)

	assert.Equal(t, original.EventType, decrypted.EventType)
	assert.Equal(t, original.Data, decrypted.Data)
	assert.Equal(t, original.ErrorMessage, decrypted.ErrorMessage)
}
```

### Implementation

- [ ] **Step 2: Extend the Encryptor interface**

```go
type Encryptor interface {
	EncryptMessage(ctx context.Context, msg *session.Message) (*session.Message, []EncryptionEvent, error)
	DecryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error)

	EncryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, []EncryptionEvent, error)
	DecryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error)

	EncryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, []EncryptionEvent, error)
	DecryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error)
}
```

Use envelope helpers to avoid duplication. Example:

```go
// encryptEnvelope JSON-marshals v, encrypts the bytes, and returns a map:
// {"_encryption": {metadata}, "_payload": "base64-ciphertext"}
func (e *encryptor) encryptEnvelope(ctx context.Context, v any) (map[string]any, *EncryptOutput, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, nil, err
	}
	out, err := e.provider.Encrypt(ctx, data)
	if err != nil {
		return nil, nil, err
	}
	return map[string]any{
		"_encryption": map[string]any{
			"keyID":      out.KeyID,
			"keyVersion": out.KeyVersion,
			"algorithm":  out.Algorithm,
		},
		"_payload": base64.StdEncoding.EncodeToString(out.Ciphertext),
	}, out, nil
}

// decryptEnvelope reverses encryptEnvelope.
func (e *encryptor) decryptEnvelope(ctx context.Context, m map[string]any, into any) error {
	payload, ok := m["_payload"].(string)
	if !ok {
		return fmt.Errorf("envelope missing _payload")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return err
	}
	plaintext, err := e.provider.Decrypt(ctx, ciphertext)
	if err != nil {
		return err
	}
	return json.Unmarshal(plaintext, into)
}

// isEncrypted returns true if m is an envelope (has _encryption key).
func isEncryptedEnvelope(m map[string]any) bool {
	_, ok := m["_encryption"]
	return ok
}
```

For `ErrorMessage` (a plain string), use a distinct sentinel-prefixed base64 format (e.g., `enc:v1:` + base64) — or wrap as JSON envelope and store as string. Pick one approach consistently.

Keep each new function below cognitive complexity 15 (SonarCloud).

- [ ] **Step 3: Run tests + goimports + commit**

```
feat(ee): extend Encryptor to cover ToolCall and RuntimeEvent

Ref #780
```

---

## Task 8: Wire Encryption into Session-API Handlers

**Files:**
- Create: `internal/session/api/encryption_adapter.go` (interface)
- Modify: `internal/session/api/handler.go` (plumb encryptor)
- Modify: `internal/session/api/handler_test.go`
- Modify: `cmd/session-api/main.go` (build and wire encryptor)

### Design

`internal/session/api/` must not import `ee/`. Define a minimal `Encryptor` interface in the api package that `ee/pkg/encryption.Encryptor` satisfies via an adapter.

### Tests first

- [ ] **Step 1: Write failing tests**

Tests should:
1. Verify `handleAppendMessage` calls `encryptor.EncryptMessage` when set
2. Verify `handleGetMessages` decrypts messages before returning
3. Verify non-encrypted path works when encryptor is nil (non-enterprise)
4. Same for ToolCall and RuntimeEvent endpoints

Use existing handler test helpers. A mock encryptor that records calls is enough.

### Implementation

- [ ] **Step 2: Define api.Encryptor interface**

Create `internal/session/api/encryption_adapter.go`:

```go
package api

import (
	"context"

	"github.com/altairalabs/omnia/internal/session"
)

// Encryptor is the subset of encryption operations session-api requires.
// ee/pkg/encryption.Encryptor satisfies this interface (via an adapter in
// cmd/session-api that drops the []EncryptionEvent return).
// When nil, session-api reads/writes plaintext (non-enterprise mode).
type Encryptor interface {
	EncryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error)
	DecryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error)

	EncryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error)
	DecryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error)

	EncryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error)
	DecryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error)
}
```

- [ ] **Step 3: Plumb encryptor into Handler**

Add `encryptor Encryptor` field to `Handler`, a `SetEncryptor(e Encryptor)` method. Modify:
- `handleAppendMessage`: if `h.encryptor != nil`, call `EncryptMessage` before forwarding to `service.AppendMessage`
- `handleGetMessages`: if `h.encryptor != nil`, decrypt each returned message (swallow decrypt errors? or return 500? — log + return 500 is safer)
- `handleRecordToolCall` / `handleGetToolCalls`: same pattern
- `handleRecordRuntimeEvent` / `handleGetRuntimeEvents`: same pattern

- [ ] **Step 4: Build encryptor in cmd/session-api/main.go**

In `buildAPIMux`, after the PolicyWatcher is set up (around line 498), check for encryption and build the encryptor:

```go
if f.enterprise && watcher != nil {
	if enc, err := buildEncryptorFromPolicy(context.Background(), mgr, watcher, log); err != nil {
		log.Error(err, "encryption disabled", "reason", "initialization failed")
	} else if enc != nil {
		handler.SetEncryptor(enc)
		log.Info("session encryption enabled")
	}
}
```

**Refactor shared helper:** Factor the `buildProviderConfig` + `loadSecretCredentials` logic from `ee/internal/controller/keyrotation_controller.go:385-425` into a shared helper in `ee/pkg/encryption/`:

```go
// In ee/pkg/encryption/config_loader.go:
func ProviderConfigFromEncryptionSpec(
	ctx context.Context, c client.Client, namespace string, enc omniav1alpha1.EncryptionConfig,
) (ProviderConfig, error) {
	cfg := ProviderConfig{
		ProviderType: ProviderType(enc.KMSProvider),
		KeyID:        enc.KeyID,
	}
	if enc.SecretRef != nil {
		creds, err := loadSecret(ctx, c, namespace, enc.SecretRef.Name)
		if err != nil {
			return cfg, err
		}
		cfg.Credentials = creds
		if v, ok := creds["vault-url"]; ok {
			cfg.VaultURL = v
		}
	}
	return cfg, nil
}
```

Update `keyrotation_controller.go` to call `encryption.ProviderConfigFromEncryptionSpec(...)` instead of its private helpers. Keep the refactor minimal — don't rename anything else.

Then `buildEncryptorFromPolicy` in session-api becomes:

```go
func buildEncryptorFromPolicy(
	ctx context.Context, mgr ctrl.Manager, watcher *privacy.PolicyWatcher, log logr.Logger,
) (api.Encryptor, error) {
	eff := watcher.GetEffectivePolicy("", "")
	if eff == nil || !eff.Encryption.Enabled {
		return nil, nil
	}

	cfg, err := encryption.ProviderConfigFromEncryptionSpec(ctx, mgr.GetClient(), "omnia-system", eff.Encryption)
	if err != nil {
		return nil, err
	}
	provider, err := encryption.NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	return &encryptorAdapter{inner: encryption.NewEncryptor(provider)}, nil
}

type encryptorAdapter struct {
	inner encryption.Encryptor
}

func (a *encryptorAdapter) EncryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error) {
	out, _, err := a.inner.EncryptMessage(ctx, msg)
	return out, err
}

func (a *encryptorAdapter) DecryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error) {
	return a.inner.DecryptMessage(ctx, msg)
}

func (a *encryptorAdapter) EncryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error) {
	out, _, err := a.inner.EncryptToolCall(ctx, tc)
	return out, err
}

func (a *encryptorAdapter) DecryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error) {
	return a.inner.DecryptToolCall(ctx, tc)
}

func (a *encryptorAdapter) EncryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error) {
	out, _, err := a.inner.EncryptRuntimeEvent(ctx, evt)
	return out, err
}

func (a *encryptorAdapter) DecryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error) {
	return a.inner.DecryptRuntimeEvent(ctx, evt)
}
```

The session-api already has a K8s manager available (since `wrapPrivacyMiddleware` creates a `k8sClient`). The implementer should reuse that same client for secret loading — don't create a second one.

- [ ] **Step 5: Wire the PolicyResolver**

Also in `buildAPIMux`, wire the watcher as the PolicyResolver on the handler:

```go
if f.enterprise && watcher != nil {
	handler.SetPolicyResolver(watcher) // PolicyWatcher.ResolveEffectivePolicy satisfies api.PolicyResolver
}
```

**Note:** `wrapPrivacyMiddleware` currently creates the watcher locally. Refactor to return the watcher so `buildAPIMux` can pass it to both the middleware and the handler. Minimal change: split `wrapPrivacyMiddleware` into `buildPolicyWatcher()` returning `*privacy.PolicyWatcher` + `wrapPrivacyMiddlewareWithWatcher(next, watcher, ...)`.

- [ ] **Step 6: Run tests**

Run: `env GOWORK=off go test ./internal/session/api/... ./cmd/session-api/... ./ee/... -count=1`
Expected: all tests PASS.

- [ ] **Step 7: goimports + commit**

```
feat(session-api): wire encryption for messages, tool calls, runtime events

Builds an ee/pkg/encryption.Provider + Encryptor from the global
SessionPrivacyPolicy's Encryption config at startup. When enabled,
all writes encrypt and all reads decrypt. Also wires the PolicyWatcher
as the PolicyResolver for GET /api/v1/privacy-policy.

Refactors buildProviderConfig/loadSecretCredentials out of
keyrotation_controller.go into a shared encryption.ProviderConfigFromEncryptionSpec
so session-api and the keyrotation controller use the same logic.

Ref #780
```

---

## Task 9: Wire KeyRotationReconciler.StoreFactory

**Files:**
- Modify: `ee/cmd/omnia-arena-controller/main.go`

The `KeyRotationReconciler.StoreFactory` field is never populated, so re-encryption during key rotation fails with "store factory not configured" (see `keyrotation_controller.go:296`).

### Implementation

- [ ] **Step 1: Check arena-controller for existing Postgres config**

Look for any `--postgres-conn` flag or `POSTGRES_*` env var in `ee/cmd/omnia-arena-controller/main.go`. If absent, add a `--session-postgres-conn` flag (distinct from arena's own storage if it has one).

- [ ] **Step 2: Find the ReEncryptionStore Postgres constructor**

Grep for `ReEncryptionStore` implementations. The postgres provider likely has something like `pgprovider.NewReEncryptionStore(pool)` or similar. Inspect `internal/session/providers/postgres/reencryption.go` for the exact export.

- [ ] **Step 3: Populate StoreFactory**

```go
storeFactory := buildReEncryptionStoreFactory(sessionPostgresConn, setupLog)

if err := (&controller.KeyRotationReconciler{
	Client:   mgr.GetClient(),
	Scheme:   mgr.GetScheme(),
	Recorder: mgr.GetEventRecorderFor("keyrotation-controller"),
	ProviderFactory: func(cfg encryption.ProviderConfig) (encryption.Provider, error) {
		return encryption.NewProvider(cfg)
	},
	StoreFactory: storeFactory,
}).SetupWithManager(mgr); err != nil {
	// ...
}

func buildReEncryptionStoreFactory(conn string, log logr.Logger) func() (encryption.ReEncryptionStore, error) {
	if conn == "" {
		log.Info("reEncryption disabled", "reason", "no session postgres connection configured")
		return nil
	}
	return func() (encryption.ReEncryptionStore, error) {
		pool, err := pgxpool.New(context.Background(), conn)
		if err != nil {
			return nil, fmt.Errorf("open session postgres pool: %w", err)
		}
		return pgprovider.NewReEncryptionStore(pool), nil
	}
}
```

Update `privacyPolicyNamespace` usage if the secret lookup needs to happen in a different namespace than `omnia-system`.

- [ ] **Step 4: Run tests**

Run: `env GOWORK=off go test ./ee/cmd/omnia-arena-controller/... ./ee/internal/controller/... -count=1 -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```
feat(ee): wire StoreFactory for KeyRotationReconciler

Arena-controller opens a session Postgres pool and provides a
ReEncryptionStore factory. Without this wiring, key rotation was
skipping re-encryption with "store factory not configured".

Ref #780
```

---

## Task 10: Final Lint + Test Suite

- [ ] **Step 1: Run golangci-lint**

Run: `env GOWORK=off golangci-lint run ./...`
Expected: no new lint errors.

- [ ] **Step 2: Run full Go test suite**

Run: `env GOWORK=off go test ./... -count=1`
Expected: all tests PASS.

- [ ] **Step 3: Fix any issues and commit**

```
fix: address lint and test issues from privacy recording + encryption
```
