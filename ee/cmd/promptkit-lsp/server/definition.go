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
	"fmt"
	"regexp"
	"strings"
)

// Reference types for go-to-definition.
var referencePatterns = map[string]*regexp.Regexp{
	"tool":     regexp.MustCompile(`(?:^|\s)tool:\s*["']?([a-zA-Z0-9_-]+)["']?`),
	"provider": regexp.MustCompile(`(?:^|\s)provider:\s*["']?([a-zA-Z0-9_-]+)["']?`),
	"prompt":   regexp.MustCompile(`(?:^|\s)prompt:\s*["']?([a-zA-Z0-9_-]+)["']?`),
	"persona":  regexp.MustCompile(`(?:^|\s)persona:\s*["']?([a-zA-Z0-9_-]+)["']?`),
	"scenario": regexp.MustCompile(`(?:^|\s)scenario:\s*["']?([a-zA-Z0-9_-]+)["']?`),
}

// directoryMap maps reference types to their directory names.
var directoryMap = map[string]string{
	"tool":     "tools",
	"provider": "providers",
	"prompt":   "prompts",
	"persona":  "personas",
	"scenario": "scenarios",
}

// handleDefinition handles the textDocument/definition request.
func (s *Server) handleDefinition(ctx context.Context, c *Connection, msg *Message) {
	var params DefinitionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(c, msg.ID, -32700, "Invalid params", err.Error())
		return
	}

	doc := s.documents.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(c, msg.ID, nil)
		return
	}

	// Find reference at position
	refType, refName := s.findReferenceAtPosition(doc, params.Position)
	if refType == "" || refName == "" {
		s.sendResponse(c, msg.ID, nil)
		return
	}

	s.log.V(1).Info("looking for definition",
		"type", refType,
		"name", refName,
		"workspace", c.workspace,
		"project", c.projectID,
	)

	// Find the definition file
	location := s.findDefinitionLocation(ctx, c.workspace, c.projectID, refType, refName)
	if location == nil {
		s.sendResponse(c, msg.ID, nil)
		return
	}

	s.sendResponse(c, msg.ID, location)
}

// findReferenceAtPosition finds a reference at the given position.
func (s *Server) findReferenceAtPosition(doc *Document, pos Position) (string, string) {
	if pos.Line >= len(doc.Lines) {
		return "", ""
	}

	line := doc.Lines[pos.Line]

	// Check each reference pattern
	for refType, pattern := range referencePatterns {
		matches := pattern.FindStringSubmatchIndex(line)
		if len(matches) >= 4 {
			// matches[2] and matches[3] are the start and end of the first capture group
			refStart := matches[2]
			refEnd := matches[3]

			// Check if cursor is on the reference value
			if pos.Character >= refStart && pos.Character <= refEnd {
				refName := line[refStart:refEnd]
				return refType, refName
			}
		}
	}

	// Also check for list items like "- toolname" under "tools:"
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "- ") {
		value := strings.TrimPrefix(trimmed, "- ")
		value = strings.Trim(value, `"'`)

		// Look at the parent line to determine the context
		refType := s.findParentListType(doc, pos.Line)
		if refType != "" {
			startIdx := strings.Index(line, value)
			endIdx := startIdx + len(value)
			if pos.Character >= startIdx && pos.Character <= endIdx {
				return refType, value
			}
		}
	}

	return "", ""
}

// findParentListType finds the parent list type for a list item.
func (s *Server) findParentListType(doc *Document, lineNum int) string {
	// Walk backwards to find the parent list key
	currentIndent := s.getIndentation(doc.Lines[lineNum])

	for i := lineNum - 1; i >= 0; i-- {
		line := doc.Lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lineIndent := s.getIndentation(line)
		if lineIndent < currentIndent {
			// Found a less-indented line, check if it's a list key
			if strings.HasSuffix(trimmed, ":") {
				key := strings.TrimSuffix(trimmed, ":")
				switch key {
				case "tools":
					return "tool"
				case "providers":
					return "provider"
				case "prompts":
					return "prompt"
				case "personas":
					return "persona"
				case "scenarios":
					return "scenario"
				}
			}
			// If we found a less-indented line that's not a list key, stop searching
			break
		}
	}

	return ""
}

// getIndentation returns the indentation level of a line.
func (s *Server) getIndentation(line string) int {
	indent := 0
	for _, ch := range line {
		switch ch {
		case ' ':
			indent++
		case '\t':
			indent += 2 // Count tab as 2 spaces
		default:
			return indent
		}
	}
	return indent
}

// findDefinitionLocation finds the location of a definition.
func (s *Server) findDefinitionLocation(ctx context.Context, workspace, projectID, refType, refName string) *Location {
	// Get project files
	files, err := s.validator.getProjectFiles(ctx, workspace, projectID)
	if err != nil {
		s.log.V(1).Info("failed to get project files", "error", err.Error())
		return nil
	}

	// Find the matching file
	dir := directoryMap[refType]
	if dir == "" {
		return nil
	}

	for _, file := range files {
		parts := strings.Split(file, "/")
		if len(parts) >= 2 {
			fileDir := parts[len(parts)-2]
			fileName := parts[len(parts)-1]
			baseName := strings.TrimSuffix(fileName, ".yaml")
			baseName = strings.TrimSuffix(baseName, ".yml")

			if fileDir == dir && baseName == refName {
				// Build the URI for the target file
				uri := s.buildFileURI(workspace, projectID, file)
				return &Location{
					URI: uri,
					Range: Range{
						Start: Position{Line: 0, Character: 0},
						End:   Position{Line: 0, Character: 0},
					},
				}
			}
		}
	}

	return nil
}

// buildFileURI builds a file URI for a project file.
func (s *Server) buildFileURI(workspace, projectID, path string) string {
	// The URI format follows the LSP convention
	// We use a custom scheme that the client will recognize
	return fmt.Sprintf("promptkit://%s/%s/%s", workspace, projectID, path)
}
