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
