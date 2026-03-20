/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"fmt"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// WarmStoreSessionLookup implements SessionLookup by querying the warm store
// (Postgres) for session metadata.
type WarmStoreSessionLookup struct {
	registry *providers.Registry
}

// NewWarmStoreSessionLookup creates a SessionLookup backed by the warm store.
func NewWarmStoreSessionLookup(registry *providers.Registry) *WarmStoreSessionLookup {
	return &WarmStoreSessionLookup{registry: registry}
}

// LookupSession retrieves namespace and agent name from the warm store.
func (l *WarmStoreSessionLookup) LookupSession(ctx context.Context, sessionID string) (*SessionMetadata, error) {
	warm, err := l.registry.WarmStore()
	if err != nil {
		return nil, fmt.Errorf("warm store unavailable: %w", err)
	}

	sess, err := warm.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session lookup: %w", err)
	}

	return sessionToMetadata(sess), nil
}

func sessionToMetadata(sess *session.Session) *SessionMetadata {
	return &SessionMetadata{
		Namespace: sess.Namespace,
		AgentName: sess.AgentName,
	}
}
