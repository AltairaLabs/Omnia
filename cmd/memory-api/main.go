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
	// the per-provider option-builder business. Vendors must match the
	// (embedding × vendor) row in api/v1alpha1.ProviderSpec CEL.
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/gemini"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/ollama"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/openai"
	_ "github.com/AltairaLabs/PromptKit/runtime/providers/voyageai"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eeaudit "github.com/altairalabs/omnia/ee/pkg/audit"
	eelicense "github.com/altairalabs/omnia/ee/pkg/license"
	eememory "github.com/altairalabs/omnia/ee/pkg/memory"
	"github.com/altairalabs/omnia/ee/pkg/memory/consolidation"
	eeprojection "github.com/altairalabs/omnia/ee/pkg/memory/projection"
	"github.com/altairalabs/omnia/ee/pkg/memory/projectionworker"
	eemetrics "github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/ee/pkg/privacy/classify"
	"github.com/altairalabs/omnia/ee/pkg/privacy/httpclient"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
	"github.com/altairalabs/omnia/internal/memory"
	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
	memorypg "github.com/altairalabs/omnia/internal/memory/postgres"
	"github.com/altairalabs/omnia/internal/serviceauth"
	sessionapi "github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/internal/tracing"
	omniak8s "github.com/altairalabs/omnia/pkg/k8s"
	"github.com/altairalabs/omnia/pkg/logctx"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
)

// auditEventSink is the subset of ee/pkg/audit.Logger the adapter needs,
// extracted so auditLoggerAdapter is unit-testable with a recording fake.
type auditEventSink interface {
	LogEvent(ctx context.Context, entry *sessionapi.AuditEntry)
}

