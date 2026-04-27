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

	"github.com/go-logr/logr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
)

// Audit event type constants for memory operations.
const (
	// eventTypeMemoryCreated is the event type published when a memory is saved.
	eventTypeMemoryCreated = "memory_created"
	// auditEventMemoryAccessed is the event type emitted when memories are read.
	auditEventMemoryAccessed = "memory_accessed"
	// auditEventMemoryExported is the event type emitted on DSAR export.
	auditEventMemoryExported = "memory_exported"
)

// eventTypeMemoryDeleted is the event type published when a memory is deleted.
const eventTypeMemoryDeleted = "memory_deleted"

// Sentinel errors returned by the memory service and handler.
var (
	ErrMissingWorkspace = errors.New("workspace parameter is required")
	ErrMissingUserID    = errors.New("user_id in scope is required — memories must be owned by a user")
	ErrMissingQuery     = errors.New("search query parameter is required")
	ErrMissingMemoryID  = errors.New("memory ID is required")
	ErrMissingBody      = errors.New("request body is required")
	ErrBodyTooLarge     = errors.New("request body too large")
	ErrExpiresAtInPast  = errors.New("expires_at must be in the future")
	ErrMissingAgentID   = errors.New("agent_id is required for agent-scoped admin operations")
	// ErrAboutRequired fires when a Save targets a kind listed in
	// MemoryServiceConfig.RequireAboutForKinds without supplying an
	// about={kind, key} metadata hint. The handler maps this to 400
	// — the agent must retry with about populated so the structured-
	// key dedup path can engage.
	ErrAboutRequired = errors.New("about={kind, key} is required for this memory kind — supply about in the request body")
)

// MemoryServiceConfig holds runtime configuration for the MemoryService.
type MemoryServiceConfig struct {
	// DefaultTTL is applied to new memories that do not carry an explicit ExpiresAt.
	// Zero means no default TTL.
	DefaultTTL time.Duration
	// Purpose is the default purpose tag sourced from the CRD configuration.
	Purpose string
	// RequireAboutForKinds enumerates memory kinds (e.g. "fact",
	// "preference") that must carry an `about={kind, key}` metadata
	// hint on Save. Without it, the structured-key dedup path can't
	// engage, and identity-class memories pile up as duplicates
	// instead of atomic supersedes — the Phil/Slim Shard failure mode.
	// Empty disables the check (back-compat default).
	RequireAboutForKinds []string
}

// MemoryAuditLogger is the audit logging interface for memory operations.
// Implemented in ee/pkg/audit for enterprise deployments.
type MemoryAuditLogger interface {
	// LogEvent records an audit entry. Implementations must be non-blocking —
	// entries may be dropped if the internal buffer is full.
	LogEvent(ctx context.Context, entry *MemoryAuditEntry)
}

// MemoryAuditEntry represents a single audit log entry for a memory operation.
type MemoryAuditEntry struct {
	EventType   string
	MemoryID    string
	WorkspaceID string
	UserID      string
	Kind        string
	IPAddress   string
	UserAgent   string
	Metadata    map[string]string
}

// MemoryService wraps the memory store with business logic for the HTTP layer.
type MemoryService struct {
	store          memory.Store
	embeddingSvc   *memory.EmbeddingService // nil if embeddings not configured
	eventPublisher MemoryEventPublisher     // nil if event publishing not configured
	auditLogger    MemoryAuditLogger        // nil if audit logging not configured
	policyLoader   memory.PolicyLoader      // nil if no MemoryPolicy resolution wired
	config         MemoryServiceConfig
	log            logr.Logger
}

// NewMemoryService creates a new MemoryService backed by the given store.
// embeddingSvc may be nil when embedding is not configured.
func NewMemoryService(store memory.Store, embeddingSvc *memory.EmbeddingService, cfg MemoryServiceConfig, log logr.Logger) *MemoryService {
	return &MemoryService{
		store:        store,
		embeddingSvc: embeddingSvc,
		config:       cfg,
		log:          log.WithName("memory-service"),
	}
}

// SetEventPublisher configures the event publisher for the service.
// It may be called at most once before the service begins handling requests.
func (s *MemoryService) SetEventPublisher(p MemoryEventPublisher) {
	s.eventPublisher = p
}

// SetPolicyLoader wires a MemoryPolicy loader so retrieval can build a
// per-tier ranker from the workspace's bound policy. May be called at
// most once before the service begins handling requests. Optional —
// without a loader the service uses the identity ranker (no per-tier
// score adjustment).
func (s *MemoryService) SetPolicyLoader(loader memory.PolicyLoader) {
	s.policyLoader = loader
}

