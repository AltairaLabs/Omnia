/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package api

// Task 13 (issue #780), Path B: end-to-end integration test for encryption.
//
// These tests wire the real *Handler with the real ee/pkg/encryption.Encryptor
// (driven by an in-memory mock KMS Provider) and verify:
//
//  1. POSTing a message/tool-call/runtime-event lands encrypted envelope data
//     in the fake store (ciphertext, NOT plaintext).
//  2. GET via the handler returns plaintext (round-trip through decrypt).
//
// The mock Provider is deliberately weak — it base64-wraps plaintext so we can
// verify encryption was invoked without depending on a real KMS. The point is
// to assert the handler → encryptor → store pipeline is wired and
// envelope-shaped data reaches the storage layer.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/go-logr/logr"

	eeencryption "github.com/altairalabs/omnia/ee/pkg/encryption"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// --- Mock KMS Provider -------------------------------------------------------

// integrationMockProvider is a minimal in-memory KMS Provider. Ciphertext is
// "enc:" + base64(plaintext); Decrypt reverses it. This is NOT secure — it is
// a deterministic transformation that proves the encryptor path was taken.
type integrationMockProvider struct{}

const integrationMockPrefix = "MOCKENC|"

func (integrationMockProvider) Encrypt(_ context.Context, plaintext []byte) (*eeencryption.EncryptOutput, error) {
	wrapped := append([]byte(integrationMockPrefix), plaintext...)
	return &eeencryption.EncryptOutput{
		Ciphertext: wrapped,
		KeyID:      "mock-key",
		KeyVersion: "1",
		Algorithm:  "MOCK-AES-256",
	}, nil
}

func (integrationMockProvider) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	if !bytes.HasPrefix(ciphertext, []byte(integrationMockPrefix)) {
		return nil, fmt.Errorf("integrationMockProvider: ciphertext missing prefix")
	}
	return ciphertext[len(integrationMockPrefix):], nil
}

func (integrationMockProvider) GetKeyMetadata(context.Context) (*eeencryption.KeyMetadata, error) {
	return &eeencryption.KeyMetadata{KeyID: "mock-key", KeyVersion: "1", Algorithm: "MOCK-AES-256", Enabled: true}, nil
}

func (integrationMockProvider) RotateKey(context.Context) (*eeencryption.KeyRotationResult, error) {
	return &eeencryption.KeyRotationResult{PreviousKeyVersion: "1", NewKeyVersion: "2"}, nil
}

func (integrationMockProvider) Close() error { return nil }

// --- Encryptor adapter: ee/pkg/encryption.Encryptor -> api.Encryptor ---------

// integrationEncryptorAdapter mirrors cmd/session-api/main.go's adapter: it
// strips the []EncryptionEvent return so the value satisfies api.Encryptor.
type integrationEncryptorAdapter struct {
	inner eeencryption.Encryptor
}

func (a *integrationEncryptorAdapter) EncryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error) {
	out, _, err := a.inner.EncryptMessage(ctx, msg)
	return out, err
}
func (a *integrationEncryptorAdapter) DecryptMessage(ctx context.Context, msg *session.Message) (*session.Message, error) {
	return a.inner.DecryptMessage(ctx, msg)
}
func (a *integrationEncryptorAdapter) EncryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error) {
	out, _, err := a.inner.EncryptToolCall(ctx, tc)
	return out, err
}
func (a *integrationEncryptorAdapter) DecryptToolCall(ctx context.Context, tc *session.ToolCall) (*session.ToolCall, error) {
	return a.inner.DecryptToolCall(ctx, tc)
}
func (a *integrationEncryptorAdapter) EncryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error) {
	out, _, err := a.inner.EncryptRuntimeEvent(ctx, evt)
	return out, err
}
func (a *integrationEncryptorAdapter) DecryptRuntimeEvent(ctx context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error) {
	return a.inner.DecryptRuntimeEvent(ctx, evt)
}

// --- Capturing warm store ----------------------------------------------------

// capturingWarmStore wraps mockWarmStore so we can capture what the handler
// passes to RecordToolCall / RecordRuntimeEvent (the base mock drops those
// arguments on the floor).
type capturingWarmStore struct {
	*mockWarmStore

	mu                sync.Mutex
	capturedToolCalls map[string][]*session.ToolCall
	capturedEvents    map[string][]*session.RuntimeEvent
}

func newCapturingWarmStore(base *mockWarmStore) *capturingWarmStore {
	return &capturingWarmStore{
		mockWarmStore:     base,
		capturedToolCalls: make(map[string][]*session.ToolCall),
		capturedEvents:    make(map[string][]*session.RuntimeEvent),
	}
}