// auditLoggerAdapter adapts ee/pkg/audit.Logger to memoryapi.MemoryAuditLogger.
// It converts MemoryAuditEntry fields into the session/api.AuditEntry shape,
// placing memory-specific fields (MemoryID, Kind) in Metadata.
type auditLoggerAdapter struct {
	inner auditEventSink
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
	// SEC-6: sessionapi.AuditEntry has no user field, so carry the subject as a
	// hashed metadata key — otherwise the audit trail can't answer "who
	// accessed/deleted user X's memories". Hashed per the project's PII rule.
	if entry.UserID != "" {
		meta["userHash"] = logging.HashID(entry.UserID)
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
	redisURL              string // --redis-url, env REDIS_URL or OMNIA_MEMORY_REDIS_URL
	cacheTTL              string // env: MEMORY_CACHE_TTL, e.g. "5m"; "" or "0" disables
	enterprise            bool
	operatorAPIURL        string // --operator-api-url, env OPERATOR_API_URL; the operator/arena-controller license endpoint, used to nag when unlicensed.
	tracingEnabled        bool
	tracingEndpoint       string
	tracingSample         float64
	tracingInsecure       bool
	embeddingProviderName string // name of the Provider CRD for embeddings
	sessionAPIURL         string // base URL of session-api for provider_usage emit (empty disables)
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

	// --- Consolidation v1 ---
	consolidationInterval string // env: CONSOLIDATION_INTERVAL, e.g. "6h". Empty disables the worker.

	// --- Memory Galaxy v2 pre-render ---
	projectionInterval string // env: PROJECTION_INTERVAL, e.g. "30s". Empty disables the worker.

	// --- Embedding-pipeline observability ---
	metricsCollectInterval string // env: METRICS_COLLECT_INTERVAL, e.g. "60s". Empty disables the collector.

	// --- Ingestion ---
	ingestStrategy     string // env: INGEST_STRATEGY; chunk (default) | summary | summaryThenChunk
	ingestSummarizer   string // env: INGEST_SUMMARIZER; extractive (default) | agent
	ingestChunkSize    int    // env: INGEST_CHUNK_SIZE; default 200
	ingestChunkOverlap int    // env: INGEST_CHUNK_OVERLAP; default 40
	ingestQueueDir     string // env: INGEST_QUEUE_DIR; empty disables the agent path

	// ServiceAccount auth (opt-in). When authEnabled is true, the JSON API
	// requires a Kubernetes ServiceAccount bearer token whose TokenReview
	// subject is either in authAllowedSubjects (exact match) or whose
	// ServiceAccount namespace is in authAllowedNamespaces. Defaults to false
	// so existing deployments see zero behavior change until an operator
	// explicitly turns it on.
	authEnabled           bool
	authAllowedSubjects   string // comma-separated SA subjects
	authAllowedNamespaces string // comma-separated trusted namespaces
	authAudiences         string // comma-separated, optional
}

func parseFlags() *flags {
	f := &flags{}
	flag.StringVar(&f.apiAddr, "api-addr", ":8080", "API server listen address")
	flag.StringVar(&f.healthAddr, "health-addr", ":8081", "Health probe listen address")
	flag.StringVar(&f.metricsAddr, "metrics-addr", ":9090", "Metrics server listen address")
	flag.StringVar(&f.postgresConn, "postgres-conn", "", "Postgres connection string")
	flag.StringVar(&f.redisURL, "redis-url", "", "Redis connection URL (redis:// or rediss://). When set, the same client is reused for both the read-through cache and the event publisher. Empty disables both.")
	flag.StringVar(&f.cacheTTL, "cache-ttl", "5m", "TTL for the Redis read-through cache (Retrieve/List). Set to 0 or empty to disable caching even when --redis-url is configured.")
	flag.BoolVar(&f.enterprise, "enterprise", false, "Enable enterprise features (audit logging)")
	flag.StringVar(&f.operatorAPIURL, "operator-api-url", "", "Base URL of the operator/arena-controller license endpoint (e.g. http://omnia-arena-controller.omnia-system:8082). When enterprise features run without a valid license, memory-api logs a startup reminder. Never blocks.")
	flag.BoolVar(&f.tracingEnabled, "tracing-enabled", false, "Enable OpenTelemetry tracing")
	flag.StringVar(&f.tracingEndpoint, "tracing-endpoint", "", "OTel collector endpoint")
	flag.Float64Var(&f.tracingSample, "tracing-sample", 0, "Tracing sample rate (0.0-1.0)")
	flag.BoolVar(&f.tracingInsecure, "tracing-insecure", false, "Use insecure gRPC for tracing")
	flag.StringVar(&f.embeddingProviderName, "embedding-provider", "", "Name of the Provider CRD to use for embeddings")
	flag.StringVar(&f.sessionAPIURL, "session-api-url", "", "Base URL of session-api; when set, embedding spend is emitted to provider_usage")
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
	flag.StringVar(&f.consolidationInterval, "consolidation-interval", "", "Schedule-evaluation (poll) interval for the consolidation worker, e.g. 1m. Each axis fires per its MemoryPolicy cron schedule; this controls how often schedules are checked. Empty disables the worker.")
	flag.StringVar(&f.projectionInterval, "projection-interval", "", "Poll interval for the Memory Galaxy pre-render worker, e.g. 30s. Empty disables the worker.")
	flag.StringVar(&f.metricsCollectInterval, "metrics-collect-interval", "60s", "Refresh interval for embedding-pipeline gauges (coverage, re-embed backlog), e.g. 60s. Empty disables the collector.")
	flag.StringVar(&f.ingestStrategy, "ingest-strategy", ingestion.StrategyChunk, "Ingestion strategy: chunk | summary | summaryThenChunk (INGEST_STRATEGY).")
	flag.StringVar(&f.ingestSummarizer, "ingest-summarizer", ingestion.SummarizerExtractive, "Summarizer backend for summary strategies: extractive | agent (INGEST_SUMMARIZER).")
	flag.IntVar(&f.ingestChunkSize, "ingest-chunk-size", 200, "Word-window size for RAG chunking (INGEST_CHUNK_SIZE).")
	flag.IntVar(&f.ingestChunkOverlap, "ingest-chunk-overlap", 40, "Word overlap between adjacent chunks (INGEST_CHUNK_OVERLAP).")
	flag.StringVar(&f.ingestQueueDir, "ingest-queue-dir", "", "Directory for the agent summary work-queue. Empty disables the agent path (INGEST_QUEUE_DIR).")
	flag.BoolVar(&f.authEnabled, "auth-enabled", false,
		"Require Kubernetes ServiceAccount bearer-token auth on the JSON API (opt-in)")
	flag.StringVar(&f.authAllowedSubjects, "auth-allowed-subjects", "",
		"Comma-separated allowed ServiceAccount subjects, exact-matched "+
			"(e.g. system:serviceaccount:omnia-system:omnia-dashboard). "+
			"Use for cross-namespace callers")
	flag.StringVar(&f.authAllowedNamespaces, "auth-allowed-namespaces", "",
		"Comma-separated trusted namespaces: any ServiceAccount in one of these "+
			"namespaces is allowed (covers per-AgentRuntime facade SAs, session-api, "+
			"eval-worker — the in-workspace callers)")
	flag.StringVar(&f.authAudiences, "auth-audiences", "",
		"Comma-separated audiences for audience-bound projected tokens (optional; empty = default)")
	flag.Parse()

	f.applyEnvFallbacks()
	return f
}

// applyEnvFallbacks applies environment variable overrides to flag defaults.
func (f *flags) applyEnvFallbacks() {
	envFallback(&f.postgresConn, "", "POSTGRES_CONN")
	envFallback(&f.redisURL, "", "REDIS_URL")
	envFallback(&f.redisURL, "", "OMNIA_MEMORY_REDIS_URL")
	envFallback(&f.cacheTTL, "5m", "MEMORY_CACHE_TTL")
	envFallback(&f.apiAddr, ":8080", "API_ADDR")
	envFallback(&f.healthAddr, ":8081", "HEALTH_ADDR")
	envFallback(&f.metricsAddr, ":9090", "METRICS_ADDR")

	envBoolFallback(&f.enterprise, "ENTERPRISE_ENABLED")
	envFallback(&f.operatorAPIURL, "", "OPERATOR_API_URL")
	envBoolFallback(&f.tracingEnabled, "TRACING_ENABLED")
	envBoolFallback(&f.tracingInsecure, "TRACING_INSECURE")
	envFallback(&f.tracingEndpoint, "", "TRACING_ENDPOINT")
	envFallback(&f.embeddingProviderName, "", "EMBEDDING_PROVIDER")
	envFallback(&f.sessionAPIURL, "", "SESSION_API_URL")
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
	envFallback(&f.consolidationInterval, "", "CONSOLIDATION_INTERVAL")
	envFallback(&f.projectionInterval, "", "PROJECTION_INTERVAL")
	envFallback(&f.metricsCollectInterval, "60s", "METRICS_COLLECT_INTERVAL")
	envFallback(&f.ingestStrategy, ingestion.StrategyChunk, "INGEST_STRATEGY")
	envFallback(&f.ingestSummarizer, ingestion.SummarizerExtractive, "INGEST_SUMMARIZER")
	envFallback(&f.ingestQueueDir, "", "INGEST_QUEUE_DIR")
	if v := os.Getenv("INGEST_CHUNK_SIZE"); v != "" && f.ingestChunkSize == 200 {
		if n, err := strconv.Atoi(v); err == nil {
			f.ingestChunkSize = n
		}
	}
	if v := os.Getenv("INGEST_CHUNK_OVERLAP"); v != "" && f.ingestChunkOverlap == 40 {
		if n, err := strconv.Atoi(v); err == nil {
			f.ingestChunkOverlap = n
		}
	}

	// ServiceAccount auth — reads the SHARED SESSION_API_AUTH_* config the
	// operator stamps onto data-plane services via applySessionAPIServerAuthEnv
	// (internal/controller/service_auth.go), same as session-api and privacy-api.
	envBoolFallback(&f.authEnabled, "SESSION_API_AUTH_ENABLED")
	envFallback(&f.authAllowedSubjects, "", "SESSION_API_AUTH_ALLOWED_SUBJECTS")
	envFallback(&f.authAllowedNamespaces, "", "SESSION_API_AUTH_ALLOWED_NAMESPACES")
	envFallback(&f.authAudiences, "", "SESSION_API_AUTH_AUDIENCES")
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
		Interval:  interval,
		BatchSize: f.reembedBatchSize,
		// MAINT-5: read the identifier the EmbeddingService actually stamps
		// (provider/model), not the bare provider name, so the worker's
		// staleness comparison matches what was written.
		CurrentModel: embeddingSvc.ModelName(),
	}, true
}

