/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/go-logr/logr"
)

const extractionSystemPrompt = `You are a memory extraction system. Analyze the conversation and extract structured memories.

Return JSON with this schema:
{
  "entities": [{"name": "...", "kind": "preference|topic|fact|need", "metadata": {}}],
  "observations": [{"entity_name": "...", "content": "...", "confidence": 0.0-1.0}],
  "relations": [{"source": "...", "target": "...", "type": "prefers|relates_to|struggled_with"}]
}

Rules:
- Extract user preferences, topics discussed, facts stated, and needs expressed
- Confidence reflects how explicitly the information was stated (explicit=0.9, inferred=0.6)
- Only extract information actually present in the conversation
- Keep entity names short and descriptive
- Return empty arrays if nothing worth remembering`

// LLMProvider generates text completions for memory extraction.
type LLMProvider interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// llmExtractionResult is the JSON schema returned by the LLM.
type llmExtractionResult struct {
	Entities     []llmEntity      `json:"entities"`
	Observations []llmObservation `json:"observations"`
	Relations    []llmRelation    `json:"relations"`
}

type llmEntity struct {
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Metadata map[string]any `json:"metadata"`
}

type llmObservation struct {
	EntityName string  `json:"entity_name"`
	Content    string  `json:"content"`
	Confidence float64 `json:"confidence"`
}

type llmRelation struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// LLMConversationPopulator extracts structured memories from conversations using an LLM.
// Falls back to rule-based extraction (ConversationPopulator) if the LLM fails.
type LLMConversationPopulator struct {
	llm      LLMProvider
	fallback *ConversationPopulator
	log      logr.Logger
}

// NewLLMConversationPopulator creates a new LLMConversationPopulator.
func NewLLMConversationPopulator(llm LLMProvider, log logr.Logger) *LLMConversationPopulator {
	return &LLMConversationPopulator{
		llm:      llm,
		fallback: NewConversationPopulator(),
		log:      log,
	}
}

// SourceType returns the populator source type identifier.
func (p *LLMConversationPopulator) SourceType() string { return conversationExtractionSource }

// TrustModel returns the trust model for this populator.
func (p *LLMConversationPopulator) TrustModel() string { return inferredTrust }

// Populate extracts structured memories from conversation messages using an LLM.
func (p *LLMConversationPopulator) Populate(ctx context.Context, source PopulationSource) (*PopulationResult, error) {
	if len(source.Messages) == 0 {
		return &PopulationResult{}, nil
	}

	userPrompt := formatConversation(source.Messages)

	response, err := p.llm.Complete(ctx, extractionSystemPrompt, userPrompt)
	if err != nil {
		p.log.V(1).Info("llm extraction failed, using fallback", "error", err)
		return p.fallback.Populate(ctx, source)
	}

	extracted, err := parseExtractionResponse(response)
	if err != nil {
		p.log.V(1).Info("llm response parse failed, using fallback", "error", err)
		return p.fallback.Populate(ctx, source)
	}

	return convertToResult(extracted, source.Scope), nil
}

// formatConversation formats messages into a prompt string.
func formatConversation(messages []types.Message) string {
	var b strings.Builder
	b.WriteString("Conversation:\n\n")
	for _, m := range messages {
		role := capitalizeFirst(m.Role)
		fmt.Fprintf(&b, "%s: %s\n", role, m.Content)
	}
	return b.String()
}

// parseExtractionResponse parses the LLM JSON response.
func parseExtractionResponse(response string) (*llmExtractionResult, error) {
	var result llmExtractionResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("unmarshal extraction response: %w", err)
	}
	return &result, nil
}

// capitalizeFirst uppercases the first rune of s.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// convertToResult converts the LLM extraction result to a PopulationResult.
func convertToResult(extracted *llmExtractionResult, scope map[string]string) *PopulationResult {
	result := &PopulationResult{
		Entities:     make([]EntityRecord, 0, len(extracted.Entities)),
		Observations: make([]ObservationRecord, 0, len(extracted.Observations)),
		Relations:    make([]RelationRecord, 0, len(extracted.Relations)),
	}

	for _, e := range extracted.Entities {
		result.Entities = append(result.Entities, EntityRecord(e))
	}

	sessionID := ""
	if scope != nil {
		sessionID = scope["session_id"]
	}

	for _, o := range extracted.Observations {
		result.Observations = append(result.Observations, ObservationRecord{
			EntityName: o.EntityName,
			Content:    o.Content,
			Confidence: float32(o.Confidence),
			SessionID:  sessionID,
		})
	}

	for _, r := range extracted.Relations {
		result.Relations = append(result.Relations, RelationRecord{
			SourceName:   r.Source,
			TargetName:   r.Target,
			RelationType: r.Type,
			Weight:       1.0,
		})
	}

	return result
}
