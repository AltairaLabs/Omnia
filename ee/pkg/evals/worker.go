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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// Constants for Redis consumer group and stream configuration.
const (
	consumerGroupPrefix = "omnia-eval-workers-"
	blockTimeout        = 5 * time.Second
	evalSource          = "worker"
	triggerPerTurn      = "per_turn"
	eventTypeMessage    = "message.assistant"
	streamPayloadField  = "payload"
)

// EvalRunner executes a rule-based eval against session messages.
// This interface wraps api.RunRuleEval to allow testing.
type EvalRunner func(evalDef api.EvalDefinition, messages []session.Message) (api.EvaluateResultItem, error)

// WorkerConfig holds the configuration for an EvalWorker.
type WorkerConfig struct {
	RedisClient goredis.UniversalClient
	SessionAPI  SessionAPIClient
	Namespace   string
	Logger      *slog.Logger
	// EvalRunner overrides the default eval runner (api.RunRuleEval).
	// If nil, api.RunRuleEval is used.
	EvalRunner EvalRunner
}

// EvalWorker consumes session events from Redis Streams and runs evals.
type EvalWorker struct {
	redisClient   goredis.UniversalClient
	sessionAPI    SessionAPIClient
	namespace     string
	consumerGroup string
	consumerName  string
	logger        *slog.Logger
	evalRunner    EvalRunner
}

// NewEvalWorker creates a new eval worker for the given namespace.
func NewEvalWorker(config WorkerConfig) *EvalWorker {
	runner := config.EvalRunner
	if runner == nil {
		runner = api.RunRuleEval
	}

	return &EvalWorker{
		redisClient:   config.RedisClient,
		sessionAPI:    config.SessionAPI,
		namespace:     config.Namespace,
		consumerGroup: consumerGroupPrefix + config.Namespace,
		consumerName:  hostname(),
		logger:        config.Logger,
		evalRunner:    runner,
	}
}

// Start begins consuming events from the Redis Stream. It blocks until
// the context is cancelled or an unrecoverable error occurs.
func (w *EvalWorker) Start(ctx context.Context) error {
	streamKey := api.StreamKey(w.namespace)

	if err := w.ensureConsumerGroup(ctx, streamKey); err != nil {
		return fmt.Errorf("ensure consumer group: %w", err)
	}

	w.logger.Info("worker started",
		"stream", streamKey,
		"consumerGroup", w.consumerGroup,
		"consumer", w.consumerName,
	)

	return w.consumeLoop(ctx, streamKey)
}

// ensureConsumerGroup creates the consumer group if it does not already exist.
func (w *EvalWorker) ensureConsumerGroup(ctx context.Context, streamKey string) error {
	err := w.redisClient.XGroupCreateMkStream(ctx, streamKey, w.consumerGroup, "0").Err()
	if err != nil && !isConsumerGroupExistsError(err) {
		return fmt.Errorf("XGroupCreateMkStream: %w", err)
	}
	return nil
}

// consumeLoop reads events from the stream in a loop until the context is done.
func (w *EvalWorker) consumeLoop(ctx context.Context, streamKey string) error {
	for {
		if ctx.Err() != nil {
			return nil
		}

		streams, err := w.readFromStream(ctx, streamKey)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			if errors.Is(err, goredis.Nil) {
				continue
			}
			w.logger.Error("XReadGroup failed", "error", err)
			continue
		}

		w.processStreams(ctx, streamKey, streams)
	}
}

// readFromStream performs the XREADGROUP call.
func (w *EvalWorker) readFromStream(ctx context.Context, streamKey string) ([]goredis.XStream, error) {
	return w.redisClient.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerName,
		Streams:  []string{streamKey, ">"},
		Count:    1,
		Block:    blockTimeout,
	}).Result()
}

// processStreams iterates over stream results and processes each message.
func (w *EvalWorker) processStreams(ctx context.Context, streamKey string, streams []goredis.XStream) {
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			w.handleMessage(ctx, streamKey, msg)
		}
	}
}

// handleMessage processes a single Redis stream message and ACKs it on success.
func (w *EvalWorker) handleMessage(ctx context.Context, streamKey string, msg goredis.XMessage) {
	event, err := parseEvent(msg)
	if err != nil {
		w.logger.Warn("failed to parse event, skipping", "messageID", msg.ID, "error", err)
		w.ackMessage(ctx, streamKey, msg.ID)
		return
	}

	if err := w.processEvent(ctx, event); err != nil {
		w.logger.Error("failed to process event",
			"messageID", msg.ID,
			"sessionID", event.SessionID,
			"error", err,
		)
		// Don't ACK — Redis will redeliver on next read.
		return
	}

	w.ackMessage(ctx, streamKey, msg.ID)
}

