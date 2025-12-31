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

package runtime

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session"
)

// SessionAdapter adapts the session.Store interface to the runtime.SessionStore interface.
type SessionAdapter struct {
	store session.Store
	ttl   time.Duration
}

// NewSessionAdapter creates a new session adapter.
func NewSessionAdapter(store session.Store, ttl time.Duration) *SessionAdapter {
	return &SessionAdapter{
		store: store,
		ttl:   ttl,
	}
}

// GetHistory retrieves the conversation history for a session.
func (a *SessionAdapter) GetHistory(ctx context.Context, sessionID string) ([]Message, error) {
	messages, err := a.store.GetMessages(ctx, sessionID)
	if err != nil {
		if err == session.ErrSessionNotFound {
			return nil, nil // Return empty history for new sessions
		}
		return nil, err
	}

	result := make([]Message, len(messages))
	for i, msg := range messages {
		result[i] = Message{
			Role:    string(msg.Role),
			Content: msg.Content,
		}
	}

	return result, nil
}

// AppendMessage adds a message to the session history.
func (a *SessionAdapter) AppendMessage(ctx context.Context, sessionID string, msg Message) error {
	sessionMsg := session.Message{
		ID:        uuid.New().String(),
		Role:      session.MessageRole(msg.Role),
		Content:   msg.Content,
		Timestamp: time.Now(),
	}

	return a.store.AppendMessage(ctx, sessionID, sessionMsg)
}

// CreateSession creates a new session if it doesn't exist.
func (a *SessionAdapter) CreateSession(ctx context.Context, sessionID, agentName, namespace string) error {
	// Check if session already exists
	_, err := a.store.GetSession(ctx, sessionID)
	if err == nil {
		// Session exists, refresh TTL
		return a.store.RefreshTTL(ctx, sessionID, a.ttl)
	}

	if err != session.ErrSessionNotFound {
		return err
	}

	// Create new session
	_, err = a.store.CreateSession(ctx, session.CreateSessionOptions{
		AgentName: agentName,
		Namespace: namespace,
		TTL:       a.ttl,
	})

	return err
}

// Close closes the underlying store.
func (a *SessionAdapter) Close() error {
	return a.store.Close()
}
