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

package cold

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

const testPrefix = "sessions/"

// --- helpers ---

func newTestProvider(t *testing.T) (*Provider, *MemoryBlobStore) {
	t.Helper()
	store := NewMemoryBlobStore()
	p := NewFromBlobStore(store, DefaultOptions())
	return p, store
}

func makeSession(id, agent, ns string, createdAt time.Time) *session.Session {
	return &session.Session{
		ID:                 id,
		AgentName:          agent,
		Namespace:          ns,
		Status:             session.SessionStatusCompleted,
		CreatedAt:          createdAt,
		UpdatedAt:          createdAt.Add(time.Hour),
		MessageCount:       5,
		ToolCallCount:      2,
		TotalInputTokens:   1000,
		TotalOutputTokens:  500,
		EstimatedCostUSD:   0.05,
		Tags:               []string{"test", "unit"},
		State:              map[string]string{"key": "value"},
		LastMessagePreview: "hello world",
		Messages: []session.Message{
			{
				ID:        "msg-1",
				Role:      session.RoleUser,
				Content:   "Hello",
				Timestamp: createdAt,
			},
			{
				ID:        "msg-2",
				Role:      session.RoleAssistant,
				Content:   "Hi there!",
				Timestamp: createdAt.Add(time.Second),
			},
		},
	}
}

// --- MemoryBlobStore tests ---

func TestMemoryBlobStore_PutGetDelete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryBlobStore()

	// Put
	if err := store.Put(ctx, "key1", []byte("data1"), "text/plain"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	data, err := store.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != "data1" {
		t.Fatalf("Get: got %q, want %q", data, "data1")
	}

	// Exists
	exists, err := store.Exists(ctx, "key1")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Fatal("Exists: want true")
	}

	// Delete
	if err := store.Delete(ctx, "key1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Get after delete
	_, err = store.Get(ctx, "key1")
	if err != ErrObjectNotFound {
		t.Fatalf("Get after delete: got %v, want ErrObjectNotFound", err)
	}
}

func TestMemoryBlobStore_List(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryBlobStore()

	_ = store.Put(ctx, "a/1", []byte("1"), "")
	_ = store.Put(ctx, "a/2", []byte("2"), "")
	_ = store.Put(ctx, "b/1", []byte("3"), "")

	keys, err := store.List(ctx, "a/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("List: got %d keys, want 2", len(keys))
	}
}

func TestMemoryBlobStore_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryBlobStore()

	_, err := store.Get(ctx, "nonexistent")
	if err != ErrObjectNotFound {
		t.Fatalf("Get: got %v, want ErrObjectNotFound", err)
	}

	err = store.Delete(ctx, "nonexistent")
	if err != ErrObjectNotFound {
		t.Fatalf("Delete: got %v, want ErrObjectNotFound", err)
	}

	exists, err := store.Exists(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists {
		t.Fatal("Exists: want false")
	}
}

// --- Parquet tests ---

func TestSessionRowRoundtrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	s := makeSession("sess-1", "agent-a", "default", now)
	s.ExpiresAt = now.Add(24 * time.Hour)
	s.EndedAt = now.Add(2 * time.Hour)
	s.WorkspaceName = "ws-1"

	row := sessionToRow(s)
	got, err := rowToSession(row)
	if err != nil {
		t.Fatalf("rowToSession: %v", err)
	}

	// Compare key fields.
	if got.ID != s.ID {
		t.Errorf("ID: got %q, want %q", got.ID, s.ID)
	}
	if got.AgentName != s.AgentName {
		t.Errorf("AgentName: got %q, want %q", got.AgentName, s.AgentName)
	}
	if got.Namespace != s.Namespace {
		t.Errorf("Namespace: got %q, want %q", got.Namespace, s.Namespace)
	}
	if got.WorkspaceName != s.WorkspaceName {
		t.Errorf("WorkspaceName: got %q, want %q", got.WorkspaceName, s.WorkspaceName)
	}
	if got.Status != s.Status {
		t.Errorf("Status: got %q, want %q", got.Status, s.Status)
	}
	if got.MessageCount != s.MessageCount {
		t.Errorf("MessageCount: got %d, want %d", got.MessageCount, s.MessageCount)
	}
	if got.TotalInputTokens != s.TotalInputTokens {
		t.Errorf("TotalInputTokens: got %d, want %d", got.TotalInputTokens, s.TotalInputTokens)
	}
	if got.EstimatedCostUSD != s.EstimatedCostUSD {
		t.Errorf("EstimatedCostUSD: got %f, want %f", got.EstimatedCostUSD, s.EstimatedCostUSD)
	}
	if len(got.Tags) != len(s.Tags) {
		t.Errorf("Tags: got %v, want %v", got.Tags, s.Tags)
	}
	if len(got.State) != len(s.State) {
		t.Errorf("State: got %v, want %v", got.State, s.State)
	}
	if len(got.Messages) != len(s.Messages) {
		t.Errorf("Messages: got %d, want %d", len(got.Messages), len(s.Messages))
	}
	if got.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be zero")
	}
	if got.EndedAt.IsZero() {
		t.Error("EndedAt should not be zero")
	}
}

