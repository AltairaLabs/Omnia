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

package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// validatePrivacyPolicyRef returns a Condition describing whether the AgentRuntime's
// privacyPolicyRef is resolvable. A missing ref is not an error — it means the
// workspace service-group or global default will apply.
// Missing refs do not block reconciliation — they are informational only.
func (r *AgentRuntimeReconciler) validatePrivacyPolicyRef(ctx context.Context, ar *omniav1alpha1.AgentRuntime) metav1.Condition {
	if ar.Spec.PrivacyPolicyRef == nil {
		return metav1.Condition{
			Type:    "PrivacyPolicyResolved",
			Status:  metav1.ConditionTrue,
			Reason:  "WorkspaceDefault",
			Message: "no privacyPolicyRef set; using workspace service group or global default",
		}
	}
	p := &eev1alpha1.SessionPrivacyPolicy{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      ar.Spec.PrivacyPolicyRef.Name,
		Namespace: ar.Namespace,
	}, p)
	if err != nil {
		return metav1.Condition{
			Type:    "PrivacyPolicyResolved",
			Status:  metav1.ConditionFalse,
			Reason:  "PolicyNotFound",
			Message: fmt.Sprintf("privacyPolicyRef %q not found: %v", ar.Spec.PrivacyPolicyRef.Name, err),
		}
	}
	return metav1.Condition{
		Type:    "PrivacyPolicyResolved",
		Status:  metav1.ConditionTrue,
		Reason:  "PolicyResolved",
		Message: fmt.Sprintf("using SessionPrivacyPolicy %q", ar.Spec.PrivacyPolicyRef.Name),
	}
}

// TODO(scalability): Reconcile calls Status().Update() multiple times per reconciliation
// (once per condition change). Accumulate all condition changes and call Status().Update()
// once at the end to reduce API server load. This requires a larger refactor across the
// reconcileReferences, reconcileDeployment, and reconcileService paths.

