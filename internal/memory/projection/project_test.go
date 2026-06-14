package projection

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func denseInputs(n int) []Input {
	out := make([]Input, n)
	for i := 0; i < n; i++ {
		emb := []float32{float32(i % 5), float32((i * 3) % 7), 1, 0, float32(i % 2)}
		out[i] = Input{
			EntityID:  fmt.Sprintf("e%04d", i),
			Embedding: emb, Tier: "user", User: "u1",
			Content: strings.Repeat("word ", 40), Confidence: 0.5,
			ObservedAt: time.Unix(int64(i), 0).UTC(),
		}
	}
	return out
}

func defaultOpts() Options {
	return Options{Cap: 8000, DenseThreshold: 0.7, PCADims: 30, LSADims: 50, PreviewChars: 120}
}

func TestProject_DenseBasisAndBounds(t *testing.T) {
	res, err := Project(denseInputs(40), nil, defaultOpts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Basis != BasisDense {
		t.Errorf("basis = %s, want dense", res.Basis)
	}
	if res.Total != 40 || len(res.Points) != 40 {
		t.Errorf("total/points = %d/%d, want 40/40", res.Total, len(res.Points))
	}
	for _, p := range res.Points {
		if p.X < -1.0001 || p.X > 1.0001 || p.Y < -1.0001 || p.Y > 1.0001 {
			t.Errorf("point out of [-1,1]: (%f,%f)", p.X, p.Y)
		}
	}
}

func TestProject_LexicalWhenLowCoverage(t *testing.T) {
	in := denseInputs(40)
	for i := range in {
		if i%2 == 0 {
			in[i].Embedding = nil // 50% < 0.7 → lexical
		}
	}
	res, err := Project(in, nil, defaultOpts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Basis != BasisLexical {
		t.Errorf("basis = %s, want lexical", res.Basis)
	}
	if res.Unembedded != 0 {
		t.Errorf("lexical projects all → unembedded 0, got %d", res.Unembedded)
	}
}

func TestProject_DenseSkipsUnembedded(t *testing.T) {
	in := denseInputs(40)
	in[0].Embedding = nil // 39/40 ≈ 0.975 ≥ 0.7 → dense, skip the 1
	res, err := Project(in, nil, defaultOpts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Basis != BasisDense {
		t.Fatalf("basis = %s, want dense", res.Basis)
	}
	if res.Unembedded != 1 || res.Total != 39 {
		t.Errorf("unembedded/total = %d/%d, want 1/39", res.Unembedded, res.Total)
	}
}

func TestProject_PreviewTruncated(t *testing.T) {
	in := denseInputs(35)
	in[0].Content = strings.Repeat("x", 500)
	res, err := Project(in, nil, defaultOpts())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Points {
		if len([]rune(p.Preview)) > 120 {
			t.Errorf("preview too long: %d", len([]rune(p.Preview)))
		}
	}
}

func TestProject_TinySetUsesPCAModel(t *testing.T) {
	opts := defaultOpts()
	opts.TinySet = 30
	res, err := Project(denseInputs(10), nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if res.Model != ModelPCA {
		t.Errorf("model = %s, want pca for tiny set", res.Model)
	}
}

func TestProject_Caps(t *testing.T) {
	opts := defaultOpts()
	opts.Cap = 20
	res, err := Project(denseInputs(50), nil, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Capped || len(res.Points) != 20 {
		t.Errorf("capped=%v points=%d, want capped + 20", res.Capped, len(res.Points))
	}
}

func TestProject_EmptyInputs(t *testing.T) {
	res, err := Project(nil, nil, defaultOpts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 0 || res.Points == nil || len(res.Points) != 0 {
		t.Errorf("empty: total=%d points=%v", res.Total, res.Points)
	}
}

func TestFromStored_ReusesCoordsRefreshesMeta(t *testing.T) {
	inputs := denseInputs(4)
	stored := map[string][2]float64{
		inputs[0].EntityID: {0.1, 0.2},
		inputs[1].EntityID: {0.3, 0.4},
		inputs[2].EntityID: {0.5, 0.6},
		// inputs[3] has no stored coord → dropped
	}
	res := FromStored(inputs, stored, defaultOpts())
	if res.Total != 3 || len(res.Points) != 3 {
		t.Fatalf("total/points = %d/%d, want 3/3", res.Total, len(res.Points))
	}
	if res.Points[0].X != 0.1 || res.Points[0].Y != 0.2 {
		t.Errorf("coords not reused: %v", res.Points[0])
	}
	if res.Points[0].Preview == "" {
		t.Error("preview should be refreshed from input content")
	}
}
