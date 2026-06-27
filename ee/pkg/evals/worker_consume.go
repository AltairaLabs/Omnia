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
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// repeatedGt returns a slice of n ">" strings for XREADGROUP multi-stream args.
func repeatedGt(n int) []string {
	gt := make([]string, n)
	for i := range gt {
		gt[i] = ">"
	}
	return gt
}

// StreamKeys returns the stream keys this worker is subscribed to. Exported for testing.
func (w *EvalWorker) StreamKeys() []string {
	return w.streamKeys
}

// ConsumerGroup returns the consumer group name. Exported for testing.
func (w *EvalWorker) ConsumerGroup() string {
	return w.consumerGroup
}

// Namespaces returns the namespaces this worker watches. Exported for testing.
func (w *EvalWorker) Namespaces() []string {
	return w.namespaces
}

// Start begins consuming events from Redis Streams. It blocks until
// the context is cancelled or an unrecoverable error occurs.
func (w *EvalWorker) Start(ctx context.Context) error {
	for _, key := range w.streamKeys {
		if err := w.ensureConsumerGroup(ctx, key); err != nil {
			return fmt.Errorf("ensure consumer group on %s: %w", key, err)
		}
	}

	w.logger.Info("worker started",
		"streams", strings.Join(w.streamKeys, ","),
		"namespaces", strings.Join(w.namespaces, ","),
		"consumerGroup", w.consumerGroup,
		"consumer", w.consumerName,
	)

	go w.completionTracker.StartPeriodicCheck(ctx, periodicCheckInterval)

	return w.consumeLoop(ctx)
}

// ensureConsumerGroup creates the consumer group if it does not already exist.
func (w *EvalWorker) ensureConsumerGroup(ctx context.Context, streamKey string) error {
	err := w.redisClient.XGroupCreateMkStream(ctx, streamKey, w.consumerGroup, "0").Err()
	if err != nil && !isConsumerGroupExistsError(err) {
		return fmt.Errorf("XGroupCreateMkStream: %w", err)
	}
	return nil
}

// consumeLoop reads events from streams in a loop until the context is done.
func (w *EvalWorker) consumeLoop(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}

		streams, err := w.readFromStreams(ctx)
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

		w.processStreams(ctx, streams)
		w.reportStreamLag(ctx)
		w.reclaimPending(ctx)
	}
}

// reclaimPending periodically reclaims stale pending entries from other consumers
// and re-processes them via the normal message handling path.
func (w *EvalWorker) reclaimPending(ctx context.Context) {
	if !w.lastPendingReclaim.IsZero() && time.Since(w.lastPendingReclaim) < pendingReclaimInterval {
		return
	}
	w.lastPendingReclaim = time.Now()

	for _, key := range w.streamKeys {
		msgs, _, err := w.redisClient.XAutoClaim(ctx, &goredis.XAutoClaimArgs{
			Stream:   key,
			Group:    w.consumerGroup,
			Consumer: w.consumerName,
			MinIdle:  pendingMinIdle,
			Start:    "0-0",
			Count:    pendingReclaimBatchSize,
		}).Result()
		if err != nil {
			if !errors.Is(err, goredis.Nil) {
				w.logger.Debug("XAUTOCLAIM failed", "stream", key, "error", err)
			}
			continue
		}

		for _, msg := range msgs {
			w.handleMessage(ctx, key, msg)
		}
	}
}

// readFromStreams performs the XREADGROUP call across all stream keys.
func (w *EvalWorker) readFromStreams(ctx context.Context) ([]goredis.XStream, error) {
	streamArgs := append(w.streamKeys, repeatedGt(len(w.streamKeys))...)
	return w.redisClient.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerName,
		Streams:  streamArgs,
		Count:    streamReadBatchSize,
		Block:    blockTimeout,
	}).Result()
}

// reportStreamLag queries XPENDING for each stream to report consumer lag.
func (w *EvalWorker) reportStreamLag(ctx context.Context) {
	for _, key := range w.streamKeys {
		pending, err := w.redisClient.XPending(ctx, key, w.consumerGroup).Result()
		if err != nil {
			w.logger.Debug("failed to get stream pending count", "stream", key, "error", err)
			continue
		}
		w.getMetrics().SetStreamLag(key, float64(pending.Count))
	}
}

