/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. GCP, Azure, OIDC) for kubeconfig authentication
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/cmd/omnia-arena-controller/api"
	"github.com/altairalabs/omnia/ee/internal/controller"
	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/ee/pkg/encryption"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
	"github.com/altairalabs/omnia/internal/session/providers/postgres"
)

const logKeyController = "controller"
const errUnableToCreateController = "unable to create controller"

// msgUnableToCreateWebhook is the structured-log message used when a
// webhook registration call fails in main(). Extracted to satisfy
// go:S1192 (duplicated 3x for each registered webhook).
const msgUnableToCreateWebhook = "unable to create webhook"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1alpha1.AddToScheme(scheme))
	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var apiAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var arenaWorkerImage string
	var arenaWorkerImagePullPolicy string
	var arenaWorkerServiceAccount string
	var arenaWorkerPodLabels string
	var arenaDevConsoleImage string
	var arenaDevConsoleServiceAccount string
	var arenaDevConsolePodLabels string
	var workspaceContentPath string
	var workspaceStorageClass string
	var workspaceContentScoped bool
	var mgmtPlaneTokenURL string
	var mgmtPlaneJWKSURL string
	var nfsServer string
	var nfsPath string
	var sessionPostgresConn string
	var redisURL string
	var redisURLSecretName string
	var redisURLSecretKey string
	var enableWebhooks bool
	var enableLicenseWebhooks bool
	var devMode bool
	var tracingEnabled bool
	var tracingEndpoint string
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to.")
	flag.StringVar(&apiAddr, "api-bind-address", ":8082", "The address the template API server binds to.")
	flag.StringVar(&arenaWorkerImage, "arena-worker-image", "",
		"The image to use for Arena worker containers.")
	flag.StringVar(&arenaWorkerImagePullPolicy, "arena-worker-image-pull-policy", "",
		"Image pull policy for Arena workers. Valid: Always, Never, IfNotPresent.")
	flag.StringVar(&arenaWorkerServiceAccount, "worker-service-account", "",
		"ServiceAccount the arena worker pod runs as. Set to the workspace runtime "+
			"ServiceAccount so evaluations inherit its cloud identity (Azure Workload "+
			"Identity, AWS IRSA, GKE Workload Identity) and can authenticate to keyless "+
			"providers (auth.type: workloadIdentity). Empty = controller creates a "+
			"per-job arena-worker SA with no cloud identity.")
	flag.StringVar(&arenaWorkerPodLabels, "worker-pod-labels", "",
		"Comma-separated key=value labels added to the arena worker pod template, "+
			"e.g. 'azure.workload.identity/use=true' to opt into the WI webhook.")
	flag.StringVar(&arenaDevConsoleImage, "arena-dev-console-image", "",
		"The image to use for Arena dev console containers.")
	flag.StringVar(&arenaDevConsoleServiceAccount, "dev-console-service-account", "",
		"ServiceAccount the dev-console pod runs as. Set to the workspace runtime "+
			"ServiceAccount so the dev console inherits its cloud identity (Azure "+
			"Workload Identity, AWS IRSA, etc.). Empty = controller creates a per-session SA.")
	flag.StringVar(&arenaDevConsolePodLabels, "dev-console-pod-labels", "",
		"Comma-separated key=value labels added to the dev-console pod template, "+
			"e.g. 'azure.workload.identity/use=true' to opt into the WI webhook.")
	flag.StringVar(&workspaceContentPath, "workspace-content-path", "",
		"Base path for workspace content volumes.")
	flag.StringVar(&workspaceStorageClass, "workspace-storage-class", "",
		"Default storage class for workspace PVCs (e.g., nfs-client).")
	flag.BoolVar(&workspaceContentScoped, "workspace-content-scoped", false,
		"When true, the per-workspace content volume is already scoped to the "+
			"workspace subtree, so the mount subPath is workspace-relative (no "+
			"{workspace}/{namespace} prefix). Default false preserves the legacy "+
			"share-root behaviour where the operator sets the full subPath.")
	flag.StringVar(&mgmtPlaneTokenURL, "mgmt-plane-token-url", "",
		"URL of the dashboard's service-token endpoint, injected onto arena "+
			"worker pods as OMNIA_MGMT_PLANE_SERVICE_TOKEN_URL so they mint "+
			"mgmt-plane JWTs to authenticate fleet-mode WS dials to agent "+
			"facades. Typically http://omnia-dashboard.<ns>.svc.cluster.local:"+
			"3000/api/auth/service-token. Empty leaves fleet dials unauthenticated.")
	flag.StringVar(&mgmtPlaneJWKSURL, "mgmt-plane-jwks-url", "",
		"URL of the dashboard's JWKS endpoint, injected onto arena-dev-console "+
			"pods as OMNIA_MGMT_PLANE_JWKS_URL so they can validate dashboard-minted "+
			"mgmt-plane JWTs on WS and HTTP routes. Empty keeps dev-console auth chain empty.")
	flag.StringVar(&nfsServer, "nfs-server", "",
		"NFS server address for workspace content.")
	flag.StringVar(&nfsPath, "nfs-path", "",
		"NFS export path for workspace content.")
	flag.StringVar(&sessionPostgresConn, "session-postgres-conn", "",
		"Postgres connection string for session storage (required for key rotation re-encryption).")
	flag.StringVar(&redisURL, "redis-url", os.Getenv("REDIS_URL"),
		"Redis URL (redis:// or rediss://) for the Arena work queue. "+
			"Defaults to the REDIS_URL env, which the chart mounts from a "+
			"Secret when the consumer uses the existingSecret form.")
	flag.StringVar(&redisURLSecretName, "redis-url-secret-name", "",
		"Kubernetes Secret name to reference on every arena-worker pod "+
			"so workers see the same Redis URL. Set when the operator-side "+
			"Redis is configured via the existingSecret form. Empty "+
			"means workers receive the literal --redis-url value as a "+
			"plain env var.")
	flag.StringVar(&redisURLSecretKey, "redis-url-secret-key", "",
		"Key within --redis-url-secret-name whose value is the Redis URL.")
	flag.BoolVar(&enableWebhooks, "enable-webhooks", false,
		"Enable webhook server for admission webhooks (requires TLS certificates).")
	flag.BoolVar(&enableLicenseWebhooks, "enable-license-webhooks", false,
		"Enable license validation webhooks for Arena resources.")
	flag.BoolVar(&devMode, "dev-mode", false,
		"Enable development mode with a full-featured license. DO NOT USE IN PRODUCTION.")
	flag.BoolVar(&tracingEnabled, "tracing-enabled", false,
		"Enable OTel tracing for arena worker pods.")
	flag.StringVar(&tracingEndpoint, "tracing-endpoint", "",
		"OTLP gRPC endpoint for arena worker tracing (e.g. tempo:4317).")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Surface the workspace-content configuration so operators can see at a
	// glance whether ArenaSource / ArenaTemplateSource / ArenaJob content
	// sync features are available. Reconcilers for those resources emit a
	// ContentStorageUnavailable condition when the path is empty.
	if workspaceContentPath == "" {
		setupLog.Info("workspace content storage is disabled",
			"reason", "workspaceContentPathEmpty",
			"effect", "ArenaSource, ArenaTemplateSource, and ArenaJob content-sync paths will report ContentStorageUnavailable",
			"fix", "re-install the chart with workspaceContent.enabled=true")
	} else {
		setupLog.Info("workspace content storage enabled", "path", workspaceContentPath)
	}

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	var webhookServer webhook.Server
	if enableWebhooks {
		webhookServerOptions := webhook.Options{TLSOpts: tlsOpts}
		if len(webhookCertPath) > 0 {
			webhookServerOptions.CertDir = webhookCertPath
			webhookServerOptions.CertName = webhookCertName
			webhookServerOptions.KeyName = webhookCertKey
		}
		webhookServer = webhook.NewServer(webhookServerOptions)
	}

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}
	if len(metricsCertPath) > 0 {
		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "arena.altairalabs.ai",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := controller.SetupIndexers(context.Background(), mgr); err != nil {
		setupLog.Error(err, "unable to setup field indexers")
		os.Exit(1)
	}

	// Create license validator
	var licenseValidator *license.Validator
	validatorOpts := []license.ValidatorOption{}
	if devMode {
		validatorOpts = append(validatorOpts, license.WithDevMode())
	}
	licenseValidator, err = license.NewValidator(mgr.GetClient(), validatorOpts...)
	if err != nil {
		setupLog.Error(err, "unable to create license validator")
		os.Exit(1)
	}
	// Nag once at startup when this deployment isn't backed by a valid license
	// (open-core, absent, or expired) — gated on the license, not on dev-mode.
	license.NagIfUnlicensed(licenseValidator.GetLicenseOrDefault(context.Background()), setupLog)

	// Create storage manager for lazy PVC creation (only used when NFS is not configured)
	var storageManager *workspace.StorageManager
	if nfsServer == "" || nfsPath == "" {
		storageManager = workspace.NewStorageManager(mgr.GetClient(), workspaceStorageClass)
		setupLog.Info("storage manager initialized for lazy PVC creation",
			"defaultStorageClass", workspaceStorageClass)
	} else {
		setupLog.Info("using direct NFS mount, storage manager not needed")
	}

	// Create Redis queue and aggregator
	var arenaAggregator *aggregator.Aggregator
	if redisURL != "" {
		redisQueue, qErr := queue.NewRedisQueue(queue.RedisOptions{
			URL:     redisURL,
			Options: queue.DefaultOptions(),
		})
		if qErr != nil {
			setupLog.Error(qErr, "failed to create Redis queue for arena aggregator")
		} else {
			arenaAggregator = aggregator.New(redisQueue)
			setupLog.Info("arena result aggregator initialized")
		}
	}

	if err := registerArenaWorkloads(mgr, registrationOptions{
		Controllers: setupOptions{
			WorkerImage:              arenaWorkerImage,
			WorkerImagePullPolicy:    corev1.PullPolicy(arenaWorkerImagePullPolicy),
			WorkerServiceAccount:     arenaWorkerServiceAccount,
			WorkerPodLabels:          parseKeyValueLabels(arenaWorkerPodLabels),
			DevConsoleImage:          arenaDevConsoleImage,
			DevConsoleServiceAccount: arenaDevConsoleServiceAccount,
			DevConsolePodLabels:      parseKeyValueLabels(arenaDevConsolePodLabels),
			WorkspaceContentPath:     workspaceContentPath,
			WorkspaceContentScoped:   workspaceContentScoped,
			NFSServer:                nfsServer,
			NFSPath:                  nfsPath,
			LicenseValidator:         licenseValidator,
			StorageManager:           storageManager,
			Aggregator:               arenaAggregator,
			RedisURL:                 redisURL,
			RedisURLSecretName:       redisURLSecretName,
			RedisURLSecretKey:        redisURLSecretKey,
			TracingEnabled:           tracingEnabled,
			TracingEndpoint:          tracingEndpoint,
			MgmtPlaneTokenURL:        mgmtPlaneTokenURL,
			MgmtPlaneJWKSURL:         mgmtPlaneJWKSURL,
			PrivacyPolicyMetrics:     newPrivacyPolicyMetrics(),
			ReEncryptionStore:        buildReEncryptionStoreFactory(sessionPostgresConn, setupLog),
		},
		Webhooks: webhookOptions{
			LicenseValidator:    licenseValidator,
			IncludeLicenseHooks: enableLicenseWebhooks,
		},
		EnableWebhooks: enableWebhooks,
	}, setupLog); err != nil {
		setupLog.Error(err, "registration failed")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	// Start API server for template rendering
	apiServer := api.NewServer(apiAddr, ctrl.Log, licenseValidator)
	go func() {
		if err := apiServer.Start(ctx); err != nil && err != http.ErrServerClosed {
			setupLog.Error(err, "API server error")
		}
	}()

	// Start periodic dev mode warning
	if devMode {
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					setupLog.Info( //nolint:lll
						"WARNING: Running with development license - NOT LICENSED FOR PRODUCTION USE. " +
							"Please obtain a valid enterprise license at https://altairalabs.ai/licensing")
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	setupLog.Info("starting arena controller manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// Shutdown API server when manager stops
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		setupLog.Error(err, "API server shutdown error")
	}
}

// buildReEncryptionStoreFactory returns a factory that opens a session Postgres
// pool and wraps it as a ReEncryptionStore. Returns nil when connStr is empty,
// which disables re-encryption but still allows key rotation (rotation without
// re-encryption rotates keys but leaves existing data encrypted with the old key).
func buildReEncryptionStoreFactory(connStr string, log logr.Logger) func() (encryption.ReEncryptionStore, error) {
	if connStr == "" {
		log.Info("re-encryption disabled", "reason", "no session postgres connection configured")
		return nil
	}
	return func() (encryption.ReEncryptionStore, error) {
		provider, err := postgres.New(postgres.Config{ConnString: connStr})
		if err != nil {
			return nil, fmt.Errorf("open session postgres: %w", err)
		}
		return provider, nil
	}
}
