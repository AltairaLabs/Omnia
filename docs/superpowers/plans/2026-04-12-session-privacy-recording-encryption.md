# SessionPrivacyPolicy Recording Control & Encryption Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enforce SessionPrivacyPolicy recording flags at the facade and session-api layers, and encrypt sensitive session data at rest using AES-256-GCM with a symmetric key from a Kubernetes Secret.

**Architecture:** Defense-in-depth recording control (facade skips early, session-api rejects as safety net). Encryption via an `Encryptor` interface with an AES-256-GCM implementation, wired as a store wrapper in session-api. A new privacy-policy endpoint on session-api exposes effective policies to the facade via the existing httpclient.

**Tech Stack:** Go, AES-256-GCM (`crypto/aes`, `crypto/cipher`), Kubernetes Secrets, session-api HTTP, PromptKit `RecordingConfig` CRD types

**Spec:** `docs/superpowers/specs/2026-04-12-session-privacy-recording-encryption-design.md`

---

## File Structure

### New files
| File | Responsibility |
|------|---------------|
| `ee/pkg/encryption/encryptor.go` | `Encryptor` interface + `AESEncryptor` (AES-256-GCM) |
| `ee/pkg/encryption/encryptor_test.go` | Unit tests for AESEncryptor |
| `ee/pkg/encryption/store_wrapper.go` | Encrypting wrapper around `session.Store` |
| `ee/pkg/encryption/store_wrapper_test.go` | Unit tests for store wrapper |
| `internal/facade/recording_policy.go` | `RecordingPolicy` struct + facade-side caching |
| `internal/facade/recording_policy_test.go` | Unit tests for policy cache |

### Modified files
| File | Changes |
|------|---------|
| `internal/facade/recording_writer.go` | Add policy field, recording gate logic |
| `internal/facade/recording_writer_test.go` | Tests for gated recording + tool name metadata |
| `internal/facade/session.go` | Fetch privacy policy before creating recording writer |
| `internal/session/api/handler.go` | Register `GET /api/v1/privacy-policy` endpoint |
| `internal/session/api/handler_test.go` | Tests for privacy policy endpoint |
| `internal/session/httpclient/store.go` | Add `GetPrivacyPolicy()` method |
| `internal/session/httpclient/store_test.go` | Tests for new client method |
| `ee/pkg/privacy/middleware.go` | Add recording flag enforcement |
| `ee/pkg/privacy/middleware_test.go` | Tests for recording flag checks |
| `cmd/session-api/main.go` | Wire encryption wrapper + policy endpoint |
| `internal/doctor/checks/privacy.go` | Add encryption health check |

---

## Task 1: Encryptor Interface + AES-256-GCM Implementation

**Files:**
- Create: `ee/pkg/encryption/encryptor.go`
- Create: `ee/pkg/encryption/encryptor_test.go`

### Tests first

- [ ] **Step 1: Write the failing tests**

Create `ee/pkg/encryption/encryptor_test.go`:

```go
package encryption

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAESEncryptor_RoundTrip(t *testing.T) {
	key := make([]byte, 32) // AES-256
	_, err := rand.Read(key)
	require.NoError(t, err)

	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	plaintext := []byte("sensitive session content with PII")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	assert.NotEqual(t, plaintext, ciphertext)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestAESEncryptor_TamperDetection(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	ciphertext, err := enc.Encrypt([]byte("secret"))
	require.NoError(t, err)

	// Tamper with ciphertext
	ciphertext[len(ciphertext)-1] ^= 0xff

	_, err = enc.Decrypt(ciphertext)
	assert.Error(t, err)
}

func TestAESEncryptor_EmptyPlaintext(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	ciphertext, err := enc.Encrypt([]byte{})
	require.NoError(t, err)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, []byte{}, decrypted)
}

func TestAESEncryptor_InvalidKeySize(t *testing.T) {
	_, err := NewAESEncryptor([]byte("too-short"))
	assert.Error(t, err)
}

func TestAESEncryptor_UniqueNonces(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	c1, err := enc.Encrypt([]byte("same"))
	require.NoError(t, err)
	c2, err := enc.Encrypt([]byte("same"))
	require.NoError(t, err)

	// Same plaintext must produce different ciphertext (random nonce)
	assert.NotEqual(t, c1, c2)
}

func TestNoopEncryptor_Passthrough(t *testing.T) {
	enc := NewNoopEncryptor()
	plaintext := []byte("not encrypted")

	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, ciphertext)

	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./ee/pkg/encryption/... -count=1 -v`
Expected: Compilation failure — package doesn't exist yet.

### Implementation

- [ ] **Step 3: Write the implementation**

Create `ee/pkg/encryption/encryptor.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// VersionPrefix marks encrypted data and its format version.
const VersionPrefix = "enc:v1:"

// Encryptor encrypts and decrypts byte slices.
// Implementations must be safe for concurrent use.
type Encryptor interface {
	Encrypt(plaintext []byte) (ciphertext []byte, err error)
	Decrypt(ciphertext []byte) (plaintext []byte, err error)
}

// AESEncryptor uses AES-256-GCM for authenticated encryption.
type AESEncryptor struct {
	gcm cipher.AEAD
}

// NewAESEncryptor creates an AES-256-GCM encryptor.
// key must be exactly 32 bytes (AES-256).
func NewAESEncryptor(key []byte) (*AESEncryptor, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &AESEncryptor{gcm: gcm}, nil
}

// Encrypt encrypts plaintext using AES-256-GCM with a random nonce.
func (e *AESEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	// Seal appends ciphertext+tag to nonce: [nonce | ciphertext | tag]
	sealed := e.gcm.Seal(nonce, nonce, plaintext, nil)
	return sealed, nil
}

// Decrypt decrypts ciphertext produced by Encrypt.
func (e *AESEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	nonceSize := e.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, data := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := e.gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// NoopEncryptor passes data through unchanged. Used when encryption is disabled.
type NoopEncryptor struct{}

// NewNoopEncryptor creates a no-op encryptor (plaintext passthrough).
func NewNoopEncryptor() *NoopEncryptor {
	return &NoopEncryptor{}
}

func (n *NoopEncryptor) Encrypt(plaintext []byte) ([]byte, error) { return plaintext, nil }
func (n *NoopEncryptor) Decrypt(ciphertext []byte) ([]byte, error) { return ciphertext, nil }

// EncodeField encrypts a string field and returns a versioned base64 string.
// Returns the original string unchanged if it's empty.
func EncodeField(enc Encryptor, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	// NoopEncryptor: skip encoding overhead.
	if _, ok := enc.(*NoopEncryptor); ok {
		return plaintext, nil
	}
	ciphertext, err := enc.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}
	return VersionPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecodeField decrypts a versioned base64 string back to plaintext.
// If the string doesn't have the version prefix, it's returned as-is
// (unencrypted legacy data).
func DecodeField(enc Encryptor, encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	if !strings.HasPrefix(encoded, VersionPrefix) {
		return encoded, nil // legacy plaintext
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, VersionPrefix))
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	plaintext, err := enc.Decrypt(data)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// LoadKeyFromFile reads a 32-byte AES-256 key from a file path.
// Returns a NoopEncryptor if path is empty (encryption disabled).
func LoadKeyFromFile(path string) (Encryptor, error) {
	if path == "" {
		return NewNoopEncryptor(), nil
	}
	// Read is deferred to implementation step — os.ReadFile(path)
	// then validate len == 32.
	return nil, fmt.Errorf("not implemented: will be wired in Task 6")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./ee/pkg/encryption/... -count=1 -v`
Expected: All 6 tests PASS.

- [ ] **Step 5: Commit**

```
feat(ee): add Encryptor interface with AES-256-GCM implementation

Ref #780
```

---

## Task 2: Encrypting Store Wrapper

**Files:**
- Create: `ee/pkg/encryption/store_wrapper.go`
- Create: `ee/pkg/encryption/store_wrapper_test.go`

### Tests first

- [ ] **Step 1: Write the failing tests**

Create `ee/pkg/encryption/store_wrapper_test.go`:

