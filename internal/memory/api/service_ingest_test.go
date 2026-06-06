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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

// recordingInstitutionalStore embeds mockMemoryStore and overrides
// SaveInstitutional to record calls for assertion.
type recordingInstitutionalStore struct {
	mockMemoryStore
	saved []*memory.Memory
}

func (r *recordingInstitutionalStore) SaveInstitutional(_ context.Context, mem *memory.Memory) error {
	r.saved = append(r.saved, mem)
	return nil
}

func TestIngestDocument_ChunkStrategy_SavesItemsWithAboutKey(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 2, ChunkOverlap: 0}, nil)

	err := svc.IngestDocument(context.Background(), "ws-1", ingestion.SourceDoc{
		Title: "Runbook", URL: testURLAllowed, Site: "allowed",
		Text: "alpha beta gamma delta", // → 2 chunks: ["alpha beta", "gamma delta"]
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 2)

	first := store.saved[0]
	assert.Equal(t, "ws-1", first.Scope[memory.ScopeWorkspaceID])
	assert.Equal(t, "alpha beta", first.Content)
	assert.Equal(t, "sharepoint_doc", first.Metadata[memory.MetaKeyAboutKind])
	assert.Equal(t, "https://sp/allowed/r.docx#0", first.Metadata[memory.MetaKeyAboutKey])
	assert.Equal(t, testURLAllowed, first.Metadata[testMetaKeyURL])

	second := store.saved[1]
	assert.Equal(t, "https://sp/allowed/r.docx#1", second.Metadata[memory.MetaKeyAboutKey])
}

func TestIngestDocument_NoConfig_DefaultsToChunk(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	// No SetIngestion — zero Config must normalise to chunk with default geometry.
	err := svc.IngestDocument(context.Background(), "ws-1", ingestion.SourceDoc{
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
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetIngestion(ingestion.Config{
		Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerExtractive,
	}, nil)

	err := svc.IngestDocument(context.Background(), "ws-1", ingestion.SourceDoc{
		URL: testURLAllowed, Text: "First sentence. Second sentence. Third sentence.",
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "https://sp/allowed/r.docx#0", store.saved[0].Metadata[memory.MetaKeyAboutKey])
}

func TestIngestDocument_AgentSummary_Enqueues(t *testing.T) {
	store := &recordingInstitutionalStore{}
	queue := &fakeSummaryQueue{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetIngestion(ingestion.Config{
		Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerAgent,
	}, queue)

	err := svc.IngestDocument(context.Background(), "ws-1", ingestion.SourceDoc{
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
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetIngestion(ingestion.Config{
		Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerAgent,
	}, nil) // no queue

	err := svc.IngestDocument(context.Background(), "ws-1", ingestion.SourceDoc{
		URL: testURLAllowed, Text: "First sentence. Second sentence.",
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1, "no queue -> extractive fallback stores synchronously")
}

func TestResolveIngestionConfig_PolicyOverridesFallback(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40}, nil)
	svc.SetPolicyLoader(&memory.StaticPolicyLoader{Policy: &omniav1alpha1.MemoryPolicy{
		Spec: omniav1alpha1.MemoryPolicySpec{Ingestion: &omniav1alpha1.MemoryIngestionConfig{
			Strategy: ingestion.StrategySummary, Summarizer: ingestion.SummarizerExtractive,
		}},
	}})

	err := svc.IngestDocument(context.Background(), "ws-1", ingestion.SourceDoc{
		URL: testURLAllowed, Text: "First sentence. Second sentence.",
	})
	require.NoError(t, err)
	require.Len(t, store.saved, 1, "policy switched chunk->summary, so one summary item")
	assert.Equal(t, "summary", store.saved[0].Metadata[ingestion.MetaKeyKind])
}
