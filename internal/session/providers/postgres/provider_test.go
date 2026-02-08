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

package postgres

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/internal/session"
	pgmigrate "github.com/altairalabs/omnia/internal/session/postgres"
	"github.com/altairalabs/omnia/internal/session/providers"
)

var testConnStr string

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		os.Exit(m.Run())
	}

	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("omnia_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	testConnStr, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if err := container.Terminate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "failed to terminate container: %v\n", err)
	}

	os.Exit(code)
}

// freshDB creates an isolated database, runs migrations, and returns a pgxpool.Pool.
func freshDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dbName := fmt.Sprintf("test_%d", time.Now().UnixNano())

	db, err := sql.Open("pgx", testConnStr)
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	connStr := replaceDBName(testConnStr, dbName)

	// Run migrations.
	logger := zap.New(zap.UseDevMode(true))
	mg, err := pgmigrate.NewMigrator(connStr, logger)
	require.NoError(t, err)
	require.NoError(t, mg.Up())
	require.NoError(t, mg.Close())

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, connStr)
	require.NoError(t, err)

	t.Cleanup(func() {
		pool.Close()
		mainDB, err := sql.Open("pgx", testConnStr)
		if err == nil {
			_, _ = mainDB.Exec(fmt.Sprintf("DROP DATABASE %s WITH (FORCE)", dbName))
			_ = mainDB.Close()
		}
	})

	return pool
}

func replaceDBName(connStr, newDB string) string {
	qIdx := len(connStr)
	for i, c := range connStr {
		if c == '?' {
			qIdx = i
			break
		}
	}
	slashIdx := 0
	for i := qIdx - 1; i >= 0; i-- {
		if connStr[i] == '/' {
			slashIdx = i
			break
		}
	}
	return connStr[:slashIdx+1] + newDB + connStr[qIdx:]
}

func newProvider(t *testing.T) *Provider {
	t.Helper()
	pool := freshDB(t)
	return NewFromPool(pool)
}

func makeSession(id string, now time.Time) *session.Session {
	return &session.Session{
		ID:                id,
		AgentName:         "test-agent",
		Namespace:         "default",
		WorkspaceName:     "test-workspace",
		Status:            session.SessionStatusActive,
		CreatedAt:         now,
		UpdatedAt:         now,
		MessageCount:      0,
		ToolCallCount:     0,
		TotalInputTokens:  0,
		TotalOutputTokens: 0,
		EstimatedCostUSD:  0,
		Tags:              []string{"tag1", "tag2"},
		State:             map[string]string{"key": "value"},
	}
}

func makeMessage(id string, seq int32, now time.Time) *session.Message {
	return &session.Message{
		ID:          id,
		Role:        session.RoleUser,
		Content:     "Hello, world!",
		Timestamp:   now,
		SequenceNum: seq,
	}
}

// --- Session CRUD -----------------------------------------------------------

func TestCreateGetSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	s.ExpiresAt = now.Add(time.Hour)
	s.EndedAt = now.Add(30 * time.Minute)
	s.LastMessagePreview = "Hello!"
	s.EstimatedCostUSD = 1.234567

	require.NoError(t, p.CreateSession(ctx, s))

	got, err := p.GetSession(ctx, s.ID)
	require.NoError(t, err)

	assert.Equal(t, s.ID, got.ID)
	assert.Equal(t, s.AgentName, got.AgentName)
	assert.Equal(t, s.Namespace, got.Namespace)
	assert.Equal(t, s.WorkspaceName, got.WorkspaceName)
	assert.Equal(t, s.Status, got.Status)
	assert.WithinDuration(t, s.CreatedAt, got.CreatedAt, time.Microsecond)
	assert.WithinDuration(t, s.UpdatedAt, got.UpdatedAt, time.Microsecond)
	assert.WithinDuration(t, s.ExpiresAt, got.ExpiresAt, time.Microsecond)
	assert.WithinDuration(t, s.EndedAt, got.EndedAt, time.Microsecond)
	assert.Equal(t, s.Tags, got.Tags)
	assert.Equal(t, s.State, got.State)
	assert.Equal(t, s.LastMessagePreview, got.LastMessagePreview)
	assert.InDelta(t, s.EstimatedCostUSD, got.EstimatedCostUSD, 0.000001)
}

