/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
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
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/cmd/privacy-api/migrations"
	eelicense "github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/internal/serviceauth"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/servicediscovery"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// Pool configuration defaults — kept low because privacy-api is deployed per-workspace.
const (
	defaultMaxConns        = int32(8)
	defaultMinConns        = int32(2)
	defaultMaxConnLifetime = time.Hour
	defaultMaxConnIdleTime = 30 * time.Minute
	defaultCacheTTL        = 60 * time.Second
)

// flags groups all CLI flags for the privacy-api binary.
type flags struct {
	apiAddr      string
	healthAddr   string
	metricsAddr  string
	postgresConn string
	redisURL     string
	workspace    string
	enterprise   bool
	// operatorAPIURL is the operator/arena-controller license endpoint, used to
	// nag when enterprise features run without a valid license (#1682).
	operatorAPIURL string

	// OTLP tracing (optional).
	tracingEnabled  bool
	tracingEndpoint string
	tracingSample   float64
	tracingInsecure bool

	// ServiceAccount auth (opt-in).
	authEnabled           bool
	authAllowedSubjects   string
	authAllowedNamespaces string
	authAudiences         string

	// Consent-outbox replay worker config.
	outboxReplayInterval time.Duration
	outboxRetention      time.Duration
}

func parseFlags() *flags {
	f := &flags{}
	flag.StringVar(&f.apiAddr, "api-addr", ":8080", "API server listen address")
	flag.StringVar(&f.healthAddr, "health-addr", ":8081", "Health probe listen address")
	flag.StringVar(&f.metricsAddr, "metrics-addr", ":9090", "Metrics server listen address")
	flag.StringVar(&f.postgresConn, "postgres-conn", "", "Postgres connection string (env POSTGRES_CONN)")
	flag.StringVar(&f.redisURL, "redis-url", "", "Redis URL (redis:// or rediss://); env REDIS_URL fallback")
	flag.StringVar(&f.workspace, "workspace", "", "Workspace name for K8s CRD config resolution (env OMNIA_WORKSPACE)")
	flag.BoolVar(&f.enterprise, "enterprise", false, "Enable enterprise features (env ENTERPRISE_ENABLED)")
	flag.StringVar(&f.operatorAPIURL, "operator-api-url", "", "Operator/arena-controller license endpoint (env OPERATOR_API_URL). When enterprise runs without a valid license, privacy-api logs a startup reminder. Never blocks.")
	flag.BoolVar(&f.tracingEnabled, "tracing-enabled", false, "Enable OTLP tracing export (env TRACING_ENABLED)")
	flag.StringVar(&f.tracingEndpoint, "tracing-endpoint", "", "OTLP collector gRPC endpoint host:port (env TRACING_ENDPOINT)")
	flag.Float64Var(&f.tracingSample, "tracing-sample-rate", 0, "Tracing sample rate 0.0–1.0; 0 → SDK default 0.1 (env TRACING_SAMPLE_RATE)")
	flag.BoolVar(&f.tracingInsecure, "tracing-insecure", false, "Disable TLS for OTLP connection (env TRACING_INSECURE)")
	flag.BoolVar(&f.authEnabled, "auth-enabled", false,
		"Require Kubernetes ServiceAccount bearer-token auth on the JSON API")
	flag.StringVar(&f.authAllowedSubjects, "auth-allowed-subjects", "",
		"Comma-separated allowed ServiceAccount subjects (exact match)")
	flag.StringVar(&f.authAllowedNamespaces, "auth-allowed-namespaces", "",
		"Comma-separated trusted namespaces; any SA in these namespaces is allowed")
	flag.StringVar(&f.authAudiences, "auth-audiences", "",
		"Comma-separated audiences for projected tokens (optional)")
	flag.DurationVar(&f.outboxReplayInterval, "consent-outbox-replay-interval", 5*time.Minute,
		"Consent-outbox replay cadence (env CONSENT_OUTBOX_REPLAY_INTERVAL)")
	flag.DurationVar(&f.outboxRetention, "consent-outbox-retention", 24*time.Hour,
		"Consent-outbox replay window + delivered-row TTL (env CONSENT_OUTBOX_RETENTION)")
	flag.Parse()

	f.applyEnvFallbacks()
	return f
}

