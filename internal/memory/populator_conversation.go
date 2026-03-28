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

import "context"

const (
	maxContentLength             = 200
	conversationExtractionSource = "conversation_extraction"
	inferredTrust                = "inferred"
	episodicEntityKind           = "episodic"
	defaultEpisodicConfidence    = float32(0.7)
	roleUser                     = "user"
	roleAssistant                = "assistant"
)

// ConversationPopulator extracts entities and observations from conversation messages.
// V1 is rule-based: scans for user preferences, topics discussed, and episodic summaries.
// Future versions will use LLM-based extraction.
type ConversationPopulator struct{}

// NewConversationPopulator creates a new ConversationPopulator.
func NewConversationPopulator() *ConversationPopulator {
	return &ConversationPopulator{}
}

// SourceType returns the populator source type identifier.
func (p *ConversationPopulator) SourceType() string { return conversationExtractionSource }

// TrustModel returns the trust model for this populator.
func (p *ConversationPopulator) TrustModel() string { return inferredTrust }

// Populate scans conversation messages and produces episodic entities and observations.
func (p *ConversationPopulator) Populate(_ context.Context, source PopulationSource) (*PopulationResult, error) {
	lastUser := findLastMessageByRole(source.Messages, roleUser)
	if lastUser == "" {
		return &PopulationResult{}, nil
	}

	lastAssistant := findLastMessageByRole(source.Messages, roleAssistant)
	summary := buildSummary(lastUser, lastAssistant)

	sessionID := source.Scope["session_id"]

	return &PopulationResult{
		Entities: []EntityRecord{
			{
				Name: truncate(lastUser, maxContentLength),
				Kind: episodicEntityKind,
			},
		},
		Observations: []ObservationRecord{
			{
				EntityName: truncate(lastUser, maxContentLength),
				Content:    truncate(summary, maxContentLength),
				Confidence: defaultEpisodicConfidence,
				SessionID:  sessionID,
			},
		},
	}, nil
}

// findLastMessageByRole returns the content of the last message with the given role,
// or an empty string if none is found.
func findLastMessageByRole(messages []SimpleMessage, role string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == role {
			return messages[i].Content
		}
	}
	return ""
}

// buildSummary creates a summary string from the last user and assistant messages.
func buildSummary(userContent, assistantContent string) string {
	if assistantContent == "" {
		return "User asked: " + userContent
	}
	return "User asked: " + userContent + " | Assistant responded: " + assistantContent
}

// truncate shortens s to maxLen runes, appending "..." if truncated.
// Operates on runes to avoid splitting multi-byte UTF-8 characters.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
