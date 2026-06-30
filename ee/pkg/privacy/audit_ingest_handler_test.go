/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/ee/pkg/audit"
)

type fakeAuditIngester struct {
	gotService string
	gotEvents  []*audit.Entry
	ingested   int
	err        error
}

func (f *fakeAuditIngester) InsertEvents(_ context.Context, svc string, ev []*audit.Entry) (int, error) {
	f.gotService = svc
	f.gotEvents = ev
	if f.err != nil {
		return 0, f.err
	}
	return f.ingested, nil
}

func auditIngestServer(store AuditIngester) http.Handler {
	mux := http.NewServeMux()
	NewAuditIngestHandler(store, logr.Discard()).RegisterRoutes(mux)
	return mux
}

func postAudit(h http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/privacy/audit-events", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuditIngest_Success(t *testing.T) {
	f := &fakeAuditIngester{ingested: 2}
	rec := postAudit(auditIngestServer(f), `{"sourceService":"memory-api","events":[
		{"id":1,"eventType":"memory_write_blocked","workspace":"ws-uid","userId":"u1"},
		{"id":2,"eventType":"pii_redacted","workspace":"ws-uid"}]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "memory-api", f.gotService)
	require.Len(t, f.gotEvents, 2)

	var resp AuditIngestResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 2, resp.Ingested)
	require.Equal(t, 0, resp.Duplicates)
}

func TestAuditIngest_ReportsDuplicates(t *testing.T) {
	// 2 events sent, store reports only 1 newly inserted -> 1 duplicate.
	f := &fakeAuditIngester{ingested: 1}
	rec := postAudit(auditIngestServer(f), `{"sourceService":"session-api","events":[
		{"id":1,"eventType":"pii_redacted"},{"id":2,"eventType":"pii_redacted"}]}`)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp AuditIngestResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Ingested)
	require.Equal(t, 1, resp.Duplicates)
}

func TestAuditIngest_BadJSON(t *testing.T) {
	rec := postAudit(auditIngestServer(&fakeAuditIngester{}), `{not json`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAuditIngest_MissingSourceService(t *testing.T) {
	rec := postAudit(auditIngestServer(&fakeAuditIngester{}),
		`{"events":[{"id":1,"eventType":"pii_redacted"}]}`)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAuditIngest_StoreError(t *testing.T) {
	f := &fakeAuditIngester{err: errors.New("db down")}
	rec := postAudit(auditIngestServer(f),
		`{"sourceService":"memory-api","events":[{"id":1,"eventType":"pii_redacted"}]}`)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}
