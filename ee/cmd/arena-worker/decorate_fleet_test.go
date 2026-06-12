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
)

const testFleetAgentID = "agent-rag-hero"

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

func TestArenaSessionTagsAndState(t *testing.T) {
	meta := arenaSessionMetadata{
		JobName:    "rag-hero-loadtest",
		Namespace:  testNamespace,
		Scenario:   "incident-runbook",
		ProviderID: testFleetAgentID,
		JobType:    "loadtest",
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
	assert.Equal(t, "rag-hero-loadtest", state["arena.job"])
	assert.Equal(t, "incident-runbook", state["arena.scenario"])
	assert.Equal(t, testFleetAgentID, state["arena.provider"])
	assert.Equal(t, "loadtest", state["arena.type"])
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
		JobName:    "rag-hero-loadtest",
		Namespace:  testNamespace,
		Scenario:   "incident-runbook",
		ProviderID: testFleetAgentID,
		JobType:    "loadtest",
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
		assert.Equal(t, "rag-hero-loadtest", opts.MergeState["arena.job"])
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