func (f *flags) applyEnvFallbacks() {
	envFallback(&f.postgresConn, "", "POSTGRES_CONN")
	envFallback(&f.redisURL, "", "REDIS_URL")
	envFallback(&f.workspace, "", "OMNIA_WORKSPACE")
	envFallback(&f.apiAddr, ":8080", "API_ADDR")
	envFallback(&f.healthAddr, ":8081", "HEALTH_ADDR")
	envFallback(&f.metricsAddr, ":9090", "METRICS_ADDR")
	envBoolFallback(&f.enterprise, "ENTERPRISE_ENABLED")
	envFallback(&f.operatorAPIURL, "", "OPERATOR_API_URL")

	envBoolFallback(&f.tracingEnabled, "TRACING_ENABLED")
	envBoolFallback(&f.tracingInsecure, "TRACING_INSECURE")
	envFallback(&f.tracingEndpoint, "", "TRACING_ENDPOINT")
	if v := os.Getenv("TRACING_SAMPLE_RATE"); v != "" && f.tracingSample == 0 {
		if rate, err := strconv.ParseFloat(v, 64); err == nil {
			f.tracingSample = rate
		}
	}

	// ServiceAccount auth — same env vars the operator stamps via
	// applySessionAPIServerAuthEnv in internal/controller/service_auth.go.
	// Mirror session-api's mapping exactly (CLI flag wins when non-default,
	// env fills in when flag is still at its zero/default value).
	envBoolFallback(&f.authEnabled, "SESSION_API_AUTH_ENABLED")
	envFallback(&f.authAllowedSubjects, "", "SESSION_API_AUTH_ALLOWED_SUBJECTS")
	envFallback(&f.authAllowedNamespaces, "", "SESSION_API_AUTH_ALLOWED_NAMESPACES")
	envFallback(&f.authAudiences, "", "SESSION_API_AUTH_AUDIENCES")

	envDurationFallback(&f.outboxReplayInterval, 5*time.Minute, "CONSENT_OUTBOX_REPLAY_INTERVAL")
	envDurationFallback(&f.outboxRetention, 24*time.Hour, "CONSENT_OUTBOX_RETENTION")
}

// envFallback sets *dst from envKey when *dst equals defaultVal and the env var is non-empty.
func envFallback(dst *string, defaultVal, envKey string) {
	if *dst == defaultVal {
		if v := os.Getenv(envKey); v != "" {
			*dst = v
		}
	}
}

// envBoolFallback enables a boolean flag from an env var when flag is still false and env var is "true".
func envBoolFallback(dst *bool, envKey string) {
	if !*dst && os.Getenv(envKey) == "true" {
		*dst = true
	}
}

// envDurationFallback sets *dst from envKey when *dst equals defaultVal and the env var is a valid duration.
func envDurationFallback(dst *time.Duration, defaultVal time.Duration, envKey string) {
	if *dst == defaultVal {
		if v := os.Getenv(envKey); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				*dst = d
			}
		}
	}
}

