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

package cold

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session/api"
)

// --- Mock EvalStore ---

// MockEvalStore implements api.EvalStore for testing.
type MockEvalStore struct {
	InsertFunc     func(ctx context.Context, results []*api.EvalResult) error
	GetSessionFunc func(ctx context.Context, sessionID string) ([]*api.EvalResult, error)
	ListFunc       func(ctx context.Context, opts api.EvalResultListOpts) ([]*api.EvalResult, int64, error)
	GetSummaryFunc func(ctx context.Context, opts api.EvalResultSummaryOpts) ([]*api.EvalResultSummary, error)
}

func (m *MockEvalStore) InsertEvalResults(ctx context.Context, results []*api.EvalResult) error {
	if m.InsertFunc != nil {
		return m.InsertFunc(ctx, results)
	}
	return nil
}

func (m *MockEvalStore) GetSessionEvalResults(
	ctx context.Context, sessionID string,
) ([]*api.EvalResult, error) {
	if m.GetSessionFunc != nil {
		return m.GetSessionFunc(ctx, sessionID)
	}
	return nil, nil
}

func (m *MockEvalStore) ListEvalResults(
	ctx context.Context, opts api.EvalResultListOpts,
) ([]*api.EvalResult, int64, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx, opts)
	}
	return nil, 0, nil
}

func (m *MockEvalStore) GetEvalResultSummary(
	ctx context.Context, opts api.EvalResultSummaryOpts,
) ([]*api.EvalResultSummary, error) {
	if m.GetSummaryFunc != nil {
		return m.GetSummaryFunc(ctx, opts)
	}
	return nil, nil
}

// Compile-time interface check.
var _ api.EvalStore = (*MockEvalStore)(nil)

// --- Helpers ---

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt(v int) *int             { return &v }

func makeEvalResult(id, sessionID, evalID string, passed bool, createdAt time.Time) *api.EvalResult {
	score := 0.95
	dur := 150
	return &api.EvalResult{
		ID:                id,
		SessionID:         sessionID,
		MessageID:         "msg-1",
		AgentName:         "agent-a",
		Namespace:         "default",
		PromptPackName:    "pp-1",
		PromptPackVersion: "v1",
		EvalID:            evalID,
		EvalType:          "llm-judge",
		Trigger:           "on-message",
		Passed:            passed,
		Score:             &score,
		Details:           json.RawMessage(`{"reason":"good"}`),
		DurationMs:        &dur,
		JudgeTokens:       ptrInt(200),
		JudgeCostUSD:      ptrFloat64(0.002),
		Source:            "runtime",
		CreatedAt:         createdAt,
	}
}

// --- ExportEvalResults tests ---

