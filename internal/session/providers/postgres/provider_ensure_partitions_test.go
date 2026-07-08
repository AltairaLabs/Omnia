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
	"testing"
	"time"
)

func TestEnsurePartitionsAhead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := newProvider(t)
	ctx := context.Background()

	if err := p.EnsurePartitionsAhead(ctx, 4); err != nil {
		t.Fatalf("EnsurePartitionsAhead: %v", err)
	}
	// Idempotent: a second call over the same weeks must not error.
	if err := p.EnsurePartitionsAhead(ctx, 4); err != nil {
		t.Fatalf("EnsurePartitionsAhead (2nd call): %v", err)
	}

	// A partition covering 4 weeks from now must now exist — the exact gap the
	// one-shot migration seed leaves open once its window lapses.
	target := time.Now().UTC().AddDate(0, 0, 4*7)
	parts, err := p.ListPartitions(ctx)
	if err != nil {
		t.Fatalf("ListPartitions: %v", err)
	}
	found := false
	for _, pi := range parts {
		if !target.Before(pi.StartDate) && target.Before(pi.EndDate) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no sessions partition covers %s after EnsurePartitionsAhead(4); got %d partitions", target.Format("2006-01-02"), len(parts))
	}
}
