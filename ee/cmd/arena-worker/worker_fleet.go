/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AltairaLabs/PromptKit/pkg/config"
	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	"github.com/go-logr/logr"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/arena/fleet"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/pkg/session/httpclient"
)

// newArenaSessionMeta builds the arena context for a work item, shared by the
// session manager (direct providers) and the facade-session decoration path.
func newArenaSessionMeta(cfg *Config, item *queue.WorkItem) arenaSessionMetadata {
	return arenaSessionMetadata{
		JobName:       cfg.JobName,
		Namespace:     cfg.JobNamespace,
		WorkspaceName: cfg.WorkspaceName,
		Scenario:      item.ScenarioID,
		ProviderID:    item.ProviderID,
		JobType:       cfg.JobType,
		TrialIndex:    extractTrialIndex(item),
	}
}

// isLoadTestFleet reports whether this run is a load test against a live agent
// (fleet provider). Such runs are recorded by the agent's facade, so the worker
// links/labels that session rather than creating its own.
func isLoadTestFleet(cfg *Config, fleetProviders []*resolvedFleetProvider) bool {
	return cfg.JobType == string(omniav1alpha1.ArenaJobTypeLoadTest) && len(fleetProviders) > 0
}

// loadedPersonaIDs returns the self-play persona IDs from the loaded arena config,
// sorted for deterministic labelling. Empty when self-play is not configured.
func loadedPersonaIDs(cfg *config.Config) []string {
	if cfg == nil || len(cfg.LoadedPersonas) == 0 {
		return nil
	}
	ids := make([]string, 0, len(cfg.LoadedPersonas))
	for id := range cfg.LoadedPersonas {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// collectedSelfPlayCalls returns the buffered self-play / judge provider calls,
// or nil when no collector was wired (non-fleet runs).
func collectedSelfPlayCalls(c *selfPlayCollector) []session.ProviderCall {
	if c == nil {
		return nil
	}
	return c.collected()
}

// resolveResultSessionID returns the session ID used to correlate the job result
// to a session. For load-test fleet runs it finalizes the facade-recorded
// session(s) — labelling them with arena context and attaching the self-play /
// judge provider calls — and returns the primary one; otherwise it returns the
// session created by the arena session manager (or fleet fallback).
func resolveResultSessionID(ctx context.Context, log logr.Logger, cfg *Config, in fleetSessionInputs) string {
	if in.loadTestFleet && cfg.SessionAPIURL != "" {
		store := httpclient.NewStore(cfg.SessionAPIURL, log)
		return finalizeFleetSession(ctx, log, store, in)
	}
	return extractSessionID(in.sessionMgr, in.registry, in.fleet)
}

// finalizeFleetSession labels the facade-recorded session(s) of a load-test fleet
// run with arena context (tags + state, including persona) and attaches the
// self-play / judge provider calls to the primary one. In fleet mode the facade
// owns the conversation + agent cost; this adds the parts that ran inside the
// worker's engine (the user simulator and judges). Returns the primary session ID,
// or "" if no facade session could be resolved.
func finalizeFleetSession(ctx context.Context, log logr.Logger, store session.Store, in fleetSessionInputs) string {
	ids := collectFleetSessionIDs(in.registry, in.fleet)
	if len(ids) == 0 {
		return ""
	}

	opts := session.DecorateSessionOptions{
		// The fleet client connects to the facade like an interactive client, so the
		// facade tags the session "source:interactive". Drop it so the arena session
		// isn't double-counted as user traffic — it's "source:arena" now.
		RemoveTags: []string{sourceInteractiveTag},
		AddTags:    fleetSessionTags(in.meta, in.personaIDs),
		MergeState: fleetSessionState(in.meta, in.personaIDs, in.selfPlayCalls),
	}

	var primary string
	for _, sid := range ids {
		if err := store.DecorateSession(ctx, sid, opts); err != nil {
			// The facade may not have persisted the session yet, or it lives in a
			// different workspace's session-api. Log and continue — the run result
			// is still valid, it just won't be labeled as arena.
			log.Error(err, "failed to decorate facade session with arena context", "sessionID", sid)
			continue
		}
		if primary == "" {
			primary = sid
		}
	}
	if primary == "" {
		return ""
	}

	recordSelfPlayCalls(ctx, log, store, primary, in)
	return primary
}

// recordSelfPlayCalls attaches the captured self-play / judge provider calls to
// the primary facade session, denormalizing namespace + agent for attribution.
func recordSelfPlayCalls(
	ctx context.Context, log logr.Logger, store session.Store, sessionID string, in fleetSessionInputs,
) {
	agent := fleetAgentName(in.fleet)
	for _, pc := range in.selfPlayCalls {
		pc.SessionID = sessionID
		pc.Namespace = in.meta.Namespace
		pc.AgentName = agent
		if err := store.RecordProviderCall(ctx, sessionID, pc); err != nil {
			log.Error(err, "failed to record self-play provider call",
				"sessionID", sessionID, "provider", pc.Provider, "source", pc.Source)
		}
	}
}

// fleetSessionTags is arenaSessionTags plus a persona:<id> tag per persona.
func fleetSessionTags(meta arenaSessionMetadata, personaIDs []string) []string {
	tags := arenaSessionTags(meta)
	for _, p := range personaIDs {
		tags = append(tags, "persona:"+p)
	}
	return tags
}

// fleetSessionState is arenaSessionState plus persona and self-play model context.
func fleetSessionState(meta arenaSessionMetadata, personaIDs []string, calls []session.ProviderCall) map[string]string {
	state := arenaSessionState(meta, "")
	if len(personaIDs) > 0 {
		state["arena.persona"] = strings.Join(personaIDs, ",")
	}
	if model, provider := selfPlaySummary(calls); model != "" {
		state["arena.selfplay.model"] = model
		state["arena.selfplay.provider"] = provider
	}
	return state
}

// fleetAgentName returns the AgentRuntime name of the first fleet provider, used
// to attribute self-play provider calls to the same agent as the facade session.
func fleetAgentName(fleetProviders []*resolvedFleetProvider) string {
	for _, fp := range fleetProviders {
		if fp.agent != "" {
			return fp.agent
		}
	}
	return ""
}

// collectFleetSessionIDs returns the de-duplicated facade session IDs across all
// fleet providers for a work item. Per-conversation connections are preferred;
// the fallback connection covers the single-connection case.
func collectFleetSessionIDs(
	registry *pkproviders.Registry,
	fleetProviders []*resolvedFleetProvider,
) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, fp := range fleetProviders {
		prov, ok := registry.Get(fp.id)
		if !ok {
			continue
		}
		fleetProv, ok := prov.(*fleet.Provider)
		if !ok {
			continue
		}
		for _, sid := range fleetSessionIDs(fleetProv) {
			if _, dup := seen[sid]; dup {
				continue
			}
			seen[sid] = struct{}{}
			ids = append(ids, sid)
		}
	}
	return ids
}

// fleetSessionIDs returns the non-empty facade session IDs known to a single
// fleet provider (pooled per-conversation connections plus the fallback).
func fleetSessionIDs(prov *fleet.Provider) []string {
	ids := make([]string, 0, 2)
	for _, sid := range prov.ConversationSessionIDs() {
		if sid != "" {
			ids = append(ids, sid)
		}
	}
	if sid := prov.SessionID(); sid != "" {
		ids = append(ids, sid)
	}
	return ids
}

// extractFleetTTFT reads LastTTFT from fleet providers and stores the value
// in result.Metrics so that recordDetailedMetrics can emit the Prometheus histogram.
func extractFleetTTFT(
	registry *pkproviders.Registry,
	fleetProviders []*resolvedFleetProvider,
	result *ExecutionResult,
) {
	if result == nil || result.Metrics == nil {
		return
	}
	// Already set (e.g. by a non-fleet provider that natively reports TTFT).
	if _, ok := result.Metrics[metricKeyTTFT]; ok {
		return
	}
	for _, fp := range fleetProviders {
		prov, ok := registry.Get(fp.id)
		if !ok {
			continue
		}
		fleetProv, ok := prov.(*fleet.Provider)
		if !ok {
			continue
		}
		ttft := fleetProv.LastTTFT()
		if ttft > 0 {
			result.Metrics[metricKeyTTFT] = ttft.Seconds()
			return // use the first non-zero value
		}
	}
}

// extractSessionID returns the first available session ID from the session manager
// (direct providers) or fleet provider connections.
func extractSessionID(
	sessionMgr *arenaSessionManager,
	registry *pkproviders.Registry,
	fleetProviders []*resolvedFleetProvider,
) string {
	// Prefer session IDs from the session manager (direct providers with recording).
	if sessionMgr != nil {
		if ids := sessionMgr.SessionIDs(); len(ids) > 0 {
			return ids[0]
		}
	}

	// Fall back to fleet provider session IDs.
	for _, fp := range fleetProviders {
		prov, ok := registry.Get(fp.id)
		if !ok {
			continue
		}
		fleetProv, ok := prov.(*fleet.Provider)
		if !ok {
			continue
		}
		if sid := fleetProv.SessionID(); sid != "" {
			return sid
		}
		for _, sid := range fleetProv.ConversationSessionIDs() {
			if sid != "" {
				return sid
			}
		}
	}
	return ""
}

// extractTrialIndex parses the trialIndex from a work item's Config JSON.
func extractTrialIndex(item *queue.WorkItem) string {
	if len(item.Config) == 0 {
		return ""
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(item.Config, &cfg); err != nil {
		return ""
	}
	if v, ok := cfg["trialIndex"]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}
