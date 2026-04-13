/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// --- Encryption test fixtures ---

// mockEncryptor XOR-encrypts with a fixed key byte, producing deterministic
// ciphertext. Used to verify that encrypted fields are NOT plaintext and that
// round-trips restore the original value.
type mockEncryptor struct {
	key byte
}

func (m mockEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	out := make([]byte, len(plaintext))
	for i, b := range plaintext {
		out[i] = b ^ m.key
	}
	return out, nil
}

func (m mockEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	return m.Encrypt(ciphertext) // XOR is its own inverse
}

// errEncryptor always fails.
type errEncryptor struct{}

func (errEncryptor) Encrypt(_ []byte) ([]byte, error) { return nil, errors.New("encrypt error") }
func (errEncryptor) Decrypt(_ []byte) ([]byte, error) { return nil, errors.New("decrypt error") }

// mockResolver returns encryptors keyed by session ID.
type mockResolver struct {
	encryptors map[string]Encryptor
}

func (m *mockResolver) EncryptorForSession(id string) (Encryptor, bool) {
	enc, ok := m.encryptors[id]
	return enc, ok
}

// encResolverFor builds a simple resolver with one encryptor per session ID.
func encResolverFor(pairs ...any) *mockResolver {
	r := &mockResolver{encryptors: make(map[string]Encryptor)}
	for i := 0; i+1 < len(pairs); i += 2 {
		id := pairs[i].(string)
		enc := pairs[i+1].(Encryptor)
		r.encryptors[id] = enc
	}
	return r
}

// --- Capturing warm store (extends mockWarmStore behaviour for tool calls / events) ---

// capturingWarmStore wraps mockWarmStore and records written tool calls and
// runtime events so tests can inspect the ciphertext that reaches storage.
type capturingWarmStore struct {
	*mockWarmStore
	capturedToolCalls     []*session.ToolCall
	capturedRuntimeEvents []*session.RuntimeEvent
}

func newCapturingWarmStore() *capturingWarmStore {
	return &capturingWarmStore{mockWarmStore: newMockWarmStore()}
}

func (c *capturingWarmStore) RecordToolCall(_ context.Context, sessionID string, tc *session.ToolCall) error {
	if _, ok := c.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	clone := *tc
	c.capturedToolCalls = append(c.capturedToolCalls, &clone)
	return nil
}

func (c *capturingWarmStore) RecordRuntimeEvent(_ context.Context, sessionID string, evt *session.RuntimeEvent) error {
	if _, ok := c.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	clone := *evt
	c.capturedRuntimeEvents = append(c.capturedRuntimeEvents, &clone)
	return nil
}

// --- Test helpers ---

func setupEncryptionHandler(t *testing.T) (*Handler, *capturingWarmStore) {
	t.Helper()
	warm := newCapturingWarmStore()

	reg := providers.NewRegistry()
	reg.SetWarmStore(warm)

	svc := NewSessionService(reg, ServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	return h, warm
}

// newEncRequest POSTs body to path.
func newEncRequest(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewBufferString(body))
}

// serveMux returns a mux with all handler routes registered.
func serveMux(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// decodeBodyJSON decodes the response body into T.
func decodeBodyJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(rec.Body).Decode(&v); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return v
}

// encryptedString returns the enc:v1: representation that encryptor m would
// produce for plaintext s.
func encryptedString(t *testing.T, m mockEncryptor, s string) string {
	t.Helper()
	ct, err := m.Encrypt([]byte(s))
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return errMsgEncPrefix + base64.StdEncoding.EncodeToString(ct)
}

// --- Test IDs ---

const (
	encSessionA = "aaaa0000-0000-0000-0000-000000000001"
	encSessionB = "bbbb0000-0000-0000-0000-000000000002"
	encSessionC = "cccc0000-0000-0000-0000-000000000003"

	appendMessageBody = `{"id":"m1","role":"user","content":"hello","sequenceNum":1}`
	plainContent      = "hello"
)

// --- Tests ---

