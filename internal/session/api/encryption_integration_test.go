//go:build integration

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// --- fakeEncryptor ---

// fakeEncryptor is a deterministic, per-tag Encryptor: ciphertext = tag ":" + plaintext.
// It is intentionally distinct from mockEncryptor (XOR-based) in encryption_test.go.
// The tag prefix makes the source session visually identifiable in stored bytes.
type fakeEncryptor struct{ tag string }

func (f fakeEncryptor) Encrypt(p []byte) ([]byte, error) {
	return append([]byte(f.tag+":"), p...), nil
}

func (f fakeEncryptor) Decrypt(c []byte) ([]byte, error) {
	prefix := f.tag + ":"
	if len(c) < len(prefix) || string(c[:len(prefix)]) != prefix {
		return nil, fmt.Errorf("fakeEncryptor: bad prefix for tag %q, ciphertext=%q", f.tag, string(c))
	}
	return c[len(prefix):], nil
}

// --- roundTripWarmStore ---

// roundTripWarmStore extends mockWarmStore so that:
//   - AppendMessage stores into messages (not a separate appendedMsgs map),
//     enabling GET /messages to return what was written by POST /messages.
//   - RecordToolCall and RecordRuntimeEvent store items in toolCalls /
//     runtimeEvents maps so GET endpoints return them.
//
// It also exposes raw accessors for direct inspection of stored ciphertext.
type roundTripWarmStore struct {
	*mockWarmStore
}

func newRoundTripWarmStore() *roundTripWarmStore {
	return &roundTripWarmStore{mockWarmStore: newMockWarmStore()}
}

// AppendMessage stores the (potentially encrypted) message in the messages map
// so GetMessages returns it — the vanilla mockWarmStore stores in appendedMsgs
// which GetMessages never reads.
func (r *roundTripWarmStore) AppendMessage(_ context.Context, sessionID string, msg *session.Message) error {
	if _, ok := r.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	r.messages[sessionID] = append(r.messages[sessionID], msg)
	return nil
}

// RecordToolCall stores the (potentially encrypted) tool call so GetToolCalls returns it.
func (r *roundTripWarmStore) RecordToolCall(_ context.Context, sessionID string, tc *session.ToolCall) error {
	if _, ok := r.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	clone := *tc
	r.toolCalls[sessionID] = append(r.toolCalls[sessionID], &clone)
	return nil
}

// RecordRuntimeEvent stores the (potentially encrypted) event so GetRuntimeEvents returns it.
func (r *roundTripWarmStore) RecordRuntimeEvent(_ context.Context, sessionID string, evt *session.RuntimeEvent) error {
	if _, ok := r.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	clone := *evt
	r.runtimeEvents[sessionID] = append(r.runtimeEvents[sessionID], &clone)
	return nil
}

// rawMessages returns stored messages without decryption.
func (r *roundTripWarmStore) rawMessages(sessionID string) []*session.Message {
	return r.messages[sessionID]
}

// rawToolCalls returns stored tool calls without decryption.
func (r *roundTripWarmStore) rawToolCalls(sessionID string) []*session.ToolCall {
	return r.toolCalls[sessionID]
}

// rawRuntimeEvents returns stored runtime events without decryption.
func (r *roundTripWarmStore) rawRuntimeEvents(sessionID string) []*session.RuntimeEvent {
	return r.runtimeEvents[sessionID]
}

// --- integration test session IDs ---

const (
	intSessionA = "aaaa0000-0000-0000-0000-000000000001"
	intSessionB = "bbbb0000-0000-0000-0000-000000000002"
	intSessionC = "cccc0000-0000-0000-0000-000000000003"
)

// --- helpers ---

func setupIntegrationHandler(t *testing.T) (*Handler, *roundTripWarmStore) {
	t.Helper()
	warm := newRoundTripWarmStore()
	reg := providers.NewRegistry()
	reg.SetWarmStore(warm)
	svc := NewSessionService(reg, ServiceConfig{}, logr.Discard())
	h := NewHandler(svc, logr.Discard())
	return h, warm
}

