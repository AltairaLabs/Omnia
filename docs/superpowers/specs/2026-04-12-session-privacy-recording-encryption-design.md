# SessionPrivacyPolicy: Recording Control & Encryption at Rest

**Issue:** #780
**Date:** 2026-04-12

## Problem

The SessionPrivacyPolicy CRD exists with a full spec for recording control and encryption, but two gaps remain:

1. **Recording control** — the facade records all session data unconditionally regardless of the privacy policy's `Recording.Enabled`, `FacadeData`, and `RichData` flags.
2. **Encryption** — the CRD defines `EncryptionConfig` with KMS provider support, but session data is stored in Postgres as plaintext.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Recording enforcement | Defense in depth: facade skips + session-api rejects | Facade avoids unnecessary work; session-api catches bugs/stale images |
| Encryption approach | Local symmetric key (K8s Secret) first; KMS providers follow | Fastest path to encrypted-at-rest without external dependencies |
| Encrypted fields | `Content` + `State` | Covers messages, tool call payloads, and session state. Metadata stays queryable for dashboards |
| Tool name visibility | `ToolCall.Name` stays plaintext in store wrapper | Analytics can show which tools were used without decrypting Arguments/Result |
| Key rotation | Deferred to KMS provider follow-up | Rotation is more meaningful with KMS lifecycle policies |
| Encryption location | Session-api only | Single service holds the key; facade/dashboard get plaintext over the wire |
| Facade policy source | New session-api endpoint via existing httpclient | No new RBAC; reuses existing PolicyWatcher and HTTP client |

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

**Encryptor interface** in a new `ee/pkg/encryption/` package:

```go
type Encryptor interface {
    Encrypt(plaintext []byte) (ciphertext []byte, err error)
    Decrypt(ciphertext []byte) (plaintext []byte, err error)
}
```

**AESEncryptor:** Initial implementation using AES-256-GCM (authenticated encryption — tampering detected on decrypt). Key read from a Kubernetes Secret mounted as a volume into session-api.

**Key loading:** Session-api reads the Secret path from `ENCRYPTION_KEY_PATH` env var. If unset or empty, encryption is disabled — plaintext passthrough. Existing deployments upgrade without breaking.

**Encrypting store wrapper:** A thin wrapper around the session store that intercepts write and read paths:

- On write: encrypt `Content` and `State` fields before passing to the Postgres store
- On read: decrypt `Content` and `State` after retrieval
- `Metadata` (including `type`, `latency_ms`, `cost_usd`) stays plaintext
- For `ToolCall` records: encrypt `Arguments`, `Result`, `ErrorMessage`; keep `Name` plaintext
- For `RuntimeEvent` records: encrypt `Data`, `ErrorMessage`; keep `EventType` plaintext
- `ProviderCall` records: no encryption needed (operational metadata only)

Encryption is orthogonal to storage logic — the Postgres store never sees plaintext when encryption is enabled, and the wrapper is independently testable.

**Encrypted field encoding:** Ciphertext is base64-encoded before storing in `text` Postgres columns. A prefix marker (`enc:v1:`) distinguishes encrypted from plaintext data, enabling gradual migration — old unencrypted sessions read fine, new ones get encrypted.

### 4. Session-API Privacy Policy Endpoint + Client

**New endpoint:** `GET /api/v1/privacy-policy?namespace={ns}&agent={agent}`

Returns the effective policy for the given namespace/agent pair. The handler calls `PolicyWatcher.GetEffectivePolicy()` — no new logic, just exposing it over HTTP.

**Response:** JSON serialization of the existing `EffectivePolicy` struct (which carries `Recording` and `UserOptOut` — the fields needed for facade recording decisions). No new struct needed.

**Client method** added to the existing session-api httpclient:

```go
func (c *Client) GetPrivacyPolicy(ctx context.Context, namespace, agent string) (*EffectivePolicy, error)
```

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

- Session-api starts with encryption key mounted → encrypting wrapper active
- Session-api starts without key → plaintext passthrough, no errors
- Privacy policy endpoint responds with correct effective policy
- Facade httpclient correctly calls and caches the policy endpoint

### Doctor health checks

- Extend `PrivacyChecker`: verify encryption is active when configured (write test message, read raw from DB, confirm ciphertext with `enc:v1:` prefix)

## Follow-up Issues

Created as separate GitHub issues after this PR ships:

1. **AWS KMS provider** — `Encryptor` backed by AWS KMS envelope encryption
2. **Azure Key Vault provider** — `Encryptor` backed by Azure Key Vault
3. **GCP KMS provider** — `Encryptor` backed by GCP Cloud KMS
4. **HashiCorp Vault provider** — `Encryptor` backed by Vault Transit secrets engine
5. **Automated key rotation** — background job for re-encrypting existing data on key change

Each provider implements the `Encryptor` interface, adds webhook validation for provider-specific config, and wires provider selection based on `EncryptionConfig.KMSProvider`.

## Files Affected

### New files
- `ee/pkg/encryption/encryptor.go` — `Encryptor` interface + `AESEncryptor`
- `ee/pkg/encryption/encryptor_test.go`
- `ee/pkg/encryption/store_wrapper.go` — encrypting store wrapper
- `ee/pkg/encryption/store_wrapper_test.go`

### Modified files
- `internal/facade/recording_writer.go` — recording gate based on privacy policy
- `internal/facade/recording_writer_test.go`
- `internal/session/httpclient/client.go` — `GetPrivacyPolicy()` method
- `internal/session/httpclient/client_test.go`
- `internal/session/api/` — new privacy policy endpoint handler
- `ee/pkg/privacy/middleware.go` — recording flag enforcement
- `ee/pkg/privacy/middleware_test.go`
- `cmd/session-api/` — wire encryption wrapper and policy endpoint
- `internal/doctor/checks/privacy.go` — encryption health check
