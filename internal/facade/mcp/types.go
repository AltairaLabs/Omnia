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

// Package mcp implements the Model Context Protocol (MCP) server facade
// for function-mode AgentRuntimes. Wire format: Streamable HTTP transport
// per the MCP 2025-03-26 spec — a single POST /mcp endpoint carrying
// JSON-RPC 2.0 envelopes.
//
// This file defines the wire types only; handlers and the server live in
// sibling files added by subsequent tasks.
package mcp

import "encoding/json"

// ProtocolVersion is the MCP protocol version this package implements.
// Reported in the InitializeResult.
const ProtocolVersion = "2025-03-26"

// JSONRPCVersion is the JSON-RPC version string carried on every envelope.
const JSONRPCVersion = "2.0"

// ContentTypeText discriminates a ContentPart that carries plain text
// (as opposed to "image" or "resource" parts the spec also defines but
// Omnia's function pods don't produce today).
const ContentTypeText = "text"

// --- JSON-RPC 2.0 envelope ---

// JSONRPCRequest is a JSON-RPC 2.0 request frame. ID is json.RawMessage
// so we can echo whatever shape the client sent (number / string / null)
// back unchanged in the corresponding response.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response frame. Exactly one of
// Result and Error should be populated.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes. Reserved for protocol-level
// failures (malformed envelope, unknown method, bad params shape).
// Tool-level failures (input/output validation, runtime errors) are
// returned as CallToolResult{IsError: true} per MCP convention.
const (
	ErrorParseError     = -32700
	ErrorInvalidRequest = -32600
	ErrorMethodNotFound = -32601
	ErrorInvalidParams  = -32602
	ErrorInternalError  = -32603
)

// --- MCP protocol types ---

// ServerInfo identifies the MCP server to clients via initialize.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolsCapability advertises tools-related server capabilities.
// ListChanged signals the server emits tools/list_changed notifications
// when its tool set changes; Omnia's function pods don't (the tool set
// is fixed at startup), so we leave this false.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerCapabilities advertises which optional MCP capabilities the
// server supports. Function-mode pods only support tools; prompts,
// resources, sampling, and logging are not advertised.
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// InitializeResult is returned from the initialize method.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
}

// Tool describes one MCP tool available to clients.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ListToolsResult is returned from the tools/list method.
type ListToolsResult struct {
	Tools []Tool `json:"tools"`
}

// CallToolParams is the params shape for tools/call.
type CallToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ContentPart is one content item in a CallToolResult. Type is typically
// "text"; future types ("image", "resource") are spec-defined but not
// produced by Omnia's function pods.
type ContentPart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// CallToolResult is returned from the tools/call method. IsError=true
// means the tool itself failed (e.g. input validation, runtime error,
// output validation) — protocol-level failures are surfaced as
// JSONRPCError instead.
type CallToolResult struct {
	Content []ContentPart `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}