func createIntSession(t *testing.T, mux *http.ServeMux, id, agentName string) {
	t.Helper()
	body := fmt.Sprintf(
		`{"id":%q,"agentName":%q,"namespace":"default","workspaceName":"test-ws"}`,
		id, agentName,
	)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "create session %s: %s", id, rec.Body.String())
}

func postMessage(t *testing.T, mux *http.ServeMux, sessionID, content string) {
	t.Helper()
	body := fmt.Sprintf(`{"id":"m1","role":"user","content":%q,"sequenceNum":1}`, content)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/messages", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "POST message to %s: %s", sessionID, rec.Body.String())
}

func getMessages(t *testing.T, mux *http.ServeMux, sessionID string) []*session.Message {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/messages", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "GET messages for %s: %s", sessionID, rec.Body.String())
	var resp MessagesResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	return resp.Messages
}

func postToolCall(t *testing.T, mux *http.ServeMux, sessionID, toolName string, args map[string]any) {
	t.Helper()
	argsJSON, err := json.Marshal(args)
	require.NoError(t, err)
	body := fmt.Sprintf(`{"id":"tc1","name":%q,"arguments":%s,"status":"pending"}`, toolName, argsJSON)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/tool-calls", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "POST tool-call to %s: %s", sessionID, rec.Body.String())
}

func getToolCalls(t *testing.T, mux *http.ServeMux, sessionID string) []*session.ToolCall {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/tool-calls", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "GET tool-calls for %s: %s", sessionID, rec.Body.String())
	var items []*session.ToolCall
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&items))
	return items
}

func postRuntimeEvent(t *testing.T, mux *http.ServeMux, sessionID, eventType string, data map[string]any) {
	t.Helper()
	dataJSON, err := json.Marshal(data)
	require.NoError(t, err)
	body := fmt.Sprintf(`{"id":"e1","eventType":%q,"data":%s}`, eventType, dataJSON)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+sessionID+"/events", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "POST event to %s: %s", sessionID, rec.Body.String())
}

func getRuntimeEvents(t *testing.T, mux *http.ServeMux, sessionID string) []*session.RuntimeEvent {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, "GET events for %s: %s", sessionID, rec.Body.String())
	var items []*session.RuntimeEvent
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&items))
	return items
}

// isEnvelope returns true if the given map looks like an encryption envelope.
func isEnvelope(m map[string]any) bool {
	if m == nil {
		return false
	}
	_, hasMark := m[envelopeMarkerKey]
	_, hasPayload := m[envelopePayloadKey]
	return hasMark && hasPayload
}

// --- Integration test ---

