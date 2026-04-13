/*
Copyright 2026.

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

package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altairalabs/omnia/internal/session"
)

// encPrefix marks content that the mock encryptor "encrypted". This is not
// real encryption — it's a deterministic transformation used to verify that
// the handler invoked the encryptor at the right points in the flow.
const encPrefix = "ENC:"

// mockHandlerEncryptor records calls and prefixes/strips encPrefix on Content
// and ErrorMessage fields to simulate encryption.
type mockHandlerEncryptor struct {
	encryptMessageCalls  int
	decryptMessageCalls  int
	encryptToolCallCalls int
	decryptToolCallCalls int
	encryptEventCalls    int
	decryptEventCalls    int
}

func (m *mockHandlerEncryptor) EncryptMessage(_ context.Context, msg *session.Message) (*session.Message, error) {
	m.encryptMessageCalls++
	out := *msg
	out.Content = encPrefix + msg.Content
	return &out, nil
}

func (m *mockHandlerEncryptor) DecryptMessage(_ context.Context, msg *session.Message) (*session.Message, error) {
	m.decryptMessageCalls++
	out := *msg
	out.Content = strings.TrimPrefix(msg.Content, encPrefix)
	return &out, nil
}

func (m *mockHandlerEncryptor) EncryptToolCall(_ context.Context, tc *session.ToolCall) (*session.ToolCall, error) {
	m.encryptToolCallCalls++
	out := *tc
	out.ErrorMessage = encPrefix + tc.ErrorMessage
	return &out, nil
}

func (m *mockHandlerEncryptor) DecryptToolCall(_ context.Context, tc *session.ToolCall) (*session.ToolCall, error) {
	m.decryptToolCallCalls++
	out := *tc
	out.ErrorMessage = strings.TrimPrefix(tc.ErrorMessage, encPrefix)
	return &out, nil
}

func (m *mockHandlerEncryptor) EncryptRuntimeEvent(_ context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error) {
	m.encryptEventCalls++
	out := *evt
	out.ErrorMessage = encPrefix + evt.ErrorMessage
	return &out, nil
}

func (m *mockHandlerEncryptor) DecryptRuntimeEvent(_ context.Context, evt *session.RuntimeEvent) (*session.RuntimeEvent, error) {
	m.decryptEventCalls++
	out := *evt
	out.ErrorMessage = strings.TrimPrefix(evt.ErrorMessage, encPrefix)
	return &out, nil
}

// TestHandleAppendMessage_EncryptsWhenEncryptorSet verifies that when an
// Encryptor is configured, the content that reaches the store is encrypted.
func TestHandleAppendMessage_EncryptsWhenEncryptorSet(t *testing.T) {
	h, _, warm := setupHandler(t)
	warm.sessions[testSessionID] = testSession(testSessionID)
	enc := &mockHandlerEncryptor{}
	h.SetEncryptor(enc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"id":"m1","role":"user","content":"hello","sequenceNum":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+testSessionID+"/messages", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if enc.encryptMessageCalls != 1 {
		t.Fatalf("expected 1 encrypt call, got %d", enc.encryptMessageCalls)
	}
	stored := warm.appendedMsgs[testSessionID]
	if len(stored) != 1 {
		t.Fatalf("expected 1 stored message, got %d", len(stored))
	}
	if stored[0].Content != encPrefix+"hello" {
		t.Fatalf("expected store to receive encrypted content, got %q", stored[0].Content)
	}
}

// TestHandleAppendMessage_NoEncryptorWhenNil verifies plaintext passthrough.
func TestHandleAppendMessage_NoEncryptorWhenNil(t *testing.T) {
	h, _, warm := setupHandler(t)
	warm.sessions[testSessionID] = testSession(testSessionID)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"id":"m1","role":"user","content":"hello","sequenceNum":1}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+testSessionID+"/messages", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	stored := warm.appendedMsgs[testSessionID]
	if len(stored) != 1 || stored[0].Content != "hello" {
		t.Fatalf("expected plaintext content in store, got %+v", stored)
	}
}

// TestHandleGetMessages_DecryptsWhenEncryptorSet verifies that messages
// returned to the client are decrypted.
func TestHandleGetMessages_DecryptsWhenEncryptorSet(t *testing.T) {
	h, _, warm := setupHandler(t)
	warm.sessions[testSessionID] = testSession(testSessionID)
	warm.messages[testSessionID] = []*session.Message{
		{ID: "m1", Content: encPrefix + "hello", SequenceNum: 1},
		{ID: "m2", Content: encPrefix + "world", SequenceNum: 2},
	}
	enc := &mockHandlerEncryptor{}
	h.SetEncryptor(enc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+testSessionID+"/messages", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[MessagesResponse](t, rec)
	if len(resp.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(resp.Messages))
	}
	if resp.Messages[0].Content != "hello" || resp.Messages[1].Content != "world" {
		t.Fatalf("expected decrypted content, got %+v", resp.Messages)
	}
	if enc.decryptMessageCalls != 2 {
		t.Fatalf("expected 2 decrypt calls, got %d", enc.decryptMessageCalls)
	}
}

// TestHandleRecordToolCall_EncryptsWhenEncryptorSet verifies the tool-call
// encrypt path.
func TestHandleRecordToolCall_EncryptsWhenEncryptorSet(t *testing.T) {
	h, _, warm := setupHandler(t)
	warm.sessions[testSessionID] = testSession(testSessionID)
	enc := &mockHandlerEncryptor{}
	h.SetEncryptor(enc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"id":"tc1","name":"myTool","status":"success","errorMessage":"oops"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+testSessionID+"/tool-calls", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if enc.encryptToolCallCalls != 1 {
		t.Fatalf("expected 1 tool-call encrypt, got %d", enc.encryptToolCallCalls)
	}
}

// TestHandleGetToolCalls_DecryptsWhenEncryptorSet verifies tool calls are
// decrypted on the way out.
func TestHandleGetToolCalls_DecryptsWhenEncryptorSet(t *testing.T) {
	h, _, warm := setupHandler(t)
	warm.sessions[testSessionID] = testSession(testSessionID)
	warm.toolCalls[testSessionID] = []*session.ToolCall{
		{ID: "tc1", Name: "t", ErrorMessage: encPrefix + "oops"},
	}
	enc := &mockHandlerEncryptor{}
	h.SetEncryptor(enc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+testSessionID+"/tool-calls", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if enc.decryptToolCallCalls != 1 {
		t.Fatalf("expected 1 tool-call decrypt, got %d", enc.decryptToolCallCalls)
	}
	resp := decodeJSON[[]*session.ToolCall](t, rec)
	if len(resp) != 1 || resp[0].ErrorMessage != "oops" {
		t.Fatalf("expected decrypted ErrorMessage, got %+v", resp)
	}
}

// TestHandleRecordRuntimeEvent_EncryptsWhenEncryptorSet verifies the
// runtime-event encrypt path.
func TestHandleRecordRuntimeEvent_EncryptsWhenEncryptorSet(t *testing.T) {
	h, _, warm := setupHandler(t)
	warm.sessions[testSessionID] = testSession(testSessionID)
	enc := &mockHandlerEncryptor{}
	h.SetEncryptor(enc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"id":"e1","eventType":"pipeline.started","errorMessage":"boom"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions/"+testSessionID+"/events", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if enc.encryptEventCalls != 1 {
		t.Fatalf("expected 1 event encrypt, got %d", enc.encryptEventCalls)
	}
}

// TestHandleGetRuntimeEvents_DecryptsWhenEncryptorSet verifies events are
// decrypted on the way out.
func TestHandleGetRuntimeEvents_DecryptsWhenEncryptorSet(t *testing.T) {
	h, _, warm := setupHandler(t)
	warm.sessions[testSessionID] = testSession(testSessionID)
	warm.runtimeEvents[testSessionID] = []*session.RuntimeEvent{
		{ID: "e1", EventType: "pipeline.started", ErrorMessage: encPrefix + "boom"},
	}
	enc := &mockHandlerEncryptor{}
	h.SetEncryptor(enc)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+testSessionID+"/events", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if enc.decryptEventCalls != 1 {
		t.Fatalf("expected 1 event decrypt, got %d", enc.decryptEventCalls)
	}
	resp := decodeJSON[[]*session.RuntimeEvent](t, rec)
	if len(resp) != 1 || resp[0].ErrorMessage != "boom" {
		t.Fatalf("expected decrypted ErrorMessage, got %+v", resp)
	}
}
