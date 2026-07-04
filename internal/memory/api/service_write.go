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

// Package api provides the HTTP API layer for the memory-api service.
package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/altairalabs/omnia/internal/memory"
)

// SaveMemory persists a memory entry and, if an embedding service is
// configured, asynchronously generates and stores its embedding.
// Backwards-compatible thin wrapper around SaveMemoryWithResult that
// discards the dedup result. New callers prefer SaveMemoryWithResult
// so the agent sees auto_superseded / potential_duplicates info.
func (s *MemoryService) SaveMemory(ctx context.Context, mem *memory.Memory) error {
	_, err := s.SaveMemoryWithResult(ctx, mem)
	return err
}

// SaveMemoryWithResult is the rich Omnia write API. Returns a
// SaveResult describing how the dedup pipeline resolved the write
// (added vs auto_superseded; supersedes ids; reason). The HTTP
// handler surfaces this to the agent so its reply ("Got it" vs
// "Updated your name from X to Y") and follow-up tool calls can be
// honest about what happened.
//
// Two dedup paths run before the write commits:
//
//  1. Structured key. When the caller set about_kind+about_key in
//     metadata the store's ON CONFLICT path supersedes any prior
//     observation under the same entity (handled inside
//     store.SaveWithResult).
//  2. Embedding similarity. When no about key is set AND an
//     embedding service is configured, the service embeds the new
//     content and queries for similar active observations under the
//     same scope. cosine ≥ 0.95 routes through
//     AppendObservationToEntity to atomically supersede the match;
//     0.85 ≤ cosine < 0.95 lands the write normally and surfaces the
//     matches as PotentialDuplicates so the agent can decide on a
//     later turn.
func (s *MemoryService) SaveMemoryWithResult(ctx context.Context, mem *memory.Memory) (*memory.SaveResult, error) {
	// Resolve the policy once at the top of the request so every
	// helper called below (enforceAboutForKind, embeddingDedupEnabled,
	// autoSupersedeThreshold, surfaceDuplicateThreshold,
	// duplicateCandidateLimit) reads the same snapshot. Without this
	// each helper goes through loadPolicy → atomic.Pointer.Load and a
	// TTL flip mid-request can change thresholds between dedup and
	// supersede.
	ctx = withPolicyContext(ctx, s.fetchPolicy(ctx))
	if err := s.enforceAboutForKind(ctx, mem); err != nil {
		return nil, err
	}
	s.applyTTLPolicy(ctx, mem)
	s.stampPurpose(mem)

	preMatches, queryVector := s.runPreWriteDedup(ctx, mem)
	if len(preMatches) > 0 && preMatches[0].Similarity >= s.autoSupersedeThreshold(ctx) {
		return s.applyAutoSupersedeViaSimilarity(ctx, mem, preMatches[0], queryVector)
	}

	res, err := s.store.SaveWithResult(ctx, mem)
	if err != nil {
		return nil, err
	}
	for _, m := range preMatches {
		res.PotentialDuplicates = append(res.PotentialDuplicates, memory.DuplicateCandidate{
			ID:         m.ObservationID,
			Content:    m.Content,
			Similarity: m.Similarity,
		})
	}
	s.publishMemoryCreatedEvent(mem.ID, mem)
	s.embedAsync(mem, queryVector)
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeVirtualUserID],
		Kind:        mem.Type,
	})
	return res, nil
}

// stampPurpose writes the service-configured purpose into the
// memory's metadata when the caller didn't set one. Same shape as
// DefaultTTL — the store reads Metadata[MetaKeyPurpose] into
// memory_entities.purpose at insert time.
func (s *MemoryService) stampPurpose(mem *memory.Memory) {
	if s.config.Purpose == "" {
		return
	}
	if mem.Metadata == nil {
		mem.Metadata = map[string]any{}
	}
	if _, ok := mem.Metadata[memory.MetaKeyPurpose]; !ok {
		mem.Metadata[memory.MetaKeyPurpose] = s.config.Purpose
	}
}

// runPreWriteDedup runs the embedding-similarity dedup pass — only
// when no structured about key (the structured path is more
// reliable), an embedding service is available, AND the resolved
// policy hasn't disabled it. Failures log + return no matches so the
// caller falls through to a normal insert rather than failing the
// write. Returns the matches plus the query embedding vector (cached
// so the post-write embed call doesn't re-embed).
func (s *MemoryService) runPreWriteDedup(ctx context.Context, mem *memory.Memory) ([]memory.SimilarObservation, []float32) {
	if hasAboutKeyInMetadata(mem) || s.embeddingSvc == nil || !s.embeddingDedupEnabled(ctx) {
		return nil, nil
	}
	matches, vec, simErr := s.findSimilarForDedup(ctx, mem)
	if simErr != nil {
		s.log.V(1).Info("similarity dedup skipped",
			"reason", "embedding/query failed",
			"error", simErr.Error())
		return nil, nil
	}
	return matches, vec
}

