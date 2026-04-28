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

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AltairaLabs/PromptKit/runtime/credentials"
	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	// Side-effect imports register each provider's embedding factory with
	// pkproviders.CreateEmbeddingProviderFromSpec — keeps this file out of
	// the per-provider option-builder business.
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/gemini"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/ollama"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/openai"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	eeaudit "github.com/altairalabs/omnia/ee/pkg/audit"
	eemetrics "github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/ee/pkg/privacy/classify"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
	"github.com/altairalabs/omnia/internal/memory"
	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	memorypg "github.com/altairalabs/omnia/internal/memory/postgres"
	sessionapi "github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/internal/tracing"
	omniak8s "github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
)

// auditLoggerAdapter adapts ee/pkg/audit.Logger to memoryapi.MemoryAuditLogger.
// It converts MemoryAuditEntry fields into the session/api.AuditEntry shape,
// placing memory-specific fields (MemoryID, Kind) in Metadata.
type auditLoggerAdapter struct {
	inner *eeaudit.Logger
}

func (a *auditLoggerAdapter) LogEvent(ctx context.Context, entry *memoryapi.MemoryAuditEntry) {
	meta := entry.Metadata
	if meta == nil {
		meta = make(map[string]string)
	}
	if entry.MemoryID != "" {
		meta["memoryId"] = entry.MemoryID
	}
	if entry.Kind != "" {
		meta["kind"] = entry.Kind
	}
	a.inner.LogEvent(ctx, &sessionapi.AuditEntry{
		EventType: entry.EventType,
		Workspace: entry.WorkspaceID,
		IPAddress: entry.IPAddress,
		UserAgent: entry.UserAgent,
		Metadata:  meta,
	})
}

// flags groups all CLI flags for the memory-api binary.
type flags struct {
	apiAddr               string
	healthAddr            string
	metricsAddr           string
	postgresConn          string
	redisAddrs            string
	enterprise            bool
	tracingEnabled        bool
	tracingEndpoint       string
	tracingSample         float64
	tracingInsecure       bool
	embeddingProviderName string // name of the Provider CRD for embeddings
	defaultTTL            string // env: DEFAULT_TTL, e.g. "720h"
	purpose               string // env: MEMORY_PURPOSE, e.g. "support_continuity"
	retentionInterval     string // env: RETENTION_INTERVAL, e.g. "1h"
	compactionInterval    string // env: COMPACTION_INTERVAL, e.g. "6h"
	compactionAge         string // env: COMPACTION_AGE, e.g. "720h" (30d)
	reembedInterval       string // env: REEMBED_INTERVAL, e.g. "30m"
	reembedBatchSize      int    // env: REEMBED_BATCH_SIZE
	tombstoneInterval     string // env: TOMBSTONE_INTERVAL, e.g. "6h"
	tombstoneMinAge       string // env: TOMBSTONE_MIN_AGE, e.g. "720h" (30d)
	tombstoneMinInactive  int    // env: TOMBSTONE_MIN_INACTIVE
	tombstoneKeepRecent   int    // env: TOMBSTONE_KEEP_RECENT
	requireAboutForKinds  string // env: REQUIRE_ABOUT_FOR_KINDS, e.g. "fact,preference"
	workspace             string
	serviceGroup          string
}

func parseFlags() *flags {
	f := &flags{}
	flag.StringVar(&f.apiAddr, "api-addr", ":8080", "API server listen address")
	flag.StringVar(&f.healthAddr, "health-addr", ":8081", "Health probe listen address")
	flag.StringVar(&f.metricsAddr, "metrics-addr", ":9090", "Metrics server listen address")
	flag.StringVar(&f.postgresConn, "postgres-conn", "", "Postgres connection string")
	flag.StringVar(&f.redisAddrs, "redis-addrs", "", "Redis addresses (comma-separated)")
	flag.BoolVar(&f.enterprise, "enterprise", false, "Enable enterprise features (audit logging)")
	flag.BoolVar(&f.tracingEnabled, "tracing-enabled", false, "Enable OpenTelemetry tracing")
	flag.StringVar(&f.tracingEndpoint, "tracing-endpoint", "", "OTel collector endpoint")
	flag.Float64Var(&f.tracingSample, "tracing-sample", 0, "Tracing sample rate (0.0-1.0)")
	flag.BoolVar(&f.tracingInsecure, "tracing-insecure", false, "Use insecure gRPC for tracing")
	flag.StringVar(&f.embeddingProviderName, "embedding-provider", "", "Name of the Provider CRD to use for embeddings")
	flag.StringVar(&f.defaultTTL, "default-ttl", "", "Default memory TTL duration (e.g. 720h)")
	flag.StringVar(&f.purpose, "purpose", "", "Default memory purpose tag (e.g. support_continuity)")
	flag.StringVar(&f.retentionInterval, "retention-interval", "", "Interval for retention worker (e.g. 1h)")
	flag.StringVar(&f.compactionInterval, "compaction-interval", "", "Interval for temporal-summarization compaction worker (e.g. 6h). Empty disables.")
	flag.StringVar(&f.compactionAge, "compaction-age", "", "Age threshold for compaction candidates (e.g. 720h = 30d). Empty uses worker default.")
	flag.StringVar(&f.reembedInterval, "reembed-interval", "", "Interval for re-embed backfill worker (e.g. 30m). Empty disables.")
	flag.IntVar(&f.reembedBatchSize, "reembed-batch-size", 0, "Re-embed batch size per pass. Zero uses worker default (50).")
	flag.StringVar(&f.tombstoneInterval, "tombstone-interval", "", "Interval for tombstone GC worker (e.g. 6h). Empty disables.")
	flag.StringVar(&f.tombstoneMinAge, "tombstone-min-age", "", "Minimum age before an inactive observation is GC-eligible (e.g. 720h). Empty uses worker default.")
	flag.IntVar(&f.tombstoneMinInactive, "tombstone-min-inactive", 0, "Chain length below which tombstone GC leaves observations alone. Zero uses worker default (20).")
	flag.IntVar(&f.tombstoneKeepRecent, "tombstone-keep-recent", 0, "Most-recent inactive observations preserved per chain for audit. Zero uses worker default (5).")
	flag.StringVar(&f.requireAboutForKinds, "require-about-for-kinds", "", "Comma-separated list of memory kinds requiring an about={kind, key} hint on save (e.g. fact,preference). Empty disables.")
	flag.StringVar(&f.workspace, "workspace", "", "Workspace name (K8s CRD resolution mode)")
	flag.StringVar(&f.serviceGroup, "service-group", "", "Service group name within workspace")
	flag.Parse()

	f.applyEnvFallbacks()
	return f
}

