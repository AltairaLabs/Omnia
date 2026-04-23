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

	pkproviders "github.com/AltairaLabs/PromptKit/runtime/providers"
	ollamaProvider "github.com/AltairaLabs/PromptKit/runtime/providers/ollama"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	eeaudit "github.com/altairalabs/omnia/ee/pkg/audit"
	eemetrics "github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
	"github.com/altairalabs/omnia/internal/memory"
	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	memorypg "github.com/altairalabs/omnia/internal/memory/postgres"
	sessionapi "github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/internal/tracing"
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
		DefaultTTL: defaultTTL,
		Purpose:    f.purpose,
	}

	// --- Retention worker ---
	startRetentionWorker(ctx, store, f.retentionInterval, log)

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
	apiMux, cleanup := buildAPIMux(store, embeddingSvc, svcCfg, eventPublisher, f.enterprise, pool, log)
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
	if memCfg.DefaultTTL != "" {
		f.defaultTTL = memCfg.DefaultTTL
	}
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
	store memory.Store,
	embeddingSvc *memory.EmbeddingService,
	cfg memoryapi.MemoryServiceConfig,
	publisher memoryapi.MemoryEventPublisher,
	enterprise bool,
	pool *pgxpool.Pool,
	log logr.Logger,
) (http.Handler, func()) {
	httpMetrics := memoryapi.NewHTTPMetrics(nil)

	svc := memoryapi.NewMemoryService(store, embeddingSvc, cfg, log)
	if publisher != nil {
		svc.SetEventPublisher(publisher)
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

	// AuditMiddleware always applied — populates request context with IP/UA.
	// The service only emits events when an audit logger is configured.
	apiHandler := memoryapi.AuditMiddleware(mux)

	// Enterprise privacy middleware (opt-out + PII redaction).
	if enterprise {
		apiHandler = wrapPrivacyMiddleware(apiHandler, pool, log)
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
func wrapPrivacyMiddleware(next http.Handler, pool *pgxpool.Pool, log logr.Logger) http.Handler {
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

	classifier := memoryapi.ContentClassifier(privacy.NewContentClassifier())
	mw := memoryapi.NewMemoryPrivacyMiddleware(checkOptOut, contentRedactor, classifier, log)
	log.Info("memory privacy middleware enabled")
	return mw.Wrap(next)
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

// startRetentionWorker constructs the composite retention worker from
// the active policy source and spawns it as a goroutine. No-op when no
// policy source is available.
func startRetentionWorker(ctx context.Context, store *memory.PostgresMemoryStore, legacyInterval string, log logr.Logger) {
	loader := buildRetentionPolicyLoader(legacyInterval, log)
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
func buildRetentionPolicyLoader(legacyInterval string, log logr.Logger) memory.PolicyLoader {
	kubeConfig, err := rest.InClusterConfig()
	if err == nil {
		scheme := k8sruntime.NewScheme()
		utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
		c, clientErr := client.New(kubeConfig, client.Options{Scheme: scheme})
		if clientErr == nil {
			log.Info("retention policy loader enabled", "source", "MemoryRetentionPolicy CRD")
			return memory.NewK8sPolicyLoader(c, log)
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

	embeddingProvider, err := createEmbeddingProviderFromCRD(&provider, log)
	if err != nil {
		log.Error(err, "failed to create embedding provider", "name", providerName, "type", provider.Spec.Type)
		return nil
	}

	adapter := &embeddingProviderAdapter{inner: embeddingProvider}
	svc := memory.NewEmbeddingService(store, adapter, log)
	log.Info("embedding service enabled",
		"provider", providerName,
		"type", provider.Spec.Type,
		"model", provider.Spec.Model,
	)
	return svc
}

// createEmbeddingProviderFromCRD creates a PromptKit EmbeddingProvider from
// the Provider CRD spec.
func createEmbeddingProviderFromCRD(provider *omniav1alpha1.Provider, log logr.Logger) (pkproviders.EmbeddingProvider, error) {
	switch provider.Spec.Type {
	case omniav1alpha1.ProviderTypeOllama:
		var opts []ollamaProvider.EmbeddingOption
		if provider.Spec.BaseURL != "" {
			opts = append(opts, ollamaProvider.WithEmbeddingBaseURL(provider.Spec.BaseURL))
		}
		if provider.Spec.Model != "" {
			opts = append(opts, ollamaProvider.WithEmbeddingModel(provider.Spec.Model))
		}
		p := ollamaProvider.NewEmbeddingProvider(opts...)
		log.V(1).Info("created Ollama embedding provider",
			"baseURL", provider.Spec.BaseURL,
			"model", provider.Spec.Model,
		)
		return p, nil

	default:
		return nil, fmt.Errorf("unsupported embedding provider type: %s (supported: ollama)", provider.Spec.Type)
	}
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