// TestHandleAppendMessage_EncryptsContent verifies that the content stored in
// the warm store has the enc:v1: prefix when an encryptor is configured.
func TestHandleAppendMessage_EncryptsContent(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)

	enc := mockEncryptor{key: 0x5A}
	h.SetEncryptorResolver(encResolverFor(encSessionA, enc))

	mux := serveMux(h)
	body := appendMessageBody
	req := newEncRequest(http.MethodPost, "/api/v1/sessions/"+encSessionA+"/messages", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	stored := warm.appendedMsgs[encSessionA]
	if len(stored) != 1 {
		t.Fatalf("expected 1 appended message, got %d", len(stored))
	}
	if stored[0].Content == plainContent {
		t.Fatal("content was stored as plaintext — expected ciphertext")
	}
	if !strings.HasPrefix(stored[0].Content, errMsgEncPrefix) {
		t.Fatalf("expected enc:v1: prefix, got %q", stored[0].Content)
	}
	want := encryptedString(t, enc, plainContent)
	if stored[0].Content != want {
		t.Fatalf("ciphertext mismatch: got %q, want %q", stored[0].Content, want)
	}
}

// TestHandleAppendMessage_DifferentEncryptorsProduceDifferentCiphertext verifies
// that two sessions with different encryptors produce different stored bytes.
func TestHandleAppendMessage_DifferentEncryptorsProduceDifferentCiphertext(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)
	warm.sessions[encSessionB] = testSession(encSessionB)

	encA := mockEncryptor{key: 0x11}
	encB := mockEncryptor{key: 0x22}
	h.SetEncryptorResolver(encResolverFor(encSessionA, encA, encSessionB, encB))

	mux := serveMux(h)

	body := appendMessageBody

	recA := httptest.NewRecorder()
	mux.ServeHTTP(recA, newEncRequest(http.MethodPost, "/api/v1/sessions/"+encSessionA+"/messages", body))
	if recA.Code != http.StatusCreated {
		t.Fatalf("session A: expected 201, got %d", recA.Code)
	}

	recB := httptest.NewRecorder()
	mux.ServeHTTP(recB, newEncRequest(http.MethodPost, "/api/v1/sessions/"+encSessionB+"/messages", body))
	if recB.Code != http.StatusCreated {
		t.Fatalf("session B: expected 201, got %d", recB.Code)
	}

	storedA := warm.appendedMsgs[encSessionA][0].Content
	storedB := warm.appendedMsgs[encSessionB][0].Content

	if storedA == storedB {
		t.Fatal("expected different ciphertext for different encryptors, got the same")
	}
	if storedA == plainContent || storedB == plainContent {
		t.Fatal("at least one session stored plaintext")
	}
}

// TestHandleAppendMessage_PlaintextPassthroughWhenNoEncryptor verifies that
// when no encryptor applies to the session, the content is stored as-is.
func TestHandleAppendMessage_PlaintextPassthroughWhenNoEncryptor(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionC] = testSession(encSessionC)

	// Resolver returns false for session C.
	h.SetEncryptorResolver(encResolverFor()) // empty — no sessions have encryptors

	mux := serveMux(h)
	body := appendMessageBody
	req := newEncRequest(http.MethodPost, "/api/v1/sessions/"+encSessionC+"/messages", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	stored := warm.appendedMsgs[encSessionC]
	if len(stored) == 0 {
		t.Fatal("no message appended")
	}
	if stored[0].Content != plainContent {
		t.Fatalf("expected plaintext, got %q", stored[0].Content)
	}
}

