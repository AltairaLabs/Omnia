package projection

import (
	"math"
	"sort"
	"strings"

	"gonum.org/v1/gonum/mat"
)

// TFIDFLSA builds a TF-IDF matrix over docs and reduces it to at most `dims`
// columns via truncated SVD (latent semantic analysis). Pure + deterministic.
func TFIDFLSA(docs []string, dims int) (*mat.Dense, error) {
	tokens, dfCount := documentFrequencies(docs)
	vocab, idx := buildVocab(dfCount)
	if len(vocab) == 0 {
		return mat.NewDense(len(docs), 1, nil), nil
	}
	tfidf := buildTFIDF(tokens, idx, dfCount, len(vocab))
	return truncatedSVD(tfidf, dims), nil
}

// documentFrequencies tokenizes each doc and counts how many docs each term
// appears in.
func documentFrequencies(docs []string) (tokens [][]string, df map[string]int) {
	tokens = make([][]string, len(docs))
	df = map[string]int{}
	for i, d := range docs {
		tokens[i] = tokenize(d)
		seen := map[string]bool{}
		for _, t := range tokens[i] {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}
	return tokens, df
}

// buildVocab returns the sorted vocabulary (stable for determinism) and a
// term→column index.
func buildVocab(df map[string]int) (vocab []string, idx map[string]int) {
	vocab = make([]string, 0, len(df))
	for t := range df {
		vocab = append(vocab, t)
	}
	sort.Strings(vocab)
	idx = make(map[string]int, len(vocab))
	for i, t := range vocab {
		idx[t] = i
	}
	return vocab, idx
}

// buildTFIDF builds the n×|vocab| TF-IDF matrix.
func buildTFIDF(tokens [][]string, idx, df map[string]int, vocabSize int) *mat.Dense {
	n := len(tokens)
	m := mat.NewDense(n, vocabSize, nil)
	for i := range tokens {
		if len(tokens[i]) == 0 {
			continue
		}
		counts := map[string]int{}
		for _, t := range tokens[i] {
			counts[t]++
		}
		for t, c := range counts {
			tf := float64(c) / float64(len(tokens[i]))
			idf := math.Log(float64(n+1) / float64(df[t]+1))
			m.Set(i, idx[t], tf*idf)
		}
	}
	return m
}

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	})
	out := fields[:0]
	for _, f := range fields {
		if len(f) > 2 { // drop very short tokens
			out = append(out, f)
		}
	}
	return out
}

// truncatedSVD returns U*S truncated to k columns (the LSA document vectors).
func truncatedSVD(m *mat.Dense, k int) *mat.Dense {
	var svd mat.SVD
	if !svd.Factorize(m, mat.SVDThin) {
		// Fallback: return the original (already low-dim or degenerate).
		return m
	}
	var u mat.Dense
	svd.UTo(&u)
	s := svd.Values(nil)
	r, _ := u.Dims()
	cols := k
	if len(s) < cols {
		cols = len(s)
	}
	out := mat.NewDense(r, cols, nil)
	for i := 0; i < r; i++ {
		for j := 0; j < cols; j++ {
			out.Set(i, j, u.At(i, j)*s[j])
		}
	}
	return out
}
