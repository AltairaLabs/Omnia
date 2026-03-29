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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// embeddingDim matches the vector(1536) column declared in migration 000025.
const embeddingDim = 1536

// --- Mock types ---

// testEmbeddingProvider generates deterministic embeddings for registered texts,
// and falls back to a character-hash embedding for unknown inputs.
type testEmbeddingProvider struct {
	embeddings map[string][]float32 // text → embedding lookup
	defaultDim int
}

func (p *testEmbeddingProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i, text := range texts {
		if emb, ok := p.embeddings[text]; ok {
			result[i] = emb
		} else {
			// Deterministic fallback: derive from rune values.
			result[i] = make([]float32, p.defaultDim)
			for j, ch := range text {
				if j >= p.defaultDim {
					break
				}
				result[i][j] = float32(ch) / 1000.0
			}
		}
	}
	return result, nil
}

func (p *testEmbeddingProvider) Dimensions() int { return p.defaultDim }

// testRedactor replaces email addresses with a placeholder.
type testRedactor struct{}

func (r *testRedactor) RedactText(_ context.Context, text string) (string, error) {
	return strings.ReplaceAll(text, "test@example.com", "[REDACTED]"), nil
}

// testLLMProvider returns a fixed response string to every Complete call.
type testLLMProvider struct {
	response string
	err      error
}

func (p *testLLMProvider) Complete(_ context.Context, _, _ string) (string, error) {
	return p.response, p.err
}

// --- Helpers ---

// unitVector returns a 1536-dim float32 slice with only the element at idx set to 1.0.
// Two unit vectors at the same index are identical (cosine distance 0).
// Two unit vectors at different indices are orthogonal (cosine distance 1).
func unitVector(idx int) []float32 {
	v := make([]float32, embeddingDim)
	v[idx] = 1.0
	return v
}

// --- Tests ---

// TestEmbeddingEndToEnd exercises: save → EmbedMemory → SemanticStrategy.Retrieve.
func TestEmbeddingEndToEnd(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)
	log := zap.New(zap.UseDevMode(true))

	const memContent = "prefers dark mode in the IDE"
	const queryText = "editor theme preference"

	// Both the stored content and the query get the same unit vector → cosine distance 0.
	provider := &testEmbeddingProvider{
		defaultDim: embeddingDim,
		embeddings: map[string][]float32{
			memContent: unitVector(0),
			queryText:  unitVector(0),
		},
	}

	// Save a memory.
	mem := &Memory{
		Type:       "preference",
		Content:    memContent,
		Confidence: 0.9,
		Scope:      scope,
	}
	require.NoError(t, store.Save(ctx, mem))
	require.NotEmpty(t, mem.ID)

	// Embed it via EmbeddingService.
	embSvc := NewEmbeddingService(store, provider, log)
	require.NoError(t, embSvc.EmbedMemory(ctx, mem))

	// Retrieve via SemanticStrategy — should return the embedded memory.
	strategy := NewSemanticStrategy(provider)
	results, err := strategy.Retrieve(ctx, store.Pool(), scope, queryText, 10)
	require.NoError(t, err)
	require.NotEmpty(t, results, "semantic retrieval should find the embedded memory")

	found := false
	for _, r := range results {
		if r.ID == mem.ID {
			found = true
		}
	}
	assert.True(t, found, "saved memory should be returned by SemanticStrategy")
}

// TestExtractionWithRedaction verifies PII in observation content is redacted before save.
// The PII is placed in the assistant message so the entity name (from user message) is clean.
func TestExtractionWithRedaction(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)
	log := zap.New(zap.UseDevMode(true))

	// Put PII in the assistant message — entity name derives from last user message.
	messages := []SimpleMessage{
		{Role: "user", Content: "Remember my contact info"},
		{Role: "assistant", Content: "Sure, I noted test@example.com"},
	}

	populator := NewConversationPopulator()
	extractor := NewOmniaExtractor(store, populator, &testRedactor{}, log)

	saved, err := extractor.Extract(ctx, scope, messages)
	require.NoError(t, err)
	require.NotEmpty(t, saved, "extractor should produce at least one memory")

	// All saved memories must not contain the raw email address.
	for _, m := range saved {
		assert.NotContains(t, m.Content, "test@example.com",
			"PII should be redacted from saved memory content")
	}

	// Verify memories are in the store via List.
	listed, err := store.List(ctx, scope, ListOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, listed, "memories should be persisted in the store")
}