// applyEnvFallbacks applies environment variable overrides to flag defaults.
func (f *flags) applyEnvFallbacks() {
	envFallback(&f.postgresConn, "", "POSTGRES_CONN")
	envFallback(&f.redisAddrs, "", "REDIS_ADDRS")
	envFallback(&f.apiAddr, ":8080", "API_ADDR")
	envFallback(&f.healthAddr, ":8081", "HEALTH_ADDR")
	envFallback(&f.metricsAddr, ":9090", "METRICS_ADDR")

	envBoolFallback(&f.enterprise, "ENTERPRISE_ENABLED")
	envBoolFallback(&f.tracingEnabled, "TRACING_ENABLED")
	envBoolFallback(&f.tracingInsecure, "TRACING_INSECURE")
	envFallback(&f.tracingEndpoint, "", "TRACING_ENDPOINT")
	envFallback(&f.embeddingProviderName, "", "EMBEDDING_PROVIDER")
	envFallback(&f.defaultTTL, "", "DEFAULT_TTL")
	envFallback(&f.purpose, "", "MEMORY_PURPOSE")
	envFallback(&f.retentionInterval, "", "RETENTION_INTERVAL")
	envFallback(&f.compactionInterval, "", "COMPACTION_INTERVAL")
	envFallback(&f.compactionAge, "", "COMPACTION_AGE")
	envFallback(&f.reembedInterval, "", "REEMBED_INTERVAL")
	if v := os.Getenv("REEMBED_BATCH_SIZE"); v != "" && f.reembedBatchSize == 0 {
		if n, err := strconv.Atoi(v); err == nil {
			f.reembedBatchSize = n
		}
	}
	envFallback(&f.requireAboutForKinds, "", "REQUIRE_ABOUT_FOR_KINDS")
	envFallback(&f.tombstoneInterval, "", "TOMBSTONE_INTERVAL")
	envFallback(&f.tombstoneMinAge, "", "TOMBSTONE_MIN_AGE")
	if v := os.Getenv("TOMBSTONE_MIN_INACTIVE"); v != "" && f.tombstoneMinInactive == 0 {
		if n, err := strconv.Atoi(v); err == nil {
			f.tombstoneMinInactive = n
		}
	}
	if v := os.Getenv("TOMBSTONE_KEEP_RECENT"); v != "" && f.tombstoneKeepRecent == 0 {
		if n, err := strconv.Atoi(v); err == nil {
			f.tombstoneKeepRecent = n
		}
	}
	if v := os.Getenv("TRACING_SAMPLE_RATE"); v != "" && f.tracingSample == 0 {
		if rate, err := strconv.ParseFloat(v, 64); err == nil {
			f.tracingSample = rate
		}
	}
}

// envFallback sets *dst from the environment variable envKey when *dst still
// equals the default value and the environment variable is non-empty.
func envFallback(dst *string, defaultVal, envKey string) {
	if *dst == defaultVal {
		if v := os.Getenv(envKey); v != "" {
			*dst = v
		}
	}
}

// envBoolFallback enables a boolean flag from an environment variable when the
// flag is still false and the env var is "true".
func envBoolFallback(dst *bool, envKey string) {
	if !*dst && os.Getenv(envKey) == "true" {
		*dst = true
	}
}

