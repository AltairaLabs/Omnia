/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"encoding/json"
	"fmt"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/providers"

	"github.com/altairalabs/omnia/internal/session/api"
)

// evalLabelsFor assembles eval metric labels for a session. variant is the
// rollout variant that served the session (carried on the session record), so a
// candidate's evals are tagged "candidate" and gateable by RolloutAnalysis.
func evalLabelsFor(agentName, namespace, packName, variant string, groups []string) EvalLabels {
	return EvalLabels{
		Agent:          agentName,
		Namespace:      namespace,
		PromptPackName: packName,
		Variant:        variant,
		Groups:         groups,
	}
}

// processAssistantMessage handles assistant message events by running per-turn evals.
func (w *EvalWorker) processAssistantMessage(ctx context.Context, event api.SessionEvent) error {
	packEvals := w.loadPackEvals(ctx, event)
	if packEvals == nil {
		w.logNoPackSkip("per_turn", event.SessionID, event.PromptPackName, event.PromptPackVersion)
		return nil
	}

	messages, err := w.getMessages(ctx, event.SessionID)
	if err != nil {
		return err
	}

	sess, err := w.getMessageStore().GetSession(ctx, event.SessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	turnIndex := countAssistantMessages(messages)
	providerSpecs := w.resolveProviders(ctx, event)
	enrichedEvent := enrichEvent(event, packEvals)

	labels := evalLabelsFor(sess.AgentName, event.Namespace, packEvals.PackName,
		sess.Variant, w.resolveWorkerGroups(ctx, event))
	items := w.getSDKRunner().RunTurnEvals(ctx, packEvals.PackData, messages,
		event.SessionID, turnIndex, providerSpecs, labels)
	w.logWorkerGroupFilteredSkip(event.SessionID, runtimeevals.TriggerEveryTurn, packEvals, labels.Groups, items)
	results := w.convertToEvalResults(items, enrichedEvent, sess.AgentName)
	return w.writeResults(ctx, results, event.SessionID)
}

// onSessionComplete is the CompletionTracker callback. It runs on_session_complete evals.
func (w *EvalWorker) onSessionComplete(ctx context.Context, sessionID string) error {
	defer w.completionTracker.Cleanup(sessionID)

	sess, err := w.getMessageStore().GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	event := api.SessionEvent{
		SessionID:         sessionID,
		AgentName:         sess.AgentName,
		Namespace:         sess.Namespace,
		PromptPackName:    sess.PromptPackName,
		PromptPackVersion: sess.PromptPackVersion,
	}

	packEvals := w.loadPackEvals(ctx, event)
	if packEvals == nil {
		w.logNoPackSkip("on_session_complete", sessionID, event.PromptPackName, event.PromptPackVersion)
		return nil
	}

	messages, err := w.getMessages(ctx, sessionID)
	if err != nil {
		return err
	}

	turnIndex := countAssistantMessages(messages)
	providerSpecs := w.resolveProviders(ctx, event)
	enrichedEvent := enrichEvent(event, packEvals)

	labels := evalLabelsFor(sess.AgentName, event.Namespace, packEvals.PackName,
		sess.Variant, w.resolveWorkerGroups(ctx, event))
	items := w.getSDKRunner().RunSessionEvals(ctx, packEvals.PackData, messages,
		sessionID, turnIndex, providerSpecs, labels)
	w.logWorkerGroupFilteredSkip(sessionID, runtimeevals.TriggerOnSessionComplete, packEvals, labels.Groups, items)
	results := w.convertToEvalResults(items, enrichedEvent, sess.AgentName)
	return w.writeResults(ctx, results, sessionID)
}

// processEvaluateRequest handles on-demand eval requests by running all evals
// (both per_turn and on_session_complete) on the full session. This is triggered
// by POST /api/v1/sessions/{id}/evaluate.
func (w *EvalWorker) processEvaluateRequest(ctx context.Context, event api.SessionEvent) error {
	packEvals := w.loadPackEvals(ctx, event)
	if packEvals == nil {
		w.logger.Info("no evals to run (no pack)", "sessionID", event.SessionID)
		return nil
	}

	messages, err := w.getMessages(ctx, event.SessionID)
	if err != nil {
		return err
	}

	sess, err := w.getMessageStore().GetSession(ctx, event.SessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	turnIndex := countAssistantMessages(messages)
	providerSpecs := w.resolveProviders(ctx, event)
	enrichedEvent := enrichEvent(event, packEvals)

	labels := evalLabelsFor(sess.AgentName, event.Namespace, packEvals.PackName, sess.Variant, nil)
	// Run all evals without tier filtering — manual trigger runs everything.
	items := w.getSDKRunner().RunSessionEvals(ctx, packEvals.PackData, messages,
		event.SessionID, turnIndex, providerSpecs, labels)
	results := w.convertToEvalResults(items, enrichedEvent, sess.AgentName)
	// Mark source as "manual" to distinguish from automatic eval worker results.
	for _, r := range results {
		r.Source = "manual"
	}
	return w.writeResults(ctx, results, event.SessionID)
}

// writeResults writes eval results if there are any.
func (w *EvalWorker) writeResults(
	ctx context.Context, results []*api.EvalResult, sessionID string,
) error {
	if len(results) == 0 {
		return nil
	}

	if err := w.getResultWriter().WriteEvalResults(ctx, results); err != nil {
		w.getMetrics().RecordResultsWritten(len(results), false)
		return fmt.Errorf("write eval results: %w", err)
	}

	w.getMetrics().RecordResultsWritten(len(results), true)
	w.logger.Info("eval results written",
		"sessionID", sessionID,
		"count", len(results),
	)

	return nil
}

// loadPackEvals loads eval definitions from the PromptPack referenced in the event.
// Returns nil if no pack loader is configured or the event has no PromptPack name.
func (w *EvalWorker) loadPackEvals(ctx context.Context, event api.SessionEvent) *CachedPack {
	if w.packLoader == nil || event.PromptPackName == "" {
		return nil
	}

	packEvals, err := w.packLoader.LoadEvals(ctx, event.Namespace, event.PromptPackName, event.PromptPackVersion)
	if err != nil {
		w.logger.Warn("failed to load PromptPack evals",
			"sessionID", event.SessionID,
			"packName", event.PromptPackName,
			"error", err,
		)
		return nil
	}

	return packEvals
}

// enrichEvent copies the event and adds PromptPack metadata for result attribution.
func enrichEvent(event api.SessionEvent, packEvals *CachedPack) api.SessionEvent {
	event.PromptPackName = packEvals.PackName
	event.PromptPackVersion = packEvals.PackVersion
	return event
}

func (w *EvalWorker) logNoPackSkip(trigger, sessionID, packName, packVersion string) {
	reason := "PromptPack evals unavailable"
	switch {
	case w.packLoader == nil:
		reason = "pack loader disabled"
	case packName == "":
		reason = "session has no PromptPack"
	}

	w.logger.Info("no evals to run",
		"sessionID", sessionID,
		"trigger", trigger,
		"packName", packName,
		"packVersion", packVersion,
		"reason", reason,
	)
}

func (w *EvalWorker) logWorkerGroupFilteredSkip(
	sessionID string,
	trigger runtimeevals.EvalTrigger,
	packEvals *CachedPack,
	groups []string,
	items []api.EvaluateResultItem,
) {
	if len(groups) == 0 || len(items) > 0 {
		return
	}

	totalForTrigger, matchedForGroups, err := countRunnableEvals(packEvals.PackData, trigger, groups)
	if err != nil || totalForTrigger == 0 || matchedForGroups > 0 {
		return
	}

	w.logger.Info("worker eval group filter matched no evals",
		"sessionID", sessionID,
		"trigger", string(trigger),
		"packName", packEvals.PackName,
		"packVersion", packEvals.PackVersion,
		"groups", groups,
		"eligibleEvalCount", totalForTrigger,
	)
}

func countRunnableEvals(
	packData []byte,
	trigger runtimeevals.EvalTrigger,
	groups []string,
) (totalForTrigger, matchedForGroups int, err error) {
	var pack packEvalDefs
	if err = json.Unmarshal(packData, &pack); err != nil {
		return 0, 0, err
	}

	for _, def := range pack.Evals {
		if def.Trigger != trigger || !def.IsEnabled() {
			continue
		}
		totalForTrigger++
		if hasGroupOverlap(def.GetGroups(), groups) {
			matchedForGroups++
		}
	}

	return totalForTrigger, matchedForGroups, nil
}

func hasGroupOverlap(evalGroups, selectedGroups []string) bool {
	for _, evalGroup := range evalGroups {
		for _, selectedGroup := range selectedGroups {
			if evalGroup == selectedGroup {
				return true
			}
		}
	}
	return false
}

// resolveProviders resolves provider specs from the AgentRuntime CRD.
// Returns nil if no resolver is configured or resolution fails (logged as warning).
func (w *EvalWorker) resolveProviders(ctx context.Context, event api.SessionEvent) map[string]providers.ProviderSpec {
	if w.providerResolver == nil || event.AgentName == "" || event.Namespace == "" {
		return nil
	}

	specs, err := w.providerResolver.ResolveProviderSpecs(ctx, event.AgentName, event.Namespace)
	if err != nil {
		w.logger.Warn("failed to resolve provider specs",
			"agentName", event.AgentName,
			"namespace", event.Namespace,
			"error", err,
		)
		return nil
	}

	return specs
}

// resolveWorkerGroups returns the eval group filter for worker-path
// execution on this agent, falling back to DefaultWorkerEvalGroups when
// the CRD does not specify one (or the resolver is not configured).
//
// When workerGroupsOverride is set on the worker struct, it takes
// precedence over both the resolver and the default. This is used by
// tests to pin the filter to match fixture eval types; production
// pods should leave it nil.
func (w *EvalWorker) resolveWorkerGroups(ctx context.Context, event api.SessionEvent) []string {
	if w.workerGroupsOverride != nil {
		return w.workerGroupsOverride
	}
	if w.providerResolver == nil || event.AgentName == "" || event.Namespace == "" {
		return DefaultWorkerEvalGroups
	}
	groups, found := w.providerResolver.ResolveWorkerGroups(ctx, event.AgentName, event.Namespace)
	if !found {
		// No AgentRuntime CR: we have no opt-in signal to apply a restrictive
		// default. Return nil (no filter) so the SDK runs every eval in the
		// pack rather than silently dropping events for agents the worker
		// knows nothing about. Matches the shape of orphaned events from
		// decommissioned agents as well.
		return nil
	}
	if len(groups) == 0 {
		return DefaultWorkerEvalGroups
	}
	return groups
}