func TestParquetWriteRead(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	rows := []sessionRow{
		sessionToRow(makeSession("s1", "a1", "ns1", now)),
		sessionToRow(makeSession("s2", "a2", "ns2", now.Add(time.Hour))),
	}

	data, err := writeParquetBytes(rows)
	if err != nil {
		t.Fatalf("writeParquetBytes: %v", err)
	}

	got, err := readParquetBytes(data)
	if err != nil {
		t.Fatalf("readParquetBytes: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("readParquetBytes: got %d rows, want 2", len(got))
	}
	if got[0].ID != "s1" || got[1].ID != "s2" {
		t.Errorf("IDs: got %q, %q; want s1, s2", got[0].ID, got[1].ID)
	}
}

// --- Manifest tests ---

func TestManifestRoundtrip(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryBlobStore()
	prefix := testPrefix

	m := newManifest()
	m.Dates = []DateEntry{
		{Date: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), FileCount: 2, SessionCount: 10},
	}
	m.SessionIndex["s1"] = "sessions/year=2025/month=01/day=15/agent=a1/part-0000.parquet"

	if err := writeManifest(ctx, store, prefix, m); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	got, err := readManifest(ctx, store, prefix)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("Version: got %d, want 1", got.Version)
	}
	if len(got.Dates) != 1 {
		t.Fatalf("Dates: got %d, want 1", len(got.Dates))
	}
	if got.Dates[0].SessionCount != 10 {
		t.Errorf("SessionCount: got %d, want 10", got.Dates[0].SessionCount)
	}
	if got.SessionIndex["s1"] == "" {
		t.Error("SessionIndex: s1 not found")
	}
}

func TestManifestSessionIndex(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryBlobStore()
	prefix := testPrefix

	err := updateManifest(ctx, store, prefix, func(m *Manifest) {
		m.SessionIndex["s1"] = "file1.parquet"
		m.SessionIndex["s2"] = "file2.parquet"
	})
	if err != nil {
		t.Fatalf("updateManifest: %v", err)
	}

	m, err := readManifest(ctx, store, prefix)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	if len(m.SessionIndex) != 2 {
		t.Errorf("SessionIndex: got %d entries, want 2", len(m.SessionIndex))
	}
}

func TestReadManifest_InvalidJSON(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryBlobStore()
	prefix := testPrefix

	// Write invalid JSON as manifest.
	_ = store.Put(ctx, prefix+"_manifest.json", []byte("{invalid"), "application/json")

	_, err := readManifest(ctx, store, prefix)
	if err == nil {
		t.Fatal("readManifest: expected error for invalid JSON")
	}
}

func TestReadManifest_NilSessionIndex(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryBlobStore()
	prefix := testPrefix

	// Write a manifest without the sessionIndex field.
	_ = store.Put(ctx, prefix+"_manifest.json", []byte(`{"version":1,"dates":[]}`), "application/json")

	m, err := readManifest(ctx, store, prefix)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	if m.SessionIndex == nil {
		t.Fatal("SessionIndex should be initialized even when absent from JSON")
	}
}

// --- WriteParquet + GetSession ---