// AgentRuntimeReconciler reconciles a AgentRuntime object
type AgentRuntimeReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	FacadeImage           string
	FacadeImagePullPolicy corev1.PullPolicy
	// FrameworkImages maps framework type (e.g. "promptkit", "langchain") to a
	// release-pinned runtime image. Populated from the repeatable
	// --framework-image flag. The selector falls back to a built-in :latest
	// default only for promptkit when a type is absent (bare-dev); every
	// other type (langchain, autogen, custom) blocks loudly for types with
	// no image (see resolveFrameworkImage).
	FrameworkImages          map[string]string
	FrameworkImagePullPolicy corev1.PullPolicy
	// Tracing configuration for runtime containers
	TracingEnabled  bool
	TracingEndpoint string
	// RedisURL is the Redis connection URL (redis:// or rediss://)
	// forwarded to eval-worker pods via REDIS_URL env. Same canonical
	// form used by every other Redis consumer in the codebase.
	RedisURL string
	// SessionRedisURL / SessionRedisURLSecret are the operator-wide session
	// redis default, used as the fallback when resolving a service group's
	// eval-worker redis (which must match the group's session-api redis).
	SessionRedisURL       string
	SessionRedisURLSecret SecretKeyRef
	// EvalWorkerImage overrides the default eval worker container image
	EvalWorkerImage string
	// EvalWorkerImagePullPolicy sets the imagePullPolicy on eval worker containers
	EvalWorkerImagePullPolicy corev1.PullPolicy
	// AgentWorkspaceReaderClusterRole is the name of the ClusterRole that grants
	// agent pods read access to Workspace CRDs (for service URL resolution).
	AgentWorkspaceReaderClusterRole string
	// DefaultExposure configures external exposure (#1553). See DefaultExposureConfig.
	DefaultExposure DefaultExposureConfig
	// LicenseAPIURL is the operator/arena-controller license endpoint,
	// stamped onto the policy-broker sidecar as OPERATOR_API_URL so it logs a
	// startup nag when unlicensed (#1682). Empty disables the nag. Never gates.
	LicenseAPIURL string
	// PolicyBrokerImage is the container image for the ToolPolicy decision
	// sidecar (P2.3). When set, this sidecar is injected into the agent pod
	// alongside the runtime, which is pointed at it via POLICY_BROKER_URL. If
	// empty, no broker sidecar is injected and the runtime's PolicyBrokerClient
	// stays a no-op. Reuses LicenseAPIURL for the broker's OPERATOR_API_URL
	// license nag.
	PolicyBrokerImage string
	// WorkspaceContentPath is the base path for the workspace content PVC.
	// When set, the runtime container mounts the workspace content PVC at
	// this path (read-only) and receives OMNIA_PROMPTPACK_MANIFEST_PATH
	// pointing at the per-pack skill manifest the PromptPack reconciler
	// emits. When empty, no PVC mount happens — skills are disabled.
	WorkspaceContentPath string
	// RolloutMetrics holds Prometheus metrics for rollout observability.
	// Nil in tests that don't need metrics.
	RolloutMetrics *RolloutMetrics
	// OIDCHTTPClient is the HTTP client used to fetch the OIDC
	// discovery document and JWKS when spec.externalAuth.oidc is set.
	// Nil uses a default client with a bounded timeout — tests inject
	// an httptest.Server-backed client here.
	OIDCHTTPClient *http.Client
	// JWKSClock provides the current time for the OIDC JWKS fresh-
	// cache calculation (T8 fast-path). Nil falls back to time.Now;
	// tests inject a deterministic clock to make cache-expiry
	// assertions stable.
	JWKSClock func() time.Time
	// MgmtPlaneJWKSURL is the dashboard's JWKS endpoint, set on every
	// facade container as OMNIA_MGMT_PLANE_JWKS_URL so cmd/agent can
	// build a JWKS-backed mgmt-plane validator. Empty disables wiring
	// (Arena E2E, headless installs without a dashboard).
	MgmtPlaneJWKSURL string

	// Recorder emits Kubernetes Events for rollout traffic-routing degrade /
	// approximation so operators see config/capability mismatch in `kubectl
	// get events`.
	Recorder record.EventRecorder

	// MeshEnabled reflects the chart's --mesh-enabled flag (Istio ambient on).
	// Gates whether `mode: mesh` is usable; when false, mesh requests degrade
	// to replicaWeighted.
	MeshEnabled bool

	// ServiceAuth carries internal service-to-service ServiceAccount auth
	// settings (SEC-1/SEC-5). When enabled, facade pods (which write sessions
	// to session-api via httpclient) and eval-worker pods get an audience-bound
	// projected SA token + SESSION_API_TOKEN_PATH. Zero value = disabled.
	ServiceAuth ServiceAuthConfig

	// gatewayAPIPresent records whether the Gateway API CRDs
	// (gateway.networking.k8s.io) are served by the cluster, detected once at
	// SetupWithManager time. When false, the HTTPRoute/Gateway watches are not
	// registered and reconcileFacadeEndpoints clears status.facade rather than
	// listing absent CRDs. Installing the CRDs requires an operator restart to
	// re-detect.
	gatewayAPIPresent bool
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=rolloutanalyses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=keda.sh,resources=scaledobjects,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch

