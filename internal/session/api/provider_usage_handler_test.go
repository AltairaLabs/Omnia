/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockProviderUsageStore is a test double for ProviderUsageStore.
type mockProviderUsageStore struct {
	recorded []*ProviderUsage
	err      error
}

func (m *mockProviderUsageStore) RecordProviderUsage(_ context.Context, rows []*ProviderUsage) error {
	if m.err != nil {
		return m.err
	}
	m.recorded = append(m.recorded, rows...)
	return nil
}

func newTestProviderUsageHandler(store ProviderUsageStore) *http.ServeMux {
	h := NewHandler(nil, logr.Discard())
	if store != nil {
		h.SetProviderUsageService(NewProviderUsageService(store, logr.Discard()))
	}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func postProviderUsage(mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider-usage", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestHandleRecordProviderUsage_Success(t *testing.T) {
	store := &mockProviderUsageStore{}
	mux := newTestProviderUsageHandler(store)

	w := postProviderUsage(mux, `[
		{"namespace":"omnia-demo","provider":"azure","providerName":"azure-embed","model":"text-embedding-3-small","source":"embedding","inputTokens":512},
		{"namespace":"omnia-demo","provider":"openai","source":"judge","inputTokens":100,"outputTokens":20}
	]`)

	require.Equal(t, http.StatusCreated, w.Code)
	require.Len(t, store.recorded, 2)
	assert.Equal(t, "embedding", store.recorded[0].Source)
	assert.Equal(t, int64(512), store.recorded[0].InputTokens)
	// CallCount defaults to 1 when omitted.
	assert.Equal(t, int32(1), store.recorded[0].CallCount)
	assert.Equal(t, "judge", store.recorded[1].Source)
}

func TestHandleRecordProviderUsage_NoStoreReturns503(t *testing.T) {
	mux := newTestProviderUsageHandler(nil)
	w := postProviderUsage(mux, `[{"namespace":"ns","provider":"openai","source":"embedding"}]`)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleRecordProviderUsage_MissingRequiredFieldReturns400(t *testing.T) {
	store := &mockProviderUsageStore{}
	mux := newTestProviderUsageHandler(store)

	// Missing source.
	w := postProviderUsage(mux, `[{"namespace":"ns","provider":"openai"}]`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, store.recorded)
}

func TestHandleRecordProviderUsage_InvalidJSONReturns400(t *testing.T) {
	store := &mockProviderUsageStore{}
	mux := newTestProviderUsageHandler(store)
	w := postProviderUsage(mux, `not json`)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleRecordProviderUsage_EmptyArrayIsAccepted(t *testing.T) {
	store := &mockProviderUsageStore{}
	mux := newTestProviderUsageHandler(store)
	w := postProviderUsage(mux, `[]`)
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Empty(t, store.recorded)
}

func TestHandleRecordProviderUsage_StoreErrorReturns500(t *testing.T) {
	store := &mockProviderUsageStore{err: errors.New("boom")}
	mux := newTestProviderUsageHandler(store)
	w := postProviderUsage(mux, `[{"namespace":"ns","provider":"openai","source":"embedding"}]`)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
