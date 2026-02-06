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
	"strings"
)

// handleCompletion handles the textDocument/completion request.
func (s *Server) handleCompletion(ctx context.Context, c *Connection, msg *Message) {
	var params CompletionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(c, msg.ID, -32700, "Invalid params", err.Error())
		return
	}

	doc := s.documents.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(c, msg.ID, CompletionList{Items: []CompletionItem{}})
		return
	}

	// Get context for completion
	line := doc.GetLineContent(params.Position.Line)
	prefix := ""
	if params.Position.Character <= len(line) {
		prefix = strings.TrimSpace(line[:params.Position.Character])
	}

	var items []CompletionItem

	// Determine completion context
	switch {
	case prefix == "" || prefix == "-":
		// At start of document or list item - suggest top-level fields
		items = s.getTopLevelCompletions()
	case strings.HasPrefix(prefix, "kind:") || prefix == "kind":
		items = s.getKindCompletions()
	case strings.HasPrefix(prefix, "type:") || prefix == "type":
		items = s.getProviderTypeCompletions()
	case strings.HasSuffix(prefix, "tool:") || strings.HasSuffix(prefix, "- tool:"):
		items = s.getToolRefCompletions(ctx, c.workspace, c.projectID)
	case strings.HasSuffix(prefix, "provider:") || strings.HasSuffix(prefix, "- provider:"):
		items = s.getProviderRefCompletions(ctx, c.workspace, c.projectID)
	case strings.HasSuffix(prefix, "prompt:") || strings.HasSuffix(prefix, "- prompt:"):
		items = s.getPromptRefCompletions(ctx, c.workspace, c.projectID)
	case strings.HasSuffix(prefix, "persona:"):
		items = s.getPersonaRefCompletions(ctx, c.workspace, c.projectID)
	default:
		// Try to provide field completions based on current context
		items = s.getFieldCompletions(doc, params.Position)
	}

	s.sendResponse(c, msg.ID, CompletionList{
		IsIncomplete: false,
		Items:        items,
	})
}

// getTopLevelCompletions returns completions for top-level fields.
func (s *Server) getTopLevelCompletions() []CompletionItem {
	return []CompletionItem{
		{
			Label:         "kind",
			Kind:          CompletionItemKindProperty,
			Detail:        "Resource type",
			Documentation: "The type of PromptKit resource (Tool, Provider, Prompt, Scenario, Arena, Persona)",
			InsertText:    "kind: ",
		},
		{
			Label:         "name",
			Kind:          CompletionItemKindProperty,
			Detail:        "Resource name",
			Documentation: "A unique identifier for this resource",
			InsertText:    "name: ",
		},
		{
			Label:         "description",
			Kind:          CompletionItemKindProperty,
			Detail:        "Description",
			Documentation: "A human-readable description of the resource",
			InsertText:    "description: ",
		},
		{
			Label:         "version",
			Kind:          CompletionItemKindProperty,
			Detail:        "Version",
			Documentation: "The version of this resource (e.g., v1.0.0)",
			InsertText:    "version: ",
		},
		{
			Label:         "spec",
			Kind:          CompletionItemKindProperty,
			Detail:        "Specification",
			Documentation: "The specification of the resource",
			InsertText:    "spec:\n  ",
		},
		{
			Label:         "metadata",
			Kind:          CompletionItemKindProperty,
			Detail:        "Metadata",
			Documentation: "Additional metadata (labels, annotations)",
			InsertText:    "metadata:\n  labels: {}\n  annotations: {}",
		},
		{
			Label:         "type",
			Kind:          CompletionItemKindProperty,
			Detail:        "Provider type",
			Documentation: "The type of provider (openai, anthropic, etc.)",
			InsertText:    "type: ",
		},
		{
			Label:         "model",
			Kind:          CompletionItemKindProperty,
			Detail:        "Model name",
			Documentation: "The model to use",
			InsertText:    "model: ",
		},
		{
			Label:         "tools",
			Kind:          CompletionItemKindProperty,
			Detail:        "Tools list",
			Documentation: "List of tools to use",
			InsertText:    "tools:\n  - ",
		},
		{
			Label:         "prompts",
			Kind:          CompletionItemKindProperty,
			Detail:        "Prompts list",
			Documentation: "List of prompts",
			InsertText:    "prompts:\n  - ",
		},
		{
			Label:         "providers",
			Kind:          CompletionItemKindProperty,
			Detail:        "Providers list",
			Documentation: "List of providers",
			InsertText:    "providers:\n  - ",
		},
		{
			Label:         "scenarios",
			Kind:          CompletionItemKindProperty,
			Detail:        "Scenarios list",
			Documentation: "List of scenarios",
			InsertText:    "scenarios:\n  - ",
		},
		{
			Label:         "steps",
			Kind:          CompletionItemKindProperty,
			Detail:        "Scenario steps",
			Documentation: "List of steps in a scenario",
			InsertText:    "steps:\n  - name: \n    prompt: ",
		},
		{
			Label:         "system",
			Kind:          CompletionItemKindProperty,
			Detail:        "System prompt",
			Documentation: "System prompt for the model",
			InsertText:    "system: |\n  ",
		},
		{
			Label:         "parameters",
			Kind:          CompletionItemKindProperty,
			Detail:        "Model parameters",
			Documentation: "Model parameters (temperature, max_tokens, etc.)",
			InsertText:    "parameters:\n  temperature: 0.7\n  max_tokens: 1000",
		},
	}
}

