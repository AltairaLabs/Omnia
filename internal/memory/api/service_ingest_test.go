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
	svc.SetIngestionStrategy(ingestion.NewChunkStrategy(2, 0)) // 2-word chunks, no overlap

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

func TestIngestDocument_NoStrategyConfigured_Errors(t *testing.T) {
	svc := NewMemoryService(&recordingInstitutionalStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	err := svc.IngestDocument(context.Background(), "ws-1", ingestion.SourceDoc{Text: "x"})
	require.Error(t, err)
}
