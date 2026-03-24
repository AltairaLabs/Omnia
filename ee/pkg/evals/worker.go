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
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"sigs.k8s.io/controller-runtime/pkg/client"

	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/AltairaLabs/PromptKit/runtime/providers"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
	redisprovider "github.com/altairalabs/omnia/internal/session/providers/redis"
)

// Constants for Redis consumer group and stream configuration.
const (
	consumerGroupPrefix   = "omnia-eval-workers-"
	blockTimeout          = 5 * time.Second
	evalSource            = "worker"
	eventTypeMessage      = "message.assistant"
	eventTypeSessionDone  = "session.completed"
	eventTypeEvaluate     = "session.evaluate"
	streamPayloadField    = "payload"
	streamReadBatchSize   = 10
	periodicCheckInterval = 30 * time.Second
)

// MessageStore provides read access to session data from the Redis hot tier.
type MessageStore interface {
	GetSession(ctx context.Context, sessionID string) (*session.Session, error)
	GetRecentMessages(ctx context.Context, sessionID string, limit int) ([]*session.Message, error)
}

// WorkerConfig holds the configuration for an EvalWorker.
type WorkerConfig struct {
	RedisClient goredis.UniversalClient
	// ResultWriter persists eval results to session-api.
	ResultWriter EvalResultWriter
	// MessageStore reads session data from the Redis hot tier.
	// If nil, the worker creates one from RedisClient with default options.
	MessageStore MessageStore
	// Namespaces is the list of namespaces to watch for eval events.
	// Each namespace gets its own Redis stream key.
	Namespaces []string
	// Namespace is deprecated; use Namespaces instead.
	// If Namespaces is empty, falls back to []string{Namespace}.
	Namespace string
	Logger    *slog.Logger
	// K8sClient is used to resolve provider specs from CRDs.
	// If nil, provider resolution is disabled and llm_judge evals will fail.
	K8sClient client.Client
	// SDKRunner overrides the default SDK-based eval runner.
	// If nil, a default SDKRunner with the full PromptKit registry is used.
	SDKRunner *SDKRunner
	// InactivityTimeout overrides the default completion inactivity timeout.
	// If zero, DefaultInactivityTimeout is used.
	InactivityTimeout time.Duration
	// RateLimiter controls eval execution throughput.
	// If nil, a default RateLimiter (50 evals/sec, 5 concurrent judges) is used.
	RateLimiter *RateLimiter
	// PackLoader loads eval definitions from PromptPack ConfigMaps.
	// If nil, no evals are loaded from PromptPacks (original behavior).
	PackLoader *PromptPackLoader
	// Metrics records Prometheus metrics for the eval worker.
	// If nil, a NoOpWorkerMetrics is used.
	Metrics WorkerMetricsRecorder
	// EvalCollector is a unified PromptKit metrics Collector that creates dynamically-named
	// per-eval Prometheus metrics (e.g., omnia_eval_helpfulness). The quality
	// dashboard discovers these via {__name__=~"omnia_eval_.*"}. If nil, one is
	// created automatically with the default Prometheus registerer.
	EvalCollector *sdkmetrics.Collector
	// TracerProvider enables OTel tracing for eval execution.
	// When set, the SDK emits per-eval spans with GenAI attributes.
	TracerProvider trace.TracerProvider
}

// EvalWorker consumes session events from Redis Streams and runs evals.
type EvalWorker struct {
	redisClient       goredis.UniversalClient
	resultWriter      EvalResultWriter
	messageStore      MessageStore
	namespaces        []string
	streamKeys        []string
	consumerGroup     string
	consumerName      string
	logger            *slog.Logger
	sdkRunner         *SDKRunner
	completionTracker *CompletionTracker
	rateLimiter       *RateLimiter
	packLoader        *PromptPackLoader
	providerResolver  *ProviderResolver
	metrics           WorkerMetricsRecorder
}

