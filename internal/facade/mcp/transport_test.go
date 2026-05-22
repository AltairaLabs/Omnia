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

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr/testr"
)

// Test-scoped string constants extracted to satisfy goconst (`text`,
// `1.0.0`, etc. appear in many fixtures below).
const (
	testServerVersion = "1.0.0"
	testServerName    = "fn"
)

// stubAdapter is a deterministic ToolAdapter for transport tests.
type stubAdapter struct {
	tools  []Tool
	result CallToolResult

	lastCallName string
	lastCallArgs json.RawMessage
}

func (s *stubAdapter) ListTools() []Tool { return s.tools }
func (s *stubAdapter) CallTool(_ context.Context, name string, args json.RawMessage) CallToolResult {
	s.lastCallName = name
	s.lastCallArgs = args
	return s.result
}

func newHandler(t *testing.T, adapter ToolAdapter) http.Handler {
	t.Helper()
	return NewTransport(TransportConfig{
		Adapter:    adapter,
		ServerInfo: ServerInfo{Name: testServerName, Version: testServerVersion},
		Log:        testr.New(t),
	})
}

func postJSON(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	return rr
}

func decodeResp(t *testing.T, rr *httptest.ResponseRecorder) JSONRPCResponse {
	t.Helper()
	var resp JSONRPCResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rr.Body.String())
	}
	return resp
}

func TestTransport_Initialize(t *testing.T) {
	h := newHandler(t, &stubAdapter{})
	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)

	if rr.Code != http.StatusOK {
		t.Fatalf("Code: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	resp := decodeResp(t, rr)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode InitializeResult: %v", err)
	}
	if result.ProtocolVersion != ProtocolVersion {
		t.Errorf("ProtocolVersion: got %q want %q", result.ProtocolVersion, ProtocolVersion)
	}
	if result.ServerInfo.Name != "fn" {
		t.Errorf("ServerInfo.Name: got %q want fn", result.ServerInfo.Name)
	}
	if result.Capabilities.Tools == nil {
		t.Error("Capabilities.Tools must be advertised")
	}
}

func TestTransport_ListTools(t *testing.T) {
	tool := Tool{Name: "echo", Description: "desc", InputSchema: json.RawMessage(`{"type":"object"}`)}
	h := newHandler(t, &stubAdapter{tools: []Tool{tool}})

	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("Code: got %d want 200", rr.Code)
	}
	resp := decodeResp(t, rr)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var result ListToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode ListToolsResult: %v", err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "echo" {
		t.Errorf("Tools: %+v", result.Tools)
	}
}

func TestTransport_CallTool_Success(t *testing.T) {
	adapter := &stubAdapter{
		result: CallToolResult{Content: []ContentPart{{Type: ContentTypeText, Text: `{"echo":"hi"}`}}},
	}
	h := newHandler(t, adapter)

	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"message":"hi"}}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("Code: got %d", rr.Code)
	}
	if adapter.lastCallName != "echo" {
		t.Errorf("adapter call name: got %q", adapter.lastCallName)
	}
	if !strings.Contains(string(adapter.lastCallArgs), `"message":"hi"`) {
		t.Errorf("adapter call args: got %s", string(adapter.lastCallArgs))
	}
	resp := decodeResp(t, rr)
	var result CallToolResult
	_ = json.Unmarshal(resp.Result, &result)
	if result.IsError {
		t.Errorf("unexpected IsError=true")
	}
	if len(result.Content) != 1 || result.Content[0].Text != `{"echo":"hi"}` {
		t.Errorf("Content: %+v", result.Content)
	}
}