// TestHandleGetMessages_DecryptsContent verifies that messages returned to the
// caller are decrypted when an encryptor is configured.
func TestHandleGetMessages_DecryptsContent(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)

	enc := mockEncryptor{key: 0x5A}
	h.SetEncryptorResolver(encResolverFor(encSessionA, enc))

	// Store pre-encrypted content.
	encrypted := encryptedString(t, enc, "hello world")
	warm.messages[encSessionA] = []*session.Message{
		{ID: "m1", Content: encrypted, SequenceNum: 1},
	}

	mux := serveMux(h)
	req := newEncRequest(http.MethodGet, "/api/v1/sessions/"+encSessionA+"/messages", "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeBodyJSON[MessagesResponse](t, rec)
	if len(resp.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Content != "hello world" {
		t.Fatalf("expected decrypted content %q, got %q", "hello world", resp.Messages[0].Content)
	}
}

// TestHandleGetMessages_PlaintextPassthroughWhenNoEncryptor verifies that
// plaintext messages are returned unchanged when no encryptor applies.
func TestHandleGetMessages_PlaintextPassthroughWhenNoEncryptor(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionC] = testSession(encSessionC)
	warm.messages[encSessionC] = []*session.Message{
		{ID: "m1", Content: plainContent, SequenceNum: 1},
	}

	// No encryptor resolver set.
	mux := serveMux(h)
	req := newEncRequest(http.MethodGet, "/api/v1/sessions/"+encSessionC+"/messages", "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	resp := decodeBodyJSON[MessagesResponse](t, rec)
	if len(resp.Messages) == 0 {
		t.Fatal("no messages returned")
	}
	if resp.Messages[0].Content != plainContent {
		t.Fatalf("expected plaintext, got %q", resp.Messages[0].Content)
	}
}

// TestHandleRecordToolCall_EncryptsFields verifies that Arguments, Result, and
// ErrorMessage are encrypted and Name stays plaintext in the stored tool call.
func TestHandleRecordToolCall_EncryptsFields(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)

	enc := mockEncryptor{key: 0x7F}
	h.SetEncryptorResolver(encResolverFor(encSessionA, enc))

	mux := serveMux(h)
	body := `{"id":"tc1","name":"myTool","arguments":{"x":1},"result":"done","errorMessage":"oops","status":"success"}`
	req := newEncRequest(http.MethodPost, "/api/v1/sessions/"+encSessionA+"/tool-calls", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(warm.capturedToolCalls) == 0 {
		t.Fatal("no tool call captured in store")
	}
	stored := warm.capturedToolCalls[0]

	// Name stays plaintext.
	if stored.Name != "myTool" {
		t.Fatalf("expected Name=myTool, got %q", stored.Name)
	}

	// Arguments must be an envelope map.
	if stored.Arguments == nil {
		t.Fatal("expected encrypted Arguments, got nil")
	}
	if _, ok := stored.Arguments[envelopePayloadKey]; !ok {
		t.Fatalf("Arguments not an envelope: %v", stored.Arguments)
	}

	// Result must be an envelope.
	if stored.Result == nil {
		t.Fatal("expected encrypted Result, got nil")
	}
	resultMap, ok := stored.Result.(map[string]any)
	if !ok || resultMap[envelopePayloadKey] == nil {
		t.Fatalf("Result not an envelope: %v", stored.Result)
	}

	// ErrorMessage uses enc:v1: prefix.
	if !strings.HasPrefix(stored.ErrorMessage, errMsgEncPrefix) {
		t.Fatalf("expected enc:v1: prefix on ErrorMessage, got %q", stored.ErrorMessage)
	}
}

// TestHandleGetToolCalls_DecryptsFields verifies that tool calls returned to the
// caller have their Arguments, Result, and ErrorMessage decrypted.
func TestHandleGetToolCalls_DecryptsFields(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)

	enc := mockEncryptor{key: 0x7F}
	h.SetEncryptorResolver(encResolverFor(encSessionA, enc))

	// Build a pre-encrypted tool call.
	plainArgs := map[string]any{"x": float64(1)}
	argsEnc, err := encryptEnvelope(enc, plainArgs)
	if err != nil {
		t.Fatalf("encryptEnvelope: %v", err)
	}
	errMsgEnc := encryptedString(t, enc, "oops")

	warm.toolCalls[encSessionA] = []*session.ToolCall{
		{
			ID:           "tc1",
			Name:         "myTool",
			Arguments:    argsEnc,
			ErrorMessage: errMsgEnc,
			CreatedAt:    time.Now(),
		},
	}

	mux := serveMux(h)
	req := newEncRequest(http.MethodGet, "/api/v1/sessions/"+encSessionA+"/tool-calls", "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []*session.ToolCall
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(items))
	}
	tc := items[0]

	if tc.Name != "myTool" {
		t.Fatalf("expected Name=myTool, got %q", tc.Name)
	}
	if tc.ErrorMessage != "oops" {
		t.Fatalf("expected decrypted ErrorMessage, got %q", tc.ErrorMessage)
	}

	// Arguments should be the original map (JSON round-trip).
	argsJSON, err := json.Marshal(tc.Arguments)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	var gotArgs map[string]any
	if err := json.Unmarshal(argsJSON, &gotArgs); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if gotArgs["x"] != float64(1) {
		t.Fatalf("expected x=1 in Arguments, got %v", gotArgs)
	}
}