// SetAuditLogger configures the audit logger for the service.
// It may be called at most once before the service begins handling requests.
func (s *MemoryService) SetAuditLogger(l MemoryAuditLogger) {
	s.auditLogger = l
}

// emitAuditEvent fires an audit log entry asynchronously. If no audit logger is
// configured the call is a no-op. Request metadata (IP, User-Agent) is extracted
// from the context when present.
func (s *MemoryService) emitAuditEvent(ctx context.Context, entry *MemoryAuditEntry) {
	if s.auditLogger == nil {
		return
	}
	if meta, ok := requestMetaFromCtx(ctx); ok {
		entry.IPAddress = meta.IPAddress
		entry.UserAgent = meta.UserAgent
	}
	logger := s.auditLogger
	// Detached background context is intentional: the audit-log write must
	// complete even if the request context is cancelled (client disconnect,
	// deadline exceeded). Losing an audit event is worse than wasting work.
	go logger.LogEvent(context.Background(), entry)
}

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
	if err := s.enforceAboutForKind(ctx, mem); err != nil {
		return nil, err
	}
	if mem.ExpiresAt == nil && s.config.DefaultTTL > 0 {
		exp := time.Now().Add(s.config.DefaultTTL)
		mem.ExpiresAt = &exp
	}
	// Stamp the service-configured purpose when the caller didn't set one.
	// Same shape as DefaultTTL — the store reads Metadata[MetaKeyPurpose]
	// into memory_entities.purpose at insert time.
	if s.config.Purpose != "" {
		if mem.Metadata == nil {
			mem.Metadata = map[string]any{}
		}
		if _, ok := mem.Metadata[memory.MetaKeyPurpose]; !ok {
			mem.Metadata[memory.MetaKeyPurpose] = s.config.Purpose
		}
	}

	// Embedding-similarity dedup — only when no structured about key
	// (the structured path is more reliable), embedding service is
	// available, AND the resolved policy hasn't disabled it.
	// Failures here log + fall through to a normal insert rather
	// than failing the write.
	var (
		preMatches  []memory.SimilarObservation
		queryVector []float32 // cached so the post-write embed call doesn't re-embed
	)
	if !hasAboutKeyInMetadata(mem) && s.embeddingSvc != nil && s.embeddingDedupEnabled(ctx) {
		matches, vec, simErr := s.findSimilarForDedup(ctx, mem)
		if simErr != nil {
			s.log.V(1).Info("similarity dedup skipped",
				"reason", "embedding/query failed",
				"error", simErr.Error())
		} else {
			preMatches = matches
			queryVector = vec
		}
	}
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
	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryCreated,
			MemoryID:    mem.ID,
			WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
			UserID:      mem.Scope[memory.ScopeUserID],
			Kind:        mem.Type,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed", "eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	s.embedAsync(mem, queryVector)
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeUserID],
		Kind:        mem.Type,
	})
	return res, nil
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

