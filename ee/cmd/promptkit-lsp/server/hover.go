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
	"strings"
)

// fieldDocumentation maps field names to their documentation.
var fieldDocumentation = map[string]struct {
	description string
	example     string
}{
	"kind": {
		description: "The type of PromptKit resource. Valid values: Tool, Provider, Prompt, Scenario, Arena, Persona.",
		example:     "kind: Tool",
	},
	"name": {
		description: "A unique identifier for this resource. Must start with a letter and " +
			"contain only alphanumeric characters, underscores, and hyphens.",
		example: "name: my-tool",
	},
	"description": {
		description: "A human-readable description of the resource. This is displayed in " +
			"the UI and helps users understand what the resource does.",
		example: "description: A tool that searches the web",
	},
	"version": {
		description: "The semantic version of this resource. Follows semver format (e.g., v1.0.0).",
		example:     "version: v1.0.0",
	},
	"type": {
		description: "The type of LLM provider. Valid values: openai, anthropic, azure, bedrock, vertex, ollama, custom.",
		example:     "type: openai",
	},
	"model": {
		description: "The model identifier to use with the provider.",
		example:     "model: gpt-4-turbo",
	},
	"spec": {
		description: "The specification object containing the resource's configuration.",
		example:     "spec:\n  input:\n    type: object",
	},
	"tools": {
		description: "A list of tools available to the model. Can be tool names (strings) " +
			"or objects with tool reference and config.",
		example: "tools:\n  - search\n  - tool: calculator\n    config: {}",
	},
	"prompts": {
		description: "A list of prompts to use. Can be prompt names (strings) or objects " +
			"with prompt reference and variables.",
		example: "prompts:\n  - greeting\n  - prompt: qa\n    variables:\n      topic: science",
	},
	"providers": {
		description: "A list of providers to use for model inference. Supports weighted routing.",
		example:     "providers:\n  - provider: openai\n    weight: 0.8\n  - provider: anthropic\n    weight: 0.2",
	},
	"scenarios": {
		description: "A list of scenarios to run in an Arena evaluation.",
		example:     "scenarios:\n  - basic-qa\n  - math-problems",
	},
	"steps": {
		description: "A list of steps in a scenario. Each step defines a prompt, provider, and optional assertions.",
		example:     "steps:\n  - name: greeting\n    prompt: Hello!\n    assert:\n      - type: contains\n        value: Hi",
	},
	"system": {
		description: "The system prompt that sets the behavior and context for the model.",
		example:     "system: |\n  You are a helpful assistant.",
	},
	"messages": {
		description: "A list of messages forming a conversation. Each message has a role and content.",
		example:     "messages:\n  - role: user\n    content: Hello\n  - role: assistant\n    content: Hi there!",
	},
	"parameters": {
		description: "Model parameters for controlling generation behavior.",
		example:     "parameters:\n  temperature: 0.7\n  max_tokens: 1000\n  top_p: 0.9",
	},
	"temperature": {
		description: "Controls randomness in generation. Higher values (0.8-1.0) make " +
			"output more random, lower values (0.1-0.4) make it more deterministic. Range: 0-2.",
		example: "temperature: 0.7",
	},
	"max_tokens": {
		description: "Maximum number of tokens to generate in the response.",
		example:     "max_tokens: 1000",
	},
	"top_p": {
		description: "Nucleus sampling parameter. The model considers tokens with top_p probability mass. Range: 0-1.",
		example:     "top_p: 0.9",
	},
	"persona": {
		description: "Reference to a persona that defines the model's personality and behavior.",
		example:     "persona: friendly-assistant",
	},
	"traits": {
		description: "A list of personality traits for a persona.",
		example:     "traits:\n  - helpful\n  - concise\n  - friendly",
	},
	"input": {
		description: "The input schema for a tool, defining the expected parameters.",
		example:     "input:\n  type: object\n  properties:\n    query:\n      type: string\n  required:\n    - query",
	},
	"output": {
		description: "The output schema for a tool, defining the expected return value.",
		example:     "output:\n  type: object\n  properties:\n    result:\n      type: string",
	},
	"handler": {
		description: "The handler configuration for a tool, defining how it's executed.",
		example:     "handler:\n  type: http\n  url: https://api.example.com/search\n  method: POST",
	},
	"config": {
		description: "Configuration options for the resource.",
		example:     "config:\n  timeout: 30s\n  retries: 3",
	},
	"metadata": {
		description: "Additional metadata for the resource including labels and annotations.",
		example:     "metadata:\n  labels:\n    env: production\n  annotations:\n    owner: team-a",
	},
	"assert": {
		description: "Assertions to validate the model's response in a scenario step.",
		example:     "assert:\n  - type: contains\n    value: expected text\n  - type: json\n    schema: {}",
	},
}

