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
	"strings"
	"testing"

	"github.com/AltairaLabs/PromptKit/runtime/types"
)

// Compile-time interface check.
var _ MemoryPopulator = (*ConversationPopulator)(nil)

func TestConversationPopulator_SourceType(t *testing.T) {
	p := NewConversationPopulator()
	if got := p.SourceType(); got != "conversation_extraction" {
		t.Errorf("SourceType() = %q, want %q", got, "conversation_extraction")
	}
}

func TestConversationPopulator_TrustModel(t *testing.T) {
	p := NewConversationPopulator()
	if got := p.TrustModel(); got != "inferred" {
		t.Errorf("TrustModel() = %q, want %q", got, "inferred")
	}
}

func TestConversationPopulator_Populate(t *testing.T) {
	p := NewConversationPopulator()
	ctx := context.Background()

	source := PopulationSource{
		Scope: map[string]string{
			"session_id": "sess-123",
		},
		Messages: []types.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "Go is a programming language."},
		},
	}

	result, err := p.Populate(ctx, source)
	if err != nil {
		t.Fatalf("Populate() error = %v", err)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	entity := result.Entities[0]
	if entity.Name != "What is Go?" {
		t.Errorf("entity Name = %q, want %q", entity.Name, "What is Go?")
	}
	if entity.Kind != "episodic" {
		t.Errorf("entity Kind = %q, want %q", entity.Kind, "episodic")
	}

	if len(result.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(result.Observations))
	}

	obs := result.Observations[0]
	if obs.EntityName != "What is Go?" {
		t.Errorf("observation EntityName = %q, want %q", obs.EntityName, "What is Go?")
	}
	if !strings.Contains(obs.Content, "User asked:") {
		t.Errorf("observation Content should contain 'User asked:', got %q", obs.Content)
	}
	if !strings.Contains(obs.Content, "Assistant responded:") {
		t.Errorf("observation Content should contain 'Assistant responded:', got %q", obs.Content)
	}
	if obs.SessionID != "sess-123" {
		t.Errorf("observation SessionID = %q, want %q", obs.SessionID, "sess-123")
	}
	if obs.Confidence != 0.7 {
		t.Errorf("observation Confidence = %v, want 0.7", obs.Confidence)
	}
}

func TestConversationPopulator_EmptyMessages(t *testing.T) {
	p := NewConversationPopulator()
	ctx := context.Background()

	source := PopulationSource{
		Messages: []types.Message{},
	}

	result, err := p.Populate(ctx, source)
	if err != nil {
		t.Fatalf("Populate() error = %v", err)
	}

	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities, got %d", len(result.Entities))
	}
	if len(result.Observations) != 0 {
		t.Errorf("expected 0 observations, got %d", len(result.Observations))
	}
}

func TestConversationPopulator_NoUserMessage(t *testing.T) {
	p := NewConversationPopulator()
	ctx := context.Background()

	source := PopulationSource{
		Messages: []types.Message{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "assistant", Content: "Hello! How can I help?"},
		},
	}

	result, err := p.Populate(ctx, source)
	if err != nil {
		t.Fatalf("Populate() error = %v", err)
	}

	if len(result.Entities) != 0 {
		t.Errorf("expected 0 entities for no-user-message, got %d", len(result.Entities))
	}
	if len(result.Observations) != 0 {
		t.Errorf("expected 0 observations for no-user-message, got %d", len(result.Observations))
	}
}

func TestConversationPopulator_LongContent(t *testing.T) {
	p := NewConversationPopulator()
	ctx := context.Background()

	longContent := strings.Repeat("a", 300)

	source := PopulationSource{
		Messages: []types.Message{
			{Role: "user", Content: longContent},
		},
	}

	result, err := p.Populate(ctx, source)
	if err != nil {
		t.Fatalf("Populate() error = %v", err)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	entity := result.Entities[0]
	if len(entity.Name) > maxContentLength {
		t.Errorf("entity Name length = %d, want <= %d", len(entity.Name), maxContentLength)
	}
	if !strings.HasSuffix(entity.Name, "...") {
		t.Errorf("truncated entity Name should end with '...', got %q", entity.Name[len(entity.Name)-5:])
	}

	obs := result.Observations[0]
	if len(obs.Content) > maxContentLength {
		t.Errorf("observation Content length = %d, want <= %d", len(obs.Content), maxContentLength)
	}
	if !strings.HasSuffix(obs.Content, "...") {
		t.Errorf("truncated observation Content should end with '...', got %q", obs.Content[len(obs.Content)-5:])
	}
}

func TestConversationPopulator_UserOnlyNoAssistant(t *testing.T) {
	p := NewConversationPopulator()
	ctx := context.Background()

	source := PopulationSource{
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	result, err := p.Populate(ctx, source)
	if err != nil {
		t.Fatalf("Populate() error = %v", err)
	}

	if len(result.Entities) != 1 {
		t.Fatalf("expected 1 entity, got %d", len(result.Entities))
	}

	obs := result.Observations[0]
	if !strings.Contains(obs.Content, "User asked:") {
		t.Errorf("observation Content should contain 'User asked:', got %q", obs.Content)
	}
	if strings.Contains(obs.Content, "Assistant responded:") {
		t.Errorf("observation Content should not contain 'Assistant responded:' when no assistant message, got %q", obs.Content)
	}
}

func TestConversationPopulator_MultipleUserMessages(t *testing.T) {
	p := NewConversationPopulator()
	ctx := context.Background()

	source := PopulationSource{
		Messages: []types.Message{
			{Role: "user", Content: "First question"},
			{Role: "assistant", Content: "First answer"},
			{Role: "user", Content: "Second question"},
			{Role: "assistant", Content: "Second answer"},
		},
	}

	result, err := p.Populate(ctx, source)
	if err != nil {
		t.Fatalf("Populate() error = %v", err)
	}

	// Should use the last user message
	entity := result.Entities[0]
	if entity.Name != "Second question" {
		t.Errorf("entity Name = %q, want %q (last user message)", entity.Name, "Second question")
	}
}

func TestConversationPopulator_NilScope(t *testing.T) {
	p := NewConversationPopulator()
	ctx := context.Background()

	source := PopulationSource{
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
	}

	result, err := p.Populate(ctx, source)
	if err != nil {
		t.Fatalf("Populate() error = %v", err)
	}

	if result.Observations[0].SessionID != "" {
		t.Errorf("SessionID should be empty when scope has no session_id, got %q", result.Observations[0].SessionID)
	}
}
