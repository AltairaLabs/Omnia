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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Workspace-specific constants
const (
	// WorkspaceFinalizerName is the finalizer for Workspace resources.
	WorkspaceFinalizerName = "workspace.omnia.altairalabs.ai/finalizer"

	// Workspace label constants
	labelWorkspace        = "omnia.altairalabs.ai/workspace"
	labelWorkspaceManaged = "omnia.altairalabs.ai/managed"
	labelWorkspaceRole    = "omnia.altairalabs.ai/role"
	labelEnvironment      = "omnia.altairalabs.ai/environment"
	labelValueTrue        = "true"

	// ClusterRole names for workspace roles
	clusterRoleOwner  = "omnia-workspace-owner"
	clusterRoleEditor = "omnia-workspace-editor"
	clusterRoleViewer = "omnia-workspace-viewer"
)

// Condition types for Workspace
const (
	ConditionTypeWorkspaceReady        = "Ready"
	ConditionTypeNamespaceReady        = "NamespaceReady"
	ConditionTypeServiceAccountsReady  = "ServiceAccountsReady"
	ConditionTypeRoleBindingsReady     = "RoleBindingsReady"
	ConditionTypeNetworkPolicyReady    = "NetworkPolicyReady"
	ConditionTypeStorageReady          = "StorageReady"
	ConditionTypeServicesReady         = "ServicesReady"
	ConditionTypePrivacyPolicyResolved = "PrivacyPolicyResolved"
)

