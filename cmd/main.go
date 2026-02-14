/*
Copyright 2025 Altaira Labs.

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
	"github.com/altairalabs/omnia/internal/controller"
	"github.com/altairalabs/omnia/internal/schema"
	"github.com/altairalabs/omnia/pkg/metrics"
	// +kubebuilder:scaffold:imports
)

const logKeyController = "controller"
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

func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var facadeImage string
	var facadeImagePullPolicy string
	var frameworkImage string
	var frameworkImagePullPolicy string
	var tracingEnabled bool
	var tracingEndpoint string
	var sessionAPIURL string
	var workspaceStorageClass string
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
	flag.StringVar(&sessionAPIURL, "session-api-url", "",
		"Internal URL of the session-api service for session recording")
	flag.StringVar(&workspaceStorageClass, "workspace-storage-class", "",
		"Default storage class for workspace PVCs (e.g., omnia-nfs). If empty, uses cluster default.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
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

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

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

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

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
		SessionAPIURL:            sessionAPIURL,
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
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("provider-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "Provider")
		os.Exit(1)
	}

	// Workspace controller for multi-tenancy
	if err := (&controller.WorkspaceReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		DefaultStorageClass: workspaceStorageClass,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "Workspace")
		os.Exit(1)
	}
	retentionMetrics := metrics.NewRetentionMetrics()
	retentionMetrics.Initialize()

	if err := (&controller.SessionRetentionPolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Recorder:  mgr.GetEventRecorderFor("sessionretentionpolicy-controller"),
		Namespace: os.Getenv("POD_NAMESPACE"),
		Metrics:   retentionMetrics,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "SessionRetentionPolicy")
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

	ctx := ctrl.SetupSignalHandler()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
