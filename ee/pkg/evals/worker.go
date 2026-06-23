/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"log/slog"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"

	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
	redisprovider "github.com/altairalabs/omnia/internal/session/providers/redis"
)

// Constants for Redis consumer group and stream configuration.
const (
	consumerGroupPrefix     = "omnia-eval-workers-"
	blockTimeout            = 5 * time.Second
	evalSource              = "worker"
	eventTypeMessage        = "message.assistant"
	eventTypeSessionDone    = "session.completed"
	eventTypeEvaluate       = "session.evaluate"
	streamPayloadField      = "payload"
	streamReadBatchSize     = 10
	periodicCheckInterval   = 30 * time.Second
	pendingReclaimInterval  = 30 * time.Second
	pendingMinIdle          = 2 * time.Minute
	pendingReclaimBatchSize = 25
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
	// ProviderCallWriter persists the provider calls the eval pipeline emits
	// (judge LLM calls, RAG-eval embeddings, …) to session-api. If nil, those
	// calls are not recorded (no event bus is attached to sdk.Evaluate).
	ProviderCallWriter ProviderCallWriter
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

	// workerGroupsOverride pins resolveWorkerGroups to a fixed list,
	// bypassing both the resolver and the default. Test-only.
	workerGroupsOverride []string
	metrics              WorkerMetricsRecorder
	lastPendingReclaim   time.Time
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
		if config.ProviderCallWriter != nil {
			runnerOpts = append(runnerOpts, WithProviderCallWriter(config.ProviderCallWriter))
		}
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

type packEvalDefs struct {
	Evals []runtimeevals.EvalDef `json:"evals"`
}

// DefaultWorkerEvalGroups is the fallback group filter for the worker
// eval path when spec.evals.worker.groups is absent or empty on the
// AgentRuntime. It covers the expensive evaluation tiers — LLM judges
// and external API calls — which are unsuitable for the synchronous
// inline path. The "default" group is deliberately excluded here so
// simple handlers (which the runtime already covers) don't run twice.
var DefaultWorkerEvalGroups = []string{
	runtimeevals.GroupLongRunning,
	runtimeevals.GroupExternal,
}
