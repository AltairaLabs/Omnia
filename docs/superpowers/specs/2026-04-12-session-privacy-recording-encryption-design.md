# SessionPrivacyPolicy: Recording Control & Encryption at Rest (REVISED)

**Issue:** #780
**Date:** 2026-04-12 (revised after discovering existing `ee/pkg/encryption/` package)

## Problem

The SessionPrivacyPolicy CRD exists with a full spec for recording control and encryption, but two gaps remain:

1. **Recording control** — the facade records all session data unconditionally regardless of the privacy policy's `Recording.Enabled`, `FacadeData`, and `RichData` flags.
2. **Encryption wiring** — the `ee/pkg/encryption/` package has a full `Encryptor` interface with four KMS providers (AWS KMS, Azure Key Vault, GCP KMS, Vault Transit), envelope encryption, and a `KeyRotationReconciler`. But **none of it is wired into session-api** — session data is stored in Postgres as plaintext because the handler layer never calls the encryptor.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Recording enforcement | Defense in depth: facade skips + session-api rejects | Facade avoids unnecessary work; session-api catches bugs/stale images |
| Encryption approach | Reuse existing `ee/pkg/encryption/` Encryptor + KMS providers | Full KMS support already built and tested — just needs wiring |
| Encrypted fields | Message `Content` + `Metadata values`, ToolCall `Arguments`/`Result`/`ErrorMessage`, RuntimeEvent `Data`/`ErrorMessage` | Covers all user-generated content; operational fields (names, types, timestamps, token counts) stay queryable |
| Plaintext for analytics | Message `Role`, ToolCall `Name`/`Status`/`DurationMs`, RuntimeEvent `EventType` | Dashboards can query "which tools ran" and "which events fired" without decryption |
| Key rotation | Existing `KeyRotationReconciler` in arena-controller — just needs `StoreFactory` wired | Already handles annotation-triggered + scheduled rotation with batched re-encryption |
| Encryption location | Session-api handler layer | Single chokepoint; facade/dashboard see plaintext over the wire |
| Facade policy source | New session-api endpoint via existing httpclient | No new RBAC; reuses PolicyWatcher |
| Package boundary | `internal/session/api` defines its own minimal `Encryptor` interface; adapter in `cmd/session-api` wraps `ee/pkg/encryption.Encryptor` | Keeps community edition free of `ee/` imports |
| Effective-policy exposure | `PolicyWatcher.EffectivePolicy` carries `Encryption` config; `facadePolicyJSON` filters it out when serving the facade | Session-api gets encryption config for provider construction; facade only sees recording flags |

## Design

### 1. Recording Control — Facade Layer

The facade's `recordingResponseWriter` gains a policy-aware recording gate.

**Policy loading:** On WebSocket connection setup, the facade calls `sessionAPIClient.GetPrivacyPolicy(ctx, namespace, agentName)` and caches the result for the session lifetime, refreshing every 60s in the background for long sessions.

**Recording gate logic in `recordingResponseWriter`:**

- `Recording.Enabled == false` → skip all recording (no messages, no tool calls, nothing)
- `Recording.RichData == false` → skip `WriteDone()` (assistant messages), `WriteToolCall()`, `WriteToolResult()` content. Still record session-level metadata (start/end, status, token counts).
- `Recording.FacadeData == false` → skip facade-layer summary recording

**Tool name visibility:** Tool names are preserved for analytics at the encryption layer, not the facade. The runtime records tool calls via `RecordToolCall` directly to session-api — the facade only sees backward-compat message recordings. The encrypting store wrapper keeps `ToolCall.Name` in plaintext while encrypting `Arguments`, `Result`, and `ErrorMessage`. Similarly, `RuntimeEvent.EventType` stays plaintext while `Data` and `ErrorMessage` are encrypted.

### 2. Recording Control — Session-API Safety Net

Extend the existing `PrivacyMiddleware` to enforce recording flags on message write requests (`AppendMessage` path):

- `Recording.Enabled == false` → return 204 No Content (same pattern as opt-out)
- `Recording.RichData == false` → inspect message metadata `type` field:
  - `tool_call`, `tool_result`, or role `assistant` → return 204 (drop the content)
  - Session-level updates (status changes, token count updates) → allow through