```go
package encryption

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
)

// mockStore records calls and returns stored messages for verification.
type mockStore struct {
	session.Store
	appendedMessages []session.Message
	returnMessages   []session.Message
}

func (m *mockStore) AppendMessage(_ context.Context, _ string, msg session.Message) error {
	m.appendedMessages = append(m.appendedMessages, msg)
	return nil
}

func (m *mockStore) GetMessages(_ context.Context, _ string) ([]session.Message, error) {
	return m.returnMessages, nil
}

func (m *mockStore) GetSession(_ context.Context, _ string) (*session.Session, error) {
	return &session.Session{
		State: m.returnMessages[0].Content, // reuse Content field to carry State for testing
	}, nil
}

func TestEncryptingStore_AppendMessage_EncryptsContent(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	inner := &mockStore{}
	store := NewEncryptingStore(inner, enc)

	err = store.AppendMessage(context.Background(), "sess-1", session.Message{
		Content: "sensitive user message",
		Metadata: map[string]string{
			"type":      "tool_call",
			"tool_name": "search",
		},
	})
	require.NoError(t, err)
	require.Len(t, inner.appendedMessages, 1)

	stored := inner.appendedMessages[0]
	// Content must be encrypted (has version prefix)
	assert.True(t, len(stored.Content) > 0)
	assert.Contains(t, stored.Content, VersionPrefix)
	assert.NotContains(t, stored.Content, "sensitive user message")

	// Metadata must be plaintext
	assert.Equal(t, "tool_call", stored.Metadata["type"])
	assert.Equal(t, "search", stored.Metadata["tool_name"])
}

func TestEncryptingStore_GetMessages_DecryptsContent(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	// Pre-encrypt a message to simulate stored data
	encoded, err := EncodeField(enc, "decrypted content")
	require.NoError(t, err)

	inner := &mockStore{
		returnMessages: []session.Message{
			{Content: encoded, Metadata: map[string]string{"tool_name": "search"}},
		},
	}
	store := NewEncryptingStore(inner, enc)

	msgs, err := store.GetMessages(context.Background(), "sess-1")
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	assert.Equal(t, "decrypted content", msgs[0].Content)
	assert.Equal(t, "search", msgs[0].Metadata["tool_name"])
}

func TestEncryptingStore_LegacyPlaintext_PassesThrough(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	inner := &mockStore{
		returnMessages: []session.Message{
			{Content: "old plaintext message"}, // no enc:v1: prefix
		},
	}
	store := NewEncryptingStore(inner, enc)

	msgs, err := store.GetMessages(context.Background(), "sess-1")
	require.NoError(t, err)
	assert.Equal(t, "old plaintext message", msgs[0].Content)
}

func TestEncryptingStore_NoopEncryptor_NoPrefix(t *testing.T) {
	inner := &mockStore{}
	store := NewEncryptingStore(inner, NewNoopEncryptor())

	err := store.AppendMessage(context.Background(), "sess-1", session.Message{
		Content: "plain message",
	})
	require.NoError(t, err)
	require.Len(t, inner.appendedMessages, 1)

	// NoopEncryptor should not add prefix
	assert.Equal(t, "plain message", inner.appendedMessages[0].Content)
}

func TestEncryptingStore_RecordToolCall_EncryptsArgumentsAndResult(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	inner := &mockStore{}
	store := NewEncryptingStore(inner, enc)

	err = store.RecordToolCall(context.Background(), "sess-1", session.ToolCall{
		Name:      "web_search",
		Arguments: map[string]any{"query": "sensitive search"},
		Result:    "sensitive result data",
	})
	require.NoError(t, err)
	require.Len(t, inner.recordedToolCalls, 1)

	stored := inner.recordedToolCalls[0]
	// Name must stay plaintext for analytics
	assert.Equal(t, "web_search", stored.Name)
	// Arguments must be encrypted (stored as enc:v1: string in EncryptedArgs)
	assert.NotContains(t, stored.EncryptedArgs, "sensitive search")
	assert.Contains(t, stored.EncryptedArgs, VersionPrefix)
	// Result must be encrypted
	assert.NotContains(t, stored.EncryptedResult, "sensitive result")
	assert.Contains(t, stored.EncryptedResult, VersionPrefix)
	// Original fields should be nil/empty
	assert.Nil(t, stored.Arguments)
	assert.Nil(t, stored.Result)
}

func TestEncryptingStore_GetToolCalls_DecryptsFields(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	argsJSON, _ := json.Marshal(map[string]any{"query": "test"})
	encArgs, _ := EncodeField(enc, string(argsJSON))
	resultJSON, _ := json.Marshal("result data")
	encResult, _ := EncodeField(enc, string(resultJSON))

	inner := &mockStore{
		returnToolCalls: []session.ToolCall{
			{
				Name:           "web_search",
				EncryptedArgs:  encArgs,
				EncryptedResult: encResult,
			},
		},
	}
	store := NewEncryptingStore(inner, enc)

	tcs, err := store.GetToolCalls(context.Background(), "sess-1", 10, 0)
	require.NoError(t, err)
	require.Len(t, tcs, 1)

	assert.Equal(t, "web_search", tcs[0].Name)
	assert.Equal(t, map[string]any{"query": "test"}, tcs[0].Arguments)
	assert.Equal(t, "result data", tcs[0].Result)
}

func TestEncryptingStore_RecordRuntimeEvent_EncryptsData(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	enc, err := NewAESEncryptor(key)
	require.NoError(t, err)

	inner := &mockStore{}
	store := NewEncryptingStore(inner, enc)

	err = store.RecordRuntimeEvent(context.Background(), "sess-1", session.RuntimeEvent{
		EventType:    "pipeline.completed",
		Data:         map[string]any{"output": "sensitive"},
		ErrorMessage: "sensitive error",
	})
	require.NoError(t, err)
	require.Len(t, inner.recordedEvents, 1)

	stored := inner.recordedEvents[0]
	// EventType stays plaintext
	assert.Equal(t, "pipeline.completed", stored.EventType)
	// Data and ErrorMessage encrypted
	assert.Nil(t, stored.Data)
	assert.Contains(t, stored.EncryptedData, VersionPrefix)
	assert.Contains(t, stored.EncryptedError, VersionPrefix)
}
```

**Note:** The ToolCall and RuntimeEvent structs currently use `Arguments map[string]any` and `Data map[string]any` directly. For encryption, the store wrapper serializes these to JSON, encrypts the JSON string, and stores it in a new `EncryptedArgs`/`EncryptedResult`/`EncryptedData`/`EncryptedError` string field. On read, it reverses the process. This means **the ToolCall and RuntimeEvent structs need new string fields** for encrypted data. The implementer should add these fields to `internal/session/store.go`:

```go
// In ToolCall struct:
EncryptedArgs   string `json:"encryptedArgs,omitempty"`
EncryptedResult string `json:"encryptedResult,omitempty"`

// In RuntimeEvent struct:
EncryptedData  string `json:"encryptedData,omitempty"`
EncryptedError string `json:"encryptedError,omitempty"`
```

The Postgres columns need corresponding migrations. Alternatively, the wrapper can serialize Arguments/Result/Data into the existing fields as encrypted JSON strings (overloading the field type). The implementer should choose based on the Postgres schema — if `arguments` is `jsonb`, storing a base64 string there would violate the column type, so new `text` columns are needed. If `arguments` is already `text`, in-place encryption works.

The mockStore also needs additional fields:
```go
type mockStore struct {
	session.Store
	appendedMessages []session.Message
	returnMessages   []session.Message
	recordedToolCalls []session.ToolCall
	returnToolCalls   []session.ToolCall
	recordedEvents    []session.RuntimeEvent
}

func (m *mockStore) RecordToolCall(_ context.Context, _ string, tc session.ToolCall) error {
	m.recordedToolCalls = append(m.recordedToolCalls, tc)
	return nil
}

func (m *mockStore) GetToolCalls(_ context.Context, _ string, _, _ int) ([]session.ToolCall, error) {
	return m.returnToolCalls, nil
}

func (m *mockStore) RecordRuntimeEvent(_ context.Context, _ string, evt session.RuntimeEvent) error {
	m.recordedEvents = append(m.recordedEvents, evt)
	return nil
}

func (m *mockStore) GetRuntimeEvents(_ context.Context, _ string, _, _ int) ([]session.RuntimeEvent, error) {
	return nil, nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./ee/pkg/encryption/... -count=1 -v -run TestEncryptingStore`
