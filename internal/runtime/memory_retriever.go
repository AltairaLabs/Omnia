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

	mu    sync.Mutex
	cache map[string]profileCacheEntry
}

type profileCacheEntry struct {
	memories []*pkmemory.Memory
	expires  time.Time
}

// NewCompositeRetriever builds a retriever backed by the given store.
func NewCompositeRetriever(store pkmemory.Store, log logr.Logger) *CompositeRetriever {
	return &CompositeRetriever{
		store:         store,
		log:           log.WithName("memory-retriever"),
		profileLimit:  defaultProfileLimit,
		episodicLimit: defaultEpisodicLimit,
		cache:         make(map[string]profileCacheEntry),
	}
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

	episodic, err := r.store.Retrieve(ctx, scope, query, pkmemory.RetrieveOptions{
		Limit: r.episodicLimit,
	})
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
