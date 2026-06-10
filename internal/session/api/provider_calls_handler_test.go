/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test fixture constants — extracted to satisfy goconst.
const (
	testProviderOpenAI    = "openai"
	testProviderAnthropic = "anthropic"
)

// mockProviderCallsStore is a test double for ProviderCallsStore.
type mockProviderCallsStore struct {
	aggregateRows []*ProviderCallAggregateRow
	aggregateErr  error
	discovery     *ProviderCallDiscoveryResult
	discoveryErr  error
}

func (m *mockProviderCallsStore) AggregateProviderCalls(_ context.Context, _ ProviderCallAggregateOpts) ([]*ProviderCallAggregateRow, error) {
	return m.aggregateRows, m.aggregateErr
}

func (m *mockProviderCallsStore) ProviderCallsDiscovery(_ context.Context, _ string) (*ProviderCallDiscoveryResult, error) {
	return m.discovery, m.discoveryErr
}

func newTestProviderCallsHandler(store ProviderCallsStore) *http.ServeMux {
	svc := NewProviderCallsService(store, logr.Discard())
	h := NewHandler(nil, logr.Discard())
	h.SetProviderCallsService(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return mux
}

// --- /api/v1/provider-calls/aggregate -------------------------------------

func TestHandleAggregateProviderCalls_Success(t *testing.T) {
	store := &mockProviderCallsStore{
		aggregateRows: []*ProviderCallAggregateRow{
			{Key: testProviderOpenAI, Value: 0.031, Count: 3},
			{Key: testProviderAnthropic, Value: 0.05, Count: 1},
		},
	}
	mux := newTestProviderCallsHandler(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?namespace=default&groupBy=provider&metric=sum_cost_usd", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp ProviderCallsAggregateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Rows, 2)
	assert.Equal(t, testProviderOpenAI, resp.Rows[0].Key)
}

func TestHandleAggregateProviderCalls_MissingNamespace(t *testing.T) {
	mux := newTestProviderCallsHandler(&mockProviderCallsStore{})
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?groupBy=provider&metric=count", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "namespace")
}

func TestHandleAggregateProviderCalls_InvalidGroupBy(t *testing.T) {
	mux := newTestProviderCallsHandler(&mockProviderCallsStore{})
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?namespace=default&groupBy=invalid&metric=count", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "groupBy")
}

func TestHandleAggregateProviderCalls_InvalidMetric(t *testing.T) {
	mux := newTestProviderCallsHandler(&mockProviderCallsStore{})
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?namespace=default&groupBy=provider&metric=invalid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "metric")
}

func TestHandleAggregateProviderCalls_InvalidFrom(t *testing.T) {
	mux := newTestProviderCallsHandler(&mockProviderCallsStore{})
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?namespace=default&groupBy=provider&metric=count&from=tomorrow", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "from")
}

func TestHandleAggregateProviderCalls_NoService(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?namespace=default&groupBy=provider&metric=count", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleAggregateProviderCalls_NilRowsReturnsEmptyArray(t *testing.T) {
	mux := newTestProviderCallsHandler(&mockProviderCallsStore{aggregateRows: nil})
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?namespace=default&groupBy=provider&metric=count", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"rows":[]`)
}

// --- /api/v1/provider-calls/discover --------------------------------------

func TestHandleDiscoverProviderCalls_Success(t *testing.T) {
	store := &mockProviderCallsStore{
		discovery: &ProviderCallDiscoveryResult{
			Providers: []string{testProviderAnthropic, testProviderOpenAI},
			Models:    []string{"claude-3-5-sonnet", "gpt-4", "gpt-4o-mini"},
		},
	}
	mux := newTestProviderCallsHandler(store)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/discover?namespace=default", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp ProviderCallDiscoveryResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []string{testProviderAnthropic, testProviderOpenAI}, resp.Providers)
	assert.Equal(t, []string{"claude-3-5-sonnet", "gpt-4", "gpt-4o-mini"}, resp.Models)
}

func TestHandleDiscoverProviderCalls_MissingNamespace(t *testing.T) {
	mux := newTestProviderCallsHandler(&mockProviderCallsStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider-calls/discover", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "namespace")
}

func TestHandleDiscoverProviderCalls_NoService(t *testing.T) {
	h := NewHandler(nil, logr.Discard())
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider-calls/discover?namespace=default", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestHandleDiscoverProviderCalls_NilReturnsEmptyArrays(t *testing.T) {
	mux := newTestProviderCallsHandler(&mockProviderCallsStore{discovery: nil})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider-calls/discover?namespace=default", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"providers":[]`)
	assert.Contains(t, body, `"providerNames":[]`)
	assert.Contains(t, body, `"models":[]`)
}

// --- ProviderCallsService -------------------------------------------------

func TestProviderCallsService_NilStore(t *testing.T) {
	svc := NewProviderCallsService(nil, logr.Discard())

	_, err := svc.AggregateProviderCalls(context.Background(), ProviderCallAggregateOpts{Namespace: "ns"})
	assert.ErrorIs(t, err, ErrMissingProviderCallsStore)

	_, err = svc.ProviderCallsDiscovery(context.Background(), "ns")
	assert.ErrorIs(t, err, ErrMissingProviderCallsStore)
}

// --- parseProviderCallsAggregateOpts --------------------------------------

func TestParseProviderCallsAggregateOpts_AllFields(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?namespace=ns&agentName=a&provider=openai&model=gpt-4&"+
			"groupBy=time:day&metric=sum_cost_usd&from=2026-05-01T00:00:00Z&to=2026-05-02T00:00:00Z&limit=100", nil)
	opts, err := parseProviderCallsAggregateOpts(req)
	require.NoError(t, err)
	assert.Equal(t, "ns", opts.Namespace)
	assert.Equal(t, "a", opts.AgentName)
	assert.Equal(t, testProviderOpenAI, opts.Provider)
	assert.Equal(t, "gpt-4", opts.Model)
	assert.Equal(t, []ProviderCallAggregateGroupBy{ProviderCallAggregateGroupByTimeDay}, opts.GroupBy)
	assert.Equal(t, ProviderCallAggregateMetricSumCostUSD, opts.Metric)
	assert.False(t, opts.From.IsZero())
	assert.False(t, opts.To.IsZero())
	assert.Equal(t, 100, opts.Limit)
}

func TestParseProviderCallsGroupByList_Single(t *testing.T) {
	got, err := parseProviderCallsGroupByList("provider")
	require.NoError(t, err)
	assert.Equal(t, []ProviderCallAggregateGroupBy{ProviderCallAggregateGroupByProvider}, got)
}

func TestParseProviderCallsGroupByList_Compound(t *testing.T) {
	got, err := parseProviderCallsGroupByList("time:hour,provider")
	require.NoError(t, err)
	assert.Equal(t, []ProviderCallAggregateGroupBy{
		ProviderCallAggregateGroupByTimeHour,
		ProviderCallAggregateGroupByProvider,
	}, got)
}

func TestParseProviderCallsGroupByList_TrimsSpaces(t *testing.T) {
	got, err := parseProviderCallsGroupByList(" provider , model ")
	require.NoError(t, err)
	assert.Equal(t, []ProviderCallAggregateGroupBy{
		ProviderCallAggregateGroupByProvider,
		ProviderCallAggregateGroupByModel,
	}, got)
}

func TestParseProviderCallsGroupByList_Empty(t *testing.T) {
	_, err := parseProviderCallsGroupByList("")
	require.ErrorIs(t, err, errProviderCallsBadGroupBy)
}

func TestParseProviderCallsGroupByList_InvalidMember(t *testing.T) {
	_, err := parseProviderCallsGroupByList("provider,bogus")
	require.ErrorIs(t, err, errProviderCallsBadGroupBy)
}

func TestParseProviderCallsAggregateOpts_InvalidTo(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/provider-calls/aggregate?namespace=ns&groupBy=provider&metric=count&to=bad", nil)
	_, err := parseProviderCallsAggregateOpts(req)
	require.ErrorIs(t, err, errAggregateBadTo)
}

func TestClampProviderCallsAggregateLimit(t *testing.T) {
	assert.Equal(t, DefaultProviderCallAggregateLimit, clampProviderCallsAggregateLimit(0))
	assert.Equal(t, DefaultProviderCallAggregateLimit, clampProviderCallsAggregateLimit(-1))
	assert.Equal(t, 42, clampProviderCallsAggregateLimit(42))
	assert.Equal(t, MaxProviderCallAggregateLimit,
		clampProviderCallsAggregateLimit(MaxProviderCallAggregateLimit+1000))
}
