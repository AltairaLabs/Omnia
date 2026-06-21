package runtime

import (
	"context"
	"testing"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/pkg/policy"
)

// stubStore is a distinguishable real store for identity assertions in
// memoryWiring tests. It embeds noopMemoryStore to satisfy the interface but is
// a distinct type, so a type switch can tell it apart from noopMemoryStore.
type stubStore struct{ noopMemoryStore }

func TestNoopMemoryStore(t *testing.T) {
	var s noopMemoryStore
	ctx := context.Background()
	scope := map[string]string{"workspace_id": "ws", "user_id": "u"}

	if err := s.Save(ctx, &pkmemory.Memory{Content: "x"}); err != nil {
		t.Errorf("Save: unexpected error %v", err)
	}
	if got, err := s.Retrieve(ctx, scope, "q", pkmemory.RetrieveOptions{}); err != nil || got != nil {
		t.Errorf("Retrieve = (%v, %v), want (nil, nil)", got, err)
	}
	if got, err := s.List(ctx, scope, pkmemory.ListOptions{}); err != nil || got != nil {
		t.Errorf("List = (%v, %v), want (nil, nil)", got, err)
	}
	if err := s.Delete(ctx, scope, "id"); err != nil {
		t.Errorf("Delete: unexpected error %v", err)
	}
	if err := s.DeleteAll(ctx, scope); err != nil {
		t.Errorf("DeleteAll: unexpected error %v", err)
	}
}

func TestMemoryWiring(t *testing.T) {
	real := stubStore{}

	tests := []struct {
		name           string
		retrieval      bool
		tools          bool
		wantNoop       bool // executor should be the no-op store
		wantAttachRetr bool
	}{
		{"both on", true, true, false, true},
		{"rag on, tools off", true, false, true, true},
		{"rag off, tools on", false, true, false, false},
		{"both off", false, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, attach := memoryWiring(real, tt.retrieval, tt.tools)

			_, isNoop := executor.(noopMemoryStore)
			if isNoop != tt.wantNoop {
				t.Errorf("executor noop = %v (%T), want noop = %v", isNoop, executor, tt.wantNoop)
			}
			if !tt.wantNoop {
				if _, ok := executor.(stubStore); !ok {
					t.Errorf("tools on: executor should be the real store, got %T", executor)
				}
			}
			if attach != tt.wantAttachRetr {
				t.Errorf("attachRetriever = %v, want %v", attach, tt.wantAttachRetr)
			}
		})
	}
}

// TestBuildConversationOptions_MemoryToggles drives the memory-wiring block in
// buildConversationOptions across the four combos, guarding the "code exists but
// isn't wired" failure mode and exercising the no-op store / retriever / tool-
// override branches.
func TestBuildConversationOptions_MemoryToggles(t *testing.T) {
	ctx := policy.WithUserID(context.Background(), "user-1")

	base := NewServer(WithLogger(logr.Discard()))
	baseOpts, err := base.buildConversationOptions(ctx, "sess")
	require.NoError(t, err)

	newServer := func(retrieval, tools bool) *Server {
		s := NewServer(
			WithLogger(logr.Discard()),
			WithMemoryStore(stubStore{}),
			WithWorkspaceUID("ws-uid"),
			WithMemoryModes(retrieval, tools),
		)
		s.agentUID = "agent-1"
		return s
	}

	tests := []struct {
		name          string
		retrieval     bool
		tools         bool
		wantMemoryOpt bool // memory wiring should add at least the WithMemory option
	}{
		{"both on", true, true, true},
		{"rag on, tools off", true, false, true},
		{"rag off, tools on", false, true, true},
		{"both off", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := newServer(tt.retrieval, tt.tools).buildConversationOptions(ctx, "sess")
			require.NoError(t, err)
			if tt.wantMemoryOpt {
				require.Greater(t, len(opts), len(baseOpts), "memory should add SDK options")
			} else {
				require.Equal(t, len(baseOpts), len(opts), "both-off should wire no memory options")
			}
		})
	}
}
