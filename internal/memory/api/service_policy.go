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

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
)

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
	if v := s.policyFloat(ctx, "autoSupersedeAbove",
		(*omniav1alpha1.MemoryPolicy).DedupAutoSupersedeAbove,
		(*omniav1alpha1.MemoryPolicy).DedupAutoSupersedeAboveRaw); v > 0 {
		return v
	}
	return memory.DefaultAutoSupersedeSimilarity
}

// surfaceDuplicateThreshold resolves the cosine floor above which
// the server surfaces a near-duplicate as a `potential_duplicates`
// hint. Falls back to DefaultSurfaceDuplicateSimilarity (0.85).
func (s *MemoryService) surfaceDuplicateThreshold(ctx context.Context) float64 {
	if v := s.policyFloat(ctx, "surfaceDuplicatesAbove",
		(*omniav1alpha1.MemoryPolicy).DedupSurfaceDuplicatesAbove,
		(*omniav1alpha1.MemoryPolicy).DedupSurfaceDuplicatesAboveRaw); v > 0 {
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
// thresholds. It loads the policy once, calls the parsed-value
// extractor, and warns when the raw extractor reports the field was
// set on the CRD but parsed to 0 (operator misconfig vs. the silent
// "use the default" path).
func (s *MemoryService) policyFloat(ctx context.Context, fieldName string,
	getParsed func(*omniav1alpha1.MemoryPolicy) float64,
	getRaw func(*omniav1alpha1.MemoryPolicy) string,
) float64 {
	policy := s.loadPolicy(ctx)
	if policy == nil {
		return 0
	}
	v := getParsed(policy)
	if v == 0 {
		if raw := getRaw(policy); raw != "" {
			s.log.Info("memory policy float invalid; falling back to default",
				"field", fieldName,
				"value", raw,
				"policy", policy.Name)
		}
	}
	return v
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
//
// When the same Save calls multiple resolver helpers
// (autoSupersedeThreshold, surfaceDuplicateThreshold,
// duplicateCandidateLimit, embeddingDedupEnabled, requireAboutKinds…)
// each one would otherwise call this and read the cache pointer.
// Stash the resolved policy on the request context to avoid those
// repeated atomic.Loads AND to give the whole Save a single
// consistent snapshot — a TTL flip mid-Save can otherwise change
// thresholds between dedup and supersede.
func (s *MemoryService) loadPolicy(ctx context.Context) *omniav1alpha1.MemoryPolicy {
	if cached, ok := policyFromContext(ctx); ok {
		return cached
	}
	return s.fetchPolicy(ctx)
}

// fetchPolicy is the uncached path — reads from the loader, swallows
// transient errors. Exported via withPolicyContext for the request
// pre-load.
func (s *MemoryService) fetchPolicy(ctx context.Context) *omniav1alpha1.MemoryPolicy {
	if s.policyLoader == nil {
		return nil
	}
	policy, err := s.policyLoader.Load(ctx)
	if err != nil {
		return nil
	}
	return policy
}

// withPolicyContext returns a child context carrying the supplied
// policy snapshot. Resolver helpers called via that context skip the
// loader and read the snapshot directly. The snapshot may be nil
// (no policy bound or loader failure) — that case is also cached so
// helpers don't retry the loader within the same request.
func withPolicyContext(ctx context.Context, policy *omniav1alpha1.MemoryPolicy) context.Context {
	return context.WithValue(ctx, policyContextKey{}, policySnapshot{policy: policy})
}

// policyFromContext extracts a previously-stashed policy snapshot.
// The boolean is true when withPolicyContext was called on this
// context (even if the wrapped policy is nil), false when no
// snapshot is present and the caller should fetch fresh.
func policyFromContext(ctx context.Context) (*omniav1alpha1.MemoryPolicy, bool) {
	v, ok := ctx.Value(policyContextKey{}).(policySnapshot)
	if !ok {
		return nil, false
	}
	return v.policy, true
}