func TestCreateSession_Duplicate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	err := p.CreateSession(ctx, s)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestGetSession_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	_, err := p.GetSession(ctx, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestUpdateSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	s.Status = session.SessionStatusCompleted
	s.UpdatedAt = now.Add(time.Minute)
	s.EndedAt = now.Add(time.Minute)
	s.MessageCount = 5
	s.Tags = []string{"updated"}
	s.State = map[string]string{"new": "state"}
	require.NoError(t, p.UpdateSession(ctx, s))

	got, err := p.GetSession(ctx, s.ID)
	require.NoError(t, err)
	assert.Equal(t, session.SessionStatusCompleted, got.Status)
	assert.Equal(t, int32(5), got.MessageCount)
	assert.Equal(t, []string{"updated"}, got.Tags)
	assert.Equal(t, map[string]string{"new": "state"}, got.State)
}

func TestUpdateSession_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	err := p.UpdateSession(ctx, s)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestDeleteSession(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	msg := makeMessage("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", 1, now)
	require.NoError(t, p.AppendMessage(ctx, s.ID, msg))

	require.NoError(t, p.DeleteSession(ctx, s.ID))

	_, err := p.GetSession(ctx, s.ID)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)

	// Messages should be gone too.
	msgs, err := p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{})
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
	assert.Nil(t, msgs)
}

func TestDeleteSession_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	err := p.DeleteSession(ctx, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

// --- Messages ---------------------------------------------------------------

func TestAppendGetMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	msg := &session.Message{
		ID:           "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
		Role:         session.RoleUser,
		Content:      "Hello!",
		Timestamp:    now,
		InputTokens:  10,
		OutputTokens: 20,
		SequenceNum:  1,
	}
	require.NoError(t, p.AppendMessage(ctx, s.ID, msg))

	msgs, err := p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, msg.ID, msgs[0].ID)
	assert.Equal(t, msg.Content, msgs[0].Content)
	assert.Equal(t, msg.Role, msgs[0].Role)
	assert.Equal(t, int32(10), msgs[0].InputTokens)
	assert.Equal(t, int32(20), msgs[0].OutputTokens)
	assert.Equal(t, int32(1), msgs[0].SequenceNum)
}

func TestAppendMessage_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	msg := makeMessage("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", 1, now)
	err := p.AppendMessage(ctx, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", msg)
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestGetMessages_Ordering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	for i := 1; i <= 3; i++ {
		msg := makeMessage(fmt.Sprintf("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a%02d", i), int32(i), now.Add(time.Duration(i)*time.Second))
		require.NoError(t, p.AppendMessage(ctx, s.ID, msg))
	}

	// ASC (default)
	msgs, err := p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{})
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, int32(1), msgs[0].SequenceNum)
	assert.Equal(t, int32(3), msgs[2].SequenceNum)

	// DESC
	msgs, err = p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{SortOrder: providers.SortDesc})
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, int32(3), msgs[0].SequenceNum)
	assert.Equal(t, int32(1), msgs[2].SequenceNum)
}

func TestGetMessages_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	for i := 1; i <= 5; i++ {
		msg := makeMessage(fmt.Sprintf("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a%02d", i), int32(i), now.Add(time.Duration(i)*time.Second))
		require.NoError(t, p.AppendMessage(ctx, s.ID, msg))
	}

	// Page 1
	msgs, err := p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{Limit: 2})
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, int32(1), msgs[0].SequenceNum)

	// Page 2
	msgs, err = p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{Limit: 2, Offset: 2})
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, int32(3), msgs[0].SequenceNum)
}

func TestGetMessages_FilterSeq(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	for i := 1; i <= 5; i++ {
		msg := makeMessage(fmt.Sprintf("b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a%02d", i), int32(i), now.Add(time.Duration(i)*time.Second))
		require.NoError(t, p.AppendMessage(ctx, s.ID, msg))
	}

	// AfterSeq=2
	msgs, err := p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{AfterSeq: 2})
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, int32(3), msgs[0].SequenceNum)

	// BeforeSeq=4
	msgs, err = p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{BeforeSeq: 4})
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	assert.Equal(t, int32(3), msgs[2].SequenceNum)

	// Combined
	msgs, err = p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{AfterSeq: 1, BeforeSeq: 4})
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	assert.Equal(t, int32(2), msgs[0].SequenceNum)
	assert.Equal(t, int32(3), msgs[1].SequenceNum)
}

func TestGetMessages_FilterRoles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	msgs := []*session.Message{
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", Role: session.RoleUser, Content: "user msg", Timestamp: now, SequenceNum: 1},
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", Role: session.RoleAssistant, Content: "assistant msg", Timestamp: now.Add(time.Second), SequenceNum: 2},
		{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a03", Role: session.RoleSystem, Content: "system msg", Timestamp: now.Add(2 * time.Second), SequenceNum: 3},
	}
	for _, msg := range msgs {
		require.NoError(t, p.AppendMessage(ctx, s.ID, msg))
	}

	result, err := p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{Roles: []session.MessageRole{session.RoleUser, session.RoleSystem}})
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, session.RoleUser, result[0].Role)
	assert.Equal(t, session.RoleSystem, result[1].Role)
}

