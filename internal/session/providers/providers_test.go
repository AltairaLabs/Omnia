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

package providers

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session"
)

// ---------------------------------------------------------------------------
// Mock HotCacheProvider
// ---------------------------------------------------------------------------

type mockHotCache struct {
	sessions map[string]*hotEntry
	closed   bool
}

type hotEntry struct {
	session  *session.Session
	messages []*session.Message
	expiry   time.Time // zero = no expiry
}

func newMockHotCache() *mockHotCache {
	return &mockHotCache{sessions: make(map[string]*hotEntry)}
}

func (m *mockHotCache) GetSession(_ context.Context, sessionID string) (*session.Session, error) {
	e, ok := m.sessions[sessionID]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	if !e.expiry.IsZero() && time.Now().After(e.expiry) {
		delete(m.sessions, sessionID)
		return nil, session.ErrSessionNotFound
	}
	return e.session, nil
}

func (m *mockHotCache) SetSession(_ context.Context, s *session.Session, ttl time.Duration) error {
	var expiry time.Time
	if ttl > 0 {
		expiry = time.Now().Add(ttl)
	}
	if existing, ok := m.sessions[s.ID]; ok {
		existing.session = s
		existing.expiry = expiry
	} else {
		m.sessions[s.ID] = &hotEntry{session: s, expiry: expiry}
	}
	return nil
}

func (m *mockHotCache) DeleteSession(_ context.Context, sessionID string) error {
	if _, ok := m.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockHotCache) AppendMessage(_ context.Context, sessionID string, msg *session.Message) error {
	e, ok := m.sessions[sessionID]
	if !ok {
		return session.ErrSessionNotFound
	}
	e.messages = append(e.messages, msg)
	return nil
}

func (m *mockHotCache) GetRecentMessages(_ context.Context, sessionID string, limit int) ([]*session.Message, error) {
	e, ok := m.sessions[sessionID]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	msgs := e.messages
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[len(msgs)-limit:]
	}
	return msgs, nil
}

func (m *mockHotCache) RefreshTTL(_ context.Context, sessionID string, ttl time.Duration) error {
	e, ok := m.sessions[sessionID]
	if !ok {
		return session.ErrSessionNotFound
	}
	if ttl > 0 {
		e.expiry = time.Now().Add(ttl)
	} else {
		e.expiry = time.Time{}
	}
	return nil
}

func (m *mockHotCache) Invalidate(_ context.Context, sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}

func (m *mockHotCache) Ping(_ context.Context) error { return nil }

func (m *mockHotCache) Close() error {
	m.closed = true
	return nil
}

// Compile-time interface check.
var _ HotCacheProvider = (*mockHotCache)(nil)

// ---------------------------------------------------------------------------
// Mock WarmStoreProvider
// ---------------------------------------------------------------------------

type mockWarmStore struct {
	sessions   map[string]*session.Session
	messages   map[string][]*session.Message
	partitions []PartitionInfo
	closed     bool
}

func newMockWarmStore() *mockWarmStore {
	return &mockWarmStore{
		sessions: make(map[string]*session.Session),
		messages: make(map[string][]*session.Message),
	}
}

func (m *mockWarmStore) CreateSession(_ context.Context, s *session.Session) error {
	if _, ok := m.sessions[s.ID]; ok {
		return errors.New("session already exists")
	}
	m.sessions[s.ID] = s
	return nil
}

func (m *mockWarmStore) GetSession(_ context.Context, sessionID string) (*session.Session, error) {
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	return s, nil
}

func (m *mockWarmStore) UpdateSession(_ context.Context, s *session.Session) error {
	if _, ok := m.sessions[s.ID]; !ok {
		return session.ErrSessionNotFound
	}
	m.sessions[s.ID] = s
	return nil
}

func (m *mockWarmStore) DeleteSession(_ context.Context, sessionID string) error {
	if _, ok := m.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	delete(m.sessions, sessionID)
	delete(m.messages, sessionID)
	return nil
}

func (m *mockWarmStore) AppendMessage(_ context.Context, sessionID string, msg *session.Message) error {
	if _, ok := m.sessions[sessionID]; !ok {
		return session.ErrSessionNotFound
	}
	m.messages[sessionID] = append(m.messages[sessionID], msg)
	return nil
}

