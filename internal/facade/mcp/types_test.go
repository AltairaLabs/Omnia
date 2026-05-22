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
	"encoding/json"
	"testing"
)

func TestJSONRPCRequest_RoundTrip(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
	var req JSONRPCRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("JSONRPC: got %q want 2.0", req.JSONRPC)
	}
	if req.Method != "tools/list" {
		t.Errorf("Method: got %q want tools/list", req.Method)
	}
	if string(req.ID) != "1" {
		t.Errorf("ID: got %q want 1", string(req.ID))
	}
	out, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(out) {
		t.Errorf("marshal output not valid JSON: %s", string(out))
	}
}

func TestJSONRPCResponse_ErrorShape(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      json.RawMessage("2"),
		Error:   &JSONRPCError{Code: ErrorMethodNotFound, Message: "method not found: nope"},
	}
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found: nope"}}`
	if string(out) != want {
		t.Errorf("got %s\nwant %s", string(out), want)
	}
}

func TestInitializeResult_AdvertisesTools(t *testing.T) {
	r := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo:      ServerInfo{Name: "fn", Version: "1.0.0"},
		Capabilities:    ServerCapabilities{Tools: &ToolsCapability{}},
	}
	out, _ := json.Marshal(r)
	want := `{"protocolVersion":"2025-03-26","serverInfo":{"name":"fn","version":"1.0.0"},"capabilities":{"tools":{}}}`
	if string(out) != want {
		t.Errorf("got %s\nwant %s", string(out), want)
	}
}

func TestCallToolResult_SuccessShape(t *testing.T) {
	r := CallToolResult{
		Content: []ContentPart{{Type: "text", Text: `{"echo":"hi"}`}},
	}
	out, _ := json.Marshal(r)
	want := `{"content":[{"type":"text","text":"{\"echo\":\"hi\"}"}]}`
	if string(out) != want {
		t.Errorf("got %s\nwant %s", string(out), want)
	}
}

func TestCallToolResult_ErrorShape(t *testing.T) {
	r := CallToolResult{
		Content: []ContentPart{{Type: "text", Text: "input_invalid: missing field"}},
		IsError: true,
	}
	out, _ := json.Marshal(r)
	want := `{"content":[{"type":"text","text":"input_invalid: missing field"}],"isError":true}`
	if string(out) != want {
		t.Errorf("got %s\nwant %s", string(out), want)
	}
}

func TestTool_RequiredFieldsSurfacedInJSON(t *testing.T) {
	tool := Tool{
		Name:        "weather",
		Description: "Look up weather",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
	out, _ := json.Marshal(tool)
	want := `{"name":"weather","description":"Look up weather","inputSchema":{"type":"object"}}`
	if string(out) != want {
		t.Errorf("got %s\nwant %s", string(out), want)
	}
}
