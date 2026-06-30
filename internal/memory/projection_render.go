/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	coreproj "github.com/altairalabs/omnia/internal/memory/projection"
)

// ErrProjectionUnavailable is returned by Render when no projection algorithm
// is wired (OSS — the t-SNE projector is an enterprise feature injected via
// SetProjector). Callers map it to a clear error rather than a layout.
var ErrProjectionUnavailable = errors.New("memory: projection unavailable (enterprise feature not enabled)")

// ProjectorProvider is implemented by stores carrying an injected projection
// algorithm (the enterprise t-SNE Projector). Render type-asserts the
// ProjectionStore to it; a store without one yields a nil projector and Render
// fails with ErrProjectionUnavailable. Mirrors the WorkspaceInvalidator
// optional-capability seam (see ee_seam.go).
type ProjectorProvider interface {
	Projector() coreproj.Projector
}

// projectorFor extracts the injected projector if the store exposes one.
func projectorFor(ps ProjectionStore) coreproj.Projector {
	if pp, ok := ps.(ProjectorProvider); ok {
		return pp.Projector()
	}
	return nil
}

// ProjectionScopeKey is the stable key for a projection scope (workspace[:user][:agent]).
func ProjectionScopeKey(scope map[string]string) string {
	return fmt.Sprintf("%s|%s|%s",
		scope[ScopeWorkspaceID], scope[ScopeUserID], scope[ScopeAgentID])
}

// Render computes the 2D layout for scope (Procrustes-aligned to the stored
// previous layout) and persists it to memory_projections. Returns the result
// and the compute time. Shared by the endpoint's on-demand path and the
// pre-render worker.
func Render(ctx context.Context, ps ProjectionStore, scope map[string]string) (coreproj.Result, time.Time, error) {
	projector := projectorFor(ps)
	if projector == nil {
		return coreproj.Result{}, time.Time{}, ErrProjectionUnavailable
	}
	live, err := ps.ProjectionFingerprint(ctx, scope)
	if err != nil {
		return coreproj.Result{}, time.Time{}, err
	}
	inputs, err := ps.LoadProjectionInputs(ctx, scope)
	if err != nil {
		return coreproj.Result{}, time.Time{}, err
	}
	key := ProjectionScopeKey(scope)
	stored, err := ps.LoadProjection(ctx, key)
	if err != nil {
		return coreproj.Result{}, time.Time{}, err
	}
	var prev map[string][2]float64
	if stored != nil {
		prev = stored.Layout
	}
	res, err := projector.Project(toProjectionInputsM(inputs), prev, coreproj.Options{})
	if err != nil {
		return coreproj.Result{}, time.Time{}, err
	}
	points := make([]ProjectionPoint, len(res.Points))
	for i, p := range res.Points {
		points[i] = ProjectionPoint{EntityID: p.ID, X: p.X, Y: p.Y}
	}
	if err := ps.SaveProjection(ctx, key, scope[ScopeWorkspaceID], live, res.Model, res.Basis, points); err != nil {
		return coreproj.Result{}, time.Time{}, err
	}
	return res, time.Now().UTC(), nil
}

func toProjectionInputsM(in []ProjectionInput) []coreproj.Input {
	out := make([]coreproj.Input, len(in))
	for i, x := range in {
		out[i] = coreproj.Input{
			EntityID: x.EntityID, Content: x.Content, Embedding: x.Embedding,
			Tier: x.Tier, User: x.User, Kind: x.Kind, Category: x.Category, Title: x.Title,
			Confidence: x.Confidence, ObservedAt: x.ObservedAt, ExpiresAt: x.ExpiresAt,
		}
	}
	return out
}
