/*
Copyright 2025.

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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fiTestNamespaceA is the canonical test namespace used in this file.
const fiTestNamespaceA = "ns-a"

// fiCreateBody returns a minimal JSON body for the create endpoint with
// the given id + function name + status. Extracted so the embedded
// JSON literals don't trip goconst on "ns-a".
func fiCreateBody(id, function, status string) string {
	return `{"id":"` + id + `","namespace":"` + fiTestNamespaceA +
		`","functionName":"` + function + `","inputHash":"h","status":"` + status + `"}`
}

// mockFunctionInvocationsStore is a small in-memory FunctionInvocationsStore.
type mockFunctionInvocationsStore struct {
	rows       map[string]*FunctionInvocation // keyed by id
	listResult []*FunctionInvocation
	createErr  error
	getErr     error
	listErr    error
}

func (m *mockFunctionInvocationsStore) CreateFunctionInvocation(_ context.Context, inv *FunctionInvocation) error {
	if m.createErr != nil {
		return m.createErr
	}
	if m.rows == nil {
		m.rows = map[string]*FunctionInvocation{}
	}
	m.rows[inv.ID] = inv
	return nil
}

func (m *mockFunctionInvocationsStore) GetFunctionInvocation(_ context.Context, namespace, id string) (*FunctionInvocation, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	inv, ok := m.rows[id]
	if !ok || inv.Namespace != namespace {
		return nil, ErrFunctionInvocationNotFound
	}
	return inv, nil
}

func (m *mockFunctionInvocationsStore) ListFunctionInvocations(_ context.Context, _ FunctionInvocationListOpts) ([]*FunctionInvocation, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResult, nil
}

func newFunctionInvocationsHandler(store FunctionInvocationsStore) *http.ServeMux {
	h := NewHandler(nil, logr.Discard())
	h.SetFunctionInvocationsService(NewFunctionInvocationsService(store, logr.Discard()))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

func TestHandleCreateFunctionInvocation_Success(t *testing.T) {
	store := &mockFunctionInvocationsStore{}
	mux := newFunctionInvocationsHandler(store)

	body := `{"id":"inv-1","namespace":"` + fiTestNamespaceA + `","functionName":"summarizer","inputHash":"abc","outputJson":{"a":1},"status":"success","durationMs":12,"costUsd":0.001}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/function-invocations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, store.rows["inv-1"])
	assert.Equal(t, "summarizer", store.rows["inv-1"].FunctionName)
}

func TestHandleCreateFunctionInvocation_MissingRequired(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/function-invocations",
		strings.NewReader(`{"id":"inv-1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateFunctionInvocation_NoService(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/function-invocations",
		strings.NewReader(`{"id":"x","namespace":"y","functionName":"z","status":"success"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleGetFunctionInvocation_Success(t *testing.T) {
	store := &mockFunctionInvocationsStore{
		rows: map[string]*FunctionInvocation{
			"inv-1": {
				ID:           "inv-1",
				Namespace:    fiTestNamespaceA,
				FunctionName: "summarizer",
				Status:       FunctionInvocationStatusSuccess,
				DurationMs:   12,
				CreatedAt:    time.Now().UTC(),
			},
		},
	}
	mux := newFunctionInvocationsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/function-invocations/inv-1?namespace=ns-a", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got FunctionInvocation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "inv-1", got.ID)
}

func TestHandleGetFunctionInvocation_NotFound(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/function-invocations/missing?namespace=ns-a", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleGetFunctionInvocation_MissingNamespace(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/function-invocations/inv-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleListFunctionInvocations_Success(t *testing.T) {
	now := time.Now().UTC()
	store := &mockFunctionInvocationsStore{
		listResult: []*FunctionInvocation{
			{ID: "a", Namespace: fiTestNamespaceA, FunctionName: "f", Status: FunctionInvocationStatusSuccess, CreatedAt: now},
			{ID: "b", Namespace: fiTestNamespaceA, FunctionName: "f", Status: FunctionInvocationStatusSuccess, CreatedAt: now.Add(-time.Minute)},
		},
	}
	mux := newFunctionInvocationsHandler(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/function-invocations?namespace=ns-a&function=f&limit=10", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp FunctionInvocationsListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Rows, 2)
}

func TestHandleListFunctionInvocations_NilRowsReturnsEmpty(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{listResult: nil})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/function-invocations?namespace=ns-a", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"rows":[]`)
}

func TestHandleListFunctionInvocations_InvalidFrom(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{})
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/function-invocations?namespace=ns-a&from=tomorrow", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFunctionInvocationsService_NilStore(t *testing.T) {
	svc := NewFunctionInvocationsService(nil, logr.Discard())

	require.ErrorIs(t,
		svc.CreateFunctionInvocation(context.Background(), &FunctionInvocation{}),
		ErrMissingFunctionInvocationsStore)

	_, err := svc.GetFunctionInvocation(context.Background(), "ns", "id")
	require.ErrorIs(t, err, ErrMissingFunctionInvocationsStore)

	_, err = svc.ListFunctionInvocations(context.Background(), FunctionInvocationListOpts{Namespace: "ns"})
	require.ErrorIs(t, err, ErrMissingFunctionInvocationsStore)
}

func TestFunctionInvocationsService_BubblesStoreError(t *testing.T) {
	sentinel := errors.New("boom")
	svc := NewFunctionInvocationsService(
		&mockFunctionInvocationsStore{createErr: sentinel, getErr: sentinel, listErr: sentinel},
		logr.Discard())

	require.ErrorIs(t,
		svc.CreateFunctionInvocation(context.Background(), &FunctionInvocation{}),
		sentinel)
}

func TestHandleCreateFunctionInvocation_MalformedJSON(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/function-invocations",
		strings.NewReader(`{not json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCreateFunctionInvocation_StoreError(t *testing.T) {
	// Generic (non-sentinel) store error must fall through to writeError's
	// 500 default — this exercises the writeFunctionInvocationsError
	// default branch.
	store := &mockFunctionInvocationsStore{createErr: errors.New("boom")}
	mux := newFunctionInvocationsHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/function-invocations",
		strings.NewReader(fiCreateBody("inv-1", "f", FunctionInvocationStatusSuccess)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleCreateFunctionInvocation_DefaultsCreatedAtWhenZero(t *testing.T) {
	store := &mockFunctionInvocationsStore{}
	mux := newFunctionInvocationsHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/function-invocations",
		strings.NewReader(fiCreateBody("inv-1", "f", FunctionInvocationStatusSuccess)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, store.rows["inv-1"])
	assert.False(t, store.rows["inv-1"].CreatedAt.IsZero(),
		"handler must populate CreatedAt when the caller omits it")
}

func TestHandleListFunctionInvocations_MissingNamespace(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/function-invocations", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleListFunctionInvocations_StoreError(t *testing.T) {
	store := &mockFunctionInvocationsStore{listErr: errors.New("boom")}
	mux := newFunctionInvocationsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/function-invocations?namespace=ns-a", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleListFunctionInvocations_NoService(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/function-invocations?namespace=ns-a", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleGetFunctionInvocation_NoService(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/function-invocations/inv-1?namespace=ns-a", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleListFunctionInvocations_InvalidTo(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{})
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/function-invocations?namespace=ns-a&to=nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleListFunctionInvocations_InvalidLimit(t *testing.T) {
	mux := newFunctionInvocationsHandler(&mockFunctionInvocationsStore{})
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/function-invocations?namespace=ns-a&limit=-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleListFunctionInvocations_ValidFromAndToParse(t *testing.T) {
	// Exercise the success path for both From and To parse paths plus the
	// integer-limit path so coverage hits each branch.
	store := &mockFunctionInvocationsStore{
		listResult: []*FunctionInvocation{
			{ID: "a", Namespace: fiTestNamespaceA, FunctionName: "f",
				Status: FunctionInvocationStatusSuccess, CreatedAt: time.Now().UTC()},
		},
	}
	mux := newFunctionInvocationsHandler(store)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/function-invocations?namespace=ns-a&from=2026-01-01T00:00:00Z&to=2026-12-31T23:59:59Z&limit=5",
		nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}
