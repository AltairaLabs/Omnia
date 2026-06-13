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

// defaultDeletionPageSize is the number of session IDs fetched per page when
// listing a subject's sessions for erasure. It is a page size, not a total cap:
// ListSessionsByUser pages until the store is exhausted.
const defaultDeletionPageSize = 10000

// WarmStoreSessionDeleter adapts a WarmStoreProvider to the SessionDeleter interface.
type WarmStoreSessionDeleter struct {
	warm     providers.WarmStoreProvider
	pageSize int
}

// NewWarmStoreSessionDeleter creates a SessionDeleter backed by a WarmStoreProvider.
func NewWarmStoreSessionDeleter(warm providers.WarmStoreProvider) *WarmStoreSessionDeleter {
	return &WarmStoreSessionDeleter{warm: warm, pageSize: defaultDeletionPageSize}
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

	pageSize := d.pageSize
	if pageSize <= 0 {
		pageSize = defaultDeletionPageSize
	}
	opts.Limit = pageSize

	// Page through every matching session. A single capped pass would leave a
	// subject with more than pageSize sessions partially erased while still
	// reporting completion (issue #1392). Listing is read-only and runs before
	// any deletion, so offset paging walks a stable result set: each full page
	// advances the offset until a short (or empty) page signals exhaustion.
	var ids []string
	for {
		opts.Offset = len(ids)
		page, err := d.warm.ListSessions(ctx, opts)
		if err != nil {
			return nil, err
		}
		for _, s := range page.Sessions {
			ids = append(ids, s.ID)
		}
		if len(page.Sessions) < pageSize {
			break
		}
	}
	return ids, nil
}

// DeleteSession deletes a single session from the warm store.
func (d *WarmStoreSessionDeleter) DeleteSession(ctx context.Context, sessionID string) error {
	return d.warm.DeleteSession(ctx, sessionID)
}
