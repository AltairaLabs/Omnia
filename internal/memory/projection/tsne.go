package projection

import (
	"github.com/danaugrs/go-tsne/tsne"
	"gonum.org/v1/gonum/mat"
)

const (
	tsneLearningRate = 200.0
	tsneMaxIter      = 1000
)

// TSNE2D projects rows of X to 2D via EXACT t-SNE (danaugrs/go-tsne builds
// full n×n pairwise affinity matrices — it is not Barnes-Hut), so cost and
// memory are O(n²) per iteration. Callers MUST bound n via Options.Cap
// (see defaultRenderCap) to keep renders tractable. Perplexity is clamped so
// it stays below the point count (t-SNE requires perplexity < n).
func TSNE2D(X *mat.Dense) *mat.Dense {
	n, _ := X.Dims()
	perplexity := 30.0
	if maxP := float64(n-1) / 3.0; perplexity > maxP {
		perplexity = maxP
	}
	if perplexity < 2 {
		perplexity = 2
	}
	t := tsne.NewTSNE(2, perplexity, tsneLearningRate, tsneMaxIter, false)
	emb := t.EmbedData(X, nil)
	out := mat.NewDense(n, 2, nil)
	out.Copy(emb)
	return out
}
