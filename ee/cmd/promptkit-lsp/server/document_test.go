/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"testing"
)

func TestDocumentStore_Open(t *testing.T) {
	store := NewDocumentStore()

	doc := store.Open("file:///test.yaml", "yaml", 1, "kind: Tool\nname: test")

	if doc == nil {
		t.Fatal("expected document to be created")
	}
	if doc.URI != "file:///test.yaml" {
		t.Errorf("expected URI 'file:///test.yaml', got %q", doc.URI)
	}
	if doc.LanguageID != "yaml" {
		t.Errorf("expected LanguageID 'yaml', got %q", doc.LanguageID)
	}
	if doc.Version != 1 {
		t.Errorf("expected Version 1, got %d", doc.Version)
	}
	if doc.Content != "kind: Tool\nname: test" {
		t.Errorf("expected Content 'kind: Tool\\nname: test', got %q", doc.Content)
	}
}

func TestDocumentStore_Get(t *testing.T) {
	store := NewDocumentStore()
	store.Open("file:///test.yaml", "yaml", 1, "content")

	doc := store.Get("file:///test.yaml")
	if doc == nil {
		t.Fatal("expected to find document")
	}

	doc = store.Get("file:///nonexistent.yaml")
	if doc != nil {
		t.Error("expected nil for nonexistent document")
	}
}

func TestDocumentStore_Update(t *testing.T) {
	store := NewDocumentStore()
	store.Open("file:///test.yaml", "yaml", 1, "initial content")

	// Update with full content
	changes := []TextDocumentContentChangeEvent{
		{Text: "updated content"},
	}
	doc := store.Update("file:///test.yaml", 2, changes)

	if doc == nil {
		t.Fatal("expected document to be updated")
	}
	if doc.Version != 2 {
		t.Errorf("expected Version 2, got %d", doc.Version)
	}
	if doc.Content != "updated content" {
		t.Errorf("expected Content 'updated content', got %q", doc.Content)
	}
}

func TestDocumentStore_Update_Incremental(t *testing.T) {
	store := NewDocumentStore()
	store.Open("file:///test.yaml", "yaml", 1, "hello world")

	// Update with incremental change - replace "world" with "there"
	changes := []TextDocumentContentChangeEvent{
		{
			Range: &Range{
				Start: Position{Line: 0, Character: 6},
				End:   Position{Line: 0, Character: 11},
			},
			Text: "there",
		},
	}
	doc := store.Update("file:///test.yaml", 2, changes)

	if doc == nil {
		t.Fatal("expected document to be updated")
	}
	if doc.Content != "hello there" {
		t.Errorf("expected Content 'hello there', got %q", doc.Content)
	}
}

func TestDocumentStore_Close(t *testing.T) {
	store := NewDocumentStore()
	store.Open("file:///test.yaml", "yaml", 1, "content")

	store.Close("file:///test.yaml")

	doc := store.Get("file:///test.yaml")
	if doc != nil {
		t.Error("expected document to be removed after close")
	}
}

func TestDocument_PositionToOffset(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Open("file:///test.yaml", "yaml", 1, "line1\nline2\nline3")

	tests := []struct {
		pos    Position
		offset int
	}{
		{Position{Line: 0, Character: 0}, 0},
		{Position{Line: 0, Character: 5}, 5},
		{Position{Line: 1, Character: 0}, 6},
		{Position{Line: 1, Character: 3}, 9},
		{Position{Line: 2, Character: 0}, 12},
		{Position{Line: 2, Character: 5}, 17},
	}

	for _, tc := range tests {
		offset := doc.PositionToOffset(tc.pos)
		if offset != tc.offset {
			t.Errorf("PositionToOffset(%v) = %d, want %d", tc.pos, offset, tc.offset)
		}
	}
}

func TestDocument_OffsetToPosition(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Open("file:///test.yaml", "yaml", 1, "line1\nline2\nline3")

	tests := []struct {
		offset int
		pos    Position
	}{
		{0, Position{Line: 0, Character: 0}},
		{5, Position{Line: 0, Character: 5}},
		{6, Position{Line: 1, Character: 0}},
		{9, Position{Line: 1, Character: 3}},
		{12, Position{Line: 2, Character: 0}},
		{17, Position{Line: 2, Character: 5}},
	}

	for _, tc := range tests {
		pos := doc.OffsetToPosition(tc.offset)
		if pos != tc.pos {
			t.Errorf("OffsetToPosition(%d) = %v, want %v", tc.offset, pos, tc.pos)
		}
	}
}

