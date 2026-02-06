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
	"regexp"
	"sort"
)

// SemanticTokenTypes defines the token types supported by the server.
// These are indices into the token legend that the client uses for highlighting.
var SemanticTokenTypes = []string{
	"variable",  // 0 - Template variables like {{name}}
	"parameter", // 1 - Variable names inside braces
	"string",    // 2 - String values
	"keyword",   // 3 - YAML keywords (kind, apiVersion, etc.)
	"property",  // 4 - YAML property names
}

// SemanticTokenModifiers defines the token modifiers supported by the server.
var SemanticTokenModifiers = []string{
	"declaration",    // 0 - Where the variable is declared (in spec.variables)
	"definition",     // 1 - Where the variable is defined
	"readonly",       // 2 - Read-only reference
	"defaultLibrary", // 3 - Built-in/default variables
}

// SemanticTokensLegend describes the tokens the server can return.
type SemanticTokensLegend struct {
	TokenTypes     []string `json:"tokenTypes"`
	TokenModifiers []string `json:"tokenModifiers"`
}

// SemanticTokensOptions describes semantic tokens provider options.
type SemanticTokensOptions struct {
	Legend SemanticTokensLegend `json:"legend"`
	Full   bool                 `json:"full"`
	Range  bool                 `json:"range,omitempty"`
}

// SemanticTokensParams is the params for textDocument/semanticTokens/full.
type SemanticTokensParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// SemanticTokens is the result of textDocument/semanticTokens/full.
type SemanticTokens struct {
	ResultID string  `json:"resultId,omitempty"`
	Data     []int32 `json:"data"`
}

// tokenLocation represents a token found in the document.
type tokenLocation struct {
	line      int    // 0-based line number
	startChar int    // 0-based character offset in line
	length    int    // length of the token
	tokenType int    // index into SemanticTokenTypes
	modifiers int    // bitmask of SemanticTokenModifiers
	text      string // the actual text (for debugging)
}

// variablePattern matches {{variable_name}} patterns in templates.
// Supports optional whitespace inside braces: {{ name }} or {{name}}
var variablePattern = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)

// GetSemanticTokensLegend returns the semantic tokens legend.
func GetSemanticTokensLegend() SemanticTokensLegend {
	return SemanticTokensLegend{
		TokenTypes:     SemanticTokenTypes,
		TokenModifiers: SemanticTokenModifiers,
	}
}

// GetSemanticTokensOptions returns the semantic tokens provider options.
func GetSemanticTokensOptions() *SemanticTokensOptions {
	return &SemanticTokensOptions{
		Legend: GetSemanticTokensLegend(),
		Full:   true,
		Range:  false,
	}
}

// handleSemanticTokensFull handles the textDocument/semanticTokens/full request.
func (s *Server) handleSemanticTokensFull(_ context.Context, c *Connection, msg *Message) {
	var params SemanticTokensParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(c, msg.ID, -32700, "Invalid params", err.Error())
		return
	}

	doc := s.documents.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendError(c, msg.ID, -32602, "Document not found", params.TextDocument.URI)
		return
	}

	tokens := s.extractSemanticTokens(doc)
	data := encodeSemanticTokens(tokens)

	result := SemanticTokens{
		Data: data,
	}

	s.sendResponse(c, msg.ID, result)
}

// extractSemanticTokens finds all semantic tokens in a document.
func (s *Server) extractSemanticTokens(doc *Document) []tokenLocation {
	var tokens []tokenLocation

	// Find template variables in all lines
	for lineNum, line := range doc.Lines {
		tokens = append(tokens, findVariablesInLine(lineNum, line)...)
	}

	// Sort tokens by position (line, then character)
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].line != tokens[j].line {
			return tokens[i].line < tokens[j].line
		}
		return tokens[i].startChar < tokens[j].startChar
	})

	return tokens
}

// findVariablesInLine finds all template variables in a single line.
func findVariablesInLine(lineNum int, line string) []tokenLocation {
	matches := variablePattern.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return nil
	}

	// Pre-allocate: each match produces 3 tokens ({{, name, }})
	tokens := make([]tokenLocation, 0, len(matches)*3)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}

		// match[0], match[1] = full match start/end (e.g., "{{name}}")
		// match[2], match[3] = captured group start/end (e.g., "name")
		fullStart := match[0]
		fullEnd := match[1]
		varStart := match[2]
		varEnd := match[3]

		// Token for the opening braces "{{"
		tokens = append(tokens, tokenLocation{
			line:      lineNum,
			startChar: fullStart,
			length:    2,
			tokenType: 0, // variable
			modifiers: 0,
			text:      "{{",
		})

		// Token for the variable name itself (highlighted as parameter)
		tokens = append(tokens, tokenLocation{
			line:      lineNum,
			startChar: varStart,
			length:    varEnd - varStart,
			tokenType: 1, // parameter
			modifiers: 0,
			text:      line[varStart:varEnd],
		})

		// Token for the closing braces "}}"
		tokens = append(tokens, tokenLocation{
			line:      lineNum,
			startChar: fullEnd - 2,
			length:    2,
			tokenType: 0, // variable
			modifiers: 0,
			text:      "}}",
		})
	}

	return tokens
}

// encodeSemanticTokens encodes tokens in the LSP delta format.
// Each token is encoded as 5 integers:
// [deltaLine, deltaStartChar, length, tokenType, tokenModifiers]
func encodeSemanticTokens(tokens []tokenLocation) []int32 {
	if len(tokens) == 0 {
		return []int32{}
	}

	result := make([]int32, 0, len(tokens)*5)

	prevLine := 0
	prevChar := 0

	for _, tok := range tokens {
		deltaLine := tok.line - prevLine
		deltaStartChar := tok.startChar
		if deltaLine == 0 {
			deltaStartChar = tok.startChar - prevChar
		}

		result = append(result,
			int32(deltaLine),
			int32(deltaStartChar),
			int32(tok.length),
			int32(tok.tokenType),
			int32(tok.modifiers),
		)

		prevLine = tok.line
		prevChar = tok.startChar
	}

	return result
}