// publishMemoryCreatedEvent fires a memory_created event asynchronously
// under the "event_publish" safeGo label. No-op when no event publisher
// is wired.
func (s *MemoryService) publishMemoryCreatedEvent(memoryID string, mem *memory.Memory) {
	if s.eventPublisher == nil {
		return
	}
	event := MemoryEvent{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    memoryID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeVirtualUserID],
		Kind:        mem.Type,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	s.safeGo("event_publish", func() {
		if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
			s.log.Error(err, logMemoryEventPublishFailed, "eventType", event.EventType, "memoryID", event.MemoryID)
		}
	})
}

// hasAboutKeyInMetadata reports whether the caller set the
// structured-dedup metadata keys; if so the store handles dedup
// via the unique index path and the embedding-similarity path is
// skipped.
func hasAboutKeyInMetadata(mem *memory.Memory) bool {
	if mem == nil || mem.Metadata == nil {
		return false
	}
	kind, _ := mem.Metadata[memory.MetaKeyAboutKind].(string)
	key, _ := mem.Metadata[memory.MetaKeyAboutKey].(string)
	return kind != "" && key != ""
}

// enforceAboutForKind returns ErrAboutRequired when a memory's
// kind is in either the static config or the resolved MemoryPolicy's
// dedup.requireAboutForKinds list, and the caller didn't set
// about={kind, key} in metadata. Identity-class kinds (fact,
// preference) need the structured-key path to dedup correctly —
// without about they pile up as duplicates that can't be atomically
// superseded on rename or update.
//
// Kind comparison is case-insensitive: an operator listing "fact"
// in the policy must also catch agents that send "Fact" or "FACT".
func (s *MemoryService) enforceAboutForKind(ctx context.Context, mem *memory.Memory) error {
	if mem == nil {
		return nil
	}
	kinds := s.requireAboutKinds(ctx)
	if len(kinds) == 0 {
		return nil
	}
	memType := strings.ToLower(strings.TrimSpace(mem.Type))
	for _, k := range kinds {
		if strings.ToLower(strings.TrimSpace(k)) == memType && !hasAboutKeyInMetadata(mem) {
			return ErrAboutRequired
		}
	}
	return nil
}

// findSimilarForDedup embeds the new content (synchronously — adds
// ~one embedding-API roundtrip to the write path) and asks the
// store for active observations within the surface-duplicates
// similarity floor. Returns the matches AND the embedding vector
// so the caller can reuse it for the post-write store update —
// without that the auto-supersede / clean-insert path embeds the
// same content a second time. Returns (nil, nil, err) when
// embedding fails or yields no result.
func (s *MemoryService) findSimilarForDedup(ctx context.Context, mem *memory.Memory) ([]memory.SimilarObservation, []float32, error) {
	embeddings, err := s.embeddingSvc.Provider().Embed(ctx, []string{mem.Content})
	if err != nil {
		return nil, nil, err
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return nil, nil, errors.New("embed returned empty result")
	}
	matches, err := s.store.FindSimilarObservations(ctx, mem.Scope, embeddings[0],
		s.duplicateCandidateLimit(ctx),
		s.surfaceDuplicateThreshold(ctx))
	if err != nil {
		return nil, embeddings[0], err
	}
	return matches, embeddings[0], nil
}

// embedAsync writes the embedding for the new observation in the
// background. When cachedVector is non-nil (the dedup path already
// computed it) it skips the provider call and writes directly to
// avoid double-embedding the same content. When the cache is nil it
// falls back to embeddingSvc.EmbedMemory which embeds + writes.
func (s *MemoryService) embedAsync(mem *memory.Memory, cachedVector []float32) {
	if s.embeddingSvc == nil {
		return
	}
	s.safeGo("embed", func() {
		embedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if len(cachedVector) > 0 {
			if err := s.embeddingSvc.WriteEmbedding(embedCtx, mem.ID, cachedVector); err != nil {
				s.log.Error(err, "async embedding write failed", "memoryID", mem.ID)
			}
			return
		}
		if err := s.embeddingSvc.EmbedMemory(embedCtx, mem); err != nil {
			s.log.Error(err, "async embedding failed", "memoryID", mem.ID)
		}
	})
}

