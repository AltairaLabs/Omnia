/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package projection

import (
	"math"
	"testing"

	"gonum.org/v1/gonum/mat"
)

func TestTSNE2D_Shape(t *testing.T) {
	rows, cols := 40, 5
	data := make([]float64, rows*cols)
	for i := 0; i < rows; i++ {
		base := 0.0
		if i >= 20 {
			base = 10.0
		}
		for j := 0; j < cols; j++ {
			data[i*cols+j] = base + float64((i*3+j)%5)
		}
	}
	in := mat.NewDense(rows, cols, data)
	out := TSNE2D(in)
	r, c := out.Dims()
	if r != rows || c != 2 {
		t.Fatalf("dims = %dx%d, want %dx2", r, c, rows)
	}
	for i := 0; i < r; i++ {
		if math.IsNaN(out.At(i, 0)) || math.IsNaN(out.At(i, 1)) {
			t.Fatalf("NaN coord at row %d", i)
		}
	}
}