// requireAboutKinds merges the static config kinds list with the
// policy-derived list. Either source can require `about` for a
// kind; the union is what's enforced. Returns nil if neither
// source has any entries (back-compat default — no enforcement).
func (s *MemoryService) requireAboutKinds(ctx context.Context) []string {
	policyKinds := s.policyRequireAboutKinds(ctx)
	if len(policyKinds) == 0 {
		return s.config.RequireAboutForKinds
	}
	if len(s.config.RequireAboutForKinds) == 0 {
		return policyKinds
	}
	// Dedup the union so a kind listed in both sources doesn't get
	// double-evaluated downstream.
	seen := make(map[string]struct{}, len(policyKinds)+len(s.config.RequireAboutForKinds))
	out := make([]string, 0, len(policyKinds)+len(s.config.RequireAboutForKinds))
	for _, k := range s.config.RequireAboutForKinds {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	for _, k := range policyKinds {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

// policyRequireAboutKinds resolves the current MemoryPolicy and
// returns its dedup.requireAboutForKinds list. Failures (loader
// nil, transient API outage, no policy bound) silently return nil
// so policy unavailability doesn't block writes — the static config
// is still in force.
func (s *MemoryService) policyRequireAboutKinds(ctx context.Context) []string {
	if s.policyLoader == nil {
		return nil
	}
	policy, err := s.policyLoader.Load(ctx)
	if err != nil || policy == nil {
		return nil
	}
	return policy.DedupRequireAboutForKinds()
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
	go func() {
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
	}()
}

// embeddingDedupEnabled resolves whether the embedding-similarity
// dedup path runs. Defaults to true (the existing behaviour) when
// no policy is configured or the policy doesn't speak to it.
func (s *MemoryService) embeddingDedupEnabled(ctx context.Context) bool {
	policy := s.loadPolicy(ctx)
	if policy == nil {
		return true
	}
	return policy.DedupEmbeddingEnabled()
}

// autoSupersedeThreshold resolves the cosine floor above which the
// server auto-supersedes a near-duplicate. Falls back to the
// package-level DefaultAutoSupersedeSimilarity (0.95) when the
// policy doesn't override.
func (s *MemoryService) autoSupersedeThreshold(ctx context.Context) float64 {
	if v := s.policyFloat(ctx, (*omniav1alpha1.MemoryPolicy).DedupAutoSupersedeAbove); v > 0 {
		return v
	}
	return memory.DefaultAutoSupersedeSimilarity
}

// surfaceDuplicateThreshold resolves the cosine floor above which
// the server surfaces a near-duplicate as a `potential_duplicates`
// hint. Falls back to DefaultSurfaceDuplicateSimilarity (0.85).
func (s *MemoryService) surfaceDuplicateThreshold(ctx context.Context) float64 {
	if v := s.policyFloat(ctx, (*omniav1alpha1.MemoryPolicy).DedupSurfaceDuplicatesAbove); v > 0 {
		return v
	}
	return memory.DefaultSurfaceDuplicateSimilarity
}

// duplicateCandidateLimit resolves the cap on `potential_duplicates`
// returned to the agent. Falls back to DefaultDuplicateCandidateLimit
// (5).
func (s *MemoryService) duplicateCandidateLimit(ctx context.Context) int {
	policy := s.loadPolicy(ctx)
	if policy != nil {
		if n := policy.DedupCandidateLimit(); n > 0 {
			return n
		}
	}
	return memory.DefaultDuplicateCandidateLimit
}

// policyFloat is the shared accessor for the string-typed similarity
// thresholds. It loads the policy once and applies the supplied
// extractor.
func (s *MemoryService) policyFloat(ctx context.Context, get func(*omniav1alpha1.MemoryPolicy) float64) float64 {
	policy := s.loadPolicy(ctx)
	if policy == nil {
		return 0
	}
	return get(policy)
}

// maxRelatedPerMemory resolves the per-memory cap on `related[]`
// returned in recall responses. Falls back to defaultRelatedPerMemory
// (3) when no policy is configured.
func (s *MemoryService) maxRelatedPerMemory(ctx context.Context) int {
	policy := s.loadPolicy(ctx)
	if policy != nil {
		if n := policy.RecallMaxRelatedPerMemory(); n > 0 {
			return n
		}
	}
	return defaultRelatedPerMemory
}

// InlineThresholdBytes returns the body-size cutoff above which
// recall returns title + summary + content_preview rather than the
// full body. Exported for the handler — it builds the response DTO
// and needs to know the cutoff per-request. Falls back to the
// package-level default when no policy is configured.
func (s *MemoryService) InlineThresholdBytes(ctx context.Context) int {
	policy := s.loadPolicy(ctx)
	if policy != nil {
		if n := policy.RecallInlineThresholdBytes(); n > 0 {
			return n
		}
	}
	return InlineBodyThresholdBytes
}

// loadPolicy fetches the bound MemoryPolicy via the loader. Returns
// nil on missing loader, no policy bound, or transient failure —
// callers fall through to package-level defaults in any of those
// cases. The K8sPolicyLoader has its own TTL cache so repeated calls
// from the same Save don't hit the K8s API more than once.
func (s *MemoryService) loadPolicy(ctx context.Context) *omniav1alpha1.MemoryPolicy {
	if s.policyLoader == nil {
		return nil
	}
	policy, err := s.policyLoader.Load(ctx)
	if err != nil {
		return nil
	}
	return policy
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
	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryCreated,
			MemoryID:    mem.ID,
			WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
			UserID:      mem.Scope[memory.ScopeUserID],
			Kind:        mem.Type,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed", "eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	s.embedAsync(mem, cachedVector)
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeUserID],
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

// OpenMemory returns the full content of a single memory by entity
// ID. Mirrors the recall scope filter; the active-only filter
// applies so superseded observations are not returned. Used by
// memory__open when the agent needs the body of a large memory.
func (s *MemoryService) OpenMemory(ctx context.Context, scope map[string]string, entityID string) (*memory.Memory, error) {
	mem, err := s.store.GetMemory(ctx, scope, entityID)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		MemoryID:    entityID,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{"operation": "open"},
	})
	return mem, nil
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

	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryCreated,
			MemoryID:    mem.ID,
			WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
			UserID:      mem.Scope[memory.ScopeUserID],
			Kind:        mem.Type,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed",
					"eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	if s.embeddingSvc != nil {
		go func() {
			embedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.embeddingSvc.EmbedMemory(embedCtx, mem); err != nil {
				s.log.Error(err, "async embedding failed", "memoryID", mem.ID)
			}
		}()
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    mem.ID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeUserID],
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

// FindConflicts returns entities whose dedup machinery missed —
// i.e. those holding more than one active observation. The dashboard
// renders this as a triage queue so operators can see when the
// `about` discipline (or the embedding-similarity threshold) has
// drifted.
func (s *MemoryService) FindConflicts(ctx context.Context, workspaceID string, limit int) ([]memory.ConflictedEntity, error) {
	out, err := s.store.FindConflictedEntities(ctx, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: workspaceID,
		Metadata:    map[string]string{"operation": "list_conflicts"},
	})
	return out, nil
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

	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryCreated,
			MemoryID:    anchorID,
			WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
			UserID:      mem.Scope[memory.ScopeUserID],
			Kind:        mem.Type,
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed",
					"eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	if s.embeddingSvc != nil {
		go func() {
			embedCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := s.embeddingSvc.EmbedMemory(embedCtx, mem); err != nil {
				s.log.Error(err, "async embedding failed", "memoryID", mem.ID)
			}
		}()
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    anchorID,
		WorkspaceID: mem.Scope[memory.ScopeWorkspaceID],
		UserID:      mem.Scope[memory.ScopeUserID],
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

// LinkMemories inserts a directed edge in memory_relations so
// derived facts (preferences, notes) attached to an anchor entity
// (the user identity) survive renames of the target. Returns the
// new relation ID.
func (s *MemoryService) LinkMemories(ctx context.Context, scope map[string]string,
	sourceEntityID, targetEntityID, relationType string, weight float64,
) (string, error) {
	id, err := s.store.LinkEntities(ctx, scope, sourceEntityID, targetEntityID, relationType, weight)
	if err != nil {
		return "", err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryCreated,
		MemoryID:    id,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata: map[string]string{
			"operation":     "link",
			"source_id":     sourceEntityID,
			"target_id":     targetEntityID,
			"relation_type": relationType,
		},
	})
	return id, nil
}