This is purely a safety net. If the facade is doing its job, these requests never arrive. If a facade runs a stale image or has a bug, session-api catches it.

No changes to `PolicyWatcher` — it already loads effective policies and makes them available to the middleware.

### 3. Encryption at Rest

**Reuse the existing `ee/pkg/encryption/` package** — it already provides:

- `Encryptor` interface with `EncryptMessage`/`DecryptMessage` on `*session.Message`
- `Provider` interface with four KMS implementations: AWS KMS, Azure Key Vault, GCP KMS, HashiCorp Vault Transit
- Envelope encryption (KMS-wrapped AES-256 data encryption keys)
- `MessageReEncryptor` for batched key rotation
- `encryptionMetadata` stored in message `Metadata["_encryption"]` to track which fields/keys were used

**What this plan adds:**

1. **Extend `Encryptor`** to cover `*session.ToolCall` and `*session.RuntimeEvent` — the existing interface only handles Messages. Use an envelope pattern: the encrypted fields (`Arguments`, `Result`, `Data`) are replaced with a map `{"_encryption": {metadata}, "_payload": "base64-ciphertext"}`. No schema migration needed — the Postgres columns are JSONB.

2. **Extend `PolicyWatcher.EffectivePolicy`** to include the merged `Encryption` config (today it only carries `Recording` and `UserOptOut`). The merge logic in `merge.go` already computes encryption merging.

3. **Wire session-api to build an `Encryptor` from the effective policy** at startup. Read the global `SessionPrivacyPolicy`; if `Encryption.Enabled`, construct a `Provider` via `encryption.NewProvider(ProviderConfigFromEncryptionSpec(...))` and wrap with `NewEncryptor`. Plaintext passthrough when disabled.

4. **Add encryption calls in session-api handlers**:
   - `handleAppendMessage` → `encryptor.EncryptMessage` before persisting
   - `handleGetMessages` → decrypt each returned message
   - Same for `ToolCall` and `RuntimeEvent` handlers

5. **Keep `internal/session/api/` free of `ee/` imports**: define a minimal `api.Encryptor` interface in the session-api package; an adapter in `cmd/session-api/main.go` wraps `ee/pkg/encryption.Encryptor` (drops the `[]EncryptionEvent` return).

6. **Wire `KeyRotationReconciler.StoreFactory`** in `ee/cmd/omnia-arena-controller/main.go` — this field is currently never populated, so re-encryption is silently skipped during key rotation. Open a Postgres pool and return a `pgprovider.NewReEncryptionStore(pool)`.

**Fields encrypted vs plaintext:**

| Struct | Plaintext (analytics) | Encrypted (user content) |
|--------|-----------------------|--------------------------|
| Message | ID, Role, Timestamp, token counts, Metadata keys | Content, Metadata values |
| ToolCall | ID, Name, Status, DurationMs, Timestamps, Labels | Arguments, Result, ErrorMessage |
| RuntimeEvent | ID, EventType, DurationMs, Timestamp | Data, ErrorMessage |
| ProviderCall | All fields (operational metadata only) | — |
| Session | ID, AgentName, Namespace, counts, timestamps | State |

### 4. Session-API Privacy Policy Endpoint + Client

**New endpoint:** `GET /api/v1/privacy-policy?namespace={ns}&agent={agent}`

Returns the effective policy for the given namespace/agent pair. The handler calls `PolicyWatcher.GetEffectivePolicy()` — no new logic, just exposing it over HTTP.

**Response:** JSON containing only the facade-visible subset (`Recording` fields). A `facadePolicyJSON` helper in `ee/pkg/privacy` filters out `Encryption` so key IDs and provider details don't leak to the facade. The full `EffectivePolicy` including `Encryption` stays server-side for session-api's own use.

**Client method** added to the existing session-api httpclient:

```go
func (s *Store) GetPrivacyPolicy(ctx context.Context, namespace, agent string) (*PrivacyPolicyResponse, error)
```

The response type `PrivacyPolicyResponse` is defined in the httpclient package (Recording fields only) so the facade doesn't import `ee/`.

**Facade-side caching:** Cached per session with 60s TTL. On cache miss or expiry, calls the client method. Short sessions: one call at connection start. Long sessions: background refresh.