// compactionWorkerOptions returns the CompactionWorkerOptions derived from
// flags/env plus a discoverer that pulls the workspace list from the store.
// enabled=false signals that the caller should skip starting the worker
// (interval unset or invalid).
func (f *flags) compactionWorkerOptions(log logr.Logger, store *memory.PostgresMemoryStore) (memory.CompactionWorkerOptions, bool) {
	if f.compactionInterval == "" {
		return memory.CompactionWorkerOptions{}, false
	}
	interval, err := time.ParseDuration(f.compactionInterval)
	if err != nil || interval <= 0 {
		log.Error(err, "invalid COMPACTION_INTERVAL, compaction worker disabled",
			"value", f.compactionInterval)
		return memory.CompactionWorkerOptions{}, false
	}
	opts := memory.CompactionWorkerOptions{
		Interval:            interval,
		WorkspaceDiscoverer: store.ListWorkspaceIDs,
	}
	if f.compactionAge != "" {
		age, ageErr := time.ParseDuration(f.compactionAge)
		if ageErr != nil {
			log.Error(ageErr, "invalid COMPACTION_AGE, using worker default", "value", f.compactionAge)
		} else {
			opts.Age = age
		}
	}
	return opts, true
}

// parseCSV splits a comma-separated string into a trimmed, non-empty
// slice. Used for list-shaped flags / env vars (kinds, providers).
func parseCSV(in string) []string {
	if in == "" {
		return nil
	}
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// tombstoneWorkerOptions returns the TombstoneWorkerOptions derived
// from flags/env. enabled=false when the interval flag is missing or
// invalid — the worker exits cleanly in that case anyway, but the
// guard avoids the noisy "disabled" log line.
func (f *flags) tombstoneWorkerOptions(log logr.Logger, store *memory.PostgresMemoryStore) (memory.TombstoneWorkerOptions, bool) {
	if f.tombstoneInterval == "" {
		return memory.TombstoneWorkerOptions{}, false
	}
	interval, err := time.ParseDuration(f.tombstoneInterval)
	if err != nil || interval <= 0 {
		log.Error(err, "invalid TOMBSTONE_INTERVAL, tombstone worker disabled",
			"value", f.tombstoneInterval)
		return memory.TombstoneWorkerOptions{}, false
	}
	opts := memory.TombstoneWorkerOptions{
		Interval:            interval,
		WorkspaceDiscoverer: store.ListWorkspaceIDs,
		MinInactiveCount:    f.tombstoneMinInactive,
		KeepRecentInactive:  f.tombstoneKeepRecent,
	}
	if f.tombstoneMinAge != "" {
		age, ageErr := time.ParseDuration(f.tombstoneMinAge)
		if ageErr != nil {
			log.Error(ageErr, "invalid TOMBSTONE_MIN_AGE, using worker default",
				"value", f.tombstoneMinAge)
		} else {
			opts.MinAge = age
		}
	}
	return opts, true
}

// reembedWorkerOptions returns the ReembedWorkerOptions derived from
// flags/env. enabled=false when no interval is set OR no embedding
// service is configured — without a provider the worker has nothing
// to call, and without an interval it would never tick.
func (f *flags) reembedWorkerOptions(embeddingSvc *memory.EmbeddingService) (memory.ReembedWorkerOptions, bool) {
	if embeddingSvc == nil || f.reembedInterval == "" {
		return memory.ReembedWorkerOptions{}, false
	}
	interval, err := time.ParseDuration(f.reembedInterval)
	if err != nil || interval <= 0 {
		return memory.ReembedWorkerOptions{}, false
	}
	return memory.ReembedWorkerOptions{
		Interval:     interval,
		BatchSize:    f.reembedBatchSize,
		CurrentModel: f.embeddingProviderName,
	}, true
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	f := parseFlags()

	// --- Logger ---
	log, syncLog, err := logging.NewLogger()
	if err != nil {
		return fmt.Errorf("creating logger: %w", err)
	}
	defer syncLog()

	// --- Workspace CRD config resolution ---
	if f.workspace != "" && f.serviceGroup != "" {
		if err := f.resolveConfigFromWorkspace(log); err != nil {
			return fmt.Errorf("resolving config from workspace: %w", err)
		}
	}

	// --- Validate ---
	if f.postgresConn == "" {
		return fmt.Errorf("--postgres-conn or POSTGRES_CONN is required")
	}

	// --- Signal context ---
	ctx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	// --- Postgres pool ---
	pool, err := initPool(ctx, f.postgresConn)
	if err != nil {
		return err
	}
	defer pool.Close()
	log.V(1).Info("postgres pool created",
		"maxConns", envInt32("PG_MAX_CONNS", defaultMaxConns),
		"minConns", envInt32("PG_MIN_CONNS", defaultMinConns),
	)

	// --- Migrations ---
	if err := runMigrations(f.postgresConn, log); err != nil {
		return err
	}
	log.V(1).Info("migrations complete")

	// --- Memory store ---
	store := memory.NewPostgresMemoryStore(pool)
	defer store.Close()

	// --- Read-path metrics ---
	// accessed_at / access_count are bumped asynchronously on every
	// retrieval. Register the Prometheus counters + histogram so the
	// signal is observable in dashboards. Failure to register isn't
	// fatal — retrieval still works, just without the counters.
	if err := memory.RegisterAccessMetrics(prometheus.DefaultRegisterer); err != nil {
		log.Error(err, "memory access metrics registration failed")
	}

	// --- Service config: TTL + purpose ---
	var defaultTTL time.Duration
	if f.defaultTTL != "" {
		if d, err := time.ParseDuration(f.defaultTTL); err == nil {
			defaultTTL = d
		} else {
			log.Error(err, "invalid DEFAULT_TTL, defaulting to no TTL", "value", f.defaultTTL)
		}
	}
	svcCfg := memoryapi.MemoryServiceConfig{
		DefaultTTL:           defaultTTL,
		Purpose:              f.purpose,
		RequireAboutForKinds: parseCSV(f.requireAboutForKinds),
	}

	// --- Policy loader (shared by retention worker + retrieval ranker) ---
	policyLoader := buildRetentionPolicyLoader(f.retentionInterval, f.workspace, f.serviceGroup, log)

	// --- Retention worker ---
	startRetentionWorkerWithLoader(ctx, store, policyLoader, log)

	// --- Analytics opt-in metric + worker ---
	// Computes the fraction of users who have granted the
	// analytics:aggregate consent category. Runs in any mode (EE or
	// OSS); in OSS the grant is never set so the ratio reports 0 which
	// is correct (no cross-user analytics consent).
	optInMetrics := memory.NewAnalyticsOptInMetrics()
	if err := memory.RegisterAnalyticsOptInMetrics(prometheus.DefaultRegisterer, optInMetrics); err != nil {
		log.Error(err, "analytics opt-in metrics registration failed")
	} else {
		optInWorker := memory.NewAnalyticsOptInWorker(pool, optInMetrics, log)
		go optInWorker.Run(ctx)
		log.Info("analytics opt-in worker started",
			"interval", memory.DefaultAnalyticsOptInInterval,
		)
	}

	// --- Compaction worker ---
	// Temporal summarization of old memories. Uses NoopSummarizer by default —
	// memory growth is still bounded because the worker supersedes originals,
	// but summaries aren't informative until a real LLM summarizer is wired.
	if compactionOpts, enabled := f.compactionWorkerOptions(log, store); enabled {
		worker := memory.NewCompactionWorker(store, memory.NoopSummarizer{}, compactionOpts, log)
		go worker.Run(ctx)
		log.Info("compaction worker started",
			"interval", compactionOpts.Interval,
			"age", compactionOpts.Age,
			"summarizer", "noop",
		)
	}

	// --- Embedding service ---
	var embeddingSvc *memory.EmbeddingService
	if f.embeddingProviderName != "" {
		embeddingSvc = createEmbeddingService(ctx, f.embeddingProviderName, store, log)
	}

	// --- Tombstone GC worker ---
	// Hard-deletes old superseded observations on long supersession
	// chains, keeping the most recent K per chain for audit. Bounds
	// storage growth without losing the agent-visible "this got
	// updated" history.
	if tombstoneOpts, enabled := f.tombstoneWorkerOptions(log, store); enabled {
		worker := memory.NewTombstoneWorker(store, tombstoneOpts, log)
		go worker.Run(ctx)
		log.Info("tombstone worker started",
			"interval", tombstoneOpts.Interval,
			"minAge", tombstoneOpts.MinAge,
			"minInactiveCount", tombstoneOpts.MinInactiveCount,
			"keepRecent", tombstoneOpts.KeepRecentInactive,
		)
	}

	// --- Re-embed backfill worker ---
	// Backfills observations missing an embedding (pre-wiring rows
	// or rows stamped with a now-superseded model) so the hybrid
	// recall path doesn't silently miss them. Requires both an
	// embedding service AND a configured interval.
	if reembedOpts, enabled := f.reembedWorkerOptions(embeddingSvc); enabled {
		worker := memory.NewReembedWorker(store, embeddingSvc.Provider(), reembedOpts, log)
		go worker.Run(ctx)
		log.Info("reembed worker started",
			"interval", reembedOpts.Interval,
			"batchSize", reembedOpts.BatchSize,
			"currentModel", reembedOpts.CurrentModel,
		)
	}

	// --- Tracing ---
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if f.tracingEnabled {
		tracingCfg := tracing.Config{
			Enabled:     true,
			Endpoint:    f.tracingEndpoint,
			ServiceName: "omnia-memory-api",
			SampleRate:  f.tracingSample,
			Insecure:    f.tracingInsecure,
		}
		tp, tpErr := tracing.NewProvider(ctx, tracingCfg)
		if tpErr != nil {
			log.Error(tpErr, "tracing provider creation failed")
		} else {
			otel.SetTracerProvider(tp.TracerProvider())
			defer func() { _ = tp.Shutdown(ctx) }()
			log.Info("tracing enabled", "endpoint", f.tracingEndpoint, "sampleRate", f.tracingSample)
		}
	}

	// --- Event publisher (optional) ---
	var eventPublisher memoryapi.MemoryEventPublisher
	if f.redisAddrs != "" {
		redisClient := goredis.NewClient(&goredis.Options{Addr: f.redisAddrs})
		eventPublisher = memoryapi.NewRedisMemoryEventPublisher(redisClient, log)
		log.Info("memory event publisher enabled", "redisAddrs", f.redisAddrs)
	}

	// --- Build API mux ---
	apiMux, cleanup := buildAPIMux(ctx, store, embeddingSvc, svcCfg, eventPublisher, f.enterprise, pool, policyLoader, log)
	defer cleanup()

	// --- Servers ---
	healthSrv := newHealthServer(f.healthAddr, pool)
	metricsSrv := newMetricsServer(f.metricsAddr)
	apiSrv := &http.Server{
		Addr:         f.apiAddr,
		Handler:      apiMux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	startHTTPServer(log, "health", f.healthAddr, healthSrv)
	startHTTPServer(log, "metrics", f.metricsAddr, metricsSrv)
	startHTTPServer(log, "memory API", f.apiAddr, apiSrv)

	log.Info("memory-api ready",
		"api", f.apiAddr,
		"health", f.healthAddr,
		"metrics", f.metricsAddr,
		"enterprise", f.enterprise,
	)

	// --- Wait for shutdown ---
	<-ctx.Done()
	log.Info("shutting down")

	shutdownServers(log, apiSrv, healthSrv, metricsSrv)
	return nil
}

// resolveConfigFromWorkspace uses the Kubernetes API to resolve memory-api
// configuration from the named Workspace CRD and service group.
func (f *flags) resolveConfigFromWorkspace(log logr.Logger) error {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("building K8s client config: %w", err)
	}
	scheme := k8sruntime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("creating K8s client: %w", err)
	}
	cr := servicediscovery.NewConfigResolver(c)
	namespace := detectNamespace()
	memCfg, err := cr.ResolveMemoryConfig(context.Background(), f.workspace, f.serviceGroup, namespace)
	if err != nil {
		return fmt.Errorf("resolving memory config: %w", err)
	}
	f.postgresConn = memCfg.PostgresConn
	f.embeddingProviderName = memCfg.EmbeddingProviderName
	log.Info("config resolved from workspace CRD",
		"workspace", f.workspace,
		"serviceGroup", f.serviceGroup,
		"hasEmbeddingProvider", memCfg.EmbeddingProviderName != "")
	return nil
}

