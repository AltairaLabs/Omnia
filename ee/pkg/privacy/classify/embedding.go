/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package classify

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// Embedder is the minimum interface the classifier needs from a provider.
// memory.EmbeddingProvider and PromptKit's EmbeddingProvider both satisfy
// this shape via the existing embeddingProviderAdapter in cmd/memory-api.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// DefaultThreshold is the cosine-similarity floor for considering a
// category match.
const DefaultThreshold = 0.7

// DefaultTimeout caps each per-save embedding call.
const DefaultTimeout = 1 * time.Second

// EmbeddingClassifier classifies content via cosine similarity against
// per-category exemplar centroids. Centroids are computed once
// (PrewarmCentroids) and cached for the process lifetime.
//
// Thread-safe: Classify holds an RLock while reading centroids; Prewarm
// holds the write lock briefly while assigning the result.
type EmbeddingClassifier struct {
	embedder  Embedder
	threshold float64
	timeout   time.Duration
	log       logr.Logger

	mu        sync.RWMutex
	centroids map[privacy.ConsentCategory][]float32 // nil until prewarmed
}

// NewEmbeddingClassifier builds a classifier with default threshold and timeout.
func NewEmbeddingClassifier(embedder Embedder, log logr.Logger) *EmbeddingClassifier {
	return &EmbeddingClassifier{
		embedder:  embedder,
		threshold: DefaultThreshold,
		timeout:   DefaultTimeout,
		log:       log.WithName("embedding-classifier"),
	}
}

// PrewarmCentroids embeds every exemplar in one batch call and computes the
// L2-normalised mean per category. Safe to call multiple times — overwrites
// existing centroids on each successful call.
func (e *EmbeddingClassifier) PrewarmCentroids(ctx context.Context) error {
	texts, perCat, order := flattenExemplars()
	if len(texts) == 0 {
		return fmt.Errorf("classify: no exemplars defined")
	}
	vecs, err := e.embedder.Embed(ctx, texts)
	if err != nil {
		return fmt.Errorf("classify: embed exemplars: %w", err)
	}
	if len(vecs) != len(texts) {
		return fmt.Errorf("classify: embedder returned %d vectors for %d texts", len(vecs), len(texts))
	}
	cents := make(map[privacy.ConsentCategory][]float32, len(perCat))
	idx := 0
	for _, cat := range order {
		n := perCat[cat]
		mean := meanVector(vecs[idx : idx+n])
		normalise(mean)
		cents[cat] = mean
		idx += n
	}
	e.mu.Lock()
	e.centroids = cents
	e.mu.Unlock()
	e.log.V(1).Info("centroids computed",
		"categories", len(cents),
		"exemplars", len(texts),
	)
	return nil
}

// Classify returns the highest-scoring category whose cosine similarity
// to the content embedding is at least threshold. Returns "" when no
// category clears the threshold; errors when centroids are unset or the
// embedder fails. The call is bounded by min(ctx deadline, timeout).
func (e *EmbeddingClassifier) Classify(ctx context.Context, content string) (privacy.ConsentCategory, error) {
	e.mu.RLock()
	cents := e.centroids
	e.mu.RUnlock()
	if cents == nil {
		return "", fmt.Errorf("classify: centroids not prewarmed")
	}
	cctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	vecs, err := e.embedder.Embed(cctx, []string{content})
	if err != nil {
		return "", err
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return "", fmt.Errorf("classify: empty embedding")
	}
	q := vecs[0]
	normalise(q)

	best := privacy.ConsentCategory("")
	bestSim := -1.0
	for cat, c := range cents {
		s := cosine(q, c)
		if s > bestSim {
			bestSim = s
			best = cat
		}
	}
	if bestSim < e.threshold {
		return "", nil
	}
	return best, nil
}

// flattenExemplars returns a flat slice of all exemplar texts, a map of
// category → count for slicing the result back, and the deterministic
// iteration order over the category map (Go map iteration isn't stable).
func flattenExemplars() ([]string, map[privacy.ConsentCategory]int, []privacy.ConsentCategory) {
	perCat := make(map[privacy.ConsentCategory]int, len(categoryExemplars))
	order := make([]privacy.ConsentCategory, 0, len(categoryExemplars))
	total := 0
	for cat, ex := range categoryExemplars {
		perCat[cat] = len(ex)
		order = append(order, cat)
		total += len(ex)
	}
	texts := make([]string, 0, total)
	for _, cat := range order {
		texts = append(texts, categoryExemplars[cat]...)
	}
	return texts, perCat, order
}

func meanVector(vecs [][]float32) []float32 {
	if len(vecs) == 0 {
		return nil
	}
	dim := len(vecs[0])
	out := make([]float32, dim)
	for _, v := range vecs {
		for i := 0; i < dim && i < len(v); i++ {
			out[i] += v[i]
		}
	}
	scale := float32(1.0 / float64(len(vecs)))
	for i := range out {
		out[i] *= scale
	}
	return out
}

func normalise(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return
	}
	inv := float32(1.0 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot
}