func TestWriteGetSession(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	s := makeSession("sess-1", "agent-a", "default", now)

	err := p.WriteParquet(ctx, []*session.Session{s}, providers.WriteOpts{})
	if err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	got, err := p.GetSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}

	if got.ID != "sess-1" {
		t.Errorf("ID: got %q, want %q", got.ID, "sess-1")
	}
	if got.AgentName != "agent-a" {
		t.Errorf("AgentName: got %q, want %q", got.AgentName, "agent-a")
	}
	if got.MessageCount != 5 {
		t.Errorf("MessageCount: got %d, want 5", got.MessageCount)
	}
	if len(got.Messages) != 2 {
		t.Errorf("Messages: got %d, want 2", len(got.Messages))
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags: got %v, want [test unit]", got.Tags)
	}
}

func TestWriteParquet_EmptySessions(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	err := p.WriteParquet(ctx, []*session.Session{}, providers.WriteOpts{})
	if err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	// No manifest should be created.
	m, err := readManifest(ctx, p.store, p.prefix)
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}
	if len(m.Dates) != 0 {
		t.Errorf("Dates: got %d, want 0", len(m.Dates))
	}
}

func TestWriteParquet_MultipleAgents(t *testing.T) {
	ctx := context.Background()
	p, store := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	s1 := makeSession("s1", "agent-a", "default", now)
	s2 := makeSession("s2", "agent-b", "default", now)

	err := p.WriteParquet(ctx, []*session.Session{s1, s2}, providers.WriteOpts{})
	if err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	// Should have separate Hive paths.
	keysA, _ := store.List(ctx, "sessions/year=2025/month=06/day=15/agent=agent-a/")
	keysB, _ := store.List(ctx, "sessions/year=2025/month=06/day=15/agent=agent-b/")
	if len(keysA) == 0 {
		t.Error("agent-a: no files written")
	}
	if len(keysB) == 0 {
		t.Error("agent-b: no files written")
	}

	// Both sessions should be retrievable.
	got1, err := p.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetSession s1: %v", err)
	}
	if got1.AgentName != "agent-a" {
		t.Errorf("s1 AgentName: got %q, want %q", got1.AgentName, "agent-a")
	}

	got2, err := p.GetSession(ctx, "s2")
	if err != nil {
		t.Fatalf("GetSession s2: %v", err)
	}
	if got2.AgentName != "agent-b" {
		t.Errorf("s2 AgentName: got %q, want %q", got2.AgentName, "agent-b")
	}
}

func TestWriteParquet_MaxFileSize(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryBlobStore()
	p := NewFromBlobStore(store, Options{
		Prefix:             testPrefix,
		DefaultCompression: "snappy",
		DefaultMaxFileSize: 1, // 1 byte forces splitting every session into its own file
	})

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	sessions := []*session.Session{
		makeSession("s1", "agent-a", "default", now),
		makeSession("s2", "agent-a", "default", now),
		makeSession("s3", "agent-a", "default", now),
	}

	err := p.WriteParquet(ctx, sessions, providers.WriteOpts{})
	if err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	keys, _ := store.List(ctx, "sessions/year=2025/month=06/day=15/agent=agent-a/")
	if len(keys) < 2 {
		t.Errorf("Expected multiple part files, got %d", len(keys))
	}

	// All sessions should still be retrievable.
	for _, id := range []string{"s1", "s2", "s3"} {
		if _, err := p.GetSession(ctx, id); err != nil {
			t.Errorf("GetSession %s: %v", id, err)
		}
	}
}

func TestGetSession_NotFound(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	_, err := p.GetSession(ctx, "nonexistent")
	if err != session.ErrSessionNotFound {
		t.Fatalf("GetSession: got %v, want ErrSessionNotFound", err)
	}
}

// --- ListAvailableDates ---

func TestListAvailableDates(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	d1 := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 7, 20, 14, 0, 0, 0, time.UTC)
	d3 := time.Date(2025, 5, 1, 8, 0, 0, 0, time.UTC)

	sessions := []*session.Session{
		makeSession("s1", "agent-a", "default", d1),
		makeSession("s2", "agent-a", "default", d2),
		makeSession("s3", "agent-a", "default", d3),
	}

	if err := p.WriteParquet(ctx, sessions, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	dates, err := p.ListAvailableDates(ctx)
	if err != nil {
		t.Fatalf("ListAvailableDates: %v", err)
	}

	if len(dates) != 3 {
		t.Fatalf("ListAvailableDates: got %d dates, want 3", len(dates))
	}

	// Should be sorted ascending.
	for i := 1; i < len(dates); i++ {
		if dates[i].Before(dates[i-1]) {
			t.Errorf("dates not sorted: %v before %v", dates[i], dates[i-1])
		}
	}
}