// detectNamespace returns the Kubernetes namespace this process is running in.
// It checks OMNIA_NAMESPACE first, then the in-cluster service account file,
// and falls back to "default".
func detectNamespace() string {
	if ns := os.Getenv("OMNIA_NAMESPACE"); ns != "" {
		return ns
	}
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "default"
	}
	return string(data)
}

// buildAPIMux assembles the HTTP handler with all memory-api routes, wrapped
// with rate limiting, privacy (enterprise), metrics, and tracing middleware.
// Returns the handler and a cleanup function.
func buildAPIMux(
	ctx context.Context,
	store memory.Store,
	embeddingSvc *memory.EmbeddingService,
	cfg memoryapi.MemoryServiceConfig,
	publisher memoryapi.MemoryEventPublisher,
	enterprise bool,
	pool *pgxpool.Pool,
	policyLoader memory.PolicyLoader,
	log logr.Logger,
) (http.Handler, func()) {
	httpMetrics := memoryapi.NewHTTPMetrics(nil)

	svc := memoryapi.NewMemoryService(store, embeddingSvc, cfg, log)
	if publisher != nil {
		svc.SetEventPublisher(publisher)
	}
	if policyLoader != nil {
		// Retrieval consults the loader to build a per-tier ranker from
		// the workspace's bound MemoryPolicy.spec.tierPrecedence.
		svc.SetPolicyLoader(policyLoader)
	}

	// Enterprise audit logging.
	var auditClose func() error
	var auditHandler *eeaudit.Handler
	if enterprise {
		auditMetrics := eemetrics.NewAuditMetrics()
		auditLogger := eeaudit.NewLogger(pool, log, auditMetrics, eeaudit.LoggerConfig{})
		auditClose = auditLogger.Close
		svc.SetAuditLogger(&auditLoggerAdapter{inner: auditLogger})
		auditHandler = eeaudit.NewHandler(auditLogger, log)
		log.Info("memory audit logging enabled")
	}

	handler := memoryapi.NewHandler(svc, log)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	if auditHandler != nil {
		auditHandler.RegisterMemoryRoutes(mux)
	}
	if enterprise {
		// Consent stats endpoint for the operator dashboard (#1004).
		// EE-only because the enforcement (Phase D) and grant capture
		// (Phases B/C) are EE features; OSS counts would always be 0.
		consentStatsHandler := privacy.NewConsentStatsHandler(privacy.NewPreferencesStore(pool), log)
		consentStatsHandler.RegisterRoutes(mux)
	}

	// AuditMiddleware always applied — populates request context with IP/UA.
	// The service only emits events when an audit logger is configured.
	apiHandler := memoryapi.AuditMiddleware(mux)

	// Enterprise privacy middleware (opt-out + PII redaction + classifier).
	if enterprise {
		apiHandler = wrapPrivacyMiddleware(ctx, apiHandler, pool, embeddingSvc, log)
	}

	// Rate limiting middleware (per-client-IP token bucket).
	rlCfg := sessionapi.RateLimitConfigFromEnv()
	rlMiddleware, rlStop := sessionapi.NewRateLimitMiddleware(rlCfg)
	log.V(1).Info("rate limiter initialized", "rps", rlCfg.RPS, "burst", rlCfg.Burst)

	traced := otelhttp.NewHandler(traceLogMiddleware(apiHandler), "memory-api",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/healthz"
		}),
	)
	cleanup := func() {
		rlStop()
		if auditClose != nil {
			_ = auditClose()
		}
	}
	return rlMiddleware(httpMetrics.MetricsMiddleware(traced)), cleanup
}