func TestGetMessages_Empty(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	msgs, err := p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{})
	require.NoError(t, err)
	assert.Empty(t, msgs)
	assert.NotNil(t, msgs, "should return empty slice, not nil")
}

func TestGetMessages_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	_, err := p.GetMessages(ctx, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", providers.MessageQueryOpts{})
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
}

func TestGetMessages_MetadataRoundtrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", now)
	require.NoError(t, p.CreateSession(ctx, s))

	msg := &session.Message{
		ID:          "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
		Role:        session.RoleUser,
		Content:     "Hello!",
		Timestamp:   now,
		SequenceNum: 1,
		Metadata:    map[string]string{"source": "api", "version": "1.0"},
	}
	require.NoError(t, p.AppendMessage(ctx, s.ID, msg))

	msgs, err := p.GetMessages(ctx, s.ID, providers.MessageQueryOpts{})
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, map[string]string{"source": "api", "version": "1.0"}, msgs[0].Metadata)
}

// --- ListSessions -----------------------------------------------------------

func TestListSessions_All(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	for i := range 3 {
		s := makeSession(fmt.Sprintf("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a%02d", i), now.Add(time.Duration(i)*time.Second))
		require.NoError(t, p.CreateSession(ctx, s))
	}

	page, err := p.ListSessions(ctx, providers.SessionListOpts{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), page.TotalCount)
	assert.Len(t, page.Sessions, 3)
	assert.False(t, page.HasMore)
}

func TestListSessions_FilterAgent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s1 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now)
	s1.AgentName = "agent-a"
	s2 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now.Add(time.Second))
	s2.AgentName = "agent-b"
	require.NoError(t, p.CreateSession(ctx, s1))
	require.NoError(t, p.CreateSession(ctx, s2))

	page, err := p.ListSessions(ctx, providers.SessionListOpts{AgentName: "agent-a"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.TotalCount)
	assert.Equal(t, "agent-a", page.Sessions[0].AgentName)
}

func TestListSessions_FilterNamespace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s1 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now)
	s1.Namespace = "ns-a"
	s2 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now.Add(time.Second))
	s2.Namespace = "ns-b"
	require.NoError(t, p.CreateSession(ctx, s1))
	require.NoError(t, p.CreateSession(ctx, s2))

	page, err := p.ListSessions(ctx, providers.SessionListOpts{Namespace: "ns-a"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.TotalCount)
	assert.Equal(t, "ns-a", page.Sessions[0].Namespace)
}

func TestListSessions_FilterStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s1 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now)
	s1.Status = session.SessionStatusActive
	s2 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now.Add(time.Second))
	s2.Status = session.SessionStatusCompleted
	require.NoError(t, p.CreateSession(ctx, s1))
	require.NoError(t, p.CreateSession(ctx, s2))

	page, err := p.ListSessions(ctx, providers.SessionListOpts{Status: session.SessionStatusActive})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.TotalCount)
	assert.Equal(t, session.SessionStatusActive, page.Sessions[0].Status)
}

func TestListSessions_FilterTags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s1 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now)
	s1.Tags = []string{"alpha", "beta"}
	s2 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now.Add(time.Second))
	s2.Tags = []string{"beta", "gamma"}
	require.NoError(t, p.CreateSession(ctx, s1))
	require.NoError(t, p.CreateSession(ctx, s2))

	// Filter for "alpha" — only s1
	page, err := p.ListSessions(ctx, providers.SessionListOpts{Tags: []string{"alpha"}})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.TotalCount)

	// Filter for "beta" — both
	page, err = p.ListSessions(ctx, providers.SessionListOpts{Tags: []string{"beta"}})
	require.NoError(t, err)
	assert.Equal(t, int64(2), page.TotalCount)

	// Filter for "alpha" AND "beta" — only s1
	page, err = p.ListSessions(ctx, providers.SessionListOpts{Tags: []string{"alpha", "beta"}})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.TotalCount)
}

func TestListSessions_FilterDateRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s1 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now.Add(-2*time.Hour))
	s2 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now.Add(-1*time.Hour))
	s3 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a03", now)
	require.NoError(t, p.CreateSession(ctx, s1))
	require.NoError(t, p.CreateSession(ctx, s2))
	require.NoError(t, p.CreateSession(ctx, s3))

	page, err := p.ListSessions(ctx, providers.SessionListOpts{
		CreatedAfter:  now.Add(-90 * time.Minute),
		CreatedBefore: now.Add(-30 * time.Minute),
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.TotalCount)
}

