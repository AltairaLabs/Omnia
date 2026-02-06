/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"context"
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

func TestValidatePromptConfigSemantics(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	doc := &Document{
		URI:     "file:///prompt.yaml",
		Content: "kind: Prompt\nspec:\n  name: test",
		Lines:   []string{"kind: Prompt", "spec:", "  name: test"},
	}

	parsed := map[string]any{
		"kind": "Prompt",
		"spec": map[string]any{
			"name": "test",
		},
	}

	diags := v.validatePromptConfigSemantics(doc, parsed)
	// Just verify it doesn't panic
	_ = diags
}

func TestValidateScenarioSemantics(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	doc := &Document{
		URI:     "file:///scenario.yaml",
		Content: "kind: Scenario\nspec:\n  name: test",
		Lines:   []string{"kind: Scenario", "spec:", "  name: test"},
	}

	parsed := map[string]any{
		"kind": "Scenario",
		"spec": map[string]any{
			"name": "test",
		},
	}

	diags := v.validateScenarioSemantics(doc, parsed)
	// Just verify it doesn't panic
	_ = diags
}

func TestValidateArenaSemantics(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	doc := &Document{
		URI:     "file:///arena.yaml",
		Content: "kind: Arena\nspec:\n  name: test",
		Lines:   []string{"kind: Arena", "spec:", "  name: test"},
	}

	parsed := map[string]any{
		"kind": "Arena",
		"spec": map[string]any{
			"name": "test",
		},
	}

	diags := v.validateArenaSemantics(doc, parsed)
	// Just verify it doesn't panic
	_ = diags
}

func TestValidatePersonaSemantics(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	doc := &Document{
		URI:     "file:///persona.yaml",
		Content: "kind: Persona\nspec:\n  name: test",
		Lines:   []string{"kind: Persona", "spec:", "  name: test"},
	}

	parsed := map[string]any{
		"kind": "Persona",
		"spec": map[string]any{
			"name": "test",
		},
	}

	diags := v.validatePersonaSemantics(doc, parsed)
	// Just verify it doesn't panic
	_ = diags
}

func TestValidateSemantics(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	tests := []struct {
		name   string
		kind   string
		parsed map[string]any
	}{
		{
			name: "Tool semantics",
			kind: "Tool",
			parsed: map[string]any{
				"kind": "Tool",
				"spec": map[string]any{"name": "test", "description": "desc"},
			},
		},
		{
			name: "Provider semantics",
			kind: "Provider",
			parsed: map[string]any{
				"kind": "Provider",
				"spec": map[string]any{"id": "test", "type": "openai", "model": "gpt-4"},
			},
		},
		{
			name: "Prompt semantics",
			kind: "Prompt",
			parsed: map[string]any{
				"kind": "Prompt",
				"spec": map[string]any{"name": "test"},
			},
		},
		{
			name: "Scenario semantics",
			kind: "Scenario",
			parsed: map[string]any{
				"kind": "Scenario",
				"spec": map[string]any{"name": "test"},
			},
		},
		{
			name: "Arena semantics",
			kind: "Arena",
			parsed: map[string]any{
				"kind": "Arena",
				"spec": map[string]any{"name": "test"},
			},
		},
		{
			name: "Persona semantics",
			kind: "Persona",
			parsed: map[string]any{
				"kind": "Persona",
				"spec": map[string]any{"name": "test"},
			},
		},
		{
			name: "Unknown kind",
			kind: "Unknown",
			parsed: map[string]any{
				"kind": "Unknown",
				"spec": map[string]any{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := &Document{
				URI:     "file:///test.yaml",
				Content: "kind: " + tc.kind,
				Lines:   []string{"kind: " + tc.kind},
			}

			diags := v.validateSemantics(doc, tc.parsed)
			// Just verify it doesn't panic
			_ = diags
		})
	}
}

func TestValidateCrossReferences(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "kind: Tool\nspec:\n  name: test",
		Lines:   []string{"kind: Tool", "spec:", "  name: test"},
	}

	// Mock project files would be empty since we can't make HTTP calls
	diags := v.validateCrossReferences(context.Background(), doc, "workspace", "project")
	// Should not panic with nil context
	_ = diags
}

func TestExtractYAMLPosition(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		wantNil  bool
		wantLine int
	}{
		{
			name:     "with line and column",
			errMsg:   "yaml: line 5: could not find expected ':'",
			wantNil:  false,
			wantLine: 4, // 0-indexed
		},
		{
			name:    "without line info",
			errMsg:  "yaml: some other error",
			wantNil: true,
		},
		{
			name:    "empty error",
			errMsg:  "",
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pos := extractYAMLPosition(tc.errMsg)
			if tc.wantNil {
				if pos != nil {
					t.Errorf("expected nil position, got %v", pos)
				}
				return
			}
			if pos == nil {
				t.Fatal("expected non-nil position")
			}
			if pos.Line != tc.wantLine {
				t.Errorf("line = %d, want %d", pos.Line, tc.wantLine)
			}
		})
	}
}

func TestFindFieldPosition(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	doc := &Document{
		URI:     "file:///test.yaml",
		Content: "kind: Tool\nspec:\n  name: test",
		Lines:   []string{"kind: Tool", "spec:", "  name: test"},
	}

	tests := []struct {
		name      string
		fieldPath string
		wantLine  int
	}{
		{"root field", "kind", 0},
		{"nested field", "spec", 1},
		{"deeply nested", "spec.name", 2},
		{"not found", "nonexistent", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line, _ := v.findFieldPosition(doc, tc.fieldPath)
			if line != tc.wantLine {
				t.Errorf("findFieldPosition(%q) line = %d, want %d", tc.fieldPath, line, tc.wantLine)
			}
		})
	}
}

func TestValidateDocument(t *testing.T) {
	v, _ := NewValidator("http://localhost:3000", logr.Discard())

	tests := []struct {
		name    string
		content string
	}{
		{"valid YAML", "kind: Tool\nspec:\n  name: test"},
		{"empty content", ""},
		{"only comments", "# This is a comment"},
		{"invalid YAML", "kind: Tool\n  bad: indent"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := &Document{
				URI:     "file:///test.yaml",
				Content: tc.content,
				Lines:   splitLines(tc.content),
			}

			// Should not panic
			diags := v.ValidateDocument(context.Background(), doc, "workspace", "project")
			_ = diags
		})
	}
}
