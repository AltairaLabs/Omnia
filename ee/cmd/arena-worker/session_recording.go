/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"sync"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/events"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	"github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/identity"
)

// arenaSessionNamespace is the UUID namespace for deriving deterministic session
// UUIDs from PromptKit run IDs via UUID5.
var arenaSessionNamespace = uuid.MustParse("a0e1c2d3-b4f5-6789-abcd-ef0123456789")

// arenaSessionMetadata carries arena context written to session InitialState.
type arenaSessionMetadata struct {
	JobName       string
	Namespace     string
	WorkspaceName string
	Scenario      string
	ProviderID    string
	JobType       string
	TrialIndex    string
}

// arenaSessionManager lazily creates PostgreSQL sessions for arena engine runs.
// Each unique event.SessionID (= PromptKit runID) gets its own session and
// OmniaEventStore instance. Safe for concurrent use by multiple engine runs.
type arenaSessionManager struct {
	store      session.Store
	log        logr.Logger
	meta       arenaSessionMetadata
	workItemID string   // unique per trial — used to derive deterministic session UUIDs
	sessions   sync.Map // runSessionID (string) → *managedSession
}

type managedSession struct {
	pgSessionID string
	eventStore  *runtime.OmniaEventStore
	failed      bool // set when an arena.run.failed event is observed
}

func newArenaSessionManager(
	store session.Store, log logr.Logger, meta arenaSessionMetadata, workItemID string,
) *arenaSessionManager {
	return &arenaSessionManager{
		store:      store,
		log:        log.WithName("arena-session-mgr"),
		meta:       meta,
		workItemID: workItemID,
	}
}

// runIDToUUID derives a deterministic UUID from a PromptKit run ID.
func runIDToUUID(runID string) string {
	return uuid.NewSHA1(arenaSessionNamespace, []byte(runID)).String()
}

// sourceInteractiveTag is the source tag the facade applies to a normal WS
// connection. The fleet client connects the same way, so the arena decoration
// path removes it (replacing it with sourceArenaTag) to avoid double-counting
// load-test traffic as interactive user sessions.
const (
	sourceInteractiveTag = "source:interactive"
	sourceArenaTag       = "source:arena"
)

// arenaSessionTags returns the tags that identify a session as belonging to an
// arena run. Shared between the lazily-created arena session (direct providers)
// and the facade-session decoration path (fleet/loadtest) so both label sessions
// identically.
func arenaSessionTags(meta arenaSessionMetadata) []string {
	tags := []string{
		sourceArenaTag,
		"arena-job:" + meta.JobName,
		"scenario:" + meta.Scenario,
		"provider:" + meta.ProviderID,
	}
	if meta.TrialIndex != "" {
		tags = append(tags, "trial:"+meta.TrialIndex)
	}
	return tags
}

// arenaSessionState returns the arena context written to a session's state.
// runID may be empty (e.g. when decorating a facade session that has its own ID).
func arenaSessionState(meta arenaSessionMetadata, runID string) map[string]string {
	state := map[string]string{
		"arena.job":           meta.JobName,
		"arena.job.name":      meta.JobName,
		"arena.job.namespace": meta.Namespace,
		"arena.scenario":      meta.Scenario,
		"arena.scenario.id":   meta.Scenario,
		"arena.provider":      meta.ProviderID,
		"arena.provider.id":   meta.ProviderID,
		"arena.type":          meta.JobType,
	}
	if runID != "" {
		state["arena.run_id"] = runID
	}
	if meta.TrialIndex != "" {
		state["arena.trial.index"] = meta.TrialIndex
	}
	return state
}

