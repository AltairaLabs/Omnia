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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

func newSummaryHandler(t *testing.T, store *recordingInstitutionalStore, queue ingestion.SummaryQueue) *Handler {
	t.Helper()
	svc := NewMemoryService(&mockMemoryStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetInstitutionalStore(store)
	svc.SetIngestion(ingestion.Config{}, queue)
	return NewHandler(svc, logr.Discard())
}

func TestSummaryCandidates_ReturnsPending(t *testing.T) {
	queue := &fakeSummaryQueue{enqueued: []ingestion.WorkItem{{
		WorkspaceID: testWS, AboutKey: testURLAllowed, Strategy: ingestion.StrategySummary,
		Doc: ingestion.SourceDoc{URL: testURLAllowed, Text: "doc text"},
	}}}
	h := newSummaryHandler(t, &recordingInstitutionalStore{}, queue)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/summary-candidates?limit=10", nil)
	rr := httptest.NewRecorder()
	h.handleListSummaryCandidates(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp ListSummaryCandidatesResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Total)
	assert.Equal(t, "doc text", resp.Candidates[0].Text)
	assert.Equal(t, testURLAllowed, resp.Candidates[0].AboutKey)
}

func TestSummaryCandidates_QueueDisabled_EmptyList(t *testing.T) {
	h := newSummaryHandler(t, &recordingInstitutionalStore{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest/summary-candidates", nil)
	rr := httptest.NewRecorder()
	h.handleListSummaryCandidates(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp ListSummaryCandidatesResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Total)
}

func TestSaveDocumentSummary_StoresAndCompletes(t *testing.T) {
	store := &recordingInstitutionalStore{}
	queue := &fakeSummaryQueue{enqueued: []ingestion.WorkItem{{
		WorkspaceID: testWS, AboutKey: testURLAllowed, Strategy: ingestion.StrategySummary,
		Doc: ingestion.SourceDoc{URL: testURLAllowed, Text: "doc text"},
	}}}
	h := newSummaryHandler(t, store, queue)

	body, _ := json.Marshal(SaveDocumentSummaryRequest{
		WorkspaceID: testWS, AboutKey: testURLAllowed, Summary: "the summary",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/summaries", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.handleSaveDocumentSummary(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	require.Len(t, store.saved, 1)
	assert.Equal(t, "the summary", store.saved[0].Content)
	assert.Equal(t, "https://sp/allowed/r.docx#0", store.saved[0].Metadata[memory.MetaKeyAboutKey])
}

func TestSaveDocumentSummary_MissingWorkspace_400(t *testing.T) {
	h := newSummaryHandler(t, &recordingInstitutionalStore{}, &fakeSummaryQueue{})
	body, _ := json.Marshal(SaveDocumentSummaryRequest{AboutKey: "k", Summary: "s"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/summaries", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.handleSaveDocumentSummary(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSaveDocumentSummary_UnknownItem_404(t *testing.T) {
	h := newSummaryHandler(t, &recordingInstitutionalStore{}, &fakeSummaryQueue{})
	body, _ := json.Marshal(SaveDocumentSummaryRequest{
		WorkspaceID: testWS, AboutKey: "does-not-exist", Summary: "s",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest/summaries", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.handleSaveDocumentSummary(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
}