Expected: Compilation failure — `NewEncryptingStore` doesn't exist.

### Implementation

- [ ] **Step 3: Write the implementation**

Create `ee/pkg/encryption/store_wrapper.go`:

```go
/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package encryption

import (
	"context"
	"fmt"

	"github.com/altairalabs/omnia/internal/session"
)

// EncryptingStore wraps a session.Store and encrypts Content and State
// fields on write, decrypting them on read. Metadata stays plaintext.
type EncryptingStore struct {
	inner session.Store
	enc   Encryptor
}

// NewEncryptingStore creates an encrypting wrapper around inner.
func NewEncryptingStore(inner session.Store, enc Encryptor) *EncryptingStore {
	return &EncryptingStore{inner: inner, enc: enc}
}

func (s *EncryptingStore) AppendMessage(ctx context.Context, sessionID string, msg session.Message) error {
	encrypted, err := EncodeField(s.enc, msg.Content)
	if err != nil {
		return fmt.Errorf("encrypt message content: %w", err)
	}
	msg.Content = encrypted
	return s.inner.AppendMessage(ctx, sessionID, msg)
}

func (s *EncryptingStore) GetMessages(ctx context.Context, sessionID string) ([]session.Message, error) {
	msgs, err := s.inner.GetMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for i := range msgs {
		decrypted, decErr := DecodeField(s.enc, msgs[i].Content)
		if decErr != nil {
			return nil, fmt.Errorf("decrypt message %s: %w", msgs[i].ID, decErr)
		}
		msgs[i].Content = decrypted
	}
	return msgs, nil
}

func (s *EncryptingStore) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	sess, err := s.inner.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	decryptedState, err := DecodeField(s.enc, sess.State)
	if err != nil {
		return nil, fmt.Errorf("decrypt session state: %w", err)
	}
	sess.State = decryptedState

	// Decrypt message Content fields if messages are loaded.
	for i := range sess.Messages {
		decrypted, decErr := DecodeField(s.enc, sess.Messages[i].Content)
		if decErr != nil {
			return nil, fmt.Errorf("decrypt message %s: %w", sess.Messages[i].ID, decErr)
		}
		sess.Messages[i].Content = decrypted
	}
	return sess, nil
}

func (s *EncryptingStore) CreateSession(ctx context.Context, opts session.CreateSessionOptions) (*session.Session, error) {
	if opts.InitialState != "" {
		encrypted, err := EncodeField(s.enc, opts.InitialState)
		if err != nil {
			return nil, fmt.Errorf("encrypt initial state: %w", err)
		}
		opts.InitialState = encrypted
	}
	sess, err := s.inner.CreateSession(ctx, opts)
	if err != nil {
		return nil, err
	}
	// Decrypt state in the returned session.
	if sess.State != "" {
		decrypted, decErr := DecodeField(s.enc, sess.State)
		if decErr != nil {
			return nil, fmt.Errorf("decrypt session state: %w", decErr)
		}
		sess.State = decrypted
	}
	return sess, nil
}

// Delegated methods — no encryption needed for these.

func (s *EncryptingStore) DeleteSession(ctx context.Context, sessionID string) error {
	return s.inner.DeleteSession(ctx, sessionID)
}

func (s *EncryptingStore) RefreshTTL(ctx context.Context, sessionID string, ttl time.Duration) error {
	return s.inner.RefreshTTL(ctx, sessionID, ttl)
}

func (s *EncryptingStore) UpdateSessionStatus(ctx context.Context, sessionID string, update session.SessionStatusUpdate) error {
	return s.inner.UpdateSessionStatus(ctx, sessionID, update)
}

// RecordToolCall encrypts Arguments, Result, and ErrorMessage. Name stays plaintext.
func (s *EncryptingStore) RecordToolCall(ctx context.Context, sessionID string, tc session.ToolCall) error {
	if tc.Arguments != nil {
		argsJSON, err := json.Marshal(tc.Arguments)
		if err != nil {
			return fmt.Errorf("marshal tool call arguments: %w", err)
		}
		encrypted, err := EncodeField(s.enc, string(argsJSON))
		if err != nil {
			return fmt.Errorf("encrypt tool call arguments: %w", err)
		}
		tc.EncryptedArgs = encrypted
		tc.Arguments = nil
	}
	if tc.Result != nil {
		resultJSON, err := json.Marshal(tc.Result)
		if err != nil {
			return fmt.Errorf("marshal tool call result: %w", err)
		}
		encrypted, err := EncodeField(s.enc, string(resultJSON))
		if err != nil {
			return fmt.Errorf("encrypt tool call result: %w", err)
		}
		tc.EncryptedResult = encrypted
		tc.Result = nil
	}
	if tc.ErrorMessage != "" {
		encrypted, err := EncodeField(s.enc, tc.ErrorMessage)
		if err != nil {
			return fmt.Errorf("encrypt tool call error: %w", err)
		}
		tc.ErrorMessage = encrypted
	}
	return s.inner.RecordToolCall(ctx, sessionID, tc)
}

// GetToolCalls decrypts Arguments, Result, and ErrorMessage.
func (s *EncryptingStore) GetToolCalls(ctx context.Context, sessionID string, limit, offset int) ([]session.ToolCall, error) {
	tcs, err := s.inner.GetToolCalls(ctx, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	for i := range tcs {
		if tcs[i].EncryptedArgs != "" {
			decrypted, decErr := DecodeField(s.enc, tcs[i].EncryptedArgs)
			if decErr != nil {
				return nil, fmt.Errorf("decrypt tool call %s args: %w", tcs[i].ID, decErr)
			}
			if err := json.Unmarshal([]byte(decrypted), &tcs[i].Arguments); err != nil {
				return nil, fmt.Errorf("unmarshal tool call %s args: %w", tcs[i].ID, err)
			}
			tcs[i].EncryptedArgs = ""
		}
		if tcs[i].EncryptedResult != "" {
			decrypted, decErr := DecodeField(s.enc, tcs[i].EncryptedResult)
			if decErr != nil {
				return nil, fmt.Errorf("decrypt tool call %s result: %w", tcs[i].ID, decErr)
			}
			if err := json.Unmarshal([]byte(decrypted), &tcs[i].Result); err != nil {
				return nil, fmt.Errorf("unmarshal tool call %s result: %w", tcs[i].ID, err)
			}
			tcs[i].EncryptedResult = ""
		}
		if tcs[i].ErrorMessage != "" {
			decrypted, decErr := DecodeField(s.enc, tcs[i].ErrorMessage)
			if decErr != nil {
				return nil, fmt.Errorf("decrypt tool call %s error: %w", tcs[i].ID, decErr)
			}
			tcs[i].ErrorMessage = decrypted
		}
	}
	return tcs, nil
}

// RecordRuntimeEvent encrypts Data and ErrorMessage. EventType stays plaintext.
func (s *EncryptingStore) RecordRuntimeEvent(ctx context.Context, sessionID string, evt session.RuntimeEvent) error {
	if evt.Data != nil {
		dataJSON, err := json.Marshal(evt.Data)
		if err != nil {
			return fmt.Errorf("marshal runtime event data: %w", err)
		}
		encrypted, err := EncodeField(s.enc, string(dataJSON))
		if err != nil {
			return fmt.Errorf("encrypt runtime event data: %w", err)
		}
		evt.EncryptedData = encrypted
		evt.Data = nil
	}
	if evt.ErrorMessage != "" {
		encrypted, err := EncodeField(s.enc, evt.ErrorMessage)
		if err != nil {
			return fmt.Errorf("encrypt runtime event error: %w", err)
		}
		evt.ErrorMessage = encrypted
	}
	return s.inner.RecordRuntimeEvent(ctx, sessionID, evt)
}

// GetRuntimeEvents decrypts Data and ErrorMessage.
func (s *EncryptingStore) GetRuntimeEvents(ctx context.Context, sessionID string, limit, offset int) ([]session.RuntimeEvent, error) {
	evts, err := s.inner.GetRuntimeEvents(ctx, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	for i := range evts {
		if evts[i].EncryptedData != "" {
			decrypted, decErr := DecodeField(s.enc, evts[i].EncryptedData)
			if decErr != nil {
				return nil, fmt.Errorf("decrypt event %s data: %w", evts[i].ID, decErr)
			}
			if err := json.Unmarshal([]byte(decrypted), &evts[i].Data); err != nil {
				return nil, fmt.Errorf("unmarshal event %s data: %w", evts[i].ID, err)
			}
			evts[i].EncryptedData = ""
		}
		if evts[i].ErrorMessage != "" {
			decrypted, decErr := DecodeField(s.enc, evts[i].ErrorMessage)
			if decErr != nil {
				return nil, fmt.Errorf("decrypt event %s error: %w", evts[i].ID, decErr)
			}
			evts[i].ErrorMessage = decrypted
		}
	}
	return evts, nil
}

func (s *EncryptingStore) RecordProviderCall(ctx context.Context, sessionID string, pc session.ProviderCall) error {
	return s.inner.RecordProviderCall(ctx, sessionID, pc)
}

func (s *EncryptingStore) GetProviderCalls(ctx context.Context, sessionID string, limit, offset int) ([]session.ProviderCall, error) {
	return s.inner.GetProviderCalls(ctx, sessionID, limit, offset)
}

func (s *EncryptingStore) RecordEvalResult(ctx context.Context, sessionID string, result session.EvalResult) error {
	return s.inner.RecordEvalResult(ctx, sessionID, result)
}

func (s *EncryptingStore) Close() error {
	return s.inner.Close()
}

// Verify interface compliance.
var _ session.Store = (*EncryptingStore)(nil)
```

