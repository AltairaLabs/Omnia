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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

func TestHandleIngest_Returns202AndSaves(t *testing.T) {
	store := &recordingInstitutionalStore{}
	svc := NewMemoryService(store, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 2, ChunkOverlap: 0}, nil)
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body := `{"workspace_id":"ws-1","title":"R","url":"https://sp/allowed/r.docx","site":"allowed","text":"alpha beta gamma"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/institutional/ingest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	assert.GreaterOrEqual(t, len(store.saved), 1)
}

func TestHandleIngest_MissingWorkspace_400(t *testing.T) {
	svc := NewMemoryService(&recordingInstitutionalStore{}, nil, MemoryServiceConfig{}, logr.Discard())
	svc.SetIngestion(ingestion.Config{Strategy: ingestion.StrategyChunk, ChunkSize: 2, ChunkOverlap: 0}, nil)
	h := NewHandler(svc, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/institutional/ingest",
		strings.NewReader(`{"url":"u","text":"x"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}