// wrapPrivacyMiddleware creates and wires the enterprise privacy middleware.
// When the K8s API is unreachable (e.g., in tests), the middleware is skipped
// and the original handler is returned unchanged.
func wrapPrivacyMiddleware(ctx context.Context, next http.Handler, pool *pgxpool.Pool, embeddingSvc *memory.EmbeddingService, log logr.Logger) http.Handler {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Info("memory privacy middleware skipped", "reason", "no in-cluster kubeconfig")
		return next
	}

	scheme := k8sruntime.NewScheme()
	utilruntime.Must(eev1alpha1.AddToScheme(scheme))
	k8sClient, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "memory privacy middleware skipped", "reason", "k8s client creation failed")
		return next
	}

	watcher := privacy.NewPolicyWatcher(k8sClient, log)

	// Start the watcher asynchronously; it syncs the policy cache in the background.
	go func() {
		if startErr := watcher.Start(context.Background()); startErr != nil {
			log.Error(startErr, "memory policy watcher start failed")
		}
	}()

	prefStore := privacy.NewPreferencesStore(pool)
	redactor := redaction.NewRedactor()

	checkOptOut := memoryapi.OptOutChecker(func(ctx context.Context, userID, workspace, category string, consentOverride []string) bool {
		cat := privacy.ConsentCategory(category)
		if cat == "" {
			cat = privacy.ConsentMemoryContext
		}
		var source privacy.ConsentSource
		if len(consentOverride) > 0 {
			source = privacy.NewStaticConsentSource(consentOverride)
		} else {
			source = prefStore
		}
		return privacy.ShouldRememberCategory(ctx, prefStore, source, userID, workspace, "", cat)
	})

	contentRedactor := memoryapi.ContentRedactor(func(ctx context.Context, workspace, content, provenance string) (string, error) {
		policy := watcher.GetEffectivePolicy(workspace, "")
		if policy == nil || policy.Recording.PII == nil || !policy.Recording.PII.Redact {
			return content, nil
		}
		trust := trustLevelForProvenance(provenance)
		redacted, _, err := redactor.RedactWithTrust(ctx, content, policy.Recording.PII, trust)
		return redacted, err
	})

	validator := buildConsentValidator(ctx, embeddingSvc, log)
	suppressMetrics := memoryapi.NewSuppressionMetrics()
	if err := suppressMetrics.Register(prometheus.DefaultRegisterer); err != nil {
		log.Error(err, "memory suppression metrics registration failed")
	}
	mw := memoryapi.NewMemoryPrivacyMiddlewareWithMetrics(checkOptOut, contentRedactor, validator, suppressMetrics, log)
	log.Info("memory privacy middleware enabled")
	return mw.Wrap(next)
}

