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
	"sync"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/session"
)

// defaultFlushInterval is the default period between batch flushes.
const defaultFlushInterval = 3 * time.Second

// statsDelta accumulates status/ended_at updates for a single session.
// Counter fields (messages, tool calls, tokens, cost) are auto-derived
// from AppendMessage and are not batched here.
type statsDelta struct {
	setStatus  session.SessionStatus
	setEndedAt time.Time
}

// statsWriter defines the method used to persist accumulated stats.
// It is satisfied by Provider.UpdateSessionStats.
type statsWriter func(ctx context.Context, sessionID string, update session.SessionStatsUpdate) error

// StatsBatcher collects session stat deltas in memory and flushes them
// to the database periodically, reducing per-message row-lock contention.
type StatsBatcher struct {
	mu       sync.Mutex
	pending  map[string]*statsDelta
	writer   statsWriter
	log      logr.Logger
	interval time.Duration

	stopCh chan struct{}
	done   chan struct{}
}

// NewStatsBatcher creates a batcher that flushes accumulated deltas at the
// given interval using the provided writer function.
func NewStatsBatcher(writer statsWriter, log logr.Logger, interval time.Duration) *StatsBatcher {
	if interval <= 0 {
		interval = defaultFlushInterval
	}
	b := &StatsBatcher{
		pending:  make(map[string]*statsDelta),
		writer:   writer,
		log:      log.WithName("stats-batcher"),
		interval: interval,
		stopCh:   make(chan struct{}),
		done:     make(chan struct{}),
	}
	go b.loop()
	return b
}

// IncrementStats accumulates a stats delta for the given session.
// It is safe for concurrent use.
func (b *StatsBatcher) IncrementStats(sessionID string, update session.SessionStatsUpdate) {
	b.mu.Lock()
	defer b.mu.Unlock()

	d, ok := b.pending[sessionID]
	if !ok {
		d = &statsDelta{}
		b.pending[sessionID] = d
	}
	applyStatusAndEndedAt(d, update)
}

// applyStatusAndEndedAt merges status/ended-at from an update into the delta.
func applyStatusAndEndedAt(d *statsDelta, update session.SessionStatsUpdate) {
	if update.SetStatus != "" {
		d.setStatus = update.SetStatus
	}
	if !update.SetEndedAt.IsZero() {
		d.setEndedAt = update.SetEndedAt
	}
}

// Shutdown stops the background loop and flushes any remaining deltas.
func (b *StatsBatcher) Shutdown() {
	close(b.stopCh)
	<-b.done
}

// Len returns the number of sessions with pending deltas.
func (b *StatsBatcher) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

// loop runs the periodic flush until Shutdown is called.
func (b *StatsBatcher) loop() {
	defer close(b.done)
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.stopCh:
			b.flush()
			return
		}
	}
}

// flush drains pending deltas and writes them to the database.
func (b *StatsBatcher) flush() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	snapshot := b.pending
	b.pending = make(map[string]*statsDelta, len(snapshot))
	b.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for sid, d := range snapshot {
		b.flushOne(ctx, sid, d)
	}
}

// flushOne writes a single session's accumulated delta to the database.
func (b *StatsBatcher) flushOne(ctx context.Context, sid string, d *statsDelta) {
	update := session.SessionStatsUpdate{
		SetStatus:  d.setStatus,
		SetEndedAt: d.setEndedAt,
	}
	if err := b.writer(ctx, sid, update); err != nil {
		b.log.Error(err, "flush stats failed", "sessionID", sid)
	}
}