// SearchMemories retrieves memories matching a query and scope.
// When an embedding service is configured and the query is non-empty,
// the call routes through Store.RetrieveHybrid so semantic-only
// matches (e.g. "what do I prefer?" → "user likes dark mode") surface
// alongside lexical hits via Reciprocal Rank Fusion. Without an
// embedder, or for empty queries, it falls through to the FTS-only
// Retrieve path.
func (s *MemoryService) SearchMemories(ctx context.Context, scope map[string]string, query string, opts memory.RetrieveOptions) ([]*memory.Memory, error) {
	results, err := s.searchMemoriesInner(ctx, scope, query, opts)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{"operation": "search"},
	})
	return results, nil
}

// searchMemoriesInner picks between the FTS-only Retrieve and the
// hybrid RRF path based on embedder availability. If the embedding
// call fails the call falls back to FTS so a transient embedder
// outage degrades recall quality rather than hard-failing the
// request — recall is too central to the agent loop to make brittle.
func (s *MemoryService) searchMemoriesInner(ctx context.Context, scope map[string]string, query string, opts memory.RetrieveOptions) ([]*memory.Memory, error) {
	if s.embeddingSvc == nil || query == "" {
		return s.store.Retrieve(ctx, scope, query, opts)
	}
	embeddings, err := s.embeddingSvc.Provider().Embed(ctx, []string{query})
	if err != nil || len(embeddings) == 0 || len(embeddings[0]) == 0 {
		s.log.V(1).Info("hybrid recall fallback to FTS",
			"reason", "embed_query_failed", "error", err)
		return s.store.Retrieve(ctx, scope, query, opts)
	}
	return s.store.RetrieveHybrid(ctx, scope, query, embeddings[0], opts)
}

// defaultRelatedPerMemory caps the per-memory related[] list. Three keeps
// the recall payload lean while still letting the agent see the strongest
// graph neighbours (an identity entity's preferences, a workspace doc's
// related skills) it might want to update or supersede.
const defaultRelatedPerMemory = 3

