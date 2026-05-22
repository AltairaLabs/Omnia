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
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
)

// Method names recognised on POST /mcp.
const (
	MethodInitialize = "initialize"
	MethodToolsList  = "tools/list"
	MethodToolsCall  = "tools/call"
)

// defaultMaxBodyBytes caps the inbound POST body. Matches the HTTP
// function-route default; one tools/call payload is well under this.
const defaultMaxBodyBytes = 1 << 20

// ToolAdapter is the back-end the transport dispatches to. Implementations
// translate MCP method shapes into concrete behavior (function invocation,
// in Omnia's case).
type ToolAdapter interface {
	ListTools() []Tool
	CallTool(ctx context.Context, name string, arguments json.RawMessage) CallToolResult
}

// TransportConfig assembles a transport handler's dependencies.
type TransportConfig struct {
	Adapter      ToolAdapter
	ServerInfo   ServerInfo
	Log          logr.Logger
	MaxBodyBytes int64
}

// NewTransport returns an http.Handler that serves the Streamable HTTP
// MCP transport (single POST /mcp endpoint). The handler is stateless —
// each request is an independent JSON-RPC round-trip.
//
// The handler does not enforce auth; wrap with auth.Middleware at the
// caller. Auth must run outermost so rejected requests don't spend the
// transport's metrics or body-read budget.
func NewTransport(cfg TransportConfig) http.Handler {
	if cfg.MaxBodyBytes == 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	if cfg.Log.GetSink() == nil {
		cfg.Log = logr.Discard()
	}
	return &transport{cfg: cfg}
}

type transport struct {
	cfg TransportConfig
}

func (t *transport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, t.cfg.MaxBodyBytes))
	if err != nil {
		writeJSONRPCError(w, nil, ErrorParseError, "read body: "+err.Error())
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONRPCError(w, nil, ErrorParseError, err.Error())
		return
	}
	if req.JSONRPC != JSONRPCVersion {
		writeJSONRPCError(w, req.ID, ErrorInvalidRequest, `jsonrpc must be "2.0"`)
		return
	}

	started := time.Now()
	t.dispatch(w, r.Context(), req)
	t.cfg.Log.V(1).Info("mcp request handled",
		"method", req.Method,
		"durationMs", time.Since(started).Milliseconds())
}

func (t *transport) dispatch(w http.ResponseWriter, ctx context.Context, req JSONRPCRequest) {
	switch req.Method {
	case MethodInitialize:
		t.handleInitialize(w, req)
	case MethodToolsList:
		t.handleListTools(w, req)
	case MethodToolsCall:
		t.handleCallTool(w, ctx, req)
	default:
		writeJSONRPCError(w, req.ID, ErrorMethodNotFound, "method not found: "+req.Method)
	}
}

func (t *transport) handleInitialize(w http.ResponseWriter, req JSONRPCRequest) {
	result := InitializeResult{
		ProtocolVersion: ProtocolVersion,
		ServerInfo:      t.cfg.ServerInfo,
		Capabilities:    ServerCapabilities{Tools: &ToolsCapability{}},
	}
	writeJSONRPCResult(w, req.ID, result)
}

func (t *transport) handleListTools(w http.ResponseWriter, req JSONRPCRequest) {
	writeJSONRPCResult(w, req.ID, ListToolsResult{Tools: t.cfg.Adapter.ListTools()})
}

func (t *transport) handleCallTool(w http.ResponseWriter, ctx context.Context, req JSONRPCRequest) {
	var params CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, ErrorInvalidParams, err.Error())
		return
	}
	if params.Name == "" {
		writeJSONRPCError(w, req.ID, ErrorInvalidParams, "params.name is required")
		return
	}
	result := t.cfg.Adapter.CallTool(ctx, params.Name, params.Arguments)
	writeJSONRPCResult(w, req.ID, result)
}

func writeJSONRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	rb, err := json.Marshal(result)
	if err != nil {
		writeJSONRPCError(w, id, ErrorInternalError, err.Error())
		return
	}
	writeResponse(w, JSONRPCResponse{JSONRPC: JSONRPCVersion, ID: id, Result: rb})
}

func writeJSONRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	writeResponse(w, JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	})
}

func writeResponse(w http.ResponseWriter, resp JSONRPCResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Best-effort encode; on error the client gets a truncated response.
	// Logging from here would need the transport's logger, which isn't
	// threaded through — acceptable since the only realistic failure is
	// a closed connection where logging won't help anyway.
	_ = json.NewEncoder(w).Encode(resp)
}