// TestHandleRecordRuntimeEvent_EncryptsFields verifies Data and ErrorMessage
// are encrypted; EventType stays plaintext.
func TestHandleRecordRuntimeEvent_EncryptsFields(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)

	enc := mockEncryptor{key: 0x33}
	h.SetEncryptorResolver(encResolverFor(encSessionA, enc))

	mux := serveMux(h)
	body := `{"id":"e1","eventType":"pipeline.started","data":{"trace":"abc"},"errorMessage":"boom"}`
	req := newEncRequest(http.MethodPost, "/api/v1/sessions/"+encSessionA+"/events", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(warm.capturedRuntimeEvents) == 0 {
		t.Fatal("no runtime event captured")
	}
	stored := warm.capturedRuntimeEvents[0]

	if stored.EventType != "pipeline.started" {
		t.Fatalf("expected EventType plaintext, got %q", stored.EventType)
	}
	if stored.Data == nil {
		t.Fatal("expected encrypted Data, got nil")
	}
	if _, ok := stored.Data[envelopePayloadKey]; !ok {
		t.Fatalf("Data not an envelope: %v", stored.Data)
	}
	if !strings.HasPrefix(stored.ErrorMessage, errMsgEncPrefix) {
		t.Fatalf("expected enc:v1: prefix on ErrorMessage, got %q", stored.ErrorMessage)
	}
}

// TestHandleGetRuntimeEvents_DecryptsFields verifies Data and ErrorMessage are
// decrypted; EventType stays plaintext.
func TestHandleGetRuntimeEvents_DecryptsFields(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)

	enc := mockEncryptor{key: 0x33}
	h.SetEncryptorResolver(encResolverFor(encSessionA, enc))

	dataEnc, err := encryptEnvelope(enc, map[string]any{"trace": "abc"})
	if err != nil {
		t.Fatalf("encryptEnvelope: %v", err)
	}
	errMsgEnc := encryptedString(t, enc, "boom")

	warm.runtimeEvents[encSessionA] = []*session.RuntimeEvent{
		{
			ID:           "e1",
			EventType:    "pipeline.started",
			Data:         dataEnc,
			ErrorMessage: errMsgEnc,
			Timestamp:    time.Now(),
		},
	}

	mux := serveMux(h)
	req := newEncRequest(http.MethodGet, "/api/v1/sessions/"+encSessionA+"/events", "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []*session.RuntimeEvent
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 event, got %d", len(items))
	}
	evt := items[0]

	if evt.EventType != "pipeline.started" {
		t.Fatalf("expected EventType plaintext, got %q", evt.EventType)
	}
	if evt.ErrorMessage != "boom" {
		t.Fatalf("expected decrypted ErrorMessage, got %q", evt.ErrorMessage)
	}
	if evt.Data["trace"] != "abc" {
		t.Fatalf("expected decrypted Data trace=abc, got %v", evt.Data)
	}
}

// TestDecryptString_LegacyPlaintextPassthrough verifies that strings without
// the enc:v1: prefix (legacy plaintext rows) are returned unchanged.
func TestDecryptString_LegacyPlaintextPassthrough(t *testing.T) {
	enc := mockEncryptor{key: 0xAB}
	got, err := decryptString(enc, "plaintext without prefix")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "plaintext without prefix" {
		t.Fatalf("expected unchanged string, got %q", got)
	}
}

// TestDecryptEnvelopeMap_LegacyPlaintextPassthrough verifies that non-envelope
// maps (legacy plaintext) are returned unchanged.
func TestDecryptEnvelopeMap_LegacyPlaintextPassthrough(t *testing.T) {
	enc := mockEncryptor{key: 0xAB}
	plain := map[string]any{"k": "v"}
	got, err := decryptEnvelopeMap(enc, plain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", plain) {
		t.Fatalf("expected unchanged map, got %v", got)
	}
}

// TestEncryptDecryptRoundTrip_Message verifies a full encrypt–store–decrypt
// cycle for a message.
func TestEncryptDecryptRoundTrip_Message(t *testing.T) {
	enc := mockEncryptor{key: 0xAA}
	msg := &session.Message{ID: "m1", Content: "secret content"}
	if err := encryptMessage(enc, msg); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if msg.Content == "secret content" {
		t.Fatal("content not encrypted")
	}
	if err := decryptMessage(enc, msg); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if msg.Content != "secret content" {
		t.Fatalf("round-trip failed: got %q", msg.Content)
	}
}

