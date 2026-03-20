/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// lookupMockWarm implements providers.WarmStoreProvider for session lookup tests.
type lookupMockWarm struct {
	sess   *session.Session
	getErr error
}

func (m *lookupMockWarm) CreateSession(context.Context, *session.Session) error { return nil }
func (m *lookupMockWarm) GetSession(_ context.Context, _ string) (*session.Session, error) {
	return m.sess, m.getErr
}
func (m *lookupMockWarm) UpdateSession(context.Context, *session.Session) error { return nil }
func (m *lookupMockWarm) UpdateSessionStatus(context.Context, string, session.SessionStatusUpdate) error {
	return nil
}
func (m *lookupMockWarm) RefreshTTL(context.Context, string, time.Time) error           { return nil }
func (m *lookupMockWarm) DeleteSession(context.Context, string) error                   { return nil }
func (m *lookupMockWarm) AppendMessage(context.Context, string, *session.Message) error { return nil }
func (m *lookupMockWarm) GetMessages(context.Context, string, providers.MessageQueryOpts) ([]*session.Message, error) {
	return nil, nil
}
func (m *lookupMockWarm) ListSessions(context.Context, providers.SessionListOpts) (*providers.SessionPage, error) {
	return nil, nil
}
func (m *lookupMockWarm) SearchSessions(
	_ context.Context, _ string, _ providers.SessionListOpts,
) (*providers.SessionPage, error) {
	return nil, nil
}
func (m *lookupMockWarm) CreatePartition(context.Context, time.Time) error { return nil }
func (m *lookupMockWarm) DropPartition(context.Context, time.Time) error   { return nil }
func (m *lookupMockWarm) ListPartitions(context.Context) ([]providers.PartitionInfo, error) {
	return nil, nil
}
func (m *lookupMockWarm) GetSessionsOlderThan(context.Context, time.Time, int) ([]*session.Session, error) {
	return nil, nil
}
func (m *lookupMockWarm) DeleteSessionsBatch(context.Context, []string) error { return nil }
func (m *lookupMockWarm) RecordToolCall(context.Context, string, *session.ToolCall) error {
	return nil
}
func (m *lookupMockWarm) RecordProviderCall(context.Context, string, *session.ProviderCall) error {
	return nil
}
func (m *lookupMockWarm) GetToolCalls(context.Context, string, providers.PaginationOpts) ([]*session.ToolCall, error) {
	return nil, nil
}
func (m *lookupMockWarm) GetProviderCalls(
	_ context.Context, _ string, _ providers.PaginationOpts,
) ([]*session.ProviderCall, error) {
	return nil, nil
}
func (m *lookupMockWarm) RecordRuntimeEvent(_ context.Context, _ string, _ *session.RuntimeEvent) error {
	return nil
}
func (m *lookupMockWarm) GetRuntimeEvents(
	_ context.Context, _ string, _ providers.PaginationOpts,
) ([]*session.RuntimeEvent, error) {
	return nil, nil
}
func (m *lookupMockWarm) SaveArtifact(context.Context, *session.Artifact) error { return nil }
func (m *lookupMockWarm) GetArtifacts(context.Context, string) ([]*session.Artifact, error) {
	return nil, nil
}
func (m *lookupMockWarm) GetSessionArtifacts(context.Context, string) ([]*session.Artifact, error) {
	return nil, nil
}
func (m *lookupMockWarm) DeleteSessionArtifacts(context.Context, string) error { return nil }
func (m *lookupMockWarm) Ping(context.Context) error                           { return nil }
func (m *lookupMockWarm) Close() error                                         { return nil }

func TestSessionToMetadata(t *testing.T) {
	sess := &session.Session{
		Namespace: "my-ns",
		AgentName: "my-agent",
		ID:        "session-1",
	}
	meta := sessionToMetadata(sess)
	assert.Equal(t, "my-ns", meta.Namespace)
	assert.Equal(t, "my-agent", meta.AgentName)
}

func TestNewWarmStoreSessionLookup(t *testing.T) {
	lookup := NewWarmStoreSessionLookup(nil)
	assert.NotNil(t, lookup)
}

func TestLookupSession_WarmStoreUnavailable(t *testing.T) {
	// Registry with no warm store configured returns ErrProviderNotConfigured.
	registry := providers.NewRegistry()
	lookup := NewWarmStoreSessionLookup(registry)

	meta, err := lookup.LookupSession(context.Background(), "sess-1")

	assert.Nil(t, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "warm store unavailable")
	assert.ErrorIs(t, err, providers.ErrProviderNotConfigured)
}

func TestLookupSession_GetSessionError(t *testing.T) {
	mock := &lookupMockWarm{getErr: errors.New("db connection lost")}
	registry := providers.NewRegistry()
	registry.SetWarmStore(mock)
	lookup := NewWarmStoreSessionLookup(registry)

	meta, err := lookup.LookupSession(context.Background(), "sess-1")

	assert.Nil(t, meta)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session lookup")
	assert.Contains(t, err.Error(), "db connection lost")
}

func TestLookupSession_Success(t *testing.T) {
	mock := &lookupMockWarm{
		sess: &session.Session{
			ID:        "sess-1",
			Namespace: "prod",
			AgentName: "chatbot",
		},
	}
	registry := providers.NewRegistry()
	registry.SetWarmStore(mock)
	lookup := NewWarmStoreSessionLookup(registry)

	meta, err := lookup.LookupSession(context.Background(), "sess-1")

	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.Equal(t, "prod", meta.Namespace)
	assert.Equal(t, "chatbot", meta.AgentName)
}
