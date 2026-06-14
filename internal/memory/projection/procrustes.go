package projection

import (
	"gonum.org/v1/gonum/mat"
)

// Align rotates/reflects cur in place so that points whose ids appear in ref
// best match their reference positions (orthogonal Procrustes on the shared
// set). If fewer than 2 shared points exist, cur is left unchanged.
func Align(cur *mat.Dense, ids []string, ref map[string][2]float64) {
	if ref == nil {
		return
	}
	a, b := sharedPoints(cur, ids, ref)
	if len(a) < 2 {
		return
	}
	ca, cb := centroid(a), centroid(b)
	r := procrustesRotation(a, b, ca, cb)
	applyTransform(cur, r, ca, cb)
}

// sharedPoints returns the current and reference coordinates for ids present
// in ref, in matching order.
func sharedPoints(cur *mat.Dense, ids []string, ref map[string][2]float64) (a, b [][2]float64) {
	for i, id := range ids {
		if p, ok := ref[id]; ok {
			a = append(a, [2]float64{cur.At(i, 0), cur.At(i, 1)})
			b = append(b, p)
		}
	}
	return a, b
}

// procrustesRotation returns the 2x2 rotation/reflection R minimizing
// ||R(a-ca) - (b-cb)|| via the SVD of the cross-covariance H = (a-ca)^T (b-cb).
func procrustesRotation(a, b [][2]float64, ca, cb [2]float64) *mat.Dense {
	h := mat.NewDense(2, 2, nil)
	for i := range a {
		ax, ay := a[i][0]-ca[0], a[i][1]-ca[1]
		bx, by := b[i][0]-cb[0], b[i][1]-cb[1]
		h.Set(0, 0, h.At(0, 0)+ax*bx)
		h.Set(0, 1, h.At(0, 1)+ax*by)
		h.Set(1, 0, h.At(1, 0)+ay*bx)
		h.Set(1, 1, h.At(1, 1)+ay*by)
	}
	var svd mat.SVD
	svd.Factorize(h, mat.SVDFull)
	var u, v mat.Dense
	svd.UTo(&u)
	svd.VTo(&v)
	var r mat.Dense
	r.Mul(&v, u.T())
	return &r
}

// applyTransform rotates every row of cur about ca by r, then shifts to cb.
func applyTransform(cur, r *mat.Dense, ca, cb [2]float64) {
	n, _ := cur.Dims()
	for i := 0; i < n; i++ {
		x, y := cur.At(i, 0)-ca[0], cur.At(i, 1)-ca[1]
		cur.Set(i, 0, r.At(0, 0)*x+r.At(0, 1)*y+cb[0])
		cur.Set(i, 1, r.At(1, 0)*x+r.At(1, 1)*y+cb[1])
	}
}

func centroid(p [][2]float64) [2]float64 {
	var sx, sy float64
	for _, q := range p {
		sx += q[0]
		sy += q[1]
	}
	n := float64(len(p))
	return [2]float64{sx / n, sy / n}
}
