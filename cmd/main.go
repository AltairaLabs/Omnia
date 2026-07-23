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
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

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
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/prometheus/client_golang/prometheus"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	eesetup "github.com/altairalabs/omnia/ee/pkg/setup"
	"github.com/altairalabs/omnia/internal/api/authz"
	"github.com/altairalabs/omnia/internal/api/content"
	"github.com/altairalabs/omnia/internal/api/deploy"
	"github.com/altairalabs/omnia/internal/controller"
	"github.com/altairalabs/omnia/internal/schema"
	"github.com/altairalabs/omnia/internal/tooltest"
	omniawebhook "github.com/altairalabs/omnia/internal/webhook"
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
	utilruntime.Must(eev1alpha1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
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
	var frameworkImages frameworkImagesFlag
	var frameworkImagePullPolicy string
	var tracingEnabled bool
	var tracingEndpoint string
	var sessionAPIImage string
	var sessionAPIImagePullPolicy string
	var memoryAPIImage string
	var memoryAPIImagePullPolicy string
	var privacyAPIImage string
	var privacyAPIImagePullPolicy string
	var workspaceStorageClass string
	var workspaceContentPath string
	var redisAddr string
	var evalWorkerImage string
	var evalWorkerImagePullPolicy string
	var policyBrokerImage string
	var workspaceReaderRBACEnabled bool
	var defaultExposureBaseDomain string
	var defaultExposureGatewayName string
	var defaultExposureGatewayNamespace string
	var defaultExposureGatewaySection string
	var apiBindAddress string
	var toolTestAllowedSubjects string
	var contentAPIBindAddress string
	var deployAPIBindAddress string
	var sessionAPIAuthEnabled bool
	var sessionAPIAuthAudience string
	var sessionAPIAuthTokenExpirationSeconds int64
	var sessionAPIAuthIstioMTLS bool
	var sessionAPIAuthExtraSubjects string
	var sessionAPITokenReviewClusterRole string
	var privacyDefaultReaderClusterRole string
	var memoryConsolidationReaderClusterRole string
	var enterpriseEnabled bool
	var licenseServerURL string
	var licenseAPIURL string
	var clusterName string
	var mgmtPlaneJWKSURL string
	var meshEnabled bool
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&facadeImage, "facade-image", "",
		"The image to use for facade containers. If not set, defaults to ghcr.io/altairalabs/omnia-facade:latest")
	flag.StringVar(&facadeImagePullPolicy, "facade-image-pull-policy", "",
		"The image pull policy for facade containers. Valid values: Always, Never, IfNotPresent. Defaults to IfNotPresent")
	flag.Var(&frameworkImages, "framework-image",
		"Runtime image as <type>=<repo:tag> (repeatable). A bare <repo:tag> maps to the promptkit framework. "+
			"Example: --framework-image=langchain=ghcr.io/altairalabs/omnia-langchain-runtime:v1")
	flag.StringVar(&frameworkImagePullPolicy, "framework-image-pull-policy", "",
		"The image pull policy for framework containers. Valid values: Always, Never, IfNotPresent. Defaults to IfNotPresent")
	flag.BoolVar(&tracingEnabled, "tracing-enabled", false,
		"Enable distributed tracing for agent runtime containers")
	flag.StringVar(&tracingEndpoint, "tracing-endpoint", "",
		"OTLP endpoint for traces (e.g., tempo.omnia-system.svc.cluster.local:4317)")
	flag.StringVar(&sessionAPIImage, "session-api-image", "",
		"Image for per-workspace session-api containers. Defaults to ghcr.io/altairalabs/omnia-session-api:latest")
	flag.StringVar(&sessionAPIImagePullPolicy, "session-api-image-pull-policy", "",
		"Image pull policy for session-api containers. Valid values: Always, Never, IfNotPresent.")
	flag.StringVar(&memoryAPIImage, "memory-api-image", "",
		"Image for per-workspace memory-api containers. Defaults to ghcr.io/altairalabs/omnia-memory-api:latest")
	flag.StringVar(&memoryAPIImagePullPolicy, "memory-api-image-pull-policy", "",
		"Image pull policy for memory-api containers. Valid values: Always, Never, IfNotPresent.")
	flag.StringVar(&privacyAPIImage, "privacy-api-image", "",
		"Image for per-workspace privacy-api containers. When empty, no privacy-api Deployments are reconciled "+
			"even if workspace.spec.privacy is set. Defaults to ghcr.io/altairalabs/omnia-privacy-api:latest")
	flag.StringVar(&privacyAPIImagePullPolicy, "privacy-api-image-pull-policy", "",
		"Image pull policy for privacy-api containers. Valid values: Always, Never, IfNotPresent.")
	flag.StringVar(&workspaceStorageClass, "workspace-storage-class", "",
		"Default storage class for workspace PVCs (e.g., omnia-nfs). If empty, uses cluster default.")
	flag.StringVar(&workspaceContentPath, "workspace-content-path", "/workspace-content",
		"Base path for the workspace content volume. SkillSource writes synced content here.")
	flag.StringVar(&redisAddr, "redis-url", "",
		"Operator-wide Redis URL (redis:// or rediss://). Forwarded to "+
			"eval-worker pods via REDIS_URL env. Per-workspace memory-api "+
			"uses --memory-redis-url for fine-grained Redis isolation; "+
			"this flag drives eval-worker only. Empty disables eval-worker.")
	var memoryRedisURL string
	flag.StringVar(&memoryRedisURL, "memory-redis-url", "",
		"Operator-wide Redis URL forwarded to every per-workspace memory-api as --redis-url. "+
			"Accepts redis:// or rediss:// URLs, and the literal placeholder \"$(REDIS_URL)\" — when set "+
			"to the placeholder, --memory-redis-url-secret-name and --memory-redis-url-secret-key MUST "+
			"reference the Kubernetes Secret containing the actual URL. Memory-api's pod will mount that "+
			"Secret as REDIS_URL env, and Kubernetes env expansion at startup fills the placeholder.")
	var memoryRedisURLSecretName string
	flag.StringVar(&memoryRedisURLSecretName, "memory-redis-url-secret-name", "",
		"Kubernetes Secret name backing $(REDIS_URL) substitution on per-workspace memory-api pods. "+
			"Required only when --memory-redis-url is the placeholder \"$(REDIS_URL)\".")
	var memoryRedisURLSecretKey string
	flag.StringVar(&memoryRedisURLSecretKey, "memory-redis-url-secret-key", "",
		"Key within --memory-redis-url-secret-name whose value is the Redis URL. "+
			"Required only when --memory-redis-url is the placeholder \"$(REDIS_URL)\".")
	var memoryCacheTTL string
	flag.StringVar(&memoryCacheTTL, "memory-cache-ttl", "5m",
		"TTL forwarded to memory-api as --cache-ttl. Empty or \"0\" disables the read-through "+
			"cache even when --memory-redis-url is set (useful when Redis is provisioned only for "+
			"the event publisher).")
	var memoryConsolidationInterval string
	flag.StringVar(&memoryConsolidationInterval, "memory-consolidation-interval", "",
		"Schedule-evaluation (poll) interval forwarded to memory-api as --consolidation-interval. "+
			"Each consolidation axis fires per its MemoryPolicy cron schedule; this controls how often "+
			"schedules are checked (e.g. \"1m\"). Empty disables the worker. Production deployments opt "+
			"in per-environment.")
	var memoryProjectionInterval string
	flag.StringVar(&memoryProjectionInterval, "memory-projection-interval", "",
		"Poll interval forwarded to memory-api as --projection-interval for the Memory Galaxy "+
			"pre-render worker (e.g. \"30s\"). Empty disables the worker. Production deployments opt "+
			"in per-environment.")
	var sessionRedisURL string
	flag.StringVar(&sessionRedisURL, "session-redis-url", "",
		"Operator-wide Redis URL forwarded to every per-workspace session-api as --redis-url for "+
			"the hot-cache layer. Same shape as --memory-redis-url: literal URL, or \"$(REDIS_URL)\" "+
			"placeholder paired with --session-redis-url-secret-{name,key}.")
	var sessionRedisURLSecretName string
	flag.StringVar(&sessionRedisURLSecretName, "session-redis-url-secret-name", "",
		"Kubernetes Secret name backing $(REDIS_URL) substitution on per-workspace session-api pods.")
	var sessionRedisURLSecretKey string
	flag.StringVar(&sessionRedisURLSecretKey, "session-redis-url-secret-key", "",
		"Key within --session-redis-url-secret-name whose value is the Redis URL.")
	flag.StringVar(&evalWorkerImage, "eval-worker-image", "",
		"Image for the arena-eval-worker container. If not set, defaults to ghcr.io/altairalabs/omnia-eval-worker:latest")
	flag.StringVar(&evalWorkerImagePullPolicy, "eval-worker-image-pull-policy", "",
		"Image pull policy for the arena-eval-worker container.")
	flag.StringVar(&policyBrokerImage, "policy-broker-image", "",
		"Image for the ToolPolicy decision-broker sidecar. If empty, uses the default from policy_broker_sidecar.go.")
	flag.BoolVar(&workspaceReaderRBACEnabled, "workspace-reader-rbac", false,
		"Enable per-workspace Workspace-reader ClusterRoleBindings for agent, eval-worker, and service pods. "+
			"False = no bindings (local dev).")
	flag.StringVar(&defaultExposureBaseDomain, "default-exposure-base-domain", "",
		"Base domain for agent HTTPRoute hostnames (#1553). Empty disables default exposure.")
	flag.StringVar(&defaultExposureGatewayName, "default-exposure-gateway-name", "",
		"Name of the Gateway agent HTTPRoutes attach to. Empty disables default exposure.")
	flag.StringVar(&defaultExposureGatewayNamespace, "default-exposure-gateway-namespace", "",
		"Default-exposure Gateway namespace (cross-ns needs a ReferenceGrant). Empty = agent's namespace.")
	flag.StringVar(&defaultExposureGatewaySection, "default-exposure-gateway-section", "",
		"Optional listener sectionName on the default-exposure Gateway.")
	flag.StringVar(&apiBindAddress, "api-bind-address", "",
		"Address for the tool test API server (e.g., :8083). If empty, the API server is not started.")
	flag.StringVar(&contentAPIBindAddress, "content-api-bind-address", "",
		"Address for the workspace-content API server (e.g., :8084) the dashboard calls instead "+
			"of mounting the NFS content volume. Requires --mgmt-plane-jwks-url (to verify the "+
			"dashboard-minted identity token) and --workspace-content-path (the mounted content "+
			"root). If empty, the content API server is not started.")
	flag.StringVar(&deployAPIBindAddress, "deploy-api-bind-address", "",
		"Address for the deploy-intent API server (e.g. :8083). Empty disables it. Requires --mgmt-plane-jwks-url.")
	flag.StringVar(&toolTestAllowedSubjects, "tool-test-allowed-subjects", "",
		"Comma-separated list of authenticated usernames allowed to call the tool-test API "+
			"(e.g. system:serviceaccount:omnia-system:omnia-dashboard). Each request must present a "+
			"bearer token that TokenReview authenticates to one of these subjects. If empty, the "+
			"tool-test API runs WITHOUT authentication (local/dev only).")
	flag.BoolVar(&sessionAPIAuthEnabled, "session-api-auth-enabled", false,
		"Require ServiceAccount auth on the operator-managed per-workspace session-api "+
			"(SEC-1/SEC-5). When set, the operator renders SESSION_API_AUTH_* env onto each "+
			"session-api Deployment, grants its ServiceAccount RBAC to create TokenReviews, and "+
			"mounts an audience-bound projected SA token onto every caller pod it manages "+
			"(facade, memory-api, eval-worker). Default off (opt-in).")
	flag.StringVar(&sessionAPIAuthAudience, "session-api-auth-audience", "",
		"Audience bound into caller projected SA tokens and enforced by session-api "+
			"(--auth-audiences). Required when --session-api-auth-enabled is set.")
	flag.Int64Var(&sessionAPIAuthTokenExpirationSeconds, "session-api-auth-token-expiration-seconds", 3600,
		"Expiration (seconds) for caller projected SA tokens. The kubelet rotates the token "+
			"before expiry. Only used when --session-api-auth-enabled is set.")
	flag.BoolVar(&sessionAPIAuthIstioMTLS, "session-api-auth-istio-mtls", false,
		"Additionally provision a STRICT Istio PeerAuthentication for session-api/memory-api in "+
			"each workspace namespace (defence-in-depth mTLS on top of SA-token auth). Requires "+
			"Istio. Only used when --session-api-auth-enabled is set.")
	flag.StringVar(&sessionAPIAuthExtraSubjects, "session-api-auth-extra-subjects", "",
		"Comma-separated exact-match ServiceAccount subjects added to the session-api allowlist, "+
			"for CROSS-NAMESPACE callers (e.g. the chart-owned dashboard SA in the release namespace: "+
			"system:serviceaccount:omnia-system:omnia-dashboard). In-workspace callers (facade, "+
			"memory-api, eval-worker) are authorized by namespace instead and need NOT be listed here.")
	flag.StringVar(&sessionAPITokenReviewClusterRole, "session-api-tokenreview-clusterrole", "",
		"Name of the install-wide ClusterRole granting authentication.k8s.io/tokenreviews:create. "+
			"When --session-api-auth-enabled is set, the operator binds each per-workspace session-api "+
			"ServiceAccount to it so session-api can validate caller tokens. Provisioned by the chart.")
	flag.StringVar(&privacyDefaultReaderClusterRole, "privacy-default-reader-clusterrole", "",
		"Name of the install-wide ClusterRole granting get on any \"default\"-named SessionPrivacyPolicy "+
			"cluster-wide. On enterprise builds the operator binds each per-workspace service-pod "+
			"ServiceAccount (session-api, memory-api, privacy-api) to it so the privacy watcher can Get the "+
			"global omnia-system/default policy. Provisioned by the chart (enterprise only); empty disables "+
			"the binding.")
	flag.StringVar(&memoryConsolidationReaderClusterRole, "memory-consolidation-reader-clusterrole", "",
		"Name of the install-wide ClusterRole granting cluster-wide get;list;watch on memorypolicies. On "+
			"enterprise builds the operator binds ONLY each per-workspace memory-api ServiceAccount to it so "+
			"the consolidation lister can enumerate MemoryPolicy CRDs across workspaces. Provisioned by the "+
			"chart (enterprise only); empty disables the binding.")
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
	flag.BoolVar(&enterpriseEnabled, "enterprise", false,
		"Enable enterprise edition controllers and webhooks")
	flag.StringVar(&licenseServerURL, "license-server-url", "",
		"URL of the license activation server for enterprise features")
	flag.StringVar(&licenseAPIURL, "license-api-url", "",
		"Arena-controller license endpoint, stamped onto data-plane pods as "+
			"OPERATOR_API_URL so they nag when unlicensed (e.g. "+
			"http://omnia-arena-controller.omnia-system:8082)")
	flag.StringVar(&clusterName, "cluster-name", "",
		"Human-readable name for this cluster in license records")
	flag.StringVar(&mgmtPlaneJWKSURL, "mgmt-plane-jwks-url", "",
		"URL of the dashboard's JWKS endpoint, set on every facade container "+
			"as OMNIA_MGMT_PLANE_JWKS_URL so cmd/agent can build a JWKS-backed "+
			"mgmt-plane validator. Typically the in-cluster service URL, e.g. "+
			"http://omnia-dashboard.omnia-system.svc.cluster.local:3000/api/auth/jwks. "+
			"Empty disables wiring — facade stays mgmt-plane-unaware (Arena E2E, "+
			"headless installs).")
	flag.BoolVar(&meshEnabled, "mesh-enabled", false,
		"Istio ambient mesh is enabled; allows rollout trafficRouting mode=mesh (operator-owned VS/DR).")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Surface the workspace-content configuration up-front so operators can
	// see at a glance whether SkillSource / ArenaSource / PromptPack.skills
	// features are available. Reconcilers for those resources emit a
	// ContentStorageUnavailable condition when the path is empty.
	if workspaceContentPath == "" {
		setupLog.Info("workspace content storage is disabled",
			"reason", "workspaceContentPathEmpty",
			"affectedResources", "SkillSource, PromptPack.spec.skills, ArenaSource",
			"effect", "these reconcilers will report ContentStorageUnavailable",
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

	if err := controller.SetupIndexers(context.Background(), mgr); err != nil {
		setupLog.Error(err, "unable to setup field indexers")
		os.Exit(1)
	}

	// Internal service-to-service auth config (SEC-1/SEC-5), shared by the
	// session-api server side (ServiceBuilder) and the facade / eval-worker
	// caller side (AgentRuntimeReconciler). Zero value = disabled.
	serviceAuth := controller.ServiceAuthConfig{
		Enabled:                sessionAPIAuthEnabled,
		Audience:               sessionAPIAuthAudience,
		TokenExpirationSeconds: sessionAPIAuthTokenExpirationSeconds,
		IstioMTLS:              sessionAPIAuthIstioMTLS,
		ExtraSubjects:          splitAndTrim(sessionAPIAuthExtraSubjects),
	}

	if err := (&controller.AgentRuntimeReconciler{
		Client:                   mgr.GetClient(),
		Scheme:                   mgr.GetScheme(),
		FacadeImage:              facadeImage,
		FacadeImagePullPolicy:    corev1.PullPolicy(facadeImagePullPolicy),
		FrameworkImages:          frameworkImages.images(),
		FrameworkImagePullPolicy: corev1.PullPolicy(frameworkImagePullPolicy),
		TracingEnabled:           tracingEnabled,
		TracingEndpoint:          tracingEndpoint,
		RedisURL:                 redisAddr,
		SessionRedisURL:          sessionRedisURL,
		SessionRedisURLSecret: controller.SecretKeyRef{
			Name: sessionRedisURLSecretName,
			Key:  sessionRedisURLSecretKey,
		},
		EvalWorkerImage:            evalWorkerImage,
		EvalWorkerImagePullPolicy:  corev1.PullPolicy(evalWorkerImagePullPolicy),
		WorkspaceReaderRBACEnabled: workspaceReaderRBACEnabled,
		DefaultExposure: controller.DefaultExposureConfig{
			BaseDomain:       defaultExposureBaseDomain,
			GatewayName:      defaultExposureGatewayName,
			GatewayNamespace: defaultExposureGatewayNamespace,
			GatewaySection:   defaultExposureGatewaySection,
		},
		LicenseAPIURL:        licenseAPIURL,
		PolicyBrokerImage:    policyBrokerImageForEnterprise(enterpriseEnabled, policyBrokerImage),
		RolloutMetrics:       controller.NewRolloutMetrics(prometheus.DefaultRegisterer),
		WorkspaceContentPath: workspaceContentPath,
		MgmtPlaneJWKSURL:     mgmtPlaneJWKSURL,
		Recorder:             mgr.GetEventRecorderFor("agentruntime-controller"),
		MeshEnabled:          meshEnabled,
		ServiceAuth:          serviceAuth,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "AgentRuntime")
		os.Exit(1)
	}
	if err := (&controller.PromptPackReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		SchemaValidator:      schema.NewSchemaValidatorWithOptions(ctrl.Log, nil, 0),
		Recorder:             mgr.GetEventRecorderFor("promptpack-controller"),
		WorkspaceContentPath: workspaceContentPath,
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
		ServiceBuilder: &controller.ServiceBuilder{
			SessionImage:           sessionAPIImage,
			SessionImagePullPolicy: corev1.PullPolicy(sessionAPIImagePullPolicy),
			MemoryImage:            memoryAPIImage,
			MemoryImagePullPolicy:  corev1.PullPolicy(memoryAPIImagePullPolicy),
			PrivacyImage:           privacyAPIImage,
			PrivacyImagePullPolicy: corev1.PullPolicy(privacyAPIImagePullPolicy),
			MemoryRedisURL:         memoryRedisURL,
			MemoryRedisURLSecret: controller.SecretKeyRef{
				Name: memoryRedisURLSecretName,
				Key:  memoryRedisURLSecretKey,
			},
			MemoryCacheTTL:              memoryCacheTTL,
			MemoryConsolidationInterval: memoryConsolidationInterval,
			MemoryProjectionInterval:    memoryProjectionInterval,
			SessionRedisURL:             sessionRedisURL,
			SessionRedisURLSecret: controller.SecretKeyRef{
				Name: sessionRedisURLSecretName,
				Key:  sessionRedisURLSecretKey,
			},
			ServiceAuth:   serviceAuth,
			Enterprise:    enterpriseEnabled,
			LicenseAPIURL: licenseAPIURL,
		},
		WorkspaceReaderRBACEnabled:           workspaceReaderRBACEnabled,
		OperatorNamespace:                    os.Getenv("POD_NAMESPACE"),
		SessionAPITokenReviewClusterRole:     sessionAPITokenReviewClusterRole,
		PrivacyDefaultReaderClusterRole:      privacyDefaultReaderClusterRole,
		MemoryConsolidationReaderClusterRole: memoryConsolidationReaderClusterRole,
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
	if err := (&controller.MemoryPolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("memorypolicy-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "MemoryPolicy")
		os.Exit(1)
	}
	if err := (&controller.AgentPolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("agentpolicy-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "AgentPolicy")
		os.Exit(1)
	}
	if err := (&controller.SkillSourceReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Recorder:             mgr.GetEventRecorderFor("skillsource-controller"),
		WorkspaceContentPath: workspaceContentPath,
		MaxVersionsPerSource: 10,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, errUnableToCreateController, logKeyController, "SkillSource")
		os.Exit(1)
	}
	// Core admission webhooks — gated on the presence of serving certs, the
	// same gate the enterprise webhooks use. When webhook.enabled is false in
	// Helm, no cert path is set and these are skipped (status-quo behaviour).
	if len(webhookCertPath) > 0 {
		if err := omniawebhook.SetupAgentRuntimeWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to register webhook", "webhook", "AgentRuntime")
			os.Exit(1)
		}
		if err := omniawebhook.SetupSkillSourceWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to register webhook", "webhook", "SkillSource")
			os.Exit(1)
		}
		if err := omniawebhook.SetupProviderWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to register webhook", "webhook", "Provider")
			os.Exit(1)
		}
		if err := omniawebhook.SetupWorkspaceWebhookWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to register webhook", "webhook", "Workspace")
			os.Exit(1)
		}
	}

	// +kubebuilder:scaffold:builder

	// Enterprise controllers — gated behind --enterprise flag
	if enterpriseEnabled {
		eeOpts := eesetup.EnterpriseOptions{
			LicenseServerURL: licenseServerURL,
			ClusterName:      clusterName,
			EnableWebhooks:   len(webhookCertPath) > 0,
		}
		if err := eesetup.RegisterEnterpriseControllers(mgr, eeOpts); err != nil {
			setupLog.Error(err, "unable to register enterprise controllers")
			os.Exit(1)
		}
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

	// Start tool test API server if configured
	var apiServer *tooltest.Server
	if apiBindAddress != "" {
		var reviewer tooltest.TokenReviewer
		allowedSubjects := splitAndTrim(toolTestAllowedSubjects)
		if len(allowedSubjects) > 0 {
			reviewer, err = tooltest.NewK8sTokenReviewer(mgr.GetConfig())
			if err != nil {
				setupLog.Error(err, "unable to build token reviewer for tool-test API")
				os.Exit(1)
			}
		}
		apiServer = tooltest.NewServer(apiBindAddress, mgr.GetClient(), ctrl.Log, reviewer, allowedSubjects)
		go func() {
			if err := apiServer.Start(ctx); err != nil {
				setupLog.Error(err, "tool test API server stopped")
			}
		}()
	}

	// Start workspace-content API server if configured. It lets the dashboard
	// read/write workspace content via authenticated HTTP instead of mounting
	// the NFS content volume directly.
	var contentServer *content.Server
	if contentAPIBindAddress != "" {
		if mgmtPlaneJWKSURL == "" {
			setupLog.Error(fmt.Errorf("mgmt-plane-jwks-url required"),
				"content-api-bind-address requires --mgmt-plane-jwks-url")
			os.Exit(1)
		}
		verifier, verr := authz.NewIdentityVerifierFromJWKS(mgmtPlaneJWKSURL)
		if verr != nil {
			setupLog.Error(verr, "unable to build identity verifier for content API")
			os.Exit(1)
		}
		contentLog := ctrl.Log.WithName("content-api")
		authorizer := authz.NewAuthorizer(verifier, authz.NewClientWorkspaceResolver(mgr.GetClient()))
		contentServer = content.NewServer(contentAPIBindAddress,
			content.NewHandler(workspaceContentPath, contentLog), authorizer, contentLog)
		go func() {
			if err := contentServer.Start(ctx); err != nil {
				setupLog.Error(err, "content API server stopped")
			}
		}()
	}

	// Start deploy-intent API server if configured. It translates a versioned,
	// CRD-agnostic DeployIntent into PromptPack/AgentRuntime objects, so the
	// deploy adapter never constructs CRDs.
	var deployServer *deploy.Server
	if deployAPIBindAddress != "" {
		if mgmtPlaneJWKSURL == "" {
			setupLog.Error(fmt.Errorf("mgmt-plane-jwks-url required"),
				"deploy-api-bind-address requires --mgmt-plane-jwks-url")
			os.Exit(1)
		}
		verifier, verr := authz.NewIdentityVerifierFromJWKS(mgmtPlaneJWKSURL)
		if verr != nil {
			setupLog.Error(verr, "unable to build identity verifier for deploy API")
			os.Exit(1)
		}
		deployLog := ctrl.Log.WithName("deploy-api")
		authorizer := authz.NewAuthorizer(verifier, authz.NewClientWorkspaceResolver(mgr.GetClient()))
		handler := deploy.NewHandler(deploy.NewApplier(mgr.GetClient(), deployLog), deployLog)
		deployServer = deploy.NewServer(deployAPIBindAddress, handler, authorizer, deployLog)
		go func() {
			if err := deployServer.Start(ctx); err != nil {
				setupLog.Error(err, "deploy API server stopped")
			}
		}()
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	// Graceful shutdown of API servers
	if apiServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := apiServer.Shutdown(shutdownCtx); err != nil {
			setupLog.Error(err, "API server shutdown error")
		}
	}
	if contentServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := contentServer.Shutdown(shutdownCtx); err != nil {
			setupLog.Error(err, "content API server shutdown error")
		}
	}
	if deployServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := deployServer.Shutdown(shutdownCtx); err != nil {
			setupLog.Error(err, "deploy API server shutdown error")
		}
	}
}

