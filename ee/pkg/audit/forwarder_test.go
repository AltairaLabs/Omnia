/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// --- mock pool tailored to the forwarder's SELECT/UPDATE ---------------------

// forwarderRows is a pgx.Rows over a fixed set of audit Entry values. Each row is
// projected in the column order scanEntry expects.
type forwarderRows struct {
	entries []*Entry
	idx     int
}

func (r *forwarderRows) Close()                                       {}
func (r *forwarderRows) Err() error                                   { return nil }
func (r *forwarderRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *forwarderRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *forwarderRows) Values() ([]any, error)                       { return nil, nil }
func (r *forwarderRows) RawValues() [][]byte                          { return nil }
func (r *forwarderRows) Conn() *pgx.Conn                              { return nil }

func (r *forwarderRows) Next() bool { return r.idx < len(r.entries) }

func (r *forwarderRows) Scan(dest ...any) error {
	e := r.entries[r.idx]
	r.idx++
	// Column order must mirror scanEntry's expectations.
	*dest[0].(*int64) = e.ID
	*dest[1].(*time.Time) = e.Timestamp
	*dest[2].(*string) = e.EventType
	setStrPtr(dest[3], e.SessionID)
	setStrPtr(dest[4], e.UserID)
	setStrPtr(dest[5], e.Workspace)
	setStrPtr(dest[6], e.AgentName)
	setStrPtr(dest[7], e.Namespace)
	setStrPtr(dest[8], e.Query)
	if e.ResultCount != 0 {
		n := e.ResultCount
		*dest[9].(**int) = &n
	} else {
		*dest[9].(**int) = nil
	}
	setStrPtr(dest[10], e.IPAddress)
	setStrPtr(dest[11], e.UserAgent)
	setStrPtr(dest[12], e.Reason)
	*dest[13].(*[]byte) = nil
	return nil
}

func setStrPtr(dest any, v string) {
	if v == "" {
		*dest.(**string) = nil
		return
	}
	s := v
	*dest.(**string) = &s
}

// forwarderPool is a dbPool whose Query returns successive batches and whose Exec
// records the marked id batches.
type forwarderPool struct {
	mu sync.Mutex

	batches    [][]*Entry // returned by successive Query calls
	queryIdx   int
	queryErr   error
	queryCalls int

	execErr   error
	execCalls int
	markedIDs [][]int64
}

func (p *forwarderPool) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queryCalls++
	if p.queryErr != nil {
		return nil, p.queryErr
	}
	var batch []*Entry
	if p.queryIdx < len(p.batches) {
		batch = p.batches[p.queryIdx]
		p.queryIdx++
	}
	return &forwarderRows{entries: batch}, nil
}

func (p *forwarderPool) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.execCalls++
	if p.execErr != nil {
		return pgconn.CommandTag{}, p.execErr
	}
	if len(args) > 0 {
		if ids, ok := args[0].([]int64); ok {
			p.markedIDs = append(p.markedIDs, ids)
		}
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (p *forwarderPool) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return &mockPgxRow{}
}

func counterValue(t *testing.T, c prometheus.Counter) float64 {
	t.Helper()
	m := &dto.Metric{}
	require.NoError(t, c.Write(m))
	return m.GetCounter().GetValue()
}

func newTestForwarder(pool dbPool, url string, batch int) *Forwarder {
	return NewForwarder(pool, url, "memory-api", nil, time.Hour, batch,
		prometheus.NewRegistry(), zap.New(zap.UseDevMode(true)))
}

// fwdTestWorkspace is shared by forwarder test fixtures to avoid a duplicated
// string literal (goconst).
const fwdTestWorkspace = "ws-1"

func sampleEntries(ids ...int64) []*Entry {
	out := make([]*Entry, 0, len(ids))
	for _, id := range ids {
		out = append(out, &Entry{
			ID:        id,
			Timestamp: time.Now().UTC(),
			EventType: EventMemoryWriteBlocked,
			Workspace: fwdTestWorkspace,
			UserID:    "u1",
		})
	}
	return out
}

// --- tests -------------------------------------------------------------------

func TestForwarder_ForwardBatch_MarksOnlyOn2xx(t *testing.T) {
	var gotBody forwardRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, auditIngestPath, r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]int{"ingested": 2, "duplicates": 0})
	}))
	defer srv.Close()

	pool := &forwarderPool{batches: [][]*Entry{sampleEntries(1, 2)}}
	f := newTestForwarder(pool, srv.URL, 10)

	n, err := f.forwardBatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, n)

	require.Equal(t, "memory-api", gotBody.SourceService)
	require.Len(t, gotBody.Events, 2)
	require.Equal(t, int64(1), gotBody.Events[0].ID)

	require.Equal(t, 1, pool.execCalls, "rows marked forwarded after 2xx")
	require.Equal(t, [][]int64{{1, 2}}, pool.markedIDs)
	require.Equal(t, float64(2), counterValue(t, f.forwarded))
}

func TestForwarder_ForwardBatch_LeavesRowsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	pool := &forwarderPool{batches: [][]*Entry{sampleEntries(1, 2)}}
	f := newTestForwarder(pool, srv.URL, 10)

	_, err := f.forwardBatch(context.Background())
	require.Error(t, err)
	require.Equal(t, 0, pool.execCalls, "no rows marked when ingest fails")
	require.Equal(t, float64(0), counterValue(t, f.forwarded))
}

