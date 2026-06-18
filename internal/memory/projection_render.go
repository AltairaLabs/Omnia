/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/memory/projection"
)

// ProjectionScopeKey is the stable key for a projection scope (workspace[:user][:agent]).
func ProjectionScopeKey(scope map[string]string) string {
	return fmt.Sprintf("%s|%s|%s",
		scope[ScopeWorkspaceID], scope[ScopeUserID], scope[ScopeAgentID])
}

// Render computes the 2D layout for scope (Procrustes-aligned to the stored
// previous layout) and persists it to memory_projections. Returns the result
// and the compute time. Shared by the endpoint's on-demand path and the
// pre-render worker.
func Render(ctx context.Context, ps ProjectionStore, scope map[string]string) (projection.Result, time.Time, error) {
	live, err := ps.ProjectionFingerprint(ctx, scope)
	if err != nil {
		return projection.Result{}, time.Time{}, err
	}
	inputs, err := ps.LoadProjectionInputs(ctx, scope)
	if err != nil {
		return projection.Result{}, time.Time{}, err
	}
	key := ProjectionScopeKey(scope)
	stored, err := ps.LoadProjection(ctx, key)
	if err != nil {
		return projection.Result{}, time.Time{}, err
	}
	var prev map[string][2]float64
	if stored != nil {
		prev = stored.Layout
	}
	res, err := projection.Project(toProjectionInputsM(inputs), prev, projection.Options{})
	if err != nil {
		return projection.Result{}, time.Time{}, err
	}
	points := make([]ProjectionPoint, len(res.Points))
	for i, p := range res.Points {
		points[i] = ProjectionPoint{EntityID: p.ID, X: p.X, Y: p.Y}
	}
	if err := ps.SaveProjection(ctx, key, scope[ScopeWorkspaceID], live, res.Model, res.Basis, points); err != nil {
		return projection.Result{}, time.Time{}, err
	}
	return res, time.Now().UTC(), nil
}

func toProjectionInputsM(in []ProjectionInput) []projection.Input {
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
