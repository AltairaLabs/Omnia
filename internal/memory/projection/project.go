package projection

import (
	"sort"

	"gonum.org/v1/gonum/mat"
)

// DefaultDenseThreshold is the embedding-coverage fraction at or above which
// the projector picks the dense (semantic) basis instead of lexical. Exported
// so the projection fingerprint can encode the same lexical↔dense eligibility
// and trigger a re-render when backfilled embeddings cross it.
const DefaultDenseThreshold = 0.7

// defaultRenderCap bounds how many points reach t-SNE. The t-SNE backend
// (danaugrs/go-tsne) is EXACT, not Barnes-Hut: it materialises n×n pairwise
// affinity matrices and runs O(n²) work per iteration, so cost and memory grow
// quadratically. At 8000 points a single render took minutes and ~1GB of
// matrices; 2000 keeps a background render to ~tens of seconds while still
// showing a dense, representative galaxy (applyCap keeps the most-recent,
// highest-confidence points). Raising this needs a Barnes-Hut/UMAP backend.
const defaultRenderCap = 2000

// Options tunes the pipeline.
type Options struct {
	Cap            int     // max points fed to t-SNE (default 8000)
	DenseThreshold float64 // dense-coverage fraction to pick dense basis (default 0.7)
	PCADims        int     // dense PCA target dim (default 50)
	LSADims        int     // lexical LSA target dim (default 50)
	PreviewChars   int     // preview length (default 120)
	TinySet        int     // below this point count, skip t-SNE, use reduced 2D (default 30)
}

func (o Options) withDefaults() Options {
	if o.Cap == 0 {
		o.Cap = defaultRenderCap
	}
	if o.DenseThreshold == 0 {
		o.DenseThreshold = DefaultDenseThreshold
	}
	if o.PCADims == 0 {
		o.PCADims = 50
	}
	if o.LSADims == 0 {
		o.LSADims = 50
	}
	if o.PreviewChars == 0 {
		o.PreviewChars = 120
	}
	if o.TinySet == 0 {
		o.TinySet = 30
	}
	return o
}

// Project runs the full pure pipeline. prev is the previous layout (entity id →
// [x,y]) to Procrustes-align against; pass nil for the first compute.
func Project(inputs []Input, prev map[string][2]float64, opts Options) (Result, error) {
	opts = opts.withDefaults()
	basis := chooseBasis(inputs, opts.DenseThreshold)
	used, unembedded := selectInputs(inputs, basis)
	used, capped := applyCap(used, opts.Cap)

	res := Result{Basis: basis, Total: len(used), Unembedded: unembedded, Capped: capped, Model: ModelTSNE}
	if len(used) == 0 {
		res.Points = []Point{}
		return res, nil
	}

	coords, model, err := layout(used, basis, opts)
	if err != nil {
		return Result{}, err
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
func layout(used []Input, basis string, opts Options) (*mat.Dense, string, error) {
	reduced, err := vectorize(used, basis, opts)
	if err != nil {
		return nil, "", err
	}
	if len(used) < opts.TinySet {
		return first2Cols(reduced), ModelPCA, nil
	}
	return TSNE2D(reduced), ModelTSNE, nil
}

func chooseBasis(inputs []Input, threshold float64) string {
	if len(inputs) == 0 {
		return BasisLexical
	}
	var withEmb int
	for _, in := range inputs {
		if len(in.Embedding) > 0 {
			withEmb++
		}
	}
	if float64(withEmb)/float64(len(inputs)) >= threshold {
		return BasisDense
	}
	return BasisLexical
}

// selectInputs returns the inputs that will be projected and the count skipped.
// Dense: only embedded entities (rest counted as unembedded). Lexical: all.
func selectInputs(inputs []Input, basis string) (used []Input, unembedded int) {
	if basis == BasisLexical {
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

func applyCap(in []Input, capN int) ([]Input, bool) {
	if len(in) <= capN {
		return in, false
	}
	sorted := append([]Input(nil), in...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].ObservedAt.Equal(sorted[j].ObservedAt) {
			return sorted[i].ObservedAt.After(sorted[j].ObservedAt)
		}
		return sorted[i].Confidence > sorted[j].Confidence
	})
	return sorted[:capN], true
}

// vectorize produces the reduced (≤PCADims/LSADims) matrix for the basis.
func vectorize(used []Input, basis string, opts Options) (*mat.Dense, error) {
	if basis == BasisLexical {
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

func assemble(used []Input, coords *mat.Dense, previewChars int) []Point {
	pts := make([]Point, len(used))
	for i, in := range used {
		pts[i] = Point{
			ID: in.EntityID, X: coords.At(i, 0), Y: coords.At(i, 1),
			Tier: in.Tier, Type: in.Kind, User: in.User, UserRef: in.User,
			Category: in.Category, Confidence: in.Confidence, Title: in.Title,
			Preview:    truncateRunes(in.Content, previewChars),
			ObservedAt: in.ObservedAt, ExpiresAt: in.ExpiresAt,
		}
	}
	return pts
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// FromStored builds a Result from previously-stored coords, refreshing point
// metadata/preview from the current inputs. Inputs without a stored coord are
// dropped (they'll be picked up on the next recompute).
func FromStored(inputs []Input, stored map[string][2]float64, opts Options) Result {
	opts = opts.withDefaults()
	res := Result{Points: []Point{}}
	for _, in := range inputs {
		xy, ok := stored[in.EntityID]
		if !ok {
			continue
		}
		res.Points = append(res.Points, Point{
			ID: in.EntityID, X: xy[0], Y: xy[1], Tier: in.Tier, Type: in.Kind,
			User: in.User, UserRef: in.User, Category: in.Category,
			Confidence: in.Confidence, Title: in.Title,
			Preview:    truncateRunes(in.Content, opts.PreviewChars),
			ObservedAt: in.ObservedAt, ExpiresAt: in.ExpiresAt,
		})
	}
	res.Total = len(res.Points)
	return res
}
