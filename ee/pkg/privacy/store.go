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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Opt-out scope constants.
const (
	ScopeAll       = "all"
	ScopeWorkspace = "workspace"
	ScopeAgent     = "agent"
)

// dbPool abstracts database operations for testability.
type dbPool interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// ErrPreferencesNotFound is returned when no privacy preferences exist for a user.
var ErrPreferencesNotFound = errors.New("privacy: user preferences not found")

// Preferences represents a user's privacy opt-out preferences.
type Preferences struct {
	UserID           string    `json:"userId"`
	OptOutAll        bool      `json:"optOutAll"`
	OptOutWorkspaces []string  `json:"optOutWorkspaces"`
	OptOutAgents     []string  `json:"optOutAgents"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// PreferencesStore defines the interface for privacy preference persistence.
type PreferencesStore interface {
	GetPreferences(ctx context.Context, userID string) (*Preferences, error)
	SetOptOut(ctx context.Context, userID, scope, target string) error
	RemoveOptOut(ctx context.Context, userID, scope, target string) error
}

// PreferencesPostgresStore implements PreferencesStore using PostgreSQL.
type PreferencesPostgresStore struct {
	pool dbPool
}

// NewPreferencesStore creates a new PreferencesPostgresStore.
func NewPreferencesStore(pool dbPool) *PreferencesPostgresStore {
	return &PreferencesPostgresStore{pool: pool}
}

// Compile-time interface check.
var _ PreferencesStore = (*PreferencesPostgresStore)(nil)

// GetPreferences retrieves privacy preferences for a user.
func (s *PreferencesPostgresStore) GetPreferences(ctx context.Context, userID string) (*Preferences, error) {
	p := &Preferences{UserID: userID}
	err := s.pool.QueryRow(ctx,
		`SELECT opt_out_all, opt_out_workspaces, opt_out_agents, created_at, updated_at
		 FROM user_privacy_preferences WHERE user_id = $1`, userID,
	).Scan(&p.OptOutAll, &p.OptOutWorkspaces, &p.OptOutAgents, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPreferencesNotFound
	}
	if err != nil {
		return nil, err
	}
	normalizeSlices(p)
	return p, nil
}

// SetOptOut sets an opt-out preference for a user.
func (s *PreferencesPostgresStore) SetOptOut(ctx context.Context, userID, scope, target string) error {
	switch scope {
	case ScopeAll:
		return s.upsertOptOutAll(ctx, userID)
	case ScopeWorkspace:
		return s.upsertArrayElement(ctx, userID, "opt_out_workspaces", target)
	case ScopeAgent:
		return s.upsertArrayElement(ctx, userID, "opt_out_agents", target)
	default:
		return errors.New("privacy: invalid opt-out scope")
	}
}

// RemoveOptOut removes an opt-out preference for a user.
func (s *PreferencesPostgresStore) RemoveOptOut(ctx context.Context, userID, scope, target string) error {
	switch scope {
	case ScopeAll:
		return s.clearOptOutAll(ctx, userID)
	case ScopeWorkspace:
		return s.removeArrayElement(ctx, userID, "opt_out_workspaces", target)
	case ScopeAgent:
		return s.removeArrayElement(ctx, userID, "opt_out_agents", target)
	default:
		return errors.New("privacy: invalid opt-out scope")
	}
}

// normalizeSlices ensures nil slices become empty slices for JSON serialization.
func normalizeSlices(p *Preferences) {
	if p.OptOutWorkspaces == nil {
		p.OptOutWorkspaces = []string{}
	}
	if p.OptOutAgents == nil {
		p.OptOutAgents = []string{}
	}
}

func (s *PreferencesPostgresStore) upsertOptOutAll(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO user_privacy_preferences (user_id, opt_out_all, updated_at)
		 VALUES ($1, TRUE, NOW())
		 ON CONFLICT (user_id) DO UPDATE SET opt_out_all = TRUE, updated_at = NOW()`,
		userID)
	return err
}

func (s *PreferencesPostgresStore) clearOptOutAll(ctx context.Context, userID string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE user_privacy_preferences SET opt_out_all = FALSE, updated_at = NOW()
		 WHERE user_id = $1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPreferencesNotFound
	}
	return nil
}

func (s *PreferencesPostgresStore) upsertArrayElement(
	ctx context.Context, userID, column, value string,
) error {
	//nolint:gosec // column is validated by the caller (SetOptOut switch)
	query := `INSERT INTO user_privacy_preferences (user_id, ` + column + `, updated_at)
		VALUES ($1, ARRAY[$2]::TEXT[], NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			` + column + ` = CASE
				WHEN $2 = ANY(user_privacy_preferences.` + column + `)
				THEN user_privacy_preferences.` + column + `
				ELSE array_append(user_privacy_preferences.` + column + `, $2)
			END,
			updated_at = NOW()`
	_, err := s.pool.Exec(ctx, query, userID, value)
	return err
}

func (s *PreferencesPostgresStore) removeArrayElement(
	ctx context.Context, userID, column, value string,
) error {
	//nolint:gosec // column is validated by the caller (RemoveOptOut switch)
	query := `UPDATE user_privacy_preferences SET
		` + column + ` = array_remove(` + column + `, $2), updated_at = NOW()
		WHERE user_id = $1`
	tag, err := s.pool.Exec(ctx, query, userID, value)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPreferencesNotFound
	}
	return nil
}
