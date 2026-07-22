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
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
	"github.com/altairalabs/omnia/pkg/session/httpclient"
)

const (
	shutdownTimeout = 30 * time.Second
	readTimeout     = 10 * time.Second
	writeTimeout    = 10 * time.Second
	idleTimeout     = 120 * time.Second
)

func main() {
	// Initialize global OpenTelemetry text map propagator for trace context propagation.
	// This must be set before any gRPC operations to ensure trace context flows through gRPC calls.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Initialize logger
	log, syncLog, err := logging.NewLogger()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer syncLog()

	// Load configuration — prefers CRD reading, falls back to env vars
	cfg, err := agent.LoadConfig(context.Background())
	if err != nil {
		log.Error(err, "failed to load configuration")
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Error(err, "invalid configuration")
		os.Exit(1)
	}

	log.Info("starting agent",
		"agent", cfg.AgentName,
		"namespace", cfg.Namespace,
		"mode", cfg.Mode,
		"facade", cfg.FacadeType,
		"port", cfg.FacadePort,
		"handler", cfg.HandlerMode,
		"tracingEnabled", cfg.TracingEnabled,
	)

	// Initialize tracing if enabled
	var tracingProvider *tracing.Provider
	if cfg.TracingEnabled {
		tracingCfg := tracing.Config{
			Enabled:        true,
			Endpoint:       cfg.TracingEndpoint,
			ServiceName:    fmt.Sprintf("omnia-facade-%s", cfg.AgentName),
			ServiceVersion: "1.0.0",
			Environment:    cfg.Namespace,
			SampleRate:     cfg.TracingSampleRate,
			Insecure:       cfg.TracingInsecure,
			ExtraAttributes: []attribute.KeyValue{
				attribute.String("omnia.workspace.name", cfg.WorkspaceName),
				// omnia.runtime.mode lets distributed-trace consumers
				// filter function-mode pods apart from agent-mode without
				// renaming the per-agent ServiceName.
				attribute.String("omnia.runtime.mode", cfg.Mode),
			},
		}

		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
		var tracingErr error
		tracingProvider, tracingErr = tracing.NewProvider(initCtx, tracingCfg)
		initCancel()
		if tracingErr != nil {
			log.Error(tracingErr, "failed to initialize tracing")
			// Continue without tracing - it's optional
		} else {
			tracingProvider = tracingProvider.WithLogger(log)
			// Set the global tracer provider so otelhttp (used by the
			// session-api httpclient) creates real spans and propagates
			// trace context into outbound HTTP calls.
			otel.SetTracerProvider(tracingProvider.TracerProvider())
			defer func() {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer shutdownCancel()
				if err := tracingProvider.Shutdown(shutdownCtx); err != nil {
					log.Error(err, "failed to shutdown tracing provider")
				}
			}()
			log.Info("tracing initialized",
				"endpoint", cfg.TracingEndpoint,
				"sampleRate", cfg.TracingSampleRate)
		}
	}

	// Branch on AgentRuntime.spec.mode first: function-mode pods run a
	// one-shot HTTP facade alongside the same runtime sidecar; agent-mode
	// pods continue with the existing WebSocket / A2A flows.
	if cfg.Mode == agent.ModeFunction {
		runFunctionsFacade(cfg, log, tracingProvider)
		return
	}

	// Branch on facade type: A2A runs the SDK in-process (no runtime sidecar),
	// while WebSocket uses the traditional facade + gRPC runtime architecture.
	if cfg.FacadeType == agent.FacadeTypeA2A {
		runA2AFacade(cfg, log, tracingProvider)
	} else {
		runWebSocketFacade(cfg, log, tracingProvider)
	}
}

// serviceURLResolver is the subset of *servicediscovery.Resolver the session
// store init needs. An interface so the store-selection logic is unit-testable.
type serviceURLResolver interface {
	ResolveServiceURLs(ctx context.Context, serviceGroup string) (*servicediscovery.ServiceURLs, error)
}

