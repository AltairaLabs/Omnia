/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/ee/pkg/arena/fleet"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
)

const (
	testFleetAgentID = "agent-rag-hero"
	testLoadJobName  = "rag-hero-loadtest"
	testLoadJobType  = "loadtest"
	testItemID       = "item-1"
)

// fakeConn is a fleet.Conn that hands back a single "connected" server message
// carrying a fixed facade session ID, then blocks/no-ops. Enough to drive
// fleet.Provider.Connect so SessionID() returns a known value.
type fakeConn struct {
	connectedJSON []byte
}

func (c *fakeConn) ReadMessage() (int, []byte, error) { return 1, c.connectedJSON, nil }
func (c *fakeConn) WriteMessage(int, []byte) error    { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error   { return nil }
func (c *fakeConn) Close() error                      { return nil }

type fakeDialer struct {
	sessionID string
}

func (d *fakeDialer) DialContext(_ context.Context, _ string, _ http.Header) (fleet.Conn, error) {
	return &fakeConn{
		connectedJSON: []byte(`{"type":"connected","session_id":"` + d.sessionID + `"}`),
	}, nil
}

func TestNewArenaSessionMeta(t *testing.T) {
	cfg := &Config{
		JobName:       testLoadJobName,
		JobNamespace:  testNamespace,
		WorkspaceName: "ws",
		JobType:       testLoadJobType,
	}
	item := &queue.WorkItem{
		ID:         testItemID,
		ScenarioID: "incident-runbook",
		ProviderID: testFleetAgentID,
	}

	meta := newArenaSessionMeta(cfg, item)
	assert.Equal(t, testLoadJobName, meta.JobName)
	assert.Equal(t, testNamespace, meta.Namespace)
	assert.Equal(t, "ws", meta.WorkspaceName)
	assert.Equal(t, "incident-runbook", meta.Scenario)
	assert.Equal(t, testFleetAgentID, meta.ProviderID)
	assert.Equal(t, testLoadJobType, meta.JobType)
}

func TestIsLoadTestFleet(t *testing.T) {
	fleetProvs := []*resolvedFleetProvider{{id: testFleetAgentID}}

	assert.True(t, isLoadTestFleet(&Config{JobType: testLoadJobType}, fleetProvs))
	assert.False(t, isLoadTestFleet(&Config{JobType: testLoadJobType}, nil), "loadtest without fleet providers")
	assert.False(t, isLoadTestFleet(&Config{JobType: "evaluation"}, fleetProvs), "evaluation against a fleet")
}

func TestResolveResultSessionID(t *testing.T) {
	meta := arenaSessionMetadata{JobName: testLoadJobName, JobType: testLoadJobType}

	t.Run("non-fleet path returns the session manager / fleet fallback ID", func(t *testing.T) {
		// loadTestFleet=false, no session manager, no providers → empty.
		sid := resolveResultSessionID(
			context.Background(), logr.Discard(), &Config{}, false, meta,
			nil, pkproviders.NewRegistry(), nil,
		)
		assert.Empty(t, sid)
	})

	t.Run("fleet path decorates without a session and returns empty", func(t *testing.T) {
		// loadTestFleet=true + session-api set, but no fleet session IDs to decorate
		// → no network call, returns empty.
		sid := resolveResultSessionID(
			context.Background(), logr.Discard(),
			&Config{SessionAPIURL: "http://session-api"}, true, meta,
			nil, pkproviders.NewRegistry(), nil,
		)
		assert.Empty(t, sid)
	})
}

func TestArenaSessionTagsAndState(t *testing.T) {
	meta := arenaSessionMetadata{
		JobName:    testLoadJobName,
		Namespace:  testNamespace,
		Scenario:   "incident-runbook",
		ProviderID: testFleetAgentID,
		JobType:    testLoadJobType,
		TrialIndex: "2",
	}

	tags := arenaSessionTags(meta)
	assert.Equal(t, []string{
		"source:arena",
		"arena-job:rag-hero-loadtest",
		"scenario:incident-runbook",
		"provider:agent-rag-hero",
		"trial:2",
	}, tags)

	state := arenaSessionState(meta, "")
	assert.Equal(t, testLoadJobName, state["arena.job"])
	assert.Equal(t, "incident-runbook", state["arena.scenario"])
	assert.Equal(t, testFleetAgentID, state["arena.provider"])
	assert.Equal(t, testLoadJobType, state["arena.type"])
	assert.Equal(t, "2", state["arena.trial.index"])
	// Empty runID is omitted.
	_, hasRunID := state["arena.run_id"]
	assert.False(t, hasRunID)

	// Without a trial index, the trial tag/state are omitted.
	meta.TrialIndex = ""
	assert.NotContains(t, arenaSessionTags(meta), "trial:")
	_, hasTrial := arenaSessionState(meta, "run-1")["arena.trial.index"]
	assert.False(t, hasTrial)
	assert.Equal(t, "run-1", arenaSessionState(meta, "run-1")["arena.run_id"])
}

func TestDecorateFleetSessions(t *testing.T) {
	meta := arenaSessionMetadata{
		JobName:    testLoadJobName,
		Namespace:  testNamespace,
		Scenario:   "incident-runbook",
		ProviderID: testFleetAgentID,
		JobType:    testLoadJobType,
	}

	t.Run("decorates the facade session and returns it as primary", func(t *testing.T) {
		prov := fleet.NewProvider(testFleetAgentID, "ws://agent", &fakeDialer{sessionID: "facade-sess-1"})
		require.NoError(t, prov.Connect(context.Background()))

		registry := pkproviders.NewRegistry()
		registry.Register(prov)
		store := newMockStore()

		primary := decorateFleetSessions(
			context.Background(), logr.Discard(), store, meta, registry,
			[]*resolvedFleetProvider{{id: testFleetAgentID}},
		)

		assert.Equal(t, "facade-sess-1", primary)
		opts, ok := store.decorations["facade-sess-1"]
		require.True(t, ok, "expected the facade session to be decorated")
		assert.Contains(t, opts.AddTags, "source:arena")
		assert.Contains(t, opts.AddTags, "arena-job:rag-hero-loadtest")
		assert.Equal(t, testLoadJobName, opts.MergeState["arena.job"])
	})

	t.Run("returns empty when no facade session is available", func(t *testing.T) {
		// Provider never connected → no fallback or pooled session IDs.
		prov := fleet.NewProvider(testFleetAgentID, "ws://agent", &fakeDialer{sessionID: "unused"})
		registry := pkproviders.NewRegistry()
		registry.Register(prov)
		store := newMockStore()

		primary := decorateFleetSessions(
			context.Background(), logr.Discard(), store, meta, registry,
			[]*resolvedFleetProvider{{id: testFleetAgentID}},
		)

		assert.Empty(t, primary)
		assert.Empty(t, store.decorations)
	})

	t.Run("returns empty when the provider is not registered", func(t *testing.T) {
		registry := pkproviders.NewRegistry()
		store := newMockStore()

		primary := decorateFleetSessions(
			context.Background(), logr.Discard(), store, meta, registry,
			[]*resolvedFleetProvider{{id: "missing-provider"}},
		)

		assert.Empty(t, primary)
		assert.Empty(t, store.decorations)
	})

	t.Run("returns empty when decoration fails", func(t *testing.T) {
		prov := fleet.NewProvider(testFleetAgentID, "ws://agent", &fakeDialer{sessionID: "facade-sess-1"})
		require.NoError(t, prov.Connect(context.Background()))

		registry := pkproviders.NewRegistry()
		registry.Register(prov)
		store := newMockStore()
		store.decorateErr = assert.AnError

		primary := decorateFleetSessions(
			context.Background(), logr.Discard(), store, meta, registry,
			[]*resolvedFleetProvider{{id: testFleetAgentID}},
		)

		assert.Empty(t, primary)
	})
}