func TestExportEvalResults_WithResults(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)

	store := &MockEvalStore{
		GetSessionFunc: func(_ context.Context, sessionID string) ([]*api.EvalResult, error) {
			if sessionID == "sess-1" {
				return []*api.EvalResult{
					makeEvalResult("e1", "sess-1", "eval-a", true, now),
					makeEvalResult("e2", "sess-1", "eval-b", false, now.Add(time.Second)),
				}, nil
			}
			return []*api.EvalResult{
				makeEvalResult("e3", "sess-2", "eval-a", true, now),
			}, nil
		},
	}

	records, err := ExportEvalResults(ctx, []string{"sess-1", "sess-2"}, store)
	if err != nil {
		t.Fatalf("ExportEvalResults: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// Verify first record fields.
	r := records[0]
	if r.ID != "e1" {
		t.Errorf("ID: got %q, want e1", r.ID)
	}
	if r.SessionID != "sess-1" {
		t.Errorf("SessionID: got %q, want sess-1", r.SessionID)
	}
	if r.EvalID != "eval-a" {
		t.Errorf("EvalID: got %q, want eval-a", r.EvalID)
	}
	if !r.Passed {
		t.Error("Passed: got false, want true")
	}
	if r.Score == nil || *r.Score != 0.95 {
		t.Errorf("Score: got %v, want 0.95", r.Score)
	}
	if r.Details == nil {
		t.Error("Details: expected non-nil")
	}
	if r.AgentName != "agent-a" {
		t.Errorf("AgentName: got %q, want agent-a", r.AgentName)
	}
	if r.DurationMs == nil || *r.DurationMs != 150 {
		t.Errorf("DurationMs: got %v, want 150", r.DurationMs)
	}
}

func TestExportEvalResults_EmptySessionIDs(t *testing.T) {
	ctx := context.Background()
	store := &MockEvalStore{}

	records, err := ExportEvalResults(ctx, nil, store)
	if err != nil {
		t.Fatalf("ExportEvalResults: %v", err)
	}
	if records != nil {
		t.Errorf("expected nil, got %v", records)
	}
}

func TestExportEvalResults_EmptySlice(t *testing.T) {
	ctx := context.Background()
	store := &MockEvalStore{}

	records, err := ExportEvalResults(ctx, []string{}, store)
	if err != nil {
		t.Fatalf("ExportEvalResults: %v", err)
	}
	if records != nil {
		t.Errorf("expected nil, got %v", records)
	}
}

func TestExportEvalResults_NoResultsForSession(t *testing.T) {
	ctx := context.Background()
	store := &MockEvalStore{
		GetSessionFunc: func(_ context.Context, _ string) ([]*api.EvalResult, error) {
			return nil, nil
		},
	}

	records, err := ExportEvalResults(ctx, []string{"sess-1"}, store)
	if err != nil {
		t.Fatalf("ExportEvalResults: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestExportEvalResults_StoreError(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("database error")
	store := &MockEvalStore{
		GetSessionFunc: func(_ context.Context, _ string) ([]*api.EvalResult, error) {
			return nil, storeErr
		},
	}

	_, err := ExportEvalResults(ctx, []string{"sess-1"}, store)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("expected %v, got %v", storeErr, err)
	}
}

func TestExportEvalResults_PartialError(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	storeErr := errors.New("session-2 failed")

	store := &MockEvalStore{
		GetSessionFunc: func(_ context.Context, sessionID string) ([]*api.EvalResult, error) {
			if sessionID == "sess-1" {
				return []*api.EvalResult{
					makeEvalResult("e1", "sess-1", "eval-a", true, now),
				}, nil
			}
			return nil, storeErr
		},
	}

	_, err := ExportEvalResults(ctx, []string{"sess-1", "sess-2"}, store)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("expected %v, got %v", storeErr, err)
	}
}

// --- evalResultToRecord ---

// assertStringField is a test helper that checks a string field value.
func assertStringField(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}

func TestEvalResultToRecord_AllFields(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	result := makeEvalResult("e1", "sess-1", "eval-a", true, now)

	rec := evalResultToRecord(result)

	// Verify string fields.
	assertStringField(t, "ID", rec.ID, "e1")
	assertStringField(t, "SessionID", rec.SessionID, "sess-1")
	assertStringField(t, "MessageID", rec.MessageID, "msg-1")
	assertStringField(t, "EvalID", rec.EvalID, "eval-a")
	assertStringField(t, "EvalType", rec.EvalType, "llm-judge")
	assertStringField(t, "Trigger", rec.Trigger, "on-message")
	assertStringField(t, "Source", rec.Source, "runtime")
	assertStringField(t, "PromptPackName", rec.PromptPackName, "pp-1")
	assertStringField(t, "PromptPackVersion", rec.PromptPackVersion, "v1")
	assertStringField(t, "Namespace", rec.Namespace, "default")
	assertStringField(t, "AgentName", rec.AgentName, "agent-a")

	// Verify non-string fields.
	if !rec.Passed {
		t.Error("Passed: got false, want true")
	}
	if rec.Score == nil || *rec.Score != 0.95 {
		t.Errorf("Score: got %v, want 0.95", rec.Score)
	}
	if rec.Details == nil {
		t.Error("Details: expected non-nil")
	}
	if rec.DurationMs == nil || *rec.DurationMs != 150 {
		t.Errorf("DurationMs: got %v, want 150", rec.DurationMs)
	}
	if rec.JudgeTokens == nil || *rec.JudgeTokens != 200 {
		t.Errorf("JudgeTokens: got %v, want 200", rec.JudgeTokens)
	}
	if rec.JudgeCostUSD == nil || *rec.JudgeCostUSD != 0.002 {
		t.Errorf("JudgeCostUSD: got %v, want 0.002", rec.JudgeCostUSD)
	}
	if !rec.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt: got %v, want %v", rec.CreatedAt, now)
	}
}

func TestEvalResultToRecord_NilOptionalFields(t *testing.T) {
	now := time.Now().UTC()
	result := &api.EvalResult{
		ID:        "e1",
		SessionID: "sess-1",
		AgentName: "agent-a",
		Namespace: "default",
		EvalID:    "eval-a",
		EvalType:  "regex",
		Trigger:   "on-end",
		Passed:    false,
		Source:    "facade",
		CreatedAt: now,
	}

	rec := evalResultToRecord(result)

	if rec.Score != nil {
		t.Errorf("Score: expected nil, got %v", rec.Score)
	}
	if rec.Details != nil {
		t.Errorf("Details: expected nil, got %v", rec.Details)
	}
	if rec.DurationMs != nil {
		t.Errorf("DurationMs: expected nil, got %v", rec.DurationMs)
	}
	if rec.JudgeTokens != nil {
		t.Errorf("JudgeTokens: expected nil, got %v", rec.JudgeTokens)
	}
	if rec.JudgeCostUSD != nil {
		t.Errorf("JudgeCostUSD: expected nil, got %v", rec.JudgeCostUSD)
	}
	if rec.MessageID != "" {
		t.Errorf("MessageID: expected empty, got %q", rec.MessageID)
	}
	if rec.PromptPackVersion != "" {
		t.Errorf("PromptPackVersion: expected empty, got %q", rec.PromptPackVersion)
	}
}

