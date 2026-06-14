/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"fmt"
	"time"

	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/projection"
)

// onDemandProjectionThreshold is the entity-count gate above which the endpoint
// does NOT compute synchronously: large cold scopes return status:"pending" and
// the pre-render worker renders them in the background.
const onDemandProjectionThreshold = 500

// ProjectionResult is the service-level result: the pure projection.Result plus
// embedding metadata, render status, and the compute timestamp.
type ProjectionResult struct {
	projection.Result
	// Status is "ready" (layout served) or "pending" (large cold scope not
	// pre-rendered yet; the worker is on it — poll until ready).
	Status string `json:"status"`
	// ProjectionInput names the representation projected: "embedding" (dense) or
	// "tfidf" (lexical) — the semantic-vs-lexical hint for the UI.
	ProjectionInput string    `json:"projectionInput,omitempty"`
	EmbeddingModel  string    `json:"embeddingModel,omitempty"`
	EmbeddingDim    int       `json:"embeddingDim,omitempty"`
	ComputedAt      time.Time `json:"computedAt"`
}

// Project serves the stored 2D Memory Galaxy layout, computes small scopes
// on-demand, or returns status:"pending" for large cold scopes (the pre-render
// worker renders those in the background).
func (s *MemoryService) Project(ctx context.Context, scope map[string]string) (ProjectionResult, error) {
	ps, ok := s.store.(memory.ProjectionStore)
	if !ok {
		return ProjectionResult{}, fmt.Errorf("memory: store does not support projection")
	}
	key := memory.ProjectionScopeKey(scope)
	live, err := ps.ProjectionFingerprint(ctx, scope)
	if err != nil {
		return ProjectionResult{}, err
	}
	stored, err := ps.LoadProjection(ctx, key)
	if err != nil {
		return ProjectionResult{}, err
	}
	// Fresh stored layout → serve it.
	if stored != nil && stored.Fingerprint == live {
		return s.serveStored(ctx, ps, scope, stored)
	}
	// Cold/stale: small → compute now; large → pending (worker will render).
	if projectionCount(live) > onDemandProjectionThreshold {
		return ProjectionResult{Status: "pending", Result: projection.Result{
			Total: projectionCount(live), Points: []projection.Point{}}}, nil
	}
	res, computedAt, err := memory.Render(ctx, ps, scope)
	if err != nil {
		return ProjectionResult{}, err
	}
	out := ProjectionResult{Result: res, Status: "ready", ComputedAt: computedAt}
	s.enrichEmbeddingMeta(&out)
	return out, nil
}

// serveStored returns the cached layout refreshed with current metadata.
func (s *MemoryService) serveStored(ctx context.Context, ps memory.ProjectionStore,
	scope map[string]string, stored *memory.StoredProjection) (ProjectionResult, error) {
	inputs, err := ps.LoadProjectionInputs(ctx, scope)
	if err != nil {
		return ProjectionResult{}, err
	}
	res := projection.FromStored(toProjectionInputs(inputs), stored.Layout, projection.Options{})
	res.Model, res.Basis = stored.Model, stored.Basis
	out := ProjectionResult{Result: res, Status: "ready", ComputedAt: stored.ComputedAt}
	s.enrichEmbeddingMeta(&out)
	return out, nil
}

// projectionCount parses the entity count from a "<count>:<nanos>" fingerprint;
// "" (no memories) → 0.
func projectionCount(fingerprint string) int {
	if fingerprint == "" {
		return 0
	}
	var count int
	_, _ = fmt.Sscanf(fingerprint, "%d:", &count)
	return count
}

// enrichEmbeddingMeta sets the representation hint + embedding model/dim per basis.
func (s *MemoryService) enrichEmbeddingMeta(out *ProjectionResult) {
	switch out.Basis {
	case projection.BasisDense:
		out.ProjectionInput = "embedding"
		if s.embeddingSvc != nil {
			out.EmbeddingModel = s.embeddingSvc.ModelName()
			out.EmbeddingDim = s.embeddingSvc.Provider().Dimensions()
		}
	case projection.BasisLexical:
		out.ProjectionInput = "tfidf"
		out.EmbeddingModel = "tfidf+lsa"
	}
}

func toProjectionInputs(in []memory.ProjectionInput) []projection.Input {
	out := make([]projection.Input, len(in))
	for i, x := range in {
		out[i] = projection.Input{
			EntityID: x.EntityID, Content: x.Content, Embedding: x.Embedding,
			Tier: x.Tier, User: x.User, Kind: x.Kind, Category: x.Category, Title: x.Title,
			Confidence: x.Confidence, ObservedAt: x.ObservedAt, ExpiresAt: x.ExpiresAt,
		}
	}
	return out
}
