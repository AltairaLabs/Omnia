/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"strings"
	"sync"
)

// Document represents an open text document.
type Document struct {
	URI        string
	LanguageID string
	Version    int
	Content    string
	Lines      []string
}

// DocumentStore manages open documents.
type DocumentStore struct {
	mu        sync.RWMutex
	documents map[string]*Document
}

// NewDocumentStore creates a new DocumentStore.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{
		documents: make(map[string]*Document),
	}
}

// Open adds or updates a document in the store.
func (ds *DocumentStore) Open(uri, languageID string, version int, content string) *Document {
	doc := &Document{
		URI:        uri,
		LanguageID: languageID,
		Version:    version,
		Content:    content,
		Lines:      splitLines(content),
	}

	ds.mu.Lock()
	ds.documents[uri] = doc
	ds.mu.Unlock()

	return doc
}

// Update updates an existing document with incremental changes.
func (ds *DocumentStore) Update(uri string, version int, changes []TextDocumentContentChangeEvent) *Document {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	doc, ok := ds.documents[uri]
	if !ok {
		return nil
	}

	doc.Version = version

	for _, change := range changes {
		if change.Range == nil {
			// Full document replacement
			doc.Content = change.Text
			doc.Lines = splitLines(change.Text)
		} else {
			// Incremental change
			doc.Content = applyChange(doc.Content, doc.Lines, change)
			doc.Lines = splitLines(doc.Content)
		}
	}

	return doc
}

// Get retrieves a document from the store.
func (ds *DocumentStore) Get(uri string) *Document {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.documents[uri]
}

// Close removes a document from the store.
func (ds *DocumentStore) Close(uri string) {
	ds.mu.Lock()
	delete(ds.documents, uri)
	ds.mu.Unlock()
}

// splitLines splits content into lines, preserving line endings.
func splitLines(content string) []string {
	if content == "" {
		return []string{""}
	}
	return strings.Split(content, "\n")
}

// applyChange applies an incremental change to the document content.
func applyChange(content string, lines []string, change TextDocumentContentChangeEvent) string {
	if change.Range == nil {
		return change.Text
	}

	startLine := change.Range.Start.Line
	startChar := change.Range.Start.Character
	endLine := change.Range.End.Line
	endChar := change.Range.End.Character

	// Calculate byte offsets
	startOffset := 0
	for i := 0; i < startLine && i < len(lines); i++ {
		startOffset += len(lines[i]) + 1 // +1 for newline
	}
	if startLine < len(lines) {
		startOffset += min(startChar, len(lines[startLine]))
	}

	endOffset := 0
	for i := 0; i < endLine && i < len(lines); i++ {
		endOffset += len(lines[i]) + 1
	}
	if endLine < len(lines) {
		endOffset += min(endChar, len(lines[endLine]))
	}

	// Ensure offsets are within bounds
	if startOffset > len(content) {
		startOffset = len(content)
	}
	if endOffset > len(content) {
		endOffset = len(content)
	}
	if startOffset > endOffset {
		startOffset = endOffset
	}

	// Apply the change
	return content[:startOffset] + change.Text + content[endOffset:]
}

// PositionToOffset converts a Position to a byte offset.
func (doc *Document) PositionToOffset(pos Position) int {
	offset := 0
	for i := 0; i < pos.Line && i < len(doc.Lines); i++ {
		offset += len(doc.Lines[i]) + 1 // +1 for newline
	}
	if pos.Line < len(doc.Lines) {
		offset += min(pos.Character, len(doc.Lines[pos.Line]))
	}
	return offset
}

// OffsetToPosition converts a byte offset to a Position.
func (doc *Document) OffsetToPosition(offset int) Position {
	if offset <= 0 {
		return Position{Line: 0, Character: 0}
	}

	currentOffset := 0
	for i, line := range doc.Lines {
		lineLen := len(line) + 1 // +1 for newline
		if currentOffset+lineLen > offset {
			return Position{
				Line:      i,
				Character: offset - currentOffset,
			}
		}
		currentOffset += lineLen
	}

	// Past end of document
	lastLine := len(doc.Lines) - 1
	if lastLine < 0 {
		lastLine = 0
	}
	return Position{
		Line:      lastLine,
		Character: len(doc.Lines[lastLine]),
	}
}

// GetWordAtPosition returns the word at the given position.
func (doc *Document) GetWordAtPosition(pos Position) string {
	if pos.Line >= len(doc.Lines) {
		return ""
	}

	line := doc.Lines[pos.Line]
	if pos.Character >= len(line) {
		return ""
	}

	// Find word boundaries
	start := pos.Character
	end := pos.Character

	// Move start to beginning of word
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}

	// Move end to end of word
	for end < len(line) && isWordChar(line[end]) {
		end++
	}

	return line[start:end]
}

// GetLineContent returns the content of a specific line.
func (doc *Document) GetLineContent(line int) string {
	if line < 0 || line >= len(doc.Lines) {
		return ""
	}
	return doc.Lines[line]
}

// isWordChar returns true if c is a word character.
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_' || c == '-'
}
