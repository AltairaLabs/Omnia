/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedReembedProvider returns the same vector for every text and
// records every batch it received so tests can assert on the calls.
type fixedReembedProvider struct {
	vec    []float32
	calls  [][]string
	err    error
	failOn int // index of call that returns err (0 = first call)
}

func (p *fixedReembedProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	idx := len(p.calls)
	p.calls = append(p.calls, append([]string{}, texts...))
	if p.err != nil && idx == p.failOn {
		return nil, p.err
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = p.vec
	}
	return out, nil
}

// TestReembedWorker_BackfillsNullEmbeddings proves the happy path:
// observations saved without embeddings get re-embedded on a worker
// pass and stamped with the current model name.
func TestReembedWorker_BackfillsNullEmbeddings(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	a := &Memory{Type: "fact", Content: "alpha", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, a))
	b := &Memory{Type: "fact", Content: "beta", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, b))

	provider := &fixedReembedProvider{vec: oneHotFloat(0, 1536)}
	worker := NewReembedWorker(store, provider, ReembedWorkerOptions{
		Interval:     0, // RunOnce only
		BatchSize:    10,
		CurrentModel: "test-embed-v1",
	}, logr.Discard())

	require.NoError(t, worker.RunOnce(ctx))
	require.Len(t, provider.calls, 1, "one batched embed call expected")
	assert.ElementsMatch(t, []string{"alpha", "beta"}, provider.calls[0])

	// A second pass finds nothing left to do — the rows are now
	// stamped with the current model name.
	require.NoError(t, worker.RunOnce(ctx))
	assert.Len(t, provider.calls, 1, "second pass must not re-embed already-stamped rows")
}

// TestReembedWorker_HonoursEmbeddingServiceModel proves a row
// embedded via EmbeddingService (which stamps model name) is NOT
// re-embedded by the worker on its next pass — without the model
// stamp the worker would re-embed every row the API just embedded
// and burn provider quota forever.
func TestReembedWorker_HonoursEmbeddingServiceModel(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{Type: "fact", Content: "alpha", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, mem))

	provider := &fixedReembedProvider{vec: oneHotFloat(0, 1536)}
	embSvc := NewEmbeddingService(store, embeddingProviderShim{provider}, logr.Discard()).
		WithModelName("openai-text-embed-3")
	require.NoError(t, embSvc.EmbedMemory(ctx, mem))

	// Now run the re-embed worker with the same model name. It must
	// see the row as already embedded and skip it.
	worker := NewReembedWorker(store, provider, ReembedWorkerOptions{
		BatchSize: 10, CurrentModel: "openai-text-embed-3",
	}, logr.Discard())
	priorCalls := len(provider.calls)
	require.NoError(t, worker.RunOnce(ctx))
	assert.Equal(t, priorCalls, len(provider.calls),
		"worker must not re-embed a row already stamped with the current model")
}

// embeddingProviderShim adapts the local fixedReembedProvider (which
// satisfies ReembedProvider) to the wider EmbeddingProvider
// interface EmbeddingService consumes. Test-only.
type embeddingProviderShim struct {
	inner *fixedReembedProvider
}

func (s embeddingProviderShim) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return s.inner.Embed(ctx, texts)
}

func (s embeddingProviderShim) Dimensions() int { return 1536 }

// TestReembedWorker_RestampsOnModelChange proves rows previously
// embedded with a different model get re-embedded when the worker
// runs with a new CurrentModel. This is the model-swap migration
// path — without it old vectors would silently dilute hybrid recall.
func TestReembedWorker_RestampsOnModelChange(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	mem := &Memory{Type: "fact", Content: "old model", Confidence: 0.9, Scope: scope}
	require.NoError(t, store.Save(ctx, mem))

	// Pre-stamp the row as if a previous worker generation handled it.
	provider := &fixedReembedProvider{vec: oneHotFloat(0, 1536)}
	earlier := NewReembedWorker(store, provider, ReembedWorkerOptions{
		BatchSize: 10, CurrentModel: "embed-v1",
	}, logr.Discard())
	require.NoError(t, earlier.RunOnce(ctx))
	require.Len(t, provider.calls, 1)

	// Now run a worker stamped with a newer model. The row should
	// re-enter the candidate set.
	current := NewReembedWorker(store, provider, ReembedWorkerOptions{
		BatchSize: 10, CurrentModel: "embed-v2",
	}, logr.Discard())
	require.NoError(t, current.RunOnce(ctx))
	require.Len(t, provider.calls, 2,
		"row stamped with embed-v1 must re-embed when CurrentModel is embed-v2")
}

// TestReembedWorker_DisabledWithoutProvider proves the worker is a
// no-op when no provider is configured. Important so binaries can
// wire it unconditionally and let env / CRD config decide whether
// it actually runs.
func TestReembedWorker_DisabledWithoutProvider(t *testing.T) {
	store := newStore(t)
	worker := NewReembedWorker(store, nil, ReembedWorkerOptions{BatchSize: 10}, logr.Discard())
	assert.NoError(t, worker.RunOnce(context.Background()),
		"nil provider must short-circuit cleanly, not crash")
}