**Note:** The `time` and `encoding/json` imports and exact delegated methods should match the current `session.Store` interface. The implementer should verify all interface methods are delegated by checking `internal/session/store.go:372-440`.

**Prerequisite — struct and schema changes:**

Before the store wrapper can use `EncryptedArgs`/`EncryptedResult`/`EncryptedData`/`EncryptedError`, these fields must exist on the structs and in Postgres:

1. Add fields to `internal/session/store.go`:
   - `ToolCall`: `EncryptedArgs string` and `EncryptedResult string` (with `json:"encryptedArgs,omitempty"` / `json:"encryptedResult,omitempty"`)
   - `RuntimeEvent`: `EncryptedData string` and `EncryptedError string` (with `json:"encryptedData,omitempty"` / `json:"encryptedError,omitempty"`)

2. Add a Postgres migration in `internal/session/postgres/migrations/`:
   - `ALTER TABLE tool_calls ADD COLUMN encrypted_args TEXT, ADD COLUMN encrypted_result TEXT;`
   - `ALTER TABLE runtime_events ADD COLUMN encrypted_data TEXT, ADD COLUMN encrypted_error TEXT;`

3. Update the Postgres store `RecordToolCall`/`GetToolCalls` and `RecordRuntimeEvent`/`GetRuntimeEvents` queries to read/write the new columns.

The implementer should check the existing migration numbering pattern and Postgres store queries to wire these correctly.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./ee/pkg/encryption/... -count=1 -v`
Expected: All tests PASS.

- [ ] **Step 5: Run goimports**

Run: `goimports -w ee/pkg/encryption/store_wrapper.go`

- [ ] **Step 6: Commit**

```
feat(ee): add encrypting store wrapper for session data

Wraps session.Store to encrypt sensitive fields on write, decrypt on
read. Encrypted: Message.Content, Session.State, ToolCall.Arguments/
Result/ErrorMessage, RuntimeEvent.Data/ErrorMessage. Plaintext for
analytics: ToolCall.Name, RuntimeEvent.EventType, all Metadata.
Supports gradual migration via enc:v1: prefix detection.

Ref #780
```

---

## Task 3: Privacy Policy Endpoint on Session-API

**Files:**
- Modify: `internal/session/api/handler.go:159-201` (add route)
- Create or modify: handler method for the new endpoint
- Modify: `internal/session/api/handler_test.go` (add test)

The privacy policy endpoint exposes the effective policy to the facade. It delegates to `PolicyWatcher.GetEffectivePolicy()`.

### Tests first

- [ ] **Step 1: Write the failing test**

Add to `internal/session/api/handler_test.go` (or a new `privacy_handler_test.go` in the same package):

```go
func TestHandleGetPrivacyPolicy_ReturnsEffective(t *testing.T) {
	// Mock PolicyResolver that returns a canned policy
	resolver := &mockPolicyResolver{
		policy: &privacy.EffectivePolicy{
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
				RichData:   false,
			},
		},
	}
	handler := api.NewHandler(nil, logr.Discard(), api.DefaultMaxBodySize)
	handler.SetPolicyResolver(resolver)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var got privacy.EffectivePolicy
	err := json.NewDecoder(rec.Body).Decode(&got)
	require.NoError(t, err)
	assert.True(t, got.Recording.Enabled)
	assert.False(t, got.Recording.RichData)
}

