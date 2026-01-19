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
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
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

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/api"
	"github.com/altairalabs/omnia/internal/controller"
	"github.com/altairalabs/omnia/internal/schema"
	// +kubebuilder:scaffold:imports
)

const logKeyController = "controller"

// Error message constants.
const errUnableToCreateController = "unable to create controller"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var apiAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var facadeImage string
	var facadeImagePullPolicy string
	var frameworkImage string
	var frameworkImagePullPolicy string
	var tracingEnabled bool
	var tracingEndpoint string
	var arenaWorkerImage string
	var arenaWorkerImagePullPolicy string
	var artifactBaseURL string
	var redisAddr string
	var redisPassword string
	var redisDB int
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&facadeImage, "facade-image", "",
		"The image to use for facade containers. If not set, defaults to ghcr.io/altairalabs/omnia-facade:latest")
	flag.StringVar(&facadeImagePullPolicy, "facade-image-pull-policy", "",
		"The image pull policy for facade containers. Valid values: Always, Never, IfNotPresent. Defaults to IfNotPresent")
	flag.StringVar(&frameworkImage, "framework-image", "",
		"The image to use for framework containers. If not set, defaults to ghcr.io/altairalabs/omnia-runtime:latest")
	flag.StringVar(&frameworkImagePullPolicy, "framework-image-pull-policy", "",
		"The image pull policy for framework containers. Valid values: Always, Never, IfNotPresent. Defaults to IfNotPresent")
	flag.BoolVar(&tracingEnabled, "tracing-enabled", false,
		"Enable distributed tracing for agent runtime containers")
	flag.StringVar(&tracingEndpoint, "tracing-endpoint", "",
		"OTLP endpoint for traces (e.g., tempo.omnia-system.svc.cluster.local:4317)")
	flag.StringVar(&arenaWorkerImage, "arena-worker-image", "",
		"The image to use for Arena worker containers. If not set, defaults to ghcr.io/altairalabs/arena-worker:latest")
	flag.StringVar(&arenaWorkerImagePullPolicy, "arena-worker-image-pull-policy", "",
		"Image pull policy for Arena workers. Valid: Always, Never, IfNotPresent. Default: IfNotPresent")
	flag.StringVar(&artifactBaseURL, "artifact-base-url", "http://localhost:8082/artifacts",
		"Base URL for serving Arena artifacts to workers. In-cluster, use the service URL.")
	flag.StringVar(&redisAddr, "redis-addr", "",
		"Redis server address for Arena work queue (e.g., redis:6379). If empty, Arena queue features are disabled.")
	flag.StringVar(&redisPassword, "redis-password", "",
		"Redis password for Arena work queue (optional).")
	flag.IntVar(&redisDB, "redis-db", 0,
		"Redis database number for Arena work queue.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&apiAddr, "api-bind-address", ":8082", "The address the REST API server binds to for dashboard access.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// To enable certManager, uncomment the following in your kustomization configs:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

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
		LeaderElectionID:       "4416a20d.altairalabs.ai",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controller.AgentRuntimeReconciler{
		Client:                   mgr.GetClient(),
		Scheme:                   mgr.GetScheme(),
		FacadeImage:              facadeImage,
		FacadeImagePullPolicy:    corev1.PullPolicy(facadeImagePullPolicy),
		FrameworkImage:           frameworkImage,
		FrameworkImagePullPolicy: corev1.PullPolicy(frameworkImagePullPolicy),
		TracingEnabled:           tracingEnabled,
		TracingEndpoint:          tracingEndpoint,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "AgentRuntime")
		os.Exit(1)
	}
	if err := (&controller.PromptPackReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		SchemaValidator: schema.NewSchemaValidatorWithOptions(ctrl.Log, nil, 0),
		Recorder:        mgr.GetEventRecorderFor("promptpack-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "PromptPack")
		os.Exit(1)
	}
	if err := (&controller.ToolRegistryReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "ToolRegistry")
		os.Exit(1)
	}
	if err := (&controller.ProviderReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "Provider")
		os.Exit(1)
	}

	// Arena Fleet controllers
	arenaArtifactDir := "/tmp/arena-artifacts"
	if err := os.MkdirAll(arenaArtifactDir, 0755); err != nil {
		setupLog.Error(err, "unable to create arena artifact directory")
		os.Exit(1)
	}
	if err := (&controller.ArenaSourceReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		Recorder:        mgr.GetEventRecorderFor("arenasource-controller"),
		ArtifactDir:     arenaArtifactDir,
		ArtifactBaseURL: artifactBaseURL,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "ArenaSource")
		os.Exit(1)
	}
	if err := (&controller.ArenaConfigReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("arenaconfig-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "ArenaConfig")
		os.Exit(1)
	}
	if err := (&controller.ArenaJobReconciler{
		Client:                mgr.GetClient(),
		Scheme:                mgr.GetScheme(),
		Recorder:              mgr.GetEventRecorderFor("arenajob-controller"),
		WorkerImage:           arenaWorkerImage,
		WorkerImagePullPolicy: corev1.PullPolicy(arenaWorkerImagePullPolicy),
		// Redis configuration for lazy connection during reconciliation
		RedisAddr:     redisAddr,
		RedisPassword: redisPassword,
		RedisDB:       redisDB,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "ArenaJob")
		os.Exit(1)
	}

	// Workspace controller for multi-tenancy
	if err := (&controller.WorkspaceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "Workspace")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Setup signal handler once (can only be called once)
	ctx := ctrl.SetupSignalHandler()

	// Start the artifact server for Arena artifacts
	if apiAddr != "" && apiAddr != "0" {
		apiServer := api.NewServer(ctrl.Log, arenaArtifactDir)
		go func() {
			if err := apiServer.Run(ctx, apiAddr); err != nil {
				setupLog.Error(err, "problem running artifact server")
			}
		}()
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
