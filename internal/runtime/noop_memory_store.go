package runtime

import (
	"context"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
)

// noopMemoryStore is a pkmemory.Store that discards writes and returns no
// memories. It backs the memory tool executor when spec.memory.tools.enabled is
// false: the tools stay registered (PromptKit always registers them), but
// memory__remember discards and memory__recall returns empty. The ambient RAG
// retriever continues to use the real store, so retrieval is unaffected.
//
// This is the interim mechanism until PromptKit can register a retriever
// without the tools (AltairaLabs/PromptKit#1427), at which point tools-off can
// simply skip tool registration and this type goes away.
type noopMemoryStore struct{}

// Save discards the memory.
func (noopMemoryStore) Save(_ context.Context, _ *pkmemory.Memory) error { return nil }

// Retrieve returns no memories.
func (noopMemoryStore) Retrieve(_ context.Context, _ map[string]string, _ string, _ pkmemory.RetrieveOptions) ([]*pkmemory.Memory, error) {
	return nil, nil
}

// List returns no memories.
func (noopMemoryStore) List(_ context.Context, _ map[string]string, _ pkmemory.ListOptions) ([]*pkmemory.Memory, error) {
	return nil, nil
}

// Delete is a no-op.
func (noopMemoryStore) Delete(_ context.Context, _ map[string]string, _ string) error { return nil }

// DeleteAll is a no-op.
func (noopMemoryStore) DeleteAll(_ context.Context, _ map[string]string) error { return nil }

// memoryWiring decides, from the real store and the two CRD axes
// (spec.memory.retrieval.enabled and spec.memory.tools.enabled), which store the
// memory tool executor should use and whether to attach the ambient retriever:
//   - tools off  → executor gets a no-op store (remember/recall do nothing)
//   - tools on   → executor gets the real store
//   - attachRetriever mirrors retrievalEnabled
//
// The retriever always runs against the real store (passed separately), so RAG
// is unaffected by the executor's store choice.
func memoryWiring(real pkmemory.Store, retrievalEnabled, toolsEnabled bool) (executor pkmemory.Store, attachRetriever bool) {
	executor = real
	if !toolsEnabled {
		executor = noopMemoryStore{}
	}
	return executor, retrievalEnabled
}