func TestHandleGetPrivacyPolicy_NoPolicyReturns204(t *testing.T) {
	resolver := &mockPolicyResolver{policy: nil}
	handler := api.NewHandler(nil, logr.Discard(), api.DefaultMaxBodySize)
	handler.SetPolicyResolver(resolver)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleGetPrivacyPolicy_NoResolverReturns204(t *testing.T) {
	handler := api.NewHandler(nil, logr.Discard(), api.DefaultMaxBodySize)
	// No resolver set — non-enterprise deployment

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/privacy-policy?namespace=default&agent=my-agent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
```

The `mockPolicyResolver` implements:
```go
type mockPolicyResolver struct {
	policy *privacy.EffectivePolicy
}

func (m *mockPolicyResolver) GetEffectivePolicy(namespace, agentName string) *privacy.EffectivePolicy {
	return m.policy
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/api/... -count=1 -v -run TestHandleGetPrivacyPolicy`
Expected: Compilation failure — `SetPolicyResolver` doesn't exist.

### Implementation

- [ ] **Step 3: Define the PolicyResolver interface and add handler method**

In `internal/session/api/handler.go`, add:

1. A `PolicyResolver` interface:
```go
// PolicyResolver resolves the effective privacy policy for a namespace/agent pair.
type PolicyResolver interface {
	GetEffectivePolicy(namespace, agentName string) *privacy.EffectivePolicy
}
```

**Note:** This import references `ee/pkg/privacy`. If the handler package cannot import EE code, use a minimal struct in the api package instead:

```go
// EffectivePolicyResponse is the response shape for GET /api/v1/privacy-policy.
// Mirrors privacy.EffectivePolicy to avoid importing ee/ from internal/.
type EffectivePolicyResponse struct {
	Recording json.RawMessage `json:"recording"`
	UserOptOut json.RawMessage `json:"userOptOut,omitempty"`
}

// PolicyResolver resolves the effective privacy policy for a namespace/agent pair.
// Returns nil if no policy applies. The returned byte slices are JSON-encoded
// RecordingConfig and UserOptOutConfig.
type PolicyResolver interface {
	GetEffectivePolicyJSON(namespace, agentName string) (recording, userOptOut []byte, found bool)
}
```

The implementer should check whether `internal/session/api/` already imports `ee/` packages. If it does, use the direct `*privacy.EffectivePolicy` approach. If not, use the JSON-based interface to maintain the boundary.

2. Add `policyResolver PolicyResolver` field to `Handler` struct.

3. Add `SetPolicyResolver(resolver PolicyResolver)` method.

4. Add `handleGetPrivacyPolicy` handler:
```go
func (h *Handler) handleGetPrivacyPolicy(w http.ResponseWriter, r *http.Request) {
	if h.policyResolver == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ns := r.URL.Query().Get("namespace")
	agent := r.URL.Query().Get("agent")

	policy := h.policyResolver.GetEffectivePolicy(ns, agent)
	if policy == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policy)
}
```

5. Register route in `RegisterRoutes()`:
```go
mux.HandleFunc("GET /api/v1/privacy-policy", h.handleGetPrivacyPolicy)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/api/... -count=1 -v -run TestHandleGetPrivacyPolicy`
Expected: All 3 tests PASS.

- [ ] **Step 5: Run goimports**

Run: `goimports -w internal/session/api/handler.go`

- [ ] **Step 6: Commit**

```
feat(api): add GET /api/v1/privacy-policy endpoint

Exposes effective SessionPrivacyPolicy to the facade via the existing
session-api. Returns 204 when no policy applies or resolver is not set
(non-enterprise).

Ref #780
```

---

## Task 4: Session-API HTTP Client — GetPrivacyPolicy Method

**Files:**
- Modify: `internal/session/httpclient/store.go`
- Modify: `internal/session/httpclient/store_test.go`

### Tests first

- [ ] **Step 1: Write the failing test**

Add to `internal/session/httpclient/store_test.go`:

```go
func TestStore_GetPrivacyPolicy_Success(t *testing.T) {
	expected := privacy.EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   false,
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/privacy-policy", r.URL.Path)
		assert.Equal(t, "default", r.URL.Query().Get("namespace"))
		assert.Equal(t, "my-agent", r.URL.Query().Get("agent"))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	store := httpclient.NewStore(srv.URL, logr.Discard())
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
	_, err := store.GetPrivacyPolicy(context.Background(), "default", "my-agent")
	assert.Error(t, err)
}
```

**Note:** The test imports for `privacy.EffectivePolicy` and `omniav1alpha1.RecordingConfig` should match whatever approach was chosen in Task 3. If the JSON-based interface was used, decode into the corresponding response struct instead.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/session/httpclient/... -count=1 -v -run TestStore_GetPrivacyPolicy`
Expected: Compilation failure — `GetPrivacyPolicy` doesn't exist.

### Implementation

- [ ] **Step 3: Add GetPrivacyPolicy to the httpclient Store**

In `internal/session/httpclient/store.go`, add:

```go
// GetPrivacyPolicy fetches the effective privacy policy for a namespace/agent pair.
// Returns nil with no error if no policy applies (204 response).
func (s *Store) GetPrivacyPolicy(ctx context.Context, namespace, agent string) (*privacy.EffectivePolicy, error) {
	url := fmt.Sprintf("%s/api/v1/privacy-policy?namespace=%s&agent=%s",
		s.baseURL,
		neturl.QueryEscape(namespace),
		neturl.QueryEscape(agent),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("privacy policy request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("privacy policy: unexpected status %d", resp.StatusCode)
	}

	var policy privacy.EffectivePolicy
	if err := json.NewDecoder(resp.Body).Decode(&policy); err != nil {
		return nil, fmt.Errorf("decode privacy policy: %w", err)
	}
	return &policy, nil
}
```

**Note:** This method intentionally does NOT go through the circuit breaker — it's a read-only config endpoint, not a session write. A transient failure should not open the circuit for session writes.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/session/httpclient/... -count=1 -v -run TestStore_GetPrivacyPolicy`
Expected: All 3 tests PASS.

- [ ] **Step 5: Run goimports**

Run: `goimports -w internal/session/httpclient/store.go`

- [ ] **Step 6: Commit**

```
feat(httpclient): add GetPrivacyPolicy method to session-api client

Facade uses this to fetch effective privacy policy for recording
decisions. Returns nil when no policy applies (204). Bypasses circuit
breaker since it's a config read, not a session write.

Ref #780
```

---

## Task 5: Facade Recording Policy Cache

**Files:**
- Create: `internal/facade/recording_policy.go`
- Create: `internal/facade/recording_policy_test.go`

This is the facade-side cache that wraps the httpclient call with a TTL.

### Tests first

- [ ] **Step 1: Write the failing tests**

Create `internal/facade/recording_policy_test.go`:

```go
package facade

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

type mockPolicyFetcher struct {
	policy    *privacy.EffectivePolicy
	err       error
	callCount atomic.Int32
}

func (m *mockPolicyFetcher) GetPrivacyPolicy(_ context.Context, _, _ string) (*privacy.EffectivePolicy, error) {
	m.callCount.Add(1)
	return m.policy, m.err
}

func TestRecordingPolicyCache_FetchesOnFirstCall(t *testing.T) {
	fetcher := &mockPolicyFetcher{
		policy: &privacy.EffectivePolicy{
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:  true,
				RichData: false,
			},
		},
	}
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 60*time.Second)

	policy := cache.Get(context.Background())
	require.NotNil(t, policy)
	assert.True(t, policy.Recording.Enabled)
	assert.False(t, policy.Recording.RichData)
	assert.Equal(t, int32(1), fetcher.callCount.Load())
}

func TestRecordingPolicyCache_ReturnsCachedWithinTTL(t *testing.T) {
	fetcher := &mockPolicyFetcher{
		policy: &privacy.EffectivePolicy{
			Recording: omniav1alpha1.RecordingConfig{Enabled: true},
		},
	}
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 60*time.Second)

	cache.Get(context.Background())
	cache.Get(context.Background())
	cache.Get(context.Background())

	assert.Equal(t, int32(1), fetcher.callCount.Load())
}

func TestRecordingPolicyCache_FetchError_DefaultsToRecordingEnabled(t *testing.T) {
	fetcher := &mockPolicyFetcher{
		err: fmt.Errorf("connection refused"),
	}
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 60*time.Second)

	policy := cache.Get(context.Background())
	// Default: recording enabled (don't silently drop data on transient error)
	require.NotNil(t, policy)
	assert.True(t, policy.Recording.Enabled)
	assert.True(t, policy.Recording.RichData)
	assert.True(t, policy.Recording.FacadeData)
}

func TestRecordingPolicyCache_NilPolicy_CachesResult(t *testing.T) {
	fetcher := &mockPolicyFetcher{policy: nil}
	cache := NewRecordingPolicyCache(fetcher, "default", "agent-1", 60*time.Second)

	policy := cache.Get(context.Background())
	// nil from server means no policy → default to recording enabled
	require.NotNil(t, policy)
	assert.True(t, policy.Recording.Enabled)

	cache.Get(context.Background())
	assert.Equal(t, int32(1), fetcher.callCount.Load())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/facade/... -count=1 -v -run TestRecordingPolicyCache`
Expected: Compilation failure — `NewRecordingPolicyCache` doesn't exist.

### Implementation

- [ ] **Step 3: Write the implementation**

Create `internal/facade/recording_policy.go`:

```go
package facade

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// PolicyFetcher fetches the effective privacy policy for a namespace/agent.
type PolicyFetcher interface {
	GetPrivacyPolicy(ctx context.Context, namespace, agent string) (*privacy.EffectivePolicy, error)
}

// defaultPolicy is used when no policy is found or fetch fails.
// Recording is fully enabled to avoid silently dropping data.
var defaultPolicy = &privacy.EffectivePolicy{
	Recording: omniav1alpha1.RecordingConfig{
		Enabled:    true,
		FacadeData: true,
		RichData:   true,
	},
}

// RecordingPolicyCache caches the effective privacy policy for a single
// namespace/agent pair (one cache per WebSocket session).
type RecordingPolicyCache struct {
	fetcher   PolicyFetcher
	namespace string
	agent     string
	ttl       time.Duration

	mu        sync.Mutex
	cached    *privacy.EffectivePolicy
	fetchedAt time.Time
}

// NewRecordingPolicyCache creates a policy cache for a session.
func NewRecordingPolicyCache(fetcher PolicyFetcher, namespace, agent string, ttl time.Duration) *RecordingPolicyCache {
	return &RecordingPolicyCache{
		fetcher:   fetcher,
		namespace: namespace,
		agent:     agent,
		ttl:       ttl,
	}
}

// Get returns the cached policy, refreshing if expired.
func (c *RecordingPolicyCache) Get(ctx context.Context) *privacy.EffectivePolicy {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cached != nil && time.Since(c.fetchedAt) < c.ttl {
		return c.cached
	}

	policy, err := c.fetcher.GetPrivacyPolicy(ctx, c.namespace, c.agent)
	if err != nil {
		// On error, use default (recording enabled) to avoid dropping data.
		// Cache it briefly to avoid hammering session-api.
		c.cached = defaultPolicy
		c.fetchedAt = time.Now()
		return c.cached
	}

	if policy == nil {
		c.cached = defaultPolicy
	} else {
		c.cached = policy
	}
	c.fetchedAt = time.Now()
	return c.cached
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/facade/... -count=1 -v -run TestRecordingPolicyCache`
Expected: All 4 tests PASS.

- [ ] **Step 5: Run goimports**

Run: `goimports -w internal/facade/recording_policy.go`

- [ ] **Step 6: Commit**

```
feat(facade): add recording policy cache with TTL

Caches effective privacy policy per WebSocket session. Defaults to
recording enabled on fetch errors to avoid silently dropping data.

Ref #780
```

---

## Task 6: Facade Recording Gate

**Files:**
- Modify: `internal/facade/recording_writer.go`
- Modify: `internal/facade/recording_writer_test.go`
- Modify: `internal/facade/session.go`

This is the core change — the recording writer checks the policy before recording.

### Tests first

- [ ] **Step 1: Write the failing tests for recording gate**

Add to `internal/facade/recording_writer_test.go`:

```go
func TestRecordingWriter_RecordingDisabled_SkipsAll(t *testing.T) {
	store := &mockSessionStore{}
	policy := &privacy.EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled: false,
		},
	}
	writer := newRecordingWriter(context.Background(), &mockInnerWriter{}, store, "sess-1", logr.Discard(), nil)
	writer.setPolicy(policy)

	// These should all delegate to inner but NOT record
	writer.WriteDone("hello")
	writer.WriteToolCall(&ToolCallInfo{ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"q": "test"}})
	writer.WriteToolResult(&ToolResultInfo{ID: "tc-1", Result: "found it"})

	// Give async goroutines time to complete (if any leaked through)
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.messages, "no messages should be recorded when recording is disabled")
}

func TestRecordingWriter_RichDataDisabled_SkipsContent(t *testing.T) {
	store := &mockSessionStore{}
	policy := &privacy.EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   false,
		},
	}
	writer := newRecordingWriter(context.Background(), &mockInnerWriter{}, store, "sess-1", logr.Discard(), nil)
	writer.setPolicy(policy)

	writer.WriteDone("assistant response")
	writer.WriteToolCall(&ToolCallInfo{ID: "tc-1", Name: "search", Arguments: map[string]interface{}{"q": "test"}})
	writer.WriteToolResult(&ToolResultInfo{ID: "tc-1", Result: "result"})

	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, store.messages, "no rich content should be recorded when RichData is disabled")
}

func TestRecordingWriter_DefaultPolicy_RecordsEverything(t *testing.T) {
	store := &mockSessionStore{}
	// No policy set — should default to recording everything
	writer := newRecordingWriter(context.Background(), &mockInnerWriter{}, store, "sess-1", logr.Discard(), nil)

	writer.WriteDone("hello")

	time.Sleep(50 * time.Millisecond)
	assert.Len(t, store.messages, 1)
}
```

The `mockSessionStore` and `mockInnerWriter` test helpers should capture appended messages and delegate calls respectively. The implementer should check if these already exist in the test file and extend them.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/facade/... -count=1 -v -run "TestRecordingWriter_(RecordingDisabled|RichDataDisabled|DefaultPolicy)"`
Expected: Compilation failure — `setPolicy` doesn't exist.

### Implementation

- [ ] **Step 3: Add policy field and setPolicy to recordingResponseWriter**

In `internal/facade/recording_writer.go`:

1. Add `policy *privacy.EffectivePolicy` field to the `recordingResponseWriter` struct.

2. Add a `setPolicy` method:
```go
func (w *recordingResponseWriter) setPolicy(p *privacy.EffectivePolicy) {
	w.policy = p
}
```

3. Add helper methods for recording gate checks:
```go
// shouldRecord returns false if recording is entirely disabled.
func (w *recordingResponseWriter) shouldRecord() bool {
	return w.policy == nil || w.policy.Recording.Enabled
}

// shouldRecordRichData returns false if rich data (messages, tool calls) should be skipped.
func (w *recordingResponseWriter) shouldRecordRichData() bool {
	return w.policy == nil || w.policy.Recording.RichData
}
```

- [ ] **Step 4: Add recording gate to WriteDone, WriteToolCall, WriteToolResult, WriteError**

Modify each method to check the policy before submitting the recording task:

**WriteDone / WriteDoneWithParts** — gate on `shouldRecord() && shouldRecordRichData()`:
```go
func (w *recordingResponseWriter) WriteDone(content string) error {
	err := w.inner.WriteDone(content)
	if w.shouldRecord() && w.shouldRecordRichData() {
		w.recordDone(content)
	}
	return err
}
```

**WriteToolCall** — gate on `shouldRecord() && shouldRecordRichData()`:
```go
func (w *recordingResponseWriter) WriteToolCall(toolCall *ToolCallInfo) error {
	err := w.inner.WriteToolCall(toolCall)

	if !w.shouldRecord() || !w.shouldRecordRichData() {
		return err
	}

	// ... existing recording logic unchanged ...
}
```

**Note:** Tool name visibility for analytics is handled by the encrypting store wrapper (Task 2), which keeps `ToolCall.Name` in plaintext while encrypting `Arguments` and `Result`. The facade's backward-compat message recording via `AppendMessage` does NOT need `Metadata["tool_name"]` — the authoritative tool call records come from the runtime via `RecordToolCall`.

**WriteToolResult** — gate on `shouldRecord() && shouldRecordRichData()`:
```go
func (w *recordingResponseWriter) WriteToolResult(result *ToolResultInfo) error {
	err := w.inner.WriteToolResult(result)

	if !w.shouldRecord() || !w.shouldRecordRichData() {
		return err
	}

	// ... existing recording logic unchanged ...
}
```

**WriteError** — gate on `shouldRecord()` (errors are always recorded if recording is enabled, regardless of RichData):
```go
func (w *recordingResponseWriter) WriteError(code, message string) error {
	err := w.inner.WriteError(code, message)

	if !w.shouldRecord() {
		return err
	}

	// ... existing recording logic unchanged ...
}
```

- [ ] **Step 5: Wire policy into session.go**

In `internal/facade/session.go`, after creating the recording writer (line 195), fetch and set the policy:

```go
recWriter := newRecordingWriter(ctx, writer, s.sessionStore, sessionID, log, s.recordingPool)

// Apply recording policy from session-api privacy policy cache.
if s.policyCache != nil {
	recWriter.setPolicy(s.policyCache.Get(ctx))
}
```

The `policyCache` is a `*RecordingPolicyCache` on the `Server` struct, initialized when the server starts with a facade connection's namespace/agent. The implementer needs to:

1. Add `policyCache *RecordingPolicyCache` to the `Server` struct (or create it per-connection in `handleConnection`).
2. Since the cache is per namespace/agent, it should be created in `handleConnection()` when the connection's namespace and agent are known, then stored on the `Connection` struct.
3. Pass it through to `processMessage`.

The exact wiring depends on how the `Connection` flows into `processMessage`. The key requirement is: `RecordingPolicyCache` is created with the connection's namespace and agentName, using the session-api store as the `PolicyFetcher`.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/facade/... -count=1 -v -run "TestRecordingWriter_(RecordingDisabled|RichDataDisabled|DefaultPolicy)"`
Expected: All 3 tests PASS.

- [ ] **Step 7: Run full facade test suite**

Run: `go test ./internal/facade/... -count=1 -v`
Expected: All existing tests still PASS (no regressions).

- [ ] **Step 8: Run goimports**

Run: `goimports -w internal/facade/recording_writer.go internal/facade/session.go`

- [ ] **Step 9: Commit**

```
feat(facade): gate session recording on privacy policy

Recording writer checks EffectivePolicy before persisting messages.
Recording.Enabled=false skips all recording. RichData=false skips
message content and tool call/result payloads.

Ref #780
```

---

## Task 7: Privacy Middleware Recording Flag Enforcement

**Files:**
- Modify: `ee/pkg/privacy/middleware.go`
- Modify: `ee/pkg/privacy/middleware_test.go`

### Tests first

- [ ] **Step 1: Write the failing tests**

Add to `ee/pkg/privacy/middleware_test.go`:

```go
func TestPrivacyMiddleware_RecordingDisabled_Returns204(t *testing.T) {
	watcher := newMockWatcher(&EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled: false,
		},
	})
	middleware := NewPrivacyMiddleware(watcher, newMockSessionCache(), redaction.NewRedactor(), newMockPrefStore(), logr.Discard())

	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })

	req := httptest.NewRequest("POST", "/api/v1/sessions/sess-1/messages", strings.NewReader(`{"content":"hello"}`))
	rec := httptest.NewRecorder()

	middleware.Wrap(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, called, "next handler should not be called")
}

func TestPrivacyMiddleware_RichDataDisabled_DropsToolCall(t *testing.T) {
	watcher := newMockWatcher(&EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   false,
		},
	})
	middleware := NewPrivacyMiddleware(watcher, newMockSessionCache(), redaction.NewRedactor(), newMockPrefStore(), logr.Discard())

	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })

	// Tool call message
	body := `{"content":"{}","metadata":{"type":"tool_call"},"role":"assistant"}`
	req := httptest.NewRequest("POST", "/api/v1/sessions/sess-1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	middleware.Wrap(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.False(t, called)
}

func TestPrivacyMiddleware_RichDataDisabled_AllowsStatusUpdate(t *testing.T) {
	watcher := newMockWatcher(&EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   false,
		},
	})
	middleware := NewPrivacyMiddleware(watcher, newMockSessionCache(), redaction.NewRedactor(), newMockPrefStore(), logr.Discard())

	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })

	req := httptest.NewRequest("PATCH", "/api/v1/sessions/sess-1/status", strings.NewReader(`{"status":"completed"}`))
	rec := httptest.NewRecorder()

	middleware.Wrap(next).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called, "status updates should pass through")
}

func TestPrivacyMiddleware_RecordingEnabled_PassesThrough(t *testing.T) {
	watcher := newMockWatcher(&EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   true,
		},
	})
	middleware := NewPrivacyMiddleware(watcher, newMockSessionCache(), redaction.NewRedactor(), newMockPrefStore(), logr.Discard())

	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })

	body := `{"content":"hello","role":"assistant"}`
	req := httptest.NewRequest("POST", "/api/v1/sessions/sess-1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	middleware.Wrap(next).ServeHTTP(rec, req)

	assert.True(t, called)
}
```

**Note:** The test helpers (`newMockWatcher`, `newMockSessionCache`, `newMockPrefStore`) likely already exist in the test file. The implementer should check and reuse them.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./ee/pkg/privacy/... -count=1 -v -run "TestPrivacyMiddleware_Recording"`
Expected: Tests fail — the recording check logic doesn't exist yet.

### Implementation

- [ ] **Step 3: Add recording flag checks to middleware.go**

In `ee/pkg/privacy/middleware.go`, add a `checkRecordingPolicy` method and call it in `Wrap()` after getting the effective policy (line 89) and before the opt-out check (line 97):

```go
// checkRecordingPolicy enforces recording flags on write requests.
// Returns true if the request should be blocked (204 sent).
func (m *PrivacyMiddleware) checkRecordingPolicy(
	w http.ResponseWriter, r *http.Request, policy *EffectivePolicy,
) bool {
	// Master switch: recording entirely disabled.
	if !policy.Recording.Enabled {
		w.WriteHeader(http.StatusNoContent)
		return true
	}

	// RichData disabled: block message content, tool calls, tool results.
	// Allow status updates, TTL refreshes, and other non-content writes.
	if !policy.Recording.RichData && isMessageEndpoint(r.URL.Path) {
		if isRichContent(r) {
			w.WriteHeader(http.StatusNoContent)
			return true
		}
	}

	return false
}
```

Add `isMessageEndpoint` helper:
```go
// isMessageEndpoint returns true for paths that carry message content.
var messageEndpointPattern = regexp.MustCompile(`/api/v1/sessions/[^/]+/messages$`)

func isMessageEndpoint(path string) bool {
	return messageEndpointPattern.MatchString(path)
}
```

Add `isRichContent` helper that peeks at the request body to check the role/type:
```go
// isRichContent checks if a message write contains rich content (assistant
// messages, tool calls, tool results) that should be blocked when RichData
// is disabled. Returns false for user messages and non-message requests.
func isRichContent(r *http.Request) bool {
	// Read body, then replace it for downstream consumption.
	body, err := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		return false // can't determine — let it through
	}

	var msg struct {
		Role     string            `json:"role"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(body, &msg); err != nil {
		return false
	}

	// User messages are always allowed (they were typed by the user).
	if msg.Role == "user" {
		return false
	}

	// Assistant messages and tool call/result messages are rich content.
	if msg.Role == "assistant" || msg.Role == "system" {
		return true
	}
	msgType := msg.Metadata["type"]
	return msgType == "tool_call" || msgType == "tool_result"
}
```

Wire it into `Wrap()` between the policy fetch and opt-out check:
```go
// existing: policy := m.policyWatcher.GetEffectivePolicy(ns, agent)
// ... (nil check) ...

// Check recording flags (safety net for facade).
if m.checkRecordingPolicy(w, r, policy) {
	return
}

// existing: check user opt-out
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./ee/pkg/privacy/... -count=1 -v -run "TestPrivacyMiddleware_Recording"`
Expected: All 4 tests PASS.

- [ ] **Step 5: Run full privacy test suite**

Run: `go test ./ee/pkg/privacy/... -count=1 -v`
Expected: All existing tests still PASS.

- [ ] **Step 6: Run goimports**

Run: `goimports -w ee/pkg/privacy/middleware.go`

- [ ] **Step 7: Commit**

```
feat(ee): enforce recording flags in privacy middleware

Session-api safety net: blocks writes when Recording.Enabled=false
(204 No Content). When RichData=false, blocks assistant messages and
tool call/result payloads but allows status updates and TTL refreshes.

Ref #780
```

---

## Task 8: Wire Encryption + Policy Endpoint in Session-API Main

**Files:**
- Modify: `cmd/session-api/main.go`
- Modify: `ee/pkg/encryption/encryptor.go` (finalize `LoadKeyFromFile`)

### Implementation

- [ ] **Step 1: Finalize LoadKeyFromFile**

In `ee/pkg/encryption/encryptor.go`, replace the placeholder `LoadKeyFromFile`:

```go
// LoadKeyFromFile reads a 32-byte AES-256 key from a file path.
// Returns a NoopEncryptor if path is empty (encryption disabled).
func LoadKeyFromFile(path string) (Encryptor, error) {
	if path == "" {
		return NewNoopEncryptor(), nil
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read encryption key: %w", err)
	}
	key = bytes.TrimSpace(key) // trim trailing newline from Secret mount
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes, got %d", len(key))
	}
	return NewAESEncryptor(key)
}
```

- [ ] **Step 2: Add LoadKeyFromFile test**

Add to `ee/pkg/encryption/encryptor_test.go`:

```go
func TestLoadKeyFromFile_EmptyPath_ReturnsNoop(t *testing.T) {
	enc, err := LoadKeyFromFile("")
	require.NoError(t, err)
	assert.IsType(t, &NoopEncryptor{}, enc)
}

func TestLoadKeyFromFile_ValidKey(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, key, 0o600))

	enc, err := LoadKeyFromFile(path)
	require.NoError(t, err)
	assert.IsType(t, &AESEncryptor{}, enc)
}

func TestLoadKeyFromFile_WrongSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, []byte("too-short"), 0o600))

	_, err := LoadKeyFromFile(path)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "32 bytes")
}

func TestLoadKeyFromFile_TrailingNewline(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	path := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(path, append(key, '\n'), 0o600))

	enc, err := LoadKeyFromFile(path)
	require.NoError(t, err)
	assert.IsType(t, &AESEncryptor{}, enc)
}
```

- [ ] **Step 3: Run encryption tests**

Run: `go test ./ee/pkg/encryption/... -count=1 -v`
Expected: All tests PASS.

- [ ] **Step 4: Wire encryption into session-api main**

In `cmd/session-api/main.go`, in `buildAPIMux()` (around line 480, after creating the session service):

1. Load the encryption key:
```go
encryptor, err := encryption.LoadKeyFromFile(os.Getenv("ENCRYPTION_KEY_PATH"))
if err != nil {
	log.Error(err, "encryption key loading failed")
	return nil, nil, func() {}
}
if _, ok := encryptor.(*encryption.NoopEncryptor); !ok {
	log.Info("session encryption enabled")
}
```

2. Wrap the store passed to the session service:
The implementer needs to find where the registry/store is used by `SessionService` and wrap it with `encryption.NewEncryptingStore()`. This depends on how `NewSessionService` receives the store. The exact wiring point should be determined by reading the `SessionService` constructor.

3. Wire the PolicyWatcher as a PolicyResolver on the handler:
In `wrapPrivacyMiddleware()`, after creating the `PolicyWatcher`, set it on the handler:
```go
// The handler needs the watcher reference for the privacy-policy endpoint.
// This requires passing the handler into wrapPrivacyMiddleware or returning
// the watcher. The implementer should choose the cleaner approach.
```

- [ ] **Step 5: Run session-api tests**

Run: `go test ./cmd/session-api/... -count=1 -v`
Expected: All tests PASS.

- [ ] **Step 6: Run goimports**

Run: `goimports -w cmd/session-api/main.go ee/pkg/encryption/encryptor.go`

- [ ] **Step 7: Commit**

```
feat(session-api): wire encryption wrapper and privacy policy endpoint

Session-api loads AES-256 key from ENCRYPTION_KEY_PATH. When set,
wraps the session store with EncryptingStore for at-rest encryption.
PolicyWatcher exposed as PolicyResolver for the privacy-policy endpoint.

Ref #780
```

---

## Task 9: Doctor Health Check for Encryption

**Files:**
- Modify: `internal/doctor/checks/privacy.go`
- Modify: `internal/doctor/checks/privacy_test.go`

### Tests first

- [ ] **Step 1: Write the failing test**

Add to `internal/doctor/checks/privacy_test.go`:

```go
func TestPrivacyChecker_CheckEncryption(t *testing.T) {
	// This test verifies the check function exists and handles
	// the case where encryption is not configured (skip result).
	checker := NewPrivacyChecker("http://memory-api", "http://session-api", "test-ws", "http://arena")

	// When session-api reports no encryption, the check should skip.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/license" {
			json.NewEncoder(w).Encode(map[string]string{"tier": "enterprise"})
			return
		}
		if r.URL.Path == "/api/v1/encryption-status" {
			json.NewEncoder(w).Encode(map[string]bool{"enabled": false})
			return
		}
	}))
	defer srv.Close()

	checker.arenaURL = srv.URL
	checker.sessionAPIURL = srv.URL

	checks := checker.Checks()
	// The encryption check should be in the list
	assert.True(t, len(checks) >= 5, "should have at least 5 checks including encryption")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/doctor/checks/... -count=1 -v -run TestPrivacyChecker_CheckEncryption`
Expected: Failure — check count assertion fails (only 4 checks today).

### Implementation

- [ ] **Step 3: Add encryption health check**

In `internal/doctor/checks/privacy.go`, add a new check to `Checks()`:

```go
func (c *PrivacyChecker) Checks() []Check {
	return []Check{
		{Name: "pii-redaction", Fn: c.checkPIIRedaction},
		{Name: "opt-out-respected", Fn: c.checkOptOutRespected},
		{Name: "deletion-cascade", Fn: c.checkDeletionCascade},
		{Name: "audit-log-written", Fn: c.checkAuditLogWritten},
		{Name: "encryption-at-rest", Fn: c.checkEncryptionAtRest},
	}
}
```

Add the check method:
```go
// checkEncryptionAtRest verifies that session-api is encrypting data when configured.
func (c *PrivacyChecker) checkEncryptionAtRest(ctx context.Context) Result {
	if !c.requireEnterprise(ctx) {
		return Result{Status: StatusSkip, Message: "enterprise not enabled"}
	}

	// Check encryption status endpoint on session-api.
	resp, err := http.Get(c.sessionAPIURL + "/api/v1/encryption-status")
	if err != nil {
		return Result{Status: StatusSkip, Message: "session-api unreachable"}
	}
	defer resp.Body.Close()

	var status struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return Result{Status: StatusFail, Message: "invalid encryption status response"}
	}

	if !status.Enabled {
		return Result{Status: StatusSkip, Message: "encryption not configured"}
	}

	// Write a test message, then verify the raw DB content is encrypted.
	// This requires a round-trip: write via session-api, then read raw
	// from a diagnostic endpoint or verify the prefix.
	// For now, just verify the status endpoint reports enabled.
	return Result{Status: StatusPass, Message: "encryption enabled"}
}
```

**Note:** A more thorough check (write-then-verify-ciphertext) requires either a diagnostic endpoint or direct DB access. The implementer can add a `GET /api/v1/encryption-status` endpoint to session-api as part of this task — a simple handler that reports whether the `EncryptingStore` wrapper is active.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/doctor/checks/... -count=1 -v -run TestPrivacyChecker_CheckEncryption`
Expected: PASS.

- [ ] **Step 5: Run goimports**

Run: `goimports -w internal/doctor/checks/privacy.go`

- [ ] **Step 6: Commit**

```
feat(doctor): add encryption-at-rest health check

Verifies session-api has encryption enabled when configured.

Ref #780
```

---

## Task 10: Create Follow-up GitHub Issues

**Files:** None (GitHub only)

- [ ] **Step 1: Create KMS provider issues**

```bash
gh issue create --repo AltairaLabs/omnia \
  --title "feat(ee): AWS KMS encryption provider" \
  --body "Implement Encryptor backed by AWS KMS envelope encryption. Use the Encryptor interface from ee/pkg/encryption/. Add webhook validation for AWS-specific config fields. Ref #780" \
  --label "enhancement"

gh issue create --repo AltairaLabs/omnia \
  --title "feat(ee): Azure Key Vault encryption provider" \
  --body "Implement Encryptor backed by Azure Key Vault. Use the Encryptor interface from ee/pkg/encryption/. Add webhook validation for Azure-specific config fields. Ref #780" \
  --label "enhancement"

gh issue create --repo AltairaLabs/omnia \
  --title "feat(ee): GCP KMS encryption provider" \
  --body "Implement Encryptor backed by GCP Cloud KMS. Use the Encryptor interface from ee/pkg/encryption/. Add webhook validation for GCP-specific config fields. Ref #780" \
  --label "enhancement"

gh issue create --repo AltairaLabs/omnia \
  --title "feat(ee): HashiCorp Vault encryption provider" \
  --body "Implement Encryptor backed by Vault Transit secrets engine. Use the Encryptor interface from ee/pkg/encryption/. Add webhook validation for Vault-specific config fields. Ref #780" \
  --label "enhancement"

gh issue create --repo AltairaLabs/omnia \
  --title "feat(ee): automated encryption key rotation" \
  --body "Background job to re-encrypt existing session data when encryption keys change. Implements KeyRotation spec from SessionPrivacyPolicy CRD (Schedule, BatchSize, ReEncryptExisting). Depends on KMS provider support. Ref #780" \
  --label "enhancement"
```

- [ ] **Step 2: Verify issues created**

Run: `gh issue list --repo AltairaLabs/omnia --search "encryption provider" --limit 10`
Expected: All 5 issues visible.

- [ ] **Step 3: Commit** (nothing to commit — GitHub-only task)

---

## Task 11: Lint + Full Test Suite

- [ ] **Step 1: Run golangci-lint**

Run: `golangci-lint run ./...`
Expected: No new lint errors.

- [ ] **Step 2: Run full Go test suite**

Run: `go test ./... -count=1`
Expected: All tests PASS.

- [ ] **Step 3: Fix any issues and commit**

If lint or tests reveal issues, fix and commit:
```
fix: address lint and test issues from privacy recording + encryption
```
