/*
Copyright 2026 Altaira Labs.

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

// Package promptkit exposes the omnia↔PromptKit runtime as a public, importable
// Go package. It is a thin facade that constructs and runs the runtime server
// that lives in internal/runtime: config → server-option wiring (provider,
// session, media, memory, evals, tools) → gRPC server (policy interceptors +
// health) → HTTP health/metrics → graceful shutdown.
//
// A first-party binary uses FromEnv (operator-injected OMNIA_* config); a
// downstream, separate-repo PromptKit runtime imports this package and brings
// its own SDK behaviour through WithSDKOptions — an opaque sdk.Option
// passthrough, so this package never references an unpublished PromptKit API and
// Omnia CI stays on the published SDK.
package promptkit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"

	pkevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	"github.com/AltairaLabs/PromptKit/runtime/logger"
	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/AltairaLabs/PromptKit/runtime/statestore"

	memoryhttpclient "github.com/altairalabs/omnia/internal/memory/httpclient"
	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/schema"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/session/httpclient"
)

// errUnregisteredEvalTypes flags eval types in the pack with no registered
// handler; the offending types are attached as a structured log field.
var errUnregisteredEvalTypes = errors.New("unregistered eval types")

// metricsNamespace is the Prometheus namespace for all runtime metrics.
const metricsNamespace = "omnia"

// httpReadHeaderTimeout bounds the health server's header read.
const httpReadHeaderTimeout = 10 * time.Second

// shutdownTimeout bounds the overall graceful shutdown; grpcStopTimeout bounds
// the gRPC GracefulStop before falling back to a hard Stop.
const (
	shutdownTimeout = 30 * time.Second
	grpcStopTimeout = 10 * time.Second
)

// Runtime is a constructed, ready-to-serve omnia↔PromptKit runtime. Build one
// with New (explicit config) or FromEnv (operator-injected config), then call
// Serve to run it and Close to release its resources.
type Runtime struct {
	server         *pkruntime.Server
	cfg            *pkruntime.Config
	log            logr.Logger
	tracing        *tracing.Provider
	gatherers      prometheus.Gatherers
	readyValidator *schema.SchemaValidator
	evalDefs       []pkevals.EvalDef
	cleanups       []func()
	logCleanup     func()
}

// buildDeps groups the constructed dependencies buildServerOpts folds into the
// runtime server's option slice.
type buildDeps struct {
	collector *sdkmetrics.Collector
	evalDefs  []pkevals.EvalDef
	store     statestore.Store
	tracing   *tracing.Provider
	mediaOpts []pkruntime.ServerOption
}

// New constructs a Runtime from an explicit config. It performs no process-wide
// side effects (no propagator install, no k8s status reporting) so it is safe to
// call from tests and downstream code that manages those concerns itself. Use
// FromEnv for the operator-injected entry point.
func New(cfg *pkruntime.Config, opts ...Option) (*Runtime, error) {
	b := applyOptions(opts)
	logCleanup, err := b.ensureLogger()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return newFromBuilder(cfg, b, logCleanup)
}

// FromEnv loads the operator-injected OMNIA_* configuration and constructs a
// Runtime from it. It is the entry point a runtime binary's main() calls: the
// operator injects the same environment it does for the built-in runtime. It
// installs the global trace propagator, resolves the pack entry point, and
// best-effort self-reports pack validation + capabilities to the AgentRuntime
// status.
func FromEnv(opts ...Option) (*Runtime, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	b := applyOptions(opts)
	logCleanup, err := b.ensureLogger()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	if b.sdkLogger != nil {
		logger.SetLogger(b.sdkLogger)
	}

	cfg, err := pkruntime.LoadConfigWithContext(context.Background())
	if err != nil {
		b.log.Error(err, "failed to load configuration")
		runCleanup(logCleanup)
		return nil, err
	}
	cfg.PromptName = pkruntime.ResolvePackEntry(cfg.PromptPackPath, cfg.PromptName, b.log)
	logStartup(b.log, cfg)

	rt, err := newFromBuilder(cfg, b, logCleanup)
	if err != nil {
		runCleanup(logCleanup)
		return nil, err
	}
	rt.reportStartup(context.Background())
	return rt, nil
}

// newFromBuilder is the shared construction core for New and FromEnv: it builds
// every runtime dependency from cfg and the resolved builder, assembles the
// server option slice, creates the server, and initializes tools.
func newFromBuilder(cfg *pkruntime.Config, b *builder, logCleanup func()) (*Runtime, error) {
	log := b.log

	collectorRegistry := prometheus.NewRegistry()
	collector := newCollector(cfg, collectorRegistry)
	registerRuntimeInfoGauge(cfg, collectorRegistry)

	evalDefs := loadEvalDefs(cfg, log)

	store, err := newStateStore(cfg, log)
	if err != nil {
		runCleanup(logCleanup)
		return nil, fmt.Errorf("state store: %w", err)
	}

	tracingProvider := newTracingProvider(cfg, log)
	mediaOpts, mediaCleanup := mediaStorageServerOpts(log)

	serverOpts := buildServerOpts(cfg, b, buildDeps{
		collector: collector,
		evalDefs:  evalDefs,
		store:     store,
		tracing:   tracingProvider,
		mediaOpts: mediaOpts,
	})
	warnIfCustomTruncation(log, cfg.TruncationStrategy)

	server := pkruntime.NewServer(serverOpts...)
	initTools(cfg, server, log)

	rt := &Runtime{
		server:         server,
		cfg:            cfg,
		log:            log,
		tracing:        tracingProvider,
		gatherers:      mergedGatherers(collectorRegistry),
		readyValidator: schema.NewSchemaValidatorWithOptions(log, nil, 0),
		evalDefs:       evalDefs,
		logCleanup:     logCleanup,
	}
	if mediaCleanup != nil {
		rt.cleanups = append(rt.cleanups, mediaCleanup)
	}
	return rt, nil
}

// buildServerOpts assembles the pkruntime.ServerOption slice from cfg, the
// resolved builder (logger + opaque SDK options), and the constructed
// dependencies. It preserves the option order the runtime binary historically
// used so behaviour is identical.
func buildServerOpts(cfg *pkruntime.Config, b *builder, d buildDeps) []pkruntime.ServerOption {
	opts := []pkruntime.ServerOption{
		pkruntime.WithLogger(b.log),
		pkruntime.WithStateStore(d.store),
	}
	if b.sdkLogger != nil {
		opts = append(opts, pkruntime.WithSlogLogger(b.sdkLogger))
	}
	opts = append(opts, configDerivedServerOpts(cfg)...)
	if d.tracing != nil {
		opts = append(opts, pkruntime.WithTracingProvider(d.tracing))
	}
	if path := getEnvOrDefault(envPromptPackManifestPath, ""); path != "" {
		opts = append(opts, pkruntime.WithSkillManifest(path))
	}
	if cfg.PromptPackVersion != "" {
		opts = append(opts, pkruntime.WithPromptPackVersion(cfg.PromptPackVersion))
	}
	if cfg.SessionAPIURL != "" {
		sessionStore := httpclient.NewStore(cfg.SessionAPIURL, b.log, httpclient.WithSource(session.SourceRuntime))
		opts = append(opts, pkruntime.WithSessionStore(sessionStore))
		b.log.Info("session recording enabled", "sessionAPIURL", cfg.SessionAPIURL)
	}
	opts = append(opts, d.mediaOpts...)
	opts = append(opts, memoryServerOpts(cfg, b.log)...)
	opts = append(opts, pkruntime.WithEvalCollector(d.collector))
	if len(d.evalDefs) > 0 {
		opts = append(opts, pkruntime.WithEvalDefs(d.evalDefs))
	}
	if len(b.sdkOpts) > 0 {
		opts = append(opts, pkruntime.WithSDKOptions(b.sdkOpts...))
	}
	return opts
}

// memoryServerOpts wires the cross-session memory store when memory is enabled
// and a memory-api URL is configured, returning nil (a logged no-op) otherwise.
func memoryServerOpts(cfg *pkruntime.Config, log logr.Logger) []pkruntime.ServerOption {
	if !cfg.MemoryEnabled {
		return nil
	}
	if cfg.MemoryAPIURL == "" {
		log.Info("memory enabled but no memory-api URL configured, skipping")
		return nil
	}
	memStore := memoryhttpclient.NewStore(cfg.MemoryAPIURL, log)
	opts := []pkruntime.ServerOption{
		pkruntime.WithMemoryStore(memStore),
		pkruntime.WithMemoryModes(cfg.MemoryRetrievalEnabled, cfg.MemoryToolsEnabled),
	}
	if cfg.WorkspaceUID != "" {
		opts = append(opts, pkruntime.WithWorkspaceUID(cfg.WorkspaceUID))
	}
	log.Info("memory store wired",
		"memoryAPIURL", cfg.MemoryAPIURL, "workspaceUID", cfg.WorkspaceUID,
		"memoryStrategy", cfg.MemoryStrategy, "hasDenyCEL", cfg.MemoryDenyCEL != "")
	return opts
}

// newCollector builds the PromptKit metrics collector for all pipeline + eval
// metrics, registered on the runtime's isolated registry.
func newCollector(cfg *pkruntime.Config, reg *prometheus.Registry) *sdkmetrics.Collector {
	return sdkmetrics.NewCollector(sdkmetrics.CollectorOpts{
		Registerer: reg,
		Namespace:  metricsNamespace,
		ConstLabels: prometheus.Labels{
			"agent":           cfg.AgentName,
			"namespace":       cfg.Namespace,
			"promptpack_name": cfg.PromptPackName,
		},
	})
}

// registerRuntimeInfoGauge registers the always-1 info gauge that confirms
// metric scraping is wired. It is registered on the runtime's own collector
// registry (not the global default) so repeated Runtime construction in-process
// — as tests do — never panics on a duplicate registration.
func registerRuntimeInfoGauge(cfg *pkruntime.Config, reg *prometheus.Registry) {
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "omnia_runtime_info",
		Help: "Runtime information gauge (always 1)",
		ConstLabels: prometheus.Labels{
			"agent":     cfg.AgentName,
			"namespace": cfg.Namespace,
		},
	})
	g.Set(1)
	reg.MustRegister(g)
}

// Serve runs the runtime: it starts the gRPC server (policy interceptors +
// health) on cfg.GRPCPort and the HTTP health/metrics server on cfg.HealthPort,
// then blocks until ctx is cancelled, at which point it gracefully shuts both
// down (health first, then gRPC with a hard-stop fallback). Serve returns only
// after shutdown completes; a nil return is a clean shutdown.
func (r *Runtime) Serve(ctx context.Context) error {
	grpcServer := r.newGRPCServer()
	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%d", r.cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("listen gRPC on :%d: %w", r.cfg.GRPCPort, err)
	}
	go func() {
		r.log.Info("gRPC server starting", "port", r.cfg.GRPCPort)
		if serveErr := grpcServer.Serve(grpcListener); serveErr != nil {
			r.log.Error(serveErr, "gRPC server error")
		}
	}()

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", r.cfg.HealthPort),
		Handler:           r.healthMux(),
		ReadHeaderTimeout: httpReadHeaderTimeout,
	}
	go func() {
		r.log.Info("health server starting", "port", r.cfg.HealthPort)
		if serveErr := httpServer.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			r.log.Error(serveErr, "health server error")
		}
	}()

	<-ctx.Done()
	r.log.Info("shutting down...")
	r.shutdown(grpcServer, httpServer)
	r.log.Info("shutdown complete")
	return nil
}

// shutdown stops the health server, then gracefully stops the gRPC server,
// falling back to a hard stop if in-flight streams do not finish within
// grpcStopTimeout. The order matches the runtime binary's historical sequence.
func (r *Runtime) shutdown(grpcServer *grpc.Server, httpServer *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		r.log.Error(err, "failed to shutdown health server")
	}

	grpcDone := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(grpcDone)
	}()
	select {
	case <-grpcDone:
	case <-time.After(grpcStopTimeout):
		r.log.Info("gRPC graceful stop timed out, forcing stop")
		grpcServer.Stop()
	}
}

// Close releases the runtime's resources: it closes the runtime server, runs any
// registered cleanups (media storage), and flushes the logger. It is safe to
// call after Serve returns, or on a construction path that never served.
func (r *Runtime) Close() error {
	var err error
	if r.server != nil {
		err = r.server.Close()
	}
	for _, c := range r.cleanups {
		c()
	}
	runCleanup(r.logCleanup)
	return err
}

// runCleanup invokes a cleanup func when non-nil.
func runCleanup(c func()) {
	if c != nil {
		c()
	}
}

// logStartup emits the single structured startup line describing the resolved
// runtime configuration.
func logStartup(log logr.Logger, cfg *pkruntime.Config) {
	log.Info("starting runtime",
		"agent", cfg.AgentName,
		"namespace", cfg.Namespace,
		"grpcPort", cfg.GRPCPort,
		"healthPort", cfg.HealthPort,
		"packPath", cfg.PromptPackPath,
		"promptName", cfg.PromptName,
		"providerType", cfg.ProviderType,
		"model", cfg.Model,
		"baseURL", cfg.BaseURL,
		"mockProvider", cfg.MockProvider,
		"toolsConfigPath", cfg.ToolsConfigPath,
		"tracingEnabled", cfg.TracingEnabled,
		"evalEnabled", cfg.EvalEnabled,
		"sessionAPIURL", cfg.SessionAPIURL,
		"memoryEnabled", cfg.MemoryEnabled)
}
