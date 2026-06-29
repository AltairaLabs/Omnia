/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// Metric name constant for the outbox replay worker.
const metricConsentOutboxStuck = "omnia_privacy_consent_outbox_stuck_total"

// outboxStore is the minimal store interface consumed by OutboxReplayWorker.
// *privacy.PreferencesPostgresStore satisfies it.
type outboxStore interface {
	ListUndeliveredOutbox(ctx context.Context, maxAge time.Duration, limit int) ([]privacy.OutboxEntry, error)
	MarkOutboxDelivered(ctx context.Context, id string) error
	PruneDeliveredOutbox(ctx context.Context, ttl time.Duration) (int64, error)
	CountStuckOutbox(ctx context.Context, stuckAge time.Duration) (int64, error)
}

// OutboxReplayWorker periodically re-delivers undelivered consent-revocation
// outbox rows, prunes old delivered rows, and exposes a stuck-row gauge.
//
// Defaults chosen:
//   - stuckThreshold = retention (a row is "stuck" once it has been undelivered
//     for the full retention window — the same age at which it would stop being
//     retried, so anything that shows up in the gauge needs operator attention).
//   - batch = 100 (enough to drain typical spikes in a single pass without
//     exhausting memory or holding long DB locks).
type OutboxReplayWorker struct {
	store          outboxStore
	notifier       privacy.ConsentNotifier
	interval       time.Duration
	retention      time.Duration
	stuckThreshold time.Duration
	batch          int
	log            logr.Logger
	stuckGauge     prometheus.Gauge
}

// NewOutboxReplayWorker creates an OutboxReplayWorker and registers its Prometheus
// collector with reg. MustRegister panics on duplicate registration, consistent
// with one-shot binary startup wiring.
func NewOutboxReplayWorker(
	store outboxStore,
	notifier privacy.ConsentNotifier,
	interval time.Duration,
	retention time.Duration,
	reg prometheus.Registerer,
	log logr.Logger,
) *OutboxReplayWorker {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: metricConsentOutboxStuck,
		Help: "Undelivered outbox rows older than the stuck threshold.",
	})
	reg.MustRegister(gauge)

	return &OutboxReplayWorker{
		store:          store,
		notifier:       notifier,
		interval:       interval,
		retention:      retention,
		stuckThreshold: retention, // see type-level comment
		batch:          100,       // see type-level comment
		log:            log.WithName("outbox-replay"),
		stuckGauge:     gauge,
	}
}

// Run drives the replay loop until ctx is cancelled. It replays once
// immediately before the first tick so undelivered rows are retried at
// startup, then repeats on every interval tick.
func (w *OutboxReplayWorker) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if err := w.replayOnce(ctx); err != nil {
		w.log.Error(err, "outbox replay failed")
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.replayOnce(ctx); err != nil {
				w.log.Error(err, "outbox replay failed")
			}
		}
	}
}

// replayOnce performs one replay cycle:
//  1. Lists undelivered outbox rows within the retention window.
//  2. For each row, attempts re-delivery; marks delivered on success.
//  3. Prunes old delivered rows.
//  4. Updates the stuck-row gauge.
//
// A list error is returned so Run can log it. Per-entry delivery/mark errors
// are logged but never abort the loop.
func (w *OutboxReplayWorker) replayOnce(ctx context.Context) error {
	entries, err := w.store.ListUndeliveredOutbox(ctx, w.retention, w.batch)
	if err != nil {
		return fmt.Errorf("list undelivered outbox: %w", err)
	}

	for _, e := range entries {
		w.deliverOne(ctx, e)
	}

	if _, pruneErr := w.store.PruneDeliveredOutbox(ctx, w.retention); pruneErr != nil {
		w.log.Error(pruneErr, "prune delivered outbox failed")
	}

	count, countErr := w.store.CountStuckOutbox(ctx, w.stuckThreshold)
	if countErr != nil {
		w.log.Error(countErr, "count stuck outbox failed")
	} else {
		w.stuckGauge.Set(float64(count))
	}

	return nil
}

// deliverOne attempts to re-deliver a single outbox entry. If the notifier
// confirms delivery, the row is marked delivered. Errors are logged and never
// propagated — one bad entry must not block the rest.
func (w *OutboxReplayWorker) deliverOne(ctx context.Context, e privacy.OutboxEntry) {
	delivered, err := w.notifier.NotifyRevocation(ctx, e.UserID, e.Category)
	if err != nil {
		// ConsentNotifier contract states err is always nil; guard defensively.
		w.log.Error(err, "consent notifier returned unexpected error",
			"entryID", e.ID,
			"category", string(e.Category),
		)
		return
	}
	if !delivered {
		w.log.V(1).Info("outbox entry not delivered",
			"entryID", e.ID,
			"category", string(e.Category),
		)
		return
	}
	if markErr := w.store.MarkOutboxDelivered(ctx, e.ID); markErr != nil {
		w.log.Error(markErr, "mark outbox delivered failed",
			"entryID", e.ID,
		)
	}
}
