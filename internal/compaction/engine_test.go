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

package compaction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/pkg/metrics"
)

// ---------------------------------------------------------------------------
// Mock providers
// ---------------------------------------------------------------------------

type mockWarmStore struct {
	sessions       []*session.Session
	deletedBatches [][]string
	deleteErr      error
}

func (m *mockWarmStore) GetSessionsOlderThan(_ context.Context, cutoff time.Time, batchSize int) ([]*session.Session, error) {
	var result []*session.Session
	for _, s := range m.sessions {
		if s.UpdatedAt.Before(cutoff) {
			result = append(result, s)
			if len(result) >= batchSize {
				break
			}
		}
	}
	return result, nil
}

func (m *mockWarmStore) DeleteSessionsBatch(_ context.Context, ids []string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedBatches = append(m.deletedBatches, ids)
	// Remove deleted sessions from the store.
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var remaining []*session.Session
	for _, s := range m.sessions {
		if !idSet[s.ID] {
			remaining = append(remaining, s)
		}
	}
	m.sessions = remaining
	return nil
}

// Unused interface methods — satisfy the interface.
func (m *mockWarmStore) CreateSession(context.Context, *session.Session) error { return nil }

func (m *mockWarmStore) GetSession(context.Context, string) (*session.Session, error) {
	return nil, nil
}

func (m *mockWarmStore) UpdateSession(context.Context, *session.Session) error { return nil }

func (m *mockWarmStore) UpdateSessionStats(context.Context, string, session.SessionStatsUpdate) error {
	return nil
}

func (m *mockWarmStore) DeleteSession(context.Context, string) error                   { return nil }
func (m *mockWarmStore) AppendMessage(context.Context, string, *session.Message) error { return nil }

func (m *mockWarmStore) GetMessages(context.Context, string, providers.MessageQueryOpts) ([]*session.Message, error) {
	return nil, nil
}

func (m *mockWarmStore) ListSessions(context.Context, providers.SessionListOpts) (*providers.SessionPage, error) {
	return nil, nil
}

func (m *mockWarmStore) SearchSessions(context.Context, string, providers.SessionListOpts) (*providers.SessionPage, error) {
	return nil, nil
}

func (m *mockWarmStore) CreatePartition(context.Context, time.Time) error { return nil }
func (m *mockWarmStore) DropPartition(context.Context, time.Time) error   { return nil }

func (m *mockWarmStore) ListPartitions(context.Context) ([]providers.PartitionInfo, error) {
	return nil, nil
}
func (m *mockWarmStore) SaveArtifact(context.Context, *session.Artifact) error { return nil }
func (m *mockWarmStore) GetArtifacts(context.Context, string) ([]*session.Artifact, error) {
	return []*session.Artifact{}, nil
}
func (m *mockWarmStore) GetSessionArtifacts(context.Context, string) ([]*session.Artifact, error) {
	return []*session.Artifact{}, nil
}
func (m *mockWarmStore) DeleteSessionArtifacts(context.Context, string) error { return nil }
func (m *mockWarmStore) Ping(context.Context) error                           { return nil }
func (m *mockWarmStore) Close() error                                         { return nil }

type mockColdArchive struct {
	written       [][]*session.Session
	writeErr      error
	writeErrOnce  bool // fail only the first call
	writeCount    int
	deletedBefore time.Time
	deleteErr     error
}

func (m *mockColdArchive) WriteParquet(_ context.Context, sessions []*session.Session, _ providers.WriteOpts) error {
	m.writeCount++
	if m.writeErr != nil {
		if m.writeErrOnce && m.writeCount > 1 {
			m.written = append(m.written, sessions)
			return nil
		}
		return m.writeErr
	}
	m.written = append(m.written, sessions)
	return nil
}

func (m *mockColdArchive) DeleteOlderThan(_ context.Context, cutoff time.Time) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.deletedBefore = cutoff
	return nil
}

func (m *mockColdArchive) GetSession(context.Context, string) (*session.Session, error) {
	return nil, nil
}
func (m *mockColdArchive) ListAvailableDates(context.Context) ([]time.Time, error) { return nil, nil }
func (m *mockColdArchive) QuerySessions(context.Context, string) ([]*session.Session, error) {
	return nil, nil
}
func (m *mockColdArchive) Ping(context.Context) error { return nil }
func (m *mockColdArchive) Close() error               { return nil }