// reconcileReferences fetches and validates all referenced resources.
// Returns promptPack (required), toolRegistry (optional), providers map, and any error.
func (r *AgentRuntimeReconciler) reconcileReferences(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
) (*omniav1alpha1.PromptPack, *omniav1alpha1.ToolRegistry, map[string]*omniav1alpha1.Provider, ctrl.Result, error) {
	// Fetch required PromptPack. resolvePromptPack fails two distinct ways:
	// errNoMatchingPromptPack (no PromptPack matches packName+version/track —
	// the ref points nowhere) or a ref validation error (both version and
	// track set). Surface these under different reasons so an operator can
	// tell "fix the ref" apart from "the pack doesn't exist yet".
	promptPack, err := r.resolvePromptPack(ctx, agentRuntime.Namespace, agentRuntime.Spec.PromptPackRef)
	if err != nil {
		reason := "PromptPackRefInvalid"
		if errors.Is(err, errNoMatchingPromptPack) {
			reason = "PromptPackNotFound"
		}
		r.handleRefError(ctx, log, agentRuntime, ConditionTypePromptPackReady, reason, err)
		return nil, nil, nil, ctrl.Result{}, err
	}
	// Gate readiness on the PromptPack's schema validity. A pack that failed
	// schema validation makes every conversation fail at open-time, so refuse
	// to bring the agent up and surface the reason clearly rather than serving
	// a silently-broken agent (#1299). This gates the STABLE pack only; the
	// candidate now resolves and mounts its own pack via
	// rollout.candidate.promptPackRef (reconcileCandidateDeployment), so a bad
	// candidate pack surfaces as candidate pods failing to roll out (the
	// rollout's pod-health auto-rollback path) rather than here.
	if reason := promptPackInvalidReason(promptPack); reason != "" {
		SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypePromptPackReady, metav1.ConditionFalse,
			"PromptPackInvalid", reason)
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseFailed
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return nil, nil, nil, ctrl.Result{RequeueAfter: 15 * time.Second}, nil
	}
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypePromptPackReady, metav1.ConditionTrue,
		"PromptPackFound", "PromptPack resource found and schema-valid")

	// Fetch optional ToolRegistry
	var toolRegistry *omniav1alpha1.ToolRegistry
	if agentRuntime.Spec.ToolRegistryRef != nil {
		toolRegistry, err = r.fetchToolRegistry(ctx, agentRuntime)
		if err != nil {
			SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeToolRegistryReady, metav1.ConditionFalse,
				"ToolRegistryNotFound", err.Error())
			log.Info("ToolRegistry not found, continuing without tools", "error", err)
		} else {
			SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeToolRegistryReady, metav1.ConditionTrue,
				"ToolRegistryFound", "ToolRegistry resource found")
		}
	}

	// Fetch providers
	providers, result, err := r.reconcileProviders(ctx, log, agentRuntime)
	if err != nil || result.RequeueAfter > 0 {
		return nil, nil, nil, result, err
	}

	return promptPack, toolRegistry, providers, ctrl.Result{}, nil
}

// reconcileProviders resolves the providers map from the AgentRuntime spec.
func (r *AgentRuntimeReconciler) reconcileProviders(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
) (map[string]*omniav1alpha1.Provider, ctrl.Result, error) {
	providers := make(map[string]*omniav1alpha1.Provider)

	for _, np := range agentRuntime.Spec.Providers {
		provider, result, err := r.fetchAndValidateProvider(ctx, log, agentRuntime, np)
		if err != nil || result.RequeueAfter > 0 {
			return nil, result, err
		}
		providers[np.Name] = provider
	}

	return providers, ctrl.Result{}, nil
}

// fetchAndValidateProvider fetches a Provider by ref, validates its status,
// and checks that it advertises all required capabilities.
func (r *AgentRuntimeReconciler) fetchAndValidateProvider(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
	np omniav1alpha1.NamedProviderRef,
) (*omniav1alpha1.Provider, ctrl.Result, error) {
	provider, err := r.fetchProviderByRef(ctx, np.ProviderRef, agentRuntime.Namespace)
	if err != nil {
		r.handleRefError(ctx, log, agentRuntime, ConditionTypeProviderReady, "ProviderNotFound", err)
		return nil, ctrl.Result{}, err
	}
	// A provider with a SET phase that isn't Ready is not usable. Previously
	// this gated only on == Error, so an Unavailable (unreachable) or Pending
	// provider sailed through and the AgentRuntime reported Ready while
	// referencing a provider it can't use. An empty phase is still treated as
	// ready — that's the brief optimistic window before the provider
	// controller writes status, and blocking it would stall every fresh agent.
	// The 10s requeue lets a recovering provider clear this without a spec edit.
	if provider.Status.Phase != "" && provider.Status.Phase != omniav1alpha1.ProviderPhaseReady {
		SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeProviderReady, metav1.ConditionFalse,
			"ProviderNotReady", fmt.Sprintf("Provider %s is not ready (phase: %q)", provider.Name, provider.Status.Phase))
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return nil, ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if mismatch := providerRoleMismatch(provider, np.Role); mismatch != "" {
		SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeProviderReady, metav1.ConditionFalse,
			"RoleMismatch", mismatch)
		// Role mismatch is a configuration error — won't self-resolve
		// until the spec changes. Park the runtime in Failed phase.
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseFailed
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return nil, ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	if missing := missingCapabilities(provider, np.RequiredCapabilities); len(missing) > 0 {
		msg := fmt.Sprintf("Provider %s missing required capabilities: %v", provider.Name, missing)
		SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeProviderReady, metav1.ConditionFalse,
			"CapabilityMismatch", msg)
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return nil, ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeProviderReady, metav1.ConditionTrue,
		"ProviderFound", "Provider resource found and ready")
	return provider, ctrl.Result{}, nil
}

