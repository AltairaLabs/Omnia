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

package recorder

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// mockWarmStoreForTest is a minimal mock that tracks appended messages and artifacts.
type mockWarmStoreForTest struct {
	mu        sync.Mutex
	sessions  map[string]*session.Session
	messages  map[string][]*session.Message
	artifacts []*session.Artifact
}

func newMockWarmStoreForTest() *mockWarmStoreForTest {
	return &mockWarmStoreForTest{
		sessions: make(map[string]*session.Session),
		messages: make(map[string][]*session.Message),
	}
}

func (m *mockWarmStoreForTest) CreateSession(_ context.Context, s *session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return nil
}

func (m *mockWarmStoreForTest) GetSession(_ context.Context, id string) (*session.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return nil, session.ErrSessionNotFound
	}
	// Return a copy
	cp := *s
	return &cp, nil
}

func (m *mockWarmStoreForTest) UpdateSession(_ context.Context, s *session.Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
	return nil
}

func (m *mockWarmStoreForTest) DeleteSession(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}

func (m *mockWarmStoreForTest) AppendMessage(_ context.Context, sessionID string, msg *session.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[sessionID] = append(m.messages[sessionID], msg)
	return nil
}

func (m *mockWarmStoreForTest) GetMessages(_ context.Context, sessionID string, _ providers.MessageQueryOpts) ([]*session.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs, ok := m.messages[sessionID]
	if !ok {
		return []*session.Message{}, nil
	}
	cp := make([]*session.Message, len(msgs))
	copy(cp, msgs)
	return cp, nil
}

func (m *mockWarmStoreForTest) ListSessions(_ context.Context, _ providers.SessionListOpts) (*providers.SessionPage, error) {
	return &providers.SessionPage{}, nil
}

func (m *mockWarmStoreForTest) SearchSessions(_ context.Context, _ string, _ providers.SessionListOpts) (*providers.SessionPage, error) {
	return &providers.SessionPage{}, nil
}

func (m *mockWarmStoreForTest) CreatePartition(_ context.Context, _ time.Time) error { return nil }
func (m *mockWarmStoreForTest) DropPartition(_ context.Context, _ time.Time) error   { return nil }
func (m *mockWarmStoreForTest) ListPartitions(_ context.Context) ([]providers.PartitionInfo, error) {
	return nil, nil
}
func (m *mockWarmStoreForTest) GetSessionsOlderThan(_ context.Context, _ time.Time, _ int) ([]*session.Session, error) {
	return nil, nil
}
func (m *mockWarmStoreForTest) DeleteSessionsBatch(_ context.Context, _ []string) error { return nil }

func (m *mockWarmStoreForTest) SaveArtifact(_ context.Context, a *session.Artifact) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.artifacts = append(m.artifacts, a)
	return nil
}

func (m *mockWarmStoreForTest) GetArtifacts(_ context.Context, messageID string) ([]*session.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*session.Artifact
	for _, a := range m.artifacts {
		if a.MessageID == messageID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockWarmStoreForTest) GetSessionArtifacts(_ context.Context, sessionID string) ([]*session.Artifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*session.Artifact
	for _, a := range m.artifacts {
		if a.SessionID == sessionID {
			result = append(result, a)
		}
	}
	return result, nil
}

func (m *mockWarmStoreForTest) DeleteSessionArtifacts(_ context.Context, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := m.artifacts[:0]
	for _, a := range m.artifacts {
		if a.SessionID != sessionID {
			filtered = append(filtered, a)
		}
	}
	m.artifacts = filtered
	return nil
}

func (m *mockWarmStoreForTest) Ping(_ context.Context) error { return nil }
func (m *mockWarmStoreForTest) Close() error                 { return nil }

// getMessages returns a snapshot of messages for a session (for test assertions).
func (m *mockWarmStoreForTest) getMessages(sessionID string) []*session.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	msgs := m.messages[sessionID]
	cp := make([]*session.Message, len(msgs))
	copy(cp, msgs)
	return cp
}

// Compile-time check.
var _ providers.WarmStoreProvider = (*mockWarmStoreForTest)(nil)

// failingBlobStore is a cold.BlobStore that always fails.
type failingBlobStore struct{}

func (f *failingBlobStore) Put(_ context.Context, _ string, _ []byte, _ string) error {
	return fmt.Errorf("cold store unavailable")
}

func (f *failingBlobStore) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, fmt.Errorf("cold store unavailable")
}

func (f *failingBlobStore) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("cold store unavailable")
}

func (f *failingBlobStore) List(_ context.Context, _ string) ([]string, error) {
	return nil, fmt.Errorf("cold store unavailable")
}

func (f *failingBlobStore) Exists(_ context.Context, _ string) (bool, error) {
	return false, fmt.Errorf("cold store unavailable")
}

func (f *failingBlobStore) Ping(_ context.Context) error { return fmt.Errorf("cold store unavailable") }
func (f *failingBlobStore) Close() error                 { return nil }