func main() {
	if err := run(); err != nil {
		if log, syncLog, lerr := logging.NewLogger(); lerr == nil {
			log.Error(err, "startup failed")
			syncLog()
		} else {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}
}

// nagLicenseAtStartup fetches the operator license once at startup and logs a
// reminder when this enterprise deployment isn't backed by a valid license.
// It never blocks — memory-api's features keep working regardless. The
// "startup license check" line is always emitted so the wiring is observable
// even when the license is valid and the nag stays silent.
func nagLicenseAtStartup(ctx context.Context, f *flags, log logr.Logger) {
	if !f.enterprise {
		return
	}
	if f.operatorAPIURL == "" {
		log.Info("startup license check skipped", "reason", "no OPERATOR_API_URL configured")
		return
	}

	licClient := eelicense.NewClient(f.operatorAPIURL, eelicense.WithClientLogger(log.WithName("license")))
	lic, err := licClient.Refresh(ctx)
	if err != nil {
		// Operator unreachable — degrade to the open-core fallback and nag.
		lic = licClient.License()
	}
	log.Info("startup license check",
		"valid", lic.IsValidEnterprise(),
		"tier", lic.Tier,
		"licenseID", lic.ID,
		"operatorURL", f.operatorAPIURL,
	)
	eelicense.NagIfUnlicensed(lic, log)
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

	// --- License awareness nag (#1682) ---
	// When enterprise features are enabled without a valid license, log a
	// one-time reminder. Never blocks; features keep working.
	nagLicenseAtStartup(ctx, f, log)

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
	// pgStore is the raw Postgres-backed store. Workers (compaction,
	// retention, tombstone GC, re-embed) call concrete methods on this
	// type, so they keep using the unwrapped value. The HTTP API gets
	// the (possibly cache-fronted) Store interface — see resolveAPIStore
	// below.
	pgStore := memory.NewPostgresMemoryStore(pool)
	defer pgStore.Close()
	if f.enterprise {
		// Wire the enterprise gonum/t-SNE projector so memory.Render computes
		// Memory Galaxy layouts. OSS leaves it nil (Render returns a clear
		// "projection unavailable" error), keeping internal/memory free of an
		// ee import (#1669).
		pgStore.SetProjector(eeprojection.GonumProjector{})
	}

	// Build the shared Redis client (if configured) before either the
	// cache wrapper or the event publisher. One client, one TCP pool —
	// keeps connection accounting honest and lets ops see "is Redis up"
	// from a single set of metrics rather than two.
	redisClient, redisErr := buildRedisClient(f.redisURL)
	if redisErr != nil {
		return fmt.Errorf("redis URL: %w", redisErr)
	}
	if redisClient != nil {
		defer func() { _ = redisClient.Close() }()
	}

	apiStore, cacheEnabled := resolveAPIStore(pgStore, redisClient, f.cacheTTL, log)
	if cacheEnabled {
		log.Info("memory store cache enabled", "ttl", f.cacheTTL)
	}

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
	// Register the composite-worker metrics before starting it, otherwise the
	// worker's nil-tolerant emit path silently no-ops and the soft/hard-delete
	// counters + run duration/error metrics never reach Prometheus.
	if err := memory.RegisterRetentionMetrics(prometheus.DefaultRegisterer); err != nil {
		log.Error(err, "memory retention metrics registration failed")
	}
	startRetentionWorkerWithLoader(ctx, pgStore, policyLoader, log)

	// --- Compaction worker ---
	// Temporal summarization of old memories. Uses NoopSummarizer by default —
	// memory growth is still bounded because the worker supersedes originals,
	// but summaries aren't informative until a real LLM summarizer is wired.
	if compactionOpts, enabled := f.compactionWorkerOptions(log, pgStore); enabled {
		worker := memory.NewCompactionWorker(pgStore, memory.NoopSummarizer{}, compactionOpts, log)
		go worker.Run(ctx)
		log.Info("compaction worker started",
			"interval", compactionOpts.Interval,
			"age", compactionOpts.Age,
			"summarizer", "noop",
		)
	}

	// --- Consolidation worker construction is deferred to after the
	// audit logger is built (so the worker can share it). The worker
	// is started near the bottom of run() once auditLogger exists.

	// --- Embedding service ---
	var embeddingSvc *memory.EmbeddingService
	if f.embeddingProviderName != "" {
		// Embedding spend has no session, so it goes to session-api's
		// provider_usage table directly. The emitter is best-effort and nil
		// when no URL resolves (the Prometheus counter still works).
		usageEmitter := newSessionUsageEmitter(
			resolveSessionAPIURL(ctx, f.sessionAPIURL, f.workspace, f.serviceGroup, log), log)
		embeddingSvc = createEmbeddingService(ctx, f.embeddingProviderName, f.workspace, detectNamespace(), pgStore, usageEmitter, log)
	}

	// --- Embedding schema reconcile ---
	// The embedding vector columns are application-managed (omitted from the
	// migrations) so their dimension can track the configured embedding
	// provider (#1309). Bring them to the resolved dimension now — before the
	// re-embed / consolidation workers and the HTTP server touch them. The
	// columns must exist even without a provider (consolidation dup-detection
	// reads memory_entities.embedding), so resolveEmbeddingDim falls back to
	// the historical default.
	embeddingDim := resolveEmbeddingDim(embeddingSvc)
	if err := memorypg.EnsureEmbeddingSchema(ctx, pool, embeddingDim, log); err != nil {
		return fmt.Errorf("ensure embedding schema: %w", err)
	}
	log.Info("embedding schema ensured",
		"dimensions", embeddingDim, "providerConfigured", embeddingSvc != nil)

	// --- Tombstone GC worker ---
	// Hard-deletes old superseded observations on long supersession
	// chains, keeping the most recent K per chain for audit. Bounds
	// storage growth without losing the agent-visible "this got
	// updated" history.
	if tombstoneOpts, enabled := f.tombstoneWorkerOptions(log, pgStore); enabled {
		worker := memory.NewTombstoneWorker(pgStore, tombstoneOpts, log)
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
		worker := memory.NewReembedWorker(pgStore, embeddingSvc.Provider(), reembedOpts, log)
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
	// Reuses the shared Redis client built above so we don't create a
	// second TCP pool against the same target.
	var eventPublisher memoryapi.MemoryEventPublisher
	if redisClient != nil {
		eventPublisher = memoryapi.NewRedisMemoryEventPublisher(redisClient, log)
		log.Info("memory event publisher enabled")
	}

	// --- Audit logger (shared by memory CRUD audit + consolidation
	// audit) ---
	auditLogger, auditClose := buildAuditLogger(f.enterprise, pool, log)
	defer func() {
		if auditClose != nil {
			_ = auditClose()
		}
	}()

	// --- Audit drain-forwarder (#1673) ---
	// Ships locally-recorded enforcement audit rows to the privacy-api central
	// audit hub so enforcement-stats can serve them. Runs only when audit
	// logging is on and a privacy-api URL resolves.
	if fwd := buildAuditForwarder(auditLogger, pool,
		resolvePrivacyURL(ctx, f.workspace, f.serviceGroup, log),
		prometheus.DefaultRegisterer, log); fwd != nil {
		go fwd.Run(ctx)
		log.Info("audit forwarder started", "sourceService", auditSourceService)
	} else if auditLogger != nil {
		log.Info("audit forwarder skipped", "reason", "no privacy URL")
	}

	// --- ServiceAccount auth (opt-in) ---
	reviewer, allowedSubjects, allowedNamespaces, err := buildServiceAuth(f, log)
	if err != nil {
		return err
	}

	// --- Build API mux ---
	apiMux, cleanup := buildAPIMux(ctx, apiStore, embeddingSvc, svcCfg, eventPublisher, f.enterprise, pool, policyLoader, auditLogger, log, buildIngestOptions(f, log), f.workspace, f.serviceGroup, pgStore, reviewer, allowedSubjects, allowedNamespaces)
	defer cleanup()

	// --- Consolidation worker ---
	// LLM-driven memory consolidation. Replaces the NoopSummarizer path with
	// a function-mode AgentRuntime that emits typed actions against the
	// memory store. See docs/local-backlog/2026-05-22-memory-consolidation-design.md.
	// Disabled by default — set --consolidation-interval to enable.
	if cw := buildConsolidationWorker(ctx, f, pgStore, auditLogger, log); cw != nil {
		go cw.Run(ctx)
		log.Info("consolidation worker started",
			"interval", f.consolidationInterval,
			"audit", auditLogger != nil,
		)
	}

	// --- Projection pre-render worker (Memory Galaxy v2) ---
	// Pre-renders the workspace-wide galaxy layout into memory_projections so
	// the projection endpoint serves it instantly. Disabled by default — set
	// --projection-interval to enable.
	if pw := buildProjectionWorker(f, pgStore, prometheus.DefaultRegisterer, log); pw != nil {
		go pw.Run(ctx)
		log.Info("projection worker started", "interval", f.projectionInterval)
	}

	// --- Embedding-pipeline metrics collector (#1442) ---
	// Periodically refreshes per-workspace embedding coverage + re-embed backlog
	// gauges so a silently-degraded semantic index is observable. On by default.
	if mc := buildEmbeddingMetricsCollector(f, pgStore, embeddingSvc, prometheus.DefaultRegisterer, log); mc != nil {
		go mc.Run(ctx)
		log.Info("embedding metrics collector started", "interval", f.metricsCollectInterval)
	}

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
		"authEnabled", f.authEnabled,
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

// buildAuditLogger constructs the shared ee/pkg/audit Logger when
// --enterprise is set. Returns nil when disabled or when the EE wiring
// isn't applicable. The caller owns the Close hook.
func buildAuditLogger(enterprise bool, pool *pgxpool.Pool, log logr.Logger) (*eeaudit.Logger, func() error) {
	if !enterprise {
		return nil, nil
	}
	auditMetrics := eemetrics.NewAuditMetrics()
	logger := eeaudit.NewLogger(pool, log, auditMetrics, eeaudit.LoggerConfig{})
	log.Info("memory audit logging enabled")
	return logger, logger.Close
}

// auditSourceService identifies this service in forwarded audit events; the
// privacy-api hub keys idempotency on (source_service, source_id).
const auditSourceService = "memory-api"

// resolveSessionAPIURL resolves the session-api base URL that embedding-spend
// usage records are emitted to. memory-api makes embedding provider calls that
// cost money but have no session, so the spend goes to session-api's
// provider_usage table directly.
//
// An explicit --session-api-url flag wins, for operators pointing at an
// out-of-band endpoint. Otherwise it resolves from this workspace's service
// group, the same way the privacy-api URL does — memory-api holds a client, so
// it has no business being told an endpoint its own workspace already knows.
//
// Returns "" when nothing resolves, which makes the emitter a no-op.
func resolveSessionAPIURL(
	ctx context.Context, flagValue, workspace, serviceGroup string, log logr.Logger,
) string {
	if flagValue != "" {
		return flagValue
	}
	if workspace == "" || serviceGroup == "" {
		return ""
	}
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.V(1).Info("embedding usage emitter disabled", "reason", "no in-cluster kubeconfig")
		return ""
	}
	c, err := client.New(kubeConfig, client.Options{Scheme: newPrivacyMiddlewareScheme()})
	if err != nil {
		log.V(1).Info("embedding usage emitter disabled", "reason", "k8s client creation failed")
		return ""
	}
	sessionURL, err := servicediscovery.NewResolver(c).SessionURL(ctx, workspace, serviceGroup)
	if err != nil {
		log.V(1).Info("embedding usage emitter disabled",
			"reason", "session-api URL not resolved from workspace",
			"workspace", workspace, "serviceGroup", serviceGroup)
		return ""
	}
	return sessionURL
}

// resolvePrivacyURL resolves the privacy-api base URL the audit forwarder ships
// to, from Workspace CRD status.privacyURL via servicediscovery, reusing the
// same resolution the privacy middleware uses for opt-out.
//
// There is no PRIVACY_API_URL override. privacy-api is per-workspace like every
// other service, so the workspace already knows its endpoint and a second
// source of truth only invites drift.
//
// Returns "" when no URL can be resolved, in which case the forwarder is skipped.
func resolvePrivacyURL(ctx context.Context, workspace, serviceGroup string, log logr.Logger) string {
	if workspace == "" || serviceGroup == "" {
		return ""
	}
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.V(1).Info("audit forwarder URL not resolved", "reason", reasonNoInClusterKubeconfig)
		return ""
	}
	k8sClient, err := client.New(kubeConfig, client.Options{Scheme: newPrivacyMiddlewareScheme()})
	if err != nil {
		log.Error(err, "audit forwarder URL not resolved", "reason", "k8s client creation failed")
		return ""
	}
	urls, err := servicediscovery.NewResolver(k8sClient).ResolveByWorkspaceName(ctx, workspace, serviceGroup)
	if err != nil {
		log.V(1).Info("audit forwarder URL not resolved from workspace",
			"workspace", workspace, "serviceGroup", serviceGroup, "error", err.Error())
		return ""
	}
	return urls.PrivacyURL
}

// buildAuditForwarder constructs the audit drain-forwarder that ships this
// service's local audit_log to the privacy-api hub (#1673). It returns nil when
// forwarding cannot run (no audit logger, or no privacy URL), so the caller can
// skip starting the goroutine. The ServiceAccount token source authenticates the
// ingest POSTs the same way the consent push does.
func buildAuditForwarder(
	auditLogger *eeaudit.Logger,
	pool *pgxpool.Pool,
	privacyURL string,
	reg prometheus.Registerer,
	log logr.Logger,
) *eeaudit.Forwarder {
	if auditLogger == nil || pool == nil || privacyURL == "" {
		return nil
	}
	ts := serviceauth.NewTokenSource("", 0)
	return eeaudit.NewForwarder(pool, privacyURL, auditSourceService, ts, 0, 0, reg, log)
}

// buildAPIMux assembles the HTTP handler with all memory-api routes, wrapped
// with rate limiting, privacy (enterprise), metrics, and tracing middleware.
// Returns the handler and a cleanup function. The auditLogger is constructed
// upstream so it can be shared with the consolidation worker.
// buildIngestOptions assembles the ingestion wiring from flags. A non-empty
// --ingest-queue-dir constructs the filesystem summary queue; on failure the
// agent path is disabled (queue=nil -> extractive fallback) rather than
// crashing memory-api.
func buildIngestOptions(f *flags, log logr.Logger) memoryapi.IngestOptions {
	opts := memoryapi.IngestOptions{Fallback: ingestion.Config{
		Strategy:     f.ingestStrategy,
		Summarizer:   f.ingestSummarizer,
		ChunkSize:    f.ingestChunkSize,
		ChunkOverlap: f.ingestChunkOverlap,
	}}
	if f.ingestQueueDir == "" {
		return opts
	}
	queue, err := ingestion.NewFileSummaryQueue(f.ingestQueueDir)
	if err != nil {
		log.Error(err, "summary queue disabled", "dir", f.ingestQueueDir)
		return opts
	}
	opts.Queue = queue
	log.Info("agent summary queue enabled", "dir", f.ingestQueueDir)
	return opts
}

// buildServiceAuth constructs the ServiceAccount TokenReviewer and parses the
// allowlists when --auth-enabled is set. It returns (nil, nil, nil, nil) when
// auth is disabled (after logging a clear startup WARNING that the API is
// unauthenticated). It returns an error when auth is enabled but misconfigured
// (both the subject and namespace allowlists empty) or the TokenReviewer cannot
// be built.
//
// A caller is authorized when its TokenReview subject is an exact match in
// allowedSubjects OR its ServiceAccount namespace is in allowedNamespaces. The
// namespace allow is what lets in-workspace callers (per-AgentRuntime facade
// SAs, session-api, eval-worker) pass without enumerating every facade SA up
// front; cross-namespace callers (dashboard) stay on the exact-subject list.
//
// The memory-api ServiceAccount must have RBAC to create TokenReviews
// (`authentication.k8s.io/tokenreviews: create`). The ClusterRole ships in the
// Helm chart (session-api-tokenreview-clusterrole.yaml) and the operator binds
// each managed memory-api's effective ServiceAccount to it at reconcile time
// (#1730, #1817); without that binding the reviewer's TokenReview calls fail
// closed (401).
func buildServiceAuth(f *flags, log logr.Logger) (serviceauth.TokenReviewer, []string, []string, error) {
	if !f.authEnabled {
		log.Info("WARNING: memory-api JSON API is UNAUTHENTICATED " +
			"(auth-enabled=false); set --auth-enabled to require ServiceAccount tokens")
		return nil, nil, nil, nil
	}

	allowedSubjects := splitAndTrim(f.authAllowedSubjects)
	allowedNamespaces := splitAndTrim(f.authAllowedNamespaces)
	if len(allowedSubjects) == 0 && len(allowedNamespaces) == 0 {
		return nil, nil, nil, fmt.Errorf(
			"--auth-enabled is set but both --auth-allowed-subjects and " +
				"--auth-allowed-namespaces are empty; refusing to start " +
				"(would reject every caller — misconfiguration)")
	}

	// In-cluster rest config — same source the privacy middleware uses.
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("auth-enabled requires in-cluster config: %w", err)
	}

	audiences := splitAndTrim(f.authAudiences)
	reviewer, err := serviceauth.NewK8sTokenReviewer(cfg, audiences)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("building token reviewer: %w", err)
	}

	log.Info("ServiceAccount auth enabled",
		"allowedSubjects", allowedSubjects,
		"allowedNamespaces", allowedNamespaces,
		"audiences", audiences)
	return reviewer, allowedSubjects, allowedNamespaces, nil
}