// AppendMessage mirrors the base behavior but also routes stored messages into
// the `messages` map that GetMessages reads from, so round-trip POST→GET works.
func (c *capturingWarmStore) AppendMessage(ctx context.Context, sessionID string, msg *session.Message) error {
	if err := c.mockWarmStore.AppendMessage(ctx, sessionID, msg); err != nil {
		return err
	}
	c.mu.Lock()
	c.messages[sessionID] = append(c.messages[sessionID], msg)
	c.mu.Unlock()
	return nil
}

func (c *capturingWarmStore) RecordToolCall(ctx context.Context, sessionID string, tc *session.ToolCall) error {
	if err := c.mockWarmStore.RecordToolCall(ctx, sessionID, tc); err != nil {
		return err
	}
	c.mu.Lock()
	c.capturedToolCalls[sessionID] = append(c.capturedToolCalls[sessionID], tc)
	// Also push into mockWarmStore.toolCalls so subsequent GETs return it.
	c.toolCalls[sessionID] = append(c.toolCalls[sessionID], tc)
	c.mu.Unlock()
	return nil
}

func (c *capturingWarmStore) RecordRuntimeEvent(ctx context.Context, sessionID string, evt *session.RuntimeEvent) error {
	if err := c.mockWarmStore.RecordRuntimeEvent(ctx, sessionID, evt); err != nil {
		return err
	}
	c.mu.Lock()
	c.capturedEvents[sessionID] = append(c.capturedEvents[sessionID], evt)
	c.runtimeEvents[sessionID] = append(c.runtimeEvents[sessionID], evt)
	c.mu.Unlock()
	return nil
}

// --- Test harness ------------------------------------------------------------

// setupEncryptionIntegration builds a real Handler wired to a real
// ee/pkg/encryption.Encryptor and a capturing in-memory store.
func setupEncryptionIntegration(t *testing.T) (http.Handler, *capturingWarmStore) {
	t.Helper()
	_, _, warm := setupHandler(t)
	warm.sessions[testSessionID] = testSession(testSessionID)

	// Swap in a capturing warm store by replacing the service's registry entry.
	cap := newCapturingWarmStore(warm)
	reg := providers.NewRegistry()
	reg.SetHotCache(newMockHotCache())
	reg.SetWarmStore(cap)
	reg.SetColdArchive(newMockColdArchive())

	log := logr.Discard()
	svc := NewSessionService(reg, ServiceConfig{}, log)
	newH := NewHandler(svc, log)

	encryptor := eeencryption.NewEncryptor(integrationMockProvider{})
	newH.SetEncryptor(&integrationEncryptorAdapter{inner: encryptor})

	// Make sure the session exists in the capturing store too.
	cap.sessions[testSessionID] = testSession(testSessionID)

	mux := http.NewServeMux()
	newH.RegisterRoutes(mux)
	return mux, cap
}

// --- Tests -------------------------------------------------------------------

// TestEncryptionIntegration_Message_EnvelopeStored verifies that POSTing a
// message stores ciphertext in the warm store and GET returns plaintext.
func TestEncryptionIntegration_Message_EnvelopeStored(t *testing.T) {
	mux, cap := setupEncryptionIntegration(t)

	plaintext := "my SSN is 123-45-6789"
	body := fmt.Sprintf(`{"id":"m1","role":"user","content":%q,"sequenceNum":1}`, plaintext)

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/sessions/"+testSessionID+"/messages",
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Assert what the store received is encrypted (not plaintext).
	stored := cap.appendedMsgs[testSessionID]
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(stored))
	}
	if stored[0].Content == plaintext {
		t.Fatalf("store received plaintext content; expected encrypted envelope")
	}
	// The envelope marker lives in message metadata under "_encryption".
	if _, ok := stored[0].Metadata["_encryption"]; !ok {
		t.Fatalf("expected _encryption envelope marker in stored message metadata, got %+v",
			stored[0].Metadata)
	}
	// Ciphertext should base64-decode to our mock-wrapped plaintext.
	decoded, err := base64.StdEncoding.DecodeString(stored[0].Content)
	if err != nil {
		t.Fatalf("stored content not base64: %v", err)
	}
	if !bytes.HasPrefix(decoded, []byte(integrationMockPrefix)) {
		t.Fatalf("stored content missing mock-encryption prefix: %q", decoded)
	}

	// GET should decrypt and return the plaintext.
	getReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/sessions/"+testSessionID+"/messages", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	resp := decodeJSON[MessagesResponse](t, getRec)
	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Content != plaintext {
		t.Fatalf("GET did not return decrypted plaintext; got %q want %q",
			resp.Messages[0].Content, plaintext)
	}
}

