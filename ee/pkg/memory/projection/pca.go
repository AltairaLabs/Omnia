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
// k is clamped to the input dimensionality.
func PCAReduce(m *mat.Dense, k int) (*mat.Dense, error) {
	rows, cols := m.Dims()
	if k > cols {
		k = cols
	}
	var pc stat.PC
	if ok := pc.PrincipalComponents(m, nil); !ok {
		return nil, fmt.Errorf("projection: PCA failed")
	}
	var vecs mat.Dense
	pc.VectorsTo(&vecs)
	var proj mat.Dense
	proj.Mul(m, vecs.Slice(0, cols, 0, k))
	out := mat.NewDense(rows, k, nil)
	out.Copy(&proj)
	return out, nil
}