// buildConsentValidator constructs the EE Validator used by the privacy
// middleware. It always wires the rule classifier; the embedding
// classifier is wired only when an embedding provider is available.
// Prometheus metrics are registered on the default registerer; a
// duplicate registration error is logged but not fatal.
func buildConsentValidator(ctx context.Context, embeddingSvc *memory.EmbeddingService, log logr.Logger) memoryapi.CategoryValidator {
	rules := classify.NewRuleClassifier()

	var embedding *classify.EmbeddingClassifier
	if embeddingSvc != nil {
		embedding = classify.NewEmbeddingClassifier(
			embedderAdapter{provider: embeddingSvc.Provider()},
			log.WithName("classifier"),
		)
		// Prewarm centroids in a goroutine — failure is non-fatal, the
		// classifier will retry on first save.
		go func() {
			pwCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := embedding.PrewarmCentroids(pwCtx); err != nil {
				log.Error(err, "centroid prewarm failed; classifier will retry on first save")
			}
		}()
	} else {
		log.Info("consent classifier running in regex-only mode",
			"reason", "no embedding provider configured")
	}

	metrics := classify.NewMetrics()
	if err := metrics.Register(prometheus.DefaultRegisterer); err != nil {
		log.Error(err, "consent classifier metrics registration failed")
	}

	v := classify.NewValidator(rules, embedding)
	return func(ctx context.Context, claimed, content string) memoryapi.ValidatorResult {
		res := v.Apply(ctx, privacy.ConsentCategory(claimed), content)
		metrics.RecordResult(claimed, res)
		return memoryapi.ValidatorResult{
			Category:   string(res.Category),
			Overridden: res.Overridden,
			From:       string(res.From),
			Source:     res.Source,
		}
	}
}

// embedderAdapter adapts memory.EmbeddingProvider (Embed + Dimensions)
// to the smaller classify.Embedder interface used by the consent
// classifier (Embed only).
type embedderAdapter struct {
	provider memory.EmbeddingProvider
}

func (a embedderAdapter) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return a.provider.Embed(ctx, texts)
}

// trustLevelForProvenance maps a PromptKit provenance string to the
// redaction trust level. user_requested and operator_curated memories are
// content the caller intentionally asked to persist, so personal-detail
// patterns (email, phone) are dropped from the redaction set and only
// structural identifiers (SSN, credit-card, IP, custom patterns) get
// scrubbed. Agent-extracted, system-generated, and unrecognised provenance
// values fall back to TrustInferred so the full pattern set applies.
func trustLevelForProvenance(provenance string) redaction.TrustLevel {
	switch provenance {
	case "user_requested", "operator_curated":
		return redaction.TrustExplicit
	default:
		return redaction.TrustInferred
	}
}