// Network policy constants
const (
	labelSharedNamespace = "omnia.altairalabs.ai/shared"
	// labelK8sMetadataName is the Kubernetes-managed namespace label that
	// `kubernetes.io/metadata.name` selectors match against. Duplicated 3+
	// times in NetworkPolicy peers; extracted for S1192.
	labelK8sMetadataName = "kubernetes.io/metadata.name"
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// DefaultStorageClass is the default storage class for workspace PVCs
	// when not specified in the Workspace spec. Used for NFS-backed storage.
	DefaultStorageClass string

	// ServiceBuilder builds Deployment and Service objects for per-workspace
	// session-api and memory-api instances.
	ServiceBuilder *ServiceBuilder

	// WorkspaceReaderRBACEnabled gates creation of the per-workspace service-pod
	// Workspace-reader ClusterRoleBinding. Per-workspace service pods (session-api,
	// memory-api) bind the get-only per-workspace reader ClusterRole to resolve
	// their config from their own Workspace CRD. False in local-dev / tests where
	// RBAC is not provisioned, so no binding is created.
	WorkspaceReaderRBACEnabled bool

	// OperatorNamespace is the namespace where the operator + dashboard run
	// (typically "omnia-system"). When a Workspace enables network isolation,
	// the generated NetworkPolicy auto-allows traffic to/from this namespace
	// so the dashboard, operator, and Prometheus scrape can reach workspace
	// pods without the user having to label namespaces. Populated from
	// POD_NAMESPACE at operator startup; empty string disables the auto-
	// allow (useful in tests).
	OperatorNamespace string

	// SessionAPITokenReviewClusterRole is the install-wide ClusterRole the
	// chart provisions to grant `authentication.k8s.io/tokenreviews: create`.
	// When internal service auth is enabled, the reconciler binds each
	// per-workspace session-api ServiceAccount to it so session-api can
	// validate caller tokens. Empty disables the binding.
	SessionAPITokenReviewClusterRole string

	// MemoryEnterpriseReaderClusterRole is the install-wide ClusterRole the
	// chart provisions (enterprise builds only) to grant cluster reads on
	// sessionprivacypolicies + agentruntimes. When set, the reconciler binds
	// each per-workspace memory-api ServiceAccount to it so the enterprise
	// memory-policy/privacy watcher can list those CRDs (#1444). Empty
	// disables the binding (OSS installs).
	MemoryEnterpriseReaderClusterRole string
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts/token,verbs=create
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// clusterroles: the per-workspace reader granting agents get on their own
// Workspace (#1875). Markers on individual reconcile helpers are not picked up
// by controller-gen, so it lives in this canonical block.
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.istio.io,resources=peerauthentications,verbs=get;list;watch;create;update;patch;delete
// These permissions are required for workspace RoleBinding creation (RBAC escalation prevention)
// The controller must have all permissions it grants via workspace ClusterRoles
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=pods;pods/log,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentpolicies;agentruntimes;promptpacks;toolpolicies;toolregistries;providers;arenasources;arenajobs;arenatemplatesources;arenadevsessions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentpolicies/status;arenasources/status;arenajobs/status;arenatemplatesources/status;arenadevsessions/status;toolpolicies/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
//nolint:gocognit // Reconcile functions inherently have high complexity due to state machine logic
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Workspace instance
	workspace := &omniav1alpha1.Workspace{}
	if err := r.Get(ctx, req.NamespacedName, workspace); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Workspace resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Workspace")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !workspace.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, workspace)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(workspace, WorkspaceFinalizerName) {
		controllerutil.AddFinalizer(workspace, WorkspaceFinalizerName)
		if err := r.Update(ctx, workspace); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Second}, nil
	}

	// Initialize status if needed
	if workspace.Status.Phase == "" {
		workspace.Status.Phase = omniav1alpha1.WorkspacePhasePending
		if err := r.Status().Update(ctx, workspace); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile namespace
	if err := r.reconcileNamespace(ctx, workspace); err != nil {
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeNamespaceReady, metav1.ConditionFalse,
			namespaceConditionReason(err), err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeNamespaceReady, metav1.ConditionTrue,
		"NamespaceReady", "Namespace is ready")

	// Reconcile ServiceAccounts
	if err := r.reconcileServiceAccounts(ctx, workspace); err != nil {
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeServiceAccountsReady, metav1.ConditionFalse,
			"ServiceAccountsFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeServiceAccountsReady, metav1.ConditionTrue,
		"ServiceAccountsReady", "ServiceAccounts are ready")

	// Reconcile RoleBindings for ServiceAccounts
	if err := r.reconcileRoleBindings(ctx, workspace); err != nil {
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeRoleBindingsReady, metav1.ConditionFalse,
			"RoleBindingsFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeRoleBindingsReady, metav1.ConditionTrue,
		"RoleBindingsReady", "RoleBindings are ready")

	// Reconcile NetworkPolicy
	if err := r.reconcileNetworkPolicy(ctx, workspace); err != nil {
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeNetworkPolicyReady, metav1.ConditionFalse,
			"NetworkPolicyFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeNetworkPolicyReady, metav1.ConditionTrue,
		"NetworkPolicyReady", "NetworkPolicy is ready")

	// Reconcile Storage (PVC)
	if err := r.reconcileStorage(ctx, workspace); err != nil {
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeStorageReady, metav1.ConditionFalse,
			"StorageFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}

	// Check if storage is provisioning (PVC exists but not yet bound)
	storageProvisioning := false
	if workspace.Status.Storage != nil && workspace.Status.Storage.Phase != "" {
		if workspace.Status.Storage.Phase != string(corev1.ClaimBound) {
			storageProvisioning = true
			SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeStorageReady, metav1.ConditionFalse,
				"StorageProvisioning", fmt.Sprintf("PVC %s is %s, waiting for volume to be provisioned",
					workspace.Status.Storage.PVCName, workspace.Status.Storage.Phase))
		} else {
			SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeStorageReady, metav1.ConditionTrue,
				"StorageReady", "Storage is ready")
		}
	} else {
		// Storage not enabled or not configured
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeStorageReady, metav1.ConditionTrue,
			"StorageNotRequired", "Storage is not enabled for this workspace")
	}

	// Reconcile service instances (session-api, memory-api per workspace)
	if err := r.reconcileServices(ctx, workspace); err != nil {
		return r.setReconcileError(ctx, workspace, ConditionTypeServicesReady, "ServicesFailed", err, log)
	}
	setServicesReadyCondition(&workspace.Status.Conditions, workspace.Generation, workspace.Status.Services)

	// Update member count
	r.updateMemberCount(workspace)

	// Validate privacyPolicyRef across all service groups (non-blocking)
	privacyCond := r.validatePrivacyPolicyRefs(ctx, workspace)
	SetCondition(&workspace.Status.Conditions, workspace.Generation,
		privacyCond.Type, privacyCond.Status, privacyCond.Reason, privacyCond.Message)

	// Set overall Ready condition based on all components
	if storageProvisioning {
		workspace.Status.Phase = omniav1alpha1.WorkspacePhasePending
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeWorkspaceReady, metav1.ConditionFalse,
			"StorageProvisioning", "Waiting for storage to be provisioned")
	} else {
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseReady
		SetCondition(&workspace.Status.Conditions, workspace.Generation, ConditionTypeWorkspaceReady, metav1.ConditionTrue,
			"WorkspaceReady", "Workspace is ready")
	}

	workspace.Status.ObservedGeneration = workspace.Generation
	if err := r.Status().Update(ctx, workspace); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue if storage is still provisioning to check again
	if storageProvisioning {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		For(&omniav1alpha1.Workspace{}).
		// Watch Deployments owned by this controller so service readiness
		// changes (pods becoming ready/unready) trigger a workspace reconcile.
		Owns(&appsv1.Deployment{}).
		// Watch PVCs with the workspace label to trigger reconciliation when PVC phase changes
		Watches(
			&corev1.PersistentVolumeClaim{},
			handler.EnqueueRequestsFromMapFunc(r.mapPVCToWorkspace),
		).
		Named("workspace").
		Complete(r)
}

// mapPVCToWorkspace maps a PVC event to a Workspace reconciliation request
// if the PVC has the workspace label.
func (r *WorkspaceReconciler) mapPVCToWorkspace(_ context.Context, obj client.Object) []reconcile.Request {
	pvc, ok := obj.(*corev1.PersistentVolumeClaim)
	if !ok {
		return nil
	}

	// Check if this PVC is managed by workspace controller
	workspaceName := pvc.Labels[labelWorkspace]
	if workspaceName == "" {
		return nil
	}

	// Workspace is cluster-scoped, so we only need the name
	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{Name: workspaceName}},
	}
}