type mockHotCache struct {
	invalidated   []string
	invalidateErr error
}

func (m *mockHotCache) Invalidate(_ context.Context, sessionID string) error {
	if m.invalidateErr != nil {
		return m.invalidateErr
	}
	m.invalidated = append(m.invalidated, sessionID)
	return nil
}

func (m *mockHotCache) GetSession(context.Context, string) (*session.Session, error) { return nil, nil }
func (m *mockHotCache) SetSession(context.Context, *session.Session, time.Duration) error {
	return nil
}
func (m *mockHotCache) DeleteSession(context.Context, string) error                   { return nil }
func (m *mockHotCache) AppendMessage(context.Context, string, *session.Message) error { return nil }
func (m *mockHotCache) GetRecentMessages(context.Context, string, int) ([]*session.Message, error) {
	return nil, nil
}
func (m *mockHotCache) RefreshTTL(context.Context, string, time.Duration) error { return nil }
func (m *mockHotCache) Ping(context.Context) error                              { return nil }
func (m *mockHotCache) Close() error                                            { return nil }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testLogger() *zap.SugaredLogger {
	l, _ := zap.NewDevelopment()
	return l.Sugar()
}

func testRetentionConfig() *RetentionConfig {
	days := int32(90)
	return &RetentionConfig{
		Default: TierConfig{
			WarmStore: &omniav1alpha1.WarmStoreConfig{RetentionDays: 7},
			ColdArchive: &omniav1alpha1.ColdArchiveConfig{
				Enabled:       true,
				RetentionDays: &days,
			},
		},
	}
}

func testSession(id, workspace string, updatedAt time.Time) *session.Session {
	return &session.Session{
		ID:            id,
		WorkspaceName: workspace,
		UpdatedAt:     updatedAt,
		CreatedAt:     updatedAt,
	}
}

