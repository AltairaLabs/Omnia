/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package projection

import (
	"sort"

	"gonum.org/v1/gonum/mat"

	coreproj "github.com/altairalabs/omnia/internal/memory/projection"
)

// GonumProjector implements coreproj.Projector using the gonum/t-SNE algorithm.
type GonumProjector struct{}

// Project runs the enterprise t-SNE pipeline (see Project).
func (GonumProjector) Project(
	inputs []coreproj.Input, prev map[string][2]float64, opts coreproj.Options,
) (coreproj.Result, error) {
	return Project(inputs, prev, opts)
}

// Project runs the full pure pipeline. prev is the previous layout (entity id →
// [x,y]) to Procrustes-align against; pass nil for the first compute.
func Project(inputs []coreproj.Input, prev map[string][2]float64, opts coreproj.Options) (coreproj.Result, error) {
	opts = opts.WithDefaults()
	basis := chooseBasis(inputs, opts.DenseThreshold)
	used, unembedded := selectInputs(inputs, basis)
	used, capped := applyCap(used, opts.Cap)

	res := coreproj.Result{
		Basis: basis, Total: len(used), Unembedded: unembedded, Capped: capped, Model: coreproj.ModelTSNE,
	}
	if len(used) == 0 {
		res.Points = []coreproj.Point{}
		return res, nil
	}

	coords, model, err := layout(used, basis, opts)
	if err != nil {
		return coreproj.Result{}, err
	}
	res.Model = model

	ids := make([]string, len(used))
	for i := range used {
		ids[i] = used[i].EntityID
	}
	Align(coords, ids, prev)
	normalize(coords)
	res.Points = assemble(used, coords, opts.PreviewChars)
	return res, nil
}

// layout reduces the inputs to 2D coordinates, returning the model used.
func layout(used []coreproj.Input, basis string, opts coreproj.Options) (*mat.Dense, string, error) {
	reduced, err := vectorize(used, basis, opts)
	if err != nil {
		return nil, "", err
	}
	if len(used) < opts.TinySet {
		return first2Cols(reduced), coreproj.ModelPCA, nil
	}
	return TSNE2D(reduced), coreproj.ModelTSNE, nil
}

func chooseBasis(inputs []coreproj.Input, threshold float64) string {
	if len(inputs) == 0 {
		return coreproj.BasisLexical
	}
	var withEmb int
	for _, in := range inputs {
		if len(in.Embedding) > 0 {
			withEmb++
		}
	}
	if float64(withEmb)/float64(len(inputs)) >= threshold {
		return coreproj.BasisDense
	}
	return coreproj.BasisLexical
}

// selectInputs returns the inputs that will be projected and the count skipped.
// Dense: only embedded entities (rest counted as unembedded). Lexical: all.
func selectInputs(inputs []coreproj.Input, basis string) (used []coreproj.Input, unembedded int) {
	if basis == coreproj.BasisLexical {
		return inputs, 0
	}
	for _, in := range inputs {
		if len(in.Embedding) > 0 {
			used = append(used, in)
		} else {
			unembedded++
		}
	}
	return used, unembedded
}

func applyCap(in []coreproj.Input, capN int) ([]coreproj.Input, bool) {
	if len(in) <= capN {
		return in, false
	}
	sorted := append([]coreproj.Input(nil), in...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].ObservedAt.Equal(sorted[j].ObservedAt) {
			return sorted[i].ObservedAt.After(sorted[j].ObservedAt)
		}
		return sorted[i].Confidence > sorted[j].Confidence
	})
	return sorted[:capN], true
}

// vectorize produces the reduced (≤PCADims/LSADims) matrix for the basis.
func vectorize(used []coreproj.Input, basis string, opts coreproj.Options) (*mat.Dense, error) {
	if basis == coreproj.BasisLexical {
		docs := make([]string, len(used))
		for i := range used {
			docs[i] = used[i].Content
		}
		return TFIDFLSA(docs, opts.LSADims)
	}
	dim := len(used[0].Embedding)
	m := mat.NewDense(len(used), dim, nil)
	for i := range used {
		for j, v := range used[i].Embedding {
			m.Set(i, j, float64(v))
		}
	}
	return PCAReduce(m, opts.PCADims)
}

func first2Cols(m *mat.Dense) *mat.Dense {
	r, c := m.Dims()
	out := mat.NewDense(r, 2, nil)
	for i := 0; i < r; i++ {
		out.Set(i, 0, m.At(i, 0))
		if c > 1 {
			out.Set(i, 1, m.At(i, 1))
		}
	}
	return out
}

// normalize scales coords into ~[-1,1] preserving aspect ratio (single scale).
func normalize(c *mat.Dense) {
	n, _ := c.Dims()
	var maxAbs float64
	for i := 0; i < n; i++ {
		for _, v := range []float64{c.At(i, 0), c.At(i, 1)} {
			if a := absf(v); a > maxAbs {
				maxAbs = a
			}
		}
	}
	if maxAbs == 0 {
		return
	}
	for i := 0; i < n; i++ {
		c.Set(i, 0, c.At(i, 0)/maxAbs)
		c.Set(i, 1, c.At(i, 1)/maxAbs)
	}
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func assemble(used []coreproj.Input, coords *mat.Dense, previewChars int) []coreproj.Point {
	pts := make([]coreproj.Point, len(used))
	for i, in := range used {
		pts[i] = coreproj.Point{
			ID: in.EntityID, X: coords.At(i, 0), Y: coords.At(i, 1),
			Tier: in.Tier, Type: in.Kind, User: in.User, UserRef: in.User,
			Category: in.Category, Confidence: in.Confidence, Title: in.Title,
			Preview:    truncateRunes(in.Content, previewChars),
			ObservedAt: in.ObservedAt, ExpiresAt: in.ExpiresAt,
		}
	}
	return pts
}

// truncateRunes mirrors the core helper (kept private here for assemble, which
// stays in ee with the gonum pipeline). See internal/memory/projection.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