// splitAndTrim splits a comma-separated string, trims whitespace from each
// element, and drops empty elements. Returns nil for an empty/whitespace input.
func splitAndTrim(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// buildAPIMux assembles the HTTP handler with all memory-api routes, wrapped
// with rate limiting, ServiceAccount auth, privacy (enterprise), metrics, and
// tracing middleware. Returns the handler and a cleanup function. The
// auditLogger is constructed upstream so it can be shared with the
// consolidation worker.
//
// reviewer, allowedSubjects and allowedNamespaces wire ServiceAccount auth: when
// reviewer is non-nil the JSON API requires a ServiceAccount bearer token whose
// subject is in allowedSubjects OR whose namespace is in allowedNamespaces
// (/healthz exempt). A nil reviewer leaves the API unauthenticated. These are
// parameters (rather than read from flags) so wiring tests can inject a fake
// reviewer.
func buildAPIMux(
	ctx context.Context,
	store memory.Store,
	embeddingSvc *memory.EmbeddingService,
	cfg memoryapi.MemoryServiceConfig,
	publisher memoryapi.MemoryEventPublisher,
	enterprise bool,
	pool *pgxpool.Pool,
	policyLoader memory.PolicyLoader,
	auditLogger *eeaudit.Logger,
	log logr.Logger,
	ingestOpts memoryapi.IngestOptions,
	workspace, serviceGroup string,
	consentPruner memory.ConsentEventPruner,
	reviewer serviceauth.TokenReviewer,
	allowedSubjects, allowedNamespaces []string,
) (http.Handler, func()) {
	httpMetrics := memoryapi.NewHTTPMetrics(nil)

	cfg.Enterprise = enterprise
	svc := memoryapi.NewMemoryService(store, embeddingSvc, cfg, log)
	if enterprise {
		svc.SetInstitutionalStore(eememory.NewInstitutionalStore(pool, log))
	}
	if publisher != nil {
		svc.SetEventPublisher(publisher)
	}
	if policyLoader != nil {
		// Retrieval consults the loader to build a per-tier ranker from
		// the workspace's bound MemoryPolicy.spec.tierPrecedence.
		svc.SetPolicyLoader(policyLoader)
		// Enterprise tier-ranking factories: the constructors live in ee
		// (policy -> weights/half-life); core consumes them via injected
		// factories so internal/memory/api no longer imports ee (#1669).
		svc.SetTierRankerFactory(eememory.NewTierRanker)
		svc.SetTierHalfLifeFactory(eememory.NewTierHalfLife)
	}
	if consentPruner != nil {
		// CE1: per-user/category consent-event prune endpoint.
		svc.SetConsentEventPruner(consentPruner)
	}

	// Wire ingestion: a flag-derived fallback Config + an optional async
	// summary queue. The effective per-request config is resolved from the
	// workspace's MemoryPolicy.spec.ingestion at /ingest time.
	svc.SetIngestion(ingestOpts.Fallback, ingestOpts.Queue)
	log.Info("ingestion configured",
		"strategy", ingestOpts.Fallback.Strategy,
		"summarizer", ingestOpts.Fallback.Summarizer,
		"chunkSize", ingestOpts.Fallback.ChunkSize,
		"chunkOverlap", ingestOpts.Fallback.ChunkOverlap,
		"agentQueue", ingestOpts.Queue != nil,
	)

	// Enterprise audit logging. Logger was constructed upstream so the
	// consolidation worker shares it; here we just register the
	// memory-CRUD adapter + the audit HTTP routes.
	var auditHandler *eeaudit.Handler
	if auditLogger != nil {
		svc.SetAuditLogger(&auditLoggerAdapter{inner: auditLogger})
		auditHandler = eeaudit.NewHandler(auditLogger, log)
	}

	handler := memoryapi.NewHandler(svc, log).
		WithEnterprise(enterprise).
		WithDimensionConsentRecorder(func(ctx context.Context, targetDim int, createdBy string) error {
			return memorypg.InsertDimensionChangeConsent(ctx, pool, targetDim, createdBy)
		})
	if enterprise {
		// Validate consent_category against the platform registry at write time.
		// privacy.CategoryInfo is the authoritative source; only registered
		// categories (memory:*, analytics:aggregate) are accepted.
		handler = handler.WithCategoryRegistration(func(category string) bool {
			_, valid := privacy.CategoryInfo(privacy.ConsentCategory(category))
			return valid
		})
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	if auditHandler != nil {
		auditHandler.RegisterMemoryRoutes(mux)
	}
	// AuditMiddleware always applied — populates request context with IP/UA.
	// The service only emits events when an audit logger is configured.
	apiHandler := memoryapi.AuditMiddleware(mux)

	// Enterprise privacy middleware (opt-out + PII redaction + classifier).
	if enterprise {
		apiHandler = wrapPrivacyMiddleware(ctx, apiHandler, embeddingSvc, auditLogger, workspace, serviceGroup, log)
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
		// auditLogger lifecycle is owned by the caller (lifted out so
		// the consolidation worker shares it).
	}

	// ServiceAccount auth runs after rate-limiting but around the
	// metrics/trace/handler chain. /healthz is exempt so liveness probes are
	// never gated. A nil reviewer makes this a pass-through (unauthenticated).
	authMW := serviceauth.RequireServiceAccount(reviewer, allowedSubjects, allowedNamespaces, "/healthz")
	return rlMiddleware(authMW(httpMetrics.MetricsMiddleware(traced))), cleanup
}

// wrapPrivacyMiddleware creates and wires the enterprise privacy middleware.
// When the K8s API is unreachable (e.g., in tests), the middleware is skipped
// and the original handler is returned unchanged.
func wrapPrivacyMiddleware(ctx context.Context, next http.Handler, embeddingSvc *memory.EmbeddingService, auditLogger *eeaudit.Logger, workspace, serviceGroup string, log logr.Logger) http.Handler {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Info("memory privacy middleware skipped", "reason", reasonNoInClusterKubeconfig)
		return next
	}

	scheme := newPrivacyMiddlewareScheme()
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

	prefStore := resolvePrivacyPrefStore(ctx, workspace, serviceGroup, k8sClient, log)
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
	// Make enforcement observable: emit an audit_log row when a write is
	// blocked by opt-out or mutated by PII redaction. Reuses the same
	// adapter the memory-CRUD audit path uses so both land in audit_log.
	if auditLogger != nil {
		mw.SetAuditLogger(&auditLoggerAdapter{inner: auditLogger})
	}
	log.Info("memory privacy middleware enabled")
	return mw.Wrap(next)
}

// privacyPrefStore is satisfied by both *httpclient.Client and the permissive
// no-op store returned by privacy.NewPermissivePreferencesStore. memory-api
// needs both PreferencesStore (for opt-out lookup) and ConsentSource (for
// consent-grant lookup inside ShouldRememberCategory) from the same value.
type privacyPrefStore interface {
	privacy.PreferencesStore
	privacy.ConsentSource
}

// resolvePrivacyPrefStore selects the PreferencesStore+ConsentSource implementation
// for the privacy middleware. Resolution order:
//
//  1. Workspace CRD lookup via servicediscovery — reads Workspace.status.privacyURL
//     when both workspace name and serviceGroup are non-empty and a k8s client is
//     available. There is no env override: privacy-api is per-workspace, so the
//     workspace is the only source of truth for its endpoint.
//  2. Permissive no-op store — when no URL can be resolved, opt-out is disabled
//     and all recording proceeds (fail-open).
func resolvePrivacyPrefStore(
	ctx context.Context,
	workspace, serviceGroup string,
	k8sClient client.Client,
	log logr.Logger,
) privacyPrefStore {
	if workspace != "" && serviceGroup != "" && k8sClient != nil {
		resolver := servicediscovery.NewResolver(k8sClient)
		urls, err := resolver.ResolveByWorkspaceName(ctx, workspace, serviceGroup)
		if err == nil && urls.PrivacyURL != "" {
			return httpclient.New(urls.PrivacyURL, log)
		}
		if err != nil {
			log.V(1).Info("privacy-api URL not resolved from workspace",
				"workspace", workspace, "serviceGroup", serviceGroup, "error", err.Error())
		}
	}

	log.V(1).Info("privacy pref store falling back to permissive", "reason", "no-privacy-url")
	return privacy.NewPermissivePreferencesStore()
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
		log.Info(fmt.Sprintf("starting %s server on %s", name, addr), "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "server error", "server", name)
		}
	}()
}