**Fallback behavior:** If the session-api call fails (network error, session-api down), the facade defaults to **recording enabled**. A transient error should not silently drop session data. The error is logged and retried on the next TTL expiry.

## Testing Strategy

### Unit tests

- **AESEncryptor**: encrypt/decrypt round-trip, tamper detection (modified ciphertext fails), prefix marker parsing, plaintext passthrough when disabled
- **Encrypting store wrapper**: `Content` and `State` encrypted on write, decrypted on read, `Metadata` stays plaintext
- **Recording gate (facade)**: policy combinations (`Enabled=false`, `RichData=false`, `FacadeData=false`) — correct messages skipped/recorded
- **ToolCall/RuntimeEvent encryption**: Arguments/Result/Data encrypted, Name/EventType plaintext
- **Privacy middleware recording checks**: 204 responses for disabled recording, pass-through for allowed writes

### Integration tests

- Extend `privacy_integration_test.go`: create SessionPrivacyPolicy with recording disabled, verify messages don't land in Postgres
- Encryption round-trip through real Postgres store: write encrypted, read back plaintext, verify raw DB content is ciphertext
- Mixed migration: write plaintext (no encryption), enable encryption, write more, read all — both old and new messages return correctly

### Wiring tests

- Session-api starts with encryption enabled in global SessionPrivacyPolicy → `Encryptor` is built from the policy, handler has it set
- Session-api starts with no policy or `Encryption.Enabled=false` → plaintext passthrough
- Privacy policy endpoint responds with correct facade subset (no encryption config)
- Facade httpclient correctly calls and caches the policy endpoint
- KeyRotationReconciler has a non-nil `StoreFactory` when arena-controller has postgres connection configured

## Follow-up Issues

None required from this PR — the KMS providers, envelope encryption, and key rotation infrastructure are already present in `ee/pkg/encryption/`. Future work items:

- Per-workspace / per-agent encryption overrides (today session-api reads only the global policy's encryption config)
- Dashboard surfacing of rotation status (the `KeyRotationStatus` CRD field is populated but not shown in the UI)

## Files Affected

### New files
- `ee/pkg/privacy/resolver.go` — `facadePolicyJSON` helper + `PolicyWatcher.ResolveEffectivePolicy` method
- `ee/pkg/privacy/resolver_test.go`
- `ee/pkg/encryption/config_loader.go` — shared `ProviderConfigFromEncryptionSpec` used by session-api and keyrotation controller
- `internal/facade/recording_policy.go` — `RecordingPolicyCache` + `PolicyFetcher` interface
- `internal/facade/recording_policy_test.go`
- `internal/session/api/encryption_adapter.go` — `api.Encryptor` interface (no ee/ imports)

### Modified files
- `internal/session/httpclient/store.go` — `GetPrivacyPolicy` method + `PrivacyPolicyResponse` type
- `internal/session/httpclient/store_test.go`
- `internal/session/api/handler.go` — `PolicyResolver` interface, `SetEncryptor`, `handleGetPrivacyPolicy`, encryption calls in message/toolcall/event handlers
- `internal/session/api/handler_test.go`
- `internal/facade/recording_writer.go` — policy gate
- `internal/facade/recording_writer_test.go`
- `internal/facade/session.go` / `connection.go` / `server.go` — policy cache wiring
- `ee/pkg/privacy/watcher.go` — add `Encryption` to `EffectivePolicy`
- `ee/pkg/privacy/watcher_test.go`
- `ee/pkg/privacy/middleware.go` — recording flag enforcement (`checkRecordingPolicy`)
- `ee/pkg/privacy/middleware_test.go`
- `ee/pkg/encryption/encryptor.go` — extend `Encryptor` for `ToolCall` and `RuntimeEvent`
- `ee/pkg/encryption/encryption_test.go`
- `ee/internal/controller/keyrotation_controller.go` — switch to shared `ProviderConfigFromEncryptionSpec`
- `cmd/session-api/main.go` — build encryptor from policy, set on handler, wire `PolicyResolver`
- `ee/cmd/omnia-arena-controller/main.go` — wire `StoreFactory` for `KeyRotationReconciler`
