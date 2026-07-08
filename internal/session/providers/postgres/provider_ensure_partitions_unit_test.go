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

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/altairalabs/omnia/internal/session/providers"
)

func TestEnsurePartitionsAhead_CreatesEachWeek(t *testing.T) {
	var dates []time.Time
	create := func(_ context.Context, d time.Time) error {
		dates = append(dates, d)
		return nil
	}
	if err := ensurePartitionsAhead(context.Background(), create, 4, time.Now().UTC()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dates) != 5 { // current week + 4 ahead
		t.Fatalf("created %d weeks, want 5", len(dates))
	}
}

func TestEnsurePartitionsAhead_SkipsExisting(t *testing.T) {
	// ErrPartitionExists for every week must be treated as success.
	create := func(_ context.Context, _ time.Time) error { return providers.ErrPartitionExists }
	if err := ensurePartitionsAhead(context.Background(), create, 2, time.Now().UTC()); err != nil {
		t.Fatalf("ErrPartitionExists must not fail: %v", err)
	}
}

func TestEnsurePartitionsAhead_AbortsOnRealError(t *testing.T) {
	boom := errors.New("db down")
	create := func(_ context.Context, _ time.Time) error { return boom }
	err := ensurePartitionsAhead(context.Background(), create, 4, time.Now().UTC())
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want wrapped %v", err, boom)
	}
}
