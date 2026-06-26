/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package projection

import (
	"testing"

	"gonum.org/v1/gonum/mat"
)

func TestPCAReduce_Shape(t *testing.T) {
	data := make([]float64, 6*8)
	for i := range data {
		data[i] = float64((i*7)%13) / 13.0
	}
	in := mat.NewDense(6, 8, data)
	out, err := PCAReduce(in, 3)
	if err != nil {
		t.Fatal(err)
	}
	r, c := out.Dims()
	if r != 6 || c != 3 {
		t.Fatalf("dims = %dx%d, want 6x3", r, c)
	}
}

func TestPCAReduce_ClampsToInputDim(t *testing.T) {
	in := mat.NewDense(4, 2, []float64{1, 0, 0, 1, 1, 1, 0, 0})
	out, err := PCAReduce(in, 50) // request more than available
	if err != nil {
		t.Fatal(err)
	}
	if _, c := out.Dims(); c > 2 {
		t.Fatalf("cols = %d, must not exceed input dim 2", c)
	}
}

// TestPCAReduce_MoreComponentsThanSamples is the #1588 regression: few samples,
// many features, k far above the sample count. Principal components can't exceed
// min(nSamples, nFeatures); the old code only clamped to nFeatures, so it sliced
// gonum's nFeatures×nSamples vector matrix to 50 columns and panicked. This is
// the exact shape the existing ClampsToInputDim test missed (it used rows>cols).
func TestPCAReduce_MoreComponentsThanSamples(t *testing.T) {
	const rows, cols = 6, 1536 // 6 memories, 1536-dim embeddings
	data := make([]float64, rows*cols)
	for i := range data {
		data[i] = float64((i*7)%97) / 97.0
	}
	in := mat.NewDense(rows, cols, data)

	out, err := PCAReduce(in, 50) // 50 components from 6 samples — must not panic
	if err != nil {
		t.Fatal(err)
	}
	r, c := out.Dims()
	if r != rows {
		t.Fatalf("rows = %d, want %d", r, rows)
	}
	if c > rows {
		t.Fatalf("cols = %d, must not exceed sample count %d", c, rows)
	}
	if c < 1 {
		t.Fatalf("cols = %d, want >= 1", c)
	}
}

// TestPCAReduce_DegenerateSets covers the tiny/edge inputs a new workspace hits:
// a single sample (no variance to decompose) and a zero-row matrix. Neither may
// panic; a single sample returns a valid low-dim projection.
func TestPCAReduce_DegenerateSets(t *testing.T) {
	t.Run("single sample", func(t *testing.T) {
		in := mat.NewDense(1, 64, nil)
		for j := 0; j < 64; j++ {
			in.Set(0, j, float64(j)/64.0)
		}
		out, err := PCAReduce(in, 50)
		if err != nil {
			t.Fatal(err)
		}
		if r, c := out.Dims(); r != 1 || c < 1 {
			t.Fatalf("dims = %dx%d, want 1 row, >=1 col", r, c)
		}
	})
	t.Run("empty matrix", func(t *testing.T) {
		// Zero-value Dense reports 0x0 dims; PCAReduce must reject it cleanly
		// rather than panic. (The pipeline short-circuits empty scopes before
		// here, but the guard keeps PCAReduce safe in isolation.)
		var in mat.Dense
		if _, err := PCAReduce(&in, 50); err == nil {
			t.Fatal("expected an error for an empty matrix, got nil")
		}
	})
}
