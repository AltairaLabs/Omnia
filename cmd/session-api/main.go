/*
Copyright 2025.

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
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	gcsstorage "cloud.google.com/go/storage"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coreomniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/audit"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/ee/pkg/privacy/httpclient"
	"github.com/altairalabs/omnia/ee/pkg/redaction"
	"github.com/altairalabs/omnia/internal/serviceauth"
	"github.com/altairalabs/omnia/internal/session/api"
	"github.com/altairalabs/omnia/internal/session/otlp"
	sessionpg "github.com/altairalabs/omnia/internal/session/postgres"
	"github.com/altairalabs/omnia/internal/session/providers"
	"github.com/altairalabs/omnia/internal/session/providers/cold"
	pgprovider "github.com/altairalabs/omnia/internal/session/providers/postgres"
	"github.com/altairalabs/omnia/internal/session/providers/redis"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/servicediscovery"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

// redisClientProvider is implemented by providers that expose the underlying Redis client.
type redisClientProvider interface {
	RedisClient() goredis.UniversalClient
}

// flags groups all CLI flags for the session-api binary.
type flags struct {
	apiAddr         string
	healthAddr      string
	metricsAddr     string
	postgresConn    string
	redisURL        string
	coldBackend     string
	coldBucket      string
	coldRegion      string
	coldEndpoint    string
	enterprise      bool
	otlpEnabled     bool
	otlpGRPCAddr    string
	otlpHTTPAddr    string
	tracingEnabled  bool
	tracingEndpoint string
	tracingSample   float64
	tracingInsecure bool
	workspace       string
	serviceGroup    string

	// ServiceAccount auth (opt-in). When authEnabled is true, the JSON API
	// requires a Kubernetes ServiceAccount bearer token whose TokenReview
	// subject is either in authAllowedSubjects (exact match) or whose
	// ServiceAccount namespace is in authAllowedNamespaces.
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
	flag.StringVar(&f.redisURL, "redis-url", "", "Redis URL (redis:// or rediss://); env REDIS_URL fallback")
	flag.StringVar(&f.coldBackend, "cold-backend", "", "Cold archive backend (s3, gcs, azure)")
	flag.StringVar(&f.coldBucket, "cold-bucket", "", "Cold archive bucket name")
	flag.StringVar(&f.coldRegion, "cold-region", "", "Cold archive region (S3)")
	flag.StringVar(&f.coldEndpoint, "cold-endpoint", "", "Cold archive endpoint (S3)")
	flag.BoolVar(&f.enterprise, "enterprise", false, "Enable enterprise features (audit)")
	flag.BoolVar(&f.otlpEnabled, "otlp-enabled", false, "Enable OTLP ingestion endpoint")
	flag.StringVar(&f.otlpGRPCAddr, "otlp-grpc-addr", ":4317", "OTLP gRPC listen address")
	flag.StringVar(&f.otlpHTTPAddr, "otlp-http-addr", ":4318", "OTLP HTTP listen address")
	flag.StringVar(&f.workspace, "workspace", "", "Workspace name (K8s CRD resolution mode)")
	flag.StringVar(&f.serviceGroup, "service-group", "", "Service group name within workspace")
	flag.BoolVar(&f.authEnabled, "auth-enabled", false,
		"Require Kubernetes ServiceAccount bearer-token auth on the JSON API (opt-in)")
	flag.StringVar(&f.authAllowedSubjects, "auth-allowed-subjects", "",
		"Comma-separated allowed ServiceAccount subjects, exact-matched "+
			"(e.g. system:serviceaccount:omnia-system:omnia-dashboard). "+
			"Use for cross-namespace callers")
	flag.StringVar(&f.authAllowedNamespaces, "auth-allowed-namespaces", "",
		"Comma-separated trusted namespaces: any ServiceAccount in one of these "+
			"namespaces is allowed (covers per-AgentRuntime facade SAs, memory-api, "+
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
	envFallback(&f.coldBackend, "", "COLD_BACKEND")
	envFallback(&f.coldBucket, "", "COLD_BUCKET")
	envFallback(&f.coldRegion, "", "COLD_REGION")
	envFallback(&f.coldEndpoint, "", "COLD_ENDPOINT")
	envFallback(&f.apiAddr, ":8080", "API_ADDR")
	envFallback(&f.healthAddr, ":8081", "HEALTH_ADDR")
	envFallback(&f.metricsAddr, ":9090", "METRICS_ADDR")
	envFallback(&f.otlpGRPCAddr, ":4317", "OTLP_GRPC_ADDR")
	envFallback(&f.otlpHTTPAddr, ":4318", "OTLP_HTTP_ADDR")

	envBoolFallback(&f.enterprise, "ENTERPRISE_ENABLED")
	envBoolFallback(&f.otlpEnabled, "OTLP_ENABLED")

	envBoolFallback(&f.authEnabled, "SESSION_API_AUTH_ENABLED")
	envFallback(&f.authAllowedSubjects, "", "SESSION_API_AUTH_ALLOWED_SUBJECTS")
	envFallback(&f.authAllowedNamespaces, "", "SESSION_API_AUTH_ALLOWED_NAMESPACES")
	envFallback(&f.authAudiences, "", "SESSION_API_AUTH_AUDIENCES")

	envBoolFallback(&f.tracingEnabled, "TRACING_ENABLED")
	envBoolFallback(&f.tracingInsecure, "TRACING_INSECURE")
	envFallback(&f.tracingEndpoint, "", "TRACING_ENDPOINT")
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
		log.Info("config resolved from workspace CRD",
			"workspace", f.workspace,
			"serviceGroup", f.serviceGroup)
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

	// --- Postgres pool (shared) ---
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

	// --- Providers ---
	registry, providerCleanup, err := initProviders(ctx, f, pool, log)
	if err != nil {
		return err
	}
	defer providerCleanup()

	// --- Partition maintenance ---
	// The initial migration seeds partitions only ~2 weeks ahead and nothing
	// else rolls the window forward, so inserts would eventually fail with
	// SQLSTATE 23514. Keep several weeks of partitions ahead, on startup + daily.
	startPartitionMaintenance(ctx, pgprovider.NewFromPool(pool), 24*time.Hour, log)

	// --- Tracing ---
	// Set propagator so incoming trace context (e.g. from facade httpclient)
	// is extracted and spans become children of the caller's trace.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if f.tracingEnabled {
		tracingCfg := tracing.Config{
			Enabled:     true,
			Endpoint:    f.tracingEndpoint,
			ServiceName: "omnia-session-api",
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

	// --- ServiceAccount auth (opt-in) ---
	reviewer, allowedSubjects, allowedNamespaces, err := buildServiceAuth(f, log)
	if err != nil {
		return err
	}

	// --- Build API mux ---
	apiMux, sessionService, auditCleanup := buildAPIMux(pool, registry, f, log, reviewer, allowedSubjects, allowedNamespaces)
	defer auditCleanup()

	// --- Audit drain-forwarder (#1673) ---
	// Ships locally-recorded enforcement audit rows (pii_redacted, etc.) to the
	// privacy-api central audit hub so enforcement-stats can serve them. Runs
	// only when enterprise audit is on and a privacy-api URL resolves.
	if fwd := buildAuditForwarder(f.enterprise, pool,
		resolvePrivacyURL(ctx, f.workspace, f.serviceGroup, log),
		prometheus.DefaultRegisterer, log); fwd != nil {
		go fwd.Run(ctx)
		log.Info("audit forwarder started", "sourceService", auditSourceService)
	} else if f.enterprise {
		log.Info("audit forwarder skipped", "reason", "no privacy URL")
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
	startHTTPServer(log, "session API", f.apiAddr, apiSrv)

	// --- OTLP servers (optional) ---
	// OTLP ingest is a write path: the transformer CREATES sessions and appends
	// messages. The same ServiceAccount auth wired onto the JSON API gates both
	// OTLP listeners. When reviewer is nil (auth disabled) these are pass-through.
	//
	// NOTE: when auth is enabled, OTLP senders (the facade/runtime OTel exporters
	// and any alloy/collector forwarding to :4317/:4318) MUST present their
	// ServiceAccount bearer token. Wiring those clients + the Helm chart RBAC is a
	// later task; without it, exporters will be rejected (401/Unauthenticated).
	var grpcSrv *grpc.Server
	var otlpHTTPSrv *http.Server
	if f.otlpEnabled {
		grpcSrv, otlpHTTPSrv = startOTLPServers(f, sessionService, log, reviewer, allowedSubjects, allowedNamespaces)
	}

	log.Info("session-api ready",
		"api", f.apiAddr,
		"health", f.healthAddr,
		"metrics", f.metricsAddr,
		"enterprise", f.enterprise,
		"otlp", f.otlpEnabled,
	)

	// --- Wait for shutdown ---
	<-ctx.Done()
	log.Info("shutting down")

	shutdownServers(log, apiSrv, healthSrv, metricsSrv, grpcSrv, otlpHTTPSrv)
	return nil
}

// startHTTPServer starts an HTTP server in a background goroutine.
// partitionWeeksAhead is how many future weeks of partitions the maintenance
// loop keeps provisioned.
const partitionWeeksAhead = 4

// partitionMaintainer is the subset of the postgres provider the maintenance
// loop needs; declared here so the behaviour is unit-testable with a fake.
type partitionMaintainer interface {
	EnsurePartitionsAhead(ctx context.Context, weeksAhead int) error
}

// startPartitionMaintenance ensures weekly partitions exist for the current week
// and partitionWeeksAhead weeks ahead — immediately, then every interval — so
// session inserts keep working as time advances. Failures are logged, not fatal.
func startPartitionMaintenance(ctx context.Context, m partitionMaintainer, interval time.Duration, log logr.Logger) {
	ensure := func() {
		if err := m.EnsurePartitionsAhead(ctx, partitionWeeksAhead); err != nil {
			log.Error(err, "partition maintenance failed", "weeksAhead", partitionWeeksAhead)
			return
		}
		log.V(1).Info("partition maintenance complete", "weeksAhead", partitionWeeksAhead)
	}
	ensure()
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ensure()
			}
		}
	}()
}

func startHTTPServer(log logr.Logger, name, addr string, srv *http.Server) {
	go func() {
		log.Info(fmt.Sprintf("starting %s server on %s", name, addr), "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "server error", "server", name)
		}
	}()
}

// shutdownServers gracefully stops all servers with a 30-second timeout.
func shutdownServers(log logr.Logger, apiSrv, healthSrv, metricsSrv *http.Server, grpcSrv *grpc.Server, otlpHTTPSrv *http.Server) {
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()

	if grpcSrv != nil {
		grpcDone := make(chan struct{})
		go func() {
			grpcSrv.GracefulStop()
			close(grpcDone)
		}()
		select {
		case <-grpcDone:
			// Graceful shutdown completed
		case <-time.After(10 * time.Second):
			log.Info("gRPC graceful stop timed out, forcing stop")
			grpcSrv.Stop()
		}
	}

	for _, s := range []struct {
		name string
		srv  *http.Server
	}{
		{"OTLP HTTP", otlpHTTPSrv},
		{"metrics", metricsSrv},
		{"API", apiSrv},
		{"health", healthSrv},
	} {
		if s.srv == nil {
			continue
		}
		if err := s.srv.Shutdown(shutCtx); err != nil {
			log.Error(err, "server shutdown error", "server", s.name)
		}
	}
}

// Pool configuration defaults.
const (
	// Low defaults: session-api is deployed per-workspace, so total DB
	// connections scale with workspace count. 50×N exhausted a small
	// (Azure B1ms, max_connections=50) instance. Override per busy
	// workspace with PG_MAX_CONNS.
	defaultMaxConns        = 8
	defaultMinConns        = 2
	defaultMaxConnLifetime = time.Hour
	defaultMaxConnIdleTime = 30 * time.Minute
)

// initPool creates and returns a pgxpool connection pool with configured limits.
// Pool settings are read from environment variables with sensible defaults:
//
//	PG_MAX_CONNS (default 8), PG_MIN_CONNS (default 2),
//	PG_MAX_CONN_LIFETIME (default 1h), PG_MAX_CONN_IDLE_TIME (default 30m).
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
	migrator, err := sessionpg.NewMigrator(connStr, log)
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

// resolveConfigFromWorkspace uses the Kubernetes API to resolve session-api
// configuration from the named Workspace CRD and service group.
func (f *flags) resolveConfigFromWorkspace(log logr.Logger) error {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("building K8s client config: %w", err)
	}
	scheme := k8sruntime.NewScheme()
	_ = coreomniav1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("creating K8s client: %w", err)
	}
	cr := servicediscovery.NewConfigResolver(c)
	namespace := detectNamespace()
	sessCfg, err := cr.ResolveSessionConfig(context.Background(), f.workspace, f.serviceGroup, namespace)
	if err != nil {
		return fmt.Errorf("resolving session config: %w", err)
	}
	f.postgresConn = sessCfg.PostgresConn
	log.V(1).Info("session config resolved",
		"workspace", f.workspace,
		"serviceGroup", f.serviceGroup)
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
// SAs, memory-api, eval-worker) pass without enumerating every facade SA up
// front; cross-namespace callers (dashboard) stay on the exact-subject list.
//
// The session-api ServiceAccount must have RBAC to create TokenReviews
// (`authentication.k8s.io/tokenreviews: create`). The Role/RoleBinding is wired
// in the Helm chart (a later task); without it the reviewer's TokenReview calls
// fail closed (401).
func buildServiceAuth(f *flags, log logr.Logger) (serviceauth.TokenReviewer, []string, []string, error) {
	if !f.authEnabled {
		log.Info("WARNING: session-api JSON API is UNAUTHENTICATED " +
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

// buildAPIMux assembles the HTTP handler with all API routes, wrapped with
// Prometheus metrics middleware. Returns the handler and a cleanup function
// for the audit logger (no-op when enterprise is disabled).
//
// reviewer, allowedSubjects and allowedNamespaces wire ServiceAccount auth: when
// reviewer is non-nil the JSON API requires a ServiceAccount bearer token whose
// subject is in allowedSubjects OR whose namespace is in allowedNamespaces
// (/healthz exempt). A nil reviewer leaves the API unauthenticated. These are
// parameters (rather than read from f) so wiring tests can inject a fake
// reviewer.
func buildAPIMux(pool *pgxpool.Pool, registry *providers.Registry, f *flags, log logr.Logger, reviewer serviceauth.TokenReviewer, allowedSubjects, allowedNamespaces []string) (http.Handler, *api.SessionService, func()) {
	svcCfg := api.ServiceConfig{}
	// Default cleanup is a no-op; only the enterprise audit-logger path below
	// replaces it with a real Close() call. Keeping cleanup non-nil lets the
	// main() defer site call it unconditionally.
	cleanup := func() { /* no-op — replaced when enterprise audit is enabled */ }

	// Enterprise: audit logger.
	var auditLogger *audit.Logger
	if f.enterprise {
		auditMetrics := metrics.NewAuditMetrics()
		auditLogger = audit.NewLogger(pool, log, auditMetrics, audit.LoggerConfig{})
		svcCfg.AuditLogger = auditLogger
		cleanup = func() { _ = auditLogger.Close() }
	}

	httpMetrics := api.NewHTTPMetrics(nil)
	httpMetrics.Initialize()

	// Event publisher (reuses the same Redis used for hot cache, if configured).
	svcCfg.EventPublisher = initEventPublisher(registry, log, httpMetrics)

	sessionService := api.NewSessionService(registry, svcCfg, log)
	maxBody := int64(envInt32("MAX_BODY_SIZE", int32(api.DefaultMaxBodySize)))
	handler := api.NewHandler(sessionService, log, maxBody)

	// Wire up eval result + provider call endpoints when Postgres is available.
	if pool != nil {
		evalStore := pgprovider.NewEvalStore(pool)
		evalService := api.NewEvalService(evalStore, log)
		handler.SetEvalService(evalService)

		providerCallsStore := pgprovider.NewProviderCallsStore(pool)
		providerCallsService := api.NewProviderCallsService(providerCallsStore, log)
		handler.SetProviderCallsService(providerCallsService)

		providerUsageStore := pgprovider.NewProviderUsageStore(pool)
		providerUsageService := api.NewProviderUsageService(providerUsageStore, log)
		handler.SetProviderUsageService(providerUsageService)
	}

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	registerEnterpriseRoutes(mux, registry, auditLogger, f, log)

	// Privacy middleware (enterprise only): PII redaction + user opt-out.
	var apiHandler http.Handler = mux
	if f.enterprise {
		wrapped, watcher, k8sClient := wrapPrivacyMiddleware(apiHandler, registry, f.workspace, f.serviceGroup, auditLogger, log)
		apiHandler = wrapped

		if watcher != nil {
			handler.SetPolicyResolver(watcher)

			// Wire per-policy encryption when we have a K8s client.
			if k8sClient != nil {
				factory := &kmsEncryptorFactory{
					kubeClient: k8sClient,
					namespace:  detectNamespace(),
					log:        log,
				}
				wireEncryptionResolver(handler, sessionService, watcher, factory, log)
			}
		}
	}

	// Rate limiting middleware (per-client-IP token bucket).
	rlCfg := api.RateLimitConfigFromEnv()
	rlMiddleware, rlStop := api.NewRateLimitMiddleware(rlCfg)
	origCleanup := cleanup
	cleanup = func() {
		rlStop()
		origCleanup()
	}
	log.V(1).Info("rate limiter initialized", "rps", rlCfg.RPS, "burst", rlCfg.Burst)

	traced := otelhttp.NewHandler(api.TraceLogMiddleware(apiHandler), "session-api",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/healthz"
		}),
	)

	// ServiceAccount auth runs after rate-limiting but around the
	// metrics/trace/handler chain. /healthz is exempt so liveness probes are
	// never gated. A nil reviewer makes this a pass-through (unauthenticated).
	authMW := serviceauth.RequireServiceAccount(reviewer, allowedSubjects, allowedNamespaces, "/healthz")
	return rlMiddleware(authMW(api.MetricsMiddleware(httpMetrics, traced))), sessionService, cleanup
}