// providerRoleMismatch returns an empty string when the Provider's role
// matches the ref's required role, or a user-facing message describing the
// mismatch otherwise. Treats an empty role on either side as `llm` for
// back-compat with pre-role Providers and AgentRuntimes.
func providerRoleMismatch(provider *omniav1alpha1.Provider, required omniav1alpha1.ProviderRole) string {
	if required == "" {
		required = omniav1alpha1.ProviderRoleLLM
	}
	if provider.EffectiveRole() == required {
		return ""
	}
	return fmt.Sprintf("Provider %s has role %q but ref requires role %q",
		provider.Name, provider.EffectiveRole(), required)
}

// missingCapabilities returns the required capabilities not present in the
// provider's advertised capabilities. Returns nil if all are satisfied or
// if required is empty.
func missingCapabilities(provider *omniav1alpha1.Provider, required []omniav1alpha1.ProviderCapability) []omniav1alpha1.ProviderCapability {
	if len(required) == 0 {
		return nil
	}
	have := make(map[omniav1alpha1.ProviderCapability]bool, len(provider.Spec.Capabilities))
	for _, c := range provider.Spec.Capabilities {
		have[c] = true
	}
	var missing []omniav1alpha1.ProviderCapability
	for _, c := range required {
		if !have[c] {
			missing = append(missing, c)
		}
	}
	return missing
}

// handleRefError handles reference fetch errors by setting condition, updating status, and logging.
func (r *AgentRuntimeReconciler) handleRefError(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
	condType string,
	reason string,
	err error,
) {
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, condType, metav1.ConditionFalse, reason, err.Error())
	agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseFailed
	if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
		log.Error(statusErr, logMsgFailedToUpdateStatus)
	}
}