func (m *mockWarmStore) GetMessages(_ context.Context, sessionID string, opts MessageQueryOpts) ([]*session.Message, error) {
	if _, ok := m.sessions[sessionID]; !ok {
		return nil, session.ErrSessionNotFound
	}
	msgs := m.messages[sessionID]

	// Apply role filter
	if len(opts.Roles) > 0 {
		roleSet := make(map[session.MessageRole]bool, len(opts.Roles))
		for _, r := range opts.Roles {
			roleSet[r] = true
		}
		var filtered []*session.Message
		for _, msg := range msgs {
			if roleSet[msg.Role] {
				filtered = append(filtered, msg)
			}
		}
		msgs = filtered
	}

	// Apply sequence filters
	if opts.AfterSeq > 0 {
		var filtered []*session.Message
		for _, msg := range msgs {
			if msg.SequenceNum > opts.AfterSeq {
				filtered = append(filtered, msg)
			}
		}
		msgs = filtered
	}
	if opts.BeforeSeq > 0 {
		var filtered []*session.Message
		for _, msg := range msgs {
			if msg.SequenceNum < opts.BeforeSeq {
				filtered = append(filtered, msg)
			}
		}
		msgs = filtered
	}

	// Apply sort
	if opts.SortOrder == SortDesc {
		sorted := make([]*session.Message, len(msgs))
		copy(sorted, msgs)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].SequenceNum > sorted[j].SequenceNum
		})
		msgs = sorted
	}

	// Apply offset and limit
	if opts.Offset > 0 && opts.Offset < len(msgs) {
		msgs = msgs[opts.Offset:]
	} else if opts.Offset >= len(msgs) {
		return nil, nil
	}
	if opts.Limit > 0 && opts.Limit < len(msgs) {
		msgs = msgs[:opts.Limit]
	}

	return msgs, nil
}

func matchesListOpts(s *session.Session, opts SessionListOpts) bool {
	if opts.Namespace != "" && s.Namespace != opts.Namespace {
		return false
	}
	if opts.AgentName != "" && s.AgentName != opts.AgentName {
		return false
	}
	if opts.Status != "" && s.Status != opts.Status {
		return false
	}
	if opts.WorkspaceName != "" && s.WorkspaceName != opts.WorkspaceName {
		return false
	}
	if !opts.CreatedAfter.IsZero() && !s.CreatedAt.After(opts.CreatedAfter) {
		return false
	}
	if !opts.CreatedBefore.IsZero() && !s.CreatedAt.Before(opts.CreatedBefore) {
		return false
	}
	if len(opts.Tags) > 0 {
		tagSet := make(map[string]bool, len(s.Tags))
		for _, t := range s.Tags {
			tagSet[t] = true
		}
		for _, t := range opts.Tags {
			if !tagSet[t] {
				return false
			}
		}
	}
	return true
}

func (m *mockWarmStore) ListSessions(_ context.Context, opts SessionListOpts) (*SessionPage, error) {
	results := make([]*session.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if !matchesListOpts(s, opts) {
			continue
		}
		results = append(results, s)
	}

	total := int64(len(results))

	// Sort by CreatedAt
	sort.Slice(results, func(i, j int) bool {
		if opts.SortOrder == SortAsc {
			return results[i].CreatedAt.Before(results[j].CreatedAt)
		}
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})

	// Apply offset/limit
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) && len(results) > 0 {
		results = nil
	}
	hasMore := false
	if opts.Limit > 0 && opts.Limit < len(results) {
		hasMore = true
		results = results[:opts.Limit]
	}

	return &SessionPage{
		Sessions:   results,
		TotalCount: total,
		HasMore:    hasMore,
	}, nil
}

func (m *mockWarmStore) SearchSessions(_ context.Context, query string, opts SessionListOpts) (*SessionPage, error) {
	var results []*session.Session
	lower := strings.ToLower(query)
	for _, s := range m.sessions {
		if strings.Contains(strings.ToLower(s.AgentName), lower) ||
			strings.Contains(strings.ToLower(s.LastMessagePreview), lower) {
			results = append(results, s)
		}
	}

	total := int64(len(results))
	if opts.Offset > 0 && opts.Offset < len(results) {
		results = results[opts.Offset:]
	} else if opts.Offset >= len(results) && len(results) > 0 {
		results = nil
	}
	hasMore := false
	if opts.Limit > 0 && opts.Limit < len(results) {
		hasMore = true
		results = results[:opts.Limit]
	}

	return &SessionPage{
		Sessions:   results,
		TotalCount: total,
		HasMore:    hasMore,
	}, nil
}

