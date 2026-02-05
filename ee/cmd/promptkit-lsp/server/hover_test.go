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

// TestHandleHoverInvalidParams tests hover with invalid params.
func TestHandleHoverInvalidParams(t *testing.T) {
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
		Method:  "textDocument/hover",
		Params:  json.RawMessage("invalid json"),
	}

	// Should not panic
	s.handleHover(context.Background(), c, msg)
}

// TestHandleHoverDocumentNotFound tests hover when document doesn't exist.
func TestHandleHoverDocumentNotFound(t *testing.T) {
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

	params := HoverParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///nonexistent.yaml",
		},
		Position: Position{Line: 0, Character: 0},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "textDocument/hover",
		Params:  paramsBytes,
	}

	// Should not panic, returns nil
	s.handleHover(context.Background(), c, msg)
}

// TestHandleHoverOnFieldName tests hover over a field name.
func TestHandleHoverOnFieldName(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// Open a document
	s.documents.Open("file:///test.yaml", "yaml", 1, "kind: Tool\nname: my-tool")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := HoverParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 2}, // On "kind"
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "textDocument/hover",
		Params:  paramsBytes,
	}

	// Should not panic
	s.handleHover(context.Background(), c, msg)
}

// TestHandleHoverOnValue tests hover over a value.
func TestHandleHoverOnValue(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// Open a document with a kind value
	s.documents.Open("file:///test.yaml", "yaml", 1, "kind: Tool")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := HoverParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
		Position: Position{Line: 0, Character: 7}, // On "Tool"
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage("1"),
		Method:  "textDocument/hover",
		Params:  paramsBytes,
	}

	// Should not panic
	s.handleHover(context.Background(), c, msg)
}

// TestGetFieldNameAtPosition tests extracting field name at position.
func TestGetFieldNameAtPosition(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "kind: Tool\nname: my-tool\n  description: A test tool",
		Lines:   []string{"kind: Tool", "name: my-tool", "  description: A test tool"},
		Version: 1,
	}

	tests := []struct {
		name     string
		pos      Position
		expected string
	}{
		{"on kind field", Position{Line: 0, Character: 2}, "kind"},
		{"on name field", Position{Line: 1, Character: 2}, "name"},
		{"on value (after colon)", Position{Line: 0, Character: 7}, ""},
		{"on indented field", Position{Line: 2, Character: 5}, "description"},
		{"out of bounds line", Position{Line: 10, Character: 0}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.getFieldNameAtPosition(doc, tt.pos)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetKindValueHover tests hover for kind values.
func TestGetKindValueHover(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	tests := []struct {
		kind     string
		hasHover bool
	}{
		{"Tool", true},
		{"Provider", true},
		{"Prompt", true},
		{"Scenario", true},
		{"Arena", true},
		{"Persona", true},
		{"Unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			hover := s.getKindValueHover(tt.kind)
			if tt.hasHover {
				assert.NotNil(t, hover)
			} else {
				assert.Nil(t, hover)
			}
		})
	}
}

// TestGetTypeValueHover tests hover for provider type values.
func TestGetTypeValueHover(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	tests := []struct {
		providerType string
		hasHover     bool
	}{
		{"openai", true},
		{"anthropic", true},
		{"azure", true},
		{"bedrock", true},
		{"vertex", true},
		{"ollama", true},
		{"custom", true},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.providerType, func(t *testing.T) {
			hover := s.getTypeValueHover(tt.providerType)
			if tt.hasHover {
				assert.NotNil(t, hover)
			} else {
				assert.Nil(t, hover)
			}
		})
	}
}

// TestGetValueHover tests the value hover dispatcher.
func TestGetValueHover(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	// Test with kind value
	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "kind: Tool",
		Lines:   []string{"kind: Tool"},
		Version: 1,
	}

	hover := s.getValueHover(doc, Position{Line: 0, Character: 7}, "Tool")
	assert.NotNil(t, hover)

	// Test with type value
	doc2 := &Document{
		URI:     "file:///test.yaml",
		Content: "type: openai",
		Lines:   []string{"type: openai"},
		Version: 1,
	}

	hover2 := s.getValueHover(doc2, Position{Line: 0, Character: 7}, "openai")
	assert.NotNil(t, hover2)

	// Test with out of bounds line
	hover3 := s.getValueHover(doc, Position{Line: 10, Character: 0}, "Tool")
	assert.Nil(t, hover3)
}

// TestGetWordRange tests getting word range.
func TestGetWordRange(t *testing.T) {
	cfg := Config{
		Addr:            ":8080",
		HealthAddr:      ":8081",
		DashboardAPIURL: "http://localhost:3000",
	}

	s, err := New(cfg, logr.Discard())
	require.NoError(t, err)

	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "kind: Tool",
		Lines:   []string{"kind: Tool"},
		Version: 1,
	}

	// Test with valid position
	wordRange := s.getWordRange(doc, Position{Line: 0, Character: 2}, "kind")
	assert.NotNil(t, wordRange)

	// Test with out of bounds line
	wordRange2 := s.getWordRange(doc, Position{Line: 10, Character: 0}, "kind")
	assert.Nil(t, wordRange2)
}
