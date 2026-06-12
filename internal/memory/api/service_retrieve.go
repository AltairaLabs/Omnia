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

package api

import (
	"context"

	"github.com/altairalabs/omnia/internal/memory"
)

// retrieveOperationTag is the metadata tag emitted on multi-tier retrieval
// audit events. Kept as a constant so tests and grep lookups share a single
// source of truth.
const retrieveOperationTag = "retrieve_multi_tier"

// RetrieveMultiTier runs a multi-tier retrieval and emits one memory_accessed
// audit event. The store result is returned unchanged so the handler can
// forward tier annotations and scores to the client. Audit runs only on
// success so failed queries are not recorded as accesses.
//
// When a PolicyLoader is wired and the request doesn't already carry a
// Ranker, the loaded MemoryPolicy.spec.tierPrecedence is used to build
// one. Loader errors / nil policy fall through to the identity ranker
// so retrieval keeps working when policy resolution is degraded.
func (s *MemoryService) RetrieveMultiTier(ctx context.Context, req memory.MultiTierRequest) (*memory.MultiTierResult, error) {
	s.applyPolicyDefaults(ctx, &req)
	result, err := s.retrieveMultiTierInner(ctx, req)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: req.WorkspaceID,
		UserID:      req.UserID,
		Metadata: map[string]string{
			"operation": retrieveOperationTag,
		},
	})
	return result, nil
}

// applyPolicyDefaults loads the MemoryPolicy once (when a loader is wired) and
// fills in the per-tier ranker and recency half-life the caller didn't supply.
// Caller-supplied values win; loader errors / nil policy fall through to the
// identity ranker and default half-lives so retrieval keeps working when policy
// resolution is degraded.
func (s *MemoryService) applyPolicyDefaults(ctx context.Context, req *memory.MultiTierRequest) {
	if s.policyLoader == nil {
		return
	}
	needRanker := req.Ranker == nil
	needHalfLife := req.HalfLife == (memory.TierHalfLife{})
	if !needRanker && !needHalfLife {
		return
	}
	policy, err := s.policyLoader.Load(ctx)
	if err != nil {
		s.log.V(1).Info("policy load failed, using defaults", "error", err.Error())
	}
	if needRanker {
		req.Ranker = memory.NewTierRanker(policy)
	}
	if needHalfLife {
		req.HalfLife = memory.NewTierHalfLife(policy)
	}
}

// retrieveMultiTierInner routes to the hybrid (semantic + FTS RRF) store path
// when an embedder is configured and the query is non-empty, embedding the
// query first. An embed failure — or no embedder / empty query — falls back to
// FTS-only multi-tier so a transient embedder outage degrades recall quality
// rather than hard-failing the request. Mirrors searchMemoriesInner's policy
// for the flat search path; recall is too central to the agent loop to make
// brittle.
func (s *MemoryService) retrieveMultiTierInner(ctx context.Context, req memory.MultiTierRequest) (*memory.MultiTierResult, error) {
	if s.embeddingSvc == nil || req.Query == "" {
		return s.store.RetrieveMultiTier(ctx, req)
	}
	embeddings, err := s.embeddingSvc.Provider().Embed(ctx, []string{req.Query})
	if err != nil || len(embeddings) == 0 || len(embeddings[0]) == 0 {
		s.log.V(1).Info("hybrid multi-tier fallback to FTS",
			"reason", "embed_query_failed", "hasError", err != nil)
		return s.store.RetrieveMultiTier(ctx, req)
	}
	return s.store.RetrieveMultiTierHybrid(ctx, req, embeddings[0])
}