// registerEnterpriseRoutes adds audit and the session-tier DSAR erasure endpoint
// when enterprise mode is enabled. The full DSAR request lifecycle
// (deletion-request[s]) is owned by privacy-api (#1676); session-api only exposes
// the per-group session erase that privacy-api orchestrates.
func registerEnterpriseRoutes(mux *http.ServeMux, registry *providers.Registry, auditLogger *audit.Logger, f *flags, log logr.Logger) {
	if f.enterprise && auditLogger != nil {
		ah := audit.NewHandler(auditLogger, log)
		ah.RegisterRoutes(mux)

		// Session-tier DSAR erasure endpoint (#1676): lists + warm-deletes this
		// group's sessions and their media for a subject. privacy-api calls this
		// per service-group when orchestrating an erasure.
		warm, _ := registry.WarmStore()
		eraser := privacy.NewSessionEraser(privacy.NewWarmStoreSessionDeleter(warm), log)
		eraser.SetMediaDeleter(buildMediaDeleter(f, log))
		privacy.NewSessionEraseHandler(eraser, log).RegisterRoutes(mux)
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

// initProviders creates the tiered storage registry (warm/hot/cold) and returns
// a cleanup function that closes all providers in reverse order.
func initProviders(ctx context.Context, f *flags, pool *pgxpool.Pool, log logr.Logger) (*providers.Registry, func(), error) {
	registry := providers.NewRegistry()
	var cleanups []func()

	// Warm store (postgres, using shared pool).
	warmProvider := pgprovider.NewFromPool(pool)
	registry.SetWarmStore(warmProvider)
	cleanups = append(cleanups, func() { _ = warmProvider.Close() })
	log.V(1).Info("warm store initialized")

	// Hot cache (redis, optional). The session-providers/redis Provider
	// supports cluster mode via Addrs[] but session-api wires it as
	// single-master only — same constraint as compaction. Parse URL to
	// extract host:port + password + db.
	if f.redisURL != "" {
		urlOpts, urlErr := goredis.ParseURL(f.redisURL)
		if urlErr != nil {
			return nil, nil, fmt.Errorf("parse redis URL: %w", urlErr)
		}
		redisCfg := redis.DefaultConfig()
		redisCfg.Addrs = []string{urlOpts.Addr}
		redisCfg.Password = urlOpts.Password
		redisCfg.DB = urlOpts.DB
		redisCfg.MaxMessagesPerSession = int(envInt32("REDIS_MAX_MESSAGES", int32(redisCfg.MaxMessagesPerSession)))
		hotProvider, err := redis.New(redisCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("creating redis provider: %w", err)
		}
		registry.SetHotCache(hotProvider)
		cleanups = append(cleanups, func() { _ = hotProvider.Close() })
		log.V(1).Info("hot cache initialized", "addr", urlOpts.Addr)
	}

	// Cold archive (optional).
	if f.coldBackend != "" && f.coldBucket != "" {
		coldCfg := cold.DefaultConfig()
		coldCfg.Backend = cold.BackendType(f.coldBackend)
		coldCfg.Bucket = f.coldBucket
		switch coldCfg.Backend {
		case cold.BackendS3:
			coldCfg.S3 = &cold.S3Config{
				Region:   f.coldRegion,
				Endpoint: f.coldEndpoint,
			}
		case cold.BackendGCS:
			coldCfg.GCS = &cold.GCSConfig{}
		case cold.BackendAzure:
			coldCfg.Azure = &cold.AzureConfig{}
		}
		coldProvider, err := cold.New(ctx, coldCfg)
		if err != nil {
			return nil, nil, fmt.Errorf("creating cold archive provider: %w", err)
		}
		registry.SetColdArchive(coldProvider)
		cleanups = append(cleanups, func() { _ = coldProvider.Close() })
		log.V(1).Info("cold archive initialized", "backend", f.coldBackend)
	}

	cleanup := func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	return registry, cleanup, nil
}

// initEventPublisher creates an EventPublisher backed by the Redis client from
// the hot cache provider, if available. Returns nil when Redis is not configured.
func initEventPublisher(registry *providers.Registry, log logr.Logger, httpMetrics ...*api.HTTPMetrics) api.EventPublisher {
	hot, err := registry.HotCache()
	if err != nil {
		log.V(1).Info("event publisher skipped", "reason", "no hot cache")
		return nil
	}
	rp, ok := hot.(redisClientProvider)
	if !ok {
		log.V(1).Info("event publisher skipped", "reason", "hot cache does not expose Redis client")
		return nil
	}
	log.V(1).Info("event publisher initialized")
	var m *api.HTTPMetrics
	if len(httpMetrics) > 0 {
		m = httpMetrics[0]
	}
	return api.NewRedisEventPublisher(rp.RedisClient(), log, m)
}

// startOTLPServers creates and starts the OTLP gRPC and HTTP servers.
// Returns the servers for graceful shutdown.
//
// reviewer, allowedSubjects and allowedNamespaces gate both listeners with
// ServiceAccount auth (the same as the JSON API): the gRPC server installs the
// serviceauth interceptors and the HTTP handler is wrapped with
// serviceauth.RequireServiceAccount. A nil reviewer makes both pass-through
// (auth disabled), leaving OTLP behavior unchanged. These are parameters (rather
// than read from f) so wiring tests can inject a fake reviewer.
func startOTLPServers(f *flags, svc *api.SessionService, log logr.Logger, reviewer serviceauth.TokenReviewer, allowedSubjects, allowedNamespaces []string) (*grpc.Server, *http.Server) {
	transformer := otlp.NewTransformer(svc, log)

	// gRPC server. The OTLP TraceService only registers the unary Export RPC, so
	// the unary interceptor is what gates ingest; the stream interceptor is added
	// defensively (harmless when no streaming RPC is registered).
	grpcSrv := grpc.NewServer(otlpGRPCServerOptions(reviewer, allowedSubjects, allowedNamespaces)...)
	receiver := otlp.NewReceiver(transformer, log)
	coltracepb.RegisterTraceServiceServer(grpcSrv, receiver)

	go func() {
		lis, err := net.Listen("tcp", f.otlpGRPCAddr)
		if err != nil {
			log.Error(err, "failed to listen for OTLP gRPC", "addr", f.otlpGRPCAddr)
			return
		}
		log.Info("starting OTLP gRPC server", "addr", f.otlpGRPCAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Error(err, "OTLP gRPC server error")
		}
	}()

	// HTTP server.
	httpSrv := &http.Server{
		Addr:    f.otlpHTTPAddr,
		Handler: buildOTLPHTTPHandler(transformer, log, reviewer, allowedSubjects, allowedNamespaces),
	}
	go func() {
		log.Info("starting OTLP HTTP server", "addr", f.otlpHTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "OTLP HTTP server error")
		}
	}()

	return grpcSrv, httpSrv
}

// otlpGRPCServerOptions builds the grpc.ServerOptions for the OTLP gRPC server,
// installing the ServiceAccount auth interceptors. A nil reviewer leaves the
// interceptors as pass-through (the serviceauth package handles nil). Extracted
// so wiring tests can assert the server is constructed with auth.
func otlpGRPCServerOptions(reviewer serviceauth.TokenReviewer, allowedSubjects, allowedNamespaces []string) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.UnaryInterceptor(serviceauth.UnaryServerInterceptor(reviewer, allowedSubjects, allowedNamespaces)),
		grpc.StreamInterceptor(serviceauth.StreamServerInterceptor(reviewer, allowedSubjects, allowedNamespaces)),
	}
}