// TestEncryptDecryptRoundTrip_ToolCall verifies a full round-trip for all
// encrypted tool call fields.
func TestEncryptDecryptRoundTrip_ToolCall(t *testing.T) {
	enc := mockEncryptor{key: 0xBB}
	tc := &session.ToolCall{
		ID:           "tc1",
		Name:         "myTool",
		Arguments:    map[string]any{"a": float64(1)},
		Result:       "success",
		ErrorMessage: "err",
	}
	if err := encryptToolCall(enc, tc); err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Name stays plaintext.
	if tc.Name != "myTool" {
		t.Fatalf("Name mutated: %q", tc.Name)
	}
	// Arguments must be an envelope.
	if _, ok := tc.Arguments[envelopePayloadKey]; !ok {
		t.Fatalf("Arguments not an envelope after encrypt: %v", tc.Arguments)
	}
	// ErrorMessage must have prefix.
	if !strings.HasPrefix(tc.ErrorMessage, errMsgEncPrefix) {
		t.Fatalf("ErrorMessage not encrypted: %q", tc.ErrorMessage)
	}

	if err := decryptToolCall(enc, tc); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if tc.Arguments["a"] != float64(1) {
		t.Fatalf("Arguments round-trip: got %v", tc.Arguments)
	}
	if tc.Result != "success" {
		t.Fatalf("Result round-trip: got %v", tc.Result)
	}
	if tc.ErrorMessage != "err" {
		t.Fatalf("ErrorMessage round-trip: got %q", tc.ErrorMessage)
	}
}

// TestEncryptDecryptRoundTrip_RuntimeEvent verifies a full round-trip for all
// encrypted runtime event fields.
func TestEncryptDecryptRoundTrip_RuntimeEvent(t *testing.T) {
	enc := mockEncryptor{key: 0xCC}
	evt := &session.RuntimeEvent{
		ID:           "e1",
		EventType:    "pipeline.started",
		Data:         map[string]any{"trace": "xyz"},
		ErrorMessage: "boom",
	}
	if err := encryptRuntimeEvent(enc, evt); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if evt.EventType != "pipeline.started" {
		t.Fatalf("EventType mutated: %q", evt.EventType)
	}
	if _, ok := evt.Data[envelopePayloadKey]; !ok {
		t.Fatalf("Data not an envelope after encrypt: %v", evt.Data)
	}
	if !strings.HasPrefix(evt.ErrorMessage, errMsgEncPrefix) {
		t.Fatalf("ErrorMessage not encrypted: %q", evt.ErrorMessage)
	}

	if err := decryptRuntimeEvent(enc, evt); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if evt.Data["trace"] != "xyz" {
		t.Fatalf("Data round-trip: got %v", evt.Data)
	}
	if evt.ErrorMessage != "boom" {
		t.Fatalf("ErrorMessage round-trip: got %q", evt.ErrorMessage)
	}
}

// TestHandleAppendMessage_EncryptError_Returns500 verifies that an encryption
// failure returns a 5xx response and the message is not stored.
func TestHandleAppendMessage_EncryptError_Returns500(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)

	h.SetEncryptorResolver(EncryptorResolverFunc(func(_ string) (Encryptor, bool) {
		return errEncryptor{}, true
	}))

	mux := serveMux(h)
	body := appendMessageBody
	req := newEncRequest(http.MethodPost, "/api/v1/sessions/"+encSessionA+"/messages", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code < 500 {
		t.Fatalf("expected 5xx, got %d", rec.Code)
	}
	// Nothing should have been stored.
	if len(warm.appendedMsgs[encSessionA]) != 0 {
		t.Fatal("message was stored despite encryption failure")
	}
}

// TestHandleGetMessages_DecryptError_Returns500 verifies that a decryption
// failure during a GET propagates as a 5xx response.
func TestHandleGetMessages_DecryptError_Returns500(t *testing.T) {
	h, warm := setupEncryptionHandler(t)
	warm.sessions[encSessionA] = testSession(encSessionA)

	// Store a value that looks encrypted (has the prefix) but an errEncryptor
	// will fail to decrypt it.
	warm.messages[encSessionA] = []*session.Message{
		{ID: "m1", Content: errMsgEncPrefix + "aW52YWxpZA==", SequenceNum: 1},
	}

	h.SetEncryptorResolver(EncryptorResolverFunc(func(_ string) (Encryptor, bool) {
		return errEncryptor{}, true
	}))

	mux := serveMux(h)
	req := newEncRequest(http.MethodGet, "/api/v1/sessions/"+encSessionA+"/messages", "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code < 500 {
		t.Fatalf("expected 5xx, got %d", rec.Code)
	}
}

