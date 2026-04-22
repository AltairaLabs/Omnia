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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
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
	Scheme                   *runtime.Scheme
	FacadeImage              string
	FacadeImagePullPolicy    corev1.PullPolicy
	FrameworkImage           string
	FrameworkImagePullPolicy corev1.PullPolicy
	// Tracing configuration for runtime containers
	TracingEnabled  bool
	TracingEndpoint string
	// RedisAddr is the Redis address for eval worker deployments
	RedisAddr string
	// EvalWorkerImage overrides the default eval worker container image
	EvalWorkerImage string
	// AgentWorkspaceReaderClusterRole is the name of the ClusterRole that grants
	// agent pods read access to Workspace CRDs (for service URL resolution).
	AgentWorkspaceReaderClusterRole string
	// PolicyProxyImage is the container image for the ToolPolicy enforcement
	// sidecar. When a ToolPolicy exists in the agent's namespace, this sidecar
	// is injected into the agent pod to evaluate CEL rules before tool execution.
	// If empty, the default image from policy_proxy_sidecar.go is used.
	PolicyProxyImage string
	// WorkspaceContentPath is the base path for the workspace content PVC.
	// When set, the runtime container mounts the workspace content PVC at
	// this path (read-only) and receives OMNIA_PROMPTPACK_MANIFEST_PATH
	// pointing at the per-pack skill manifest the PromptPack reconciler
	// emits. When empty, no PVC mount happens — skills are disabled.
	WorkspaceContentPath string
	// RolloutMetrics holds Prometheus metrics for rollout observability.
	// Nil in tests that don't need metrics.
	RolloutMetrics *RolloutMetrics
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch
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

// reconcileReferences fetches and validates all referenced resources.
// Returns promptPack (required), toolRegistry (optional), providers map, and any error.
func (r *AgentRuntimeReconciler) reconcileReferences(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
) (*omniav1alpha1.PromptPack, *omniav1alpha1.ToolRegistry, map[string]*omniav1alpha1.Provider, ctrl.Result, error) {
	// Fetch required PromptPack
	promptPack, err := r.fetchPromptPack(ctx, agentRuntime)
	if err != nil {
		r.handleRefError(ctx, log, agentRuntime, ConditionTypePromptPackReady, "PromptPackNotFound", err)
		return nil, nil, nil, ctrl.Result{}, err
	}
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypePromptPackReady, metav1.ConditionTrue,
		"PromptPackFound", "PromptPack resource found")

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
	if provider.Status.Phase == omniav1alpha1.ProviderPhaseError {
		SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation, ConditionTypeProviderReady, metav1.ConditionFalse,
			"ProviderNotReady", fmt.Sprintf("Provider %s is in %s phase", provider.Name, provider.Status.Phase))
		agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
		if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return nil, ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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
	// Reconcile facade RBAC (ServiceAccount, Role, RoleBinding)
	if err := r.reconcileFacadeRBAC(ctx, agentRuntime); err != nil {
		log.Error(err, "Failed to reconcile facade RBAC")
		// Don't fail the reconciliation for RBAC errors, just log
	}

	// Reconcile tools ConfigMap
	if toolRegistry != nil {
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

	// Project the deprecated spec.a2a.authentication.secretRef into
	// spec.externalAuth.sharedToken so downstream code (cmd/agent's
	// chain builder, the dashboard UI, etc.) only has to look at one
	// field. In-memory only — never persisted. PR 2a added the helper;
	// PR 2b wires it.
	projectLegacyA2AAuth(agentRuntime)

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

	// Reconcile resources
	deployment, err := r.reconcileResources(ctx, log, agentRuntime, promptPack, toolRegistry, providers)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Reconcile rollout (candidate Deployment, step progression)
	if rolloutResult, rolloutErr := r.reconcileRollout(ctx, agentRuntime, promptPack, toolRegistry, providers); rolloutErr != nil {
		log.Error(rolloutErr, "rollout reconciliation failed")
		return ctrl.Result{}, rolloutErr
	} else if rolloutResult.RequeueAfter > 0 {
		// Persist status before early return so rollout progress is not lost.
		agentRuntime.Status.ObservedGeneration = agentRuntime.Generation
		if err := r.Status().Update(ctx, agentRuntime); err != nil {
			return ctrl.Result{}, err
		}
		return rolloutResult, nil
	}

	// Reconcile autoscaling (HPA or KEDA if enabled)
	if err := r.reconcileAutoscaling(ctx, agentRuntime); err != nil {
		log.Error(err, "Failed to reconcile autoscaling")
		// Don't fail the reconciliation for autoscaling errors, just log
	}

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

	agentRuntime.Status.ObservedGeneration = agentRuntime.Generation
	if err := r.Status().Update(ctx, agentRuntime); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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

func (r *AgentRuntimeReconciler) fetchPromptPack(ctx context.Context, agentRuntime *omniav1alpha1.AgentRuntime) (*omniav1alpha1.PromptPack, error) {
	promptPack := &omniav1alpha1.PromptPack{}
	key := types.NamespacedName{
		Name:      agentRuntime.Spec.PromptPackRef.Name,
		Namespace: agentRuntime.Namespace,
	}
	if err := r.Get(ctx, key, promptPack); err != nil {
		return nil, fmt.Errorf("failed to get PromptPack %s: %w", key, err)
	}
	return promptPack, nil
}

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

	port := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		port = *agentRuntime.Spec.Facade.Port
	}

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

		// Prometheus scrape annotations on Service (not pod, as Istio overrides pod annotations)
		annotations := map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   fmt.Sprintf("%d", port),
			"prometheus.io/path":   "/metrics",
		}

		service.Labels = labels
		service.Annotations = annotations
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
			a2aPort := int32(DefaultA2APort)
			if agentRuntime.Spec.A2A.Port != nil {
				a2aPort = *agentRuntime.Spec.A2A.Port
			}
			ports = append(ports, corev1.ServicePort{
				Name:       "a2a",
				Port:       a2aPort,
				TargetPort: intstr.FromString("a2a"),
				Protocol:   corev1.ProtocolTCP,
			})
		}

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
	isA2A := agentRuntime.Spec.Facade.Type == omniav1alpha1.FacadeTypeA2A || isDualProtocol(agentRuntime)
	if !isA2A {
		return
	}

	port := int32(DefaultFacadePort)
	if agentRuntime.Spec.Facade.Port != nil {
		port = *agentRuntime.Spec.Facade.Port
	}
	if isDualProtocol(agentRuntime) && agentRuntime.Spec.A2A != nil && agentRuntime.Spec.A2A.Port != nil {
		port = *agentRuntime.Spec.A2A.Port
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

// SetupWithManager sets up the controller with the Manager.
func (r *AgentRuntimeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		For(&omniav1alpha1.AgentRuntime{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		// Watch Provider changes and reconcile AgentRuntimes that reference them
		Watches(
			&omniav1alpha1.Provider{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForProvider),
		).
		// Watch PromptPack changes and reconcile AgentRuntimes that reference them
		Watches(
			&omniav1alpha1.PromptPack{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForPromptPack),
		).
		// Watch Secret changes and reconcile AgentRuntimes that use them for credentials
		// This triggers pod rollouts when API keys are rotated
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findAgentRuntimesForSecret),
		).
		Named("agentruntime").
		Complete(r)
}
