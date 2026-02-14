/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresDeletionStore implements DeletionStore using PostgreSQL.
type PostgresDeletionStore struct {
	pool dbPool
}

// NewPostgresDeletionStore creates a new PostgresDeletionStore.
func NewPostgresDeletionStore(pool *pgxpool.Pool) *PostgresDeletionStore {
	return &PostgresDeletionStore{pool: pool}
}

// Compile-time interface check.
var _ DeletionStore = (*PostgresDeletionStore)(nil)

const insertDeletionRequestSQL = `
	INSERT INTO deletion_requests (id, user_id, reason, scope, workspace,
		date_from, date_to, status, created_at, sessions_deleted, errors)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

const getDeletionRequestSQL = `
	SELECT id, user_id, reason, scope, workspace,
		date_from, date_to, status, created_at,
		started_at, completed_at, sessions_deleted, errors
	FROM deletion_requests WHERE id = $1`

const updateDeletionRequestSQL = `
	UPDATE deletion_requests
	SET status = $1, started_at = $2, completed_at = $3,
		sessions_deleted = $4, errors = $5
	WHERE id = $6`

const listDeletionRequestsByUserSQL = `
	SELECT id, user_id, reason, scope, workspace,
		date_from, date_to, status, created_at,
		started_at, completed_at, sessions_deleted, errors
	FROM deletion_requests
	WHERE user_id = $1
	ORDER BY created_at DESC`

// CreateRequest inserts a new deletion request into the database.
func (s *PostgresDeletionStore) CreateRequest(
	ctx context.Context, req *DeletionRequest,
) error {
	errorsJSON, err := json.Marshal(req.Errors)
	if err != nil {
		return fmt.Errorf("marshal errors: %w", err)
	}

	_, err = s.pool.Exec(ctx, insertDeletionRequestSQL,
		req.ID, req.UserID, req.Reason, req.Scope,
		nullableString(req.Workspace),
		req.DateFrom, req.DateTo,
		req.Status, req.CreatedAt, req.SessionsDeleted, errorsJSON,
	)
	if err != nil {
		return fmt.Errorf("insert deletion request: %w", err)
	}
	return nil
}

// GetRequest retrieves a deletion request by ID.
func (s *PostgresDeletionStore) GetRequest(
	ctx context.Context, id string,
) (*DeletionRequest, error) {
	row := s.pool.QueryRow(ctx, getDeletionRequestSQL, id)
	return scanDeletionRequest(row)
}

// UpdateRequest updates an existing deletion request.
func (s *PostgresDeletionStore) UpdateRequest(
	ctx context.Context, req *DeletionRequest,
) error {
	errorsJSON, err := json.Marshal(req.Errors)
	if err != nil {
		return fmt.Errorf("marshal errors: %w", err)
	}

	_, err = s.pool.Exec(ctx, updateDeletionRequestSQL,
		req.Status, req.StartedAt, req.CompletedAt,
		req.SessionsDeleted, errorsJSON, req.ID,
	)
	if err != nil {
		return fmt.Errorf("update deletion request: %w", err)
	}
	return nil
}

// ListRequestsByUser retrieves all deletion requests for a user.
func (s *PostgresDeletionStore) ListRequestsByUser(
	ctx context.Context, userID string,
) ([]*DeletionRequest, error) {
	rows, err := s.pool.Query(ctx, listDeletionRequestsByUserSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("query deletion requests: %w", err)
	}
	defer rows.Close()

	var result []*DeletionRequest
	for rows.Next() {
		req, scanErr := scanDeletionRequest(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, req)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate deletion requests: %w", err)
	}

	if result == nil {
		result = []*DeletionRequest{}
	}
	return result, nil
}

// scanDeletionRequest scans a single row into a DeletionRequest.
func scanDeletionRequest(row pgx.Row) (*DeletionRequest, error) {
	var req DeletionRequest
	var workspace *string
	var errorsJSON []byte

	err := row.Scan(
		&req.ID, &req.UserID, &req.Reason, &req.Scope, &workspace,
		&req.DateFrom, &req.DateTo, &req.Status, &req.CreatedAt,
		&req.StartedAt, &req.CompletedAt, &req.SessionsDeleted,
		&errorsJSON,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrRequestNotFound
		}
		return nil, fmt.Errorf("scan deletion request: %w", err)
	}

	if workspace != nil {
		req.Workspace = *workspace
	}

	if len(errorsJSON) > 0 {
		if jsonErr := json.Unmarshal(errorsJSON, &req.Errors); jsonErr != nil {
			return nil, fmt.Errorf("unmarshal errors: %w", jsonErr)
		}
	}
	if req.Errors == nil {
		req.Errors = []string{}
	}

	return &req, nil
}

// nullableString returns a pointer to s if non-empty, or nil.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