func (m *mockWarmStore) CreatePartition(_ context.Context, date time.Time) error {
	for _, p := range m.partitions {
		if p.StartDate.Equal(date) {
			return ErrPartitionExists
		}
	}
	m.partitions = append(m.partitions, PartitionInfo{
		Name:      "sessions_" + date.Format("2006_01"),
		StartDate: date,
		EndDate:   date.AddDate(0, 1, 0),
	})
	return nil
}

func (m *mockWarmStore) DropPartition(_ context.Context, date time.Time) error {
	for i, p := range m.partitions {
		if p.StartDate.Equal(date) {
			m.partitions = append(m.partitions[:i], m.partitions[i+1:]...)
			return nil
		}
	}
	return ErrPartitionNotFound
}

func (m *mockWarmStore) ListPartitions(_ context.Context) ([]PartitionInfo, error) {
	result := make([]PartitionInfo, len(m.partitions))
	copy(result, m.partitions)
	return result, nil
}

func (m *mockWarmStore) GetSessionsOlderThan(_ context.Context, cutoff time.Time, batchSize int) ([]*session.Session, error) {
	var results []*session.Session
	for _, s := range m.sessions {
		if s.UpdatedAt.Before(cutoff) {
			results = append(results, s)
			if batchSize > 0 && len(results) >= batchSize {
				break
			}
		}
	}
	return results, nil
}

func (m *mockWarmStore) DeleteSessionsBatch(_ context.Context, sessionIDs []string) error {
	for _, id := range sessionIDs {
		delete(m.sessions, id)
		delete(m.messages, id)
	}
	return nil
}

func (m *mockWarmStore) SaveArtifact(_ context.Context, _ *session.Artifact) error { return nil }
func (m *mockWarmStore) GetArtifacts(_ context.Context, _ string) ([]*session.Artifact, error) {
	return []*session.Artifact{}, nil
}
func (m *mockWarmStore) GetSessionArtifacts(_ context.Context, _ string) ([]*session.Artifact, error) {
	return []*session.Artifact{}, nil
}
func (m *mockWarmStore) DeleteSessionArtifacts(_ context.Context, _ string) error { return nil }

func (m *mockWarmStore) Ping(_ context.Context) error { return nil }

func (m *mockWarmStore) Close() error {
	m.closed = true
	return nil
}

// Compile-time interface check.
var _ WarmStoreProvider = (*mockWarmStore)(nil)

// ---------------------------------------------------------------------------
// Mock ColdArchiveProvider
// ---------------------------------------------------------------------------

type mockColdArchive struct {
	sessions map[string]*session.Session
	dates    map[time.Time]bool
	closed   bool
}

func newMockColdArchive() *mockColdArchive {
	return &mockColdArchive{
		sessions: make(map[string]*session.Session),
		dates:    make(map[time.Time]bool),
	}
}

func (m *mockColdArchive) WriteParquet(_ context.Context, sessions []*session.Session, _ WriteOpts) error {
	for _, s := range sessions {
		m.sessions[s.ID] = s
		day := s.CreatedAt.Truncate(24 * time.Hour)
		m.dates[day] = true
	}
	return nil
}

func (m *mockColdArchive) GetSession(_ context.Context, sessionID string) (*session.Session, error) {
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	return s, nil
}

func (m *mockColdArchive) ListAvailableDates(_ context.Context) ([]time.Time, error) {
	dates := make([]time.Time, 0, len(m.dates))
	for d := range m.dates {
		dates = append(dates, d)
	}
	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Before(dates[j])
	})
	return dates, nil
}

func (m *mockColdArchive) QuerySessions(_ context.Context, query string) ([]*session.Session, error) {
	var results []*session.Session
	lower := strings.ToLower(query)
	for _, s := range m.sessions {
		if strings.Contains(strings.ToLower(s.AgentName), lower) {
			results = append(results, s)
		}
	}
	return results, nil
}