// TestHandler_PerSessionEncryption_RoundTrip exercises Handler + EncryptorResolver
// with three sessions each routing to a different encryptor. The test proves:
//
//  1. Distinct sessions produce distinct stored ciphertexts for the same plaintext.
//  2. Round-trip decryption (POST → raw store → GET) returns the original value.
//  3. Structural fields (Name, EventType) stay plaintext; payload fields (Content,
//     Arguments, Data) are stored as ciphertext.
//  4. Sessions with no encryptor entry round-trip as plaintext.
func TestHandler_PerSessionEncryption_RoundTrip(t *testing.T) {
	h, warm := setupIntegrationHandler(t)

	// Three sessions, three encryptors:
	//   A → tag "AAA"  B → tag "BBB"  C → no encryptor (plaintext passthrough)
	encA := fakeEncryptor{tag: "AAA"}
	encB := fakeEncryptor{tag: "BBB"}

	h.SetEncryptorResolver(EncryptorResolverFunc(func(id string) (Encryptor, bool) {
		switch id {
		case intSessionA:
			return encA, true
		case intSessionB:
			return encB, true
		default:
			return nil, false
		}
	}))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Seed the warm store sessions (CreateSession requires a valid session row).
	warm.sessions[intSessionA] = &session.Session{ID: intSessionA, AgentName: "a", Status: session.SessionStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	warm.sessions[intSessionB] = &session.Session{ID: intSessionB, AgentName: "b", Status: session.SessionStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	warm.sessions[intSessionC] = &session.Session{ID: intSessionC, AgentName: "c", Status: session.SessionStatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}

	// -------------------------------------------------------------------------
	// 1. Messages round-trip
	// -------------------------------------------------------------------------

	const sharedPlaintext = "marker-payload"

	postMessage(t, mux, intSessionA, sharedPlaintext)
	postMessage(t, mux, intSessionB, sharedPlaintext)
	postMessage(t, mux, intSessionC, sharedPlaintext)

	// Inspect raw stored bytes directly.
	rawMsgsA := warm.rawMessages(intSessionA)
	rawMsgsB := warm.rawMessages(intSessionB)
	rawMsgsC := warm.rawMessages(intSessionC)

	require.Len(t, rawMsgsA, 1, "session A: expected 1 stored message")
	require.Len(t, rawMsgsB, 1, "session B: expected 1 stored message")
	require.Len(t, rawMsgsC, 1, "session C: expected 1 stored message")

	rawContentA := rawMsgsA[0].Content
	rawContentB := rawMsgsB[0].Content
	rawContentC := rawMsgsC[0].Content

	// Encrypted sessions must not store plaintext.
	assert.NotEqual(t, sharedPlaintext, rawContentA, "session A: content stored as plaintext")
	assert.NotEqual(t, sharedPlaintext, rawContentB, "session B: content stored as plaintext")

	// Session C (no encryptor) must store plaintext unchanged.
	assert.Equal(t, sharedPlaintext, rawContentC, "session C: content should be plaintext")

	// The enc:v1: wrapper is applied; each session's ciphertext must carry it.
	assert.True(t, strings.HasPrefix(rawContentA, errMsgEncPrefix), "session A: missing enc:v1: prefix")
	assert.True(t, strings.HasPrefix(rawContentB, errMsgEncPrefix), "session B: missing enc:v1: prefix")

	// KEY ISOLATION ASSERTION: same plaintext, different encryptors → different ciphertext.
	assert.NotEqual(t, rawContentA, rawContentB,
		"sessions A and B produced the same ciphertext — encryptor isolation broken")

	// Round-trip: GET /messages returns decrypted plaintext.
	msgsA := getMessages(t, mux, intSessionA)
	msgsB := getMessages(t, mux, intSessionB)
	msgsC := getMessages(t, mux, intSessionC)

	require.Len(t, msgsA, 1)
	require.Len(t, msgsB, 1)
	require.Len(t, msgsC, 1)

	assert.Equal(t, sharedPlaintext, msgsA[0].Content, "session A: round-trip failed")
	assert.Equal(t, sharedPlaintext, msgsB[0].Content, "session B: round-trip failed")
	assert.Equal(t, sharedPlaintext, msgsC[0].Content, "session C: round-trip failed")

	// -------------------------------------------------------------------------
	// 2. Tool calls round-trip
	// -------------------------------------------------------------------------

	toolArgs := map[string]any{"q": "secret query"}

	postToolCall(t, mux, intSessionA, "browser_search", toolArgs)
	postToolCall(t, mux, intSessionB, "browser_search", toolArgs)
	postToolCall(t, mux, intSessionC, "browser_search", toolArgs)

	// Raw store inspection for tool calls.
	rawTCsA := warm.rawToolCalls(intSessionA)
	rawTCsB := warm.rawToolCalls(intSessionB)
	rawTCsC := warm.rawToolCalls(intSessionC)

	require.Len(t, rawTCsA, 1, "session A: expected 1 stored tool call")
	require.Len(t, rawTCsB, 1, "session B: expected 1 stored tool call")
	require.Len(t, rawTCsC, 1, "session C: expected 1 stored tool call")

	// Name must always be plaintext.
	assert.Equal(t, "browser_search", rawTCsA[0].Name, "session A: tool Name must be plaintext")
	assert.Equal(t, "browser_search", rawTCsB[0].Name, "session B: tool Name must be plaintext")
	assert.Equal(t, "browser_search", rawTCsC[0].Name, "session C: tool Name must be plaintext")

	// Arguments must be envelope-wrapped for encrypted sessions.
	assert.True(t, isEnvelope(rawTCsA[0].Arguments), "session A: Arguments not encrypted (no envelope)")
	assert.True(t, isEnvelope(rawTCsB[0].Arguments), "session B: Arguments not encrypted (no envelope)")

	// Session C: Arguments must NOT be wrapped (plaintext passthrough).
	assert.False(t, isEnvelope(rawTCsC[0].Arguments), "session C: Arguments should not be wrapped")
	assert.Equal(t, "secret query", rawTCsC[0].Arguments["q"], "session C: plain Arguments mismatch")

	// Round-trip: GET /tool-calls returns decrypted data.
	tcsA := getToolCalls(t, mux, intSessionA)
	tcsB := getToolCalls(t, mux, intSessionB)
	tcsC := getToolCalls(t, mux, intSessionC)

	require.Len(t, tcsA, 1)
	require.Len(t, tcsB, 1)
	require.Len(t, tcsC, 1)

	assert.Equal(t, "browser_search", tcsA[0].Name, "session A: tool Name round-trip")
	assert.Equal(t, "browser_search", tcsB[0].Name, "session B: tool Name round-trip")

	assert.Equal(t, "secret query", fmt.Sprintf("%v", tcsA[0].Arguments["q"]), "session A: args q round-trip")
	assert.Equal(t, "secret query", fmt.Sprintf("%v", tcsB[0].Arguments["q"]), "session B: args q round-trip")
	assert.Equal(t, "secret query", fmt.Sprintf("%v", tcsC[0].Arguments["q"]), "session C: args q round-trip")

	// -------------------------------------------------------------------------
	// 3. Runtime events round-trip
	// -------------------------------------------------------------------------

	eventData := map[string]any{"msg": "sensitive"}

	postRuntimeEvent(t, mux, intSessionA, "provider_error", eventData)
	postRuntimeEvent(t, mux, intSessionB, "provider_error", eventData)
	postRuntimeEvent(t, mux, intSessionC, "provider_error", eventData)

	// Raw store inspection.
	rawEvtsA := warm.rawRuntimeEvents(intSessionA)
	rawEvtsB := warm.rawRuntimeEvents(intSessionB)
	rawEvtsC := warm.rawRuntimeEvents(intSessionC)

	require.Len(t, rawEvtsA, 1, "session A: expected 1 stored event")
	require.Len(t, rawEvtsB, 1, "session B: expected 1 stored event")
	require.Len(t, rawEvtsC, 1, "session C: expected 1 stored event")

	// EventType always plaintext.
	assert.Equal(t, "provider_error", rawEvtsA[0].EventType, "session A: EventType must be plaintext")
	assert.Equal(t, "provider_error", rawEvtsB[0].EventType, "session B: EventType must be plaintext")
	assert.Equal(t, "provider_error", rawEvtsC[0].EventType, "session C: EventType must be plaintext")

	// Data envelope-wrapped for encrypted sessions.
	assert.True(t, isEnvelope(rawEvtsA[0].Data), "session A: Data not encrypted (no envelope)")
	assert.True(t, isEnvelope(rawEvtsB[0].Data), "session B: Data not encrypted (no envelope)")
	assert.False(t, isEnvelope(rawEvtsC[0].Data), "session C: Data should not be wrapped")
	assert.Equal(t, "sensitive", rawEvtsC[0].Data["msg"], "session C: plain Data mismatch")

	// Round-trip GET.
	evtsA := getRuntimeEvents(t, mux, intSessionA)
	evtsB := getRuntimeEvents(t, mux, intSessionB)
	evtsC := getRuntimeEvents(t, mux, intSessionC)

	require.Len(t, evtsA, 1)
	require.Len(t, evtsB, 1)
	require.Len(t, evtsC, 1)

	assert.Equal(t, "provider_error", evtsA[0].EventType)
	assert.Equal(t, "provider_error", evtsB[0].EventType)

	assert.Equal(t, "sensitive", fmt.Sprintf("%v", evtsA[0].Data["msg"]), "session A: data msg round-trip")
	assert.Equal(t, "sensitive", fmt.Sprintf("%v", evtsB[0].Data["msg"]), "session B: data msg round-trip")
	assert.Equal(t, "sensitive", fmt.Sprintf("%v", evtsC[0].Data["msg"]), "session C: data msg round-trip")
}
