/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleInitialize tests the initialize handler.
func TestHandleInitialize(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// Create a mock connection (closed so we don't actually send)
	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	processID := 1234
	params := InitializeParams{
		ProcessID: &processID,
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "initialize",
		Params:  paramsBytes,
	}

	// Should not panic
	s.handleInitialize(context.Background(), c, msg)
}

// TestHandleInitializeInvalidParams tests initialize with invalid params.
func TestHandleInitializeInvalidParams(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	msg := &Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "initialize",
		Params:  json.RawMessage("invalid json"),
	}

	// Should not panic, will send error
	s.handleInitialize(context.Background(), c, msg)
}

// TestHandleShutdown tests the shutdown handler.
func TestHandleShutdown(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	msg := &Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "shutdown",
	}

	// Should not panic
	s.handleShutdown(context.Background(), c, msg)
}

// TestHandleDidOpenInvalidParams tests didOpen with invalid params.
func TestHandleDidOpenInvalidParams(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	msg := &Message{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params:  json.RawMessage("invalid json"),
	}

	// Should not panic
	s.handleDidOpen(context.Background(), c, msg)
}

// TestHandleDidOpen tests the didOpen handler.
func TestHandleDidOpen(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        "file:///test.yaml",
			LanguageID: "yaml",
			Version:    1,
			Text:       "apiVersion: v1\nkind: Test",
		},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params:  paramsBytes,
	}

	// Should not panic
	s.handleDidOpen(context.Background(), c, msg)

	// Verify document was stored
	doc := s.documents.Get("file:///test.yaml")
	assert.NotNil(t, doc)
	assert.Equal(t, "apiVersion: v1\nkind: Test", doc.Content)
}

// TestHandleDidChangeInvalidParams tests didChange with invalid params.
func TestHandleDidChangeInvalidParams(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	msg := &Message{
		JSONRPC: "2.0",
		Method:  "textDocument/didChange",
		Params:  json.RawMessage("invalid json"),
	}

	// Should not panic
	s.handleDidChange(context.Background(), c, msg)
}

// TestHandleDidChangeDocumentNotFound tests didChange for unknown document.
func TestHandleDidChangeDocumentNotFound(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: TextDocumentIdentifier{
				URI: "file:///nonexistent.yaml",
			},
			Version: 2,
		},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: "new content"},
		},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		Method:  "textDocument/didChange",
		Params:  paramsBytes,
	}

	// Should not panic
	s.handleDidChange(context.Background(), c, msg)
}

// TestHandleDidChange tests the didChange handler.
func TestHandleDidChange(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	// First open a document
	s.documents.Open("file:///test.yaml", "yaml", 1, "original content")

	params := DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: TextDocumentIdentifier{
				URI: "file:///test.yaml",
			},
			Version: 2,
		},
		ContentChanges: []TextDocumentContentChangeEvent{
			{Text: "updated content"},
		},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		Method:  "textDocument/didChange",
		Params:  paramsBytes,
	}

	// Should not panic
	s.handleDidChange(context.Background(), c, msg)

	// Verify document was updated
	doc := s.documents.Get("file:///test.yaml")
	assert.NotNil(t, doc)
	assert.Equal(t, "updated content", doc.Content)
	assert.Equal(t, 2, doc.Version)
}

// TestHandleDidCloseInvalidParams tests didClose with invalid params.
func TestHandleDidCloseInvalidParams(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	msg := &Message{
		JSONRPC: "2.0",
		Method:  "textDocument/didClose",
		Params:  json.RawMessage("invalid json"),
	}

	// Should not panic
	s.handleDidClose(context.Background(), c, msg)
}

// TestHandleDidClose tests the didClose handler.
func TestHandleDidClose(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	// First open a document
	s.documents.Open("file:///test.yaml", "yaml", 1, "content")

	params := DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		Method:  "textDocument/didClose",
		Params:  paramsBytes,
	}

	// Should not panic
	s.handleDidClose(context.Background(), c, msg)

	// Verify document was removed
	doc := s.documents.Get("file:///test.yaml")
	assert.Nil(t, doc)
}

// TestHandleMessage tests the message dispatcher.
func TestHandleMessage(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	tests := []struct {
		name   string
		method string
	}{
		{"initialized", "initialized"},
		// Skip "exit" as it tries to close the WebSocket connection
		{"unknown", "unknown/method"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &Message{
				JSONRPC: "2.0",
				Method:  tt.method,
			}

			// Should not panic
			s.handleMessage(context.Background(), c, msg)
		})
	}
}

// TestHandleMessageWithID tests unknown method with ID returns error.
func TestHandleMessageWithID(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	id := json.RawMessage("1")
	msg := &Message{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "unknown/method",
	}

	// Should not panic, will send method not found error
	s.handleMessage(context.Background(), c, msg)
}

// TestValidateAndPublish tests the validation and publish helper.
func TestValidateAndPublish(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "apiVersion: v1\nkind: Test",
		Version: 1,
	}

	// Should not panic
	s.validateAndPublish(context.Background(), c, doc)
}
