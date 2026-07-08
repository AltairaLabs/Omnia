/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

type fakePartitionMaintainer struct {
	calls     atomic.Int32
	lastWeeks atomic.Int32
	err       error
}

func (f *fakePartitionMaintainer) EnsurePartitionsAhead(_ context.Context, weeksAhead int) error {
	f.calls.Add(1)
	f.lastWeeks.Store(int32(weeksAhead))
	return f.err
}

func TestStartPartitionMaintenance_EnsuresOnStartup(t *testing.T) {
	fake := &fakePartitionMaintainer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startPartitionMaintenance(ctx, fake, time.Hour, logr.Discard())

	if got := fake.calls.Load(); got < 1 {
		t.Fatalf("EnsurePartitionsAhead called %d times on startup, want >= 1", got)
	}
	if got := fake.lastWeeks.Load(); got != partitionWeeksAhead {
		t.Fatalf("weeksAhead = %d, want %d", got, partitionWeeksAhead)
	}
}

func TestStartPartitionMaintenance_StartupErrorNotFatal(t *testing.T) {
	// A maintenance failure must be logged, not panic/block startup.
	fake := &fakePartitionMaintainer{err: errors.New("boom")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startPartitionMaintenance(ctx, fake, time.Hour, logr.Discard())

	if fake.calls.Load() < 1 {
		t.Fatal("expected EnsurePartitionsAhead to be attempted despite error")
	}
}

func TestStartPartitionMaintenance_TicksThenStopsOnCancel(t *testing.T) {
	fake := &fakePartitionMaintainer{}
	ctx, cancel := context.WithCancel(context.Background())
	// Tiny interval so the ticker branch fires quickly (startup call + >=1 tick).
	startPartitionMaintenance(ctx, fake, time.Millisecond, logr.Discard())

	deadline := time.Now().Add(2 * time.Second)
	for fake.calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if fake.calls.Load() < 2 {
		t.Fatalf("expected >= 2 calls (startup + tick), got %d", fake.calls.Load())
	}

	// Cancelling ctx must stop the loop (exercises the ctx.Done() branch).
	cancel()
	stopped := fake.calls.Load()
	time.Sleep(20 * time.Millisecond)
	if grew := fake.calls.Load() - stopped; grew > 2 {
		t.Fatalf("loop kept running after cancel: grew by %d", grew)
	}
}
