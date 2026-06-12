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
	"github.com/altairalabs/omnia/internal/session"
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
		sid := resolveResultSessionID(context.Background(), logr.Discard(), &Config{}, fleetSessionInputs{
			loadTestFleet: false,
			meta:          meta,
			registry:      pkproviders.NewRegistry(),
		})
		assert.Empty(t, sid)
	})

	t.Run("fleet path with no facade session returns empty", func(t *testing.T) {
		// loadTestFleet=true + session-api set, but no fleet session IDs to decorate
		// → no network call, returns empty.
		sid := resolveResultSessionID(context.Background(), logr.Discard(),
			&Config{SessionAPIURL: "http://session-api"}, fleetSessionInputs{
				loadTestFleet: true,
				meta:          meta,
				registry:      pkproviders.NewRegistry(),
			})
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

func TestFinalizeFleetSession(t *testing.T) {
	meta := arenaSessionMetadata{
		JobName:    testLoadJobName,
		Namespace:  testNamespace,
		Scenario:   "incident-runbook",
		ProviderID: testFleetAgentID,
		JobType:    testLoadJobType,
	}

	connectedFleet := func() fleetSessionInputs {
		prov := fleet.NewProvider(testFleetAgentID, "ws://agent", &fakeDialer{sessionID: "facade-sess-1"})
		require.NoError(t, prov.Connect(context.Background()))
		registry := pkproviders.NewRegistry()
		registry.Register(prov)
		return fleetSessionInputs{
			loadTestFleet: true,
			meta:          meta,
			personaIDs:    []string{"sre-user"},
			selfPlayCalls: []session.ProviderCall{{
				Source: sourceSelfPlay, Provider: testProvOllama, Model: testModelLlama,
				Status: session.ProviderCallStatusCompleted, InputTokens: 322, OutputTokens: 42,
			}},
			registry: registry,
			fleet:    []*resolvedFleetProvider{{id: testFleetAgentID, agent: "rag-hero"}},
		}
	}

	t.Run("decorates, labels persona, and records self-play calls", func(t *testing.T) {
		store := newMockStore()
		in := connectedFleet()

		primary := finalizeFleetSession(context.Background(), logr.Discard(), store, in)

		assert.Equal(t, "facade-sess-1", primary)
		opts := store.decorations["facade-sess-1"]
		assert.Contains(t, opts.AddTags, "source:arena")
		assert.Contains(t, opts.AddTags, "persona:sre-user")
		assert.Contains(t, opts.RemoveTags, "source:interactive")
		assert.Equal(t, "sre-user", opts.MergeState["arena.persona"])
		assert.Equal(t, "llama3.2:3b", opts.MergeState["arena.selfplay.model"])
		assert.Equal(t, "ollama", opts.MergeState["arena.selfplay.provider"])

		// The self-play provider call is attached to the facade session.
		calls := store.providerCalls["facade-sess-1"]
		require.Len(t, calls, 1)
		assert.Equal(t, sourceSelfPlay, calls[0].Source)
		assert.Equal(t, "llama3.2:3b", calls[0].Model)
		assert.Equal(t, "facade-sess-1", calls[0].SessionID)
		assert.Equal(t, testNamespace, calls[0].Namespace)
		assert.Equal(t, "rag-hero", calls[0].AgentName)
	})

	t.Run("returns empty when no facade session is available", func(t *testing.T) {
		prov := fleet.NewProvider(testFleetAgentID, "ws://agent", &fakeDialer{sessionID: "unused"})
		registry := pkproviders.NewRegistry()
		registry.Register(prov) // never connected → no session IDs
		store := newMockStore()

		primary := finalizeFleetSession(context.Background(), logr.Discard(), store, fleetSessionInputs{
			meta: meta, registry: registry, fleet: []*resolvedFleetProvider{{id: testFleetAgentID}},
		})

		assert.Empty(t, primary)
		assert.Empty(t, store.decorations)
		assert.Empty(t, store.providerCalls)
	})

	t.Run("returns empty when the provider is not registered", func(t *testing.T) {
		store := newMockStore()
		primary := finalizeFleetSession(context.Background(), logr.Discard(), store, fleetSessionInputs{
			meta: meta, registry: pkproviders.NewRegistry(),
			fleet: []*resolvedFleetProvider{{id: "missing-provider"}},
		})
		assert.Empty(t, primary)
		assert.Empty(t, store.decorations)
	})

	t.Run("returns empty when decoration fails", func(t *testing.T) {
		store := newMockStore()
		store.decorateErr = assert.AnError
		in := connectedFleet()

		primary := finalizeFleetSession(context.Background(), logr.Discard(), store, in)

		assert.Empty(t, primary)
		// No self-play calls recorded when the session couldn't be labelled.
		assert.Empty(t, store.providerCalls)
	})
}