// TestHandlerEncryptorForSession_NilResolverReturnsNil verifies encryptorFor
// returns nil when no resolver is set.
func TestHandlerEncryptorForSession_NilResolverReturnsNil(t *testing.T) {
	h, _ := setupEncryptionHandler(t)
	if enc := h.encryptorFor("any-session"); enc != nil {
		t.Fatalf("expected nil encryptor, got %v", enc)
	}
}

// TestHandlerEncryptorForSession_ResolverReturnsFalse verifies encryptorFor
// returns nil when the resolver returns (nil, false).
func TestHandlerEncryptorForSession_ResolverReturnsFalse(t *testing.T) {
	h, _ := setupEncryptionHandler(t)
	h.SetEncryptorResolver(EncryptorResolverFunc(func(_ string) (Encryptor, bool) {
		return nil, false
	}))
	if enc := h.encryptorFor("any-session"); enc != nil {
		t.Fatalf("expected nil encryptor, got %v", enc)
	}
}

// --- Additional unit tests for helper coverage ---

// TestEncryptEnvelope_NilEncNilV verifies that encryptEnvelope(nil, nil) returns nil.
func TestEncryptEnvelope_NilEncNilV(t *testing.T) {
	m, err := encryptEnvelope(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Fatalf("expected nil map, got %v", m)
	}
}