// NewEvalWorker creates a new eval worker for the given namespace(s).
func NewEvalWorker(config WorkerConfig) *EvalWorker {
	metricsRecorder := config.Metrics
	if metricsRecorder == nil {
		metricsRecorder = &NoOpWorkerMetrics{}
	}

	evalCollector := config.EvalCollector
	if evalCollector == nil {
		evalCollector = sdkmetrics.NewEvalOnlyCollector(sdkmetrics.CollectorOpts{
			Namespace: "omnia_eval",
		})
	}

	sdkRunner := config.SDKRunner
	if sdkRunner == nil {
		var runnerOpts []SDKRunnerOption
		if config.TracerProvider != nil {
			runnerOpts = append(runnerOpts, WithTracerProvider(config.TracerProvider))
		}
		if config.Logger != nil {
			runnerOpts = append(runnerOpts, WithLogger(config.Logger))
		}
		runnerOpts = append(runnerOpts, WithEvalCollector(evalCollector))
		runnerOpts = append(runnerOpts, WithMetrics(metricsRecorder))
		sdkRunner = NewSDKRunner(runnerOpts...)
	}

	msgStore := config.MessageStore
	if msgStore == nil {
		msgStore = redisprovider.NewFromClient(config.RedisClient, redisprovider.DefaultOptions())
	}

	timeout := config.InactivityTimeout
	if timeout == 0 {
		timeout = DefaultInactivityTimeout
	}

	rateLimiter := config.RateLimiter
	if rateLimiter == nil {
		rateLimiter = NewRateLimiter(nil)
	}

	var resolver *ProviderResolver
	if config.K8sClient != nil {
		resolver = NewProviderResolver(config.K8sClient)
	}

	namespaces := resolveNamespaces(config)
	streamKeys := buildStreamKeys(namespaces)
	consumerGroup := buildConsumerGroup(namespaces)

	w := &EvalWorker{
		redisClient:      config.RedisClient,
		resultWriter:     config.ResultWriter,
		messageStore:     msgStore,
		namespaces:       namespaces,
		streamKeys:       streamKeys,
		consumerGroup:    consumerGroup,
		consumerName:     hostname(),
		logger:           config.Logger,
		sdkRunner:        sdkRunner,
		rateLimiter:      rateLimiter,
		packLoader:       config.PackLoader,
		providerResolver: resolver,
		metrics:          metricsRecorder,
	}

	w.completionTracker = NewCompletionTracker(timeout, w.onSessionComplete, config.Logger)

	return w
}

// resolveNamespaces returns the effective namespace list from config,
// falling back from Namespaces to the deprecated Namespace field.
func resolveNamespaces(config WorkerConfig) []string {
	if len(config.Namespaces) > 0 {
		return config.Namespaces
	}
	if config.Namespace != "" {
		return []string{config.Namespace}
	}
	return nil
}

// buildStreamKeys converts namespaces to Redis stream keys.
func buildStreamKeys(namespaces []string) []string {
	keys := make([]string, len(namespaces))
	for i, ns := range namespaces {
		keys[i] = api.StreamKey(ns)
	}
	return keys
}

// buildConsumerGroup returns the consumer group name.
// For multi-namespace mode, uses a "cluster" suffix; for single-namespace, uses the namespace name.
func buildConsumerGroup(namespaces []string) string {
	if len(namespaces) > 1 {
		return consumerGroupPrefix + "cluster"
	}
	if len(namespaces) == 1 {
		return consumerGroupPrefix + namespaces[0]
	}
	return consumerGroupPrefix + "default"
}

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