// policyBrokerImageForEnterprise returns the policy broker image when enterprise
// is enabled, or empty string when disabled (which prevents sidecar injection
// and leaves the runtime's PolicyBrokerClient a no-op). When enterprise is
// enabled but no image is specified, the default is used.
func policyBrokerImageForEnterprise(enterpriseEnabled bool, image string) string {
	if !enterpriseEnabled {
		return ""
	}
	if image == "" {
		return controller.DefaultPolicyBrokerImage
	}
	return image
}

// frameworkImagesFlag is a repeatable --framework-image flag. Each value is
// "type=repo:tag"; a value with no "=" is the legacy bare form and maps to the
// "promptkit" framework for back-compat. Split is on the FIRST "=" so the
// "repo:tag" colon is preserved. Kept in main.go (not a sibling file) because
// the operator binary is built single-file (go build cmd/main.go) across the
// Makefile, Dockerfile, and E2E suite.
type frameworkImagesFlag struct {
	m map[string]string
}

const promptkitFrameworkKey = "promptkit"

func (f *frameworkImagesFlag) String() string {
	if f == nil || len(f.m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(f.m))
	for k, v := range f.m {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (f *frameworkImagesFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("framework-image value is empty")
	}
	if f.m == nil {
		f.m = map[string]string{}
	}
	key := promptkitFrameworkKey
	img := value
	if i := strings.Index(value, "="); i >= 0 {
		key = strings.TrimSpace(value[:i])
		img = strings.TrimSpace(value[i+1:])
		if key == "" {
			key = promptkitFrameworkKey
		}
	}
	if img == "" {
		return fmt.Errorf("framework-image %q has no image", value)
	}
	f.m[key] = img
	return nil
}

// images returns the accumulated type->image map (never nil).
func (f *frameworkImagesFlag) images() map[string]string {
	if f.m == nil {
		return map[string]string{}
	}
	return f.m
}

// splitAndTrim splits a comma-separated list into non-empty, trimmed entries.
func splitAndTrim(s string) []string {
	out := make([]string, 0)
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