func TestListSessions_SortOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s1 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now)
	s2 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now.Add(time.Second))
	require.NoError(t, p.CreateSession(ctx, s1))
	require.NoError(t, p.CreateSession(ctx, s2))

	// Default (DESC)
	page, err := p.ListSessions(ctx, providers.SessionListOpts{})
	require.NoError(t, err)
	assert.Equal(t, s2.ID, page.Sessions[0].ID)

	// ASC
	page, err = p.ListSessions(ctx, providers.SessionListOpts{SortOrder: providers.SortAsc})
	require.NoError(t, err)
	assert.Equal(t, s1.ID, page.Sessions[0].ID)
}

func TestListSessions_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	for i := range 5 {
		s := makeSession(fmt.Sprintf("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a%02d", i), now.Add(time.Duration(i)*time.Second))
		require.NoError(t, p.CreateSession(ctx, s))
	}

	// Page 1
	page, err := p.ListSessions(ctx, providers.SessionListOpts{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, page.Sessions, 2)
	assert.Equal(t, int64(5), page.TotalCount)
	assert.True(t, page.HasMore)

	// Page 2
	page, err = p.ListSessions(ctx, providers.SessionListOpts{Limit: 2, Offset: 2})
	require.NoError(t, err)
	assert.Len(t, page.Sessions, 2)
	assert.True(t, page.HasMore)

	// Last page
	page, err = p.ListSessions(ctx, providers.SessionListOpts{Limit: 2, Offset: 4})
	require.NoError(t, err)
	assert.Len(t, page.Sessions, 1)
	assert.False(t, page.HasMore)
}

// --- SearchSessions ---------------------------------------------------------

func TestSearchSessions_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s1 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now)
	s2 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now.Add(time.Second))
	require.NoError(t, p.CreateSession(ctx, s1))
	require.NoError(t, p.CreateSession(ctx, s2))

	// Add messages with searchable content.
	msg1 := &session.Message{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", Role: session.RoleUser, Content: "Tell me about Kubernetes deployments", Timestamp: now, SequenceNum: 1}
	msg2 := &session.Message{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", Role: session.RoleUser, Content: "What is a Redis cache", Timestamp: now.Add(time.Second), SequenceNum: 1}
	require.NoError(t, p.AppendMessage(ctx, s1.ID, msg1))
	require.NoError(t, p.AppendMessage(ctx, s2.ID, msg2))

	// Search for "kubernetes"
	page, err := p.SearchSessions(ctx, "kubernetes", providers.SessionListOpts{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.TotalCount)
	assert.Equal(t, s1.ID, page.Sessions[0].ID)
}

func TestSearchSessions_NoResults(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now)
	require.NoError(t, p.CreateSession(ctx, s))

	msg := &session.Message{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", Role: session.RoleUser, Content: "Hello world", Timestamp: now, SequenceNum: 1}
	require.NoError(t, p.AppendMessage(ctx, s.ID, msg))

	page, err := p.SearchSessions(ctx, "nonexistentterm", providers.SessionListOpts{})
	require.NoError(t, err)
	assert.Equal(t, int64(0), page.TotalCount)
	assert.Empty(t, page.Sessions)
}

func TestSearchSessions_WithFilters(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	s1 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now)
	s1.AgentName = "agent-a"
	s2 := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now.Add(time.Second))
	s2.AgentName = "agent-b"
	require.NoError(t, p.CreateSession(ctx, s1))
	require.NoError(t, p.CreateSession(ctx, s2))

	// Both sessions have "kubernetes" in messages.
	msg1 := &session.Message{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", Role: session.RoleUser, Content: "Kubernetes pods", Timestamp: now, SequenceNum: 1}
	msg2 := &session.Message{ID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", Role: session.RoleUser, Content: "Kubernetes services", Timestamp: now.Add(time.Second), SequenceNum: 1}
	require.NoError(t, p.AppendMessage(ctx, s1.ID, msg1))
	require.NoError(t, p.AppendMessage(ctx, s2.ID, msg2))

	// Search with agent filter.
	page, err := p.SearchSessions(ctx, "kubernetes", providers.SessionListOpts{AgentName: "agent-a"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), page.TotalCount)
	assert.Equal(t, "agent-a", page.Sessions[0].AgentName)
}

// --- Partitions -------------------------------------------------------------

func TestCreatePartition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	// Create a partition far in the future to avoid collision with initial partitions.
	futureDate := time.Date(2030, 6, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, p.CreatePartition(ctx, futureDate))

	// Verify partitions were created for all 4 tables.
	pool := p.pool
	for _, table := range partitionTables {
		isoYear, isoWeek := futureDate.ISOWeek()
		partName := fmt.Sprintf("%s_w%04d_%02d", table, isoYear, isoWeek)
		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_class WHERE relname = $1
		)`, partName).Scan(&exists)
		require.NoError(t, err)
		assert.True(t, exists, "partition %s should exist", partName)
	}
}

func TestCreatePartition_AlreadyExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	futureDate := time.Date(2030, 7, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, p.CreatePartition(ctx, futureDate))

	err := p.CreatePartition(ctx, futureDate)
	assert.ErrorIs(t, err, providers.ErrPartitionExists)
}

func TestDropPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	futureDate := time.Date(2030, 8, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, p.CreatePartition(ctx, futureDate))
	require.NoError(t, p.DropPartition(ctx, futureDate))

	// Verify sessions partition is gone.
	isoYear, isoWeek := futureDate.ISOWeek()
	partName := fmt.Sprintf("sessions_w%04d_%02d", isoYear, isoWeek)
	var exists bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_class WHERE relname = $1
	)`, partName).Scan(&exists)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestDropPartition_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	farFuture := time.Date(2040, 1, 1, 0, 0, 0, 0, time.UTC)
	err := p.DropPartition(ctx, farFuture)
	assert.ErrorIs(t, err, providers.ErrPartitionNotFound)
}

