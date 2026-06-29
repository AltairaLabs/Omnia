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

package api

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

const (
	testWS           = "ws-1"
	testTwoSentences = "First sentence. Second sentence."
)

// recordingInstitutionalStore implements InstitutionalStore and
// records SaveInstitutional calls for assertion.
type recordingInstitutionalStore struct {
	saved []*memory.Memory
}

func (r *recordingInstitutionalStore) SaveInstitutional(_ context.Context, mem *memory.Memory) error {
	r.saved = append(r.saved, mem)
	return nil
}

func (r *recordingInstitutionalStore) ListInstitutional(_ context.Context, _ string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}

func (r *recordingInstitutionalStore) DeleteInstitutional(_ context.Context, _, _ string) error {
	return nil
}

// erroringInstitutionalStore fails every SaveInstitutional so the ingest
// error path (outcome=error) can be exercised.
type erroringInstitutionalStore struct{}

func (e *erroringInstitutionalStore) SaveInstitutional(_ context.Context, _ *memory.Memory) error {
	return assert.AnError
}

func (e *erroringInstitutionalStore) ListInstitutional(_ context.Context, _ string, _ memory.ListOptions) ([]*memory.Memory, error) {
	return nil, nil
}

func (e *erroringInstitutionalStore) DeleteInstitutional(_ context.Context, _, _ string) error {
	return nil
}

func TestIngestDocument_ChunkStrategy_SavesItemsWithAboutKey(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 2, ChunkOverlap: 0}, nil)

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		Title: "Runbook", URL: testURLAllowed, Site: "allowed",
		Text: "alpha beta gamma delta", // → 2 chunks: ["alpha beta", "gamma delta"]
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 2)

	first := store.saved[0]
	assert.Equal(t, testWS, first.Scope[memory.ScopeWorkspaceID])
	assert.Equal(t, "alpha beta", first.Content)
	assert.Equal(t, "sharepoint_doc", first.Metadata[memory.MetaKeyAboutKind])
	assert.Equal(t, "https://sp/allowed/r.docx#0", first.Metadata[memory.MetaKeyAboutKey])
	assert.Equal(t, testURLAllowed, first.Metadata[testMetaKeyURL])

	second := store.saved[1]
	assert.Equal(t, "https://sp/allowed/r.docx#1", second.Metadata[memory.MetaKeyAboutKey])
}

func TestIngestDocument_NoConfig_DefaultsToChunk(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	// No SetIngestion — zero Config must normalise to chunk with default geometry.
	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: "alpha beta gamma",
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1) // one chunk (default 200-word window)
	assert.Equal(t, "https://sp/allowed/r.docx#0", store.saved[0].Metadata[memory.MetaKeyAboutKey])
}

// fakeSummaryQueue records enqueues for assertion.
type fakeSummaryQueue struct {
	enqueued []ingestion.WorkItem
}

func (f *fakeSummaryQueue) Enqueue(_ context.Context, it ingestion.WorkItem) error {
	f.enqueued = append(f.enqueued, it)
	return nil
}
func (f *fakeSummaryQueue) List(context.Context, int) ([]ingestion.WorkItem, error) {
	return f.enqueued, nil
}
func (f *fakeSummaryQueue) Get(_ context.Context, _, aboutKey string) (ingestion.WorkItem, error) {
	for _, it := range f.enqueued {
		if it.AboutKey == aboutKey {
			return it, nil
		}
	}
	return ingestion.WorkItem{}, ingestion.ErrWorkItemNotFound
}
func (f *fakeSummaryQueue) Complete(context.Context, string, string) error { return nil }

func TestIngestDocument_ExtractiveSummary_SavesOneItem(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{
		Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerExtractive,
	}, nil)

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: "First sentence. Second sentence. Third sentence.",
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "https://sp/allowed/r.docx#0", store.saved[0].Metadata[memory.MetaKeyAboutKey])
}

func TestIngestDocument_AgentSummary_Enqueues(t *testing.T) {
	store := &recordingInstitutionalStore{}
	queue := &fakeSummaryQueue{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{
		Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerAgent,
	}, queue)

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: "some doc text",
	})
	require.NoError(t, err)
	assert.Empty(t, store.saved, "agent path stores nothing synchronously")
	require.Len(t, queue.enqueued, 1)
	assert.Equal(t, testURLAllowed, queue.enqueued[0].AboutKey)
	assert.Equal(t, ingestion.StrategySummary, queue.enqueued[0].Strategy)
}