// reconcileResources creates/updates Deployment and Service.
func (r *AgentRuntimeReconciler) reconcileResources(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
	promptPack *omniav1alpha1.PromptPack,
	toolRegistry *omniav1alpha1.ToolRegistry,
	providers map[string]*omniav1alpha1.Provider,
) (*appsv1.Deployment, error) {
	// #1206: block (don't silently substitute PromptKit) when the declared
	// framework.type has no resolvable runtime image.
	if _, ok := r.resolveFrameworkImage(agentRuntime); !ok {
		ft := frameworkTypeKey(agentRuntime)
		msg := fmt.Sprintf("no runtime image configured for framework type %q; set spec.framework.image or configure --framework-image=%s=<image>", ft, ft)
		SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation,
			ConditionTypeFrameworkReady, metav1.ConditionFalse, reasonFrameworkImageUnavailable, msg)
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		if r.Recorder != nil {
			r.Recorder.Event(agentRuntime, corev1.EventTypeWarning, reasonFrameworkImageUnavailable, msg)
		}
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return nil, fmt.Errorf("framework image unavailable for type %q", ft)
	}
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation,
		ConditionTypeFrameworkReady, metav1.ConditionTrue, "FrameworkImageResolved", "runtime image resolved for framework type")

	// Reject cross-namespace toolRegistryRef before provisioning anything (#1874).
	if err := r.rejectCrossNamespaceToolRegistry(ctx, log, agentRuntime); err != nil {
		return nil, err
	}

	// Reconcile facade RBAC (ServiceAccount, Role, RoleBinding)
	if err := r.reconcileFacadeRBAC(ctx, agentRuntime); err != nil {
		log.Error(err, "Failed to reconcile facade RBAC")
		// Don't fail the reconciliation for RBAC errors, just log
	}

	// Reconcile operator-provisioned external exposure (#1553); best-effort.
	if err := r.reconcileFacadeRoute(ctx, agentRuntime); err != nil {
		log.Error(err, "Failed to reconcile facade HTTPRoute")
	}

	// Reconcile tools ConfigMap and companion tool-secrets Secret
	if toolRegistry != nil {
		// Fail loud on tool auth the operator cannot honor (e.g. workloadIdentity
		// on a non-http handler) before provisioning any credentials.
		if err := validateToolAuthTypes(toolRegistry); err != nil {
			log.Error(err, "unsupported tool auth configuration")
			r.handleRefError(ctx, log, agentRuntime, ConditionTypeDeploymentReady, "ToolAuthUnsupported", err)
			return nil, err
		}
		if err := r.reconcileToolSecrets(ctx, agentRuntime, toolRegistry); err != nil {
			log.Error(err, "Failed to reconcile tool-secrets Secret")
			return nil, err // fail loud — do not proceed with a broken auth config
		}
		if err := r.reconcileToolsConfigMap(ctx, agentRuntime, toolRegistry); err != nil {
			log.Error(err, "Failed to reconcile tools ConfigMap")
		}
	}

	// Reconcile Deployment
	deployment, err := r.reconcileDeployment(ctx, agentRuntime, promptPack, toolRegistry, providers)
	if err != nil {
		r.handleRefError(ctx, log, agentRuntime, ConditionTypeDeploymentReady, "DeploymentFailed", err)
		return nil, err
	}
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeDeploymentReady, metav1.ConditionTrue,
		"DeploymentCreated", "Deployment created/updated successfully")

	// Reconcile Service
	if err := r.reconcileService(ctx, agentRuntime); err != nil {
		r.handleRefError(ctx, log, agentRuntime, ConditionTypeServiceReady, "ServiceFailed", err)
		return nil, err
	}
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeServiceReady, metav1.ConditionTrue,
		"ServiceCreated", "Service created/updated successfully")

	// Reconcile PDB (only meaningful when replicas > 1)
	if err := r.reconcilePDB(ctx, agentRuntime); err != nil {
		log.Error(err, "Failed to reconcile PDB")
	}

	return deployment, nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *AgentRuntimeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the AgentRuntime instance
	agentRuntime := &omniav1alpha1.AgentRuntime{}
	if err := r.Get(ctx, req.NamespacedName, agentRuntime); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AgentRuntime resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AgentRuntime")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !agentRuntime.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, agentRuntime)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(agentRuntime, FinalizerName) {
		controllerutil.AddFinalizer(agentRuntime, FinalizerName)
		if err := r.Update(ctx, agentRuntime); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Millisecond}, nil
	}

	// Track the observed generation up front so EVERY status write below —
	// including the dependency-missing / Failed early-return paths — carries a
	// current observedGeneration. Without this, a Failed agent keeps an empty
	// observedGeneration and consumers can't tell a current failure from a
	// stale snapshot (#1491).
	agentRuntime.Status.ObservedGeneration = agentRuntime.Generation

	// Initialize status if needed
	if agentRuntime.Status.Phase == "" {
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		if err := r.Status().Update(ctx, agentRuntime); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Fetch all references
	promptPack, toolRegistry, providers, result, err := r.reconcileReferences(ctx, log, agentRuntime)
	if err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	// Version-triggered rollout (#1838): if a newer PromptPack version has
	// appeared on the watched channel, set it as the rollout candidate and
	// requeue so the next pass runs the rollout engine cleanly (avoids a
	// resourceVersion conflict with the rollout's own Status write below).
	if triggered, triggerErr := r.maybeTriggerVersionRollout(ctx, agentRuntime); triggerErr != nil {
		return ctrl.Result{}, triggerErr
	} else if triggered {
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile resources
	deployment, err := r.reconcileResources(ctx, log, agentRuntime, promptPack, toolRegistry, providers)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Gate on the capabilities the running runtime advertises (§4.4). Sets the
	// CapabilitiesSatisfied condition (the deployment builder scales to 0 while
	// it's False for the current generation); requeues while within the report
	// grace window.
	capRequeue := r.enforceCapabilities(log, agentRuntime, deployment)

	// Reconcile rollout (candidate Deployment, step progression)
	if rolloutResult, rolloutErr := r.reconcileRollout(ctx, agentRuntime, promptPack, toolRegistry, providers); rolloutErr != nil {
		log.Error(rolloutErr, "rollout reconciliation failed")
		return ctrl.Result{}, rolloutErr
	} else if rolloutResult.RequeueAfter > 0 {
		// Persist status before early return so rollout progress is not lost.
		// observedGeneration is already set at the top of Reconcile.
		if err := r.Status().Update(ctx, agentRuntime); err != nil {
			return ctrl.Result{}, err
		}
		return rolloutResult, nil
	}

	// Reconcile autoscaling (HPA or KEDA, resolved from the agent's own policy
	// or the inherited WorkspaceServiceGroup default). Non-blocking: errors are
	// logged and surfaced via the AutoscalingReady condition, not fatal.
	autoscalingCond, err := r.reconcileAutoscaling(ctx, agentRuntime)
	if err != nil {
		log.Error(err, "Failed to reconcile autoscaling")
	}
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation,
		autoscalingCond.Type, autoscalingCond.Status, autoscalingCond.Reason, autoscalingCond.Message)

	// Reconcile eval worker deployment for non-PromptKit agents with evals enabled
	if err := r.reconcileEvalWorker(ctx, agentRuntime); err != nil {
		log.Error(err, "Failed to reconcile eval worker")
		// Don't fail the reconciliation for eval worker errors, just log
	}

	// Resolve A2A clients and update A2A status.
	r.reconcileA2AStatus(ctx, log, agentRuntime)

	// Update status from deployment
	r.updateStatusFromDeployment(agentRuntime, deployment, promptPack)

	// Validate privacyPolicyRef (non-blocking)
	privacyCond := r.validatePrivacyPolicyRef(ctx, agentRuntime)
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation,
		privacyCond.Type, privacyCond.Status, privacyCond.Reason, privacyCond.Message)

	// Surface facade auth configuration as a status condition so
	// operators can see at a glance whether the agent admits traffic.
	// Catches the Unreachable combo (no facade has managementPlane enabled and no
	// data-plane validator) which otherwise 401s silently at runtime.
	authCond := evaluateExternalAuthCondition(agentRuntime)
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation,
		authCond.Type, authCond.Status, authCond.Reason, authCond.Message)

	// Mirror the OIDC issuer's JWKS into a per-agent Secret (if
	// spec.externalAuth.oidc is configured). Non-blocking: failures
	// set the OIDCJWKSReady=False condition and schedule a refresh.
	jwksNext, err := r.reconcileOIDCJWKS(ctx, agentRuntime)
	if err != nil {
		log.Error(err, "OIDC JWKS reconciliation failed")
	}

	// Set overall Ready condition
	if agentRuntime.Status.Replicas != nil && agentRuntime.Status.Replicas.Ready > 0 {
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhaseRunning
		SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeReady, metav1.ConditionTrue,
			"RuntimeReady", "AgentRuntime is ready")
	} else {
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeReady, metav1.ConditionFalse,
			"RuntimeNotReady", "Waiting for pods to be ready")
	}

	// Publish externally-reachable facade endpoints derived from observed
	// HTTPRoutes/Gateways into status.facade. No-op when the Gateway API is not
	// installed. Done just before the status write so it persists in the same
	// Status().Update below — do not add a second status write.
	r.reconcileFacadeEndpoints(ctx, agentRuntime)

	// observedGeneration is already set at the top of Reconcile.
	if err := r.Status().Update(ctx, agentRuntime); err != nil {
		return ctrl.Result{}, err
	}

	return scheduleOIDCJWKSRefresh(earliestRequeue(jwksNext, capRequeue)), nil
}