func TestListPartitions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	infos, err := p.ListPartitions(ctx)
	require.NoError(t, err)
	// Initial migrations create partitions (4 weeks back + 2 weeks ahead).
	assert.GreaterOrEqual(t, len(infos), 5, "should have at least 5 partitions from initial migration")

	for _, info := range infos {
		assert.NotEmpty(t, info.Name)
		assert.False(t, info.StartDate.IsZero())
		assert.False(t, info.EndDate.IsZero())
		assert.True(t, info.EndDate.After(info.StartDate))
	}
}

// --- Batch operations -------------------------------------------------------

func TestGetSessionsOlderThan(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Create old and new sessions.
	old := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01", now.Add(-2*time.Hour))
	old.UpdatedAt = now.Add(-2 * time.Hour)
	recent := makeSession("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02", now)
	require.NoError(t, p.CreateSession(ctx, old))
	require.NoError(t, p.CreateSession(ctx, recent))

	sessions, err := p.GetSessionsOlderThan(ctx, now.Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, old.ID, sessions[0].ID)
}

func TestDeleteSessionsBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	ids := []string{
		"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a01",
		"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a02",
		"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a03",
	}
	for i, id := range ids {
		s := makeSession(id, now.Add(time.Duration(i)*time.Second))
		require.NoError(t, p.CreateSession(ctx, s))
	}

	// Delete first two.
	require.NoError(t, p.DeleteSessionsBatch(ctx, ids[:2]))

	// Verify first two are gone, third remains.
	_, err := p.GetSession(ctx, ids[0])
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
	_, err = p.GetSession(ctx, ids[1])
	assert.ErrorIs(t, err, session.ErrSessionNotFound)
	_, err = p.GetSession(ctx, ids[2])
	assert.NoError(t, err)
}

// --- Infrastructure ---------------------------------------------------------

func TestPing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	assert.NoError(t, p.Ping(context.Background()))
}

func TestClose_OwnsPool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := freshDB(t)
	p := &Provider{pool: pool, ownsPool: true}
	assert.NoError(t, p.Close())

	// Pool should be closed — Ping should fail.
	err := pool.Ping(context.Background())
	assert.Error(t, err)
}

func TestClose_SharedPool(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := freshDB(t)
	p := &Provider{pool: pool, ownsPool: false}
	assert.NoError(t, p.Close())

	// Pool should still be usable.
	assert.NoError(t, pool.Ping(context.Background()))
}

func TestNew_ConnectionError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	cfg := DefaultConfig()
	cfg.ConnString = "postgres://invalid:5432/nonexistent?sslmode=disable&connect_timeout=1"
	_, err := New(cfg)
	assert.Error(t, err)
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, int32(10), cfg.MaxConns)
	assert.Equal(t, int32(2), cfg.MinConns)
	assert.Equal(t, time.Hour, cfg.MaxConnLifetime)
	assert.Equal(t, 30*time.Minute, cfg.MaxConnIdleTime)
	assert.Equal(t, time.Minute, cfg.HealthCheckPeriod)
	assert.Empty(t, cfg.ConnString)
	assert.Nil(t, cfg.TLS)
}
