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

package runtime

import (
	"context"
	"strings"
	"sync"
	"time"

	pkmemory "github.com/AltairaLabs/PromptKit/runtime/memory"
	"github.com/AltairaLabs/PromptKit/runtime/types"
	"github.com/altairalabs/omnia/internal/memory/access"
	"github.com/go-logr/logr"
)

// Categories that are part of the user's "profile" — facts the agent
// should always have in ambient context regardless of what the user
// just said. The first turn of session 2 ("plan me a trip to Philly")
// has no lexical overlap with "peanut allergy", so a pure
// similarity-search retriever would miss it. Profile categories
// guarantee identity / preferences / health are present.
var profileCategories = map[string]bool{
	"memory:identity":    true,
	"memory:preferences": true,
	"memory:health":      true,
}

// StrategySemantic is the spec.memory.retrieval.strategy value that enables
// semantic hybrid retrieval with the access deny-filter.
const StrategySemantic = "semantic"

const (
	// defaultProfileLimit caps the always-included subset.
	defaultProfileLimit = 20
	// defaultEpisodicLimit caps the per-turn similarity-search subset.
	defaultEpisodicLimit = 10
	// profileCacheTTL controls how long the profile pull is cached
	// per (workspace, user). Long enough to avoid per-turn list
	// calls in a chatty conversation, short enough that a fresh
	// remember in this session shows up before the next turn.
	profileCacheTTL = 30 * time.Second
	// listFetchLimit is how many memories we ask for client-side
	// when filtering by category. memory-api's List endpoint takes
	// a Type filter, not a consent_category filter, so we pull a
	// generous prefix and filter in Go. Demo-scale only — see the
	// follow-up issue to add a category param to memory-api.
	listFetchLimit = 200
)

// SemanticRetriever is the optional capability CompositeRetriever uses when the
// configured strategy is "semantic": workspace-scoped hybrid retrieval with a
// CEL deny-filter. The Omnia memory httpclient satisfies it once
// RetrieveSemantic ships; until then the construction-time type-assert is false
// and retrieval falls back to the keyword/FTS path. Kept as a local interface
// so this package doesn't import the concrete httpclient type.
type SemanticRetriever interface {
	RetrieveSemantic(ctx context.Context, workspaceID, query, denyCEL string, limit int) ([]*pkmemory.Memory, error)
}

// RetrievalConfig carries the CRD-derived retrieval settings.
type RetrievalConfig struct {
	Strategy    string
	DenyCEL     string
	WorkspaceID string
	// Limit is the maximum number of memories injected per turn via the episodic
	// (per-turn similarity search) path. 0 means use defaultEpisodicLimit (10).
	Limit int
}

// CompositeRetriever combines an always-on "profile" pull with a
// per-turn similarity search. It satisfies pkmemory.Retriever and is
// wired by the conversation builder when a memory store is configured.
//
// Strategy: pull memory:identity / memory:preferences / memory:health
// for the user (the "profile") and merge with similarity search hits
// for the rest (memory:context / memory:history / memory:location)
// against the last user message.
type CompositeRetriever struct {
	store         pkmemory.Store
	log           logr.Logger
	profileLimit  int
	episodicLimit int

	semantic    SemanticRetriever // non-nil only when store implements it
	strategy    string            // spec.memory.retrieval.strategy
	denyCEL     string            // spec.memory.retrieval.accessFilter.denyCEL
	workspaceID string            // for the semantic call's scope

	deny       *access.DenyFilter // compiled from cfg.DenyCEL; allow-all when empty
	denyActive bool               // cfg.DenyCEL != "" (drives keyword over-fetch)

	mu    sync.Mutex
	cache map[string]profileCacheEntry
}

type profileCacheEntry struct {
	memories []*pkmemory.Memory
	expires  time.Time
}

// NewCompositeRetriever builds a retriever backed by the given store. When the
// store implements SemanticRetriever AND cfg.Strategy=="semantic", per-turn
// retrieval uses semantic hybrid search with the deny-filter; otherwise it uses
// keyword FTS.
func NewCompositeRetriever(store pkmemory.Store, cfg RetrievalConfig, log logr.Logger) (*CompositeRetriever, error) {
	deny, err := access.NewDenyFilter(cfg.DenyCEL)
	if err != nil {
		return nil, err
	}
	r := &CompositeRetriever{
		store:         store,
		log:           log.WithName("memory-retriever"),
		profileLimit:  defaultProfileLimit,
		episodicLimit: defaultEpisodicLimit,
		cache:         make(map[string]profileCacheEntry),
		strategy:      cfg.Strategy,
		denyCEL:       cfg.DenyCEL,
		workspaceID:   cfg.WorkspaceID,
		deny:          deny,
		denyActive:    cfg.DenyCEL != "",
	}
	if sr, ok := store.(SemanticRetriever); ok {
		r.semantic = sr
	}
	if cfg.Limit > 0 {
		r.episodicLimit = cfg.Limit
	}
	return r, nil
}