// buildOTLPHTTPHandler assembles the OTLP/HTTP handler wrapped with
// ServiceAccount auth. The OTLP HTTP listener only serves the export endpoint
// (no /healthz), so there are no exempt paths. A nil reviewer makes the wrapper
// pass-through. Extracted so the build path is testable.
func buildOTLPHTTPHandler(transformer *otlp.Transformer, log logr.Logger, reviewer serviceauth.TokenReviewer, allowedSubjects, allowedNamespaces []string) http.Handler {
	handler := otlp.NewHandler(transformer, log)
	otlpMux := http.NewServeMux()
	handler.RegisterRoutes(otlpMux)

	authMW := serviceauth.RequireServiceAccount(reviewer, allowedSubjects, allowedNamespaces)
	return authMW(otlpMux)
}

// buildMediaDeleter returns a MediaDeleter based on the configured cold storage
// backend. When no object storage is configured, nil is returned and the
// deletion service retains its default NoOpMediaDeleter.
func buildMediaDeleter(f *flags, log logr.Logger) privacy.MediaDeleter {
	if f.coldBackend == "" || f.coldBucket == "" {
		log.V(1).Info("media deleter skipped", "reason", "no object storage configured")
		return nil
	}

	client, err := buildObjectStoreClient(f, log)
	if err != nil {
		log.Error(err, "media deleter creation failed, using no-op")
		return privacy.NoOpMediaDeleter{}
	}

	log.Info("media deleter enabled", "backend", f.coldBackend, "bucket", f.coldBucket)
	return privacy.NewObjectStoreMediaDeleter(client, f.coldBucket, "sessions/", log)
}