// getKindCompletions returns completions for the 'kind' field.
func (s *Server) getKindCompletions() []CompletionItem {
	kinds := []struct {
		name        string
		description string
	}{
		{"Tool", "A tool that can be called by the model"},
		{"Provider", "An LLM provider configuration"},
		{"Prompt", "A prompt template"},
		{"Scenario", "A test scenario with steps"},
		{"Arena", "An Arena configuration for batch evaluation"},
		{"Persona", "A persona configuration for the model"},
	}

	items := make([]CompletionItem, len(kinds))
	for i, k := range kinds {
		items[i] = CompletionItem{
			Label:         k.name,
			Kind:          CompletionItemKindEnumMember,
			Detail:        "Kind",
			Documentation: k.description,
		}
	}
	return items
}

// getProviderTypeCompletions returns completions for provider types.
func (s *Server) getProviderTypeCompletions() []CompletionItem {
	types := []struct {
		name        string
		description string
	}{
		{"openai", "OpenAI API (GPT-3.5, GPT-4, etc.)"},
		{"anthropic", "Anthropic API (Claude)"},
		{"azure", "Azure OpenAI Service"},
		{"bedrock", "AWS Bedrock"},
		{"vertex", "Google Vertex AI"},
		{"ollama", "Ollama local models"},
		{"custom", "Custom HTTP endpoint"},
	}

	items := make([]CompletionItem, len(types))
	for i, t := range types {
		items[i] = CompletionItem{
			Label:         t.name,
			Kind:          CompletionItemKindEnumMember,
			Detail:        "Provider type",
			Documentation: t.description,
		}
	}
	return items
}

// getToolRefCompletions returns completions for tool references.
func (s *Server) getToolRefCompletions(ctx context.Context, workspace, projectID string) []CompletionItem {
	return s.getRefCompletions(ctx, workspace, projectID, "tools", "Tool")
}

// getProviderRefCompletions returns completions for provider references.
func (s *Server) getProviderRefCompletions(ctx context.Context, workspace, projectID string) []CompletionItem {
	return s.getRefCompletions(ctx, workspace, projectID, "providers", "Provider")
}

// getPromptRefCompletions returns completions for prompt references.
func (s *Server) getPromptRefCompletions(ctx context.Context, workspace, projectID string) []CompletionItem {
	return s.getRefCompletions(ctx, workspace, projectID, "prompts", "Prompt")
}

// getPersonaRefCompletions returns completions for persona references.
func (s *Server) getPersonaRefCompletions(ctx context.Context, workspace, projectID string) []CompletionItem {
	return s.getRefCompletions(ctx, workspace, projectID, "personas", "Persona")
}

// getRefCompletions returns completions for resource references.
func (s *Server) getRefCompletions(ctx context.Context, workspace, projectID, dir, kind string) []CompletionItem {
	files, err := s.validator.getProjectFiles(ctx, workspace, projectID)
	if err != nil {
		s.log.V(1).Info("failed to get project files for completion", "error", err.Error())
		return nil
	}

	var items []CompletionItem
	for _, file := range files {
		parts := strings.Split(file, "/")
		if len(parts) >= 2 && parts[len(parts)-2] == dir {
			name := strings.TrimSuffix(parts[len(parts)-1], ".yaml")
			name = strings.TrimSuffix(name, ".yml")
			items = append(items, CompletionItem{
				Label:  name,
				Kind:   CompletionItemKindReference,
				Detail: kind + " reference",
			})
		}
	}
	return items
}

// getFieldCompletions returns field completions based on context.
func (s *Server) getFieldCompletions(_ *Document, _ Position) []CompletionItem {
	// Analyze the document to determine context
	// For now, return a subset of common fields
	return []CompletionItem{
		{
			Label:      "name",
			Kind:       CompletionItemKindProperty,
			InsertText: "name: ",
		},
		{
			Label:      "description",
			Kind:       CompletionItemKindProperty,
			InsertText: "description: ",
		},
		{
			Label:      "config",
			Kind:       CompletionItemKindProperty,
			InsertText: "config:\n  ",
		},
	}
}
