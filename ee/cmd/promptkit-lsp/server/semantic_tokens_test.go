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
	"reflect"
	"testing"

	"github.com/go-logr/logr"
)

func TestFindVariablesInLine(t *testing.T) {
	tests := []struct {
		name     string
		lineNum  int
		line     string
		expected []tokenLocation
	}{
		{
			name:     "no variables",
			lineNum:  0,
			line:     "This is a normal line",
			expected: nil,
		},
		{
			name:    "single variable",
			lineNum: 0,
			line:    "Hello {{name}}!",
			expected: []tokenLocation{
				{line: 0, startChar: 6, length: 2, tokenType: 0, modifiers: 0, text: "{{"},
				{line: 0, startChar: 8, length: 4, tokenType: 1, modifiers: 0, text: "name"},
				{line: 0, startChar: 12, length: 2, tokenType: 0, modifiers: 0, text: "}}"},
			},
		},
		{
			name:    "variable with spaces",
			lineNum: 5,
			line:    "Hello {{ username }}!",
			expected: []tokenLocation{
				{line: 5, startChar: 6, length: 2, tokenType: 0, modifiers: 0, text: "{{"},
				{line: 5, startChar: 9, length: 8, tokenType: 1, modifiers: 0, text: "username"},
				{line: 5, startChar: 18, length: 2, tokenType: 0, modifiers: 0, text: "}}"},
			},
		},
		{
			name:    "multiple variables",
			lineNum: 2,
			line:    "{{greeting}} {{name}}",
			expected: []tokenLocation{
				{line: 2, startChar: 0, length: 2, tokenType: 0, modifiers: 0, text: "{{"},
				{line: 2, startChar: 2, length: 8, tokenType: 1, modifiers: 0, text: "greeting"},
				{line: 2, startChar: 10, length: 2, tokenType: 0, modifiers: 0, text: "}}"},
				{line: 2, startChar: 13, length: 2, tokenType: 0, modifiers: 0, text: "{{"},
				{line: 2, startChar: 15, length: 4, tokenType: 1, modifiers: 0, text: "name"},
				{line: 2, startChar: 19, length: 2, tokenType: 0, modifiers: 0, text: "}}"},
			},
		},
		{
			name:    "variable with underscore",
			lineNum: 1,
			line:    "{{user_name}}",
			expected: []tokenLocation{
				{line: 1, startChar: 0, length: 2, tokenType: 0, modifiers: 0, text: "{{"},
				{line: 1, startChar: 2, length: 9, tokenType: 1, modifiers: 0, text: "user_name"},
				{line: 1, startChar: 11, length: 2, tokenType: 0, modifiers: 0, text: "}}"},
			},
		},
		{
			name:    "variable with numbers",
			lineNum: 0,
			line:    "{{var123}}",
			expected: []tokenLocation{
				{line: 0, startChar: 0, length: 2, tokenType: 0, modifiers: 0, text: "{{"},
				{line: 0, startChar: 2, length: 6, tokenType: 1, modifiers: 0, text: "var123"},
				{line: 0, startChar: 8, length: 2, tokenType: 0, modifiers: 0, text: "}}"},
			},
		},
		{
			name:     "invalid variable (starts with number)",
			lineNum:  0,
			line:     "{{123var}}",
			expected: nil,
		},
		{
			name:     "empty braces",
			lineNum:  0,
			line:     "{{}}",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findVariablesInLine(tt.lineNum, tt.line)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("findVariablesInLine(%d, %q) = %v, expected %v",
					tt.lineNum, tt.line, result, tt.expected)
			}
		})
	}
}

func TestEncodeSemanticTokens(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []tokenLocation
		expected []int32
	}{
		{
			name:     "empty tokens",
			tokens:   []tokenLocation{},
			expected: []int32{},
		},
		{
			name: "single token",
			tokens: []tokenLocation{
				{line: 0, startChar: 5, length: 4, tokenType: 1, modifiers: 0},
			},
			expected: []int32{0, 5, 4, 1, 0},
		},
		{
			name: "two tokens same line",
			tokens: []tokenLocation{
				{line: 0, startChar: 0, length: 2, tokenType: 0, modifiers: 0},
				{line: 0, startChar: 5, length: 4, tokenType: 1, modifiers: 0},
			},
			expected: []int32{
				0, 0, 2, 0, 0, // first token
				0, 5, 4, 1, 0, // second token (deltaLine=0, deltaChar=5)
			},
		},
		{
			name: "tokens on different lines",
			tokens: []tokenLocation{
				{line: 0, startChar: 5, length: 4, tokenType: 1, modifiers: 0},
				{line: 2, startChar: 3, length: 2, tokenType: 0, modifiers: 0},
			},
			expected: []int32{
				0, 5, 4, 1, 0, // first token
				2, 3, 2, 0, 0, // second token (deltaLine=2, char resets)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeSemanticTokens(tt.tokens)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("encodeSemanticTokens(%v) = %v, expected %v",
					tt.tokens, result, tt.expected)
			}
		})
	}
}

func TestGetSemanticTokensLegend(t *testing.T) {
	legend := GetSemanticTokensLegend()

	if len(legend.TokenTypes) == 0 {
		t.Error("TokenTypes should not be empty")
	}
	if len(legend.TokenModifiers) == 0 {
		t.Error("TokenModifiers should not be empty")
	}

	// Verify expected token types exist
	expectedTypes := []string{"variable", "parameter"}
	for _, expected := range expectedTypes {
		found := false
		for _, tokenType := range legend.TokenTypes {
			if tokenType == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected token type %q not found in legend", expected)
		}
	}
}

