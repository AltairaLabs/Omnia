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
)

func TestGetKindCompletions(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	items := srv.getKindCompletions()

	if len(items) == 0 {
		t.Fatal("expected kind completions")
	}

	// Check for known kinds
	kinds := make(map[string]bool)
	for _, item := range items {
		kinds[item.Label] = true
	}

	// Check that we have some expected kinds
	expectedKinds := []string{"Tool", "Provider", "Scenario", "Arena", "Persona"}
	for _, kind := range expectedKinds {
		if !kinds[kind] {
			t.Errorf("expected kind %q in completions", kind)
		}
	}
}

func TestGetProviderTypeCompletions(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	items := srv.getProviderTypeCompletions()

	if len(items) == 0 {
		t.Fatal("expected type completions")
	}

	// Check for known provider types
	types := make(map[string]bool)
	for _, item := range items {
		types[item.Label] = true
	}

	expectedTypes := []string{"openai", "anthropic", "azure", "bedrock"}
	for _, typ := range expectedTypes {
		if !types[typ] {
			t.Errorf("expected provider type %q in completions", typ)
		}
	}
}

func TestGetTopLevelCompletions(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	items := srv.getTopLevelCompletions()

	if len(items) == 0 {
		t.Fatal("expected top-level completions")
	}

	// Check for some expected fields
	fields := make(map[string]bool)
	for _, item := range items {
		fields[item.Label] = true
	}

	// At minimum we expect kind and spec
	if !fields["kind"] {
		t.Error("expected 'kind' in top-level completions")
	}
	if !fields["spec"] {
		t.Error("expected 'spec' in top-level completions")
	}
}

func TestGetFieldCompletions(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	items := srv.getFieldCompletions(nil, Position{})

	if len(items) == 0 {
		t.Fatal("expected field completions")
	}

	// Should have at least some common fields
	fields := make(map[string]bool)
	for _, item := range items {
		fields[item.Label] = true
	}

	if !fields["name"] {
		t.Error("expected 'name' field in completions")
	}
	if !fields["description"] {
		t.Error("expected 'description' field in completions")
	}
}

func TestHandleCompletionInvalidParams(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  []byte("invalid json"),
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionDocumentNotFound(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///nonexistent.yaml",
		},
		Position: Position{Line: 0, Character: 0},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionEmptyLine(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with empty content
	srv.documents.Open("file:///test.yaml", "yaml", 1, "")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 0},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionKindField(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with kind field
	srv.documents.Open("file:///test.yaml", "yaml", 1, "kind:")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 5},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionTypeField(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with type field
	srv.documents.Open("file:///test.yaml", "yaml", 1, "type:")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 5},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionListItem(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with list item
	srv.documents.Open("file:///test.yaml", "yaml", 1, "-")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 1},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionToolRef(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with tool reference
	srv.documents.Open("file:///test.yaml", "yaml", 1, "  - tool:")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 9},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionProviderRef(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with provider reference
	srv.documents.Open("file:///test.yaml", "yaml", 1, "  - provider:")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 13},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionPromptRef(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with prompt reference
	srv.documents.Open("file:///test.yaml", "yaml", 1, "  - prompt:")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 11},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionPersonaRef(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with persona reference
	srv.documents.Open("file:///test.yaml", "yaml", 1, "persona:")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 8},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}

func TestHandleCompletionDefaultField(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with some content
	srv.documents.Open("file:///test.yaml", "yaml", 1, "spec:\n  ")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := CompletionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 1, Character: 2},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/completion",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleCompletion(context.Background(), c, msg)
}