// embeddingProviderAdapter adapts a PromptKit EmbeddingProvider to Omnia's
// memory.EmbeddingProvider interface. It also surfaces the per-call token usage
// that PromptKit reports (and that the EmbeddingProvider interface drops) to a
// usage recorder, so embedding spend lands in session-api's provider_usage
// table + the omnia_embedding_tokens_total counter.
type embeddingProviderAdapter struct {
	inner pkproviders.EmbeddingProvider
	usage *memory.EmbeddingUsageRecorder // nil-safe; may be unset
}

func (a *embeddingProviderAdapter) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	resp, err := a.inner.Embed(ctx, pkproviders.EmbeddingRequest{Texts: texts})
	if err != nil {
		return nil, err
	}
	if resp.Usage != nil {
		a.usage.RecordEmbeddingUsage(ctx, resp.Model, resp.Usage.TotalTokens)
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
		log.V(1).Info(reasonNoInClusterKubeconfig, "error", err.Error())
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

// buildConsolidationWorker constructs the LLM-driven consolidation worker
// when the operator has set CONSOLIDATION_INTERVAL. Returns nil (disabled)
// when:
//   - interval is unset or unparseable
//   - in-cluster k8s config is unavailable (the worker needs to list
//     MemoryPolicy CRs cluster-wide)
//
// Worker lifecycle is identical to the existing compaction worker —
// caller invokes go cw.Run(ctx).
//
// The function is split: parseConsolidationInterval + newConsolidationK8sClient
// gather external deps (untestable from unit tests), newConsolidationWorker
// composes them into a worker (fully testable with a fake client).
func buildConsolidationWorker(_ context.Context, f *flags, pgStore *memory.PostgresMemoryStore, auditLogger *eeaudit.Logger, log logr.Logger) *consolidation.Worker {
	if !f.enterprise {
		return nil
	}
	interval, ok := parseConsolidationInterval(f.consolidationInterval, log)
	if !ok {
		return nil
	}
	c, ok := newConsolidationK8sClient(log)
	if !ok {
		return nil
	}
	return newConsolidationWorker(interval, c, f.workspace, pgStore, auditLogger, log)
}

// buildProjectionWorker constructs the Memory Galaxy pre-render worker when the
// operator has set PROJECTION_INTERVAL. Returns nil (disabled) when the interval
// is unset/unparseable or in-cluster k8s config is unavailable (the worker lists
// MemoryPolicy + Workspace CRs cluster-wide, reusing the consolidation client).
// Caller invokes go pw.Run(ctx).
func buildProjectionWorker(f *flags, pgStore *memory.PostgresMemoryStore, reg prometheus.Registerer, log logr.Logger) *projectionworker.Worker {
	if !f.enterprise {
		return nil
	}
	interval, err := time.ParseDuration(f.projectionInterval)
	if f.projectionInterval == "" || err != nil {
		if f.projectionInterval != "" {
			log.Error(err, "invalid --projection-interval; worker disabled", "value", f.projectionInterval)
		}
		return nil
	}
	c, ok := newConsolidationK8sClient(log) // reuse: in-cluster client + scheme
	if !ok {
		return nil
	}
	return newProjectionWorker(interval, c, f.workspace, pgStore, reg, log)
}

// buildEmbeddingMetricsCollector constructs the embedding-pipeline metrics
// collector when --metrics-collect-interval is non-empty (default 60s). It is a
// pure-DB poller (no k8s client, not enterprise-gated): coverage + re-embed
// backlog are OSS operational signals. currentModel comes from the embedding
// service; when no embedding provider is configured it is empty and the backlog
// counts only rows with no embedding at all. Returns nil when disabled or the
// interval is unparseable. Caller invokes go mc.Run(ctx).
func buildEmbeddingMetricsCollector(
	f *flags, pgStore *memory.PostgresMemoryStore, embeddingSvc *memory.EmbeddingService,
	reg prometheus.Registerer, log logr.Logger,
) *memory.EmbeddingMetricsCollector {
	if f.metricsCollectInterval == "" {
		return nil
	}
	interval, err := time.ParseDuration(f.metricsCollectInterval)
	if err != nil {
		log.Error(err, "invalid --metrics-collect-interval; collector disabled", "value", f.metricsCollectInterval)
		return nil
	}
	var currentModel string
	if embeddingSvc != nil {
		currentModel = embeddingSvc.ModelName()
	}
	metrics := memory.NewEmbeddingMetrics()
	metrics.MustRegister(reg)
	return memory.NewEmbeddingMetricsCollector(
		pgStore, metrics, currentModel, interval, log.WithName("embedding-metrics"))
}

// newProjectionWorker composes the worker from already-acquired deps. Pure
// function — wiring tests pass a fake client.Client + a fresh registry and
// assert every field is populated without in-cluster kubeconfig or Postgres.
// workspaceName scopes the Workspaces lister to memory-api's own Workspace
// CR (#1899) — it is the operator-injected --workspace flag value.
func newProjectionWorker(interval time.Duration, c client.Client, workspaceName string, pgStore *memory.PostgresMemoryStore, reg prometheus.Registerer, log logr.Logger) *projectionworker.Worker {
	pool := pgStore.Pool()
	metrics := projectionworker.NewMetrics()
	metrics.MustRegister(reg)
	return projectionworker.NewWorker(projectionworker.WorkerOptions{
		Store:      pgStore,
		Policies:   consolidation.NewK8sPolicyLister(c),
		Workspaces: consolidation.NewK8sWorkspaceLister(c, workspaceName),
		Lock:       memorypg.NewAdvisoryLockStore(pool),
		Interval:   interval,
		Metrics:    metrics,
		Log:        log.WithName("projection"),
	})
}

// parseConsolidationInterval validates the CONSOLIDATION_INTERVAL flag.
// Returns (0, false) and logs when the worker should be disabled.
func parseConsolidationInterval(raw string, log logr.Logger) (time.Duration, bool) {
	const msgDisabled = "consolidation worker disabled"
	if raw == "" {
		log.V(1).Info(msgDisabled, "reason", "CONSOLIDATION_INTERVAL not set")
		return 0, false
	}
	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		log.Error(err, "invalid CONSOLIDATION_INTERVAL, "+msgDisabled, "value", raw)
		return 0, false
	}
	return interval, true
}

