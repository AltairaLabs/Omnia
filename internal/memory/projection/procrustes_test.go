package projection

import (
	"math"
	"testing"

	"gonum.org/v1/gonum/mat"
)

func TestProcrustes_RecoversRotation(t *testing.T) {
	ref := map[string][2]float64{
		"a": {1, 0}, "b": {0, 1}, "c": {-1, 0}, "d": {0, -1},
	}
	// cur = ref rotated 90°; alignment should rotate it back to ref.
	ids := []string{"a", "b", "c", "d"}
	cur := mat.NewDense(4, 2, []float64{
		0, 1,
		-1, 0,
		0, -1,
		1, 0,
	})
	Align(cur, ids, ref)
	for i, id := range ids {
		wantX, wantY := ref[id][0], ref[id][1]
		if math.Abs(cur.At(i, 0)-wantX) > 1e-6 || math.Abs(cur.At(i, 1)-wantY) > 1e-6 {
			t.Errorf("%s = (%.3f,%.3f), want (%.1f,%.1f)", id, cur.At(i, 0), cur.At(i, 1), wantX, wantY)
		}
	}
}

func TestProcrustes_NoRefIsNoop(t *testing.T) {
	cur := mat.NewDense(2, 2, []float64{1, 2, 3, 4})
	Align(cur, []string{"x", "y"}, nil)
	if cur.At(0, 0) != 1 || cur.At(1, 1) != 4 {
		t.Error("with no reference, Align must not change coords")
	}
}

func TestProcrustes_FewerThanTwoSharedIsNoop(t *testing.T) {
	cur := mat.NewDense(2, 2, []float64{5, 6, 7, 8})
	Align(cur, []string{"x", "y"}, map[string][2]float64{"x": {0, 0}}) // only 1 shared
	if cur.At(0, 0) != 5 || cur.At(1, 1) != 8 {
		t.Error("with <2 shared points, Align must not change coords")
	}
}