// TestEncryptEnvelope_NilEncWithMap verifies that when enc is nil, a map[string]any
// is returned unchanged.
func TestEncryptEnvelope_NilEncWithMap(t *testing.T) {
	plain := map[string]any{"k": "v"}
	m, err := encryptEnvelope(nil, plain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["k"] != "v" {
		t.Fatalf("expected map passthrough, got %v", m)
	}
}

// TestEncryptEnvelope_NilV verifies that encryptEnvelope(enc, nil) returns nil.
func TestEncryptEnvelope_NilV(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	m, err := encryptEnvelope(enc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Fatalf("expected nil map, got %v", m)
	}
}

// TestEncryptEnvelope_EncryptError verifies that encrypt failures surface.
func TestEncryptEnvelope_EncryptError(t *testing.T) {
	_, err := encryptEnvelope(errEncryptor{}, map[string]any{"k": "v"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestDecryptEnvelopeMap_EncryptError verifies that decrypt failures surface.
func TestDecryptEnvelopeMap_EncryptError(t *testing.T) {
	enc := errEncryptor{}
	// Build a valid-looking envelope so we get past the isEnvelopeMap check.
	envelope := map[string]any{
		envelopeMarkerKey:  true,
		envelopePayloadKey: base64.StdEncoding.EncodeToString([]byte("fake")),
	}
	_, err := decryptEnvelopeMap(enc, envelope)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestDecryptEnvelopeAny_NonMap verifies that non-map values pass through.
func TestDecryptEnvelopeAny_NonMap(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	got, err := decryptEnvelopeAny(enc, "a string")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "a string" {
		t.Fatalf("expected passthrough, got %v", got)
	}
}

// TestDecryptEnvelopeAny_EncryptError verifies that decrypt failures surface.
func TestDecryptEnvelopeAny_EncryptError(t *testing.T) {
	enc := errEncryptor{}
	envelope := map[string]any{
		envelopeMarkerKey:  true,
		envelopePayloadKey: base64.StdEncoding.EncodeToString([]byte("fake")),
	}
	_, err := decryptEnvelopeAny(enc, envelope)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestEncryptToolCall_NilFields verifies that nil Arguments and Result do not
// panic and produce no envelope.
func TestEncryptToolCall_NilFields(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	tc := &session.ToolCall{ID: "tc1", Name: "tool"}
	if err := encryptToolCall(enc, tc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tc.Arguments != nil {
		t.Fatalf("expected nil Arguments, got %v", tc.Arguments)
	}
	if tc.Result != nil {
		t.Fatalf("expected nil Result, got %v", tc.Result)
	}
}

// TestEncryptToolCall_ErrorOnArgumentsEncryptFail verifies error propagation.
func TestEncryptToolCall_ErrorOnArgumentsEncryptFail(t *testing.T) {
	tc := &session.ToolCall{
		ID:        "tc1",
		Arguments: map[string]any{"x": 1},
	}
	if err := encryptToolCall(errEncryptor{}, tc); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestEncryptRuntimeEvent_NilFields verifies that nil Data does not panic.
func TestEncryptRuntimeEvent_NilFields(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	evt := &session.RuntimeEvent{ID: "e1", EventType: "test.event"}
	if err := encryptRuntimeEvent(enc, evt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Data != nil {
		t.Fatalf("expected nil Data, got %v", evt.Data)
	}
}

// TestDecryptRuntimeEvent_ErrorOnDataDecryptFail verifies error propagation.
func TestDecryptRuntimeEvent_ErrorOnDataDecryptFail(t *testing.T) {
	envelope := map[string]any{
		envelopeMarkerKey:  true,
		envelopePayloadKey: base64.StdEncoding.EncodeToString([]byte("fake")),
	}
	evt := &session.RuntimeEvent{Data: envelope}
	if err := decryptRuntimeEvent(errEncryptor{}, evt); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestDecryptToolCall_ErrorOnArgumentsDecryptFail verifies error propagation.
func TestDecryptToolCall_ErrorOnArgumentsDecryptFail(t *testing.T) {
	envelope := map[string]any{
		envelopeMarkerKey:  true,
		envelopePayloadKey: base64.StdEncoding.EncodeToString([]byte("fake")),
	}
	tc := &session.ToolCall{Arguments: envelope}
	if err := decryptToolCall(errEncryptor{}, tc); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestDecryptToolCall_ErrorOnResultDecryptFail verifies error propagation on
// the Result field.
func TestDecryptToolCall_ErrorOnResultDecryptFail(t *testing.T) {
	envelope := map[string]any{
		envelopeMarkerKey:  true,
		envelopePayloadKey: base64.StdEncoding.EncodeToString([]byte("fake")),
	}
	tc := &session.ToolCall{Result: envelope}
	if err := decryptToolCall(errEncryptor{}, tc); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestEncryptString_EmptyPassthrough verifies that empty strings pass through
// unchanged.
func TestEncryptString_EmptyPassthrough(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	got, err := encryptString(enc, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

// TestDecryptString_NilEncPassthrough verifies that when enc is nil, any string
// is returned unchanged regardless of content.
func TestDecryptString_NilEncPassthrough(t *testing.T) {
	got, err := decryptString(nil, errMsgEncPrefix+"something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != errMsgEncPrefix+"something" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

// TestIsEnvelopeMap_NilReturnsFalse verifies the nil case.
func TestIsEnvelopeMap_NilReturnsFalse(t *testing.T) {
	if isEnvelopeMap(nil) {
		t.Fatal("expected false for nil map")
	}
}

// TestEncryptMessage_NilNoOp verifies encryptMessage with nil msg returns nil error.
func TestEncryptMessage_NilNoOp(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	if err := encryptMessage(enc, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDecryptMessage_NilNoOp verifies decryptMessage with nil msg returns nil error.
func TestDecryptMessage_NilNoOp(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	if err := decryptMessage(enc, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDecryptEnvelopeAny_InvalidBase64 verifies that a malformed payload
// returns an error.
func TestDecryptEnvelopeAny_InvalidBase64(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	envelope := map[string]any{
		envelopeMarkerKey:  true,
		envelopePayloadKey: "not valid base64!!!",
	}
	_, err := decryptEnvelopeAny(enc, envelope)
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// TestDecryptEnvelopeMap_InvalidBase64 verifies that a malformed payload
// returns an error for the map variant.
func TestDecryptEnvelopeMap_InvalidBase64(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	envelope := map[string]any{
		envelopeMarkerKey:  true,
		envelopePayloadKey: "not valid base64!!!",
	}
	_, err := decryptEnvelopeMap(enc, envelope)
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// TestEncryptRuntimeEvent_ErrorOnDataEncryptFail verifies error propagation
// when Data encryption fails.
func TestEncryptRuntimeEvent_ErrorOnDataEncryptFail(t *testing.T) {
	evt := &session.RuntimeEvent{
		ID:   "e1",
		Data: map[string]any{"k": "v"},
	}
	if err := encryptRuntimeEvent(errEncryptor{}, evt); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestDecryptString_BadBase64 verifies that malformed base64 in a prefixed
// value returns an error.
func TestDecryptString_BadBase64(t *testing.T) {
	enc := mockEncryptor{key: 0x01}
	_, err := decryptString(enc, errMsgEncPrefix+"not!!!valid$base64")
	if err == nil {
		t.Fatal("expected error for bad base64, got nil")
	}
}
