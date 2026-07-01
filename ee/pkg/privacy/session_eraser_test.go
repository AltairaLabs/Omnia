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
	"testing"
	"time"

	"github.com/go-logr/logr"
)

// mockSessionDeleter is a test double for SessionDeleter.
type mockSessionDeleter struct {
	ids       []string
	listErr   error
	deleted   []string
	deleteErr map[string]error
	gotUserID string
}

func (m *mockSessionDeleter) ListSessionsByUser(
	_ context.Context, virtualUserID, _ string, _, _ *time.Time,
) ([]string, error) {
	m.gotUserID = virtualUserID
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.ids, nil
}

func (m *mockSessionDeleter) DeleteSession(_ context.Context, sessionID string) error {
	if err := m.deleteErr[sessionID]; err != nil {
		return err
	}
	m.deleted = append(m.deleted, sessionID)
	return nil
}

// mockMediaDeleter records media deletions.
type mockMediaDeleter struct {
	deleted []string
	err     map[string]error
}

func (m *mockMediaDeleter) DeleteSessionMedia(_ context.Context, sessionID string) error {
	if err := m.err[sessionID]; err != nil {
		return err
	}
	m.deleted = append(m.deleted, sessionID)
	return nil
}

func TestSessionEraser_Erase_DeletesSessionsAndMedia(t *testing.T) {
	deleter := &mockSessionDeleter{ids: []string{"s1", "s2", "s3"}}
	media := &mockMediaDeleter{}
	e := NewSessionEraser(deleter, logr.Discard())
	e.SetMediaDeleter(media)

	res, err := e.Erase(context.Background(), EraseScope{VirtualUserID: "vu-1", Workspace: "ws"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.SessionsDeleted != 3 {
		t.Fatalf("SessionsDeleted = %d, want 3", res.SessionsDeleted)
	}
	if len(deleter.deleted) != 3 || len(media.deleted) != 3 {
		t.Fatalf("sessions=%v media=%v", deleter.deleted, media.deleted)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("Errors = %v, want none", res.Errors)
	}
	if deleter.gotUserID != "vu-1" {
		t.Fatalf("gotUserID = %q, want vu-1", deleter.gotUserID)
	}
}

func TestSessionEraser_Erase_FailsClosedOnEmptyUser(t *testing.T) {
	deleter := &mockSessionDeleter{listErr: ErrMissingVirtualUserID}
	e := NewSessionEraser(deleter, logr.Discard())

	_, err := e.Erase(context.Background(), EraseScope{VirtualUserID: ""})
	if !errors.Is(err, ErrMissingVirtualUserID) {
		t.Fatalf("err = %v, want ErrMissingVirtualUserID", err)
	}
	if len(deleter.deleted) != 0 {
		t.Fatalf("deleted %v, want none (fail-closed)", deleter.deleted)
	}
}

func TestSessionEraser_Erase_RecordsPerSessionErrors(t *testing.T) {
	deleter := &mockSessionDeleter{
		ids:       []string{"s1", "s2"},
		deleteErr: map[string]error{"s2": errors.New("boom")},
	}
	media := &mockMediaDeleter{err: map[string]error{"s1": errors.New("media boom")}}
	e := NewSessionEraser(deleter, logr.Discard())
	e.SetMediaDeleter(media)

	res, err := e.Erase(context.Background(), EraseScope{VirtualUserID: "vu-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// s1: session deleted, media fails → counted as error, not deleted.
	// s2: session delete fails → error.
	if res.SessionsDeleted != 0 {
		t.Fatalf("SessionsDeleted = %d, want 0", res.SessionsDeleted)
	}
	if len(res.Errors) != 2 {
		t.Fatalf("Errors = %v, want 2", res.Errors)
	}
}

func TestSessionEraser_Erase_PropagatesListError(t *testing.T) {
	deleter := &mockSessionDeleter{listErr: errors.New("db down")}
	e := NewSessionEraser(deleter, logr.Discard())
	if _, err := e.Erase(context.Background(), EraseScope{VirtualUserID: "vu-1"}); err == nil {
		t.Fatal("expected error, got nil")
	}
}