func (m *mockColdArchive) DeleteOlderThan(_ context.Context, cutoff time.Time) error {
	for id, s := range m.sessions {
		if s.CreatedAt.Before(cutoff) {
			delete(m.sessions, id)
		}
	}
	// Clean up dates
	for d := range m.dates {
		if d.Before(cutoff) {
			delete(m.dates, d)
		}
	}
	return nil
}

func (m *mockColdArchive) Ping(_ context.Context) error { return nil }

func (m *mockColdArchive) Close() error {
	m.closed = true
	return nil
}

// Compile-time interface check.
var _ ColdArchiveProvider = (*mockColdArchive)(nil)

// ---------------------------------------------------------------------------
// HotCacheProvider Tests
// ---------------------------------------------------------------------------

func TestHotCache_SetGetSession(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	s := &session.Session{
		ID:        "sess-1",
		AgentName: "agent-a",
		Namespace: "ns-1",
		CreatedAt: time.Now(),
		Status:    session.SessionStatusActive,
	}

	if err := cache.SetSession(ctx, s, time.Hour); err != nil {
		t.Fatalf("SetSession failed: %v", err)
	}

	got, err := cache.GetSession(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("ID = %q, want %q", got.ID, s.ID)
	}
	if got.AgentName != s.AgentName {
		t.Errorf("AgentName = %q, want %q", got.AgentName, s.AgentName)
	}
	if got.Status != session.SessionStatusActive {
		t.Errorf("Status = %q, want %q", got.Status, session.SessionStatusActive)
	}
}

func TestHotCache_GetSessionNotFound(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	_, err := cache.GetSession(ctx, "nonexistent")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestHotCache_TTLAndInvalidate(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	s := &session.Session{ID: "sess-ttl", AgentName: "agent-a"}

	// Set with very short TTL
	if err := cache.SetSession(ctx, s, time.Millisecond); err != nil {
		t.Fatalf("SetSession failed: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	// Should be expired
	_, err := cache.GetSession(ctx, s.ID)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("expected expired session to return ErrSessionNotFound, got %v", err)
	}

	// Re-add and test RefreshTTL
	if err := cache.SetSession(ctx, s, time.Hour); err != nil {
		t.Fatalf("SetSession failed: %v", err)
	}
	if err := cache.RefreshTTL(ctx, s.ID, 2*time.Hour); err != nil {
		t.Fatalf("RefreshTTL failed: %v", err)
	}
	if _, err := cache.GetSession(ctx, s.ID); err != nil {
		t.Fatalf("GetSession after RefreshTTL failed: %v", err)
	}

	// Test Invalidate
	if err := cache.Invalidate(ctx, s.ID); err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}
	_, err = cache.GetSession(ctx, s.ID)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("expected invalidated session to return ErrSessionNotFound, got %v", err)
	}
}

