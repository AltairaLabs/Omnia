package projection

import (
	"math"
	"testing"

	"gonum.org/v1/gonum/mat"
)

func matEqual(a, b mat.Matrix) bool {
	ar, ac := a.Dims()
	br, bc := b.Dims()
	if ar != br || ac != bc {
		return false
	}
	for i := 0; i < ar; i++ {
		for j := 0; j < ac; j++ {
			if math.Abs(a.At(i, j)-b.At(i, j)) > 1e-9 {
				return false
			}
		}
	}
	return true
}

func TestTFIDFLSA_ShapeAndDeterminism(t *testing.T) {
	docs := []string{
		"refund policy refund money back",
		"refund money back guarantee",
		"kubernetes pod restart crash loop",
		"kubernetes node restart crash",
	}
	m, err := TFIDFLSA(docs, 3)
	if err != nil {
		t.Fatal(err)
	}
	r, c := m.Dims()
	if r != 4 {
		t.Fatalf("rows = %d, want 4", r)
	}
	if c < 1 || c > 3 {
		t.Fatalf("cols = %d, want 1..3", c)
	}
	m2, _ := TFIDFLSA(docs, 3)
	if !matEqual(m, m2) {
		t.Error("TFIDFLSA not deterministic")
	}
}

func TestTFIDFLSA_EmptyDocsNoPanic(t *testing.T) {
	m, err := TFIDFLSA([]string{"", ""}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := m.Dims(); r != 2 {
		t.Fatalf("rows = %d, want 2", r)
	}
}