// initSessionStore returns the session store and its mode (for the
// omnia_agent_session_store metric the caller sets).
func initSessionStore(log logr.Logger) (session.Store, string, error) {
	return sessionStoreFromResolver(context.Background(), servicediscovery.NewResolver(buildK8sClient()), log)
}

// sessionStoreFromResolver selects the session store. A service-discovery
// failure is LOUD: it logs at error level and the returned "none" mode drives
// the omnia_agent_session_store metric, because without session-api the agent
// records no session/token/cost product data. Previously this was a silent
// info-level log and the agent reported healthy while dropping all of it
// (issue #1223).
//
// The failure returns NO store rather than an in-memory one. The in-memory
// store satisfied the interface while discarding every write, so an operator
// reading the code saw "a store" and the facade carried a session archive that
// silently went nowhere. Conversations never needed it: resumability comes from
// the runtime's context store (#1876), and the facade treats a nil store as
// "no archive configured" and serves normally.
func sessionStoreFromResolver(
	ctx context.Context, resolver serviceURLResolver, log logr.Logger,
) (session.Store, string, error) {
	urls, err := resolver.ResolveServiceURLs(ctx, resolveServiceGroup())
	if err != nil {
		log.Error(err,
			"session store fallback",
			"reason", "session-api service discovery failed",
			"impact", "no session/token/cost product data; dashboard session views will be empty")
		return nil, agent.SessionStoreModeNone, nil
	}
	log.Info("using session-api HTTP store", "url", urls.SessionURL)
	return httpclient.NewStore(urls.SessionURL, log, httpclient.WithSource(session.SourceFacade)),
		agent.SessionStoreModeHTTPClient, nil
}

func buildK8sClient() client.Client {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil // Not in cluster
	}
	c, err := client.New(cfg, client.Options{Scheme: newFacadeScheme()})
	if err != nil {
		return nil
	}
	return c
}

// newFacadeScheme builds the scheme for the facade's k8s client. It MUST
// register the client-go built-in types (core/v1 in particular) alongside the
// omnia CRD types: the auth chain Lists *corev1.SecretList (client-key store) and
// Gets *corev1.Secret (oidc JWKS). Without core/v1 those calls
// fail with "no kind is registered for the type v1.SecretList" and the facade
// crash-loops whenever spec.externalAuth is set (#1571). Mirrors the operator's
// scheme wiring in cmd/main.go.
func newFacadeScheme() *k8sruntime.Scheme {
	scheme := k8sruntime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
	return scheme
}

func resolveServiceGroup() string {
	if sg := os.Getenv("OMNIA_SERVICE_GROUP"); sg != "" {
		return sg
	}
	return "default"
}

func closeStore(store session.Store, log logr.Logger) {
	if closer, ok := store.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			log.Error(err, "error closing session store")
		}
	}
}

func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// pinger is an optional interface for stores that support lightweight health checks.
type pinger interface {
	Ping(ctx context.Context) error
}

// checkStoreReady returns a non-nil error when the session store is unhealthy.
// It prefers Ping when the store implements pinger; otherwise falls back to a
// dummy GetSession which succeeds even for not-found.
func checkStoreReady(ctx context.Context, store session.Store) error {
	if store == nil {
		return nil
	}
	if p, ok := store.(pinger); ok {
		return p.Ping(ctx)
	}
	_, err := store.GetSession(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil && err != session.ErrSessionNotFound {
		return err
	}
	return nil
}

// checkRuntimeReady returns a non-nil error when the runtime handler is
// unhealthy. Returns nil when handler is not a *agent.RuntimeHandler.
func checkRuntimeReady(ctx context.Context, handler facade.MessageHandler) error {
	rh, ok := handler.(*agent.RuntimeHandler)
	if !ok {
		return nil
	}
	resp, err := rh.Client().Health(ctx)
	if err != nil {
		return fmt.Errorf("runtime unavailable: %w", err)
	}
	if !resp.Healthy {
		return fmt.Errorf("runtime unhealthy: %s", resp.Status)
	}
	return nil
}

func readyzHandler(store session.Store, handler facade.MessageHandler, wsServer *facade.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Report not-ready as soon as the facade enters drain mode so that
		// the load balancer stops sending new traffic before we tear down.
		if wsServer != nil && wsServer.IsDraining() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("draining"))
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := checkStoreReady(ctx, store); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprintf(w, "session store unavailable: %v", err)
			return
		}

		if err := checkRuntimeReady(ctx, handler); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// createHandler creates the appropriate message handler based on configuration.