func TestHotCache_RefreshTTLNotFound(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	err := cache.RefreshTTL(ctx, "nonexistent", time.Hour)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestHotCache_AppendAndGetRecentMessages(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	s := &session.Session{ID: "sess-msg", AgentName: "agent-a"}
	if err := cache.SetSession(ctx, s, 0); err != nil {
		t.Fatalf("SetSession failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		msg := &session.Message{
			ID:          "msg-" + string(rune('a'+i)),
			Role:        session.RoleUser,
			Content:     "message",
			SequenceNum: int32(i + 1),
		}
		if err := cache.AppendMessage(ctx, s.ID, msg); err != nil {
			t.Fatalf("AppendMessage failed: %v", err)
		}
	}

	// Get all
	msgs, err := cache.GetRecentMessages(ctx, s.ID, 0)
	if err != nil {
		t.Fatalf("GetRecentMessages failed: %v", err)
	}
	if len(msgs) != 5 {
		t.Errorf("message count = %d, want 5", len(msgs))
	}

	// Get last 3
	msgs, err = cache.GetRecentMessages(ctx, s.ID, 3)
	if err != nil {
		t.Fatalf("GetRecentMessages(limit=3) failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("message count = %d, want 3", len(msgs))
	}
	if msgs[0].SequenceNum != 3 {
		t.Errorf("first message seq = %d, want 3", msgs[0].SequenceNum)
	}
}

func TestHotCache_AppendMessageNotFound(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	err := cache.AppendMessage(ctx, "nonexistent", &session.Message{})
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestHotCache_GetRecentMessagesNotFound(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	_, err := cache.GetRecentMessages(ctx, "nonexistent", 10)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestHotCache_DeleteSession(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	s := &session.Session{ID: "sess-del"}
	_ = cache.SetSession(ctx, s, 0)

	if err := cache.DeleteSession(ctx, s.ID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	_, err := cache.GetSession(ctx, s.ID)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestHotCache_DeleteSessionNotFound(t *testing.T) {
	cache := newMockHotCache()
	ctx := context.Background()

	err := cache.DeleteSession(ctx, "nonexistent")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestHotCache_Ping(t *testing.T) {
	cache := newMockHotCache()
	if err := cache.Ping(context.Background()); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// WarmStoreProvider Tests
// ---------------------------------------------------------------------------

func TestWarmStore_CRUD(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()
	now := time.Now()

	s := &session.Session{
		ID:            "sess-warm-1",
		AgentName:     "agent-a",
		Namespace:     "ns-1",
		WorkspaceName: "ws-1",
		CreatedAt:     now,
		UpdatedAt:     now,
		Status:        session.SessionStatusActive,
	}

	// Create
	if err := store.CreateSession(ctx, s); err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Create duplicate
	if err := store.CreateSession(ctx, s); err == nil {
		t.Fatal("CreateSession duplicate should fail")
	}

	// Get
	got, err := store.GetSession(ctx, s.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.ID != s.ID || got.Status != session.SessionStatusActive {
		t.Errorf("unexpected session: %+v", got)
	}

	// Update
	s.Status = session.SessionStatusCompleted
	s.EndedAt = time.Now()
	if err := store.UpdateSession(ctx, s); err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}
	got, _ = store.GetSession(ctx, s.ID)
	if got.Status != session.SessionStatusCompleted {
		t.Errorf("Status = %q, want %q", got.Status, session.SessionStatusCompleted)
	}

	// Delete
	if err := store.DeleteSession(ctx, s.ID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}
	_, err = store.GetSession(ctx, s.ID)
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestWarmStore_UpdateNotFound(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()

	err := store.UpdateSession(ctx, &session.Session{ID: "nonexistent"})
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestWarmStore_DeleteNotFound(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()

	err := store.DeleteSession(ctx, "nonexistent")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestWarmStore_Messages(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()

	s := &session.Session{ID: "sess-msg", AgentName: "agent-a"}
	_ = store.CreateSession(ctx, s)

	msgs := []*session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "hello", SequenceNum: 1},
		{ID: "m2", Role: session.RoleAssistant, Content: "hi", SequenceNum: 2},
		{ID: "m3", Role: session.RoleUser, Content: "bye", SequenceNum: 3},
	}
	for _, msg := range msgs {
		if err := store.AppendMessage(ctx, s.ID, msg); err != nil {
			t.Fatalf("AppendMessage failed: %v", err)
		}
	}

	t.Run("get all", func(t *testing.T) {
		got, err := store.GetMessages(ctx, s.ID, MessageQueryOpts{})
		if err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("count = %d, want 3", len(got))
		}
	})

	t.Run("filter by role", func(t *testing.T) {
		got, err := store.GetMessages(ctx, s.ID, MessageQueryOpts{
			Roles: []session.MessageRole{session.RoleUser},
		})
		if err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("count = %d, want 2", len(got))
		}
	})

	t.Run("after sequence", func(t *testing.T) {
		got, err := store.GetMessages(ctx, s.ID, MessageQueryOpts{
			AfterSeq: 1,
		})
		if err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("count = %d, want 2", len(got))
		}
	})

	t.Run("before sequence", func(t *testing.T) {
		got, err := store.GetMessages(ctx, s.ID, MessageQueryOpts{
			BeforeSeq: 3,
		})
		if err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("count = %d, want 2", len(got))
		}
	})

	t.Run("limit and offset", func(t *testing.T) {
		got, err := store.GetMessages(ctx, s.ID, MessageQueryOpts{
			Limit:  1,
			Offset: 1,
		})
		if err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("count = %d, want 1", len(got))
		}
		if got[0].ID != "m2" {
			t.Errorf("ID = %q, want %q", got[0].ID, "m2")
		}
	})

	t.Run("sort descending", func(t *testing.T) {
		got, err := store.GetMessages(ctx, s.ID, MessageQueryOpts{
			SortOrder: SortDesc,
		})
		if err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("count = %d, want 3", len(got))
		}
		if got[0].SequenceNum != 3 {
			t.Errorf("first seq = %d, want 3", got[0].SequenceNum)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := store.GetMessages(ctx, "nonexistent", MessageQueryOpts{})
		if !errors.Is(err, session.ErrSessionNotFound) {
			t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
		}
	})
}

func TestWarmStore_AppendMessageNotFound(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()

	err := store.AppendMessage(ctx, "nonexistent", &session.Message{})
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestWarmStore_ListSessions(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions := []*session.Session{
		{ID: "s1", AgentName: "agent-a", Namespace: "ns-1", Status: session.SessionStatusActive, CreatedAt: base, UpdatedAt: base, Tags: []string{"prod"}},
		{ID: "s2", AgentName: "agent-b", Namespace: "ns-1", Status: session.SessionStatusCompleted, CreatedAt: base.Add(time.Hour), UpdatedAt: base.Add(time.Hour)},
		{ID: "s3", AgentName: "agent-a", Namespace: "ns-2", Status: session.SessionStatusActive, CreatedAt: base.Add(2 * time.Hour), UpdatedAt: base.Add(2 * time.Hour), Tags: []string{"prod", "v2"}},
	}
	for _, s := range sessions {
		_ = store.CreateSession(ctx, s)
	}

	t.Run("filter by namespace", func(t *testing.T) {
		page, err := store.ListSessions(ctx, SessionListOpts{Namespace: "ns-1"})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if page.TotalCount != 2 {
			t.Errorf("TotalCount = %d, want 2", page.TotalCount)
		}
	})

	t.Run("filter by agent", func(t *testing.T) {
		page, err := store.ListSessions(ctx, SessionListOpts{AgentName: "agent-a"})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if page.TotalCount != 2 {
			t.Errorf("TotalCount = %d, want 2", page.TotalCount)
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		page, err := store.ListSessions(ctx, SessionListOpts{Status: session.SessionStatusCompleted})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if page.TotalCount != 1 {
			t.Errorf("TotalCount = %d, want 1", page.TotalCount)
		}
	})

	t.Run("filter by tags", func(t *testing.T) {
		page, err := store.ListSessions(ctx, SessionListOpts{Tags: []string{"prod"}})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if page.TotalCount != 2 {
			t.Errorf("TotalCount = %d, want 2", page.TotalCount)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		page, err := store.ListSessions(ctx, SessionListOpts{Limit: 2})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(page.Sessions) != 2 {
			t.Errorf("count = %d, want 2", len(page.Sessions))
		}
		if !page.HasMore {
			t.Error("HasMore should be true")
		}
		if page.TotalCount != 3 {
			t.Errorf("TotalCount = %d, want 3", page.TotalCount)
		}
	})

	t.Run("sort ascending", func(t *testing.T) {
		page, err := store.ListSessions(ctx, SessionListOpts{SortOrder: SortAsc})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if page.Sessions[0].ID != "s1" {
			t.Errorf("first session = %q, want s1", page.Sessions[0].ID)
		}
	})
}

func TestWarmStore_SearchSessions(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()

	_ = store.CreateSession(ctx, &session.Session{
		ID:                 "s1",
		AgentName:          "chatbot",
		LastMessagePreview: "Hello world",
	})
	_ = store.CreateSession(ctx, &session.Session{
		ID:                 "s2",
		AgentName:          "coder",
		LastMessagePreview: "Build failed",
	})

	page, err := store.SearchSessions(ctx, "chatbot", SessionListOpts{})
	if err != nil {
		t.Fatalf("SearchSessions failed: %v", err)
	}
	if page.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", page.TotalCount)
	}

	page, err = store.SearchSessions(ctx, "hello", SessionListOpts{})
	if err != nil {
		t.Fatalf("SearchSessions failed: %v", err)
	}
	if page.TotalCount != 1 {
		t.Errorf("TotalCount = %d, want 1", page.TotalCount)
	}
}

func TestWarmStore_Partitions(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()

	jan := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)

	// Create
	if err := store.CreatePartition(ctx, jan); err != nil {
		t.Fatalf("CreatePartition failed: %v", err)
	}
	if err := store.CreatePartition(ctx, feb); err != nil {
		t.Fatalf("CreatePartition failed: %v", err)
	}

	// Create duplicate
	if err := store.CreatePartition(ctx, jan); !errors.Is(err, ErrPartitionExists) {
		t.Errorf("error = %v, want %v", err, ErrPartitionExists)
	}

	// List
	parts, err := store.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions failed: %v", err)
	}
	if len(parts) != 2 {
		t.Errorf("count = %d, want 2", len(parts))
	}

	// Drop
	if err := store.DropPartition(ctx, jan); err != nil {
		t.Fatalf("DropPartition failed: %v", err)
	}
	parts, _ = store.ListPartitions(ctx)
	if len(parts) != 1 {
		t.Errorf("count after drop = %d, want 1", len(parts))
	}

	// Drop nonexistent
	if err := store.DropPartition(ctx, jan); !errors.Is(err, ErrPartitionNotFound) {
		t.Errorf("error = %v, want %v", err, ErrPartitionNotFound)
	}
}

func TestWarmStore_BulkOps(t *testing.T) {
	store := newMockWarmStore()
	ctx := context.Background()
	cutoff := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	old := &session.Session{
		ID:        "old-1",
		UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	recent := &session.Session{
		ID:        "new-1",
		UpdatedAt: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	_ = store.CreateSession(ctx, old)
	_ = store.CreateSession(ctx, recent)

	// GetSessionsOlderThan
	older, err := store.GetSessionsOlderThan(ctx, cutoff, 10)
	if err != nil {
		t.Fatalf("GetSessionsOlderThan failed: %v", err)
	}
	if len(older) != 1 {
		t.Errorf("count = %d, want 1", len(older))
	}

	// DeleteSessionsBatch
	ids := make([]string, len(older))
	for i, s := range older {
		ids[i] = s.ID
	}
	if err := store.DeleteSessionsBatch(ctx, ids); err != nil {
		t.Fatalf("DeleteSessionsBatch failed: %v", err)
	}

	_, err = store.GetSession(ctx, "old-1")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
	if _, err := store.GetSession(ctx, "new-1"); err != nil {
		t.Errorf("recent session should still exist: %v", err)
	}
}

func TestWarmStore_Ping(t *testing.T) {
	store := newMockWarmStore()
	if err := store.Ping(context.Background()); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ColdArchiveProvider Tests
// ---------------------------------------------------------------------------

func TestColdArchive_WriteAndRead(t *testing.T) {
	archive := newMockColdArchive()
	ctx := context.Background()

	sessions := []*session.Session{
		{ID: "cold-1", AgentName: "agent-a", CreatedAt: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)},
		{ID: "cold-2", AgentName: "agent-b", CreatedAt: time.Date(2025, 2, 20, 0, 0, 0, 0, time.UTC)},
	}

	if err := archive.WriteParquet(ctx, sessions, WriteOpts{
		BasePath:    "sessions/2025/",
		Compression: "snappy",
	}); err != nil {
		t.Fatalf("WriteParquet failed: %v", err)
	}

	// Read back
	got, err := archive.GetSession(ctx, "cold-1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got.AgentName != "agent-a" {
		t.Errorf("AgentName = %q, want %q", got.AgentName, "agent-a")
	}

	// Not found
	_, err = archive.GetSession(ctx, "nonexistent")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
}

func TestColdArchive_ListAvailableDates(t *testing.T) {
	archive := newMockColdArchive()
	ctx := context.Background()

	sessions := []*session.Session{
		{ID: "c1", CreatedAt: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)},
		{ID: "c2", CreatedAt: time.Date(2025, 3, 20, 5, 0, 0, 0, time.UTC)},
		{ID: "c3", CreatedAt: time.Date(2025, 1, 15, 22, 0, 0, 0, time.UTC)}, // same day as c1
	}
	_ = archive.WriteParquet(ctx, sessions, WriteOpts{})

	dates, err := archive.ListAvailableDates(ctx)
	if err != nil {
		t.Fatalf("ListAvailableDates failed: %v", err)
	}
	if len(dates) != 2 {
		t.Errorf("date count = %d, want 2", len(dates))
	}
	if dates[0].After(dates[1]) {
		t.Error("dates should be sorted ascending")
	}
}

func TestColdArchive_QuerySessions(t *testing.T) {
	archive := newMockColdArchive()
	ctx := context.Background()

	sessions := []*session.Session{
		{ID: "q1", AgentName: "chatbot"},
		{ID: "q2", AgentName: "coder"},
	}
	_ = archive.WriteParquet(ctx, sessions, WriteOpts{})

	results, err := archive.QuerySessions(ctx, "chat")
	if err != nil {
		t.Fatalf("QuerySessions failed: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("count = %d, want 1", len(results))
	}
}

func TestColdArchive_DeleteOlderThan(t *testing.T) {
	archive := newMockColdArchive()
	ctx := context.Background()

	sessions := []*session.Session{
		{ID: "d1", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: "d2", CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)},
	}
	_ = archive.WriteParquet(ctx, sessions, WriteOpts{})

	cutoff := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := archive.DeleteOlderThan(ctx, cutoff); err != nil {
		t.Fatalf("DeleteOlderThan failed: %v", err)
	}

	// d1 should be gone
	_, err := archive.GetSession(ctx, "d1")
	if !errors.Is(err, session.ErrSessionNotFound) {
		t.Errorf("error = %v, want %v", err, session.ErrSessionNotFound)
	}
	// d2 should remain
	if _, err := archive.GetSession(ctx, "d2"); err != nil {
		t.Errorf("d2 should still exist: %v", err)
	}
}

func TestColdArchive_Ping(t *testing.T) {
	archive := newMockColdArchive()
	if err := archive.Ping(context.Background()); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Registry Tests
// ---------------------------------------------------------------------------

func TestRegistry_NotConfigured(t *testing.T) {
	reg := NewRegistry()

	if _, err := reg.HotCache(); !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("HotCache error = %v, want %v", err, ErrProviderNotConfigured)
	}
	if _, err := reg.WarmStore(); !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("WarmStore error = %v, want %v", err, ErrProviderNotConfigured)
	}
	if _, err := reg.ColdArchive(); !errors.Is(err, ErrProviderNotConfigured) {
		t.Errorf("ColdArchive error = %v, want %v", err, ErrProviderNotConfigured)
	}
}

func TestRegistry_SetAndGet(t *testing.T) {
	reg := NewRegistry()
	hot := newMockHotCache()
	warm := newMockWarmStore()
	cold := newMockColdArchive()

	reg.SetHotCache(hot)
	reg.SetWarmStore(warm)
	reg.SetColdArchive(cold)

	gotHot, err := reg.HotCache()
	if err != nil {
		t.Fatalf("HotCache error: %v", err)
	}
	if gotHot != hot {
		t.Error("HotCache returned wrong instance")
	}

	gotWarm, err := reg.WarmStore()
	if err != nil {
		t.Fatalf("WarmStore error: %v", err)
	}
	if gotWarm != warm {
		t.Error("WarmStore returned wrong instance")
	}

	gotCold, err := reg.ColdArchive()
	if err != nil {
		t.Fatalf("ColdArchive error: %v", err)
	}
	if gotCold != cold {
		t.Error("ColdArchive returned wrong instance")
	}
}

func TestRegistry_Close(t *testing.T) {
	reg := NewRegistry()
	hot := newMockHotCache()
	warm := newMockWarmStore()
	cold := newMockColdArchive()

	reg.SetHotCache(hot)
	reg.SetWarmStore(warm)
	reg.SetColdArchive(cold)

	if err := reg.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if !hot.closed {
		t.Error("hot cache should be closed")
	}
	if !warm.closed {
		t.Error("warm store should be closed")
	}
	if !cold.closed {
		t.Error("cold archive should be closed")
	}
}

func TestRegistry_ClosePartial(t *testing.T) {
	reg := NewRegistry()
	hot := newMockHotCache()
	reg.SetHotCache(hot)

	// Close with only hot cache configured (warm+cold nil)
	if err := reg.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !hot.closed {
		t.Error("hot cache should be closed")
	}
}

func TestRegistry_CloseEmpty(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Close(); err != nil {
		t.Fatalf("Close empty registry failed: %v", err)
	}
}
