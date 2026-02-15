/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package facade

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/altairalabs/omnia/ee/pkg/evals"
	"github.com/altairalabs/omnia/internal/session"
	sessionapi "github.com/altairalabs/omnia/internal/session/api"
)

// Eval trigger types matching PromptPack definitions.
const (
	evalTriggerPerTurn           = "per_turn"
	evalTriggerOnSessionComplete = "on_session_complete"
	evalSourceInProc             = "in_proc"
)

// EvalResultWriter writes eval results to session-api.
type EvalResultWriter interface {
	WriteEvalResults(ctx context.Context, results []EvalResultInput) error
}

// EvalResultInput represents a single eval result to be written.
type EvalResultInput struct {
	SessionID         string          `json:"sessionId"`
	MessageID         string          `json:"messageId,omitempty"`
	AgentName         string          `json:"agentName"`
	Namespace         string          `json:"namespace"`
	PromptPackName    string          `json:"promptpackName"`
	PromptPackVersion string          `json:"promptpackVersion,omitempty"`
	EvalID            string          `json:"evalId"`
	EvalType          string          `json:"evalType"`
	Trigger           string          `json:"trigger"`
	Passed            bool            `json:"passed"`
	Score             *float64        `json:"score,omitempty"`
	Details           json.RawMessage `json:"details,omitempty"`
	DurationMs        int             `json:"durationMs"`
	Source            string          `json:"source"`
}

// EvalListenerConfig holds configuration for the EvalListener.
type EvalListenerConfig struct {
	AgentName    string
	Namespace    string
	PackName     string
	PackVersion  string
	Enabled      bool
	SamplingRate int32 // 0-100, default 100
	LLMJudgeRate int32 // 0-100, default 10
}

// EvalLoader defines the interface for loading eval definitions.
// This decouples EvalListener from the concrete PromptPackLoader.
type EvalLoader interface {
	LoadEvals(ctx context.Context, namespace, packName, packVersion string) (*evals.PromptPackEvals, error)
	ResolveEvals(packEvals *evals.PromptPackEvals, trigger string) []evals.EvalDef
}

// SessionMessageFetcher fetches session messages for eval context.
type SessionMessageFetcher interface {
	GetMessages(ctx context.Context, sessionID string) ([]session.Message, error)
}

// EvalListener listens to EventBridge events and runs evals in-process.
// This is the Pattern C eval path for PromptKit agents.
type EvalListener struct {
	evalLoader        EvalLoader
	messageFetcher    SessionMessageFetcher
	resultWriter      EvalResultWriter
	config            EvalListenerConfig
	logger            *slog.Logger
	completionTracker *evals.CompletionTracker
}

// recordingMessageData is the subset of recording.message event data we parse.
type recordingMessageData struct {
	Role      string `json:"role"`
	MessageID string `json:"messageId"`
}

// NewEvalListener creates a new listener.
func NewEvalListener(
	config EvalListenerConfig,
	loader EvalLoader,
	messageFetcher SessionMessageFetcher,
	resultWriter EvalResultWriter,
	logger *slog.Logger,
) *EvalListener {
	l := &EvalListener{
		evalLoader:     loader,
		messageFetcher: messageFetcher,
		resultWriter:   resultWriter,
		config:         config,
		logger:         logger.With("component", "eval-listener"),
	}

	l.completionTracker = evals.NewCompletionTracker(
		evals.DefaultInactivityTimeout,
		l.onCompletionDetected,
		l.logger,
	)

	return l
}

// StartCompletionTracker starts the periodic inactivity check. It blocks
// until the context is cancelled. Call this in a goroutine.
func (l *EvalListener) StartCompletionTracker(ctx context.Context) {
	l.completionTracker.StartPeriodicCheck(ctx, 30*time.Second)
}

// onCompletionDetected is the CompletionTracker callback for inactivity-based
// session completion detection. It delegates to OnSessionComplete.
func (l *EvalListener) onCompletionDetected(ctx context.Context, sessionID string) error {
	defer l.completionTracker.Cleanup(sessionID)
	return l.OnSessionComplete(ctx, sessionID)
}

// OnEvent is called by the EventBridge when an event occurs.
// It checks if the event should trigger evals and runs them.
func (l *EvalListener) OnEvent(ctx context.Context, event EventBusEvent) error {
	if !l.config.Enabled {
		return nil
	}

	if !l.isAssistantMessage(event) {
		return nil
	}

	l.completionTracker.RecordActivity(event.SessionID)

	if !l.shouldSample(event.SessionID, evalTriggerPerTurn) {
		return nil
	}

	msgData := l.parseMessageData(event)
	return l.runEvalsForTrigger(ctx, event.SessionID, msgData.MessageID, evalTriggerPerTurn)
}

// OnSessionComplete is called when a session is detected as complete.
func (l *EvalListener) OnSessionComplete(ctx context.Context, sessionID string) error {
	if !l.config.Enabled {
		return nil
	}

	if !l.shouldSample(sessionID, evalTriggerOnSessionComplete) {
		return nil
	}

	return l.runEvalsForTrigger(ctx, sessionID, "", evalTriggerOnSessionComplete)
}