func (r *AgentRuntimeReconciler) reconcileDelete(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Handling deletion of AgentRuntime")

	// Owned resources (Deployment, Service) will be garbage collected automatically
	// due to OwnerReferences

	// Remove finalizer
	controllerutil.RemoveFinalizer(agentRuntime, FinalizerName)
	if err := r.Update(ctx, agentRuntime); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// promptPackInvalidReason returns a user-facing message when the PromptPack is
// definitively unusable (failed schema validation), or "" when it is usable.
// Reusable so a rollout can validate a candidate pack independently once the
// candidate.promptPackVersion override resolves a distinct pack.
func promptPackInvalidReason(pp *omniav1alpha1.PromptPack) string {
	for i := range pp.Status.Conditions {
		c := pp.Status.Conditions[i]
		if c.Type == PromptPackConditionTypeSchemaValid && c.Status == metav1.ConditionFalse {
			if c.Message != "" {
				return fmt.Sprintf("PromptPack %s failed schema validation: %s", pp.Name, c.Message)
			}
			return fmt.Sprintf("PromptPack %s failed schema validation", pp.Name)
		}
	}
	if pp.Status.Phase == omniav1alpha1.PromptPackPhaseFailed {
		return fmt.Sprintf("PromptPack %s is in %s phase", pp.Name, pp.Status.Phase)
	}
	return ""
}

// resolvePromptPack resolves a PromptPackRef to a single PromptPack object.
// PromptPacks are label-keyed, not name-keyed: every version of a logical
// pack is its own resource sharing the label LabelPromptPackName=ref.Name, so
// resolution lists the candidates sharing that label and then picks one by
// exact version pin or channel (track) via selectPromptPack. Used for both
// the stable ref and a candidate override — each resolves independently since
// version/track (not just name) can differ between them.
func (r *AgentRuntimeReconciler) fetchToolRegistry(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) (*omniav1alpha1.ToolRegistry, error) {
	ref := agentRuntime.Spec.ToolRegistryRef
	toolRegistry := &omniav1alpha1.ToolRegistry{}

	namespace := agentRuntime.Namespace
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}

	key := types.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}
	if err := r.Get(ctx, key, toolRegistry); err != nil {
		return nil, fmt.Errorf("failed to get ToolRegistry %s: %w", key, err)
	}
	return toolRegistry, nil
}