// applyTTLPolicy stamps the write-time expires_at and caps it (#1336). The
// default comes from the memory's tier policy (spec.tiers[tier].ttl.default),
// falling back to the global --default-ttl when the tier sets none. Any
// expires_at (caller-supplied or defaulted) is then capped to the tier's
// ttl.maxAge so a misbehaving caller can't pin a row indefinitely. The policy
// snapshot is read from the request context stashed by SaveMemoryWithResult.
func (s *MemoryService) applyTTLPolicy(ctx context.Context, mem *memory.Memory) {
	policy, _ := policyFromContext(ctx)
	ttl := memory.ResolveTierTTL(policy, mem.Scope)
	now := time.Now()

	if mem.ExpiresAt == nil {
		d := ttl.Default
		if d <= 0 {
			d = s.config.DefaultTTL
		}
		if d > 0 {
			exp := now.Add(d)
			mem.ExpiresAt = &exp
		}
	}
	if ttl.MaxAge > 0 && mem.ExpiresAt != nil {
		capped := now.Add(ttl.MaxAge)
		if mem.ExpiresAt.After(capped) {
			mem.ExpiresAt = &capped
		}
	}
}

// applyAutoSupersedeViaSimilarity attaches the new observation to
// the matched entity and supersedes the entity's prior active
// observations atomically. Returns a SaveResult marked
// auto_superseded with reason=high_similarity. The agent uses this
// to phrase its reply honestly ("I already had something like that
// — refreshed").
func (s *MemoryService) applyAutoSupersedeViaSimilarity(
	ctx context.Context,
	mem *memory.Memory,
	match memory.SimilarObservation,
	cachedVector []float32,
) (*memory.SaveResult, error) {
	supersededIDs, err := s.store.AppendObservationToEntity(ctx, match.EntityID, mem)
	if err != nil {
		return nil, err
	}

	// Audit + async embed for the new observation, mirroring the
	// happy-path behaviour. Event publish + audit fire even on
	// supersede so dashboards see "this entity was updated".
	s.publishMemoryCreatedEvent(mem.ID, mem)
	s.embedAsync(mem, cachedVector)
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeVirtualUserID],
		Kind:        mem.Type,
		Metadata: map[string]string{
			"dedup_reason": string(memory.ReasonHighSimilarity),
		},
	})

	return &memory.SaveResult{
		ID:                       mem.ID,
		Action:                   memory.SaveActionAutoSuperseded,
		SupersededObservationIDs: supersededIDs,
		SupersedeReason:          memory.ReasonHighSimilarity,
	}, nil
}

// UpdateMemory atomically supersedes the prior active observation
// under the given entity and inserts a new one with the supplied
// content. Returns a SaveResult shaped for the agent's reply
// (action=auto_superseded with reason=explicit). The agent uses
// memory__update for cases where it knows the entity ID — most
// commonly when recall surfaced the prior observation and the agent
// decides this is a replacement.
func (s *MemoryService) UpdateMemory(ctx context.Context, entityID string, mem *memory.Memory) (*memory.SaveResult, error) {
	supersededIDs, err := s.store.AppendObservationToEntity(ctx, entityID, mem)
	if err != nil {
		return nil, err
	}

	s.publishMemoryCreatedEvent(mem.ID, mem)
	s.embedAsync(mem, nil)
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeVirtualUserID],
		Kind:        mem.Type,
		Metadata:    map[string]string{"dedup_reason": "explicit"},
	})

	return &memory.SaveResult{
		ID:                       mem.ID,
		Action:                   memory.SaveActionAutoSuperseded,
		SupersededObservationIDs: supersededIDs,
		SupersedeReason:          memory.SaveSupersedeReason("explicit"),
	}, nil
}

// SupersedeManyMemories collapses multiple stale entities into one
// canonical truth: every source entity's active observations are
// marked inactive and a single new observation is written under the
// first source entity. Powers the memory__supersede agent tool —
// the agent surfaces a recall result, sees N memories about the
// same fact (different entities because the agent forgot to set
// `about`), and consolidates them in one round trip.
//
// Returns the same SaveResult shape as UpdateMemory so the agent's
// reply ("Got it — I had three memories about your name; collapsed
// them into the new value") can be honest about the count of rows
// it took inactive.
func (s *MemoryService) SupersedeManyMemories(ctx context.Context, sourceMemoryIDs []string, mem *memory.Memory) (*memory.SaveResult, error) {
	anchorID, supersededIDs, err := s.store.SupersedeMany(ctx, sourceMemoryIDs, mem)
	if err != nil {
		return nil, err
	}

	s.publishMemoryCreatedEvent(anchorID, mem)
	s.embedAsync(mem, nil)
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    anchorID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeVirtualUserID],
		Kind:        mem.Type,
		Metadata: map[string]string{
			"dedup_reason": "supersede_many",
			"source_count": fmt.Sprintf("%d", len(sourceMemoryIDs)),
			"superseded_n": fmt.Sprintf("%d", len(supersededIDs)),
		},
	})

	return &memory.SaveResult{
		ID:                       anchorID,
		Action:                   memory.SaveActionAutoSuperseded,
		SupersededObservationIDs: supersededIDs,
		SupersedeReason:          memory.SaveSupersedeReason("supersede_many"),
	}, nil
}