// isAssistantMessage checks if the event is a recording.message with assistant role.
func (l *EvalListener) isAssistantMessage(event EventBusEvent) bool {
	if event.Type != EventTypeRecordingMessage {
		return false
	}

	msgData := l.parseMessageData(event)
	return msgData.Role == "assistant"
}

// parseMessageData extracts role and messageId from event data.
func (l *EvalListener) parseMessageData(event EventBusEvent) recordingMessageData {
	var data recordingMessageData
	if event.Data == nil {
		return data
	}
	// Ignore parse errors; empty data means no role match.
	_ = json.Unmarshal(event.Data, &data)
	return data
}

// shouldSample determines if this event should be sampled for eval execution.
// Uses FNV hash of sessionID + trigger for deterministic, uniform sampling.
func (l *EvalListener) shouldSample(sessionID, trigger string) bool {
	rate := l.config.SamplingRate
	if rate <= 0 {
		return false
	}
	if rate >= 100 {
		return true
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(sessionID + trigger))
	return int32(h.Sum32()%100) < rate
}

// runEvalsForTrigger loads eval definitions, runs rule-based evals, and writes results.
func (l *EvalListener) runEvalsForTrigger(ctx context.Context, sessionID, messageID, trigger string) error {
	evalDefs, err := l.loadEvalDefs(ctx, trigger)
	if err != nil {
		return fmt.Errorf("failed to load eval definitions: %w", err)
	}

	if len(evalDefs) == 0 {
		return nil
	}

	messages, err := l.messageFetcher.GetMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to fetch session messages: %w", err)
	}

	results := l.executeRuleEvals(evalDefs, messages, sessionID, messageID, trigger)

	if len(results) == 0 {
		return nil
	}

	if err := l.resultWriter.WriteEvalResults(ctx, results); err != nil {
		return fmt.Errorf("failed to write eval results: %w", err)
	}

	l.logger.InfoContext(ctx, "eval results written",
		"sessionID", sessionID,
		"trigger", trigger,
		"count", len(results),
	)
	return nil
}

// loadEvalDefs loads and filters eval definitions for the given trigger.
func (l *EvalListener) loadEvalDefs(ctx context.Context, trigger string) ([]evals.EvalDef, error) {
	packEvals, err := l.evalLoader.LoadEvals(
		ctx, l.config.Namespace, l.config.PackName, l.config.PackVersion,
	)
	if err != nil {
		return nil, err
	}

	allDefs := l.evalLoader.ResolveEvals(packEvals, trigger)
	return filterRuleEvals(allDefs), nil
}

// filterRuleEvals returns only rule-based eval definitions, skipping LLM judge evals.
func filterRuleEvals(defs []evals.EvalDef) []evals.EvalDef {
	result := make([]evals.EvalDef, 0, len(defs))
	for _, d := range defs {
		if d.Type != "llm_judge" {
			result = append(result, d)
		}
	}
	return result
}

// executeRuleEvals runs rule-based evals and converts results to EvalResultInput.
func (l *EvalListener) executeRuleEvals(
	evalDefs []evals.EvalDef,
	messages []session.Message,
	sessionID, messageID, trigger string,
) []EvalResultInput {
	results := make([]EvalResultInput, 0, len(evalDefs))

	for _, def := range evalDefs {
		result, err := l.runSingleEval(def, messages, sessionID, messageID, trigger)
		if err != nil {
			l.logger.Warn("eval execution failed",
				"evalID", def.ID,
				"evalType", def.Type,
				"error", err,
			)
			continue
		}
		results = append(results, result)
	}

	return results
}

// runSingleEval executes a single rule-based eval and returns the result.
func (l *EvalListener) runSingleEval(
	def evals.EvalDef,
	messages []session.Message,
	sessionID, messageID, trigger string,
) (EvalResultInput, error) {
	apiDef := toAPIEvalDefinition(def)

	start := time.Now()
	item, err := sessionapi.RunRuleEval(apiDef, messages)
	if err != nil {
		return EvalResultInput{}, err
	}
	durationMs := int(time.Since(start).Milliseconds())

	return EvalResultInput{
		SessionID:         sessionID,
		MessageID:         messageID,
		AgentName:         l.config.AgentName,
		Namespace:         l.config.Namespace,
		PromptPackName:    l.config.PackName,
		PromptPackVersion: l.config.PackVersion,
		EvalID:            def.ID,
		EvalType:          def.Type,
		Trigger:           trigger,
		Passed:            item.Passed,
		Score:             item.Score,
		DurationMs:        durationMs,
		Source:            evalSourceInProc,
	}, nil
}

// toAPIEvalDefinition converts an evals.EvalDef to a sessionapi.EvalDefinition.
func toAPIEvalDefinition(def evals.EvalDef) sessionapi.EvalDefinition {
	return sessionapi.EvalDefinition{
		ID:      def.ID,
		Type:    def.Type,
		Trigger: def.Trigger,
		Params:  def.Params,
	}
}
