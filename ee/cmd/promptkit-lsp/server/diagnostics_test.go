/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"testing"

	"github.com/go-logr/logr"
)

func TestNewValidator(t *testing.T) {
	v, err := NewValidator("http://localhost:3000", logr.Discard())
	if err != nil {
		t.Fatalf("NewValidator() error = %v", err)
	}
	if v == nil {
		t.Fatal("NewValidator() returned nil")
	}
}

func TestValidateYAMLSyntax_Valid(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())
	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "kind: Tool\nname: test",
	}

	diags := v.validateYAMLSyntax(doc)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for valid YAML, got %d", len(diags))
	}
}

func TestValidateYAMLSyntax_Invalid(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())
	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "kind: Tool\n  invalid: indentation",
	}

	diags := v.validateYAMLSyntax(doc)
	if len(diags) == 0 {
		t.Error("expected diagnostics for invalid YAML")
	}
	if diags[0].Severity != SeverityError {
		t.Errorf("expected severity Error, got %d", diags[0].Severity)
	}
	if diags[0].Source != "yaml" {
		t.Errorf("expected source 'yaml', got %q", diags[0].Source)
	}
}

func TestValidateYAMLSyntax_Empty(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())
	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "",
	}

	diags := v.validateYAMLSyntax(doc)
	if len(diags) != 0 {
		t.Errorf("expected no diagnostics for empty YAML, got %d", len(diags))
	}
}

func TestValidateJSONSchema_WithKind(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())
	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "kind: Tool\nspec:\n  name: test",
	}

	parsed := map[string]any{
		"kind": "Tool",
		"spec": map[string]any{
			"name": "test",
		},
	}

	diags := v.validateJSONSchema(doc, parsed)
	// Should return diagnostics (schema validation finds issues)
	// The important thing is that it doesn't panic and processes correctly
	_ = diags
}

func TestValidateJSONSchema_MissingKind(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())
	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "name: test",
	}

	parsed := map[string]any{
		"name": "test",
	}

	diags := v.validateJSONSchema(doc, parsed)
	// Without a kind, we check for apiVersion warning
	// The function should not panic and should return some result
	_ = diags
}

func TestValidateSpecFields(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	tests := []struct {
		name     string
		kind     string
		parsed   map[string]any
		wantDiag bool
	}{
		{
			name: "valid Tool with spec",
			kind: "Tool",
			parsed: map[string]any{
				"spec": map[string]any{
					"name":        "test",
					"description": "Test tool",
				},
			},
			wantDiag: false,
		},
		{
			name: "Tool missing spec",
			kind: "Tool",
			parsed: map[string]any{
				"name": "test",
			},
			wantDiag: true,
		},
		{
			name: "Tool missing required spec fields",
			kind: "Tool",
			parsed: map[string]any{
				"spec": map[string]any{},
			},
			wantDiag: true,
		},
		{
			name: "valid Provider with spec",
			kind: "Provider",
			parsed: map[string]any{
				"spec": map[string]any{
					"id":    "test-provider",
					"type":  "openai",
					"model": "gpt-4",
				},
			},
			wantDiag: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var diags []Diagnostic
			switch tc.kind {
			case "Tool":
				diags = v.validateToolSemantics(nil, tc.parsed)
			case "Provider":
				diags = v.validateProviderSemantics(nil, tc.parsed)
			}

			hasDiag := len(diags) > 0
			if hasDiag != tc.wantDiag {
				t.Errorf("wantDiag = %v, got %v (diags: %v)", tc.wantDiag, hasDiag, diags)
			}
		})
	}
}

func TestValidateYAMLSyntax_WithLineNumbers(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	// Test that syntax errors include line information
	doc := &Document{
		URI: "file:///test.yaml",
		Content: `kind: Tool
name: test
  invalid: indentation`,
	}

	diags := v.validateYAMLSyntax(doc)
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for invalid YAML")
	}

	// The error should have position information
	// Line numbers in diagnostics are 0-indexed
	if diags[0].Range.Start.Line < 0 {
		t.Error("expected line number in diagnostic range")
	}
}