// splitAndTrim splits a comma-separated string, trims whitespace, and drops empties.
func splitAndTrim(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
// It never blocks — privacy-api's features keep working regardless. The
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
	// privacy-api is per-workspace; no service-group needed.
	if f.workspace != "" && f.postgresConn == "" {
		if err := resolveConfigFromWorkspace(f, log); err != nil {
			return fmt.Errorf("resolving config from workspace: %w", err)
		}
		log.Info("config resolved from workspace CRD", "workspace", f.workspace)
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
	// privacy-api is enterprise-only; when it runs without a valid license, log
	// a one-time reminder. Never blocks.
	nagLicenseAtStartup(ctx, f, log)

	// --- Tracing ---
	// Set propagator so incoming trace context is extracted and spans become
	// children of the caller's trace.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if f.tracingEnabled {
		tracingCfg := tracing.Config{
			Enabled:     true,
			Endpoint:    f.tracingEndpoint,
			ServiceName: "omnia-privacy-api",
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
	if err := runPrivacyMigrations(f.postgresConn, log); err != nil {
		return err
	}
	log.V(1).Info("migrations complete")

	// --- Base preferences store ---
	base := privacy.NewPreferencesStore(pool)

	// --- Enterprise features (consent/opt-out, memory fan-out, analytics metric) ---
	// When enterprise is false, privacy-api is still deployed but serves only /healthz
	// on the API port (belt-and-suspenders: privacy-api is enterprise-only by design).
	var (
		optOutStore     privacy.PreferencesStore = base
		notifier        privacy.ConsentNotifier  = privacy.NoopConsentNotifier{}
		deletionHandler *privacy.DeletionHandler
	)

	if f.enterprise {
		// Optional Redis warm cache.
		if f.redisURL != "" {
			cacheTTL := envDuration("PRIVACY_CACHE_TTL", defaultCacheTTL)
			opts, parseErr := goredis.ParseURL(f.redisURL)
			if parseErr != nil {
				return fmt.Errorf("parsing redis URL: %w", parseErr)
			}
			redisClient := goredis.NewClient(opts)
			if pingErr := redisClient.Ping(ctx).Err(); pingErr != nil {
				return fmt.Errorf("redis ping failed: %w", pingErr)
			}
			defer func() { _ = redisClient.Close() }()
			kv := &redisKV{client: redisClient, prefix: "privacy:"}
			optOutStore = privacy.NewCachedPreferencesStore(base, kv, cacheTTL, log)
			log.V(1).Info("redis warm cache enabled", "addr", opts.Addr, "ttl", cacheTTL)
		}

		// Consent notifier: fan-out revocations to this workspace's memory-api
		// service-groups, resolved from workspace status. No env override.
		memoryURLs, workspaceUID := resolveMemoryFanout(f, log)
		if len(memoryURLs) == 0 {
			// No fan-out targets: revocations are recorded then immediately marked
			// delivered without any prune, and the stuck gauge stays 0 — the backstop
			// is inert. Surface this at Info so an empty-target misconfiguration in an
			// enterprise deploy is visible rather than hidden behind delivered=true.
			log.Info("consent fan-out has no memory targets; revocations will not be pushed (check workspace status)")
		} else {
			log.V(1).Info("consent notifier configured", "memoryURLCount", len(memoryURLs))
		}
		tokenSrc := serviceauth.NewTokenSource("", 0)
		notifier = privacy.NewMemoryAPINotifier(memoryURLs, workspaceUID, tokenSrc, log)

		// Analytics opt-in metric (relocated from memory-api CE2).
		optInWorker := NewOptInMetricWorker(
			base,
			envDuration("PRIVACY_OPTIN_METRIC_INTERVAL", 5*time.Minute),
			prometheus.DefaultRegisterer,
			log,
		)
		go optInWorker.Run(ctx)

		// Consent-outbox replay: re-delivers undelivered revocation rows and prunes old ones.
		replayWorker := NewOutboxReplayWorker(base, notifier, f.outboxReplayInterval, f.outboxRetention, prometheus.DefaultRegisterer, log)
		go replayWorker.Run(ctx)

		// DSAR orchestrator (#1676): privacy-api owns the deletion_requests lifecycle
		// and fans erasure out across the workspace's service-groups.
		groups, dsarUID := resolveGroupTargets(f, log)
		deletionHandler = buildDeletionHandler(pool, groups, dsarUID, tokenSrc, log)
	} else {
		log.Info("privacy-api enterprise features disabled; consent and opt-out routes not registered")
	}

	// --- ServiceAccount auth (opt-in) ---
	reviewer, allowedSubjects, allowedNamespaces, err := buildServiceAuth(f, log)
	if err != nil {
		return err
	}

	// --- Build API mux ---
	// /healthz is always registered; enterprise routes are gated on f.enterprise.
	auditStore := privacy.NewAuditStore(pool)
	apiMux := buildAPIMux(f.enterprise, optOutStore, base, auditStore, log, notifier, deletionHandler)

	// Wire up auth handler.
	apiHandler := buildHandler(reviewer, allowedSubjects, allowedNamespaces, apiMux)

	// --- Servers ---
	healthSrv := newHealthServer(f.healthAddr, pool)
	metricsSrv := newMetricsServer(f.metricsAddr)
	apiSrv := &http.Server{
		Addr:         f.apiAddr,
		Handler:      apiHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	startHTTPServer(log, "health", f.healthAddr, healthSrv)
	startHTTPServer(log, "metrics", f.metricsAddr, metricsSrv)
	startHTTPServer(log, "privacy API", f.apiAddr, apiSrv)

	log.Info("privacy-api ready",
		"api", f.apiAddr,
		"health", f.healthAddr,
		"metrics", f.metricsAddr,
		"workspace", f.workspace,
		"authEnabled", f.authEnabled,
		"enterprise", f.enterprise,
		"tracingEnabled", f.tracingEnabled,
	)

	// --- Wait for shutdown ---
	<-ctx.Done()
	log.Info("shutting down")
	shutdownServers(log, apiSrv, healthSrv, metricsSrv)
	return nil
}

// registerRoutes mounts all privacy handlers on mux. optOutStore is the
// (optionally cached) PreferencesStore; concrete is the raw postgres store
// the consent and stats handlers require. notifier fans out consent
// revocations to memory-api instances (use NoopConsentNotifier{} when no
// memory URLs are configured).
func registerRoutes(
	mux *http.ServeMux,
	optOutStore privacy.PreferencesStore,
	concrete *privacy.PreferencesPostgresStore,
	auditStore privacy.AuditIngester,
	log logr.Logger,
	notifier privacy.ConsentNotifier,
	deletionHandler *privacy.DeletionHandler,
) {
	privacy.RegisterDocs(mux)
	privacy.NewOptOutHandler(optOutStore, log).RegisterRoutes(mux)
	privacy.NewConsentHandler(concrete, nil, log).WithConsentNotifier(notifier).RegisterRoutes(mux)
	privacy.NewConsentStatsHandler(concrete, log).RegisterRoutes(mux)
	privacy.NewEnforcementStatsHandler(concrete, log).RegisterRoutes(mux)
	// Audit hub (#1673): ingest enforcement events forwarded by memory/session-api.
	privacy.NewAuditIngestHandler(auditStore, log).RegisterRoutes(mux)
	// DSAR lifecycle (#1676): privacy-api owns deletion-request[s]; nil when the
	// orchestrator could not be built (no workspace targets resolved).
	if deletionHandler != nil {
		deletionHandler.RegisterRoutes(mux)
	}
}

// buildDeletionHandler constructs the DSAR orchestrator: a PostgresDeletionStore
// over privacy's deletion_requests table plus a fan-out SubjectEraser that erases
// across the workspace's service-groups. DSAR lifecycle events are written to
// privacy's central audit_log via DeletionAuditLogger (#1678).
func buildDeletionHandler(
	pool *pgxpool.Pool,
	groups []privacy.GroupTarget,
	workspaceUID string,
	ts *serviceauth.TokenSource,
	log logr.Logger,
) *privacy.DeletionHandler {
	store := privacy.NewPostgresDeletionStore(pool)
	eraser := privacy.NewFanOutSubjectEraser(groups, workspaceUID, privacy.NewSessionGroupEraser(ts, log), ts, log)
	auditLogger := privacy.NewDeletionAuditLogger(privacy.NewAuditStore(pool), workspaceUID, log)
	svc := privacy.NewDeletionService(store, privacy.NoOpSessionDeleter{}, auditLogger, log)
	svc.SetSubjectEraser(eraser)
	return privacy.NewDeletionHandler(svc, log)
}

// buildAPIMux creates the HTTP mux for the privacy-api. /healthz is always
// registered. When enterprise is true, the full consent/opt-out/stats route set
// is also registered via registerRoutes; when false, only /healthz is served on
// the API port (belt-and-suspenders for enterprise-only deployments).
func buildAPIMux(
	enterprise bool,
	optOutStore privacy.PreferencesStore,
	concrete *privacy.PreferencesPostgresStore,
	auditStore privacy.AuditIngester,
	log logr.Logger,
	notifier privacy.ConsentNotifier,
	deletionHandler *privacy.DeletionHandler,
) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if enterprise {
		registerRoutes(mux, optOutStore, concrete, auditStore, log, notifier, deletionHandler)
	}
	return mux
}

// buildHandler wraps inner with ServiceAccount auth middleware. /healthz is exempt.
// A nil reviewer is a pass-through (auth disabled).
func buildHandler(
	reviewer serviceauth.TokenReviewer,
	allowedSubjects, allowedNamespaces []string,
	inner http.Handler,
) http.Handler {
	authMW := serviceauth.RequireServiceAccount(reviewer, allowedSubjects, allowedNamespaces, "/healthz")
	return authMW(inner)
}

// resolveMemoryFanout returns the memory-api base URLs for all service-groups
// in the workspace AND the Workspace CRD UID. The UID is what
// memory_entities.workspace_id stores (the runtime writes string(ws.UID) there),
// so it MUST be used as the ?workspace= scope on consent-revocation POSTs — the
// workspace name would fail the uuid cast and match no rows. Returns (nil, "")
// when no workspace is configured or the K8s lookup fails.
//
// These URLs are the only source: there is no MEMORY_API_URLS override any
// more, since privacy-api serves one workspace and that workspace knows its own
// memory-api endpoints.
func resolveMemoryFanout(f *flags, log logr.Logger) ([]string, string) {
	if f.workspace == "" {
		return nil, ""
	}
	c, ok := newWorkspaceClient(log, "memory fan-out resolution skipped")
	if !ok {
		return nil, ""
	}
	urls, uid := memoryFanoutFromWorkspace(context.Background(), c, f.workspace)
	if uid == "" {
		log.V(1).Info("memory fan-out resolution skipped", "reason", "workspace not found", "workspace", f.workspace)
	}
	return urls, uid
}

// newWorkspaceClient builds a controller-runtime client with the omnia scheme for
// reading Workspace CRDs at startup. reason is the log message used when config or
// client construction fails. Returns (nil, false) on failure.
func newWorkspaceClient(log logr.Logger, reason string) (client.Client, bool) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.V(1).Info(reason, "reason", "no K8s config", "err", err.Error())
		return nil, false
	}
	scheme := k8sruntime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.V(1).Info(reason, "reason", "K8s client error", "err", err.Error())
		return nil, false
	}
	return c, true
}

