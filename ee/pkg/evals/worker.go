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

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AltairaLabs/PromptKit/runtime/providers"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// Constants for Redis consumer group and stream configuration.
const (
	consumerGroupPrefix   = "omnia-eval-workers-"
	blockTimeout          = 5 * time.Second
	evalSource            = "worker"
	triggerPerTurn        = "per_turn"
	triggerOnComplete     = "on_session_complete"
	eventTypeMessage      = "message.assistant"
	eventTypeSessionDone  = "session.completed"
	streamPayloadField    = "payload"
	periodicCheckInterval = 30 * time.Second
	evalTypeLLMJudge      = "llm_judge"
)

// EvalRunner executes a rule-based eval against session messages.
type EvalRunner func(evalDef api.EvalDefinition, messages []session.Message) (api.EvaluateResultItem, error)

// ProviderAwareEvalRunner executes evals with access to resolved provider specs.
// The providers map is keyed by provider name (e.g., "default", "judge").
type ProviderAwareEvalRunner func(
	evalDef api.EvalDefinition,
	messages []session.Message,
	providerSpecs map[string]providers.ProviderSpec,
) (api.EvaluateResultItem, error)

// WorkerConfig holds the configuration for an EvalWorker.
type WorkerConfig struct {
	RedisClient goredis.UniversalClient
	SessionAPI  SessionAPIClient
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
	// EvalRunner overrides the default eval runner.
	// If nil, a dispatcher that routes to RunArenaAssertion or RunRuleEval is used.
	EvalRunner EvalRunner
	// InactivityTimeout overrides the default completion inactivity timeout.
	// If zero, DefaultInactivityTimeout is used.
	InactivityTimeout time.Duration
	// Sampler controls hash-based deterministic eval sampling.
	// If nil, a default Sampler (100% default, 10% LLM judge) is used.
	Sampler *Sampler
	// RateLimiter controls eval execution throughput.
	// If nil, a default RateLimiter (50 evals/sec, 5 concurrent judges) is used.
	RateLimiter *RateLimiter
	// PackLoader loads eval definitions from PromptPack ConfigMaps.
	// If nil, no evals are loaded from PromptPacks (original behavior).
	PackLoader *PromptPackLoader
}

// EvalWorker consumes session events from Redis Streams and runs evals.
type EvalWorker struct {
	redisClient       goredis.UniversalClient
	sessionAPI        SessionAPIClient
	namespaces        []string
	streamKeys        []string
	consumerGroup     string
	consumerName      string
	logger            *slog.Logger
	evalRunner        EvalRunner
	evalRunnerPA      ProviderAwareEvalRunner
	completionTracker *CompletionTracker
	sampler           *Sampler
	rateLimiter       *RateLimiter
	packLoader        *PromptPackLoader
	providerResolver  *ProviderResolver
}

// NewEvalWorker creates a new eval worker for the given namespace(s).
func NewEvalWorker(config WorkerConfig) *EvalWorker {
	runner := config.EvalRunner
	if runner == nil {
		runner = func(def api.EvalDefinition, msgs []session.Message) (api.EvaluateResultItem, error) {
			if def.Type == EvalTypeArenaAssertion {
				return RunArenaAssertion(def, msgs)
			}
			return RunRuleEval(def, msgs)
		}
	}

	paRunner := defaultProviderAwareRunner(runner)

	timeout := config.InactivityTimeout
	if timeout == 0 {
		timeout = DefaultInactivityTimeout
	}

	sampler := config.Sampler
	if sampler == nil {
		sampler = NewSampler(nil)
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
		sessionAPI:       config.SessionAPI,
		namespaces:       namespaces,
		streamKeys:       streamKeys,
		consumerGroup:    consumerGroup,
		consumerName:     hostname(),
		logger:           config.Logger,
		evalRunner:       runner,
		evalRunnerPA:     paRunner,
		sampler:          sampler,
		rateLimiter:      rateLimiter,
		packLoader:       config.PackLoader,
		providerResolver: resolver,
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
	}
}