func TestIngestDocument_AgentSummary_NoQueue_FallsBackToExtractive(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{
		Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerAgent,
	}, nil) // no queue

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: testTwoSentences,
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1, "no queue -> extractive fallback stores synchronously")
}

func TestIngestDocument_RecordsSuccessAndItemsMetrics(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 2, ChunkOverlap: 0}, nil)

	docsBefore := testutil.ToFloat64(ingestDocumentsTotal.WithLabelValues(ingestion.StrategyChunk, ingestOutcomeSuccess))
	itemsBefore := testutil.ToFloat64(ingestItemsTotal.WithLabelValues(ingestion.StrategyChunk))

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: "alpha beta gamma delta", // → 2 chunks
	})
	require.NoError(t, err)

	docsAfter := testutil.ToFloat64(ingestDocumentsTotal.WithLabelValues(ingestion.StrategyChunk, ingestOutcomeSuccess))
	itemsAfter := testutil.ToFloat64(ingestItemsTotal.WithLabelValues(ingestion.StrategyChunk))
	assert.Equal(t, float64(1), docsAfter-docsBefore, "one successful document")
	assert.Equal(t, float64(2), itemsAfter-itemsBefore, "two chunks persisted")
}

func TestIngestDocument_RecordsErrorOutcome(t *testing.T) {
	store := &erroringInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 2, ChunkOverlap: 0}, nil)

	before := testutil.ToFloat64(ingestDocumentsTotal.WithLabelValues(ingestion.StrategyChunk, ingestOutcomeError))

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: "alpha beta",
	})
	require.Error(t, err)

	after := testutil.ToFloat64(ingestDocumentsTotal.WithLabelValues(ingestion.StrategyChunk, ingestOutcomeError))
	assert.Equal(t, float64(1), after-before, "error path bumps outcome=error")
}

func TestIngestDocument_RecordsEnqueuedOutcome(t *testing.T) {
	store := &recordingInstitutionalStore{}
	queue := &fakeSummaryQueue{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{
		Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerAgent,
	}, queue)

	before := testutil.ToFloat64(ingestDocumentsTotal.WithLabelValues(ingestion.StrategySummary, ingestOutcomeEnqueued))

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: "some doc text",
	})
	require.NoError(t, err)

	after := testutil.ToFloat64(ingestDocumentsTotal.WithLabelValues(ingestion.StrategySummary, ingestOutcomeEnqueued))
	assert.Equal(t, float64(1), after-before, "agent-enqueue path bumps outcome=enqueued")
}

func TestResolveIngestionConfig_PolicyOverridesFallback(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40}, nil)
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{Policy: &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{Ingestion: &omniav1alpha1.MemoryIngestionConfig{
			Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerExtractive,
		}},
	}})

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: testTwoSentences,
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1, "policy switched chunk->summary, so one summary item")
	assert.Equal(t, "summary", store.saved[0].Metadata[ingestion.MetaKeyKind])
}

func TestResolveIngestionConfig_NilPolicy_UsesFallback(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{
		Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerExtractive,
	}, nil)
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{Policy: nil})

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: testTwoSentences,
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1, "nil policy -> fallback summary strategy, one item")
	assert.Equal(t, "summary", store.saved[0].Metadata[ingestion.MetaKeyKind])
}

func TestResolveIngestionConfig_LoaderError_UsesFallback(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 2, ChunkOverlap: 0}, nil)
	svc.SetPolicyLoader(erroringPolicyLoader{})

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: "alpha beta gamma delta", // → 2 chunks
	})
	require.NoError(t, err, "loader error must not block ingest")
	require.Len(t, store.saved, 2, "loader error -> fallback chunk strategy, ingest proceeds")
}

func TestResolveIngestionConfig_PartialPolicy_FieldLevelFallback(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	// Fallback carries the summarizer + chunk geometry; policy supplies only strategy.
	svc.SetIngestion(ingestion.Config{
		Summarizer: ingestion.SummarizerExtractive, ChunkSize: 2, ChunkOverlap: 0,
	}, nil)
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{Policy: &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{Ingestion: &omniav1alpha1.MemoryIngestionConfig{
			Strategy: ingestion.StrategySummary,
		}},
	}})

	err := svc.IngestDocument(context.Background(), testWS, ingestion.SourceDoc{
		URL: testURLAllowed, Text: testTwoSentences,
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1, "strategy from policy, summarizer from fallback -> one summary item")
	assert.Equal(t, "summary", store.saved[0].Metadata[ingestion.MetaKeyKind])
}