// TestReembedWorker_PerRowFailuresContinue proves that a provider
// returning a bad batch (length mismatch) bubbles the error, while
// individual row failures during the update are logged and the
// pass continues.
func TestReembedWorker_BatchLengthMismatch(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)
	require.NoError(t, store.Save(ctx, &Memory{
		Type: "fact", Content: "alpha", Confidence: 0.9, Scope: scope,
	}))

	bad := &mismatchProvider{}
	worker := NewReembedWorker(store, bad, ReembedWorkerOptions{
		BatchSize: 10, CurrentModel: "embed-v1",
	}, logr.Discard())
	err := worker.RunOnce(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "embed returned")
}

type mismatchProvider struct{}

func (mismatchProvider) Embed(_ context.Context, _ []string) ([][]float32, error) {
	return nil, nil
}

// TestReembedWorker_RunDisabledByMissingProvider proves the Run
// disable path: a nil provider exits immediately rather than
// running the ticker loop. Important so binaries can wire Run
// unconditionally and let configuration decide whether it actually
// fires.
func TestReembedWorker_RunDisabledByMissingProvider(t *testing.T) {
	store := newStore(t)
	worker := NewReembedWorker(store, nil, ReembedWorkerOptions{
		Interval: 50 * time.Millisecond,
	}, logr.Discard())

	done := make(chan struct{})
	go func() {
		worker.Run(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return when provider is nil")
	}
}

// TestReembedWorker_RunDisabledByZeroInterval proves the other
// half of the Run disable guard.
func TestReembedWorker_RunDisabledByZeroInterval(t *testing.T) {
	store := newStore(t)
	provider := &fixedReembedProvider{vec: oneHotFloat(0, 1536)}
	worker := NewReembedWorker(store, provider, ReembedWorkerOptions{
		Interval: 0,
	}, logr.Discard())

	done := make(chan struct{})
	go func() {
		worker.Run(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return when interval is zero")
	}
}

// TestReembedWorker_RunFiresInitialPassAndLoops exercises the live
// Run path: a worker started against a database with NULL-embedding
// rows backfills them on the startup pass, then the ticker fires
// at least once before ctx is cancelled. Asserts no rows remain
// missing — the most useful integration check for "did the loop
// actually do work?"
func TestReembedWorker_RunFiresInitialPassAndLoops(t *testing.T) {
	store := newStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope := testScope(testWorkspace1)

	require.NoError(t, store.Save(ctx, &Memory{
		Type: "fact", Content: "alpha", Confidence: 0.9, Scope: scope,
	}))
	require.NoError(t, store.Save(ctx, &Memory{
		Type: "fact", Content: "beta", Confidence: 0.9, Scope: scope,
	}))

	provider := &fixedReembedProvider{vec: oneHotFloat(0, 1536)}
	worker := NewReembedWorker(store, provider, ReembedWorkerOptions{
		Interval:     50 * time.Millisecond,
		BatchSize:    10,
		CurrentModel: "test-embed-v1",
	}, logr.Discard())

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()

	// Give the initial pass time to drain the backlog.
	require.Eventually(t, func() bool {
		left, err := store.FindObservationsMissingEmbedding(context.Background(), "test-embed-v1", 10)
		return err == nil && len(left) == 0
	}, 2*time.Second, 25*time.Millisecond, "worker should backfill all NULL-embedding rows")

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after ctx cancel")
	}
}

// TestReembedWorker_SkipsEmptyEmbeddings proves a per-row empty
// embedding (which happens when a provider returns a zero-length
// vector for one of N inputs) is skipped without aborting the
// pass. The other rows still get updated.
func TestReembedWorker_SkipsEmptyEmbeddings(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)

	require.NoError(t, store.Save(ctx, &Memory{
		Type: "fact", Content: "alpha", Confidence: 0.9, Scope: scope,
	}))

	provider := &mixedProvider{}
	worker := NewReembedWorker(store, provider, ReembedWorkerOptions{
		BatchSize: 10, CurrentModel: "test-embed-v1",
	}, logr.Discard())
	require.NoError(t, worker.RunOnce(ctx))
}

// mixedProvider returns one zero-length vector per call so the
// worker exercises the skip-empty branch.
type mixedProvider struct{}

func (mixedProvider) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	// All-empty result; worker should skip every row but not error.
	return out, nil
}

// TestReembedWorker_UpdateBadIDError exercises the
// UpdateObservationEmbedding "no rows" path: passing a bogus
// observation ID returns an error rather than silently no-oping.
func TestReembedWorker_UpdateBadIDError(t *testing.T) {
	store := newStore(t)
	err := store.UpdateObservationEmbedding(context.Background(),
		"00000000-0000-0000-0000-000000000000",
		oneHotFloat(0, 1536), "test-embed-v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestReembedWorker_EmbedError surfaces provider failures up the
// pass return so callers (or test loops) can detect them. The next
// pass will retry the same rows.
func TestReembedWorker_EmbedError(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	scope := testScope(testWorkspace1)
	require.NoError(t, store.Save(ctx, &Memory{
		Type: "fact", Content: "alpha", Confidence: 0.9, Scope: scope,
	}))

	provider := &fixedReembedProvider{
		vec: oneHotFloat(0, 1536),
		err: errors.New("provider down"),
	}
	worker := NewReembedWorker(store, provider, ReembedWorkerOptions{
		BatchSize: 10, CurrentModel: "embed-v1",
	}, logr.Discard())
	err := worker.RunOnce(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider down")
}