// fetchProviderByRef fetches a Provider by ref with a default namespace.
func (r *AgentRuntimeReconciler) fetchProviderByRef(ctx context.Context, ref omniav1alpha1.ProviderRef, defaultNS string) (*omniav1alpha1.Provider, error) {
	provider := &omniav1alpha1.Provider{}

	namespace := defaultNS
	if ref.Namespace != nil {
		namespace = *ref.Namespace
	}

	key := types.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}
	if err := r.Get(ctx, key, provider); err != nil {
		return nil, fmt.Errorf("failed to get Provider %s: %w", key, err)
	}
	return provider, nil
}

func (r *AgentRuntimeReconciler) reconcileService(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) error {
	log := logf.FromContext(ctx)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      agentRuntime.Name,
			Namespace: agentRuntime.Namespace,
		},
	}

	port := primaryFacadePort(agentRuntime)

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, func() error {
		// Set owner reference
		if err := controllerutil.SetControllerReference(agentRuntime, service, r.Scheme); err != nil {
			return err
		}

		labels := map[string]string{
			labelAppName:      labelValueOmniaAgent,
			labelAppInstance:  agentRuntime.Name,
			labelAppManagedBy: labelValueOmniaOperator,
			labelOmniaComp:    "agent",
		}

		// In ambient mesh mode, enroll the Service in its waypoint so the
		// operator-owned VirtualService's L7 stable/candidate split actually
		// takes effect. Stamped here (not left to manual labelling) because the
		// operator owns this Service and overwrites its labels every reconcile.
		if wp := r.meshWaypointFor(ctx, agentRuntime); wp != "" {
			labels[labelIstioUseWaypoint] = wp
		}

		// No prometheus.io/* scrape annotations on the Service: it only exposes
		// the facade app port (8080) and optional a2a/mcp ports — NOT the
		// metrics ports (facade 8081, runtime 9001 live on the pod, not the
		// Service). Pointing an annotation-based scraper here would 404 on
		// 8080. Metrics are discovered via the pod's "metrics"-named container
		// ports instead (see deployment_builder podAnnotations and the
		// omnia-agents scrape job / PodMonitor).
		service.Labels = labels
		ports := []corev1.ServicePort{
			{
				Name:       "facade",
				Port:       port,
				TargetPort: intstr.FromString("facade"),
				Protocol:   corev1.ProtocolTCP,
			},
		}

		// Dual-protocol: expose A2A port alongside the primary facade.
		if isDualProtocol(agentRuntime) {
			ports = append(ports, corev1.ServicePort{
				Name:       "a2a",
				Port:       a2aSecondaryPort(agentRuntime),
				TargetPort: intstr.FromString("a2a"),
				Protocol:   corev1.ProtocolTCP,
			})
		}

		// MCP: expose MCP port on function-mode pods when enabled.
		ports = appendMCPServicePort(ports, agentRuntime)

		// Internal management-plane twin ports (ClusterIP-only) for each facade
		// whose managementPlane is enabled.
		ports = appendManagementServicePorts(ports, agentRuntime)

		// Classify every port for Istio L7 (waypoint/sidecar). All facade
		// protocols are HTTP, so this stamps appProtocol=http on each port —
		// never silently left as opaque TCP, which would break mode=mesh routing
		// and the facade WS upgrade.
		setAgentPortAppProtocols(ports)

		service.Spec = corev1.ServiceSpec{
			Selector: labels,
			Ports:    ports,
			Type:     corev1.ServiceTypeClusterIP,
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Set the service endpoint in status for dashboard/client connections
	agentRuntime.Status.ServiceEndpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
		agentRuntime.Name, agentRuntime.Namespace, port)

	// Advertise the internal management-plane ports so the dashboard and
	// in-cluster callers can discover them explicitly (never computed).
	agentRuntime.Status.ManagementEndpoints = managementEndpointsStatus(agentRuntime)

	log.Info("Service reconciled", "result", result, "endpoint", agentRuntime.Status.ServiceEndpoint)
	return nil
}

