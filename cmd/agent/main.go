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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/facade"
	"github.com/altairalabs/omnia/internal/media"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/httpclient"
	"github.com/altairalabs/omnia/internal/tracing"
	"github.com/altairalabs/omnia/pkg/logging"
	"github.com/altairalabs/omnia/pkg/servicediscovery"
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

	// Branch on facade type: A2A runs the SDK in-process (no runtime sidecar),
	// while WebSocket uses the traditional facade + gRPC runtime architecture.
	if cfg.FacadeType == agent.FacadeTypeA2A {
		runA2AFacade(cfg, log, tracingProvider)
	} else {
		runWebSocketFacade(cfg, log, tracingProvider)
	}
}

func initSessionStore(log logr.Logger) (session.Store, error) {
	resolver := servicediscovery.NewResolver(buildK8sClient())
	urls, err := resolver.ResolveServiceURLs(context.Background(), resolveServiceGroup())
	if err != nil {
		log.Info("service discovery unavailable, using in-memory session store", "reason", err.Error())
		return session.NewMemoryStore(), nil
	}
	log.Info("using session-api HTTP store", "url", urls.SessionURL)
	return httpclient.NewStore(urls.SessionURL, log), nil
}

func buildK8sClient() client.Client {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil // Not in cluster
	}
	scheme := k8sruntime.NewScheme()
	_ = omniav1alpha1.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil
	}
	return c
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

func readyzHandler(store session.Store, handler facade.MessageHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// Prefer lightweight Ping if the store supports it (e.g. httpclient);
		// fall back to a dummy GetSession for stores that don't.
		if p, ok := store.(pinger); ok {
			if err := p.Ping(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, "session store unavailable: %v", err)
				return
			}
		} else {
			_, err := store.GetSession(ctx, "00000000-0000-0000-0000-000000000000")
			if err != nil && err != session.ErrSessionNotFound {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, "session store unavailable: %v", err)
				return
			}
		}

		// Check runtime health if using runtime handler
		if runtimeHandler, ok := handler.(*agent.RuntimeHandler); ok {
			resp, err := runtimeHandler.Client().Health(ctx)
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, "runtime unavailable: %v", err)
				return
			}
			if !resp.Healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprintf(w, "runtime unhealthy: %s", resp.Status)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

// createHandler creates the appropriate message handler based on configuration.
// Returns the handler and an optional cleanup function.
func createHandler(cfg *agent.Config, log logr.Logger, tp *tracing.Provider) (facade.MessageHandler, func()) {
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
		log.Info("using runtime handler mode", "address", cfg.RuntimeAddress)

		// Retry connection with exponential backoff (runtime container may still be starting)
		var client *facade.RuntimeClient
		var err error
		maxRetries := 10
		backoff := 500 * time.Millisecond

		// Build RuntimeClient config with optional tracing
		runtimeCfg := facade.RuntimeClientConfig{
			Address:     cfg.RuntimeAddress,
			DialTimeout: 5 * time.Second,
			Log:         log,
		}
		if tp != nil {
			runtimeCfg.TracerProvider = tp.TracerProvider()
		}

		for i := 0; i < maxRetries; i++ {
			client, err = facade.NewRuntimeClient(runtimeCfg)
			if err == nil {
				log.Info("connected to runtime", "address", cfg.RuntimeAddress, "attempt", i+1)
				break
			}

			log.Info("waiting for runtime to be ready", "address", cfg.RuntimeAddress, "attempt", i+1, "error", err.Error())
			time.Sleep(backoff)
			backoff = min(backoff*2, 5*time.Second) // Cap at 5 seconds
		}

		if err != nil {
			log.Error(err, "failed to connect to runtime after retries, falling back to nil handler")
			return nil, nil
		}

		handler := agent.NewRuntimeHandler(client)
		cleanup := func() {
			if err := client.Close(); err != nil {
				log.Error(err, "error closing runtime client")
			}
		}
		return handler, cleanup
	default:
		log.Info("unknown handler mode, using nil handler", "mode", cfg.HandlerMode)
		return nil, nil
	}
}

