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

	"github.com/altairalabs/omnia/internal/serviceauth"
)

// GroupTarget is one service-group's session-api + memory-api base URLs, resolved
// from Workspace.status.services[].
type GroupTarget struct {
	Name       string
	SessionURL string
	MemoryURL  string
}

// groupSessionEraser is the subset of SessionGroupEraser the fan-out needs
// (extracted for testability).
type groupSessionEraser interface {
	Erase(ctx context.Context, sessionURL string, scope EraseScope) (EraseResult, error)
}

// memoryDeleterFactory builds a MemoryDeleter for a memory-api base URL.
type memoryDeleterFactory func(memoryURL string) MemoryDeleter

// FanOutSubjectEraser erases a subject across every service-group in a workspace:
// per group it calls session-api delete-by-user (SessionURL) and memory-api
// batch-delete (MemoryURL, scoped by the workspace UID). Counts are summed and
// per-target failures are recorded (prefixed by group name) without aborting the
// remaining groups. It satisfies SubjectEraser.
type FanOutSubjectEraser struct {
	groups        []GroupTarget
	workspaceUID  string
	sessionEraser groupSessionEraser
	newMemory     memoryDeleterFactory
	log           logr.Logger
}

// NewFanOutSubjectEraser builds a FanOutSubjectEraser. ts authenticates the
// per-group memory batch-delete calls (session calls are authenticated by the
// SessionGroupEraser's own token source).
func NewFanOutSubjectEraser(
	groups []GroupTarget,
	workspaceUID string,
	sessionEraser groupSessionEraser,
	ts *serviceauth.TokenSource,
	log logr.Logger,
) *FanOutSubjectEraser {
	return &FanOutSubjectEraser{
		groups:        groups,
		workspaceUID:  workspaceUID,
		sessionEraser: sessionEraser,
		newMemory: func(memoryURL string) MemoryDeleter {
			return NewMemoryHTTPDeleter(memoryURL, log).WithTokenSource(ts)
		},
		log: log.WithName("fanout-eraser"),
	}
}

// EraseSubject fans erasure out across all groups. Returns the total sessions
// deleted and the aggregated per-target errors. A partial failure is recorded,
// not fatal; the caller marks the request failed when errors are present.
func (f *FanOutSubjectEraser) EraseSubject(ctx context.Context, req *DeletionRequest) (int, []string, error) {
	if len(f.groups) == 0 {
		f.log.Info("DSAR fan-out has no service-group targets; nothing erased (check workspace status)")
		return 0, nil, nil
	}
	total := 0
	var errs []string
	for _, g := range f.groups {
		total += f.eraseGroupSessions(ctx, g, req, &errs)
		f.eraseGroupMemory(ctx, g, req, &errs)
	}
	return total, errs, nil
}

// eraseGroupSessions erases one group's sessions, returning its deleted count and
// appending any errors (prefixed by group name).
func (f *FanOutSubjectEraser) eraseGroupSessions(
	ctx context.Context, g GroupTarget, req *DeletionRequest, errs *[]string,
) int {
	if g.SessionURL == "" {
		return 0
	}
	res, err := f.sessionEraser.Erase(ctx, g.SessionURL, EraseScope{
		VirtualUserID: req.VirtualUserID,
		Workspace:     req.Workspace,
		DateFrom:      req.DateFrom,
		DateTo:        req.DateTo,
	})
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("group %s sessions: %v", g.Name, err))
		return 0
	}
	for _, e := range res.Errors {
		*errs = append(*errs, fmt.Sprintf("group %s: %s", g.Name, e))
	}
	return res.SessionsDeleted
}

// eraseGroupMemory erases one group's memories for the subject, scoped by the
// workspace UID; failures are appended (prefixed by group name).
func (f *FanOutSubjectEraser) eraseGroupMemory(
	ctx context.Context, g GroupTarget, req *DeletionRequest, errs *[]string,
) {
	if g.MemoryURL == "" {
		return
	}
	if err := f.newMemory(g.MemoryURL).DeleteAllMemories(ctx, req.VirtualUserID, f.workspaceUID); err != nil {
		*errs = append(*errs, fmt.Sprintf("group %s memory: %v", g.Name, err))
	}
}

// NoOpSessionDeleter satisfies SessionDeleter without doing anything. privacy-api
// passes it to NewDeletionService because the SubjectEraser path never consults
// the warm-store deleter; it exists only so the field is non-nil.
type NoOpSessionDeleter struct{}

// ListSessionsByUser returns no sessions.
func (NoOpSessionDeleter) ListSessionsByUser(
	context.Context, string, string, *time.Time, *time.Time,
) ([]string, error) {
	return nil, nil
}

// DeleteSession is a no-op.
func (NoOpSessionDeleter) DeleteSession(context.Context, string) error { return nil }