// Returns the handler and an optional cleanup function.
func createHandler(
	cfg *agent.Config, log logr.Logger, tp *tracing.Provider,
	store session.Store, pool *facade.RecordingPool, policy *facade.RecordingPolicyCache,
) (facade.MessageHandler, func()) {
	switch cfg.HandlerMode {
	case agent.HandlerModeEcho:
		log.Info("using echo handler mode")
		return agent.NewEchoHandler(), nil
	case agent.HandlerModeDemo:
		log.Info("using demo handler mode")
		var demoOpts []agent.DemoHandlerOption
		if tp != nil {
			demoOpts = append(demoOpts, agent.WithDemoTracing(tp))
		}
		return agent.NewDemoHandler(demoOpts...), nil
	case agent.HandlerModeRuntime:
		return createRuntimeHandler(cfg, log, tp, store, pool, policy)
	default:
		log.Info("unknown handler mode, using nil handler", "mode", cfg.HandlerMode)
		return nil, nil
	}
}

// createRuntimeHandler dials the runtime gRPC sidecar (with retry/backoff) and
// wraps it in a RuntimeHandler. store/pool/policy enable the RuntimeClient bus
// recorder — recording conversation messages off the gRPC bus, protocol- and
// runtime-agnostically. Returns (nil, nil) if the runtime never becomes ready.
func createRuntimeHandler(
	cfg *agent.Config, log logr.Logger, tp *tracing.Provider,
	store session.Store, pool *facade.RecordingPool, policy *facade.RecordingPolicyCache,
) (facade.MessageHandler, func()) {
	log.Info("using runtime handler mode", "address", cfg.RuntimeAddress)

	// Build RuntimeClient config with optional tracing. policy is guarded to
	// avoid wrapping a typed-nil in the interface field (which would defeat the
	// nil check in the recorder).
	runtimeCfg := facade.RuntimeClientConfig{
		Address:       cfg.RuntimeAddress,
		DialTimeout:   5 * time.Second,
		Log:           log,
		SessionStore:  store,
		RecordingPool: pool,
	}
	if policy != nil {
		runtimeCfg.RecordingPolicy = policy
	}
	if tp != nil {
		runtimeCfg.TracerProvider = tp.TracerProvider()
	}

	rc := dialRuntimeWithRetry(runtimeCfg, log)
	if rc == nil {
		log.Error(nil, "failed to connect to runtime after retries, falling back to nil handler")
		return nil, nil
	}

	handler := agent.NewRuntimeHandler(rc)
	// Apply CRD-driven client tool timeout, populated from
	// the primary facade's clientToolTimeout by agent.LoadFromCRD.
	if cfg.ClientToolTimeout > 0 {
		handler.SetClientToolTimeout(cfg.ClientToolTimeout)
		log.V(1).Info("client tool timeout override applied", "timeout", cfg.ClientToolTimeout)
	}
	cleanup := func() {
		if err := rc.Close(); err != nil {
			log.Error(err, "error closing runtime client")
		}
	}
	return handler, cleanup
}

// dialRuntimeWithRetry connects to the runtime sidecar with exponential backoff
// (the runtime container may still be starting). Returns nil after maxRetries.
func dialRuntimeWithRetry(runtimeCfg facade.RuntimeClientConfig, log logr.Logger) *facade.RuntimeClient {
	const maxRetries = 10
	backoff := 500 * time.Millisecond
	for i := 0; i < maxRetries; i++ {
		rc, err := facade.NewRuntimeClient(runtimeCfg)
		if err == nil {
			log.Info("connected to runtime", "address", runtimeCfg.Address, "attempt", i+1)
			return rc
		}
		log.Info("waiting for runtime to be ready", "address", runtimeCfg.Address, "attempt", i+1, "error", err.Error())
		time.Sleep(backoff)
		backoff = min(backoff*2, 5*time.Second) // Cap at 5 seconds
	}
	return nil
}

