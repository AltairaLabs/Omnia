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
