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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	goredis "github.com/redis/go-redis/v9"
	corev1 "k8s.io/api/core/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/cmd/privacy-api/migrations"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
	"github.com/altairalabs/omnia/internal/serviceauth"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
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

	// ServiceAccount auth (opt-in).
	authEnabled           bool
	authAllowedSubjects   string
	authAllowedNamespaces string
	authAudiences         string
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
	flag.BoolVar(&f.authEnabled, "auth-enabled", false,
		"Require Kubernetes ServiceAccount bearer-token auth on the JSON API")
	flag.StringVar(&f.authAllowedSubjects, "auth-allowed-subjects", "",
		"Comma-separated allowed ServiceAccount subjects (exact match)")
	flag.StringVar(&f.authAllowedNamespaces, "auth-allowed-namespaces", "",
		"Comma-separated trusted namespaces; any SA in these namespaces is allowed")
	flag.StringVar(&f.authAudiences, "auth-audiences", "",
		"Comma-separated audiences for projected tokens (optional)")
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

	// ServiceAccount auth — same env vars the operator stamps via
	// applySessionAPIServerAuthEnv in internal/controller/service_auth.go.
	// Mirror session-api's mapping exactly (CLI flag wins when non-default,
	// env fills in when flag is still at its zero/default value).
	envBoolFallback(&f.authEnabled, "SESSION_API_AUTH_ENABLED")
	envFallback(&f.authAllowedSubjects, "", "SESSION_API_AUTH_ALLOWED_SUBJECTS")
	envFallback(&f.authAllowedNamespaces, "", "SESSION_API_AUTH_ALLOWED_NAMESPACES")
	envFallback(&f.authAudiences, "", "SESSION_API_AUTH_AUDIENCES")
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

	// --- Optional Redis warm cache ---
	var optOutStore privacy.PreferencesStore = base
	cacheTTL := envDuration("PRIVACY_CACHE_TTL", defaultCacheTTL)
	if f.redisURL != "" {
		opts, parseErr := goredis.ParseURL(f.redisURL)
		if parseErr != nil {
			return fmt.Errorf("parsing redis URL: %w", parseErr)
		}
		redisClient := goredis.NewClient(opts)
		if pingErr := redisClient.Ping(ctx).Err(); pingErr != nil {
			return fmt.Errorf("redis ping failed: %w", pingErr)
		}
		kv := &redisKV{client: redisClient, prefix: "privacy:"}
		optOutStore = privacy.NewCachedPreferencesStore(base, kv, cacheTTL, log)
		log.V(1).Info("redis warm cache enabled", "addr", opts.Addr, "ttl", cacheTTL)
	}

	// --- ServiceAccount auth (opt-in) ---
	reviewer, allowedSubjects, allowedNamespaces, err := buildServiceAuth(f, log)
	if err != nil {
		return err
	}

	// --- Build API mux ---
	apiMux := http.NewServeMux()
	// /healthz on the API mux so the auth middleware can exempt it from token checks.
	apiMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	registerRoutes(apiMux, optOutStore, base, log)

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
	)

	// --- Wait for shutdown ---
	<-ctx.Done()
	log.Info("shutting down")
	shutdownServers(log, apiSrv, healthSrv, metricsSrv)
	return nil
}

// registerRoutes mounts all privacy handlers on mux. optOutStore is the
// (optionally cached) PreferencesStore; concrete is the raw postgres store
// the consent and stats handlers require.
func registerRoutes(
	mux *http.ServeMux,
	optOutStore privacy.PreferencesStore,
	concrete *privacy.PreferencesPostgresStore,
	log logr.Logger,
) {
	privacy.NewOptOutHandler(optOutStore, log).RegisterRoutes(mux)
	// TODO(#1642-P2): wire privacy-api audit if consent audit must live here.
	privacy.NewConsentHandler(concrete, nil, log).RegisterRoutes(mux)
	privacy.NewConsentStatsHandler(concrete, log).RegisterRoutes(mux)
	privacy.NewEnforcementStatsHandler(concrete, log).RegisterRoutes(mux)
	privacy.NewConsentUsersHandler(concrete, log).RegisterRoutes(mux)
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
	namespace := detectNamespace()
	privCfg, err := cr.ResolvePrivacyConfig(context.Background(), f.workspace, namespace)
	if err != nil {
		return fmt.Errorf("resolving privacy config: %w", err)
	}
	f.postgresConn = privCfg.PostgresConn
	log.V(1).Info("privacy config resolved", "workspace", f.workspace)
	return nil
}

// detectNamespace returns the Kubernetes namespace this process is running in.
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
		log.Info("starting server", "server", name, "addr", addr)
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
