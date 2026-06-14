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

// ProjectionResult is the service-level result: the pure projection.Result plus
// embedding metadata and the compute timestamp.
type ProjectionResult struct {
	projection.Result
	// ProjectionInput names the representation projected: "embedding" (dense) or
	// "tfidf" (lexical) — the semantic-vs-lexical hint for the UI.
	ProjectionInput string    `json:"projectionInput,omitempty"`
	EmbeddingModel  string    `json:"embeddingModel,omitempty"`
	EmbeddingDim    int       `json:"embeddingDim,omitempty"`
	ComputedAt      time.Time `json:"computedAt"`
}

func scopeKey(scope map[string]string) string {
	return fmt.Sprintf("%s|%s|%s",
		scope[memory.ScopeWorkspaceID], scope[memory.ScopeUserID], scope[memory.ScopeAgentID])
}

// Project builds (or serves a cached) 2D Memory Galaxy layout for the scope.
func (s *MemoryService) Project(ctx context.Context, scope map[string]string) (ProjectionResult, error) {
	ps, ok := s.store.(memory.ProjectionStore)
	if !ok {
		return ProjectionResult{}, fmt.Errorf("memory: store does not support projection")
	}
	key := scopeKey(scope)
	live, err := ps.ProjectionFingerprint(ctx, scope)
	if err != nil {
		return ProjectionResult{}, err
	}
	inputs, err := ps.LoadProjectionInputs(ctx, scope)
	if err != nil {
		return ProjectionResult{}, err
	}
	stored, err := ps.LoadProjection(ctx, key)
	if err != nil {
		return ProjectionResult{}, err
	}

	res, computedAt, err := s.computeOrServe(ctx, ps, key, scope, live, inputs, stored)
	if err != nil {
		return ProjectionResult{}, err
	}
	out := ProjectionResult{Result: res, ComputedAt: computedAt}
	s.enrichEmbeddingMeta(&out)
	return out, nil
}

// computeOrServe returns the stored layout when the fingerprint still matches,
// otherwise recomputes (Procrustes-aligned to the stored layout) and persists.
func (s *MemoryService) computeOrServe(
	ctx context.Context, ps memory.ProjectionStore, key string, scope map[string]string,
	live string, inputs []memory.ProjectionInput, stored *memory.StoredProjection,
) (projection.Result, time.Time, error) {
	pInputs := toProjectionInputs(inputs)
	var prev map[string][2]float64
	if stored != nil {
		prev = stored.Layout
	}
	if stored != nil && stored.Fingerprint == live {
		res := projection.FromStored(pInputs, stored.Layout, projection.Options{})
		res.Model, res.Basis = stored.Model, stored.Basis
		return res, stored.ComputedAt, nil
	}
	res, err := projection.Project(pInputs, prev, projection.Options{})
	if err != nil {
		return projection.Result{}, time.Time{}, err
	}
	points := make([]memory.ProjectionPoint, len(res.Points))
	for i, p := range res.Points {
		points[i] = memory.ProjectionPoint{EntityID: p.ID, X: p.X, Y: p.Y}
	}
	if err := ps.SaveProjection(ctx, key, scope[memory.ScopeWorkspaceID], live, res.Model, res.Basis, points); err != nil {
		return projection.Result{}, time.Time{}, err
	}
	return res, time.Now().UTC(), nil
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