func testConfig() Config {
	return Config{
		BatchSize:   100,
		MaxRetries:  1,
		RetryDelay:  time.Millisecond,
		Compression: "snappy",
	}
}

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestLoadRetentionConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "retention.yaml")
	content := `
default:
  warmStore:
    retentionDays: 14
  coldArchive:
    enabled: true
    retentionDays: 365
perWorkspace:
  staging:
    warmStore:
      retentionDays: 3
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadRetentionConfig(path)
	if err != nil {
		t.Fatalf("LoadRetentionConfig: %v", err)
	}
	if cfg.Default.WarmStore.RetentionDays != 14 {
		t.Errorf("expected default warm retention 14, got %d", cfg.Default.WarmStore.RetentionDays)
	}
	if !cfg.Default.ColdArchive.Enabled {
		t.Error("expected cold archive enabled")
	}
	if ws, ok := cfg.PerWorkspace["staging"]; !ok || ws.WarmStore.RetentionDays != 3 {
		t.Error("expected staging workspace with 3 day retention")
	}
}

func TestWarmCutoff_Default(t *testing.T) {
	cfg := testRetentionConfig()
	now := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	cutoff := cfg.WarmCutoff("unknown-workspace", now)
	expected := now.AddDate(0, 0, -7)
	if !cutoff.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, cutoff)
	}
}

func TestWarmCutoff_PerWorkspace(t *testing.T) {
	cfg := testRetentionConfig()
	cfg.PerWorkspace = map[string]TierConfig{
		"prod": {WarmStore: &omniav1alpha1.WarmStoreConfig{RetentionDays: 30}},
	}
	now := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	cutoff := cfg.WarmCutoff("prod", now)
	expected := now.AddDate(0, 0, -30)
	if !cutoff.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, cutoff)
	}
}

func TestMinWarmCutoff(t *testing.T) {
	cfg := testRetentionConfig()
	cfg.PerWorkspace = map[string]TierConfig{
		"prod": {WarmStore: &omniav1alpha1.WarmStoreConfig{RetentionDays: 30}},
	}
	now := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	min := cfg.MinWarmCutoff(now)
	// 30 days is more aggressive (earlier) than 7 days.
	expected := now.AddDate(0, 0, -30)
	if !min.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, min)
	}
}

func TestColdCutoff(t *testing.T) {
	cfg := testRetentionConfig()
	now := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	cutoff := cfg.ColdCutoff(now)
	expected := now.AddDate(0, 0, -90)
	if !cutoff.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, cutoff)
	}
}

func TestColdCutoff_Disabled(t *testing.T) {
	cfg := &RetentionConfig{
		Default: TierConfig{
			ColdArchive: &omniav1alpha1.ColdArchiveConfig{Enabled: false},
		},
	}
	cutoff := cfg.ColdCutoff(time.Now())
	if !cutoff.IsZero() {
		t.Errorf("expected zero time for disabled cold archive, got %v", cutoff)
	}
}

func TestColdArchiveEnabled(t *testing.T) {
	cfg := testRetentionConfig()
	if !cfg.ColdArchiveEnabled() {
		t.Error("expected ColdArchiveEnabled() == true")
	}
	cfg.Default.ColdArchive.Enabled = false
	if cfg.ColdArchiveEnabled() {
		t.Error("expected ColdArchiveEnabled() == false")
	}
}

// ---------------------------------------------------------------------------
// Engine tests
// ---------------------------------------------------------------------------

func TestRun_HappyPath(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{
			testSession("s1", "", old),
			testSession("s2", "", old),
		},
	}
	cold := &mockColdArchive{}
	hot := &mockHotCache{}

	e := NewEngine(warm, cold, hot, testRetentionConfig(), testConfig(), nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.SessionsCompacted != 2 {
		t.Errorf("expected 2 sessions compacted, got %d", result.SessionsCompacted)
	}
	if result.BatchesProcessed != 1 {
		t.Errorf("expected 1 batch, got %d", result.BatchesProcessed)
	}
	if len(cold.written) != 1 || len(cold.written[0]) != 2 {
		t.Error("expected 1 write of 2 sessions to cold archive")
	}
	if len(warm.deletedBatches) != 1 || len(warm.deletedBatches[0]) != 2 {
		t.Error("expected 1 batch delete of 2 sessions from warm store")
	}
	if len(hot.invalidated) != 2 {
		t.Errorf("expected 2 hot cache invalidations, got %d", len(hot.invalidated))
	}
	if result.ColdPurged != true {
		t.Error("expected cold purge to have run")
	}
}

func TestRun_EmptyWarmStore(t *testing.T) {
	warm := &mockWarmStore{}
	cold := &mockColdArchive{}

	e := NewEngine(warm, cold, nil, testRetentionConfig(), testConfig(), nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.SessionsCompacted != 0 {
		t.Errorf("expected 0 sessions compacted, got %d", result.SessionsCompacted)
	}
	if result.BatchesProcessed != 0 {
		t.Errorf("expected 0 batches, got %d", result.BatchesProcessed)
	}
}

func TestRun_PerWorkspaceFiltering(t *testing.T) {
	now := time.Now()
	// 10 days old — older than default 7d, but within long-retain's 30d.
	old := now.Add(-10 * 24 * time.Hour)

	retention := testRetentionConfig()
	retention.PerWorkspace = map[string]TierConfig{
		"long-retain": {WarmStore: &omniav1alpha1.WarmStoreConfig{RetentionDays: 30}},
	}

	// MinWarmCutoff = 30 days → we need the mock to return sessions updated
	// before that cutoff. Set both sessions to 10 days old in the store, but
	// the query cutoff is 30 days. Sessions at 10 days won't be returned by
	// GetSessionsOlderThan(cutoff=30d). So make sessions 35 days old for the
	// query to see them, then rely on filterByWorkspaceCutoff.
	veryOld := now.Add(-35 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{
			testSession("s1", "", veryOld),            // default 7d → eligible (35 > 7)
			testSession("s2", "long-retain", old),     // mock returns it (10 < 30 cutoff → not returned by query)
			testSession("s3", "long-retain", veryOld), // 30d → eligible (35 > 30)
		},
	}
	cold := &mockColdArchive{}

	e := NewEngine(warm, cold, nil, retention, testConfig(), nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// s1 (35d, default 7d) → eligible
	// s2 (10d, long-retain 30d) → not returned by query (10 < 30)
	// s3 (35d, long-retain 30d) → eligible
	if result.SessionsCompacted != 2 {
		t.Errorf("expected 2 sessions compacted, got %d", result.SessionsCompacted)
	}
}

func TestRun_NoHotCache(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{testSession("s1", "", old)},
	}
	cold := &mockColdArchive{}

	e := NewEngine(warm, cold, nil, testRetentionConfig(), testConfig(), nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.SessionsCompacted != 1 {
		t.Errorf("expected 1 session compacted, got %d", result.SessionsCompacted)
	}
}

func TestRun_WriteRetrySuccess(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{testSession("s1", "", old)},
	}
	cold := &mockColdArchive{
		writeErr:     errors.New("transient"),
		writeErrOnce: true,
	}

	e := NewEngine(warm, cold, nil, testRetentionConfig(), testConfig(), nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.SessionsCompacted != 1 {
		t.Errorf("expected 1 session compacted after retry, got %d", result.SessionsCompacted)
	}
}

func TestRun_WriteRetryExhausted(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{testSession("s1", "", old)},
	}
	cold := &mockColdArchive{writeErr: errors.New("permanent")}

	cfg := testConfig()
	cfg.MaxRetries = 2

	e := NewEngine(warm, cold, nil, testRetentionConfig(), cfg, nil, testLogger())
	_, err := e.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from exhausted retries")
	}
}

func TestRun_DeleteFailure(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions:  []*session.Session{testSession("s1", "", old)},
		deleteErr: errors.New("delete failed"),
	}
	cold := &mockColdArchive{}

	e := NewEngine(warm, cold, nil, testRetentionConfig(), testConfig(), nil, testLogger())
	_, err := e.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from delete failure")
	}
}

func TestRun_ColdPurgeFailure(t *testing.T) {
	warm := &mockWarmStore{}
	cold := &mockColdArchive{deleteErr: errors.New("purge failed")}

	e := NewEngine(warm, cold, nil, testRetentionConfig(), testConfig(), nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run should not fail for cold purge error: %v", err)
	}
	if len(result.Errors) == 0 {
		t.Error("expected error in result.Errors")
	}
	if result.ColdPurged {
		t.Error("expected ColdPurged == false")
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{testSession("s1", "", old)},
	}
	cold := &mockColdArchive{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	e := NewEngine(warm, cold, nil, testRetentionConfig(), testConfig(), nil, testLogger())
	result, err := e.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.SessionsCompacted != 0 {
		t.Errorf("expected 0 sessions compacted on cancellation, got %d", result.SessionsCompacted)
	}
}

func TestRun_DryRun(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{testSession("s1", "", old)},
	}
	cold := &mockColdArchive{}

	cfg := testConfig()
	cfg.DryRun = true

	e := NewEngine(warm, cold, nil, testRetentionConfig(), cfg, nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// In dry-run, sessions are counted but not actually written/deleted.
	if result.SessionsCompacted != 1 {
		t.Errorf("expected 1 session in dry-run count, got %d", result.SessionsCompacted)
	}
	if len(cold.written) != 0 {
		t.Error("expected no cold writes in dry-run")
	}
	if len(warm.deletedBatches) != 0 {
		t.Error("expected no warm deletes in dry-run")
	}
}

func TestRun_MultipleBatches(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	var sessions []*session.Session
	for i := 0; i < 5; i++ {
		sessions = append(sessions, testSession(
			"s"+string(rune('0'+i)),
			"",
			old.Add(time.Duration(i)*time.Minute),
		))
	}

	warm := &mockWarmStore{sessions: sessions}
	cold := &mockColdArchive{}

	cfg := testConfig()
	cfg.BatchSize = 2

	e := NewEngine(warm, cold, nil, testRetentionConfig(), cfg, nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.SessionsCompacted != 5 {
		t.Errorf("expected 5 sessions compacted, got %d", result.SessionsCompacted)
	}
	if result.BatchesProcessed < 3 {
		t.Errorf("expected at least 3 batches, got %d", result.BatchesProcessed)
	}
}

func TestFilterByWorkspaceCutoff(t *testing.T) {
	now := time.Now()
	retention := testRetentionConfig()
	retention.PerWorkspace = map[string]TierConfig{
		"long": {WarmStore: &omniav1alpha1.WarmStoreConfig{RetentionDays: 30}},
	}

	sessions := []*session.Session{
		testSession("s1", "", now.Add(-10*24*time.Hour)),     // default 7d → eligible
		testSession("s2", "long", now.Add(-10*24*time.Hour)), // 30d → NOT eligible
		testSession("s3", "long", now.Add(-40*24*time.Hour)), // 30d → eligible
	}

	e := &Engine{retention: retention}
	eligible := e.filterByWorkspaceCutoff(sessions, now)
	if len(eligible) != 2 {
		t.Errorf("expected 2 eligible, got %d", len(eligible))
	}
	if eligible[0].ID != "s1" {
		t.Errorf("expected s1, got %s", eligible[0].ID)
	}
	if eligible[1].ID != "s3" {
		t.Errorf("expected s3, got %s", eligible[1].ID)
	}
}

// ---------------------------------------------------------------------------
// Additional coverage tests
// ---------------------------------------------------------------------------

func TestRun_WithMetrics(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{
			testSession("s1", "", old),
		},
	}
	cold := &mockColdArchive{}
	hot := &mockHotCache{}

	m := newTestMetrics()
	e := NewEngine(warm, cold, hot, testRetentionConfig(), testConfig(), m, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.SessionsCompacted != 1 {
		t.Errorf("expected 1 session, got %d", result.SessionsCompacted)
	}
}

func TestRun_HotCacheInvalidationError(t *testing.T) {
	now := time.Now()
	old := now.Add(-10 * 24 * time.Hour)

	warm := &mockWarmStore{
		sessions: []*session.Session{testSession("s1", "", old)},
	}
	cold := &mockColdArchive{}
	hot := &mockHotCache{invalidateErr: errors.New("cache error")}

	e := NewEngine(warm, cold, hot, testRetentionConfig(), testConfig(), nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run should succeed despite hot cache error: %v", err)
	}
	if result.SessionsCompacted != 1 {
		t.Errorf("expected 1 session compacted, got %d", result.SessionsCompacted)
	}
}

func TestRun_ColdPurgeSkipped_NoCutoff(t *testing.T) {
	warm := &mockWarmStore{}
	cold := &mockColdArchive{}

	// Retention config with no cold retention days configured.
	retention := &RetentionConfig{
		Default: TierConfig{
			WarmStore:   &omniav1alpha1.WarmStoreConfig{RetentionDays: 7},
			ColdArchive: &omniav1alpha1.ColdArchiveConfig{Enabled: true},
		},
	}

	e := NewEngine(warm, cold, nil, retention, testConfig(), nil, testLogger())
	result, err := e.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.ColdPurged {
		t.Error("expected ColdPurged == false when no cutoff configured")
	}
}

func TestRun_WarmStoreQueryError(t *testing.T) {
	warm := &mockWarmStoreWithQueryErr{
		queryErr: errors.New("query failed"),
	}
	cold := &mockColdArchive{}

	e := NewEngine(warm, cold, nil, testRetentionConfig(), testConfig(), nil, testLogger())
	_, err := e.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from warm store query failure")
	}
}

func TestLoadRetentionConfig_FileNotFound(t *testing.T) {
	_, err := LoadRetentionConfig("/nonexistent/path/retention.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadRetentionConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRetentionConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestWithRetry_ContextCancelledDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := &Engine{
		cfg: Config{MaxRetries: 3, RetryDelay: 10 * time.Second},
		log: testLogger(),
	}

	count := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := e.withRetry(ctx, "test", func() error {
		count++
		return errors.New("transient")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// mockWarmStoreWithQueryErr always returns an error from GetSessionsOlderThan.
type mockWarmStoreWithQueryErr struct {
	mockWarmStore
	queryErr error
}

func (m *mockWarmStoreWithQueryErr) GetSessionsOlderThan(
	_ context.Context, _ time.Time, _ int,
) ([]*session.Session, error) {
	return nil, m.queryErr
}

// newTestMetrics creates compaction metrics for testing (unexported helper
// delegates to the metrics package test helper).
func newTestMetrics() *metrics.CompactionMetrics {
	return metrics.NewCompactionMetrics()
}