// --- marshalJSONLines ---

func TestMarshalJSONLines(t *testing.T) {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	records := []EvalExportRecord{
		{ID: "e1", SessionID: "s1", EvalID: "eval-a", Passed: true, CreatedAt: now},
		{ID: "e2", SessionID: "s1", EvalID: "eval-b", Passed: false, CreatedAt: now},
	}

	data, err := marshalJSONLines(records)
	if err != nil {
		t.Fatalf("marshalJSONLines: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Verify each line is valid JSON.
	for i, line := range lines {
		var rec EvalExportRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}
	}

	// Verify content.
	var first EvalExportRecord
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
	if first.ID != "e1" {
		t.Errorf("first record ID: got %q, want e1", first.ID)
	}
}

func TestMarshalJSONLines_Empty(t *testing.T) {
	data, err := marshalJSONLines(nil)
	if err != nil {
		t.Fatalf("marshalJSONLines: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d bytes", len(data))
	}
}

// --- evalExportKey ---

func TestEvalExportKey(t *testing.T) {
	ts := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	got := evalExportKey("sessions/", ts)
	want := "sessions/evals/year=2026/month=02/day=15/eval_results.jsonl"
	if got != want {
		t.Errorf("evalExportKey: got %q, want %q", got, want)
	}
}

func TestEvalExportKey_NonUTC(t *testing.T) {
	loc := time.FixedZone("EST", -5*3600)
	// Feb 15 at 2am EST = Feb 15 at 7am UTC
	ts := time.Date(2026, 2, 15, 2, 0, 0, 0, loc)
	got := evalExportKey("data/", ts)
	want := "data/evals/year=2026/month=02/day=15/eval_results.jsonl"
	if got != want {
		t.Errorf("evalExportKey: got %q, want %q", got, want)
	}
}

// --- WriteEvalExport ---

func TestWriteEvalExport_Success(t *testing.T) {
	ctx := context.Background()
	p, store := newTestProvider(t)

	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	records := []EvalExportRecord{
		{ID: "e1", SessionID: "s1", EvalID: "eval-a", Passed: true, CreatedAt: now},
		{ID: "e2", SessionID: "s1", EvalID: "eval-b", Passed: false, CreatedAt: now},
	}

	err := p.WriteEvalExport(ctx, records, EvalExportOpts{})
	if err != nil {
		t.Fatalf("WriteEvalExport: %v", err)
	}

	// Verify the file was written.
	key := evalExportKey(testPrefix, now)
	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
}

func TestWriteEvalExport_EmptyRecords(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	err := p.WriteEvalExport(ctx, nil, EvalExportOpts{})
	if err != nil {
		t.Fatalf("WriteEvalExport: %v", err)
	}
}

func TestWriteEvalExport_BasePathOverride(t *testing.T) {
	ctx := context.Background()
	p, store := newTestProvider(t)

	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	records := []EvalExportRecord{
		{ID: "e1", SessionID: "s1", EvalID: "eval-a", CreatedAt: now},
	}

	err := p.WriteEvalExport(ctx, records, EvalExportOpts{
		BasePath: "custom/prefix/",
	})
	if err != nil {
		t.Fatalf("WriteEvalExport: %v", err)
	}

	key := evalExportKey("custom/prefix/", now)
	_, err = store.Get(ctx, key)
	if err != nil {
		t.Fatalf("expected file at custom path: %v", err)
	}
}

// --- Integration: ExportEvalResults + WriteEvalExport ---

func TestExportAndWriteEvalResults(t *testing.T) {
	ctx := context.Background()
	p, store := newTestProvider(t)

	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	evalStore := &MockEvalStore{
		GetSessionFunc: func(_ context.Context, _ string) ([]*api.EvalResult, error) {
			return []*api.EvalResult{
				makeEvalResult("e1", "sess-1", "eval-a", true, now),
			}, nil
		},
	}

	records, err := ExportEvalResults(ctx, []string{"sess-1"}, evalStore)
	if err != nil {
		t.Fatalf("ExportEvalResults: %v", err)
	}

	err = p.WriteEvalExport(ctx, records, EvalExportOpts{})
	if err != nil {
		t.Fatalf("WriteEvalExport: %v", err)
	}

	// Verify the file exists and contains the record.
	key := evalExportKey(testPrefix, now)
	data, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var rec EvalExportRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if rec.ID != "e1" {
		t.Errorf("ID: got %q, want e1", rec.ID)
	}
	if rec.SessionID != "sess-1" {
		t.Errorf("SessionID: got %q, want sess-1", rec.SessionID)
	}
	if !rec.Passed {
		t.Error("Passed: got false, want true")
	}
}