// initMediaStorage creates the appropriate media storage backend based on
// configuration. Returns the storage and an optional cleanup function.
//
// The actual per-backend construction (local/S3/GCS/Azure) lives in
// media.Build — internal/media/builder.go — shared with cmd/runtime's
// initMediaStorage (cmd/runtime/media.go) so the four backend-construction
// paths aren't duplicated per binary. This function's job is just mapping
// the facade's CRD/env-derived agent.Config onto a media.BuilderConfig and
// keeping the facade's existing per-type Info logging.
func initMediaStorage(cfg *agent.Config, log logr.Logger) (media.Storage, func()) {
	if cfg.MediaStorageType == agent.MediaStorageTypeNone {
		log.Info("media storage disabled")
		return nil, nil
	}

	logMediaStorageInit(cfg, log)

	bcfg := media.BuilderConfig{
		Type:           media.BackendType(cfg.MediaStorageType),
		DefaultTTL:     cfg.MediaDefaultTTL,
		MaxFileSize:    cfg.MediaMaxFileSize,
		UploadURLTTL:   cfg.MediaUploadURLTTL,
		DownloadURLTTL: cfg.MediaDownloadURLTTL,
		LocalPath:      cfg.MediaStoragePath,
		LocalBaseURL:   fmt.Sprintf("http://localhost:%d", cfg.FacadePort),
		S3Bucket:       cfg.MediaS3Bucket,
		S3Region:       cfg.MediaS3Region,
		S3Prefix:       cfg.MediaS3Prefix,
		S3Endpoint:     cfg.MediaS3Endpoint,
		GCSBucket:      cfg.MediaGCSBucket,
		GCSPrefix:      cfg.MediaGCSPrefix,
		AzureAccount:   cfg.MediaAzureAccount,
		AzureContainer: cfg.MediaAzureContainer,
		AzurePrefix:    cfg.MediaAzurePrefix,
		AzureKey:       cfg.MediaAzureKey,
	}

	// cfg.MediaStorageType != None was already checked above, so media.Build
	// here only ever returns (non-nil store, nil error) for a recognized
	// backend or (nil store, non-nil error) for an unrecognized one — never
	// (nil, nil).
	store, err := media.Build(context.Background(), bcfg)
	if err != nil {
		log.Error(err, "failed to initialize media storage", "type", cfg.MediaStorageType)
		return nil, nil
	}

	return store, func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Error(closeErr, "error closing media storage")
		}
	}
}

// logMediaStorageInit emits the facade's pre-construction Info log for the
// configured backend, preserving the per-type log fields the previous
// inline-switch implementation had (bucket/region/account/etc.). An
// unrecognized MediaStorageType is intentionally not logged here — media.Build
// rejects it and initMediaStorage logs that failure once, so callers don't see
// the "unknown type" condition reported twice.
func logMediaStorageInit(cfg *agent.Config, log logr.Logger) {
	switch cfg.MediaStorageType {
	case agent.MediaStorageTypeLocal:
		log.Info("using local filesystem media storage", "path", cfg.MediaStoragePath)
	case agent.MediaStorageTypeS3:
		log.Info("using S3 media storage",
			"bucket", cfg.MediaS3Bucket,
			"region", cfg.MediaS3Region,
			"prefix", cfg.MediaS3Prefix,
			"endpoint", cfg.MediaS3Endpoint,
		)
	case agent.MediaStorageTypeGCS:
		log.Info("using GCS media storage",
			"bucket", cfg.MediaGCSBucket,
			"prefix", cfg.MediaGCSPrefix,
		)
	case agent.MediaStorageTypeAzure:
		log.Info("using Azure Blob storage",
			"account", cfg.MediaAzureAccount,
			"container", cfg.MediaAzureContainer,
			"prefix", cfg.MediaAzurePrefix,
		)
	}
}
