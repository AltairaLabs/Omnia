/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package projection holds the pure data types for the Memory Galaxy 2D layout
// (inputs, points, results, options) plus the Projector seam. The enterprise
// gonum/t-SNE algorithm (ee/pkg/memory/projection) provides the Projector
// implementation; core (internal/memory) consumes it via this interface so it
// no longer imports ee (#1669). These types carry no DB or HTTP dependency.
package projection

import "time"

// Input is one memory entity to project (most-recent observation).
type Input struct {
	EntityID   string
	Content    string    // most-recent observation content (lexical basis + preview)
	Embedding  []float32 // dense vector; nil when the entity has no embedding
	Tier       string    // institutional|agent|user|user_for_agent
	User       string    // virtual_user_id pseudonym; "" for institutional/agent
	Kind       string    // entity kind (the memory type)
	Category   string    // consent_category; "" if none
	Title      string
	Confidence float64
	ObservedAt time.Time
	ExpiresAt  *time.Time // entity TTL; nil if none
}

// Point is one projected memory in the 2D layout.
type Point struct {
	ID         string     `json:"id,omitempty"`
	X          float64    `json:"x"`
	Y          float64    `json:"y"`
	Tier       string     `json:"tier"`
	Type       string     `json:"type,omitempty"`    // entity kind (label sub-line + popup)
	User       string     `json:"user,omitempty"`    // pseudonym, for grouping/colour
	UserRef    string     `json:"userRef,omitempty"` // pseudonym, for the popup (== User)
	Category   string     `json:"category,omitempty"`
	Confidence float64    `json:"confidence"`
	Title      string     `json:"title,omitempty"`
	Preview    string     `json:"preview,omitempty"`
	ObservedAt time.Time  `json:"observedAt"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty"` // for age-fade toward expiry
	// Masked is true when consent policy stripped this point's identifying and
	// content fields server-side (sensitive category). A masked point is an
	// anonymous, non-interactive dot: only position/tier/confidence/timestamps.
	Masked bool `json:"masked,omitempty"`
}

// Result is the full projection outcome.
type Result struct {
	Model      string  `json:"model"` // "tsne" | "pca"
	Basis      string  `json:"basis"` // "dense" | "lexical"
	Total      int     `json:"total"`
	Unembedded int     `json:"unembedded"`
	Capped     bool    `json:"capped"`
	Points     []Point `json:"points"`
}

const (
	BasisDense   = "dense"
	BasisLexical = "lexical"
	// BasisUnknown labels a render whose basis was never determined (e.g. the
	// render failed before Project chose one). Used only for metric labels.
	BasisUnknown = "unknown"
	ModelTSNE    = "tsne"
	ModelPCA     = "pca"
)

// DefaultDenseThreshold is the embedding-coverage fraction at or above which
// the projector picks the dense (semantic) basis instead of lexical. Exported
// so the projection fingerprint can encode the same lexical↔dense eligibility
// and trigger a re-render when backfilled embeddings cross it.
const DefaultDenseThreshold = 0.7

// defaultRenderCap bounds how many points reach t-SNE. The t-SNE backend
// (danaugrs/go-tsne) is EXACT, not Barnes-Hut: it materialises n×n pairwise
// affinity matrices and runs O(n²) work per iteration, so cost and memory grow
// quadratically. At 8000 points a single render took minutes and ~1GB of
// matrices; 2000 keeps a background render to ~tens of seconds while still
// showing a dense, representative galaxy (applyCap keeps the most-recent,
// highest-confidence points). Raising this needs a Barnes-Hut/UMAP backend.
const defaultRenderCap = 2000

// Options tunes the pipeline.
type Options struct {
	Cap            int     // max points fed to t-SNE (default 8000)
	DenseThreshold float64 // dense-coverage fraction to pick dense basis (default 0.7)
	PCADims        int     // dense PCA target dim (default 50)
	LSADims        int     // lexical LSA target dim (default 50)
	PreviewChars   int     // preview length (default 120)
	TinySet        int     // below this point count, skip t-SNE, use reduced 2D (default 30)
}

// WithDefaults returns a copy of o with zero-valued fields filled from the
// pipeline defaults. Shared by the enterprise Project algorithm and the pure
// FromStored refresh so both apply identical defaulting.
func (o Options) WithDefaults() Options {
	if o.Cap == 0 {
		o.Cap = defaultRenderCap
	}
	if o.DenseThreshold == 0 {
		o.DenseThreshold = DefaultDenseThreshold
	}
	if o.PCADims == 0 {
		o.PCADims = 50
	}
	if o.LSADims == 0 {
		o.LSADims = 50
	}
	if o.PreviewChars == 0 {
		o.PreviewChars = 120
	}
	if o.TinySet == 0 {
		o.TinySet = 30
	}
	return o
}

// Projector computes a 2-D projection. The enterprise implementation
// (ee/pkg/memory/projection) provides the gonum/t-SNE algorithm; core consumes
// it via this interface so internal/memory does not import ee (#1669).
type Projector interface {
	Project(inputs []Input, prev map[string][2]float64, opts Options) (Result, error)
}

// FromStored builds a Result from previously-stored coords, refreshing point
// metadata/preview from the current inputs. Inputs without a stored coord are
// dropped (they'll be picked up on the next recompute).
func FromStored(inputs []Input, stored map[string][2]float64, opts Options) Result {
	opts = opts.WithDefaults()
	res := Result{Points: []Point{}}
	for _, in := range inputs {
		xy, ok := stored[in.EntityID]
		if !ok {
			continue
		}
		res.Points = append(res.Points, Point{
			ID: in.EntityID, X: xy[0], Y: xy[1], Tier: in.Tier, Type: in.Kind,
			User: in.User, UserRef: in.User, Category: in.Category,
			Confidence: in.Confidence, Title: in.Title,
			Preview:    truncateRunes(in.Content, opts.PreviewChars),
			ObservedAt: in.ObservedAt, ExpiresAt: in.ExpiresAt,
		})
	}
	res.Total = len(res.Points)
	return res
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
