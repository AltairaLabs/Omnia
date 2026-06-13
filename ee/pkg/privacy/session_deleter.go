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
	"time"

	"github.com/altairalabs/omnia/internal/session/providers"
)

// ErrMissingVirtualUserID is returned when a user-scoped delete is attempted
// without a pseudonym. Fail closed — never fall back to listing all sessions.
var ErrMissingVirtualUserID = errors.New("virtual_user_id is required for user-scoped session deletion")

// WarmStoreSessionDeleter adapts a WarmStoreProvider to the SessionDeleter interface.
type WarmStoreSessionDeleter struct {
	warm providers.WarmStoreProvider
}

// NewWarmStoreSessionDeleter creates a SessionDeleter backed by a WarmStoreProvider.
func NewWarmStoreSessionDeleter(warm providers.WarmStoreProvider) *WarmStoreSessionDeleter {
	return &WarmStoreSessionDeleter{warm: warm}
}

// ListSessionsByUser lists session IDs for a pseudonymous subject, optionally
// filtered by workspace and date range. It fails closed: an empty virtualUserID
// returns ErrMissingVirtualUserID and never queries the store, so a per-user
// erasure request can never degrade into listing every user's sessions.
func (d *WarmStoreSessionDeleter) ListSessionsByUser(
	ctx context.Context, virtualUserID string, workspace string, dateFrom *time.Time, dateTo *time.Time,
) ([]string, error) {
	if virtualUserID == "" {
		return nil, ErrMissingVirtualUserID
	}
	opts := providers.SessionListOpts{
		WorkspaceName: workspace,
		VirtualUserID: virtualUserID,
	}
	if dateFrom != nil {
		opts.CreatedAfter = *dateFrom
	}
	if dateTo != nil {
		opts.CreatedBefore = *dateTo
	}
	// Use a large limit to get all sessions. In production, this should
	// be paginated, but for deletion workflows we need all IDs.
	opts.Limit = 10000

	page, err := d.warm.ListSessions(ctx, opts)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(page.Sessions))
	for _, s := range page.Sessions {
		ids = append(ids, s.ID)
	}
	return ids, nil
}

// DeleteSession deletes a single session from the warm store.
func (d *WarmStoreSessionDeleter) DeleteSession(ctx context.Context, sessionID string) error {
	return d.warm.DeleteSession(ctx, sessionID)
}