// OnEvent is a bus subscriber that lazily creates sessions and delegates to
// per-session OmniaEventStore instances.
func (m *arenaSessionManager) OnEvent(event *events.Event) {
	if event.SessionID == "" {
		return
	}

	runSessionID := event.SessionID
	// Derive session UUID from work item ID (unique per trial) rather than
	// PromptKit run ID (which can collide across concurrent workers).
	pgID := runIDToUUID(m.workItemID + ":" + runSessionID)

	// Fast path: session already exists.
	if v, ok := m.sessions.Load(runSessionID); ok {
		ms := v.(*managedSession)
		if isRunFailedEvent(event) {
			ms.failed = true
		}
		event.SessionID = ms.pgSessionID
		ms.eventStore.OnEvent(event)
		return
	}

	// Slow path: create session lazily. LoadOrStore ensures only one goroutine creates it.
	ms := &managedSession{pgSessionID: pgID}
	if actual, loaded := m.sessions.LoadOrStore(runSessionID, ms); loaded {
		// Another goroutine created it first — use theirs.
		ms = actual.(*managedSession)
		event.SessionID = ms.pgSessionID
		ms.eventStore.OnEvent(event)
		return
	}

	// We won the race — create the session and event store.
	opts := session.SessionRecordOptions{
		ID:            pgID,
		AgentName:     m.meta.JobName,
		Namespace:     m.meta.Namespace,
		WorkspaceName: m.meta.WorkspaceName,
		Tags:          arenaSessionTags(m.meta),
		InitialState:  arenaSessionState(m.meta, runSessionID),
		// Loadtest runs have no human; the per-run id is the synthetic subject.
		// guaranteed non-empty: empty SessionID is rejected at the top of OnEvent.
		VirtualUserID: identity.PseudonymizeID(runSessionID),
	}

	_, err := m.createSessionWithRetry(opts)
	if err != nil {
		m.log.Error(err, "failed to create arena session",
			"runID", runSessionID, "pgSessionID", pgID)
		m.sessions.Delete(runSessionID)
		return
	}

	es := runtime.NewOmniaEventStore(m.store, m.log)
	es.SetSessionID(pgID)
	ms.eventStore = es
	if isRunFailedEvent(event) {
		ms.failed = true
	}

	m.log.Info("arena session created",
		"runID", runSessionID, "pgSessionID", pgID)

	event.SessionID = pgID
	es.OnEvent(event)
}

const (
	sessionCreateMaxRetries = 3
	sessionCreateBaseWait   = 500 * time.Millisecond
	sessionCreateTimeout    = 10 * time.Second
)

// createSessionWithRetry attempts to create a session with exponential backoff.
// Transient errors (timeouts, server errors) are retried up to sessionCreateMaxRetries times.
func (m *arenaSessionManager) createSessionWithRetry(opts session.SessionRecordOptions) (*session.Session, error) {
	var lastErr error
	for attempt := range sessionCreateMaxRetries {
		if attempt > 0 {
			wait := sessionCreateBaseWait << uint(attempt-1)
			m.log.V(1).Info("retrying session creation",
				"pgSessionID", opts.ID, "attempt", attempt+1, "backoff", wait.String())
			time.Sleep(wait)
		}

		ctx, cancel := context.WithTimeout(context.Background(), sessionCreateTimeout)
		sess, err := m.store.EnsureSessionRecord(ctx, opts)
		cancel()

		if err == nil {
			return sess, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// SessionIDs returns all PostgreSQL session IDs created by this manager.
func (m *arenaSessionManager) SessionIDs() []string {
	var ids []string
	m.sessions.Range(func(_, value any) bool {
		ms := value.(*managedSession)
		if ms.pgSessionID != "" {
			ids = append(ids, ms.pgSessionID)
		}
		return true
	})
	return ids
}

// CompleteAll marks all lazily created sessions as completed or errored
// based on whether an arena.run.failed event was observed for the session.
func (m *arenaSessionManager) CompleteAll(ctx context.Context) {
	m.sessions.Range(func(key, value any) bool {
		ms := value.(*managedSession)
		if ms.eventStore == nil {
			return true
		}
		status := session.SessionStatusCompleted
		if ms.failed {
			status = session.SessionStatusError
		}
		if err := m.store.UpdateSessionStatus(ctx, ms.pgSessionID, session.SessionStatusUpdate{
			SetStatus:  status,
			SetEndedAt: time.Now(),
		}); err != nil {
			m.log.Error(err, "failed to complete arena session",
				"runID", key, "pgSessionID", ms.pgSessionID)
		}
		return true
	})
}

// isRunFailedEvent checks if the event indicates a failed arena run.
func isRunFailedEvent(event *events.Event) bool {
	return event.Type == events.EventType("arena.run.failed") ||
		event.Type == events.EventType("arena.turn.failed")
}