// traceLogMiddleware injects the OTel trace ID into the logging context.
func traceLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			ctx = logctx.WithTraceID(ctx, sc.TraceID().String())
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// startHTTPServer starts an HTTP server in a background goroutine.
func startHTTPServer(log logr.Logger, name, addr string, srv *http.Server) {
	go func() {
		log.Info("starting server", "server", name, "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "server error", "server", name)
		}
	}()
}

// embeddingProviderAdapter adapts a PromptKit EmbeddingProvider to Omnia's
// memory.EmbeddingProvider interface.
type embeddingProviderAdapter struct {
	inner pkproviders.EmbeddingProvider
}

func (a *embeddingProviderAdapter) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := a.inner.Embed(ctx, pkproviders.EmbeddingRequest{Texts: texts})
	if err != nil {
		return nil, err
	}
	return resp.Embeddings, nil
}

func (a *embeddingProviderAdapter) Dimensions() int {
	return a.inner.EmbeddingDimensions()
}

// startRetentionWorkerWithLoader spawns the composite retention worker
// using the supplied loader. No-op when loader is nil so callers can
// share one loader between the worker and the retrieval ranker.
func startRetentionWorkerWithLoader(ctx context.Context, store *memory.PostgresMemoryStore, loader memory.PolicyLoader, log logr.Logger) {
	if loader == nil {
		return
	}
	worker := memory.NewRetentionWorker(store, loader, log)
	go worker.Run(ctx)
}

// buildRetentionPolicyLoader returns the PolicyLoader the composite
// retention worker should use. Prefers a K8s-backed loader; falls
// back to a static policy synthesised from RETENTION_INTERVAL so
// deployments pre-dating the CRD keep working. Returns nil when
// neither source yields a useful policy, disabling the worker.
func buildRetentionPolicyLoader(legacyInterval, workspace, serviceGroup string, log logr.Logger) memory.PolicyLoader {
	kubeConfig, err := rest.InClusterConfig()
	if err == nil {
		scheme := k8sruntime.NewScheme()
		utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
		c, clientErr := client.New(kubeConfig, client.Options{Scheme: scheme})
		if clientErr == nil {
			log.Info("retention policy loader enabled",
				"source", "MemoryPolicy CRD", "workspace", workspace, "serviceGroup", serviceGroup)
			return memory.NewK8sPolicyLoader(c, log, workspace, serviceGroup)
		}
		log.Error(clientErr, "k8s client creation failed, falling back to legacy interval")
	} else {
		log.V(1).Info("no in-cluster kubeconfig", "error", err.Error())
	}
	if legacyInterval == "" {
		log.Info("retention worker disabled", "reason", "no policy source")
		return nil
	}
	d, err := time.ParseDuration(legacyInterval)
	if err != nil || d <= 0 {
		log.Error(err, "invalid RETENTION_INTERVAL, retention worker disabled", "value", legacyInterval)
		return nil
	}
	log.Info("retention policy loader enabled",
		"source", "legacy RETENTION_INTERVAL",
		"interval", d.String())
	return &memory.StaticPolicyLoader{Policy: memory.LegacyIntervalPolicy(d)}
}

// createEmbeddingService reads a Provider CRD by name and creates an
// EmbeddingService with the appropriate PromptKit embedding provider.
// Returns nil if the provider can't be resolved (logs the error).
func createEmbeddingService(ctx context.Context, providerName string, store *memory.PostgresMemoryStore, log logr.Logger) *memory.EmbeddingService {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Info("embedding service skipped", "reason", "no in-cluster kubeconfig")
		return nil
	}

	scheme := k8sruntime.NewScheme()
	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
	// Secrets live in core/v1 — register so we can read embedding API keys.
	utilruntime.Must(corev1.AddToScheme(scheme))
	k8sClient, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "embedding service skipped", "reason", "k8s client creation failed")
		return nil
	}

	// Read the Provider CRD from the configured namespace.
	var provider omniav1alpha1.Provider
	ns := os.Getenv("EMBEDDING_PROVIDER_NAMESPACE")
	if ns == "" {
		ns = os.Getenv("POD_NAMESPACE")
	}
	if ns == "" {
		ns = "omnia-system"
	}
	key := client.ObjectKey{Namespace: ns, Name: providerName}
	if err := k8sClient.Get(ctx, key, &provider); err != nil {
		log.Error(err, "embedding provider CRD not found", "name", providerName, "namespace", ns)
		return nil
	}

	embeddingProvider, err := createEmbeddingProviderFromCRD(ctx, k8sClient, &provider, ns, log)
	if err != nil {
		log.Error(err, "failed to create embedding provider", "name", providerName, "type", provider.Spec.Type)
		return nil
	}

	adapter := &embeddingProviderAdapter{inner: embeddingProvider}
	svc := memory.NewEmbeddingService(store, adapter, log).WithModelName(providerName)
	log.Info("embedding service enabled",
		"provider", providerName,
		"type", provider.Spec.Type,
		"model", provider.Spec.Model,
	)
	return svc
}

