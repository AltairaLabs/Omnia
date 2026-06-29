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
	"time"

	"github.com/go-logr/logr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eememory "github.com/altairalabs/omnia/ee/pkg/memory"
	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
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

// eventTypeConsentPrune is the audit event type emitted when memories are
// pruned in response to a per-user consent-revocation event (CE1).
const eventTypeConsentPrune = "memory_consent_prune"

// logMemoryEventPublishFailed is the structured-log message emitted when an
// event-bus publish fails. Extracted to a constant to keep grep-ability
// across 5+ call sites (and satisfy Sonar's S1192).
const logMemoryEventPublishFailed = "memory event publish failed"

// metaKeyOperation is the audit-entry metadata key tagging which service
// operation produced the entry. Extracted to a constant — the literal
// recurs across every read/delete audit call (satisfies goconst / S1192).
const metaKeyOperation = "operation"

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
	// Enterprise gates advanced-memory recall. When false, RetrieveMultiTier
	// is restricted to the OSS floor (user+agent tiers, identity ranker,
	// default half-life); the institutional tier and policy-driven ranking
	// are enterprise-only.
	Enterprise bool
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
	store              memory.Store
	institutional      eememory.InstitutionalStore // nil unless enterprise (set via SetInstitutionalStore)
	embeddingSvc       *memory.EmbeddingService    // nil if embeddings not configured
	eventPublisher     MemoryEventPublisher        // nil if event publishing not configured
	auditLogger        MemoryAuditLogger           // nil if audit logging not configured
	policyLoader       memory.PolicyLoader         // nil if no MemoryPolicy resolution wired
	consentEventPruner memory.ConsentEventPruner   // nil if not wired; used by PruneUserConsentCategory
	ingestFallback     ingestion.Config            // default ingestion config (from --ingest-* flags)
	summaryQueue       ingestion.SummaryQueue      // nil disables the agent path (falls back to extractive)
	config             MemoryServiceConfig
	log                logr.Logger
	enterprise         bool
}

// NewMemoryService creates a new MemoryService backed by the given store.
// embeddingSvc may be nil when embedding is not configured.
func NewMemoryService(store memory.Store, embeddingSvc *memory.EmbeddingService, cfg MemoryServiceConfig, log logr.Logger) *MemoryService {
	return &MemoryService{
		store:        store,
		embeddingSvc: embeddingSvc,
		config:       cfg,
		log:          log.WithName("memory-service"),
		enterprise:   cfg.Enterprise,
	}
}

// SetEventPublisher configures the event publisher for the service.
// It may be called at most once before the service begins handling requests.
func (s *MemoryService) SetEventPublisher(p MemoryEventPublisher) {
	s.eventPublisher = p
}

// SetInstitutionalStore wires the enterprise institutional admin store. Called
// once at startup under --enterprise; left nil otherwise (the institutional
// HTTP routes are requireEnterprise-gated so they never reach a nil store).
func (s *MemoryService) SetInstitutionalStore(store eememory.InstitutionalStore) {
	s.institutional = store
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

// SetConsentEventPruner wires the per-user/category consent prune store
// used by PruneUserConsentCategory. Called at startup under --enterprise;
// left nil otherwise (the consent-events route is requireEnterprise-gated).
func (s *MemoryService) SetConsentEventPruner(p memory.ConsentEventPruner) {
	s.consentEventPruner = p
}

// safeGoMaxInFlight bounds the total number of fire-and-forget
// side-effect goroutines (audit log, event publish, async embedding)
// in flight at any moment. Without a bound a degraded embedding
// provider sitting on the 30s timeout context lets a write burst
// pile up unbounded goroutines all racing for the same provider
// HTTP client and Postgres pool.
//
// 1024 is wide enough to absorb normal bursts yet narrow enough to
// surface backpressure (via dropped-side-effect metrics) before
// memory pressure hits the GC.
const safeGoMaxInFlight = 1024

// safeGoSem is the package-level semaphore. Token in flight = one
// goroutine spawned via safeGo currently running. A non-blocking
// acquire on the recall hot path means we drop side effects under
// burst rather than back-pressuring the caller.
var safeGoSem = make(chan struct{}, safeGoMaxInFlight)

// safeGo runs fn in a goroutine that recovers from panics. Used
// for fire-and-forget side effects (audit log, event publish, async
// embedding) where a panic in the side-effect path must not take
// down memory-api. Logs the panic with the supplied label so the
// caller is identifiable in the log stream.
//
// Bounded by safeGoSem: when the in-flight count is at capacity the
// side effect is dropped (with a warning + metric increment via
// recordSafeGoDrop) rather than queuing unbounded goroutines.
func (s *MemoryService) safeGo(label string, fn func()) {
	select {
	case safeGoSem <- struct{}{}:
	default:
		s.log.V(1).Info("safeGo dropped side effect",
			"label", label, "reason", "in_flight_at_capacity")
		recordSafeGoDrop(label)
		return
	}
	go func() {
		defer func() {
			<-safeGoSem
			if r := recover(); r != nil {
				s.log.Error(fmt.Errorf("panic: %v", r),
					"async side effect panicked", "label", label)
			}
		}()
		fn()
	}()
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
	s.safeGo("audit_log", func() { logger.LogEvent(context.Background(), entry) })
}

// policyContextKey distinguishes the per-request policy snapshot
// stash from other context values. Unexported to prevent external
// packages from poisoning the cache.
type policyContextKey struct{}

// policySnapshot wraps the resolved policy so the type assertion in
// policyFromContext can distinguish "snapshot present, nil policy"
// from "no snapshot at all".
type policySnapshot struct {
	policy *omniav1alpha1.MemoryPolicy
}

// defaultRelatedPerMemory caps the per-memory related[] list. Three keeps
// the recall payload lean while still letting the agent see the strongest
// graph neighbours (an identity entity's preferences, a workspace doc's
// related skills) it might want to update or supersede.
const defaultRelatedPerMemory = 3
