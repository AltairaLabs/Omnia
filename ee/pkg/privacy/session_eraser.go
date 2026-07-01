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
	"time"

	"github.com/go-logr/logr"
)

// EraseScope selects which of a subject's sessions to erase within one
// session-api's own warm store.
type EraseScope struct {
	VirtualUserID string
	Workspace     string
	DateFrom      *time.Time
	DateTo        *time.Time
}

// EraseResult reports the outcome of a session-tier erase.
type EraseResult struct {
	SessionsDeleted int      `json:"sessions_deleted"`
	Errors          []string `json:"errors"`
}

// SessionEraser performs session-tier DSAR erasure — list + warm delete + media
// cleanup — for a single session-api's own data. It is the session-tier half of
// DSAR, exposed over HTTP so privacy-api can orchestrate erasure across
// service-groups without holding warm-store or object-storage credentials.
// Listing pages internally in the deleter, so this deletes one session at a
// time; a per-session failure is recorded and does not abort the run.
type SessionEraser struct {
	deleter SessionDeleter
	media   MediaDeleter
	log     logr.Logger
}

// NewSessionEraser builds a SessionEraser with a no-op media deleter; override
// with SetMediaDeleter.
func NewSessionEraser(deleter SessionDeleter, log logr.Logger) *SessionEraser {
	return &SessionEraser{
		deleter: deleter,
		media:   NoOpMediaDeleter{},
		log:     log,
	}
}

// SetMediaDeleter installs a media deleter (nil is ignored).
func (e *SessionEraser) SetMediaDeleter(m MediaDeleter) {
	if m != nil {
		e.media = m
	}
}

// Erase lists the subject's sessions and deletes each session and its media.
// It fails closed: an empty VirtualUserID surfaces ErrMissingVirtualUserID from
// the deleter and nothing is deleted. Per-session failures are recorded in
// EraseResult.Errors and do not abort the run.
func (e *SessionEraser) Erase(ctx context.Context, scope EraseScope) (EraseResult, error) {
	ids, err := e.deleter.ListSessionsByUser(ctx, scope.VirtualUserID, scope.Workspace, scope.DateFrom, scope.DateTo)
	if err != nil {
		return EraseResult{}, err
	}
	res := EraseResult{Errors: []string{}}
	for _, id := range ids {
		if derr := e.deleter.DeleteSession(ctx, id); derr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("session %s: %v", id, derr))
			continue
		}
		if merr := e.media.DeleteSessionMedia(ctx, id); merr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("media %s: %v", id, merr))
			continue
		}
		res.SessionsDeleted++
	}
	e.log.V(1).Info("session erase complete",
		"virtualUserID", scope.VirtualUserID,
		"sessionsDeleted", res.SessionsDeleted,
		"errorCount", len(res.Errors))
	return res, nil
}