// resolveGroupTargets returns one privacy.GroupTarget per service-group in the
// workspace (session + memory base URLs) plus the Workspace CRD UID used to scope
// memory erasure. Returns (nil, "") when no workspace is configured or the lookup
// fails.
func resolveGroupTargets(f *flags, log logr.Logger) ([]privacy.GroupTarget, string) {
	if f.workspace == "" {
		return nil, ""
	}
	c, ok := newWorkspaceClient(log, "DSAR group-target resolution skipped")
	if !ok {
		return nil, ""
	}
	targets, uid := groupTargetsFromWorkspace(context.Background(), c, f.workspace)
	if uid == "" {
		log.V(1).Info("DSAR group-target resolution skipped", "reason", "workspace not found", "workspace", f.workspace)
	}
	return targets, uid
}

// groupTargetsFromWorkspace loads the named Workspace CRD via c and returns a
// GroupTarget for each service-group that has a session or memory URL, plus the
// Workspace UID. Returns (nil, "") if the workspace cannot be loaded.
func groupTargetsFromWorkspace(ctx context.Context, c client.Client, workspaceName string) ([]privacy.GroupTarget, string) {
	var ws omniav1alpha1.Workspace
	if err := c.Get(ctx, client.ObjectKey{Name: workspaceName}, &ws); err != nil {
		return nil, ""
	}
	var targets []privacy.GroupTarget
	for _, svc := range ws.Status.Services {
		if svc.SessionURL == "" && svc.MemoryURL == "" {
			continue
		}
		targets = append(targets, privacy.GroupTarget{
			Name:       svc.Name,
			SessionURL: svc.SessionURL,
			MemoryURL:  svc.MemoryURL,
		})
	}
	return targets, string(ws.UID)
}

