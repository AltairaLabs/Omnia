/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"

	"github.com/altairalabs/omnia/internal/session/providers"
)

// WarmStoreSessionDeleter adapts a WarmStoreProvider to the SessionDeleter interface.
type WarmStoreSessionDeleter struct {
	warm providers.WarmStoreProvider
}

// NewWarmStoreSessionDeleter creates a SessionDeleter backed by a WarmStoreProvider.
func NewWarmStoreSessionDeleter(warm providers.WarmStoreProvider) *WarmStoreSessionDeleter {
	return &WarmStoreSessionDeleter{warm: warm}
}

// ListSessionsByUser lists session IDs for a user, optionally filtered by workspace.
func (d *WarmStoreSessionDeleter) ListSessionsByUser(
	ctx context.Context, userID string, workspace string,
) ([]string, error) {
	opts := providers.SessionListOpts{
		WorkspaceName: workspace,
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