// RelatedForMemories returns a map keyed by source entity ID, with each
// value being the relations originating from that entity. Used by the
// recall path to attach `related[]` to each result so the agent can
// navigate the memory graph and reason about supersession candidates
// without making a second round-trip per memory.
//
// Returns an empty map (not nil) when there are no memories so the
// handler can call this unconditionally and look up by ID without nil
// guards.
func (s *MemoryService) RelatedForMemories(ctx context.Context, scope map[string]string, mems []*memory.Memory) map[string][]memory.EntityRelation {
	out := make(map[string][]memory.EntityRelation, len(mems))
	if len(mems) == 0 {
		return out
	}
	ids := make([]string, 0, len(mems))
	for _, m := range mems {
		if m != nil && m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return out
	}
	rels, err := s.store.FindRelatedEntities(ctx, scope, ids, s.maxRelatedPerMemory(ctx))
	if err != nil {
		s.log.V(1).Info("recall related lookup failed", "error", err, "ids", len(ids))
		return out
	}
	for _, r := range rels {
		out[r.SourceEntityID] = append(out[r.SourceEntityID], r)
	}
	return out
}

// ListMemories returns memories for a given scope with pagination.
func (s *MemoryService) ListMemories(ctx context.Context, scope map[string]string, opts memory.ListOptions) ([]*memory.Memory, error) {
	results, err := s.store.List(ctx, scope, opts)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{"operation": "list"},
	})
	return results, nil
}

// AggregateMemories runs a workspace-scoped aggregate over memory_entities.
// Thin pass-through to the store; kept here for symmetry with other Service
// methods so handlers always go through one indirection. Type-asserts to
// *PostgresMemoryStore because the fake stores in the test suite don't
// implement Aggregate; an interface addition would break every fake for
// no real benefit.
func (s *MemoryService) AggregateMemories(ctx context.Context, opts memory.AggregateOptions) ([]memory.AggregateRow, error) {
	pgStore, ok := s.store.(*memory.PostgresMemoryStore)
	if !ok {
		return nil, errors.New("memory service: aggregate requires a PostgresMemoryStore")
	}
	return pgStore.Aggregate(ctx, opts)
}

// DeleteMemory performs a soft delete (forget) of a single memory.
func (s *MemoryService) DeleteMemory(ctx context.Context, scope map[string]string, memoryID string) error {
	if err := s.store.Delete(ctx, scope, memoryID); err != nil {
		return err
	}
	if s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryDeleted,
			MemoryID:    memoryID,
			WorkspaceID: scope[memory.ScopeWorkspaceID],
			UserID:      scope[memory.ScopeUserID],
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory event publish failed", "eventType", event.EventType, "memoryID", event.MemoryID)
			}
		}()
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryDeleted,
		MemoryID:    memoryID,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
	})
	return nil
}

// DeleteAllMemories hard-deletes all memories for the given scope (DSAR).
func (s *MemoryService) DeleteAllMemories(ctx context.Context, scope map[string]string) error {
	if err := s.store.DeleteAll(ctx, scope); err != nil {
		return err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   eventTypeMemoryDeleted,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
		Metadata:    map[string]string{"operation": "delete_all"},
	})
	return nil
}

// BatchDeleteMemories hard-deletes up to limit memories for the given scope (paginated DSAR).
// Returns the count of deleted rows so the caller can loop until 0.
func (s *MemoryService) BatchDeleteMemories(ctx context.Context, scope map[string]string, limit int) (int, error) {
	n, err := s.store.BatchDelete(ctx, scope, limit)
	if err != nil {
		return 0, err
	}
	if n > 0 && s.eventPublisher != nil {
		event := MemoryEvent{
			EventType:   eventTypeMemoryDeleted,
			WorkspaceID: scope[memory.ScopeWorkspaceID],
			UserID:      scope[memory.ScopeUserID],
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}
		go func() {
			if err := s.eventPublisher.PublishMemoryEvent(context.Background(), event); err != nil {
				s.log.Error(err, "memory batch delete event publish failed", "eventType", event.EventType, "count", n)
			}
		}()
	}
	if n > 0 {
		s.emitAuditEvent(ctx, &MemoryAuditEntry{
			EventType:   eventTypeMemoryDeleted,
			WorkspaceID: scope[memory.ScopeWorkspaceID],
			UserID:      scope[memory.ScopeUserID],
			Metadata:    map[string]string{"operation": "batch_delete"},
		})
	}
	return n, nil
}

// ExportMemories returns all memories for a scope without pagination (DSAR export).
func (s *MemoryService) ExportMemories(ctx context.Context, scope map[string]string) ([]*memory.Memory, error) {
	memories, err := s.store.ExportAll(ctx, scope)
	if err != nil {
		return nil, err
	}
	s.log.V(1).Info("memories exported", "workspace", scope[memory.ScopeWorkspaceID], "count", len(memories))
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryExported,
		WorkspaceID: scope[memory.ScopeWorkspaceID],
		UserID:      scope[memory.ScopeUserID],
	})
	return memories, nil
}