func TestListAvailableDates_Empty(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	dates, err := p.ListAvailableDates(ctx)
	if err != nil {
		t.Fatalf("ListAvailableDates: %v", err)
	}
	if len(dates) != 0 {
		t.Errorf("ListAvailableDates: got %d, want 0", len(dates))
	}
}

// --- QuerySessions ---

func TestQuerySessions_ByAgentName(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	sessions := []*session.Session{
		makeSession("s1", "agent-a", "default", now),
		makeSession("s2", "agent-b", "default", now),
		makeSession("s3", "agent-a", "default", now),
	}

	if err := p.WriteParquet(ctx, sessions, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	results, err := p.QuerySessions(ctx, "agent_name=agent-a")
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("QuerySessions: got %d, want 2", len(results))
	}
	for _, r := range results {
		if r.AgentName != "agent-a" {
			t.Errorf("AgentName: got %q, want agent-a", r.AgentName)
		}
	}
}

func TestQuerySessions_ByStatus(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	s1 := makeSession("s1", "agent-a", "default", now)
	s2 := makeSession("s2", "agent-a", "default", now)
	s2.Status = session.SessionStatusActive

	if err := p.WriteParquet(ctx, []*session.Session{s1, s2}, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	results, err := p.QuerySessions(ctx, "status=active")
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("QuerySessions: got %d, want 1", len(results))
	}
	if results[0].ID != "s2" {
		t.Errorf("ID: got %q, want s2", results[0].ID)
	}
}

func TestQuerySessions_ByDateRange(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	sessions := []*session.Session{
		makeSession("s1", "agent-a", "default", time.Date(2025, 1, 10, 10, 0, 0, 0, time.UTC)),
		makeSession("s2", "agent-a", "default", time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)),
		makeSession("s3", "agent-a", "default", time.Date(2025, 12, 20, 10, 0, 0, 0, time.UTC)),
	}

	if err := p.WriteParquet(ctx, sessions, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	results, err := p.QuerySessions(ctx, "created_after=2025-06-01T00:00:00Z created_before=2025-07-01T00:00:00Z")
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("QuerySessions: got %d, want 1", len(results))
	}
	if results[0].ID != "s2" {
		t.Errorf("ID: got %q, want s2", results[0].ID)
	}
}

func TestQuerySessions_MultipleFilters(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	s1 := makeSession("s1", "agent-a", "ns1", now)
	s2 := makeSession("s2", "agent-a", "ns2", now)
	s3 := makeSession("s3", "agent-b", "ns1", now)

	if err := p.WriteParquet(ctx, []*session.Session{s1, s2, s3}, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	results, err := p.QuerySessions(ctx, "agent_name=agent-a namespace=ns1")
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("QuerySessions: got %d, want 1", len(results))
	}
	if results[0].ID != "s1" {
		t.Errorf("ID: got %q, want s1", results[0].ID)
	}
}

func TestQuerySessions_NoResults(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	if err := p.WriteParquet(ctx, []*session.Session{makeSession("s1", "agent-a", "default", now)}, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	results, err := p.QuerySessions(ctx, "agent_name=nonexistent")
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("QuerySessions: got %d, want 0", len(results))
	}
}

func TestQuerySessions_EmptyQuery(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	sessions := []*session.Session{
		makeSession("s1", "agent-a", "default", now),
		makeSession("s2", "agent-b", "default", now),
	}

	if err := p.WriteParquet(ctx, sessions, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	results, err := p.QuerySessions(ctx, "")
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("QuerySessions: got %d, want 2", len(results))
	}
}

// --- DeleteOlderThan ---

