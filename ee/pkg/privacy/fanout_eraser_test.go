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
	"strings"
	"testing"

	"github.com/go-logr/logr"
)

// compile-time check that FanOutSubjectEraser satisfies SubjectEraser.
var _ SubjectEraser = (*FanOutSubjectEraser)(nil)

type fakeGroupSessionEraser struct {
	byURL    map[string]EraseResult
	errByURL map[string]error
	calls    []string
}

func (f *fakeGroupSessionEraser) Erase(_ context.Context, sessionURL string, _ EraseScope) (EraseResult, error) {
	f.calls = append(f.calls, sessionURL)
	if e := f.errByURL[sessionURL]; e != nil {
		return EraseResult{}, e
	}
	return f.byURL[sessionURL], nil
}

type fakeMemoryDeleter struct {
	err          error
	calls        int
	gotWorkspace string
}

func (f *fakeMemoryDeleter) DeleteAllMemories(_ context.Context, _, workspace string) error {
	f.calls++
	f.gotWorkspace = workspace
	return f.err
}

func newTestFanOut(groups []GroupTarget, se groupSessionEraser, mem *fakeMemoryDeleter) *FanOutSubjectEraser {
	return &FanOutSubjectEraser{
		groups:        groups,
		workspaceUID:  "ws-uid",
		sessionEraser: se,
		newMemory:     func(string) MemoryDeleter { return mem },
		log:           logr.Discard(),
	}
}

func TestFanOut_AllGroupsSucceed_SumsCounts(t *testing.T) {
	se := &fakeGroupSessionEraser{byURL: map[string]EraseResult{
		"http://s-a": {SessionsDeleted: 2},
		"http://s-b": {SessionsDeleted: 3},
	}}
	mem := &fakeMemoryDeleter{}
	f := newTestFanOut([]GroupTarget{
		{Name: "a", SessionURL: "http://s-a", MemoryURL: "http://m-a"},
		{Name: "b", SessionURL: "http://s-b", MemoryURL: "http://m-b"},
	}, se, mem)

	total, errs, err := f.EraseSubject(context.Background(), &DeletionRequest{VirtualUserID: "vu-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want none", errs)
	}
	if mem.calls != 2 {
		t.Fatalf("memory calls = %d, want 2", mem.calls)
	}
	if mem.gotWorkspace != "ws-uid" {
		t.Fatalf("memory workspace = %q, want ws-uid (workspace UID scope)", mem.gotWorkspace)
	}
}

func TestFanOut_OneSessionTargetFails_OthersStillProcessed(t *testing.T) {
	se := &fakeGroupSessionEraser{
		byURL:    map[string]EraseResult{"http://s-b": {SessionsDeleted: 3}},
		errByURL: map[string]error{"http://s-a": errors.New("boom")},
	}
	mem := &fakeMemoryDeleter{}
	f := newTestFanOut([]GroupTarget{
		{Name: "a", SessionURL: "http://s-a", MemoryURL: "http://m-a"},
		{Name: "b", SessionURL: "http://s-b", MemoryURL: "http://m-b"},
	}, se, mem)

	total, errs, err := f.EraseSubject(context.Background(), &DeletionRequest{VirtualUserID: "vu-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3 (group b only)", total)
	}
	if len(errs) != 1 || !contains(errs, "group a sessions:") {
		t.Fatalf("errs = %v, want one group-a sessions error", errs)
	}
	// Both groups' memory still erased.
	if mem.calls != 2 {
		t.Fatalf("memory calls = %d, want 2", mem.calls)
	}
}

func TestFanOut_PropagatesSessionAndMemoryErrors(t *testing.T) {
	se := &fakeGroupSessionEraser{byURL: map[string]EraseResult{
		"http://s-a": {SessionsDeleted: 1, Errors: []string{"session x: warm boom"}},
	}}
	mem := &fakeMemoryDeleter{err: errors.New("mem down")}
	f := newTestFanOut([]GroupTarget{
		{Name: "a", SessionURL: "http://s-a", MemoryURL: "http://m-a"},
	}, se, mem)

	total, errs, err := f.EraseSubject(context.Background(), &DeletionRequest{VirtualUserID: "vu-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if !contains(errs, "group a: session x: warm boom") {
		t.Fatalf("missing session sub-error: %v", errs)
	}
	if !contains(errs, "group a memory: mem down") {
		t.Fatalf("missing memory error: %v", errs)
	}
}

func TestFanOut_NoGroups_ReturnsZero(t *testing.T) {
	f := newTestFanOut(nil, &fakeGroupSessionEraser{}, &fakeMemoryDeleter{})
	total, errs, err := f.EraseSubject(context.Background(), &DeletionRequest{VirtualUserID: "vu-1"})
	if err != nil || total != 0 || len(errs) != 0 {
		t.Fatalf("got total=%d errs=%v err=%v; want 0/nil/nil", total, errs, err)
	}
}

func TestFanOut_SkipsEmptyURLs(t *testing.T) {
	se := &fakeGroupSessionEraser{byURL: map[string]EraseResult{"http://s-a": {SessionsDeleted: 1}}}
	mem := &fakeMemoryDeleter{}
	// group with only a session URL, and group with only a memory URL.
	f := newTestFanOut([]GroupTarget{
		{Name: "a", SessionURL: "http://s-a"},
		{Name: "b", MemoryURL: "http://m-b"},
	}, se, mem)

	total, errs, err := f.EraseSubject(context.Background(), &DeletionRequest{VirtualUserID: "vu-1"})
	if err != nil || len(errs) != 0 {
		t.Fatalf("errs=%v err=%v", errs, err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(se.calls) != 1 || mem.calls != 1 {
		t.Fatalf("session calls=%v memory calls=%d; want 1 each", se.calls, mem.calls)
	}
}

func contains(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}
