/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. GCP, Azure, OIDC) for kubeconfig authentication
	_ "k8s.io/client-go/plugin/pkg/client/auth"

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
	"github.com/altairalabs/omnia/ee/internal/controller"
	arenawebhook "github.com/altairalabs/omnia/ee/internal/webhook"
	"github.com/altairalabs/omnia/ee/pkg/arena/aggregator"
	"github.com/altairalabs/omnia/ee/pkg/arena/queue"
	"github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/workspace"
)

const logKeyController = "controller"
const errUnableToCreateController = "unable to create controller"

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
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var arenaWorkerImage string
	var arenaWorkerImagePullPolicy string
	var arenaDevConsoleImage string
	var workspaceContentPath string
	var workspaceStorageClass string
	var nfsServer string
	var nfsPath string
	var redisAddr string
	var redisPassword string
	var redisDB int
	var enableLicenseWebhooks bool
	var devMode bool
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to.")
	flag.StringVar(&arenaWorkerImage, "arena-worker-image", "",
		"The image to use for Arena worker containers.")
	flag.StringVar(&arenaWorkerImagePullPolicy, "arena-worker-image-pull-policy", "",
		"Image pull policy for Arena workers. Valid: Always, Never, IfNotPresent.")
	flag.StringVar(&arenaDevConsoleImage, "arena-dev-console-image", "",
		"The image to use for Arena dev console containers.")
	flag.StringVar(&workspaceContentPath, "workspace-content-path", "",
		"Base path for workspace content volumes.")
	flag.StringVar(&workspaceStorageClass, "workspace-storage-class", "",
		"Default storage class for workspace PVCs (e.g., nfs-client).")
	flag.StringVar(&nfsServer, "nfs-server", "",
		"NFS server address for workspace content.")
	flag.StringVar(&nfsPath, "nfs-path", "",
		"NFS export path for workspace content.")
	flag.StringVar(&redisAddr, "redis-addr", "",
		"Redis server address for Arena work queue.")
	flag.StringVar(&redisPassword, "redis-password", "",
		"Redis password for Arena work queue.")
	flag.IntVar(&redisDB, "redis-db", 0,
		"Redis database number for Arena work queue.")
	flag.BoolVar(&enableLicenseWebhooks, "enable-license-webhooks", false,
		"Enable license validation webhooks for Arena resources.")
	flag.BoolVar(&devMode, "dev-mode", false,
		"Enable development mode with a full-featured license. DO NOT USE IN PRODUCTION.")
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

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServerOptions := webhook.Options{TLSOpts: tlsOpts}
	if len(webhookCertPath) > 0 {
		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}
	webhookServer := webhook.NewServer(webhookServerOptions)

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

	// Create license validator
	var licenseValidator *license.Validator
	validatorOpts := []license.ValidatorOption{}
	if devMode {
		setupLog.Info("WARNING: Running in dev mode with full-featured license. DO NOT USE IN PRODUCTION.")
		validatorOpts = append(validatorOpts, license.WithDevMode())
	}
	licenseValidator, err = license.NewValidator(mgr.GetClient(), validatorOpts...)
	if err != nil {
		setupLog.Error(err, "unable to create license validator")
		os.Exit(1)
	}

	// Create storage manager for lazy PVC creation (only used when NFS is not configured)
	var storageManager *workspace.StorageManager
	if nfsServer == "" || nfsPath == "" {
		storageManager = workspace.NewStorageManager(mgr.GetClient(), workspaceStorageClass)
		setupLog.Info("storage manager initialized for lazy PVC creation",
			"defaultStorageClass", workspaceStorageClass)
	} else {
		setupLog.Info("using direct NFS mount, storage manager not needed")
	}

	// ArenaSource controller
	if err := (&controller.ArenaSourceReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Recorder:             mgr.GetEventRecorderFor("arenasource-controller"),
		WorkspaceContentPath: workspaceContentPath,
		MaxVersionsPerSource: 10,
		LicenseValidator:     licenseValidator,
		StorageManager:       storageManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "ArenaSource")
		os.Exit(1)
	}

	// ArenaTemplateSource controller
	if err := (&controller.ArenaTemplateSourceReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Recorder:             mgr.GetEventRecorderFor("arenatemplatesource-controller"),
		WorkspaceContentPath: workspaceContentPath,
		MaxVersionsPerSource: 10,
		LicenseValidator:     licenseValidator,
		StorageManager:       storageManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "ArenaTemplateSource")
		os.Exit(1)
	}

	// Create Redis queue and aggregator
	var arenaAggregator *aggregator.Aggregator
	if redisAddr != "" {
		redisQueue, err := queue.NewRedisQueue(queue.RedisOptions{
			Addr:     redisAddr,
			Password: redisPassword,
			DB:       redisDB,
			Options:  queue.DefaultOptions(),
		})
		if err != nil {
			setupLog.Error(err, "failed to create Redis queue for arena aggregator")
		} else {
			arenaAggregator = aggregator.New(redisQueue)
			setupLog.Info("arena result aggregator initialized", "redisAddr", redisAddr)
		}
	}

	// ArenaJob controller
	if err := (&controller.ArenaJobReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		Recorder:              mgr.GetEventRecorderFor("arenajob-controller"),
		WorkerImage:           arenaWorkerImage,
		WorkerImagePullPolicy: corev1.PullPolicy(arenaWorkerImagePullPolicy),
		LicenseValidator:      licenseValidator,
		Aggregator:            arenaAggregator,
		RedisAddr:             redisAddr,
		RedisPassword:         redisPassword,
		RedisDB:               redisDB,
		WorkspaceContentPath:  workspaceContentPath,
		NFSServer:             nfsServer,
		NFSPath:               nfsPath,
		StorageManager:        storageManager,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "ArenaJob")
		os.Exit(1)
	}

	// ArenaDevSession controller
	if err := (&controller.ArenaDevSessionReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		DevConsoleImage: arenaDevConsoleImage,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "ArenaDevSession")
		os.Exit(1)
	}

	// Setup license validation webhooks
	if enableLicenseWebhooks {
		if err := arenawebhook.SetupArenaSourceWebhookWithManager(mgr, licenseValidator); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ArenaSource")
			os.Exit(1)
		}
		if err := arenawebhook.SetupArenaJobWebhookWithManager(mgr, licenseValidator); err != nil {
			setupLog.Error(err, "unable to create webhook", "webhook", "ArenaJob")
			os.Exit(1)
		}
		setupLog.Info("license validation webhooks enabled")
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

	setupLog.Info("starting arena controller manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