// memoryFanoutFromWorkspace loads the named Workspace CRD via c and returns its
// memory-api fan-out URLs plus the Workspace UID (the workspace_id used by
// memory_entities). Returns (nil, "") if the workspace cannot be loaded.
func memoryFanoutFromWorkspace(ctx context.Context, c client.Client, workspaceName string) ([]string, string) {
	var ws omniav1alpha1.Workspace
	if err := c.Get(ctx, client.ObjectKey{Name: workspaceName}, &ws); err != nil {
		return nil, ""
	}
	var urls []string
	for _, svc := range ws.Status.Services {
		if svc.MemoryURL != "" {
			urls = append(urls, svc.MemoryURL)
		}
	}
	return urls, string(ws.UID)
}

// resolveConfigFromWorkspace reads the Workspace CRD to obtain the privacy-api
// postgres connection string for the given workspace.
func resolveConfigFromWorkspace(f *flags, log logr.Logger) error {
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
	namespace := servicediscovery.DetectNamespace()
	privCfg, err := cr.ResolvePrivacyConfig(context.Background(), f.workspace, namespace)
	if err != nil {
		return fmt.Errorf("resolving privacy config: %w", err)
	}
	f.postgresConn = privCfg.PostgresConn
	log.V(1).Info("privacy config resolved", "workspace", f.workspace)
	return nil
}