func buildObjectStoreClient(f *flags, log logr.Logger) (privacy.ObjectStoreClient, error) {
	switch cold.BackendType(f.coldBackend) {
	case cold.BackendS3:
		return buildS3ObjectStoreClient(f, log)
	case cold.BackendGCS:
		return buildGCSObjectStoreClient(log)
	case cold.BackendAzure:
		return buildAzureObjectStoreClient(f, log)
	default:
		return nil, fmt.Errorf("unsupported cold backend for media deleter: %s", f.coldBackend)
	}
}

func buildS3ObjectStoreClient(f *flags, log logr.Logger) (*privacy.S3ObjectStoreClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(f.coldRegion),
	)
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}
	opts := []func(*s3sdk.Options){}
	if f.coldEndpoint != "" {
		opts = append(opts, func(o *s3sdk.Options) {
			o.BaseEndpoint = aws.String(f.coldEndpoint)
			o.UsePathStyle = true
		})
	}
	client := s3sdk.NewFromConfig(cfg, opts...)
	return privacy.NewS3ObjectStoreClient(client, log), nil
}

func buildGCSObjectStoreClient(log logr.Logger) (*privacy.GCSObjectStoreClient, error) {
	client, err := gcsstorage.NewClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("creating GCS client: %w", err)
	}
	return privacy.NewGCSObjectStoreClient(client, log), nil
}