func TestTransport_CallTool_AdapterReturnsError(t *testing.T) {
	adapter := &stubAdapter{
		result: CallToolResult{IsError: true, Content: []ContentPart{{Type: ContentTypeText, Text: "input_invalid: missing"}}},
	}
	h := newHandler(t, adapter)

	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo"}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("Code: got %d (JSON-RPC tool-level errors stay 200)", rr.Code)
	}
	resp := decodeResp(t, rr)
	if resp.Error != nil {
		t.Fatalf("JSON-RPC error layer should be nil for tool-level errors; got %+v", resp.Error)
	}
	var result CallToolResult
	_ = json.Unmarshal(resp.Result, &result)
	if !result.IsError {
		t.Error("expected IsError=true")
	}
}

func TestTransport_CallTool_MissingParamsName(t *testing.T) {
	h := newHandler(t, &stubAdapter{})
	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{}}`)
	resp := decodeResp(t, rr)
	if resp.Error == nil || resp.Error.Code != ErrorInvalidParams {
		t.Errorf("Error: %+v want InvalidParams", resp.Error)
	}
}

func TestTransport_CallTool_BadParamsShape(t *testing.T) {
	h := newHandler(t, &stubAdapter{})
	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":"not-an-object"}`)
	resp := decodeResp(t, rr)
	if resp.Error == nil || resp.Error.Code != ErrorInvalidParams {
		t.Errorf("Error: %+v want InvalidParams", resp.Error)
	}
}

func TestTransport_UnknownMethod(t *testing.T) {
	h := newHandler(t, &stubAdapter{})
	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":7,"method":"nope","params":{}}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("Code: got %d (JSON-RPC errors stay 200)", rr.Code)
	}
	resp := decodeResp(t, rr)
	if resp.Error == nil || resp.Error.Code != ErrorMethodNotFound {
		t.Errorf("Error: %+v want MethodNotFound", resp.Error)
	}
}

func TestTransport_MalformedJSON(t *testing.T) {
	h := newHandler(t, &stubAdapter{})
	rr := postJSON(t, h, `{not json`)
	resp := decodeResp(t, rr)
	if resp.Error == nil || resp.Error.Code != ErrorParseError {
		t.Errorf("Error: %+v want ParseError", resp.Error)
	}
}

func TestTransport_InvalidJSONRPCVersion(t *testing.T) {
	h := newHandler(t, &stubAdapter{})
	rr := postJSON(t, h, `{"jsonrpc":"1.0","id":8,"method":"initialize","params":{}}`)
	resp := decodeResp(t, rr)
	if resp.Error == nil || resp.Error.Code != ErrorInvalidRequest {
		t.Errorf("Error: %+v want InvalidRequest", resp.Error)
	}
}

func TestTransport_MethodNotPost(t *testing.T) {
	h := newHandler(t, &stubAdapter{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/mcp", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Code: got %d want 405", rr.Code)
	}
	if rr.Header().Get("Allow") != http.MethodPost {
		t.Errorf("Allow: got %q want POST", rr.Header().Get("Allow"))
	}
}

func TestTransport_PayloadTooLarge(t *testing.T) {
	h := NewTransport(TransportConfig{
		Adapter:      &stubAdapter{},
		ServerInfo:   ServerInfo{Name: "fn", Version: "1.0.0"},
		Log:          testr.New(t),
		MaxBodyBytes: 8,
	})
	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":9,"method":"initialize","params":{}}`)
	resp := decodeResp(t, rr)
	if resp.Error == nil || resp.Error.Code != ErrorParseError {
		t.Errorf("Error: %+v want ParseError for oversized body", resp.Error)
	}
}

func TestTransport_NoLogger_FallsBackToDiscard(t *testing.T) {
	// Cover the cfg.Log fallback when caller passes a zero logr.Logger.
	// Just exercise the path; no assertions needed beyond no-panic.
	h := NewTransport(TransportConfig{
		Adapter:    &stubAdapter{},
		ServerInfo: ServerInfo{Name: testServerName, Version: testServerVersion},
	})
	rr := postJSON(t, h, `{"jsonrpc":"2.0","id":10,"method":"initialize","params":{}}`)
	if rr.Code != http.StatusOK {
		t.Errorf("Code: got %d want 200", rr.Code)
	}
}