// TestLLMExtractionEndToEnd verifies the LLM-backed populator path.
// The mock LLM returns a structured JSON response; we verify entities and observations
// are saved to the store and retrievable.
func TestLLMExtractionEndToEnd(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)
	log := zap.New(zap.UseDevMode(true))

	// Build a valid llmExtractionResult JSON response.
	llmResponse, err := json.Marshal(map[string]any{
		"entities": []map[string]any{
			{"name": "Go programming language", "kind": "topic", "metadata": map[string]any{}},
		},
		"observations": []map[string]any{
			{
				"entity_name": "Go programming language",
				"content":     "User enjoys writing Go for backend services",
				"confidence":  0.9,
			},
		},
		"relations": []map[string]any{},
	})
	require.NoError(t, err)

	llmProvider := &testLLMProvider{response: string(llmResponse)}
	populator := NewLLMConversationPopulator(llmProvider, log)
	extractor := NewOmniaExtractor(store, populator, nil, log)

	messages := []SimpleMessage{
		{Role: "user", Content: "I love writing Go for backend services"},
		{Role: "assistant", Content: "Go is an excellent choice for backend work"},
	}

	saved, err := extractor.Extract(ctx, scope, messages)
	require.NoError(t, err)
	require.NotEmpty(t, saved, "LLM extractor should produce at least one memory")

	// Verify the extracted content is in the store.
	results, err := store.Retrieve(ctx, scope, "Go", RetrieveOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, results, "store.Retrieve should find the LLM-extracted memory")

	found := false
	for _, r := range results {
		if strings.Contains(r.Content, "Go") {
			found = true
		}
	}
	assert.True(t, found, "extracted memory content should be retrievable by keyword")
}

// TestSemanticVsKeyword saves 3 memories, embeds one, then compares strategies.
// KeywordStrategy finds the memory via ILIKE; SemanticStrategy finds via cosine similarity.
func TestSemanticVsKeyword(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)
	log := zap.New(zap.UseDevMode(true))

	contents := []string{
		"user prefers dark mode in all applications",
		"user uses mechanical keyboard for coding",
		"user drinks coffee every morning",
	}

	// Save all 3.
	memories := make([]*Memory, len(contents))
	for i, c := range contents {
		mem := &Memory{
			Type:       "preference",
			Content:    c,
			Confidence: 0.9,
			Scope:      scope,
		}
		require.NoError(t, store.Save(ctx, mem))
		memories[i] = mem
	}

	// Assign unit vectors: dark-mode memory gets vector at index 5;
	// query "theme" also gets index 5 → perfect match.
	// Other memories get different axes → orthogonal to query.
	const queryText = "application theme preference"
	provider := &testEmbeddingProvider{
		defaultDim: embeddingDim,
		embeddings: map[string][]float32{
			contents[0]: unitVector(5),  // dark mode — matches query
			contents[1]: unitVector(10), // keyboard — orthogonal
			contents[2]: unitVector(15), // coffee — orthogonal
			queryText:   unitVector(5),  // same as dark-mode
		},
	}

	// Embed all 3 via EmbeddingService.
	embSvc := NewEmbeddingService(store, provider, log)
	for _, mem := range memories {
		require.NoError(t, embSvc.EmbedMemory(ctx, mem))
	}

	// --- KeywordStrategy ---
	kwStrategy := &KeywordStrategy{}
	kwResults, err := kwStrategy.Retrieve(ctx, store.Pool(), scope, "dark", 10)
	require.NoError(t, err)
	require.NotEmpty(t, kwResults, "KeywordStrategy should find 'dark mode' memory")
	assert.Equal(t, contents[0], kwResults[0].Content)

	// --- SemanticStrategy ---
	semStrategy := NewSemanticStrategy(provider)
	semResults, err := semStrategy.Retrieve(ctx, store.Pool(), scope, queryText, 10)
	require.NoError(t, err)
	require.NotEmpty(t, semResults, "SemanticStrategy should find at least one embedded memory")

	// The dark-mode memory should appear in the results (it has the matching embedding).
	// Note: SemanticStrategy orders by e.id (UUID) after DISTINCT ON, not by similarity,
	// so we search the result set rather than asserting on position.
	foundDarkMode := false
	for _, r := range semResults {
		if r.Content == contents[0] {
			foundDarkMode = true
		}
	}
	assert.True(t, foundDarkMode, "SemanticStrategy should return the dark-mode memory (matching embedding)")
}
