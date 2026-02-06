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

func TestHandleDefinitionInvalidParams(t *testing.T) {
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
		Method:  "textDocument/definition",
		Params:  []byte("invalid json"),
	}

	// Should not panic
	srv.handleDefinition(context.Background(), c, msg)
}

func TestHandleDefinitionDocumentNotFound(t *testing.T) {
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

	params := DefinitionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///nonexistent.yaml",
		},
		Position: Position{Line: 0, Character: 0},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/definition",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleDefinition(context.Background(), c, msg)
}

func TestHandleDefinitionNoReference(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document without references
	srv.documents.Open("file:///test.yaml", "yaml", 1, "kind: Tool\nname: test")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := DefinitionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 2},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/definition",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleDefinition(context.Background(), c, msg)
}

func TestHandleDefinitionWithToolReference(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Open a document with a tool reference
	srv.documents.Open("file:///test.yaml", "yaml", 1, "tool: my-tool")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := DefinitionParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 8},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/definition",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleDefinition(context.Background(), c, msg)
}

func TestFindReferenceAtPosition(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	tests := []struct {
		name        string
		content     string
		pos         Position
		wantRefType string
		wantRefName string
	}{
		{
			name:        "tool reference",
			content:     "tool: my-tool",
			pos:         Position{Line: 0, Character: 8},
			wantRefType: "tool",
			wantRefName: "my-tool",
		},
		{
			name:        "provider reference",
			content:     "provider: openai",
			pos:         Position{Line: 0, Character: 12},
			wantRefType: "provider",
			wantRefName: "openai",
		},
		{
			name:        "prompt reference",
			content:     "prompt: default",
			pos:         Position{Line: 0, Character: 10},
			wantRefType: "prompt",
			wantRefName: "default",
		},
		{
			name:        "no reference",
			content:     "kind: Tool",
			pos:         Position{Line: 0, Character: 2},
			wantRefType: "",
			wantRefName: "",
		},
		{
			name:        "out of bounds line",
			content:     "tool: test",
			pos:         Position{Line: 10, Character: 0},
			wantRefType: "",
			wantRefName: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := srv.documents.Open("file:///test.yaml", "yaml", 1, tc.content)
			refType, refName := srv.findReferenceAtPosition(doc, tc.pos)
			if refType != tc.wantRefType {
				t.Errorf("findReferenceAtPosition() refType = %q, want %q", refType, tc.wantRefType)
			}
			if refName != tc.wantRefName {
				t.Errorf("findReferenceAtPosition() refName = %q, want %q", refName, tc.wantRefName)
			}
		})
	}
}

func TestFindReferenceAtPosition_ListItem(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	content := `tools:
  - my-tool
  - other-tool`

	doc := srv.documents.Open("file:///test.yaml", "yaml", 1, content)

	// Test list item reference
	refType, refName := srv.findReferenceAtPosition(doc, Position{Line: 1, Character: 5})
	if refType != "tool" {
		t.Errorf("expected refType 'tool', got %q", refType)
	}
	if refName != "my-tool" {
		t.Errorf("expected refName 'my-tool', got %q", refName)
	}
}

func TestFindParentListType(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	tests := []struct {
		name     string
		content  string
		lineNum  int
		expected string
	}{
		{
			name:     "tools list",
			content:  "tools:\n  - item",
			lineNum:  1,
			expected: "tool",
		},
		{
			name:     "providers list",
			content:  "providers:\n  - item",
			lineNum:  1,
			expected: "provider",
		},
		{
			name:     "prompts list",
			content:  "prompts:\n  - item",
			lineNum:  1,
			expected: "prompt",
		},
		{
			name:     "personas list",
			content:  "personas:\n  - item",
			lineNum:  1,
			expected: "persona",
		},
		{
			name:     "scenarios list",
			content:  "scenarios:\n  - item",
			lineNum:  1,
			expected: "scenario",
		},
		{
			name:     "no list parent",
			content:  "name: test\nvalue: item",
			lineNum:  1,
			expected: "",
		},
		{
			name:     "unknown list",
			content:  "unknown:\n  - item",
			lineNum:  1,
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := srv.documents.Open("file:///test.yaml", "yaml", 1, tc.content)
			result := srv.findParentListType(doc, tc.lineNum)
			if result != tc.expected {
				t.Errorf("findParentListType() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestGetIndentation(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	tests := []struct {
		line     string
		expected int
	}{
		{"no indent", 0},
		{"  two spaces", 2},
		{"    four spaces", 4},
		{"\ttab", 2},
		{"\t\ttwo tabs", 4},
		{"  \tmixed", 4},
		{"", 0},
	}

	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			result := srv.getIndentation(tc.line)
			if result != tc.expected {
				t.Errorf("getIndentation(%q) = %d, want %d", tc.line, result, tc.expected)
			}
		})
	}
}

func TestBuildFileURI(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	uri := srv.buildFileURI("workspace", "project", "tools/my-tool.yaml")
	expected := "promptkit://workspace/project/tools/my-tool.yaml"
	if uri != expected {
		t.Errorf("buildFileURI() = %q, want %q", uri, expected)
	}
}

func TestFindDefinitionLocation(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	// Test with unknown ref type
	result := srv.findDefinitionLocation(context.Background(), "ws", "proj", "unknown", "name")
	if result != nil {
		t.Error("expected nil for unknown ref type")
	}
}