// readFromStreams performs the XREADGROUP call across all stream keys.
func (w *EvalWorker) readFromStreams(ctx context.Context) ([]goredis.XStream, error) {
	streamArgs := append(w.streamKeys, repeatedGt(len(w.streamKeys))...)
	return w.redisClient.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    w.consumerGroup,
		Consumer: w.consumerName,
		Streams:  streamArgs,
		Count:    1,
		Block:    blockTimeout,
	}).Result()
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
	if isSessionCompletedEvent(event) {
		w.getTracker().MarkCompleted(ctx, event.SessionID)
		return nil
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

	evalDefs := filterPerTurnEvals(packEvals.Evals)
	if len(evalDefs) == 0 {
		w.logger.Debug("no per_turn evals to run", "sessionID", event.SessionID)
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

	turnIndex := countAssistantMessages(messages)

	providerSpecs := w.resolveProviders(ctx, event)

	enrichedEvent := enrichEvent(event, packEvals)
	results := w.runEvalsWithSampling(ctx, evalDefs, messages, enrichedEvent, sess.AgentName, turnIndex, providerSpecs)
	return w.writeResults(ctx, results, event.SessionID)
}

// onSessionComplete is the CompletionTracker callback. It runs on_session_complete evals.
func (w *EvalWorker) onSessionComplete(ctx context.Context, sessionID string) error {
	defer w.completionTracker.Cleanup(sessionID)

	sess, err := w.sessionAPI.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session: %w", err)
	}

	event := api.SessionEvent{
		SessionID:         sessionID,
		Namespace:         sess.Namespace,
		PromptPackName:    sess.PromptPackName,
		PromptPackVersion: sess.PromptPackVersion,
	}

	packEvals := w.loadPackEvals(ctx, event)
	if packEvals == nil {
		w.logger.Debug("no on_session_complete evals to run (no pack)", "sessionID", sessionID)
		return nil
	}

	evalDefs := filterOnCompleteEvals(packEvals.Evals)
	if len(evalDefs) == 0 {
		w.logger.Debug("no on_session_complete evals to run", "sessionID", sessionID)
		return nil
	}

	messages, err := w.sessionAPI.GetSessionMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get session messages: %w", err)
	}

	turnIndex := countAssistantMessages(messages)

	providerSpecs := w.resolveProviders(ctx, event)

	enrichedEvent := enrichEvent(event, packEvals)
	results := w.runEvalsWithSampling(ctx, evalDefs, messages, enrichedEvent, sess.AgentName, turnIndex, providerSpecs)
	return w.writeResults(ctx, results, sessionID)
}

