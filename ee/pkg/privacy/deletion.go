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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/session/api"
)

// Sentinel errors returned by the deletion service.
var (
	ErrMissingUserID     = errors.New("user_id is required")
	ErrInvalidReason     = errors.New("reason must be one of: gdpr_erasure, ccpa_delete, user_request")
	ErrInvalidScope      = errors.New("scope must be one of: all, workspace, date_range")
	ErrRequestNotFound   = errors.New("deletion request not found")
	ErrAlreadyProcessing = errors.New("deletion request is already being processed")
)

// Valid reason and scope values.
var (
	validReasons = map[string]bool{
		"gdpr_erasure": true,
		"ccpa_delete":  true,
		"user_request": true,
	}
	validScopes = map[string]bool{
		"all":        true,
		"workspace":  true,
		"date_range": true,
	}
)

// DeletionRequest represents a user data deletion request.
type DeletionRequest struct {
	ID              string     `json:"id"`
	UserID          string     `json:"userId"`
	Reason          string     `json:"reason"`
	Scope           string     `json:"scope"`
	Workspace       string     `json:"workspace,omitempty"`
	DateFrom        *time.Time `json:"dateFrom,omitempty"`
	DateTo          *time.Time `json:"dateTo,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"createdAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
	SessionsDeleted int        `json:"sessionsDeleted"`
	Errors          []string   `json:"errors"`
}

// CreateDeletionRequest is the input for creating a new deletion request.
type CreateDeletionRequest struct {
	UserID    string     `json:"userId"`
	Reason    string     `json:"reason"`
	Scope     string     `json:"scope"`
	Workspace string     `json:"workspace,omitempty"`
	DateFrom  *time.Time `json:"dateFrom,omitempty"`
	DateTo    *time.Time `json:"dateTo,omitempty"`
}

// DeletionStore abstracts persistence for deletion requests.
type DeletionStore interface {
	CreateRequest(ctx context.Context, req *DeletionRequest) error
	GetRequest(ctx context.Context, id string) (*DeletionRequest, error)
	UpdateRequest(ctx context.Context, req *DeletionRequest) error
	ListRequestsByUser(ctx context.Context, userID string) ([]*DeletionRequest, error)
}

// SessionDeleter abstracts session lookup and deletion across storage tiers.
type SessionDeleter interface {
	ListSessionsByUser(ctx context.Context, userID string, workspace string) ([]string, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// AuditLogger abstracts audit event logging.
type AuditLogger interface {
	LogEvent(ctx context.Context, entry *api.AuditEntry)
}

// DeletionService orchestrates GDPR/CCPA data deletion requests.
type DeletionService struct {
	store   DeletionStore
	deleter SessionDeleter
	audit   AuditLogger
	log     logr.Logger
}

// NewDeletionService creates a new DeletionService.
func NewDeletionService(
	store DeletionStore, deleter SessionDeleter, audit AuditLogger, log logr.Logger,
) *DeletionService {
	return &DeletionService{
		store:   store,
		deleter: deleter,
		audit:   audit,
		log:     log.WithName("deletion-service"),
	}
}

// CreateRequest validates and persists a new deletion request.
func (s *DeletionService) CreateRequest(ctx context.Context, input *CreateDeletionRequest) (*DeletionRequest, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	req := &DeletionRequest{
		ID:        uuid.New().String(),
		UserID:    input.UserID,
		Reason:    input.Reason,
		Scope:     input.Scope,
		Workspace: input.Workspace,
		DateFrom:  input.DateFrom,
		DateTo:    input.DateTo,
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
		Errors:    []string{},
	}

	if err := s.store.CreateRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("creating deletion request: %w", err)
	}

	s.logAuditEvent(ctx, "deletion_requested", req)
	return req, nil
}

// ProcessRequest executes a pending deletion request by finding and deleting sessions.
func (s *DeletionService) ProcessRequest(ctx context.Context, id string) error {
	req, err := s.store.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	if req.Status != "pending" {
		return ErrAlreadyProcessing
	}

	// Mark as in-progress.
	now := time.Now().UTC()
	req.Status = "in_progress"
	req.StartedAt = &now
	if err := s.store.UpdateRequest(ctx, req); err != nil {
		return fmt.Errorf("updating request status: %w", err)
	}

	// Find sessions for the user.
	sessionIDs, err := s.deleter.ListSessionsByUser(ctx, req.UserID, req.Workspace)
	if err != nil {
		return s.failRequest(ctx, req, fmt.Sprintf("listing sessions: %v", err))
	}

	// Delete each session, collecting errors.
	deleted, delErrors := s.deleteSessions(ctx, sessionIDs)
	req.SessionsDeleted = deleted
	req.Errors = delErrors

	return s.completeRequest(ctx, req)
}

// GetRequest retrieves a deletion request by ID.
func (s *DeletionService) GetRequest(ctx context.Context, id string) (*DeletionRequest, error) {
	return s.store.GetRequest(ctx, id)
}

// ListRequestsByUser retrieves all deletion requests for a given user.
func (s *DeletionService) ListRequestsByUser(ctx context.Context, userID string) ([]*DeletionRequest, error) {
	return s.store.ListRequestsByUser(ctx, userID)
}

// validateInput checks the CreateDeletionRequest fields.
func validateInput(input *CreateDeletionRequest) error {
	if input.UserID == "" {
		return ErrMissingUserID
	}
	if !validReasons[input.Reason] {
		return ErrInvalidReason
	}
	if input.Scope == "" {
		input.Scope = "all"
	}
	if !validScopes[input.Scope] {
		return ErrInvalidScope
	}
	return nil
}

// deleteSessions deletes sessions one by one, collecting errors for partial failures.
func (s *DeletionService) deleteSessions(ctx context.Context, sessionIDs []string) (int, []string) {
	var deleted int
	var delErrors []string
	for _, sid := range sessionIDs {
		if err := s.deleter.DeleteSession(ctx, sid); err != nil {
			delErrors = append(delErrors, fmt.Sprintf("session %s: %v", sid, err))
			s.log.Error(err, "failed to delete session", "sessionID", sid)
			continue
		}
		deleted++
	}
	return deleted, delErrors
}

// failRequest marks a deletion request as failed.
func (s *DeletionService) failRequest(ctx context.Context, req *DeletionRequest, errMsg string) error {
	now := time.Now().UTC()
	req.Status = "failed"
	req.CompletedAt = &now
	req.Errors = append(req.Errors, errMsg)
	if updateErr := s.store.UpdateRequest(ctx, req); updateErr != nil {
		return fmt.Errorf("updating failed request: %w", updateErr)
	}
	s.logAuditEvent(ctx, "deletion_failed", req)
	return fmt.Errorf("deletion failed: %s", errMsg)
}

// completeRequest marks a deletion request as completed or failed based on errors.
func (s *DeletionService) completeRequest(ctx context.Context, req *DeletionRequest) error {
	now := time.Now().UTC()
	req.CompletedAt = &now
	if len(req.Errors) > 0 {
		req.Status = "failed"
	} else {
		req.Status = "completed"
	}
	if err := s.store.UpdateRequest(ctx, req); err != nil {
		return fmt.Errorf("updating completed request: %w", err)
	}

	eventType := "deletion_completed"
	if req.Status == "failed" {
		eventType = "deletion_failed"
	}
	s.logAuditEvent(ctx, eventType, req)
	return nil
}

// logAuditEvent emits an audit log entry for a deletion operation.
func (s *DeletionService) logAuditEvent(ctx context.Context, eventType string, req *DeletionRequest) {
	if s.audit == nil {
		return
	}
	s.audit.LogEvent(ctx, &api.AuditEntry{
		EventType: eventType,
		Metadata: map[string]string{
			"deletion_request_id": req.ID,
			"user_id":             req.UserID,
			"reason":              req.Reason,
			"scope":               req.Scope,
			"sessions_deleted":    fmt.Sprintf("%d", req.SessionsDeleted),
		},
	})
}