func buildAzureObjectStoreClient(f *flags, log logr.Logger) (*privacy.AzureObjectStoreClient, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("creating Azure credential: %w", err)
	}
	endpoint := f.coldEndpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://%s.blob.core.windows.net", f.coldBucket)
	}
	client, err := azblob.NewClient(endpoint, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Azure blob client: %w", err)
	}
	return privacy.NewAzureObjectStoreClient(client, log), nil
}

// newPrivacyWatcherScheme builds the scheme for the privacy PolicyWatcher's
// client. The watcher lists THREE cluster-scoped kinds: SessionPrivacyPolicy
// (EE api) plus Workspace and AgentRuntime (core api). Registering only the EE
// api left WorkspaceList/AgentRuntimeList unknown, so the watcher failed to
// start with "no kind is registered for the type v1alpha1.WorkspaceList" even
// after its RBAC was granted (#1567; sibling of the facade scheme fix #1573).
func newPrivacyWatcherScheme() *k8sruntime.Scheme {
	scheme := k8sruntime.NewScheme()
	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))     // EE: SessionPrivacyPolicy
	utilruntime.Must(coreomniav1alpha1.AddToScheme(scheme)) // core: Workspace, AgentRuntime
	utilruntime.Must(corev1.AddToScheme(scheme))
	return scheme
}