// processAssistantMessage handles assistant message events by running per-turn evals.
func (w *EvalWorker) processAssistantMessage(ctx context.Context, event api.SessionEvent) error {
	packEvals := w.loadPackEvals(ctx, event)
	if packEvals == nil {
		w.logger.Debug("no per_turn evals to run (no pack)", "sessionID", event.SessionID)
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

	labels := EvalLabels{
		Agent:          sess.AgentName,
		Namespace:      event.Namespace,
		PromptPackName: packEvals.PackName,
	}
	items := w.getSDKRunner().RunTurnEvals(ctx, packEvals.PackData, messages,
		event.SessionID, turnIndex, providerSpecs, labels)
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
		w.logger.Debug("no on_session_complete evals to run (no pack)", "sessionID", sessionID)
		return nil
	}

	messages, err := w.getMessages(ctx, sessionID)
	if err != nil {
		return err
	}

	turnIndex := countAssistantMessages(messages)
	providerSpecs := w.resolveProviders(ctx, event)
	enrichedEvent := enrichEvent(event, packEvals)

	labels := EvalLabels{
		Agent:          sess.AgentName,
		Namespace:      event.Namespace,
		PromptPackName: packEvals.PackName,
	}
	items := w.getSDKRunner().RunSessionEvals(ctx, packEvals.PackData, messages,
		sessionID, turnIndex, providerSpecs, labels)
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

	labels := EvalLabels{
		Agent:          sess.AgentName,
		Namespace:      event.Namespace,
		PromptPackName: packEvals.PackName,
	}
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

// isEvaluateEvent returns true if the event is a manual eval trigger.
func isEvaluateEvent(event api.SessionEvent) bool {
	return event.EventType == eventTypeEvaluate
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

// EvalCollector returns the unified PromptKit metrics Collector for per-eval-name metrics.
// The caller (main.go) uses the Collector's Registry() for merged Prometheus gathering.
func (w *EvalWorker) EvalCollector() *sdkmetrics.Collector {
	return w.getSDKRunner().EvalCollector()
}

// getMessages reads session messages from the Redis hot tier.
func (w *EvalWorker) getMessages(ctx context.Context, sessionID string) ([]session.Message, error) {
	ptrMsgs, err := w.getMessageStore().GetRecentMessages(ctx, sessionID, 0)
	if err != nil {
		return nil, fmt.Errorf("get messages from redis: %w", err)
	}
	messages := make([]session.Message, 0, len(ptrMsgs))
	for _, m := range ptrMsgs {
		if m != nil {
			messages = append(messages, *m)
		}
	}
	return messages, nil
}

// convertToEvalResults converts SDK result items into persistable EvalResults.
func (w *EvalWorker) convertToEvalResults(
	items []api.EvaluateResultItem,
	event api.SessionEvent,
	agentName string,
) []*api.EvalResult {
	results := make([]*api.EvalResult, 0, len(items))
	for _, item := range items {
		result := toEvalResult(item, event, agentName)
		results = append(results, result)
	}
	return results
}

// getSDKRunner returns the SDK runner, initializing a default one if needed.
func (w *EvalWorker) getSDKRunner() *SDKRunner {
	if w.sdkRunner == nil {
		w.sdkRunner = NewSDKRunner()
	}
	return w.sdkRunner
}

// TracerProvider returns the tracer provider, if any. Exported for testing.
func (w *EvalWorker) TracerProvider() trace.TracerProvider {
	if w.sdkRunner != nil {
		return w.sdkRunner.tracerProvider
	}
	return nil
}

// getMessageStore returns the message store, initializing from the redis client if needed.
func (w *EvalWorker) getMessageStore() MessageStore {
	if w.messageStore == nil {
		w.messageStore = redisprovider.NewFromClient(w.redisClient, redisprovider.DefaultOptions())
	}
	return w.messageStore
}

// getResultWriter returns the result writer.
func (w *EvalWorker) getResultWriter() EvalResultWriter {
	return w.resultWriter
}

// getMetrics returns the metrics recorder, initializing a no-op one if needed.
func (w *EvalWorker) getMetrics() WorkerMetricsRecorder {
	if w.metrics == nil {
		w.metrics = &NoOpWorkerMetrics{}
	}
	return w.metrics
}

// getRateLimiter returns the rate limiter, initializing a default one if needed.
func (w *EvalWorker) getRateLimiter() *RateLimiter {
	if w.rateLimiter == nil {
		w.rateLimiter = NewRateLimiter(nil)
	}
	return w.rateLimiter
}

// countAssistantMessages counts the number of assistant messages to derive turn index.
func countAssistantMessages(messages []session.Message) int {
	count := 0
	for _, m := range messages {
		if m.Role == session.RoleAssistant {
			count++
		}
	}
	return count
}

// toEvalResult converts an EvaluateResultItem to an EvalResult for persistence.
func toEvalResult(item api.EvaluateResultItem, event api.SessionEvent, agentName string) *api.EvalResult {
	result := &api.EvalResult{
		SessionID:         event.SessionID,
		MessageID:         event.MessageID,
		AgentName:         agentName,
		Namespace:         event.Namespace,
		PromptPackName:    event.PromptPackName,
		PromptPackVersion: event.PromptPackVersion,
		EvalID:            item.EvalID,
		EvalType:          item.EvalType,
		Trigger:           item.Trigger,
		Passed:            item.Passed,
		Score:             item.Score,
		Details:           item.Details,
		Source:            evalSource,
		CreatedAt:         time.Now(),
	}

	if item.DurationMs > 0 {
		d := item.DurationMs
		result.DurationMs = &d
	}

	return result
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

// isAssistantMessageEvent returns true if the event is for an assistant message.
func isAssistantMessageEvent(event api.SessionEvent) bool {
	return event.EventType == eventTypeMessage && event.MessageRole == "assistant"
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
