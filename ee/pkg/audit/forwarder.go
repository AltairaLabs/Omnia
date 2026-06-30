/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// DefaultForwardInterval is how often the forwarder drains the local
	// audit_log backlog when no interval is configured.
	DefaultForwardInterval = 30 * time.Second
	// DefaultForwardBatchSize is the maximum number of rows shipped per tick.
	DefaultForwardBatchSize = 200
	// auditIngestPath is the privacy-api ingest endpoint the forwarder POSTs to.
	auditIngestPath = "/api/v1/privacy/audit-events"
	// forwardSelectColumns mirrors the column order scanEntry expects. It reads
	// the same local audit_log the Logger writes, casting INET to text via
	// host() so it scans into Entry.IPAddress (a string).
	forwardSelectColumns = `id, "timestamp", event_type, session_id, user_id,
		workspace, agent_name, namespace, query, result_count,
		host(ip_address), user_agent, reason, metadata`
)

// tokenAuthorizer attaches a bearer token to an outbound request. It is the
// subset of *serviceauth.TokenSource the forwarder needs, so a nil-safe fake can
// be injected in tests. Pass nil to send requests unauthenticated.
type tokenAuthorizer interface {
	Authorize(r *http.Request) error
}

// forwardRequest is the JSON body POSTed to the privacy-api ingest endpoint. It
// matches privacy.AuditIngestRequest by field name.
type forwardRequest struct {
	SourceService string   `json:"sourceService"`
	Events        []*Entry `json:"events"`
}

// Forwarder drains a service's local audit_log to privacy-api's central audit
// hub (#1673). Delivery is at-least-once: a batch is marked forwarded only after
// the ingest endpoint returns 2xx, so a crash between POST and UPDATE re-sends
// the batch (the hub dedups on (source_service, source_id)). Failures leave rows
// unforwarded for the next tick.
type Forwarder struct {
	pool          dbPool
	ingestURL     string
	sourceService string
	ts            tokenAuthorizer
	interval      time.Duration
	batchSize     int
	client        *http.Client
	log           logr.Logger

	forwarded prometheus.Counter
	failed    prometheus.Counter
}

// NewForwarder creates a Forwarder. ingestBaseURL is the privacy-api base URL
// (the ingest path is appended). ts attaches a ServiceAccount bearer token; pass
// nil to send unauthenticated (development / tests). interval and batchSize fall
// back to defaults when non-positive. The Prometheus collectors are registered
// with reg; a nil reg skips registration (the counters still work).
func NewForwarder(
	pool dbPool,
	ingestBaseURL, sourceService string,
	ts tokenAuthorizer,
	interval time.Duration,
	batchSize int,
	reg prometheus.Registerer,
	log logr.Logger,
) *Forwarder {
	if interval <= 0 {
		interval = DefaultForwardInterval
	}
	if batchSize <= 0 {
		batchSize = DefaultForwardBatchSize
	}

	forwarded := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "omnia_audit_forwarded_total",
		Help:        "Audit rows successfully forwarded to the privacy-api audit hub.",
		ConstLabels: prometheus.Labels{"source_service": sourceService},
	})
	failed := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        "omnia_audit_forward_failures_total",
		Help:        "Audit forwarding ticks that failed to deliver a batch.",
		ConstLabels: prometheus.Labels{"source_service": sourceService},
	})
	if reg != nil {
		reg.MustRegister(forwarded, failed)
	}

	return &Forwarder{
		pool:          pool,
		ingestURL:     ingestBaseURL + auditIngestPath,
		sourceService: sourceService,
		ts:            ts,
		interval:      interval,
		batchSize:     batchSize,
		client:        &http.Client{Timeout: 15 * time.Second},
		log:           log.WithName("audit-forwarder"),
		forwarded:     forwarded,
		failed:        failed,
	}
}

// Run drives the drain loop until ctx is cancelled. It drains once immediately so
// the startup backlog ships without waiting a full interval, then repeats on each
// tick.
func (f *Forwarder) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	f.drainOnce(ctx)

	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.drainOnce(ctx)
		}
	}
}

// drainOnce ships successive batches until the backlog is empty, an error
// occurs, or a short tick budget is exhausted. Draining the whole backlog in one
// tick keeps the hub current after a spike without waiting many intervals.
func (f *Forwarder) drainOnce(ctx context.Context) {
	for {
		n, err := f.forwardBatch(ctx)
		if err != nil {
			f.failed.Inc()
			f.log.Error(err, "audit forward batch failed", "sourceService", f.sourceService)
			return
		}
		if n < f.batchSize {
			// Partial (or empty) batch means the backlog is drained.
			return
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// forwardBatch reads one batch of unforwarded rows, POSTs them to the hub, and
// marks them forwarded on success. It returns the number of rows read (0 when the
// backlog is empty). Rows are marked forwarded only after a 2xx response, so a
// failure leaves them for retry.
func (f *Forwarder) forwardBatch(ctx context.Context) (int, error) {
	entries, ids, err := f.selectBatch(ctx)
	if err != nil {
		return 0, fmt.Errorf("select unforwarded audit rows: %w", err)
	}
	if len(entries) == 0 {
		return 0, nil
	}

	if postErr := f.post(ctx, entries); postErr != nil {
		return 0, postErr
	}

	if markErr := f.markForwarded(ctx, ids); markErr != nil {
		// The hub already has the rows; failing to mark them only risks a
		// duplicate re-send next tick, which the hub dedups. Surface as error so
		// the failure counter and log fire, but the data is safe.
		return 0, fmt.Errorf("mark forwarded: %w", markErr)
	}

	f.forwarded.Add(float64(len(entries)))
	f.log.V(1).Info("audit batch forwarded",
		"sourceService", f.sourceService, "count", len(entries))
	return len(entries), nil
}

// selectBatch reads up to batchSize unforwarded rows ordered by id, returning the
// mapped entries and their ids (for the subsequent mark-forwarded UPDATE).
func (f *Forwarder) selectBatch(ctx context.Context) ([]*Entry, []int64, error) {
	query := "SELECT " + forwardSelectColumns +
		" FROM audit_log WHERE forwarded_at IS NULL ORDER BY id LIMIT $1"
	rows, err := f.pool.Query(ctx, query, f.batchSize)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	entries, err := scanEntries(rows)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]int64, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	return entries, ids, nil
}

// post sends the batch to the privacy-api ingest endpoint. Any non-2xx response
// or transport error is returned so the rows stay unforwarded.
func (f *Forwarder) post(ctx context.Context, entries []*Entry) error {
	body, err := json.Marshal(forwardRequest{SourceService: f.sourceService, Events: entries})
	if err != nil {
		return fmt.Errorf("marshal forward request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.ingestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build forward request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if f.ts != nil {
		if authErr := f.ts.Authorize(req); authErr != nil {
			return fmt.Errorf("authorize forward request: %w", authErr)
		}
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", f.ingestURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("POST %s returned status %d", f.ingestURL, resp.StatusCode)
	}
	return nil
}

// markForwarded stamps forwarded_at on the delivered rows so they are not sent
// again.
func (f *Forwarder) markForwarded(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := f.pool.Exec(ctx,
		"UPDATE audit_log SET forwarded_at = now() WHERE id = ANY($1)", ids)
	return err
}
