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
)

// handleInitialize handles the initialize request.
func (s *Server) handleInitialize(_ context.Context, c *Connection, msg *Message) {
	var params InitializeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(c, msg.ID, -32700, "Invalid params", err.Error())
		return
	}

	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    SyncFull, // Full document sync for simplicity
				Save: &SaveOptions{
					IncludeText: true,
				},
			},
			CompletionProvider: &CompletionOptions{
				TriggerCharacters: []string{":", " ", "\n", "-"},
				ResolveProvider:   false,
			},
			HoverProvider:      true,
			DefinitionProvider: true,
		},
		ServerInfo: &ServerInfo{
			Name:    "promptkit-lsp",
			Version: "1.0.0",
		},
	}

	s.sendResponse(c, msg.ID, result)
}

// handleShutdown handles the shutdown request.
func (s *Server) handleShutdown(_ context.Context, c *Connection, msg *Message) {
	s.sendResponse(c, msg.ID, nil)
}

// handleDidOpen handles the textDocument/didOpen notification.
func (s *Server) handleDidOpen(ctx context.Context, c *Connection, msg *Message) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.log.Error(err, "failed to parse didOpen params")
		return
	}

	// Store the document
	doc := s.documents.Open(
		params.TextDocument.URI,
		params.TextDocument.LanguageID,
		params.TextDocument.Version,
		params.TextDocument.Text,
	)

	s.log.V(1).Info("document opened",
		"uri", params.TextDocument.URI,
		"version", params.TextDocument.Version,
	)

	// Validate and publish diagnostics
	s.validateAndPublish(ctx, c, doc)
}

// handleDidChange handles the textDocument/didChange notification.
func (s *Server) handleDidChange(ctx context.Context, c *Connection, msg *Message) {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.log.Error(err, "failed to parse didChange params")
		return
	}

	// Update the document
	doc := s.documents.Update(
		params.TextDocument.URI,
		params.TextDocument.Version,
		params.ContentChanges,
	)

	if doc == nil {
		s.log.Error(nil, "document not found", "uri", params.TextDocument.URI)
		return
	}

	s.log.V(1).Info("document changed",
		"uri", params.TextDocument.URI,
		"version", params.TextDocument.Version,
	)

	// Validate and publish diagnostics
	s.validateAndPublish(ctx, c, doc)
}

// handleDidClose handles the textDocument/didClose notification.
func (s *Server) handleDidClose(_ context.Context, c *Connection, msg *Message) {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.log.Error(err, "failed to parse didClose params")
		return
	}

	// Remove the document
	s.documents.Close(params.TextDocument.URI)

	s.log.V(1).Info("document closed", "uri", params.TextDocument.URI)

	// Clear diagnostics for the closed document
	s.sendNotification(c, "textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []Diagnostic{},
	})
}

// validateAndPublish validates a document and publishes diagnostics.
func (s *Server) validateAndPublish(ctx context.Context, c *Connection, doc *Document) {
	// Run validation
	diagnostics := s.validator.ValidateDocument(ctx, doc, c.workspace, c.projectID)

	// Publish diagnostics
	version := doc.Version
	s.sendNotification(c, "textDocument/publishDiagnostics", PublishDiagnosticsParams{
		URI:         doc.URI,
		Version:     &version,
		Diagnostics: diagnostics,
	})
}