// newConsolidationK8sClient acquires the in-cluster k8s client used to list
// MemoryPolicy and Workspace CRs. Returns (nil, false) outside a cluster.
// The rest.InClusterConfig call is the only unit-test untestable bit; the
// scheme + client construction is split out to newClientForConsolidation.
func newConsolidationK8sClient(log logr.Logger) (client.Client, bool) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.V(1).Info("consolidation worker disabled",
			"reason", "no in-cluster kubeconfig", "error", err.Error())
		return nil, false
	}
	return newClientForConsolidation(kubeConfig, log)
}

// newClientForConsolidation builds a controller-runtime client with the
// scheme the consolidation worker needs (omniav1alpha1 for MemoryPolicy +
// Workspace lookups). Returns (nil, false) when client construction fails;
// the kubeConfig argument lets unit tests cover the success path with a
// stub config.
func newClientForConsolidation(kubeConfig *rest.Config, log logr.Logger) (client.Client, bool) {
	scheme := k8sruntime.NewScheme()
	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
	c, err := client.New(kubeConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "consolidation worker disabled: k8s client creation failed")
		return nil, false
	}
	return c, true
}

// newConsolidationWorker composes the worker from already-acquired deps.
// Pure function — wiring tests pass a fake client.Client and exercise it.
// workspaceName scopes the Workspaces lister to memory-api's own Workspace
// CR (#1899) — it is the operator-injected --workspace flag value.
func newConsolidationWorker(
	interval time.Duration,
	c client.Client,
	workspaceName string,
	pgStore *memory.PostgresMemoryStore,
	auditLogger *eeaudit.Logger,
	log logr.Logger,
) *consolidation.Worker {
	return consolidation.NewWorker(newConsolidationWorkerOptions(interval, c, workspaceName, pgStore, auditLogger, log))
}