func TestForwarder_ForwardBatch_LeavesRowsOnTransportError(t *testing.T) {
	pool := &forwarderPool{batches: [][]*Entry{sampleEntries(1)}}
	// Unreachable URL forces a transport error.
	f := newTestForwarder(pool, "http://127.0.0.1:0", 10)

	_, err := f.forwardBatch(context.Background())
	require.Error(t, err)
	require.Equal(t, 0, pool.execCalls)
}

func TestForwarder_ForwardBatch_EmptyBacklogNoPost(t *testing.T) {
	posted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		posted = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool := &forwarderPool{batches: nil} // Query returns empty rows
	f := newTestForwarder(pool, srv.URL, 10)

	n, err := f.forwardBatch(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.False(t, posted, "no POST when backlog empty")
	require.Equal(t, 0, pool.execCalls)
}

func TestForwarder_ForwardBatch_SelectError(t *testing.T) {
	pool := &forwarderPool{queryErr: fmt.Errorf("db down")}
	f := newTestForwarder(pool, "http://example.invalid", 10)

	_, err := f.forwardBatch(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "select unforwarded audit rows")
}

func TestForwarder_ForwardBatch_MarkErrorAfterDelivery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool := &forwarderPool{
		batches: [][]*Entry{sampleEntries(1)},
		execErr: fmt.Errorf("update failed"),
	}
	f := newTestForwarder(pool, srv.URL, 10)

	_, err := f.forwardBatch(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "mark forwarded")
}

func TestForwarder_DrainOnce_DrainsMultipleBatchesThenStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Two full batches (size 2) then a partial batch signals drained.
	pool := &forwarderPool{batches: [][]*Entry{
		sampleEntries(1, 2),
		sampleEntries(3, 4),
		sampleEntries(5), // partial -> stop
	}}
	f := newTestForwarder(pool, srv.URL, 2)

	f.drainOnce(context.Background())

	require.Equal(t, 3, pool.execCalls, "marked all three batches")
	require.Equal(t, float64(5), counterValue(t, f.forwarded))
}

func TestForwarder_DrainOnce_StopsOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	pool := &forwarderPool{batches: [][]*Entry{sampleEntries(1, 2), sampleEntries(3, 4)}}
	f := newTestForwarder(pool, srv.URL, 2)

	f.drainOnce(context.Background())
	require.Equal(t, float64(1), counterValue(t, f.failed))
	require.Equal(t, 1, pool.queryCalls, "stopped after the first failed batch")
}

func TestForwarder_Run_StopsOnContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	pool := &forwarderPool{batches: [][]*Entry{sampleEntries(1)}}
	// A long interval makes the goroutine idle in select after the startup drain,
	// so cancelling never races an in-flight request.
	f := NewForwarder(pool, srv.URL, "session-api", nil, time.Hour, 10,
		prometheus.NewRegistry(), zap.New(zap.UseDevMode(true)))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.Run(ctx); close(done) }()

	// Wait for the startup drain to fully complete (batch forwarded) before
	// cancelling, so no HTTP request is in flight when ctx is cancelled.
	require.Eventually(t, func() bool {
		return counterValue(t, f.forwarded) == 1
	}, 2*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	require.Equal(t, 1, pool.execCalls)
}

func TestForwarder_Run_ReturnsImmediatelyOnCancelledContext(t *testing.T) {
	pool := &forwarderPool{}
	f := newTestForwarder(pool, "http://example.invalid", 10)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.Run(ctx)
	require.Equal(t, 0, pool.queryCalls, "no work on an already-cancelled context")
}

func TestNewForwarder_Defaults(t *testing.T) {
	f := NewForwarder(&forwarderPool{}, "http://privacy", "memory-api", nil, 0, 0, nil,
		zap.New(zap.UseDevMode(true)))
	assert.Equal(t, DefaultForwardInterval, f.interval)
	assert.Equal(t, DefaultForwardBatchSize, f.batchSize)
	assert.Equal(t, "http://privacy"+auditIngestPath, f.ingestURL)
}

// fakeAuthorizer records that Authorize was invoked.
type fakeAuthorizer struct{ called bool }

func (a *fakeAuthorizer) Authorize(r *http.Request) error {
	a.called = true
	r.Header.Set("Authorization", "Bearer test")
	return nil
}

func TestForwarder_Post_AppliesAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	auth := &fakeAuthorizer{}
	f := NewForwarder(&forwarderPool{}, srv.URL, "memory-api", auth, time.Hour, 10,
		prometheus.NewRegistry(), zap.New(zap.UseDevMode(true)))

	err := f.post(context.Background(), sampleEntries(1))
	require.NoError(t, err)
	require.True(t, auth.called)
	require.Equal(t, "Bearer test", gotAuth)
}

func TestForwarder_Post_AuthError(t *testing.T) {
	f := NewForwarder(&forwarderPool{}, "http://privacy", "memory-api",
		errAuthorizer{}, time.Hour, 10, prometheus.NewRegistry(),
		zap.New(zap.UseDevMode(true)))
	err := f.post(context.Background(), sampleEntries(1))
	require.Error(t, err)
	require.Contains(t, err.Error(), "authorize forward request")
}

type errAuthorizer struct{}

func (errAuthorizer) Authorize(*http.Request) error { return fmt.Errorf("no token") }

func TestForwarder_MarkForwarded_EmptyIsNoOp(t *testing.T) {
	pool := &forwarderPool{}
	f := newTestForwarder(pool, "http://privacy", 10)
	require.NoError(t, f.markForwarded(context.Background(), nil))
	require.Equal(t, 0, pool.execCalls)
}
