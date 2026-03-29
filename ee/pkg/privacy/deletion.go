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
	ErrMissingDateRange  = errors.New("at least one of dateFrom or dateTo is required for date_range scope")
)

// Status constants for deletion requests.
const (
	StatusPending    = "pending"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
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
	ListSessionsByUser(
		ctx context.Context, userID, workspace string,
		dateFrom, dateTo *time.Time,
	) ([]string, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// MemoryDeleter handles memory deletion for privacy requests.
type MemoryDeleter interface {
	DeleteAllMemories(ctx context.Context, userID, workspace string) error
}

// AuditLogger abstracts audit event logging.
type AuditLogger interface {
	LogEvent(ctx context.Context, entry *api.AuditEntry)
}

// Default batch size for processing sessions in chunks.
const DefaultBatchSize = 100

// DeletionProgress tracks the progress of a deletion request.
type DeletionProgress struct {
	TotalSessions   int    `json:"totalSessions"`
	DeletedSessions int    `json:"deletedSessions"`
	FailedSessions  int    `json:"failedSessions"`
	CurrentPhase    string `json:"currentPhase"`
}

// DeletionService orchestrates GDPR/CCPA data deletion requests.
type DeletionService struct {
	store     DeletionStore
	deleter   SessionDeleter
	media     MediaDeleter
	memory    MemoryDeleter
	audit     AuditLogger
	log       logr.Logger
	batchSize int
}

// NewDeletionService creates a new DeletionService.
func NewDeletionService(
	store DeletionStore, deleter SessionDeleter, audit AuditLogger, log logr.Logger,
) *DeletionService {
	return &DeletionService{
		store:     store,
		deleter:   deleter,
		media:     NoOpMediaDeleter{},
		audit:     audit,
		log:       log.WithName("deletion-service"),
		batchSize: DefaultBatchSize,
	}
}

// SetMediaDeleter configures the MediaDeleter for artifact cleanup.
func (s *DeletionService) SetMediaDeleter(m MediaDeleter) {
	if m != nil {
		s.media = m
	}
}

// SetMemoryDeleter configures the MemoryDeleter for memory cleanup during DSAR processing.
func (s *DeletionService) SetMemoryDeleter(m MemoryDeleter) {
	if m != nil {
		s.memory = m
	}
}

// SetBatchSize configures the number of sessions processed per batch.
func (s *DeletionService) SetBatchSize(size int) {
	if size > 0 {
		s.batchSize = size
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
		Status:    StatusPending,
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

	if req.Status != StatusPending {
		return ErrAlreadyProcessing
	}

	// Mark as in-progress.
	now := time.Now().UTC()
	req.Status = StatusInProgress
	req.StartedAt = &now
	if err := s.store.UpdateRequest(ctx, req); err != nil {
		return fmt.Errorf("updating request status: %w", err)
	}

	// Find sessions for the user, applying date range when scope requires it.
	sessionIDs, err := s.deleter.ListSessionsByUser(ctx, req.UserID, req.Workspace, req.DateFrom, req.DateTo)
	if err != nil {
		return s.failRequest(ctx, req, fmt.Sprintf("listing sessions: %v", err))
	}

	// Process sessions in batches.
	s.processBatches(ctx, req, sessionIDs)

	// Delete all memories for the user. Errors are recorded but do not fail the request.
	if s.memory != nil {
		if err := s.memory.DeleteAllMemories(ctx, req.UserID, req.Workspace); err != nil {
			s.log.Error(err, "memory deletion failed",
				"requestID", req.ID,
				"userID", req.UserID,
			)
			req.Errors = append(req.Errors, fmt.Sprintf("memory deletion: %v", err))
		}
	}

	return s.completeRequest(ctx, req)
}

// processBatches iterates over session IDs in configurable batches,
// deleting from the warm store and cleaning up media for each session.
func (s *DeletionService) processBatches(ctx context.Context, req *DeletionRequest, sessionIDs []string) {
	for start := 0; start < len(sessionIDs); start += s.batchSize {
		end := start + s.batchSize
		if end > len(sessionIDs) {
			end = len(sessionIDs)
		}
		batch := sessionIDs[start:end]

		deleted, failed, batchErrors := s.processBatch(ctx, batch)
		req.SessionsDeleted += deleted
		req.Errors = append(req.Errors, batchErrors...)

		s.log.V(1).Info("batch processed",
			"batchStart", start,
			"batchSize", len(batch),
			"deleted", deleted,
			"failed", failed,
		)

		// Persist progress after each batch so callers can poll status.
		s.updateProgress(ctx, req)
	}
}

// processBatch handles a single batch: warm-store deletion then media cleanup.
func (s *DeletionService) processBatch(ctx context.Context, batch []string) (int, int, []string) {
	var deleted, failed int
	var batchErrors []string

	for _, sid := range batch {
		if err := s.deleteSessionAndMedia(ctx, sid); err != nil {
			failed++
			batchErrors = append(batchErrors, fmt.Sprintf("session %s: %v", sid, err))
			s.log.Error(err, "session deletion failed", "sessionID", sid)
			continue
		}
		deleted++
	}
	return deleted, failed, batchErrors
}

// deleteSessionAndMedia deletes a session from the warm store then removes
// any associated media artifacts.
func (s *DeletionService) deleteSessionAndMedia(ctx context.Context, sessionID string) error {
	if err := s.deleter.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("warm store: %w", err)
	}
	if err := s.media.DeleteSessionMedia(ctx, sessionID); err != nil {
		return fmt.Errorf("media: %w", err)
	}
	return nil
}

// updateProgress persists the current deletion progress. Errors are logged
// but do not stop the pipeline.
func (s *DeletionService) updateProgress(ctx context.Context, req *DeletionRequest) {
	if err := s.store.UpdateRequest(ctx, req); err != nil {
		s.log.Error(err, "progress update failed", "requestID", req.ID)
	}
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
	if input.Scope == "date_range" && input.DateFrom == nil && input.DateTo == nil {
		return ErrMissingDateRange
	}
	return nil
}

// GetProgress returns a snapshot of the current deletion progress for a request.
func (s *DeletionService) GetProgress(ctx context.Context, id string) (*DeletionProgress, error) {
	req, err := s.store.GetRequest(ctx, id)
	if err != nil {
		return nil, err
	}

	var phase string
	switch req.Status {
	case StatusInProgress:
		phase = "warm-store"
	case StatusPending:
		phase = StatusPending
	default:
		phase = "complete"
	}

	return &DeletionProgress{
		TotalSessions:   req.SessionsDeleted + len(req.Errors),
		DeletedSessions: req.SessionsDeleted,
		FailedSessions:  len(req.Errors),
		CurrentPhase:    phase,
	}, nil
}

// failRequest marks a deletion request as failed.
func (s *DeletionService) failRequest(ctx context.Context, req *DeletionRequest, errMsg string) error {
	now := time.Now().UTC()
	req.Status = StatusFailed
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
		req.Status = StatusFailed
	} else {
		req.Status = StatusCompleted
	}
	if err := s.store.UpdateRequest(ctx, req); err != nil {
		return fmt.Errorf("updating completed request: %w", err)
	}

	eventType := "deletion_completed"
	if req.Status == StatusFailed {
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