// createEmbeddingProviderFromCRD translates an Omnia Provider CRD into a
// PromptKit EmbeddingProviderSpec and hands construction off to PromptKit's
// shared factory. We stay out of per-provider option-building entirely —
// the per-provider factories registered in pkproviders own that.
//
// The only provider-specific concern that lives on the Omnia side is
// reading the API key from the Secret referenced by the CRD's
// credential.secretRef; PromptKit's resolver fallbacks (env, file) are
// not appropriate here because the memory-api pod doesn't carry per-
// provider env vars.
func createEmbeddingProviderFromCRD(
	ctx context.Context,
	c client.Client,
	provider *omniav1alpha1.Provider,
	namespace string,
	log logr.Logger,
) (pkproviders.EmbeddingProvider, error) {
	cred, err := embeddingCredentialForCRD(ctx, c, provider, namespace)
	if err != nil {
		return nil, err
	}
	spec := pkproviders.EmbeddingProviderSpec{
		ID:         provider.Name,
		Type:       string(provider.Spec.Type),
		Model:      provider.Spec.Model,
		BaseURL:    provider.Spec.BaseURL,
		Credential: cred,
	}
	p, err := pkproviders.CreateEmbeddingProviderFromSpec(spec)
	if err != nil {
		return nil, fmt.Errorf("create embedding provider %q: %w", provider.Name, err)
	}
	log.V(1).Info("created embedding provider",
		"name", provider.Name,
		"type", provider.Spec.Type,
		"model", provider.Spec.Model,
	)
	return p, nil
}

// embeddingCredentialForCRD resolves a CRD's credential block into a
// PromptKit Credential. Returns nil credential for keyless providers
// (today: ollama). Errors surface so a misconfiguration trips boot
// loudly instead of silently disabling embeddings.
func embeddingCredentialForCRD(
	ctx context.Context,
	c client.Client,
	provider *omniav1alpha1.Provider,
	namespace string,
) (credentials.Credential, error) {
	if provider.Spec.Type == omniav1alpha1.ProviderTypeOllama {
		return nil, nil
	}
	ref := omniak8s.EffectiveSecretRef(provider)
	if ref == nil {
		return nil, fmt.Errorf("provider %q has no credential.secretRef — required for type %s", provider.Name, provider.Spec.Type)
	}
	secret, err := omniak8s.GetSecret(ctx, c, ref.Name, namespace)
	if err != nil {
		return nil, fmt.Errorf("get embedding api key Secret %q: %w", ref.Name, err)
	}
	keyName := omniak8s.DetermineSecretKey(ref, provider.Spec.Type)
	val, ok := secret.Data[keyName]
	if !ok || len(val) == 0 {
		return nil, fmt.Errorf("secret %q missing key %q for embedding provider %s", ref.Name, keyName, provider.Spec.Type)
	}
	return credentials.NewAPIKeyCredential(string(val)), nil
}

// shutdownServers gracefully stops all HTTP servers with a 30-second timeout.
func shutdownServers(log logr.Logger, servers ...*http.Server) {
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	for _, srv := range servers {
		if srv == nil {
			continue
		}
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Error(err, "server shutdown error", "addr", srv.Addr)
		}
	}
}

// Pool configuration defaults.
const (
	defaultMaxConns        = 50
	defaultMinConns        = 5
	defaultMaxConnLifetime = time.Hour
	defaultMaxConnIdleTime = 30 * time.Minute
)

// initPool creates and returns a pgxpool connection pool with configured limits.
func initPool(ctx context.Context, connStr string) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("parsing postgres connection string: %w", err)
	}

	poolCfg.MaxConns = envInt32("PG_MAX_CONNS", defaultMaxConns)
	poolCfg.MinConns = envInt32("PG_MIN_CONNS", defaultMinConns)
	poolCfg.MaxConnLifetime = envDuration("PG_MAX_CONN_LIFETIME", defaultMaxConnLifetime)
	poolCfg.MaxConnIdleTime = envDuration("PG_MAX_CONN_IDLE_TIME", defaultMaxConnIdleTime)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("creating postgres pool: %w", err)
	}
	return pool, nil
}

// envInt32 reads an environment variable as int32, returning def on missing/invalid values.
func envInt32(key string, def int32) int32 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return def
	}
	return int32(n)
}

// envDuration reads an environment variable as a time.Duration, returning def on missing/invalid.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// runMigrations applies database schema migrations.
func runMigrations(connStr string, log logr.Logger) error {
	migrator, err := memorypg.NewMigrator(connStr, log)
	if err != nil {
		return fmt.Errorf("creating migrator: %w", err)
	}
	if err := migrator.Up(); err != nil {
		_ = migrator.Close()
		return fmt.Errorf("running migrations: %w", err)
	}
	_ = migrator.Close()
	return nil
}

// newMetricsServer creates a dedicated HTTP server for Prometheus metrics.
func newMetricsServer(addr string) *http.Server {
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", promhttp.Handler())
	return &http.Server{Addr: addr, Handler: metricsMux}
}

// newHealthServer creates an HTTP server for health and readiness probes.
func newHealthServer(addr string, pool *pgxpool.Pool) *http.Server {
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	healthMux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(r.Context()); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("postgres unavailable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return &http.Server{Addr: addr, Handler: healthMux}
}
