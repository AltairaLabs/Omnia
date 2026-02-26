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

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/pgutil"
	"github.com/altairalabs/omnia/internal/session/api"
)

const testSessionID = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

func newEvalStore(t *testing.T) *EvalStoreImpl {
	t.Helper()
	pool := freshDB(t)
	return NewEvalStore(pool)
}

// seedSession creates a session so that eval_results FK constraints are satisfied.
func seedSession(t *testing.T, store *EvalStoreImpl, sessionID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	_, err := store.pool.Exec(ctx, `INSERT INTO sessions (
		id, agent_name, namespace, workspace_name, status,
		created_at, updated_at, message_count, tool_call_count,
		total_input_tokens, total_output_tokens, estimated_cost_usd, tags
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		sessionID, "test-agent", "default", "test-workspace", "active",
		now, now, 0, 0, 0, 0, 0, []string{},
	)
	require.NoError(t, err)
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }
func ptrBool(v bool) *bool          { return &v }

func makeEvalResult(sessionID, evalID, evalType string) *api.EvalResult {
	return &api.EvalResult{
		SessionID:         sessionID,
		AgentName:         "test-agent",
		Namespace:         "default",
		PromptPackName:    "test-pack",
		PromptPackVersion: "v1.0",
		EvalID:            evalID,
		EvalType:          evalType,
		Trigger:           "on_message",
		Passed:            true,
		Score:             ptrFloat64(0.95),
		Details:           json.RawMessage(`{"reason":"looks good"}`),
		DurationMs:        ptrInt(150),
		JudgeTokens:       ptrInt(100),
		JudgeCostUSD:      ptrFloat64(0.001),
		Source:            "unit-test",
	}
}

// --- InsertEvalResults ------------------------------------------------------

func TestInsertEvalResults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sessionID := testSessionID
	seedSession(t, store, sessionID)

	results := []*api.EvalResult{
		makeEvalResult(sessionID, "eval-1", "llm_judge"),
		makeEvalResult(sessionID, "eval-2", "assertion"),
	}
	results[1].Passed = false
	results[1].Score = ptrFloat64(0.3)

	err := store.InsertEvalResults(ctx, results)
	require.NoError(t, err)

	// Verify they were stored.
	got, err := store.GetSessionEvalResults(ctx, sessionID)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestInsertEvalResults_EmptySlice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()

	// Empty slice produces invalid SQL (no VALUES), expect error or no-op.
	err := store.InsertEvalResults(ctx, []*api.EvalResult{})
	// With empty slice, the query has "VALUES " with no rows — Postgres returns a syntax error.
	assert.Error(t, err)
}

func TestInsertEvalResults_NullOptionals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sessionID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12"
	seedSession(t, store, sessionID)

	r := makeEvalResult(sessionID, "eval-null", "assertion")
	r.MessageID = ""
	r.PromptPackVersion = ""
	r.Score = nil
	r.Details = nil
	r.DurationMs = nil
	r.JudgeTokens = nil
	r.JudgeCostUSD = nil

	err := store.InsertEvalResults(ctx, []*api.EvalResult{r})
	require.NoError(t, err)

	got, err := store.GetSessionEvalResults(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Empty(t, got[0].MessageID)
	assert.Empty(t, got[0].PromptPackVersion)
	assert.Nil(t, got[0].Score)
	assert.Nil(t, got[0].Details)
	assert.Nil(t, got[0].DurationMs)
}

// --- GetSessionEvalResults --------------------------------------------------

func TestGetSessionEvalResults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sessionID := testSessionID
	seedSession(t, store, sessionID)

	results := []*api.EvalResult{
		makeEvalResult(sessionID, "eval-1", "llm_judge"),
		makeEvalResult(sessionID, "eval-2", "assertion"),
	}
	results[0].MessageID = "msg-001"

	require.NoError(t, store.InsertEvalResults(ctx, results))

	got, err := store.GetSessionEvalResults(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Should be ordered by created_at ASC.
	assert.Equal(t, "eval-1", got[0].EvalID)
	assert.Equal(t, "eval-2", got[1].EvalID)

	// Verify field mapping.
	assert.Equal(t, sessionID, got[0].SessionID)
	assert.Equal(t, "msg-001", got[0].MessageID)
	assert.Equal(t, "test-agent", got[0].AgentName)
	assert.Equal(t, "default", got[0].Namespace)
	assert.Equal(t, "test-pack", got[0].PromptPackName)
	assert.Equal(t, "v1.0", got[0].PromptPackVersion)
	assert.Equal(t, "llm_judge", got[0].EvalType)
	assert.Equal(t, "on_message", got[0].Trigger)
	assert.True(t, got[0].Passed)
	assert.InDelta(t, 0.95, *got[0].Score, 0.001)
	assert.NotNil(t, got[0].Details)
	assert.Equal(t, "unit-test", got[0].Source)
	assert.False(t, got[0].CreatedAt.IsZero())
	assert.NotEmpty(t, got[0].ID)
}

func TestGetSessionEvalResults_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sessionID := testSessionID
	seedSession(t, store, sessionID)

	got, err := store.GetSessionEvalResults(ctx, sessionID)
	require.NoError(t, err)
	assert.Empty(t, got)
	assert.NotNil(t, got, "should return empty slice, not nil")
}

// --- ListEvalResults --------------------------------------------------------

func TestListEvalResults_NoFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sessionID := testSessionID
	seedSession(t, store, sessionID)

	for i := range 3 {
		r := makeEvalResult(sessionID, "eval-list", "assertion")
		r.Passed = i%2 == 0
		require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r}))
	}

	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, results, 3)
}

func TestListEvalResults_FilterAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid1 := testSessionID
	sid2 := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12"
	seedSession(t, store, sid1)
	seedSession(t, store, sid2)

	r1 := makeEvalResult(sid1, "eval-a", "assertion")
	r1.AgentName = "agent-alpha"
	r2 := makeEvalResult(sid2, "eval-b", "assertion")
	r2.AgentName = "agent-beta"
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1}))
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r2}))

	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{AgentName: "agent-alpha"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)
	assert.Equal(t, "agent-alpha", results[0].AgentName)
}

func TestListEvalResults_FilterNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	r1 := makeEvalResult(sid, "eval-ns", "assertion")
	r1.Namespace = "ns-prod"
	r2 := makeEvalResult(sid, "eval-ns2", "assertion")
	r2.Namespace = "ns-staging"
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1}))
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r2}))

	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{Namespace: "ns-prod"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)
	assert.Equal(t, "ns-prod", results[0].Namespace)
}

func TestListEvalResults_FilterEvalID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	r1 := makeEvalResult(sid, "eval-specific", "assertion")
	r2 := makeEvalResult(sid, "eval-other", "assertion")
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1}))
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r2}))

	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{EvalID: "eval-specific"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)
	assert.Equal(t, "eval-specific", results[0].EvalID)
}

func TestListEvalResults_FilterPassed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	rPassed := makeEvalResult(sid, "eval-pass", "assertion")
	rPassed.Passed = true
	rFailed := makeEvalResult(sid, "eval-fail", "assertion")
	rFailed.Passed = false
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{rPassed, rFailed}))

	// Filter passed=true.
	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{Passed: ptrBool(true)})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.True(t, results[0].Passed)

	// Filter passed=false.
	results, total, err = store.ListEvalResults(ctx, api.EvalResultListOpts{Passed: ptrBool(false)})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.False(t, results[0].Passed)
}

func TestListEvalResults_FilterDateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	// Insert a result and record the time window.
	now := time.Now().UTC()
	r := makeEvalResult(sid, "eval-date", "assertion")
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r}))

	// Should find result created after "before now".
	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{
		CreatedAfter: now.Add(-time.Minute),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)

	// Should not find result created after "far future".
	results, total, err = store.ListEvalResults(ctx, api.EvalResultListOpts{
		CreatedAfter: now.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total)
	assert.Empty(t, results)

	// CreatedBefore filter.
	results, total, err = store.ListEvalResults(ctx, api.EvalResultListOpts{
		CreatedBefore: now.Add(time.Minute),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)
}

func TestListEvalResults_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	for i := range 5 {
		r := makeEvalResult(sid, "eval-page", "assertion")
		r.Score = ptrFloat64(float64(i))
		require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r}))
	}

	// Page 1.
	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{Limit: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, results, 2)

	// Page 2.
	results, total, err = store.ListEvalResults(ctx, api.EvalResultListOpts{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, results, 2)

	// Last page.
	results, total, err = store.ListEvalResults(ctx, api.EvalResultListOpts{Limit: 2, Offset: 4})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, results, 1)
}

func TestListEvalResults_CombinedFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	r1 := makeEvalResult(sid, "eval-combo", "assertion")
	r1.AgentName = "agent-x"
	r1.Namespace = "ns-x"
	r1.Passed = true

	r2 := makeEvalResult(sid, "eval-combo", "assertion")
	r2.AgentName = "agent-x"
	r2.Namespace = "ns-y"
	r2.Passed = false

	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1, r2}))

	// Agent + Namespace + Passed.
	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{
		AgentName: "agent-x",
		Namespace: "ns-x",
		Passed:    ptrBool(true),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)
	assert.Equal(t, "ns-x", results[0].Namespace)
}

// --- GetEvalResultSummary ---------------------------------------------------

func TestGetEvalResultSummary(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	// Insert 3 passed + 1 failed for eval-sum-1.
	for i := range 4 {
		r := makeEvalResult(sid, "eval-sum-1", "llm_judge")
		r.Passed = i < 3
		r.Score = ptrFloat64(float64(i) * 0.25)
		r.DurationMs = ptrInt(100 + i*50)
		require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r}))
	}

	// Insert 1 for eval-sum-2.
	r := makeEvalResult(sid, "eval-sum-2", "assertion")
	r.Passed = true
	r.Score = ptrFloat64(1.0)
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r}))

	summaries, err := store.GetEvalResultSummary(ctx, api.EvalResultSummaryOpts{})
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	// Find eval-sum-1 summary.
	var sum1 *api.EvalResultSummary
	for _, s := range summaries {
		if s.EvalID == "eval-sum-1" {
			sum1 = s
			break
		}
	}
	require.NotNil(t, sum1)
	assert.Equal(t, 4, sum1.Total)
	assert.Equal(t, 3, sum1.Passed)
	assert.Equal(t, 1, sum1.Failed)
	assert.InDelta(t, 0.75, sum1.PassRate, 0.001)
	assert.NotNil(t, sum1.AvgScore)
	assert.NotNil(t, sum1.AvgDurationMs)
}

func TestGetEvalResultSummary_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()

	summaries, err := store.GetEvalResultSummary(ctx, api.EvalResultSummaryOpts{})
	require.NoError(t, err)
	assert.Empty(t, summaries)
	assert.NotNil(t, summaries, "should return empty slice, not nil")
}

func TestGetEvalResultSummary_FilterAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	r1 := makeEvalResult(sid, "eval-agent-filter", "assertion")
	r1.AgentName = "agent-filtered"
	r2 := makeEvalResult(sid, "eval-agent-filter", "assertion")
	r2.AgentName = "agent-other"
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1}))
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r2}))

	summaries, err := store.GetEvalResultSummary(ctx, api.EvalResultSummaryOpts{AgentName: "agent-filtered"})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, 1, summaries[0].Total)
}

func TestGetEvalResultSummary_FilterNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	r1 := makeEvalResult(sid, "eval-ns-filter", "assertion")
	r1.Namespace = "ns-target"
	r2 := makeEvalResult(sid, "eval-ns-filter", "assertion")
	r2.Namespace = "ns-other"
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1}))
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r2}))

	summaries, err := store.GetEvalResultSummary(ctx, api.EvalResultSummaryOpts{Namespace: "ns-target"})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, 1, summaries[0].Total)
}

func TestGetEvalResultSummary_FilterDateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	now := time.Now().UTC()
	r := makeEvalResult(sid, "eval-date-sum", "assertion")
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r}))

	// Should find with broad range.
	summaries, err := store.GetEvalResultSummary(ctx, api.EvalResultSummaryOpts{
		CreatedAfter:  now.Add(-time.Minute),
		CreatedBefore: now.Add(time.Minute),
	})
	require.NoError(t, err)
	assert.Len(t, summaries, 1)

	// Should not find with future range.
	summaries, err = store.GetEvalResultSummary(ctx, api.EvalResultSummaryOpts{
		CreatedAfter: now.Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Empty(t, summaries)
}

// --- ListEvalResults with EvalType filter -----------------------------------

func TestListEvalResults_FilterEvalType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	r1 := makeEvalResult(sid, "eval-type-1", "assertion")
	r2 := makeEvalResult(sid, "eval-type-2", "llm_judge")
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1}))
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r2}))

	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{EvalType: "assertion"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)
	assert.Equal(t, "assertion", results[0].EvalType)
}

func TestListEvalResults_FilterEvalTypeWithOtherFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	r1 := makeEvalResult(sid, "eval-combo-type", "assertion")
	r1.AgentName = "agent-combo"
	r1.Passed = true
	r2 := makeEvalResult(sid, "eval-combo-type", "llm_judge")
	r2.AgentName = "agent-combo"
	r2.Passed = true
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1, r2}))

	results, total, err := store.ListEvalResults(ctx, api.EvalResultListOpts{
		AgentName: "agent-combo",
		EvalType:  "llm_judge",
		Passed:    ptrBool(true),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	assert.Len(t, results, 1)
	assert.Equal(t, "llm_judge", results[0].EvalType)
}

// --- GetEvalResultSummary with EvalType filter ------------------------------

func TestGetEvalResultSummary_FilterEvalType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	store := newEvalStore(t)
	ctx := context.Background()
	sid := testSessionID
	seedSession(t, store, sid)

	r1 := makeEvalResult(sid, "eval-st-1", "assertion")
	r2 := makeEvalResult(sid, "eval-st-2", "llm_judge")
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r1}))
	require.NoError(t, store.InsertEvalResults(ctx, []*api.EvalResult{r2}))

	summaries, err := store.GetEvalResultSummary(ctx, api.EvalResultSummaryOpts{EvalType: "assertion"})
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.Equal(t, "assertion", summaries[0].EvalType)
}

// --- Helper unit tests ------------------------------------------------------

func TestNullJSONB(t *testing.T) {
	assert.Nil(t, nullJSONB(nil))
	assert.Nil(t, nullJSONB(json.RawMessage{}))
	assert.Equal(t, []byte(`{"a":1}`), nullJSONB(json.RawMessage(`{"a":1}`)))
}

func TestApplyEvalFilters(t *testing.T) {
	qb := &pgutil.QueryBuilder{}
	now := time.Now()
	applyEvalFilters(qb, api.EvalResultListOpts{
		AgentName:     "agent",
		Namespace:     "ns",
		EvalID:        "eid",
		EvalType:      "assertion",
		Passed:        ptrBool(true),
		CreatedAfter:  now.Add(-time.Hour),
		CreatedBefore: now,
	})
	assert.Len(t, qb.Args(), 7)
	assert.Contains(t, qb.Where(), "agent_name=$1")
}

func TestApplySummaryFilters(t *testing.T) {
	qb := &pgutil.QueryBuilder{}
	now := time.Now()
	applySummaryFilters(qb, api.EvalResultSummaryOpts{
		AgentName:     "agent",
		Namespace:     "ns",
		EvalType:      "assertion",
		CreatedAfter:  now.Add(-time.Hour),
		CreatedBefore: now,
	})
	assert.Len(t, qb.Args(), 5)
	assert.Contains(t, qb.Where(), "agent_name=$1")
}

func TestApplyEvalFilters_Empty(t *testing.T) {
	qb := &pgutil.QueryBuilder{}
	applyEvalFilters(qb, api.EvalResultListOpts{})
	assert.Empty(t, qb.Args())
	assert.Empty(t, qb.Where())
}

func TestApplySummaryFilters_Empty(t *testing.T) {
	qb := &pgutil.QueryBuilder{}
	applySummaryFilters(qb, api.EvalResultSummaryOpts{})
	assert.Empty(t, qb.Args())
	assert.Empty(t, qb.Where())
}