// wrapPrivacyMiddleware creates and returns the privacy middleware handler
// alongside the PolicyWatcher and the Kubernetes client used to build it.
// The watcher is returned so callers can wire it into the session handler's
// PolicyResolver and install an OnPolicyChange callback for cache invalidation.
// The k8s client is returned so callers can build KMS encryptor factories.
//
// When the K8s API is unreachable (e.g., in tests), the middleware is skipped
// and the original handler is returned unchanged, with nil watcher and client.
//
// workspace and serviceGroup are used by resolvePrivacyPrefStore to look up
// the privacy-api URL from the Workspace CRD status when PRIVACY_API_URL is
// not set as an environment variable.
func wrapPrivacyMiddleware(
	next http.Handler,
	registry *providers.Registry,
	workspace, serviceGroup string,
	auditLogger *audit.Logger,
	log logr.Logger,
) (http.Handler, *privacy.PolicyWatcher, client.Client) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Info("privacy middleware skipped", "reason", "no in-cluster kubeconfig")
		return next, nil, nil
	}

	k8sClient, err := client.New(kubeConfig, client.Options{Scheme: newPrivacyWatcherScheme()})
	if err != nil {
		log.Error(err, "privacy middleware skipped", "reason", "k8s client creation failed")
		return next, nil, nil
	}

	watcher := privacy.NewPolicyWatcher(k8sClient, log, workspace, detectNamespace())

	// Start the watcher asynchronously; it syncs the cache in the background.
	go func() {
		if startErr := watcher.Start(context.Background()); startErr != nil {
			log.Error(startErr, "policy watcher start failed")
		}
	}()

	sessionLookup := privacy.NewWarmStoreSessionLookup(registry)
	sessionCache := privacy.NewSessionMetadataCache(sessionLookup, 10000)
	redactor := redaction.NewRedactor()
	prefStore := resolvePrivacyPrefStore(context.Background(), workspace, serviceGroup, k8sClient, log)

	middleware := privacy.NewPrivacyMiddleware(watcher, sessionCache, redactor, prefStore, log)
	if auditLogger != nil {
		middleware.SetAuditLogger(auditLogger)
	}
	log.Info("privacy middleware enabled")
	return middleware.Wrap(next), watcher, k8sClient
}