func (r *AgentRuntimeReconciler) updateStatusFromDeployment(
	agentRuntime *omniav1alpha1.AgentRuntime,
	deployment *appsv1.Deployment,
	promptPack *omniav1alpha1.PromptPack,
) {
	agentRuntime.Status.Replicas = &omniav1alpha1.ReplicaStatus{
		Desired:   deployment.Status.Replicas,
		Ready:     deployment.Status.ReadyReplicas,
		Available: deployment.Status.AvailableReplicas,
	}

	version := promptPack.Spec.Version
	agentRuntime.Status.ActiveVersion = &version
}

// reconcileA2AStatus resolves A2A client references and populates A2A status fields.
func (r *AgentRuntimeReconciler) reconcileA2AStatus(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
) {
	if facadeOfType(agentRuntime, omniav1alpha1.FacadeTypeA2A) == nil {
		return
	}

	// Standalone a2a binds the primary facade port; a dual-protocol secondary
	// a2a binds the a2a port.
	port := primaryFacadePort(agentRuntime)
	if isDualProtocol(agentRuntime) {
		port = a2aSecondaryPort(agentRuntime)
	}

	endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		agentRuntime.Name, agentRuntime.Namespace, port)

	if agentRuntime.Status.A2A == nil {
		agentRuntime.Status.A2A = &omniav1alpha1.A2AStatus{}
	}
	agentRuntime.Status.A2A.Endpoint = endpoint
	agentRuntime.Status.A2A.AgentCardURL = endpoint + "/.well-known/agent.json"

	// Resolve client references.
	_, clientStatuses := r.resolveA2AClients(ctx, log, agentRuntime)
	agentRuntime.Status.A2A.Clients = clientStatuses
}

// SetupWithManager and the controller's watch wiring live in
// agentruntime_watches.go.