func TestDeleteOlderThan(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	old := time.Date(2024, 1, 10, 10, 0, 0, 0, time.UTC)
	recent := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	sessions := []*session.Session{
		makeSession("s-old", "agent-a", "default", old),
		makeSession("s-new", "agent-a", "default", recent),
	}

	if err := p.WriteParquet(ctx, sessions, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	// Delete everything before 2025-01-01.
	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := p.DeleteOlderThan(ctx, cutoff); err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}

	// Old session should be gone.
	_, err := p.GetSession(ctx, "s-old")
	if err != session.ErrSessionNotFound {
		t.Errorf("GetSession s-old: got %v, want ErrSessionNotFound", err)
	}

	// New session should still exist.
	got, err := p.GetSession(ctx, "s-new")
	if err != nil {
		t.Fatalf("GetSession s-new: %v", err)
	}
	if got.ID != "s-new" {
		t.Errorf("ID: got %q, want s-new", got.ID)
	}

	// Dates should only include the recent one.
	dates, err := p.ListAvailableDates(ctx)
	if err != nil {
		t.Fatalf("ListAvailableDates: %v", err)
	}
	if len(dates) != 1 {
		t.Fatalf("ListAvailableDates: got %d, want 1", len(dates))
	}
}

func TestDeleteOlderThan_NothingToDelete(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	if err := p.WriteParquet(ctx, []*session.Session{makeSession("s1", "agent-a", "default", now)}, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	// Cutoff is way in the past - nothing should be deleted.
	cutoff := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := p.DeleteOlderThan(ctx, cutoff); err != nil {
		t.Fatalf("DeleteOlderThan: %v", err)
	}

	got, err := p.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.ID != "s1" {
		t.Errorf("ID: got %q, want s1", got.ID)
	}
}

// --- Ping / Close ---

func TestPing(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	if err := p.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestClose_OwnsStore(t *testing.T) {
	store := NewMemoryBlobStore()
	p := &Provider{
		store:     store,
		prefix:    testPrefix,
		ownsStore: true,
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !store.closed {
		t.Error("Close: store should be closed when ownsStore=true")
	}
}

func TestClose_SharedStore(t *testing.T) {
	store := NewMemoryBlobStore()
	p := &Provider{
		store:     store,
		prefix:    testPrefix,
		ownsStore: false,
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.closed {
		t.Error("Close: store should NOT be closed when ownsStore=false")
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	ctx := context.Background()

	_, err := New(ctx, Config{})
	if err == nil {
		t.Fatal("New: expected error for empty config")
	}

	_, err = New(ctx, Config{Bucket: "test", Backend: "invalid"})
	if err == nil {
		t.Fatal("New: expected error for invalid backend")
	}

	_, err = New(ctx, Config{Bucket: "test", Backend: BackendS3})
	if err == nil {
		t.Fatal("New: expected error when S3 config is nil")
	}

	_, err = New(ctx, Config{Bucket: "test", Backend: BackendAzure})
	if err == nil {
		t.Fatal("New: expected error when Azure config is nil")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Prefix != testPrefix {
		t.Errorf("Prefix: got %q, want %q", cfg.Prefix, testPrefix)
	}
	if cfg.DefaultCompression != "snappy" {
		t.Errorf("DefaultCompression: got %q, want %q", cfg.DefaultCompression, "snappy")
	}
	if cfg.DefaultMaxFileSize != 128*1024*1024 {
		t.Errorf("DefaultMaxFileSize: got %d, want %d", cfg.DefaultMaxFileSize, 128*1024*1024)
	}
}

// --- Query tag filtering ---

func TestQuerySessions_ByTag(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	s1 := makeSession("s1", "agent-a", "default", now)
	s1.Tags = []string{"production", "critical"}
	s2 := makeSession("s2", "agent-a", "default", now)
	s2.Tags = []string{"staging"}

	if err := p.WriteParquet(ctx, []*session.Session{s1, s2}, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	results, err := p.QuerySessions(ctx, "tag=production")
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("QuerySessions: got %d, want 1", len(results))
	}
	if results[0].ID != "s1" {
		t.Errorf("ID: got %q, want s1", results[0].ID)
	}
}

// --- splitRows ---

func TestSplitRows(t *testing.T) {
	rows := []sessionRow{
		{ID: "1"}, {ID: "2"}, {ID: "3"},
	}

	// No splitting needed.
	chunks := splitRows(rows, 1<<30)
	if len(chunks) != 1 {
		t.Errorf("splitRows (large max): got %d chunks, want 1", len(chunks))
	}

	// Force splitting.
	chunks = splitRows(rows, 1)
	if len(chunks) < 2 {
		t.Errorf("splitRows (tiny max): got %d chunks, want >= 2", len(chunks))
	}

	// Verify all rows are present.
	var all []sessionRow
	for _, c := range chunks {
		all = append(all, c...)
	}
	if len(all) != 3 {
		t.Errorf("splitRows: total rows %d, want 3", len(all))
	}
}

// --- parseQuery ---

func TestParseQuery(t *testing.T) {
	f := parseQuery("agent_name=foo namespace=bar status=active created_after=2025-01-01T00:00:00Z tag=v1 tag=v2")
	if f.agentName != "foo" {
		t.Errorf("agentName: got %q, want foo", f.agentName)
	}
	if f.namespace != "bar" {
		t.Errorf("namespace: got %q, want bar", f.namespace)
	}
	if f.status != "active" {
		t.Errorf("status: got %q, want active", f.status)
	}
	if f.createdAfter.IsZero() {
		t.Error("createdAfter: should not be zero")
	}
	if len(f.tags) != 2 {
		t.Errorf("tags: got %d, want 2", len(f.tags))
	}
}

// --- hivePath ---

func TestHivePath(t *testing.T) {
	s := &session.Session{
		AgentName: "my-agent",
		CreatedAt: time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
	}
	got := hivePath(testPrefix, s)
	want := "sessions/year=2025/month=06/day=15/agent=my-agent/"
	if got != want {
		t.Errorf("hivePath: got %q, want %q", got, want)
	}
}

func TestHivePath_SanitizesAgentName(t *testing.T) {
	s := &session.Session{
		AgentName: "agent with spaces/and-slashes",
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	got := hivePath(testPrefix, s)
	if got != "sessions/year=2025/month=01/day=01/agent=agent_with_spaces_and-slashes/" {
		t.Errorf("hivePath: got %q", got)
	}
}

// --- matchesFilters ---

func TestMatchesFilters(t *testing.T) {
	now := time.Now().UTC()
	row := sessionToRow(makeSession("s1", "agent-a", "ns1", now))
	row.Tags = mustJSON(t, []string{"prod", "v2"})

	tests := []struct {
		name    string
		filters queryFilters
		want    bool
	}{
		{"empty filters", queryFilters{}, true},
		{"matching agent", queryFilters{agentName: "agent-a"}, true},
		{"non-matching agent", queryFilters{agentName: "agent-b"}, false},
		{"matching namespace", queryFilters{namespace: "ns1"}, true},
		{"matching tag", queryFilters{tags: []string{"prod"}}, true},
		{"non-matching tag", queryFilters{tags: []string{"staging"}}, false},
		{"matching status", queryFilters{status: "completed"}, true},
		{"non-matching status", queryFilters{status: "active"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesFilters(row, tt.filters)
			if got != tt.want {
				t.Errorf("matchesFilters: got %v, want %v", got, tt.want)
			}
		})
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// --- sanitizeAgentName ---

func TestSanitizeAgentName(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"simple", "simple"},
		{"with spaces", "with_spaces"},
		{"with/slash", "with_slash"},
		{"ok-dashes_underscores", "ok-dashes_underscores"},
	}
	for _, tt := range tests {
		got := sanitizeAgentName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeAgentName(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- datePrefixForDate ---

func TestDatePrefixForDate(t *testing.T) {
	p, _ := newTestProvider(t)
	d := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	got := p.datePrefixForDate(d)
	want := "sessions/year=2025/month=06/day=15/"
	if got != want {
		t.Errorf("datePrefixForDate: got %q, want %q", got, want)
	}
}

// --- datePrefixesForQuery ---

func TestDatePrefixesForQuery(t *testing.T) {
	p, _ := newTestProvider(t)

	m := &Manifest{
		Dates: []DateEntry{
			{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)},
			{Date: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)},
			{Date: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)},
		},
	}

	// No filters - all dates.
	prefixes := p.datePrefixesForQuery(m, queryFilters{})
	if len(prefixes) != 3 {
		t.Errorf("no filters: got %d prefixes, want 3", len(prefixes))
	}

	// With date range.
	prefixes = p.datePrefixesForQuery(m, queryFilters{
		createdAfter:  time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		createdBefore: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
	})
	if len(prefixes) != 1 {
		t.Errorf("date range: got %d prefixes, want 1", len(prefixes))
	}
}

// --- DefaultOptions ---

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Prefix != testPrefix {
		t.Errorf("Prefix: got %q, want %q", opts.Prefix, testPrefix)
	}
	if opts.DefaultCompression != "snappy" {
		t.Errorf("DefaultCompression: got %q, want %q", opts.DefaultCompression, "snappy")
	}
	if opts.DefaultMaxFileSize != 128*1024*1024 {
		t.Errorf("DefaultMaxFileSize: got %d, want %d", opts.DefaultMaxFileSize, 128*1024*1024)
	}
}

// --- NewFromBlobStore ---

func TestNewFromBlobStore_AppliesDefaults(t *testing.T) {
	store := NewMemoryBlobStore()
	p := NewFromBlobStore(store, Options{})

	if p.prefix != testPrefix {
		t.Errorf("prefix: got %q, want %q", p.prefix, testPrefix)
	}
	if p.compression != "snappy" {
		t.Errorf("compression: got %q, want %q", p.compression, "snappy")
	}
	if p.ownsStore {
		t.Error("ownsStore: should be false")
	}
}

// --- WriteParquet with BasePath override ---

func TestWriteParquet_BasePathOverride(t *testing.T) {
	ctx := context.Background()
	p, store := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	s := makeSession("s1", "agent-a", "default", now)

	err := p.WriteParquet(ctx, []*session.Session{s}, providers.WriteOpts{
		BasePath: "custom/prefix/",
	})
	if err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	keys, _ := store.List(ctx, "custom/prefix/")
	if len(keys) == 0 {
		t.Error("expected files under custom/prefix/")
	}
}

// --- Write then query by workspace_name ---

func TestQuerySessions_ByWorkspaceName(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	s1 := makeSession("s1", "agent-a", "default", now)
	s1.WorkspaceName = "ws-prod"
	s2 := makeSession("s2", "agent-a", "default", now)
	s2.WorkspaceName = "ws-staging"

	if err := p.WriteParquet(ctx, []*session.Session{s1, s2}, providers.WriteOpts{}); err != nil {
		t.Fatalf("WriteParquet: %v", err)
	}

	results, err := p.QuerySessions(ctx, "workspace_name=ws-prod")
	if err != nil {
		t.Fatalf("QuerySessions: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("QuerySessions: got %d, want 1", len(results))
	}
	if results[0].ID != "s1" {
		t.Errorf("ID: got %q, want s1", results[0].ID)
	}
}

// --- Multiple writes to same date accumulate ---

func TestWriteParquet_AccumulatesOnSameDate(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// First write.
	err := p.WriteParquet(ctx, []*session.Session{makeSession("s1", "agent-a", "default", now)}, providers.WriteOpts{})
	if err != nil {
		t.Fatalf("WriteParquet 1: %v", err)
	}

	// Second write to same date.
	err = p.WriteParquet(ctx, []*session.Session{makeSession("s2", "agent-a", "default", now)}, providers.WriteOpts{})
	if err != nil {
		t.Fatalf("WriteParquet 2: %v", err)
	}

	// Both sessions should be retrievable.
	for _, id := range []string{"s1", "s2"} {
		if _, err := p.GetSession(ctx, id); err != nil {
			t.Errorf("GetSession %s: %v", id, err)
		}
	}

	// Should still be a single date entry.
	dates, err := p.ListAvailableDates(ctx)
	if err != nil {
		t.Fatalf("ListAvailableDates: %v", err)
	}
	if len(dates) != 1 {
		t.Errorf("ListAvailableDates: got %d, want 1", len(dates))
	}
}

// --- Verify sorted date output ---

func TestListAvailableDates_Sorted(t *testing.T) {
	ctx := context.Background()
	p, _ := newTestProvider(t)

	// Write in reverse chronological order.
	dates := []time.Time{
		time.Date(2025, 12, 31, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
	}
	for i, d := range dates {
		s := makeSession(fmt.Sprintf("s%d", i), "agent-a", "default", d)
		if err := p.WriteParquet(ctx, []*session.Session{s}, providers.WriteOpts{}); err != nil {
			t.Fatalf("WriteParquet: %v", err)
		}
	}

	result, err := p.ListAvailableDates(ctx)
	if err != nil {
		t.Fatalf("ListAvailableDates: %v", err)
	}

	if !sort.SliceIsSorted(result, func(i, j int) bool { return result[i].Before(result[j]) }) {
		t.Errorf("dates not sorted: %v", result)
	}
}