// newConsolidationWorkerOptions builds the WorkerOptions value from
// externally-acquired deps. Pulled out of buildConsolidationWorker so
// a wiring test can assert every field is populated without spinning
// up in-cluster kubeconfig or Postgres. The Postgres adapters
// (Store/LockStore/PreFilterRunner) accept nil pools at construction
// — they only fail when actually called, so a wiring test can pass
// a store with a nil pool (NewPostgresMemoryStore(nil)) and still
// verify wiring.
//
// metrics registration uses the default Prometheus registerer
// (freshPromRegistry isolates per-test).
func newConsolidationWorkerOptions(
	interval time.Duration,
	c client.Client,
	workspaceName string,
	pgStore *memory.PostgresMemoryStore,
	auditLogger *eeaudit.Logger,
	log logr.Logger,
) consolidation.WorkerOptions {
	pool := pgStore.Pool()
	metrics := consolidation.NewMetrics()
	metrics.MustRegister(prometheus.DefaultRegisterer)
	var auditor consolidation.Auditor
	if auditLogger != nil {
		auditor = &consolidationAuditAdapter{inner: auditLogger}
	}
	return consolidation.WorkerOptions{
		Store:           memorypg.NewConsolidationWriter(pool),
		LockStore:       memorypg.NewAdvisoryLockStore(pool),
		Policies:        consolidation.NewK8sPolicyLister(c),
		Workspaces:      consolidation.NewK8sWorkspaceLister(c, workspaceName),
		PreFilterRunner: memorypg.NewPreFilterRunner(pool),
		RunTracker:      memorypg.NewConsolidationRunStore(pool),
		Client:          consolidation.NewClient(5 * time.Minute),
		Metrics:         metrics,
		Interval:        interval,
		Log:             log.WithName("consolidation"),
		LivenessMark:    func() { memory.MarkWorkerRunning(memory.WorkerNameConsolidation) },
		LivenessUnmark:  func() { memory.MarkWorkerStopped(memory.WorkerNameConsolidation) },
		PIIRedactor:     newConsolidationPIIRedactor(),
		Auditor:         auditor,
	}
}