// buildServiceAuth constructs the TokenReviewer and parses allowlists when
// auth is enabled. Returns (nil, nil, nil, nil) when auth is disabled.
func buildServiceAuth(f *flags, log logr.Logger) (serviceauth.TokenReviewer, []string, []string, error) {
	if !f.authEnabled {
		log.Info("WARNING: privacy-api JSON API is UNAUTHENTICATED " +
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

// initPool creates and returns a pgxpool connection pool with env-configured limits.
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

// runPrivacyMigrations applies the privacy-api database schema migrations.
func runPrivacyMigrations(connStr string, log logr.Logger) error {
	migrator, err := migrations.NewMigrator(connStr, log)
	if err != nil {
		return fmt.Errorf("creating privacy migrator: %w", err)
	}
	if err := migrator.Up(); err != nil {
		_ = migrator.Close()
		return fmt.Errorf("running privacy migrations: %w", err)
	}
	_ = migrator.Close()
	return nil
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

// envDuration reads an environment variable as time.Duration, returning def on missing/invalid.
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

// startHTTPServer starts an HTTP server in a background goroutine.
func startHTTPServer(log logr.Logger, name, addr string, srv *http.Server) {
	go func() {
		log.Info(fmt.Sprintf("starting %s server on %s", name, addr), "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "server error", "server", name)
		}
	}()
}

// shutdownServers gracefully stops all servers with a 30-second timeout.
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