// TestEncryptionIntegration_ToolCall_EnvelopeStored verifies that ToolCall
// Arguments and Result are stored as encrypted envelopes and round-trip
// cleanly on GET.
func TestEncryptionIntegration_ToolCall_EnvelopeStored(t *testing.T) {
	mux, cap := setupEncryptionIntegration(t)

	body := `{
		"id":"tc1",
		"name":"lookup",
		"status":"success",
		"arguments":{"query":"alice@example.com"},
		"result":{"rows":[{"id":"42","ssn":"123-45-6789"}]},
		"errorMessage":"boom"
	}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/sessions/"+testSessionID+"/tool-calls",
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	captured := cap.capturedToolCalls[testSessionID]
	if len(captured) != 1 {
		t.Fatalf("expected 1 captured tool call, got %d", len(captured))
	}
	tc := captured[0]

	// Arguments should be an envelope map with the _encryption + _payload keys.
	assertEnvelopeMap(t, "arguments", tc.Arguments)
	// Result is stored as any; if it's a map it should be an envelope.
	resultMap, ok := tc.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected Result to be an envelope map, got %T", tc.Result)
	}
	assertEnvelopeMap(t, "result", resultMap)
	// ErrorMessage should carry the enc:v1: sentinel prefix, not plaintext.
	if tc.ErrorMessage == "boom" {
		t.Fatalf("error message was stored as plaintext")
	}
	if len(tc.ErrorMessage) == 0 || !bytes.HasPrefix([]byte(tc.ErrorMessage), []byte("enc:v1:")) {
		t.Fatalf("expected enc:v1: prefix on stored error message, got %q", tc.ErrorMessage)
	}

	// GET round-trips back to plaintext.
	getReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/sessions/"+testSessionID+"/tool-calls", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	tcs := decodeJSON[[]*session.ToolCall](t, getRec)
	if len(tcs) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(tcs))
	}
	got := tcs[0]
	if got.ErrorMessage != "boom" {
		t.Fatalf("errorMessage not decrypted; got %q", got.ErrorMessage)
	}
	if got.Arguments["query"] != "alice@example.com" {
		t.Fatalf("arguments not decrypted; got %+v", got.Arguments)
	}
}

// TestEncryptionIntegration_RuntimeEvent_EnvelopeStored verifies RuntimeEvent
// Data and ErrorMessage are encrypted end-to-end.
func TestEncryptionIntegration_RuntimeEvent_EnvelopeStored(t *testing.T) {
	mux, cap := setupEncryptionIntegration(t)

	body := `{
		"id":"e1",
		"eventType":"pipeline.started",
		"data":{"prompt":"secret input"},
		"errorMessage":"fatal"
	}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/sessions/"+testSessionID+"/events",
		bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	captured := cap.capturedEvents[testSessionID]
	if len(captured) != 1 {
		t.Fatalf("expected 1 captured runtime event, got %d", len(captured))
	}
	evt := captured[0]

	assertEnvelopeMap(t, "data", evt.Data)
	if evt.ErrorMessage == "fatal" {
		t.Fatalf("error message stored as plaintext")
	}

	// GET round-trip.
	getReq := httptest.NewRequest(http.MethodGet,
		"/api/v1/sessions/"+testSessionID+"/events", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", getRec.Code, getRec.Body.String())
	}
	evts := decodeJSON[[]*session.RuntimeEvent](t, getRec)
	if len(evts) != 1 {
		t.Fatalf("expected 1 event, got %d", len(evts))
	}
	if evts[0].ErrorMessage != "fatal" {
		t.Fatalf("errorMessage not decrypted; got %q", evts[0].ErrorMessage)
	}
	if evts[0].Data["prompt"] != "secret input" {
		t.Fatalf("data not decrypted; got %+v", evts[0].Data)
	}
}

// assertEnvelopeMap checks that a map[string]any carries the _encryption +
// _payload envelope markers produced by ee/pkg/encryption.
func assertEnvelopeMap(t *testing.T, field string, m map[string]any) {
	t.Helper()
	if m == nil {
		t.Fatalf("%s: envelope map is nil", field)
	}
	if _, ok := m["_encryption"]; !ok {
		t.Fatalf("%s: missing _encryption key; got %+v", field, m)
	}
	if _, ok := m["_payload"]; !ok {
		t.Fatalf("%s: missing _payload key; got %+v", field, m)
	}
}