// RetrieveContext implements pkmemory.Retriever. Returns nil when the
// scope has no user_id (anonymous deviceId not yet plumbed, or
// scope-less invocation): the PromptKit retrieval stage treats nil as
// "no memories" and skips injection.
func (r *CompositeRetriever) RetrieveContext(
	ctx context.Context, scope map[string]string, messages []types.Message,
) ([]*pkmemory.Memory, error) {
	if scope["user_id"] == "" {
		return nil, nil
	}

	profile := r.fetchProfile(ctx, scope)

	query := lastUserContent(messages)
	if query == "" {
		// Cold-start turn (system prompt only) — profile alone.
		return profile, nil
	}

	episodic, err := r.retrieveEpisodic(ctx, scope, query)
	if err != nil {
		// Profile is still useful; episodic failure shouldn't black out
		// the whole context.
		r.log.V(1).Info("episodic retrieve failed",
			"err", err.Error(),
			"workspace", scope["workspace_id"])
		return profile, nil
	}

	return mergeNoDup(profile, filterOutProfile(episodic)), nil
}

// retrieveEpisodic runs the per-turn retrieval: semantic hybrid + deny-filter
// when configured and supported, else keyword FTS.
//
// memory-api's PostgresMemoryStore uses websearch_to_tsquery, which
// applies AND semantics across terms — a query like "remind me where
// I stayed in Chicago" requires *every* word to appear in the doc.
// That's right for memory__recall (precise lookup) but wrong for
// ambient retrieval, where we want any meaningful overlap to surface
// context. Rewrite to OR semantics: postgres's stopword filter then
// drops "remind / me / where / I / in" and what's left ("stay /
// chicago") matches ambiently.
func (r *CompositeRetriever) retrieveEpisodic(
	ctx context.Context, scope map[string]string, query string,
) ([]*pkmemory.Memory, error) {
	if r.strategy == StrategySemantic && r.semantic != nil {
		wsID := r.workspaceID
		if wsID == "" {
			wsID = scope["workspace_id"]
		}
		return r.semantic.RetrieveSemantic(ctx, wsID, query, r.denyCEL, r.episodicLimit)
	}
	return r.store.Retrieve(ctx, scope, toFTSOrQuery(query), pkmemory.RetrieveOptions{Limit: r.episodicLimit})
}

// fetchProfile returns the always-include subset, populating the cache
// on miss. Errors are logged and swallowed — a partial context is more
// useful than nothing.
func (r *CompositeRetriever) fetchProfile(
	ctx context.Context, scope map[string]string,
) []*pkmemory.Memory {
	cacheKey := scope["workspace_id"] + "|" + scope["user_id"]

	r.mu.Lock()
	if entry, ok := r.cache[cacheKey]; ok && time.Now().Before(entry.expires) {
		r.mu.Unlock()
		return entry.memories
	}
	r.mu.Unlock()

	all, err := r.store.List(ctx, scope, pkmemory.ListOptions{Limit: listFetchLimit})
	if err != nil {
		r.log.V(1).Info("profile list failed",
			"err", err.Error(),
			"workspace", scope["workspace_id"])
		return nil
	}

	profile := make([]*pkmemory.Memory, 0, r.profileLimit)
	for _, m := range all {
		if !isProfileCategory(m) {
			continue
		}
		profile = append(profile, m)
		if len(profile) >= r.profileLimit {
			break
		}
	}

	r.mu.Lock()
	r.cache[cacheKey] = profileCacheEntry{
		memories: profile,
		expires:  time.Now().Add(profileCacheTTL),
	}
	r.mu.Unlock()

	return profile
}

// metaKeyConsentCategory mirrors PromptKit's pkmemory.MetaKeyConsentCategory.
// Duplicated as a string literal because the constant isn't in the
// published SDK yet — switch to the symbol once a release ships it.
// TODO: replace with pkmemory.MetaKeyConsentCategory after PromptKit publish.
const metaKeyConsentCategory = "consent_category"

// isProfileCategory reports whether the memory's consent category is in
// the always-include set. Memories with no category fall through to
// the episodic path (similarity search).
func isProfileCategory(m *pkmemory.Memory) bool {
	if m == nil || m.Metadata == nil {
		return false
	}
	cat, _ := m.Metadata[metaKeyConsentCategory].(string)
	return profileCategories[cat]
}

// filterOutProfile drops memories that are already covered by the
// profile pull, so similarity-search results don't double up identity
// / preferences / health rows.
func filterOutProfile(memories []*pkmemory.Memory) []*pkmemory.Memory {
	out := make([]*pkmemory.Memory, 0, len(memories))
	for _, m := range memories {
		if isProfileCategory(m) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// mergeNoDup concatenates two slices, dropping any episodic memory
// whose ID already appeared in the profile slice.
func mergeNoDup(profile, episodic []*pkmemory.Memory) []*pkmemory.Memory {
	seen := make(map[string]bool, len(profile))
	out := make([]*pkmemory.Memory, 0, len(profile)+len(episodic))
	for _, m := range profile {
		if m == nil || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	for _, m := range episodic {
		if m == nil || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		out = append(out, m)
	}
	return out
}

// lastUserContent returns the trimmed Content of the most recent user
// message, or "" when no user message is present (cold-start turn).
func lastUserContent(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

// toFTSOrQuery rewrites whitespace-separated terms into a websearch
// OR query — "alpha beta gamma" → "alpha OR beta OR gamma". Postgres's
// websearch_to_tsquery treats the literal token "OR" as alternation,
// and its stopword filter strips noise tokens before matching, so
// the resulting query matches any document containing any meaningful
// term. Empty input passes through unchanged so the store's empty-
// query branch (recency list) still triggers if a caller hands us "".
func toFTSOrQuery(query string) string {
	fields := strings.Fields(query)
	if len(fields) <= 1 {
		return query
	}
	return strings.Join(fields, " OR ")
}