// initMediaStorage creates the appropriate media storage backend based on configuration.
// Returns the storage and an optional cleanup function.
//
//nolint:gocognit // switch over storage backends
func initMediaStorage(cfg *agent.Config, log logr.Logger) (media.Storage, func()) {
	ctx := context.Background()

	switch cfg.MediaStorageType {
	case agent.MediaStorageTypeNone:
		log.Info("media storage disabled")
		return nil, nil

	case agent.MediaStorageTypeLocal:
		log.Info("using local filesystem media storage", "path", cfg.MediaStoragePath)

		// Build base URL from the facade port
		baseURL := fmt.Sprintf("http://localhost:%d", cfg.FacadePort)

		storageCfg := media.LocalStorageConfig{
			BasePath:     cfg.MediaStoragePath,
			BaseURL:      baseURL,
			DefaultTTL:   cfg.MediaDefaultTTL,
			UploadURLTTL: 15 * time.Minute,
			MaxFileSize:  cfg.MediaMaxFileSize,
		}

		storage, err := media.NewLocalStorage(storageCfg)
		if err != nil {
			log.Error(err, "failed to initialize local media storage")
			return nil, nil
		}

		return storage, func() {
			if err := storage.Close(); err != nil {
				log.Error(err, "error closing media storage")
			}
		}

	case agent.MediaStorageTypeS3:
		log.Info("using S3 media storage",
			"bucket", cfg.MediaS3Bucket,
			"region", cfg.MediaS3Region,
			"prefix", cfg.MediaS3Prefix,
			"endpoint", cfg.MediaS3Endpoint,
		)

		storageCfg := media.S3Config{
			Bucket:         cfg.MediaS3Bucket,
			Region:         cfg.MediaS3Region,
			Prefix:         cfg.MediaS3Prefix,
			Endpoint:       cfg.MediaS3Endpoint,
			UsePathStyle:   cfg.MediaS3Endpoint != "", // Use path style for custom endpoints (MinIO)
			UploadURLTTL:   15 * time.Minute,
			DownloadURLTTL: 1 * time.Hour,
			DefaultTTL:     cfg.MediaDefaultTTL,
			MaxFileSize:    cfg.MediaMaxFileSize,
		}

		storage, err := media.NewS3Storage(ctx, storageCfg)
		if err != nil {
			log.Error(err, "failed to initialize S3 media storage")
			return nil, nil
		}

		return storage, func() {
			if err := storage.Close(); err != nil {
				log.Error(err, "error closing S3 storage")
			}
		}

	case agent.MediaStorageTypeGCS:
		log.Info("using GCS media storage",
			"bucket", cfg.MediaGCSBucket,
			"prefix", cfg.MediaGCSPrefix,
		)

		storageCfg := media.GCSConfig{
			Bucket:         cfg.MediaGCSBucket,
			Prefix:         cfg.MediaGCSPrefix,
			UploadURLTTL:   15 * time.Minute,
			DownloadURLTTL: 1 * time.Hour,
			DefaultTTL:     cfg.MediaDefaultTTL,
			MaxFileSize:    cfg.MediaMaxFileSize,
		}

		storage, err := media.NewGCSStorage(ctx, storageCfg)
		if err != nil {
			log.Error(err, "failed to initialize GCS media storage")
			return nil, nil
		}

		return storage, func() {
			if err := storage.Close(); err != nil {
				log.Error(err, "error closing GCS storage")
			}
		}

	case agent.MediaStorageTypeAzure:
		log.Info("using Azure Blob storage",
			"account", cfg.MediaAzureAccount,
			"container", cfg.MediaAzureContainer,
			"prefix", cfg.MediaAzurePrefix,
		)

		storageCfg := media.AzureConfig{
			AccountName:    cfg.MediaAzureAccount,
			ContainerName:  cfg.MediaAzureContainer,
			Prefix:         cfg.MediaAzurePrefix,
			AccountKey:     cfg.MediaAzureKey, // Optional - uses DefaultAzureCredential if empty
			UploadURLTTL:   15 * time.Minute,
			DownloadURLTTL: 1 * time.Hour,
			DefaultTTL:     cfg.MediaDefaultTTL,
			MaxFileSize:    cfg.MediaMaxFileSize,
		}

		storage, err := media.NewAzureStorage(ctx, storageCfg)
		if err != nil {
			log.Error(err, "failed to initialize Azure media storage")
			return nil, nil
		}

		return storage, func() {
			if err := storage.Close(); err != nil {
				log.Error(err, "error closing Azure storage")
			}
		}

	default:
		log.Info("unknown media storage type, disabling", "type", cfg.MediaStorageType)
		return nil, nil
	}
}