func TestDocument_OffsetToPosition_OutOfBounds(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Open("file:///test.yaml", "yaml", 1, "short")

	// Test negative offset
	pos := doc.OffsetToPosition(-1)
	if pos.Line != 0 || pos.Character != 0 {
		t.Errorf("expected position (0,0) for negative offset, got %v", pos)
	}

	// Test offset beyond content length
	pos = doc.OffsetToPosition(100)
	if pos.Line != 0 {
		t.Errorf("expected last line for large offset, got line %d", pos.Line)
	}
}

func TestDocument_GetWordAtPosition(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Open("file:///test.yaml", "yaml", 1, "kind: Tool\nname: my_tool")

	tests := []struct {
		name string
		pos  Position
		word string
	}{
		{"word at start of line", Position{Line: 0, Character: 2}, "kind"},
		{"word after colon", Position{Line: 0, Character: 7}, "Tool"},
		{"word with underscore", Position{Line: 1, Character: 9}, "my_tool"},
		{"at colon", Position{Line: 0, Character: 4}, "kind"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			word := doc.GetWordAtPosition(tc.pos)
			if word != tc.word {
				t.Errorf("GetWordAtPosition(%v) = %q, want %q", tc.pos, word, tc.word)
			}
		})
	}
}

func TestDocument_GetWordAtPosition_EdgeCases(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Open("file:///test.yaml", "yaml", 1, "")

	// Empty document
	word := doc.GetWordAtPosition(Position{Line: 0, Character: 0})
	if word != "" {
		t.Errorf("expected empty word for empty document, got %q", word)
	}

	// Out of bounds line
	doc2 := store.Open("file:///test2.yaml", "yaml", 1, "hello")
	word = doc2.GetWordAtPosition(Position{Line: 5, Character: 0})
	if word != "" {
		t.Errorf("expected empty word for out of bounds line, got %q", word)
	}
}

func TestDocument_GetLineContent(t *testing.T) {
	store := NewDocumentStore()
	doc := store.Open("file:///test.yaml", "yaml", 1, "line1\nline2\nline3")

	// Valid line
	line := doc.GetLineContent(1)
	if line != "line2" {
		t.Errorf("GetLineContent(1) = %q, want %q", line, "line2")
	}

	// First line
	line = doc.GetLineContent(0)
	if line != "line1" {
		t.Errorf("GetLineContent(0) = %q, want %q", line, "line1")
	}

	// Last line
	line = doc.GetLineContent(2)
	if line != "line3" {
		t.Errorf("GetLineContent(2) = %q, want %q", line, "line3")
	}

	// Out of bounds
	line = doc.GetLineContent(10)
	if line != "" {
		t.Errorf("GetLineContent(10) = %q, want empty string", line)
	}

	// Negative line
	line = doc.GetLineContent(-1)
	if line != "" {
		t.Errorf("GetLineContent(-1) = %q, want empty string", line)
	}
}

func TestIsWordChar(t *testing.T) {
	tests := []struct {
		b    byte
		want bool
	}{
		{'a', true},
		{'Z', true},
		{'0', true},
		{'_', true},
		{'-', true}, // '-' is considered a word char
		{' ', false},
		{':', false},
		{'.', false},
	}

	for _, tc := range tests {
		t.Run(string(tc.b), func(t *testing.T) {
			if got := isWordChar(tc.b); got != tc.want {
				t.Errorf("isWordChar(%q) = %v, want %v", tc.b, got, tc.want)
			}
		})
	}
}

func TestDocument_ApplyChange_MultipleLinesInsertion(t *testing.T) {
	store := NewDocumentStore()
	store.Open("file:///test.yaml", "yaml", 1, "line1\nline2")

	// Insert multiple lines at the end of line1
	changes := []TextDocumentContentChangeEvent{
		{
			Range: &Range{
				Start: Position{Line: 0, Character: 5},
				End:   Position{Line: 0, Character: 5},
			},
			Text: "\nnewline1\nnewline2",
		},
	}
	doc := store.Update("file:///test.yaml", 2, changes)

	expected := "line1\nnewline1\nnewline2\nline2"
	if doc.Content != expected {
		t.Errorf("expected %q, got %q", expected, doc.Content)
	}
}

func TestDocument_ApplyChange_DeleteAcrossLines(t *testing.T) {
	store := NewDocumentStore()
	store.Open("file:///test.yaml", "yaml", 1, "line1\nline2\nline3")

	// Delete from end of line1 to start of line3 (removes line2 entirely)
	changes := []TextDocumentContentChangeEvent{
		{
			Range: &Range{
				Start: Position{Line: 0, Character: 5},
				End:   Position{Line: 2, Character: 0},
			},
			Text: "",
		},
	}
	doc := store.Update("file:///test.yaml", 2, changes)

	expected := "line1line3"
	if doc.Content != expected {
		t.Errorf("expected %q, got %q", expected, doc.Content)
	}
}