// processEvent handles a single session event.
func (w *EvalWorker) processEvent(ctx context.Context, event api.SessionEvent) error {
	if !isAssistantMessageEvent(event) {
		w.logger.Debug("skipping non-assistant event",
			"eventType", event.EventType,
			"messageRole", event.MessageRole,
		)
		return nil
	}

	sess, err := w.sessionAPI.GetSession(ctx, event.SessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	messages, err := w.sessionAPI.GetSessionMessages(ctx, event.SessionID)
	if err != nil {
		return fmt.Errorf("get session messages: %w", err)
	}

	// Build per_turn rule eval definitions from messages.
	// In this initial implementation without K8s API access, we don't have
	// PromptPack definitions. This will be wired in #473 (operator wiring).
	// For now, the worker correctly processes events but has no evals to run.
	evalDefs := filterPerTurnRuleEvals(nil)
	if len(evalDefs) == 0 {
		w.logger.Debug("no per_turn rule evals to run", "sessionID", event.SessionID)
		return nil
	}

	results := w.runEvals(evalDefs, messages, event, sess.AgentName)
	if len(results) == 0 {
		return nil
	}

	if err := w.sessionAPI.WriteEvalResults(ctx, results); err != nil {
		return fmt.Errorf("write eval results: %w", err)
	}

	w.logger.Info("eval results written",
		"sessionID", event.SessionID,
		"count", len(results),
	)

	return nil
}

// runEvals executes the given eval definitions against session messages.
func (w *EvalWorker) runEvals(
	evalDefs []api.EvalDefinition,
	messages []session.Message,
	event api.SessionEvent,
	agentName string,
) []*api.EvalResult {
	results := make([]*api.EvalResult, 0, len(evalDefs))

	for _, def := range evalDefs {
		item, err := w.evalRunner(def, messages)
		if err != nil {
			w.logger.Warn("eval failed",
				"evalID", def.ID,
				"sessionID", event.SessionID,
				"error", err,
			)
			continue
		}

		item.Source = evalSource
		result := toEvalResult(item, event, agentName)
		results = append(results, result)
	}

	return results
}

// toEvalResult converts an EvaluateResultItem to an EvalResult for persistence.
func toEvalResult(item api.EvaluateResultItem, event api.SessionEvent, agentName string) *api.EvalResult {
	result := &api.EvalResult{
		SessionID: event.SessionID,
		MessageID: event.MessageID,
		AgentName: agentName,
		Namespace: event.Namespace,
		EvalID:    item.EvalID,
		EvalType:  item.EvalType,
		Trigger:   item.Trigger,
		Passed:    item.Passed,
		Score:     item.Score,
		Source:    evalSource,
		CreatedAt: time.Now(),
	}

	if item.DurationMs > 0 {
		d := item.DurationMs
		result.DurationMs = &d
	}

	return result
}

// filterPerTurnRuleEvals filters eval definitions to only per_turn rule evals.
func filterPerTurnRuleEvals(defs []EvalDef) []api.EvalDefinition {
	var result []api.EvalDefinition
	for _, d := range defs {
		if d.Trigger == triggerPerTurn && d.Type == "rule" {
			result = append(result, api.EvalDefinition{
				ID:      d.ID,
				Type:    d.Type,
				Trigger: d.Trigger,
				Params:  d.Params,
			})
		}
	}
	return result
}

// isAssistantMessageEvent returns true if the event is for an assistant message.
func isAssistantMessageEvent(event api.SessionEvent) bool {
	return event.EventType == eventTypeMessage && event.MessageRole == "assistant"
}

// parseEvent extracts a SessionEvent from a Redis stream message.
func parseEvent(msg goredis.XMessage) (api.SessionEvent, error) {
	payload, ok := msg.Values[streamPayloadField]
	if !ok {
		return api.SessionEvent{}, fmt.Errorf("missing %q field", streamPayloadField)
	}

	payloadStr, ok := payload.(string)
	if !ok {
		return api.SessionEvent{}, fmt.Errorf("payload is not a string")
	}

	var event api.SessionEvent
	if err := json.Unmarshal([]byte(payloadStr), &event); err != nil {
		return api.SessionEvent{}, fmt.Errorf("unmarshal event: %w", err)
	}

	return event, nil
}

// ackMessage acknowledges a processed message in the consumer group.
func (w *EvalWorker) ackMessage(ctx context.Context, streamKey, messageID string) {
	if err := w.redisClient.XAck(ctx, streamKey, w.consumerGroup, messageID).Err(); err != nil {
		w.logger.Error("failed to ACK message", "messageID", messageID, "error", err)
	}
}

// isConsumerGroupExistsError checks if the error indicates the group already exists.
func isConsumerGroupExistsError(err error) bool {
	return err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists"
}

// hostname returns the hostname or a fallback value.
func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
