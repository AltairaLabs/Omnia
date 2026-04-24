/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package classify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// fakeEmbedder returns deterministic vectors. Each text gets a vector
// derived from the routing function so we can craft inputs that map to
// specific category centroids.
type fakeEmbedder struct {
	dim    int
	vector func(text string) []float32
	err    error
	delay  time.Duration
}

func (f *fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = f.vector(t)
	}
	return out, nil
}

// unitVector returns a vector that points along axis i in dim dimensions.
func unitVector(i, dim int) []float32 {
	v := make([]float32, dim)
	v[i%dim] = 1
	return v
}

// withExemplars overrides categoryExemplars for the duration of t.
func withExemplars(t *testing.T, e map[privacy.ConsentCategory][]string) {
	t.Helper()
	saved := categoryExemplars
	categoryExemplars = e
	t.Cleanup(func() { categoryExemplars = saved })
}

func TestEmbeddingClassifier_AboveThreshold_ReturnsCategory(t *testing.T) {
	const dim = 8
	categoryAxis := map[string]int{
		"health":      0,
		"identity":    1,
		"location":    2,
		"preferences": 3,
		"context":     4,
		"history":     5,
	}
	axisFor := func(text string) int {
		for k, v := range categoryAxis {
			if len(text) >= len(k) && text[:len(k)] == k {
				return v
			}
		}
		return 7
	}
	embedder := &fakeEmbedder{
		dim: dim,
		vector: func(text string) []float32 {
			return unitVector(axisFor(text), dim)
		},
	}
	withExemplars(t, map[privacy.ConsentCategory][]string{
		privacy.ConsentMemoryHealth:      {"health-exemplar"},
		privacy.ConsentMemoryIdentity:    {"identity-exemplar"},
		privacy.ConsentMemoryLocation:    {"location-exemplar"},
		privacy.ConsentMemoryPreferences: {"preferences-exemplar"},
		privacy.ConsentMemoryContext:     {"context-exemplar"},
		privacy.ConsentMemoryHistory:     {"history-exemplar"},
	})

	ec := NewEmbeddingClassifier(embedder, logr.Discard())
	if err := ec.PrewarmCentroids(context.Background()); err != nil {
		t.Fatalf("PrewarmCentroids: %v", err)
	}

	cat, err := ec.Classify(context.Background(), "health: I get migraines")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if cat != privacy.ConsentMemoryHealth {
		t.Errorf("got %q, want %q", cat, privacy.ConsentMemoryHealth)
	}
}

func TestEmbeddingClassifier_BelowThreshold_ReturnsEmpty(t *testing.T) {
	const dim = 4
	// Exemplar lives on axis 0; query lives on axis 1 (orthogonal → cosine 0).
	// 0 is below the default 0.7 threshold so Classify returns "".
	embedder := &fakeEmbedder{
		dim: dim,
		vector: func(text string) []float32 {
			if text == "exemplar-a" {
				return unitVector(0, dim)
			}
			return unitVector(1, dim)
		},
	}
	withExemplars(t, map[privacy.ConsentCategory][]string{
		privacy.ConsentMemoryHealth: {"exemplar-a"},
	})

	ec := NewEmbeddingClassifier(embedder, logr.Discard())
	if err := ec.PrewarmCentroids(context.Background()); err != nil {
		t.Fatalf("PrewarmCentroids: %v", err)
	}
	got, err := ec.Classify(context.Background(), "ambiguous-query")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestEmbeddingClassifier_ProviderError_BubblesUp(t *testing.T) {
	withExemplars(t, map[privacy.ConsentCategory][]string{
		privacy.ConsentMemoryHealth: {"a"},
	})
	want := errors.New("provider down")
	embedder := &fakeEmbedder{
		dim:    4,
		vector: func(_ string) []float32 { return []float32{1, 0, 0, 0} },
	}
	ec := NewEmbeddingClassifier(embedder, logr.Discard())
	if err := ec.PrewarmCentroids(context.Background()); err != nil {
		t.Fatalf("PrewarmCentroids: %v", err)
	}
	embedder.err = want
	_, err := ec.Classify(context.Background(), "any")
	if !errors.Is(err, want) {
		t.Errorf("got %v, want errors.Is(want)", err)
	}
}

func TestEmbeddingClassifier_Timeout_ReturnsContextError(t *testing.T) {
	withExemplars(t, map[privacy.ConsentCategory][]string{
		privacy.ConsentMemoryHealth: {"a"},
	})
	embedder := &fakeEmbedder{
		dim:    4,
		vector: func(_ string) []float32 { return []float32{1, 0, 0, 0} },
	}
	ec := NewEmbeddingClassifier(embedder, logr.Discard())
	if err := ec.PrewarmCentroids(context.Background()); err != nil {
		t.Fatalf("PrewarmCentroids: %v", err)
	}
	embedder.delay = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := ec.Classify(ctx, "any")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("got %v, want context.DeadlineExceeded", err)
	}
}

func TestEmbeddingClassifier_PrewarmIdempotent(t *testing.T) {
	withExemplars(t, map[privacy.ConsentCategory][]string{
		privacy.ConsentMemoryHealth: {"a"},
	})
	embedder := &fakeEmbedder{
		dim:    4,
		vector: func(_ string) []float32 { return []float32{1, 0, 0, 0} },
	}
	ec := NewEmbeddingClassifier(embedder, logr.Discard())
	if err := ec.PrewarmCentroids(context.Background()); err != nil {
		t.Fatalf("first prewarm: %v", err)
	}
	if err := ec.PrewarmCentroids(context.Background()); err != nil {
		t.Fatalf("second prewarm: %v", err)
	}
}

func TestEmbeddingClassifier_ClassifyBeforePrewarm_Errors(t *testing.T) {
	withExemplars(t, map[privacy.ConsentCategory][]string{
		privacy.ConsentMemoryHealth: {"a"},
	})
	embedder := &fakeEmbedder{
		dim:    4,
		vector: func(_ string) []float32 { return []float32{1, 0, 0, 0} },
	}
	ec := NewEmbeddingClassifier(embedder, logr.Discard())
	_, err := ec.Classify(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error when centroids unset, got nil")
	}
}
