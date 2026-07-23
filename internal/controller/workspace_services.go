/*
Copyright 2026 Altaira Labs.

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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// setReconcileError sets a failure condition, marks the workspace as error, and updates status.
func (r *WorkspaceReconciler) setReconcileError(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	condType, reason string,
	err error,
	log logr.Logger,
) (ctrl.Result, error) {
	SetCondition(&workspace.Status.Conditions, workspace.Generation, condType, metav1.ConditionFalse,
		reason, err.Error())
	workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
	if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
		log.Error(statusErr, logMsgFailedToUpdateStatus)
	}
	return ctrl.Result{}, err
}

// setServicesReadyCondition updates the ServicesReady condition based on service group statuses.
func setServicesReadyCondition(conditions *[]metav1.Condition, generation int64, services []omniav1alpha1.ServiceGroupStatus) {
	if len(services) == 0 {
		return
	}
	for _, svc := range services {
		if !svc.Ready {
			SetCondition(conditions, generation, ConditionTypeServicesReady, metav1.ConditionFalse,
				"ServicesNotReady", "One or more service groups are not ready")
			return
		}
	}
	SetCondition(conditions, generation, ConditionTypeServicesReady, metav1.ConditionTrue,
		"ServicesReady", "All service groups are ready")
}

// reconcileServices ensures that each service group defined in the workspace
// spec has the correct Deployments, Services, and status entries.
func (r *WorkspaceReconciler) reconcileServices(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespace := workspace.Spec.Namespace.Name

	statuses := make([]omniav1alpha1.ServiceGroupStatus, 0, len(workspace.Spec.Services))

	for _, sg := range workspace.Spec.Services {
		status, err := r.reconcileServiceGroup(ctx, workspace, namespace, sg)
		if err != nil {
			return fmt.Errorf("service group %s: %w", sg.Name, err)
		}
		statuses = append(statuses, status)
	}

	workspace.Status.Services = statuses

	if err := r.cleanupRemovedServiceGroups(ctx, workspace, namespace); err != nil {
		log.Error(err, "service group cleanup failed")
		return err
	}

	if err := r.reconcilePrivacyService(ctx, workspace); err != nil {
		return fmt.Errorf("privacy service: %w", err)
	}

	return nil
}

// reconcilePrivacyService creates or updates the per-workspace privacy-api
// Deployment, Service, and ServiceAccount. It is called once per workspace
// reconciliation after the per-service-group loop. The privacy-api is
// per-workspace (not per-service-group), so it lives outside the group loop.
//
// Gate: when Spec.Privacy is nil, any existing privacy-<ws> resources are
// torn down (cleanupPrivacyService). When PrivacyImage is absent (operator
// misconfig), we skip without teardown — an operator upgrade should not
// destroy a running deployment. When both gates pass, resources are
// created/updated.
func (r *WorkspaceReconciler) reconcilePrivacyService(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	if workspace.Spec.Privacy == nil {
		if err := r.cleanupPrivacyService(ctx, workspace); err != nil {
			return err
		}
		workspace.Status.PrivacyURL = ""
		return nil
	}
	if r.ServiceBuilder == nil || r.ServiceBuilder.PrivacyImage == "" {
		workspace.Status.PrivacyURL = ""
		return nil
	}
	namespace := workspace.Spec.Namespace.Name
	depName := fmt.Sprintf("privacy-%s", workspace.Name)

	// privacy-api has no podOverrides path, so its effective SA is the default
	// per-deployment SA (depName).
	if err := r.reconcileServicePodSA(ctx, workspace, namespace, depName, depName); err != nil {
		return fmt.Errorf("service account %s: %w", depName, err)
	}
	// privacy-api validates caller tokens via the TokenReview API (same as
	// session-api), so it needs the tokenreview ClusterRole binding.
	if err := r.reconcileSessionAPITokenReviewBinding(ctx, namespace, depName); err != nil {
		return fmt.Errorf("privacy tokenreview binding: %w", err)
	}
	// privacy-api also runs the privacy watcher (own-namespace list;watch + global
	// default Get). It gets the namespaced reader Role + binding and the
	// default-policy ClusterRoleBinding, but NOT the memory-consolidation grant
	// (memorypolicies are memory-api's concern). No-op on OSS installs (#1899).
	if err := r.reconcilePrivacyReaderRole(ctx, workspace, namespace); err != nil {
		return fmt.Errorf("privacy reader role: %w", err)
	}
	if err := r.reconcilePrivacyReaderBinding(ctx, workspace, namespace, depName); err != nil {
		return fmt.Errorf("privacy reader binding: %w", err)
	}
	if err := r.reconcilePrivacyDefaultBinding(ctx, namespace, depName); err != nil {
		return fmt.Errorf("privacy default binding: %w", err)
	}
	if err := r.reconcileServiceAuthNetworkHardening(ctx, workspace, namespace); err != nil {
		return fmt.Errorf("privacy network hardening: %w", err)
	}

	dep := r.ServiceBuilder.BuildPrivacyDeployment(workspace.Name, namespace, *workspace.Spec.Privacy)
	if err := r.reconcileManagedDeployment(ctx, workspace, namespace, dep); err != nil {
		return fmt.Errorf("privacy deployment: %w", err)
	}

	svc := BuildService(depName, namespace, "privacy-api", workspace.Name, "")
	if err := r.reconcileManagedService(ctx, workspace, namespace, svc); err != nil {
		return fmt.Errorf("privacy service create: %w", err)
	}

	workspace.Status.PrivacyURL = ServiceURL(depName, namespace)
	return nil
}

// cleanupPrivacyService deletes all per-workspace privacy-<ws> resources when
// spec.privacy is removed from a live Workspace. The Deployment, Service, and
// ServiceAccount carry owner-references to the Workspace and would be GC'd on
// Workspace deletion, but they are NOT cleaned up when only spec.privacy is
// removed. The tokenreview + privacy-default ClusterRoleBindings are
// cluster-scoped and are never covered by owner-ref GC, so they must always be
// deleted explicitly here. The privacy pod's namespaced privacy-reader
// RoleBinding IS owner-ref-GC'd on Workspace deletion, but not on spec.privacy
// removal, so it too is deleted here. The shared namespaced privacy-reader Role
// is left in place — session-api/memory-api in the same namespace still use it.
// NotFound errors are ignored so the function is idempotent.
func (r *WorkspaceReconciler) cleanupPrivacyService(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	namespace := workspace.Spec.Namespace.Name
	depName := fmt.Sprintf("privacy-%s", workspace.Name)

	dep := &appsv1.Deployment{}
	dep.Name = depName
	dep.Namespace = namespace

	svc := &corev1.Service{}
	svc.Name = depName
	svc.Namespace = namespace

	sa := &corev1.ServiceAccount{}
	sa.Name = depName
	sa.Namespace = namespace

	trCRB := &rbacv1.ClusterRoleBinding{}
	trCRB.Name = fmt.Sprintf("session-tokenreview-%s-%s", namespace, depName)

	pdCRB := &rbacv1.ClusterRoleBinding{}
	pdCRB.Name = privacyDefaultBindingName(namespace, depName)

	prRB := &rbacv1.RoleBinding{}
	prRB.Name = privacyReaderBindingName(depName)
	prRB.Namespace = namespace

	for _, obj := range []client.Object{dep, svc, sa, trCRB, pdCRB, prRB} {
		if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("cleanup privacy resource %s: %w", obj.GetName(), err)
		}
	}
	return nil
}

// reconcileServiceGroup handles a single service group, returning its status.
func (r *WorkspaceReconciler) reconcileServiceGroup(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
	sg omniav1alpha1.WorkspaceServiceGroup,
) (omniav1alpha1.ServiceGroupStatus, error) {
	if sg.Mode == omniav1alpha1.ServiceModeExternal {
		return r.reconcileExternalServiceGroup(sg), nil
	}
	return r.reconcileManagedServiceGroup(ctx, workspace, namespace, sg)
}

// reconcileExternalServiceGroup returns a status for an externally-provided service group.
func (r *WorkspaceReconciler) reconcileExternalServiceGroup(sg omniav1alpha1.WorkspaceServiceGroup) omniav1alpha1.ServiceGroupStatus {
	return omniav1alpha1.ServiceGroupStatus{
		Name:       sg.Name,
		SessionURL: sg.External.SessionURL,
		MemoryURL:  sg.External.MemoryURL,
		Ready:      true,
	}
}

// reconcileManagedServiceGroup creates/updates Deployments and Services for a managed group.
func (r *WorkspaceReconciler) reconcileManagedServiceGroup(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
	sg omniav1alpha1.WorkspaceServiceGroup,
) (omniav1alpha1.ServiceGroupStatus, error) {
	sessionDepName := fmt.Sprintf("session-%s-%s", workspace.Name, sg.Name)
	memoryDepName := fmt.Sprintf("memory-%s-%s", workspace.Name, sg.Name)

	// The SA each pod actually runs as may be overridden via podOverrides (e.g.
	// memory-api on a Workload-Identity SA for keyless embeddings). RBAC bindings
	// below must target that effective SA, not the default per-deployment SA, or
	// the grant lands on an SA the pod never uses (#1817).
	var sessionOverrides, memoryOverrides *omniav1alpha1.PodOverrides
	if sg.Session != nil {
		sessionOverrides = sg.Session.PodOverrides
	}
	if sg.Memory != nil {
		memoryOverrides = sg.Memory.PodOverrides
	}
	sessionSA := podServiceAccountName(sessionDepName, sessionOverrides)
	memorySA := podServiceAccountName(memoryDepName, memoryOverrides)

	// Reconcile ServiceAccounts for service pods. Per-workspace session-api
	// and memory-api need to read the cluster-scoped Workspace CRD to
	// resolve their own config (workspace name, service group, DB secret).
	// The Workspace-reader + secrets bindings must target the EFFECTIVE SA the
	// pod actually runs as (override-aware), consistent with the tokenreview and
	// enterprise-reader bindings below — binding the default SA leaves an
	// override-SA pod (e.g. a Workload-Identity memory-api) without get on its
	// own Workspace (#1899).
	for _, svc := range []struct{ dep, sa string }{
		{sessionDepName, sessionSA},
		{memoryDepName, memorySA},
	} {
		if err := r.reconcileServicePodSA(ctx, workspace, namespace, svc.dep, svc.sa); err != nil {
			return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("service account %s: %w", svc.dep, err)
		}
	}

	// When internal service auth is enabled, session-api validates caller
	// tokens via the TokenReview API, which requires its ServiceAccount to be
	// able to create TokenReviews (cluster-scoped). Bind the session-api SA to
	// the install-wide tokenreview ClusterRole (provisioned by the chart).
	if err := r.reconcileSessionAPITokenReviewBinding(ctx, namespace, sessionSA); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("session tokenreview binding: %w", err)
	}
	if err := r.reconcileSessionAPITokenReviewBinding(ctx, namespace, memorySA); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("memory tokenreview binding: %w", err)
	}

	// On enterprise builds, BOTH memory-api and session-api run a privacy-policy
	// watcher that lists SessionPrivacyPolicy + AgentRuntime CRDs in their own
	// namespace and Gets the global default policy at omnia-system/default. Grant
	// those via a namespaced Role (list;watch) + a cluster-wide default-policy
	// reader (get default only). memory-api additionally lists memorypolicies
	// cluster-wide for consolidation. No-op on OSS installs (#1899, #1444/#1567).
	if err := r.reconcileServiceGroupPrivacyReaders(ctx, workspace, namespace, sessionSA, memorySA); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, err
	}

	// Internal-service-auth network hardening: default-deny ingress +
	// allow-from-known-callers NetworkPolicy for session-api/memory-api, and
	// (when Istio mTLS is requested) a STRICT PeerAuthentication. No-op when
	// auth is disabled.
	if err := r.reconcileServiceAuthNetworkHardening(ctx, workspace, namespace); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("service auth network hardening: %w", err)
	}

	// Reconcile session-api deployment and service
	sessionDep := r.ServiceBuilder.BuildSessionDeployment(workspace.Name, namespace, sg)
	if err := r.reconcileManagedDeployment(ctx, workspace, namespace, sessionDep); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("session deployment: %w", err)
	}

	sessionSvc := BuildService(sessionDepName, namespace, "session-api", workspace.Name, sg.Name)
	if err := r.reconcileManagedService(ctx, workspace, namespace, sessionSvc); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("session service: %w", err)
	}

	// Reconcile memory-api deployment and service
	memoryDep := r.ServiceBuilder.BuildMemoryDeployment(workspace.Name, namespace, sg)
	if err := r.reconcileManagedDeployment(ctx, workspace, namespace, memoryDep); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("memory deployment: %w", err)
	}

	memorySvc := BuildService(memoryDepName, namespace, "memory-api", workspace.Name, sg.Name)
	if err := r.reconcileManagedService(ctx, workspace, namespace, memorySvc); err != nil {
		return omniav1alpha1.ServiceGroupStatus{}, fmt.Errorf("memory service: %w", err)
	}

	// Check readiness
	sessionReady := r.isDeploymentReady(ctx, sessionDepName, namespace)
	memoryReady := r.isDeploymentReady(ctx, memoryDepName, namespace)

	return omniav1alpha1.ServiceGroupStatus{
		Name:       sg.Name,
		SessionURL: ServiceURL(sessionDepName, namespace),
		MemoryURL:  ServiceURL(memoryDepName, namespace),
		Ready:      sessionReady && memoryReady,
	}, nil
}

// reconcileManagedDeployment creates or updates a Deployment for a managed service group.
func (r *WorkspaceReconciler) reconcileManagedDeployment(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
	desired *appsv1.Deployment,
) error {
	existing := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)

	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, desired, r.Scheme); err != nil {
			return fmt.Errorf("set controller reference: %w", err)
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get deployment %s/%s: %w", namespace, desired.Name, err)
	}

	// Update existing deployment
	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

// reconcileManagedService creates or updates a Service for a managed service group.
func (r *WorkspaceReconciler) reconcileManagedService(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
	desired *corev1.Service,
) error {
	existing := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), existing)

	if apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, desired, r.Scheme); err != nil {
			return fmt.Errorf("set controller reference: %w", err)
		}
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("get service %s/%s: %w", namespace, desired.Name, err)
	}

	// Update existing service
	existing.Spec.Selector = desired.Spec.Selector
	existing.Spec.Ports = desired.Spec.Ports
	existing.Labels = desired.Labels
	return r.Update(ctx, existing)
}

// isDeploymentReady returns true if the named deployment has at least one ready replica.
func (r *WorkspaceReconciler) isDeploymentReady(ctx context.Context, name, namespace string) bool {
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, dep); err != nil {
		return false
	}
	return dep.Status.ReadyReplicas > 0
}

// reconcileServicePodSA ensures a ServiceAccount, ClusterRoleBinding, and
// namespaced RoleBinding exist for a per-workspace service pod. The service pods
// (session-api, memory-api) need to Get the cluster-scoped Workspace CRD to
// resolve their own config and read the DB secret in their namespace.
//
// name is the default per-deployment SA name (also the ServiceAccount object it
// creates — the fallback SA for the non-override case). saName is the EFFECTIVE
// SA the pod actually runs as (override-aware, from podServiceAccountName): both
// bindings' subjects target saName, not name — so an override-SA pod (e.g. a
// Workload-Identity memory-api) still receives get on its own Workspace and read
// on its DB secret, consistent with the tokenreview/enterprise-reader bindings
// (#1899, #1817).
//
// The ClusterRoleBinding binds the get-only per-workspace Workspace reader
// ClusterRole (resourceNames-scoped to this workspace), NOT the cluster-wide
// agent-workspace-reader — a pod in one workspace must not be able to enumerate
// the config of others (#1899). Gated on WorkspaceReaderRBACEnabled so
// local-dev / envtest without RBAC provisioned still reconcile.
func (r *WorkspaceReconciler) reconcileServicePodSA(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace, name, saName string,
) error {
	// ServiceAccount — always the default per-deployment SA. Harmless (unused)
	// when the pod runs as an override SA; the actual grant subjects below use
	// saName.
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
	existing := &corev1.ServiceAccount{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(sa), existing); apierrors.IsNotFound(err) {
		if err := controllerutil.SetControllerReference(workspace, sa, r.Scheme); err != nil {
			return err
		}
		if err := r.Create(ctx, sa); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if err := r.reconcileServicePodReaderBinding(ctx, workspace, namespace, name, saName); err != nil {
		return err
	}
	return r.reconcileServicePodSecretsBinding(ctx, workspace, namespace, name, saName)
}

// reconcileServicePodReaderBinding binds the effective service-pod SA to the
// get-only per-workspace Workspace reader ClusterRole (resourceNames-scoped to
// this workspace), NOT a cluster-wide role. roleRef is immutable, so an upgrade
// from the old cluster-wide binding must delete + recreate; the SUBJECT is not
// immutable but a plain create-if-NotFound never repoints it, so CreateOrUpdate
// reconciles it when the override SA changes (mirrors the facade path). Skipped
// when RBAC isn't provisioned (local-dev / envtest), mirroring the agent path.
func (r *WorkspaceReconciler) reconcileServicePodReaderBinding(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace, name, saName string,
) error {
	if !r.WorkspaceReaderRBACEnabled {
		return nil
	}
	crbName := fmt.Sprintf("service-%s-%s", namespace, name)
	roleName := WorkspaceReaderClusterRoleName(workspace.Name)
	if err := deleteStaleRoleRefBinding(ctx, r.Client, crbName, roleName); err != nil {
		return err
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: crbName},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, crb, func() error {
		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleName,
		}
		crb.Subjects = []rbacv1.Subject{{
			Kind:      kindServiceAccount,
			Name:      saName,
			Namespace: namespace,
		}}
		return nil
	})
	return err
}

// reconcileServicePodSecretsBinding binds the effective service-pod SA to the
// namespaced editor ClusterRole via a RoleBinding, granting the pod access to
// secrets in its namespace (needed to read database connection strings from the
// secretRef). The subject is the effective SA (saName), reconciled via
// CreateOrUpdate so an override-SA change is repointed.
func (r *WorkspaceReconciler) reconcileServicePodSecretsBinding(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace, name, saName string,
) error {
	rbName := fmt.Sprintf("service-%s-secrets", name)
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: rbName, Namespace: namespace},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		if err := controllerutil.SetControllerReference(workspace, rb, r.Scheme); err != nil {
			return err
		}
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     clusterRoleEditor,
		}
		rb.Subjects = []rbacv1.Subject{{
			Kind:      kindServiceAccount,
			Name:      saName,
			Namespace: namespace,
		}}
		return nil
	})
	return err
}

// reconcileSessionAPITokenReviewBinding binds the per-workspace session-api
// ServiceAccount to the install-wide tokenreview ClusterRole when internal
// service auth is enabled. session-api validates caller bearer tokens via the
// TokenReview API (authentication.k8s.io/tokenreviews: create), which is a
// cluster-scoped resource, so the grant must be a ClusterRole + ClusterRole
// Binding. The ClusterRole itself is provisioned by the Helm chart; here we
// only create the binding for this workspace's session-api SA. No-op when auth
// is disabled or the ClusterRole name is unset.
func (r *WorkspaceReconciler) reconcileSessionAPITokenReviewBinding(
	ctx context.Context,
	namespace, sessionSAName string,
) error {
	if r.ServiceBuilder == nil || !r.ServiceBuilder.ServiceAuth.Enabled {
		return nil
	}
	if r.SessionAPITokenReviewClusterRole == "" {
		return nil
	}

	crbName := fmt.Sprintf("session-tokenreview-%s-%s", namespace, sessionSAName)
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: crbName},
		Subjects: []rbacv1.Subject{{
			Kind:      kindServiceAccount,
			Name:      sessionSAName,
			Namespace: namespace,
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     r.SessionAPITokenReviewClusterRole,
		},
	}
	existing := &rbacv1.ClusterRoleBinding{}
	if err := r.Get(ctx, client.ObjectKeyFromObject(crb), existing); apierrors.IsNotFound(err) {
		return r.Create(ctx, crb)
	} else if err != nil {
		return err
	}
	return nil
}

// Resources the privacy watcher lists;watches in its own namespace. Extracted
// so the Role rule can't drift from the watcher's expectations.
const (
	resSessionPrivacyPolicies = "sessionprivacypolicies"
	resAgentRuntimes          = "agentruntimes"
)

// privacyReaderRoleName is the namespaced Role granting list;watch on
// sessionprivacypolicies + agentruntimes in a workspace's namespace. One per
// namespace, shared by every service pod (session-api, memory-api, privacy-api).
func privacyReaderRoleName(namespace string) string {
	return fmt.Sprintf("service-%s-privacy-reader", namespace)
}

// privacyReaderBindingName is the per-SA namespaced RoleBinding to the
// namespaced privacy-reader Role.
func privacyReaderBindingName(saName string) string {
	return fmt.Sprintf("service-%s-privacy-reader", saName)
}

// privacyDefaultBindingName is the per-SA ClusterRoleBinding to the
// privacy-default-reader ClusterRole (get on the global default policy).
func privacyDefaultBindingName(namespace, saName string) string {
	return fmt.Sprintf("privacy-default-%s-%s", namespace, saName)
}

// memoryConsolidationBindingName is the memory-api-only ClusterRoleBinding to
// the memory-consolidation-reader ClusterRole (cluster-wide memorypolicies).
func memoryConsolidationBindingName(namespace, saName string) string {
	return fmt.Sprintf("memory-consolidation-%s-%s", namespace, saName)
}

// reconcilePrivacyReaderRole ensures the namespaced Role granting list;watch on
// sessionprivacypolicies + agentruntimes in a workspace's namespace. Every
// service pod (session-api, memory-api, privacy-api) runs a privacy watcher that
// lists those CRDs in its OWN namespace (ee/pkg/privacy/watcher.go, #1899), so
// the grant is namespaced — a pod in one workspace can no longer enumerate the
// policies/agents of others. The Role is owner-referenced to the (cluster-scoped)
// Workspace so it is garbage-collected with it; idempotent CreateOrUpdate lets
// every service group in the namespace re-ensure the single shared Role.
func (r *WorkspaceReconciler) reconcilePrivacyReaderRole(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: privacyReaderRoleName(namespace), Namespace: namespace},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		if err := controllerutil.SetControllerReference(workspace, role, r.Scheme); err != nil {
			return err
		}
		role.Rules = []rbacv1.PolicyRule{{
			APIGroups: []string{omniaAPIGroup},
			Resources: []string{resSessionPrivacyPolicies, resAgentRuntimes},
			Verbs:     []string{verbList, verbWatch},
		}}
		return nil
	})
	return err
}

// reconcilePrivacyReaderBinding binds a service pod's effective SA to the
// namespaced privacy-reader Role. The subject is reconciled via CreateOrUpdate
// so an override-SA change is repointed; the RoleBinding is owner-referenced to
// the Workspace and GC'd with it. roleRef never changes (always the same
// namespaced Role), so no stale-ref deletion is needed.
func (r *WorkspaceReconciler) reconcilePrivacyReaderBinding(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace, saName string,
) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: privacyReaderBindingName(saName), Namespace: namespace},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
		if err := controllerutil.SetControllerReference(workspace, rb, r.Scheme); err != nil {
			return err
		}
		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     kindRole,
			Name:     privacyReaderRoleName(namespace),
		}
		rb.Subjects = []rbacv1.Subject{{
			Kind:      kindServiceAccount,
			Name:      saName,
			Namespace: namespace,
		}}
		return nil
	})
	return err
}

// reconcilePrivacyDefaultBinding binds a service pod's effective SA to the
// install-wide privacy-default-reader ClusterRole, which grants get on any
// "default"-named SessionPrivacyPolicy cluster-wide. The watcher Gets the global
// default policy at omnia-system/default (ee/pkg/privacy/watcher.go, #1899); this
// mild, documented over-grant satisfies that single cross-namespace read without
// a per-pod omnia-system RoleBinding (per-workspace policies are not named
// "default", so it leaks nothing). roleRef is immutable, so an upgrade from an
// old cluster-wide binding must delete + recreate. No-op when the ClusterRole
// name is unset (OSS / kustomize).
func (r *WorkspaceReconciler) reconcilePrivacyDefaultBinding(
	ctx context.Context,
	namespace, saName string,
) error {
	if r.PrivacyDefaultReaderClusterRole == "" {
		return nil
	}
	crbName := privacyDefaultBindingName(namespace, saName)
	if err := deleteStaleRoleRefBinding(ctx, r.Client, crbName, r.PrivacyDefaultReaderClusterRole); err != nil {
		return err
	}
	crb := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: crbName}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, crb, func() error {
		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     kindClusterRole,
			Name:     r.PrivacyDefaultReaderClusterRole,
		}
		crb.Subjects = []rbacv1.Subject{{
			Kind:      kindServiceAccount,
			Name:      saName,
			Namespace: namespace,
		}}
		return nil
	})
	return err
}

// reconcileMemoryConsolidationBinding binds ONLY the memory-api SA to the
// install-wide memory-consolidation-reader ClusterRole, granting cluster-wide
// get;list;watch on memorypolicies. The memory-api consolidation lister
// genuinely enumerates MemoryPolicy CRDs across workspaces, so this grant stays
// cluster-wide — but scoped to memory-api alone (session-api / privacy-api never
// bind it). roleRef is immutable → delete-stale + CreateOrUpdate. No-op when the
// ClusterRole name is unset (OSS / kustomize).
func (r *WorkspaceReconciler) reconcileMemoryConsolidationBinding(
	ctx context.Context,
	namespace, memorySAName string,
) error {
	if r.MemoryConsolidationReaderClusterRole == "" {
		return nil
	}
	crbName := memoryConsolidationBindingName(namespace, memorySAName)
	if err := deleteStaleRoleRefBinding(ctx, r.Client, crbName, r.MemoryConsolidationReaderClusterRole); err != nil {
		return err
	}
	crb := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: crbName}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, crb, func() error {
		crb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     kindClusterRole,
			Name:     r.MemoryConsolidationReaderClusterRole,
		}
		crb.Subjects = []rbacv1.Subject{{
			Kind:      kindServiceAccount,
			Name:      memorySAName,
			Namespace: namespace,
		}}
		return nil
	})
	return err
}

// reconcileServiceGroupPrivacyReaders wires the privacy/memory reader grants for
// one managed service group: the shared namespaced Role, then per-SA bindings.
// Both session-api and memory-api get the namespaced reader + the default-policy
// ClusterRoleBinding; ONLY memory-api additionally gets the cluster-wide
// memorypolicies consolidation binding.
func (r *WorkspaceReconciler) reconcileServiceGroupPrivacyReaders(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace, sessionSA, memorySA string,
) error {
	if err := r.reconcilePrivacyReaderRole(ctx, workspace, namespace); err != nil {
		return fmt.Errorf("privacy reader role: %w", err)
	}
	if err := r.reconcilePrivacyReaderBinding(ctx, workspace, namespace, memorySA); err != nil {
		return fmt.Errorf("memory privacy reader binding: %w", err)
	}
	if err := r.reconcilePrivacyDefaultBinding(ctx, namespace, memorySA); err != nil {
		return fmt.Errorf("memory privacy default binding: %w", err)
	}
	if err := r.reconcileMemoryConsolidationBinding(ctx, namespace, memorySA); err != nil {
		return fmt.Errorf("memory consolidation binding: %w", err)
	}
	if err := r.reconcilePrivacyReaderBinding(ctx, workspace, namespace, sessionSA); err != nil {
		return fmt.Errorf("session privacy reader binding: %w", err)
	}
	if err := r.reconcilePrivacyDefaultBinding(ctx, namespace, sessionSA); err != nil {
		return fmt.Errorf("session privacy default binding: %w", err)
	}
	return nil
}

// cleanupRemovedServiceGroups deletes Deployments and Services for service groups
// that are no longer present in the workspace spec.
func (r *WorkspaceReconciler) cleanupRemovedServiceGroups(
	ctx context.Context,
	workspace *omniav1alpha1.Workspace,
	namespace string,
) error {
	// Build set of expected managed group names
	expected := make(map[string]bool)
	for _, sg := range workspace.Spec.Services {
		if sg.Mode != omniav1alpha1.ServiceModeExternal {
			expected[sg.Name] = true
		}
	}

	// Clean up deployments
	if err := r.cleanupOrphanedDeployments(ctx, workspace.Name, namespace, expected); err != nil {
		return err
	}

	// Clean up services
	return r.cleanupOrphanedServices(ctx, workspace.Name, namespace, expected)
}

// cleanupOrphanedDeployments deletes Deployments whose service group is not in the expected set.
func (r *WorkspaceReconciler) cleanupOrphanedDeployments(
	ctx context.Context,
	workspaceName, namespace string,
	expected map[string]bool,
) error {
	depList := &appsv1.DeploymentList{}
	if err := r.List(ctx, depList,
		client.InNamespace(namespace),
		client.MatchingLabels{labelAppManagedBy: labelValueOmniaOperator, labelWorkspace: workspaceName},
	); err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	for i := range depList.Items {
		groupName := depList.Items[i].Labels[labelServiceGroup]
		if groupName != "" && !expected[groupName] {
			if err := r.Delete(ctx, &depList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete deployment %s: %w", depList.Items[i].Name, err)
			}
		}
	}
	return nil
}

// cleanupOrphanedServices deletes Services whose service group is not in the expected set.
func (r *WorkspaceReconciler) cleanupOrphanedServices(
	ctx context.Context,
	workspaceName, namespace string,
	expected map[string]bool,
) error {
	svcList := &corev1.ServiceList{}
	if err := r.List(ctx, svcList,
		client.InNamespace(namespace),
		client.MatchingLabels{labelAppManagedBy: labelValueOmniaOperator, labelWorkspace: workspaceName},
	); err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	for i := range svcList.Items {
		groupName := svcList.Items[i].Labels[labelServiceGroup]
		if groupName != "" && !expected[groupName] {
			if err := r.Delete(ctx, &svcList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete service %s: %w", svcList.Items[i].Name, err)
			}
		}
	}
	return nil
}