// createEmbeddingService reads a Provider CRD by name and creates an
// EmbeddingService with the appropriate PromptKit embedding provider.
// Returns nil if the provider can't be resolved (logs the error).
func createEmbeddingService(ctx context.Context, providerName, workspaceName, workspaceNamespace string, store *memory.PostgresMemoryStore, usageEmitter memory.ProviderUsageEmitter, log logr.Logger) *memory.EmbeddingService {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Info("embedding service skipped", "reason", reasonNoInClusterKubeconfig)
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

	// Require role=embedding. Pre-role Providers default to llm
	// (and rejecting them here gives a clearer error than the downstream
	// PromptKit factory complaint).
	if err := omniav1alpha1.RequireProviderRole(&provider, omniav1alpha1.ProviderRoleEmbedding); err != nil {
		log.Error(err, "embedding service skipped", "provider", providerName)
		return nil
	}

	embeddingProvider, err := createEmbeddingProviderFromCRD(ctx, k8sClient, &provider, ns, log)
	if err != nil {
		log.Error(err, "failed to create embedding provider", "name", providerName, "type", provider.Spec.Type)
		return nil
	}

	// The usage recorder funnels every Embed call's token count into the
	// omnia_embedding_tokens_total counter and (when configured) session-api's
	// provider_usage table, making embedding spend visible.
	usageRecorder := memory.NewEmbeddingUsageRecorder(
		workspaceNamespace, workspaceName, string(provider.Spec.Type), providerName, usageEmitter, log)

	// Wrap with the metered decorator so every Embed caller — the dedup
	// similarity path (embeddingSvc.Provider().Embed), the EmbeddingService
	// write path, and the re-embed worker — emits the embed_* metrics
	// without touching individual call sites.
	metered := memory.NewMeteredEmbeddingProvider(&embeddingProviderAdapter{inner: embeddingProvider, usage: usageRecorder})
	modelID := embeddingModelIdentifier(providerName, provider.Spec.Model)
	svc := memory.NewEmbeddingService(store, metered, log).WithModelName(modelID)
	log.Info("embedding service enabled",
		"provider", providerName,
		"type", provider.Spec.Type,
		"model", provider.Spec.Model,
		"modelIdentifier", modelID,
	)
	return svc
}

// embeddingModelIdentifier is the value stamped on every embedding write so the
// re-embed worker can detect a model swap (MAINT-5). It combines the Provider
// CRD name with the resolved model so editing spec.model in place (same CRD
// name) still changes the identifier and triggers a re-embed — keying on the
// CRD name alone missed that, silently mixing vector spaces. When the provider
// exposes no model, the bare CRD name is used (no spurious re-embed).
func embeddingModelIdentifier(providerName, model string) string {
	if model == "" {
		return providerName
	}
	return providerName + "/" + model
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
	// Hyperscaler hosting (e.g. Azure OpenAI): pass platform + config through so
	// PromptKit's ResolveEmbeddingTransport resolves the platform endpoint and
	// applies the credential (token refresh) per request.
	if provider.Spec.Platform != nil {
		spec.Platform = string(provider.Spec.Platform.Type)
		spec.PlatformConfig = &pkproviders.PlatformConfig{
			Type:     string(provider.Spec.Platform.Type),
			Region:   provider.Spec.Platform.Region,
			Project:  provider.Spec.Platform.Project,
			Endpoint: provider.Spec.Platform.Endpoint,
		}
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
	// Keyless hyperscaler auth (workload identity): build a platform credential
	// whose Apply injects a refreshed AAD token per request, instead of reading
	// an API-key Secret. PromptKit's ResolveEmbeddingTransport wires it into the
	// embedding HTTP client when spec.Platform is set (see createEmbeddingProviderFromCRD).
	if provider.Spec.Auth != nil && provider.Spec.Auth.Type == omniav1alpha1.AuthMethodWorkloadIdentity {
		endpoint := ""
		if provider.Spec.Platform != nil {
			endpoint = provider.Spec.Platform.Endpoint
		}
		cred, err := credentials.NewAzureCredential(ctx, endpoint)
		if err != nil {
			return nil, fmt.Errorf("build azure workload-identity credential for embedding provider %q: %w", provider.Name, err)
		}
		return cred, nil
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
	// Low defaults: memory-api is deployed per-workspace (API + background
	// workers share this pool), so total DB connections scale with workspace
	// count. 50×N exhausted a small (Azure B1ms, max_connections=50)
	// instance. Override per busy workspace with PG_MAX_CONNS.
	defaultMaxConns        = 8
	defaultMinConns        = 2
	defaultMaxConnLifetime = time.Hour
	defaultMaxConnIdleTime = 30 * time.Minute
)

// defaultEmbeddingDimensions is the embedding vector size used when no
// embedding provider is configured. The columns must still exist (consolidation
// dup-detection reads memory_entities.embedding), so we fall back to the
// historical hardcoded size rather than skip the reconcile. Matches OpenAI
// text-embedding-3-small / ada-002.
const defaultEmbeddingDimensions = 1536

// resolveEmbeddingDim returns the embedding dimension the schema reconciler
// should target: the configured provider's Dimensions() when a provider is
// wired, otherwise the historical default so the columns still exist.
func resolveEmbeddingDim(embeddingSvc *memory.EmbeddingService) int {
	if embeddingSvc == nil {
		return defaultEmbeddingDimensions
	}
	if dim := embeddingSvc.Provider().Dimensions(); dim > 0 {
		return dim
	}
	return defaultEmbeddingDimensions
}

// reasonNoInClusterKubeconfig is the log reason used by initialisation paths
// that gracefully degrade when the binary is not running inside a Kubernetes
// pod (no in-cluster kubeconfig). Extracted for S1192 — duplicated 3+ times.
const reasonNoInClusterKubeconfig = "no in-cluster kubeconfig"

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
