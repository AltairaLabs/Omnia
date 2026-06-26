/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package projection

import (
	"fmt"

	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"
)

// PCAReduce projects rows of m onto its top-k principal components.
//
// k is clamped to the data's rank — at most min(nSamples, nFeatures). The
// number of principal components cannot exceed the sample count, so requesting
// more (e.g. PCADims=50 from a 6-memory workspace) used to slice gonum's
// nFeatures×min(nSamples,nFeatures) vector matrix out of bounds and panic
// ("mat: index out of range"), crashing the projection handler → dashboard 502
// (#1588). A degenerate set (<2 samples, or an empty matrix) can't be
// decomposed: a single sample is truncated to its first k features; an empty
// matrix is an error.
func PCAReduce(m *mat.Dense, k int) (*mat.Dense, error) {
	rows, cols := m.Dims()
	if rows == 0 || cols == 0 {
		return nil, fmt.Errorf("projection: empty input matrix (%dx%d)", rows, cols)
	}
	// Components can't exceed min(nSamples, nFeatures). Clamp to >= 1.
	if k > cols {
		k = cols
	}
	if k > rows {
		k = rows
	}
	if k < 1 {
		k = 1
	}
	// PCA needs >= 2 samples to have variance to decompose. For a single
	// sample just truncate to the first k feature columns — a valid (if
	// trivial) low-dimensional projection rather than a failure/panic.
	if rows < 2 {
		return firstKCols(m, k), nil
	}
	var pc stat.PC
	if ok := pc.PrincipalComponents(m, nil); !ok {
		return nil, fmt.Errorf("projection: PCA failed (%dx%d, k=%d)", rows, cols, k)
	}
	var vecs mat.Dense
	pc.VectorsTo(&vecs)
	// gonum returns the component vectors in a vr×vc matrix (vr == nFeatures,
	// vc == min(nSamples, nFeatures)). Never slice past vc, even if gonum
	// returns fewer vectors than expected.
	vr, vc := vecs.Dims()
	if k > vc {
		k = vc
	}
	var proj mat.Dense
	proj.Mul(m, vecs.Slice(0, vr, 0, k))
	out := mat.NewDense(rows, k, nil)
	out.Copy(&proj)
	return out, nil
}

// firstKCols returns the first k columns of m (k clamped to m's column count).
// Used for the degenerate single-sample case where PCA is undefined.
func firstKCols(m *mat.Dense, k int) *mat.Dense {
	rows, cols := m.Dims()
	if k > cols {
		k = cols
	}
	out := mat.NewDense(rows, k, nil)
	for i := 0; i < rows; i++ {
		for j := 0; j < k; j++ {
			out.Set(i, j, m.At(i, j))
		}
	}
	return out
}