// resolvePrivacyPrefStore selects the PreferencesStore implementation for the
// privacy middleware. Resolution order:
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
) privacy.PreferencesStore {
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

// auditSourceService identifies this service in forwarded audit events; the
// privacy-api hub keys idempotency on (source_service, source_id).
const auditSourceService = "session-api"

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
		log.V(1).Info("audit forwarder URL not resolved", "reason", "no in-cluster kubeconfig")
		return ""
	}
	k8sClient, err := client.New(kubeConfig, client.Options{Scheme: newPrivacyWatcherScheme()})
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
// forwarding cannot run (audit disabled, or no privacy URL), so the caller can
// skip starting the goroutine. The ServiceAccount token source authenticates the
// ingest POSTs the same way the consent push does.
func buildAuditForwarder(
	enterprise bool,
	pool *pgxpool.Pool,
	privacyURL string,
	reg prometheus.Registerer,
	log logr.Logger,
) *audit.Forwarder {
	if !enterprise || pool == nil || privacyURL == "" {
		return nil
	}
	ts := serviceauth.NewTokenSource("", 0)
	return audit.NewForwarder(pool, privacyURL, auditSourceService, ts, 0, 0, reg, log)
}

// wireEncryptionResolver builds a PerPolicyEncryptorResolver from the policy
// watcher and installs it on the handler. It also registers an OnPolicyChange
// callback so the resolver's cache is invalidated whenever a policy's
// EncryptionConfig changes.
//
// Extracted as a standalone function for testability.
func wireEncryptionResolver(
	h *api.Handler,
	svc *api.SessionService,
	watcher *privacy.PolicyWatcher,
	factory EncryptorFactory,
	log logr.Logger,
) {
	encSource := func(sessionID string) (*omniav1alpha1.EncryptionConfig, bool) {
		sess, err := svc.GetSession(context.Background(), sessionID)
		if err != nil || sess == nil {
			return nil, false
		}
		eff := watcher.GetEffectivePolicy(sess.Namespace, sess.AgentName)
		if eff == nil {
			return nil, false
		}
		enc := eff.Encryption
		if !enc.Enabled {
			return nil, false
		}
		return &enc, true
	}

	resolver := NewPerPolicyEncryptorResolver(encSource, factory, log)
	h.SetEncryptorResolver(resolver)
	watcher.OnPolicyChange(makeEncryptionInvalidator(resolver))
	log.Info("encryption resolver wired")
}

// makeEncryptionInvalidator returns a PolicyChangeCallback that drops stale
// encryptor cache entries when a SessionPrivacyPolicy's encryption config
// changes. It invalidates both the old (to remove stale entries) and the new
// (to force a rebuild on the next request) (provider, keyID) pairs.
func makeEncryptionInvalidator(resolver *PerPolicyEncryptorResolver) privacy.PolicyChangeCallback {
	return func(oldP, newP *omniav1alpha1.SessionPrivacyPolicy) {
		if oldP != nil && oldP.Spec.Encryption != nil && oldP.Spec.Encryption.Enabled {
			resolver.Invalidate(string(oldP.Spec.Encryption.KMSProvider), oldP.Spec.Encryption.KeyID)
		}
		if newP != nil && newP.Spec.Encryption != nil && newP.Spec.Encryption.Enabled {
			resolver.Invalidate(string(newP.Spec.Encryption.KMSProvider), newP.Spec.Encryption.KeyID)
		}
	}
}