func TestGetSemanticTokensOptions(t *testing.T) {
	options := GetSemanticTokensOptions()

	if options == nil {
		t.Fatal("Options should not be nil")
	}
	if !options.Full {
		t.Error("Full should be true")
	}
	if len(options.Legend.TokenTypes) == 0 {
		t.Error("Legend.TokenTypes should not be empty")
	}
}

func TestExtractSemanticTokens(t *testing.T) {
	// Create a minimal server for testing
	s := &Server{
		documents: NewDocumentStore(),
	}

	// Test with a document containing variables
	doc := s.documents.Open("file:///test.yaml", "yaml", 1, `
system_template: |
  You are a helpful assistant for {{company_name}}.
  Your role is to assist {{user_name}} with their questions.
  Please be professional and {{tone}}.
`)

	tokens := s.extractSemanticTokens(doc)

	// Should find 9 tokens (3 variables * 3 tokens each: {{, name, }})
	expectedTokenCount := 9
	if len(tokens) != expectedTokenCount {
		t.Errorf("Expected %d tokens, got %d", expectedTokenCount, len(tokens))
	}

	// Verify first variable (company_name)
	if len(tokens) >= 3 {
		// Opening braces
		if tokens[0].text != "{{" {
			t.Errorf("Expected '{{', got %q", tokens[0].text)
		}
		// Variable name
		if tokens[1].text != "company_name" {
			t.Errorf("Expected 'company_name', got %q", tokens[1].text)
		}
		// Closing braces
		if tokens[2].text != "}}" {
			t.Errorf("Expected '}}', got %q", tokens[2].text)
		}
	}
}

func TestExtractSemanticTokens_EmptyDocument(t *testing.T) {
	s := &Server{
		documents: NewDocumentStore(),
	}

	doc := s.documents.Open("file:///empty.yaml", "yaml", 1, "")

	tokens := s.extractSemanticTokens(doc)

	if len(tokens) != 0 {
		t.Errorf("Expected 0 tokens for empty document, got %d", len(tokens))
	}
}

func TestExtractSemanticTokens_NoVariables(t *testing.T) {
	s := &Server{
		documents: NewDocumentStore(),
	}

	doc := s.documents.Open("file:///no-vars.yaml", "yaml", 1, `
kind: Persona
metadata:
  name: test
spec:
  system_template: "This has no variables."
`)

	tokens := s.extractSemanticTokens(doc)

	if len(tokens) != 0 {
		t.Errorf("Expected 0 tokens for document without variables, got %d", len(tokens))
	}
}

func TestVariablePatternMatches(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string // variable names
	}{
		{"{{name}}", []string{"name"}},
		{"{{ name }}", []string{"name"}},
		{"{{a}} and {{b}}", []string{"a", "b"}},
		{"{{_underscore}}", []string{"_underscore"}},
		{"{{var123}}", []string{"var123"}},
		{"no variables here", nil},
		{"{{123invalid}}", nil}, // starts with number
		{"{{}}", nil},           // empty
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			matches := variablePattern.FindAllStringSubmatch(tc.input, -1)
			var got []string
			for _, m := range matches {
				if len(m) > 1 {
					got = append(got, m[1])
				}
			}
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("For %q: got %v, expected %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestHandleSemanticTokensFullInvalidParams(t *testing.T) {
	srv, _ := newTestServer()

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/semanticTokens/full",
		Params:  []byte("invalid json"),
	}

	// Should not panic
	srv.handleSemanticTokensFull(context.Background(), c, msg)
}

func TestHandleSemanticTokensFullDocumentNotFound(t *testing.T) {
	srv, _ := newTestServer()

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := SemanticTokensParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///nonexistent.yaml",
		},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/semanticTokens/full",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleSemanticTokensFull(context.Background(), c, msg)
}

func TestHandleSemanticTokensFullWithVariables(t *testing.T) {
	srv, _ := newTestServer()

	// Open a document with variables
	srv.documents.Open("file:///test.yaml", "yaml", 1, "Hello {{name}}!")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := SemanticTokensParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/semanticTokens/full",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleSemanticTokensFull(context.Background(), c, msg)
}

func TestHandleSemanticTokensFullNoVariables(t *testing.T) {
	srv, _ := newTestServer()

	// Open a document without variables
	srv.documents.Open("file:///test.yaml", "yaml", 1, "kind: Tool\nname: test")

	c := &Connection{
		workspace:  "test",
		projectID:  "proj",
		closed:     true,
		pendingReq: make(map[int]chan *Response),
	}

	params := SemanticTokensParams{
		TextDocument: TextDocumentIdentifier{
			URI: "file:///test.yaml",
		},
	}
	paramsBytes, _ := json.Marshal(params)

	msg := &Message{
		JSONRPC: "2.0",
		ID:      []byte("1"),
		Method:  "textDocument/semanticTokens/full",
		Params:  paramsBytes,
	}

	// Should not panic
	srv.handleSemanticTokensFull(context.Background(), c, msg)
}

func newTestServer() (*Server, error) {
	cfg := Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}
	return New(cfg, logr.Discard())
}