// processStreams iterates over stream results and processes each message.
// The stream key for ACK is taken from each XStream entry.
func (w *EvalWorker) processStreams(ctx context.Context, streams []goredis.XStream) {
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			w.handleMessage(ctx, stream.Stream, msg)
		}
	}
}

// handleMessage processes a single Redis stream message and ACKs it on success.
func (w *EvalWorker) handleMessage(ctx context.Context, streamKey string, msg goredis.XMessage) {
	start := time.Now()

	event, err := parseEvent(msg)
	if err != nil {
		w.logger.Warn("failed to parse event, skipping", "messageID", msg.ID, "error", err)
		w.getMetrics().RecordEventReceived("parse_error")
		w.ackMessage(ctx, streamKey, msg.ID)
		return
	}

	// Restore trace context from the event's traceparent so spans are nested
	// under the originating session trace.
	ctx = restoreTraceContext(ctx, event)

	w.getMetrics().RecordEventReceived(event.EventType)

	if err := w.processEvent(ctx, event); err != nil {
		w.logger.Error("failed to process event",
			"messageID", msg.ID,
			"sessionID", event.SessionID,
			"error", err,
		)
		w.getMetrics().RecordEventProcessing(event.EventType, time.Since(start).Seconds())
		// Don't ACK — Redis will redeliver on next read.
		return
	}

	w.getMetrics().RecordEventProcessing(event.EventType, time.Since(start).Seconds())
	w.ackMessage(ctx, streamKey, msg.ID)
}

// getTracker returns the completion tracker, initializing a no-op one if needed.
// This ensures backward compatibility with tests that construct EvalWorker directly.
func (w *EvalWorker) getTracker() *CompletionTracker {
	if w.completionTracker == nil {
		w.completionTracker = NewCompletionTracker(DefaultInactivityTimeout, nil, w.logger)
	}
	return w.completionTracker
}

// processEvent handles a single session event.
func (w *EvalWorker) processEvent(ctx context.Context, event api.SessionEvent) error {
	ctx, span := otel.Tracer("omnia-eval-worker").Start(ctx, "eval.process-event",
		trace.WithAttributes(
			attribute.String("session.id", event.SessionID),
			attribute.String("event.type", event.EventType),
		),
	)
	defer span.End()

	if isSessionCompletedEvent(event) {
		w.getTracker().MarkCompleted(ctx, event.SessionID)
		return nil
	}

	if isEvaluateEvent(event) {
		return w.processEvaluateRequest(ctx, event)
	}

	if isAssistantMessageEvent(event) {
		w.getTracker().RecordActivity(event.SessionID)
		return w.processAssistantMessage(ctx, event)
	}

	w.logger.Debug("skipping unhandled event",
		"eventType", event.EventType,
		"messageRole", event.MessageRole,
	)
	return nil
}

// isEvaluateEvent returns true if the event is a manual eval trigger.
func isEvaluateEvent(event api.SessionEvent) bool {
	return event.EventType == eventTypeEvaluate
}

// isAssistantMessageEvent returns true if the event is for an assistant message.
func isAssistantMessageEvent(event api.SessionEvent) bool {
	return event.EventType == eventTypeMessage && event.MessageRole == string(session.RoleAssistant)
}

// isSessionCompletedEvent returns true if the event signals session completion.
func isSessionCompletedEvent(event api.SessionEvent) bool {
	return event.EventType == eventTypeSessionDone
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

// restoreTraceContext extracts the W3C traceparent from the event and sets it as
// the remote span context on the returned context. If no valid traceparent is
// present, the context is returned unchanged.
func restoreTraceContext(ctx context.Context, event api.SessionEvent) context.Context {
	sc := api.ParseTraceparent(event.Traceparent)
	if !sc.IsValid() {
		return ctx
	}
	return trace.ContextWithRemoteSpanContext(ctx, sc)
}