// writeResults writes eval results if there are any.
func (w *EvalWorker) writeResults(ctx context.Context, results []*api.EvalResult, sessionID string) error {
	if len(results) == 0 {
		return nil
	}

	if err := w.sessionAPI.WriteEvalResults(ctx, results); err != nil {
		return fmt.Errorf("write eval results: %w", err)
	}

	w.logger.Info("eval results written",
		"sessionID", sessionID,
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

// runEvalsWithSampling runs evals with sampling and rate limiting applied.
// Each eval is checked against the sampler before execution, and the rate
// limiter is consulted to avoid exceeding throughput limits.
func (w *EvalWorker) runEvalsWithSampling(
	ctx context.Context,
	evalDefs []api.EvalDefinition,
	messages []session.Message,
	event api.SessionEvent,
	agentName string,
	turnIndex int,
	providerSpecs map[string]providers.ProviderSpec,
) []*api.EvalResult {
	results := make([]*api.EvalResult, 0, len(evalDefs))

	for _, def := range evalDefs {
		isJudge := def.Type == evalTypeLLMJudge
		if !w.getSampler().ShouldSample(event.SessionID, turnIndex, isJudge) {
			continue
		}

		if err := w.acquireRateLimit(ctx, isJudge); err != nil {
			w.logger.Warn("rate limit acquisition failed",
				"evalID", def.ID,
				"sessionID", event.SessionID,
				"error", err,
			)
			break
		}

		if isJudge {
			defer w.getRateLimiter().ReleaseJudge()
		}

		result := w.executeSingleEval(def, messages, event, agentName, providerSpecs)
		if result != nil {
			results = append(results, result)
		}
	}

	return results
}

// executeSingleEval runs one eval and returns the result, or nil on error.
func (w *EvalWorker) executeSingleEval(
	def api.EvalDefinition,
	messages []session.Message,
	event api.SessionEvent,
	agentName string,
	providerSpecs map[string]providers.ProviderSpec,
) *api.EvalResult {
	item, err := w.getProviderAwareRunner()(def, messages, providerSpecs)
	if err != nil {
		w.logger.Warn("eval failed",
			"evalID", def.ID,
			"sessionID", event.SessionID,
			"error", err,
		)
		return nil
	}

	item.Source = evalSource
	return toEvalResult(item, event, agentName)
}

// acquireRateLimit acquires the appropriate rate limit token.
func (w *EvalWorker) acquireRateLimit(ctx context.Context, isJudge bool) error {
	rl := w.getRateLimiter()
	if isJudge {
		return rl.AcquireJudge(ctx)
	}
	return rl.Acquire(ctx)
}

// getSampler returns the sampler, initializing a default one if needed.
func (w *EvalWorker) getSampler() *Sampler {
	if w.sampler == nil {
		w.sampler = NewSampler(nil)
	}
	return w.sampler
}

// getProviderAwareRunner returns the provider-aware eval runner, initializing from
// the legacy evalRunner if needed. This ensures backward compatibility with tests
// that construct EvalWorker directly without using NewEvalWorker.
func (w *EvalWorker) getProviderAwareRunner() ProviderAwareEvalRunner {
	if w.evalRunnerPA == nil {
		w.evalRunnerPA = defaultProviderAwareRunner(w.evalRunner)
	}
	return w.evalRunnerPA
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
		Source:            evalSource,
		CreatedAt:         time.Now(),
	}

	if item.DurationMs > 0 {
		d := item.DurationMs
		result.DurationMs = &d
	}

	return result
}

// isDeterministicEval returns true for eval types that are deterministic
// (not requiring an LLM call) and can be run synchronously in-process.
func isDeterministicEval(evalType string) bool {
	return evalType != evalTypeLLMJudge
}

// filterPerTurnDeterministicEvals filters eval definitions to per_turn deterministic evals.
// This includes rule-based evals and arena assertions, but excludes LLM judge evals.
func filterPerTurnDeterministicEvals(defs []EvalDef) []api.EvalDefinition {
	var result []api.EvalDefinition
	for _, d := range defs {
		if d.Trigger == triggerPerTurn && isDeterministicEval(d.Type) {
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

// filterPerTurnEvals filters eval definitions to all per_turn evals (including LLM judges).
func filterPerTurnEvals(defs []EvalDef) []api.EvalDefinition {
	var result []api.EvalDefinition
	for _, d := range defs {
		if d.Trigger == triggerPerTurn {
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

// filterOnCompleteEvals filters eval definitions to all on_session_complete evals.
func filterOnCompleteEvals(defs []EvalDef) []api.EvalDefinition {
	var result []api.EvalDefinition
	for _, d := range defs {
		if d.Trigger == triggerOnComplete {
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

// loadPackEvals loads eval definitions from the PromptPack referenced in the event.
// Returns nil if no pack loader is configured or the event has no PromptPack name.
func (w *EvalWorker) loadPackEvals(ctx context.Context, event api.SessionEvent) *PromptPackEvals {
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
func enrichEvent(event api.SessionEvent, packEvals *PromptPackEvals) api.SessionEvent {
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

// filterOnCompleteDeterministicEvals filters eval definitions to on_session_complete deterministic evals.
// This includes rule-based evals and arena assertions, but excludes LLM judge evals.
func filterOnCompleteDeterministicEvals(defs []EvalDef) []api.EvalDefinition {
	var result []api.EvalDefinition
	for _, d := range defs {
		if d.Trigger == triggerOnComplete && isDeterministicEval(d.Type) {
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

// defaultProviderAwareRunner wraps a legacy EvalRunner in a ProviderAwareEvalRunner.
// For arena assertions and PromptKit eval types, it passes providers via
// RunArenaAssertionWithProviders. Other eval types fall through to the legacy runner.
func defaultProviderAwareRunner(legacy EvalRunner) ProviderAwareEvalRunner {
	return func(
		def api.EvalDefinition,
		msgs []session.Message,
		providerSpecs map[string]providers.ProviderSpec,
	) (api.EvaluateResultItem, error) {
		if def.Type == EvalTypeArenaAssertion {
			return RunArenaAssertionWithProviders(def, msgs, providerSpecs)
		}
		// Rule-based evals don't need providers
		return legacy(def, msgs)
	}
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