// handleHover handles the textDocument/hover request.
func (s *Server) handleHover(_ context.Context, c *Connection, msg *Message) {
	var params HoverParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(c, msg.ID, -32700, "Invalid params", err.Error())
		return
	}

	doc := s.documents.Get(params.TextDocument.URI)
	if doc == nil {
		s.sendResponse(c, msg.ID, nil)
		return
	}

	// Get the word at the cursor position
	word := s.getFieldNameAtPosition(doc, params.Position)
	if word == "" {
		s.sendResponse(c, msg.ID, nil)
		return
	}

	// Look up documentation
	docInfo, ok := fieldDocumentation[word]
	if !ok {
		// Try to provide value-specific hover info
		hover := s.getValueHover(doc, params.Position, word)
		if hover != nil {
			s.sendResponse(c, msg.ID, hover)
			return
		}
		s.sendResponse(c, msg.ID, nil)
		return
	}

	// Build markdown content
	content := fmt.Sprintf("### `%s`\n\n%s\n\n**Example:**\n```yaml\n%s\n```",
		word, docInfo.description, docInfo.example)

	s.sendResponse(c, msg.ID, Hover{
		Contents: MarkupContent{
			Kind:  MarkupKindMarkdown,
			Value: content,
		},
		Range: s.getWordRange(doc, params.Position, word),
	})
}

// getFieldNameAtPosition extracts the field name at a position.
func (s *Server) getFieldNameAtPosition(doc *Document, pos Position) string {
	if pos.Line >= len(doc.Lines) {
		return ""
	}

	line := doc.Lines[pos.Line]

	// Check if cursor is on a field name (before the colon)
	colonIdx := strings.Index(line, ":")
	if colonIdx == -1 {
		return ""
	}

	// If cursor is after the colon, we're on a value
	if pos.Character > colonIdx {
		return ""
	}

	// Extract field name (handling indentation)
	fieldPart := strings.TrimSpace(line[:colonIdx])
	fieldPart = strings.TrimPrefix(fieldPart, "- ")
	return fieldPart
}

// getValueHover provides hover info for values.
func (s *Server) getValueHover(doc *Document, pos Position, word string) *Hover {
	if pos.Line >= len(doc.Lines) {
		return nil
	}

	line := doc.Lines[pos.Line]

	// Check if this is a 'kind' value
	if strings.Contains(line, "kind:") && pos.Character > strings.Index(line, ":") {
		return s.getKindValueHover(word)
	}

	// Check if this is a 'type' value (for providers)
	if strings.Contains(line, "type:") && pos.Character > strings.Index(line, ":") {
		return s.getTypeValueHover(word)
	}

	return nil
}

// getKindValueHover provides hover info for kind values.
func (s *Server) getKindValueHover(kind string) *Hover {
	kindDocs := map[string]string{
		"Tool":     "A **Tool** defines a function called by the model to perform actions.",
		"Provider": "A **Provider** configures an LLM backend (OpenAI, Anthropic, etc.).",
		"Prompt":   "A **Prompt** defines a reusable prompt template with variables.",
		"Scenario": "A **Scenario** defines a test case with steps to evaluate model behavior.",
		"Arena":    "An **Arena** configures batch evaluation across providers and scenarios.",
		"Persona":  "A **Persona** defines personality traits and behavior for the model.",
	}

	doc, ok := kindDocs[kind]
	if !ok {
		return nil
	}

	return &Hover{
		Contents: MarkupContent{
			Kind:  MarkupKindMarkdown,
			Value: doc,
		},
	}
}

// getTypeValueHover provides hover info for provider type values.
func (s *Server) getTypeValueHover(providerType string) *Hover {
	typeDocs := map[string]string{
		"openai":    "**OpenAI** - GPT-3.5, GPT-4, and other OpenAI models via the OpenAI API.",
		"anthropic": "**Anthropic** - Claude models via the Anthropic API.",
		"azure":     "**Azure OpenAI** - OpenAI models hosted on Azure.",
		"bedrock":   "**AWS Bedrock** - Various models via AWS Bedrock service.",
		"vertex":    "**Google Vertex AI** - Gemini and other models via Google Cloud.",
		"ollama":    "**Ollama** - Local models running via Ollama.",
		"custom":    "**Custom** - Custom HTTP endpoint for any model API.",
	}

	doc, ok := typeDocs[providerType]
	if !ok {
		return nil
	}

	return &Hover{
		Contents: MarkupContent{
			Kind:  MarkupKindMarkdown,
			Value: doc,
		},
	}
}

// getWordRange returns the range of a word at a position.
func (s *Server) getWordRange(doc *Document, pos Position, word string) *Range {
	if pos.Line >= len(doc.Lines) {
		return nil
	}

	line := doc.Lines[pos.Line]
	idx := strings.Index(line, word)
	if idx == -1 {
		return nil
	}

	return &Range{
		Start: Position{Line: pos.Line, Character: idx},
		End:   Position{Line: pos.Line, Character: idx + len(word)},
	}
}
