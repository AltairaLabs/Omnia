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

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

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

	// ClusterRole names for workspace roles
	clusterRoleOwner  = "omnia-workspace-owner"
	clusterRoleEditor = "omnia-workspace-editor"
	clusterRoleViewer = "omnia-workspace-viewer"
)

// Condition types for Workspace
const (
	ConditionTypeWorkspaceReady       = "Ready"
	ConditionTypeNamespaceReady       = "NamespaceReady"
	ConditionTypeServiceAccountsReady = "ServiceAccountsReady"
	ConditionTypeRoleBindingsReady    = "RoleBindingsReady"
	ConditionTypeNetworkPolicyReady   = "NetworkPolicyReady"
)

// Network policy constants
const (
	labelSharedNamespace = "omnia.altairalabs.ai/shared"
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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
		r.setCondition(workspace, ConditionTypeNamespaceReady, metav1.ConditionFalse,
			"NamespaceFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	r.setCondition(workspace, ConditionTypeNamespaceReady, metav1.ConditionTrue,
		"NamespaceReady", "Namespace is ready")

	// Reconcile ServiceAccounts
	if err := r.reconcileServiceAccounts(ctx, workspace); err != nil {
		r.setCondition(workspace, ConditionTypeServiceAccountsReady, metav1.ConditionFalse,
			"ServiceAccountsFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	r.setCondition(workspace, ConditionTypeServiceAccountsReady, metav1.ConditionTrue,
		"ServiceAccountsReady", "ServiceAccounts are ready")

	// Reconcile RoleBindings for ServiceAccounts
	if err := r.reconcileRoleBindings(ctx, workspace); err != nil {
		r.setCondition(workspace, ConditionTypeRoleBindingsReady, metav1.ConditionFalse,
			"RoleBindingsFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	r.setCondition(workspace, ConditionTypeRoleBindingsReady, metav1.ConditionTrue,
		"RoleBindingsReady", "RoleBindings are ready")

	// Reconcile NetworkPolicy
	if err := r.reconcileNetworkPolicy(ctx, workspace); err != nil {
		r.setCondition(workspace, ConditionTypeNetworkPolicyReady, metav1.ConditionFalse,
			"NetworkPolicyFailed", err.Error())
		workspace.Status.Phase = omniav1alpha1.WorkspacePhaseError
		if statusErr := r.Status().Update(ctx, workspace); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	r.setCondition(workspace, ConditionTypeNetworkPolicyReady, metav1.ConditionTrue,
		"NetworkPolicyReady", "NetworkPolicy is ready")

	// Update member count
	r.updateMemberCount(workspace)

	// Set overall Ready condition
	workspace.Status.Phase = omniav1alpha1.WorkspacePhaseReady
	r.setCondition(workspace, ConditionTypeWorkspaceReady, metav1.ConditionTrue,
		"WorkspaceReady", "Workspace is ready")

	workspace.Status.ObservedGeneration = workspace.Generation
	if err := r.Status().Update(ctx, workspace); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) reconcileDelete(ctx context.Context, workspace *omniav1alpha1.Workspace) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Handling deletion of Workspace")

	namespaceName := workspace.Spec.Namespace.Name

	// Clean up NetworkPolicies in the namespace
	networkPolicies := &networkingv1.NetworkPolicyList{}
	if err := r.List(ctx, networkPolicies, client.InNamespace(namespaceName),
		client.MatchingLabels{labelWorkspace: workspace.Name, labelWorkspaceManaged: "true"}); err == nil {
		for i := range networkPolicies.Items {
			if err := r.Delete(ctx, &networkPolicies.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete NetworkPolicy", "name", networkPolicies.Items[i].Name)
			}
		}
	}

	// Clean up RoleBindings in the namespace
	roleBindings := &rbacv1.RoleBindingList{}
	if err := r.List(ctx, roleBindings, client.InNamespace(namespaceName),
		client.MatchingLabels{labelWorkspace: workspace.Name, labelWorkspaceManaged: "true"}); err == nil {
		for i := range roleBindings.Items {
			if err := r.Delete(ctx, &roleBindings.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete RoleBinding", "name", roleBindings.Items[i].Name)
			}
		}
	}

	// Clean up ServiceAccounts in the namespace
	serviceAccounts := &corev1.ServiceAccountList{}
	if err := r.List(ctx, serviceAccounts, client.InNamespace(namespaceName),
		client.MatchingLabels{labelWorkspace: workspace.Name, labelWorkspaceManaged: "true"}); err == nil {
		for i := range serviceAccounts.Items {
			if err := r.Delete(ctx, &serviceAccounts.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete ServiceAccount", "name", serviceAccounts.Items[i].Name)
			}
		}
	}

	// Only delete namespace if we created it
	if workspace.Status.Namespace != nil && workspace.Status.Namespace.Created {
		ns := &corev1.Namespace{}
		if err := r.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err == nil {
			// Check if namespace has our label
			if ns.Labels[labelWorkspace] == workspace.Name {
				if err := r.Delete(ctx, ns); err != nil && !apierrors.IsNotFound(err) {
					log.Error(err, "Failed to delete namespace", "name", namespaceName)
				}
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(workspace, WorkspaceFinalizerName)
	if err := r.Update(ctx, workspace); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *WorkspaceReconciler) reconcileNamespace(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespaceName := workspace.Spec.Namespace.Name

	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: namespaceName}, ns)

	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get namespace %s: %w", namespaceName, err)
	}

	namespaceExists := err == nil

	if !namespaceExists {
		if !workspace.Spec.Namespace.Create {
			return fmt.Errorf("namespace %s does not exist and spec.namespace.create is false", namespaceName)
		}

		// Create the namespace
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceName,
				Labels: map[string]string{
					labelWorkspace:        workspace.Name,
					labelWorkspaceManaged: "true",
					labelEnvironment:      string(workspace.Spec.Environment),
				},
			},
		}

		// Add spec labels
		for k, v := range workspace.Spec.Namespace.Labels {
			ns.Labels[k] = v
		}

		// Add default tags as labels
		for k, v := range workspace.Spec.DefaultTags {
			ns.Labels[k] = v
		}

		// Add spec annotations
		ns.Annotations = workspace.Spec.Namespace.Annotations

		if err := r.Create(ctx, ns); err != nil {
			return fmt.Errorf("failed to create namespace %s: %w", namespaceName, err)
		}

		log.Info("Created namespace", "name", namespaceName)

		// Update status to indicate we created the namespace
		workspace.Status.Namespace = &omniav1alpha1.NamespaceStatus{
			Name:    namespaceName,
			Created: true,
		}
	} else {
		// Namespace exists, update labels if needed
		updated := false
		if ns.Labels == nil {
			ns.Labels = make(map[string]string)
		}

		// Ensure workspace labels are present
		if ns.Labels[labelWorkspace] != workspace.Name {
			ns.Labels[labelWorkspace] = workspace.Name
			updated = true
		}
		if ns.Labels[labelEnvironment] != string(workspace.Spec.Environment) {
			ns.Labels[labelEnvironment] = string(workspace.Spec.Environment)
			updated = true
		}

		// Add default tags
		for k, v := range workspace.Spec.DefaultTags {
			if ns.Labels[k] != v {
				ns.Labels[k] = v
				updated = true
			}
		}

		if updated {
			if err := r.Update(ctx, ns); err != nil {
				return fmt.Errorf("failed to update namespace %s: %w", namespaceName, err)
			}
			log.Info("Updated namespace labels", "name", namespaceName)
		}

		workspace.Status.Namespace = &omniav1alpha1.NamespaceStatus{
			Name:    namespaceName,
			Created: false,
		}
	}

	return nil
}

func (r *WorkspaceReconciler) reconcileServiceAccounts(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespaceName := workspace.Spec.Namespace.Name

	// Create ServiceAccounts for each role
	roles := []struct {
		role   omniav1alpha1.WorkspaceRole
		suffix string
	}{
		{omniav1alpha1.WorkspaceRoleOwner, "owner"},
		{omniav1alpha1.WorkspaceRoleEditor, "editor"},
		{omniav1alpha1.WorkspaceRoleViewer, "viewer"},
	}

	saStatus := &omniav1alpha1.ServiceAccountStatus{}

	for _, roleInfo := range roles {
		saName := fmt.Sprintf("workspace-%s-%s-sa", workspace.Name, roleInfo.suffix)

		sa := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: namespaceName,
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, sa, func() error {
			sa.Labels = map[string]string{
				labelWorkspace:        workspace.Name,
				labelWorkspaceManaged: "true",
				labelWorkspaceRole:    string(roleInfo.role),
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to create/update ServiceAccount %s: %w", saName, err)
		}

		if result != controllerutil.OperationResultNone {
			log.Info("ServiceAccount reconciled", "name", saName, "result", result)
		}

		// Update status
		switch roleInfo.role {
		case omniav1alpha1.WorkspaceRoleOwner:
			saStatus.Owner = saName
		case omniav1alpha1.WorkspaceRoleEditor:
			saStatus.Editor = saName
		case omniav1alpha1.WorkspaceRoleViewer:
			saStatus.Viewer = saName
		}
	}

	workspace.Status.ServiceAccounts = saStatus
	return nil
}
func (r *WorkspaceReconciler) reconcileRoleBindings(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespaceName := workspace.Spec.Namespace.Name

	// Create RoleBindings for workspace ServiceAccounts
	roles := []struct {
		role        omniav1alpha1.WorkspaceRole
		suffix      string
		clusterRole string
	}{
		{omniav1alpha1.WorkspaceRoleOwner, "owner", clusterRoleOwner},
		{omniav1alpha1.WorkspaceRoleEditor, "editor", clusterRoleEditor},
		{omniav1alpha1.WorkspaceRoleViewer, "viewer", clusterRoleViewer},
	}

	for _, role := range roles {
		saName := fmt.Sprintf("workspace-%s-%s-sa", workspace.Name, role.suffix)
		rbName := fmt.Sprintf("workspace-%s-%s", workspace.Name, role.suffix)

		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      rbName,
				Namespace: namespaceName,
			},
		}

		result, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
			rb.Labels = map[string]string{
				labelWorkspace:        workspace.Name,
				labelWorkspaceManaged: "true",
				labelWorkspaceRole:    string(role.role),
			}
			rb.RoleRef = rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     role.clusterRole,
			}
			rb.Subjects = []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: namespaceName,
				},
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to create/update RoleBinding %s: %w", rbName, err)
		}

		if result != controllerutil.OperationResultNone {
			log.Info("RoleBinding reconciled", "name", rbName, "result", result)
		}
	}

	// Reconcile RoleBindings for external ServiceAccounts from spec
	for _, binding := range workspace.Spec.RoleBindings {
		if len(binding.ServiceAccounts) == 0 {
			continue // Skip group-only bindings (handled at app layer)
		}

		clusterRole := r.getClusterRoleForRole(binding.Role)

		for _, sa := range binding.ServiceAccounts {
			rbName := fmt.Sprintf("%s-sa-%s-%s", workspace.Name, sanitizeName(sa.Name), sanitizeName(sa.Namespace))

			rb := &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      rbName,
					Namespace: namespaceName,
				},
			}

			result, err := controllerutil.CreateOrUpdate(ctx, r.Client, rb, func() error {
				rb.Labels = map[string]string{
					labelWorkspace:        workspace.Name,
					labelWorkspaceManaged: "true",
					labelWorkspaceRole:    string(binding.Role),
				}
				rb.RoleRef = rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "ClusterRole",
					Name:     clusterRole,
				}
				rb.Subjects = []rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Name:      sa.Name,
						Namespace: sa.Namespace,
					},
				}
				return nil
			})

			if err != nil {
				return fmt.Errorf("failed to create/update RoleBinding %s: %w", rbName, err)
			}

			if result != controllerutil.OperationResultNone {
				log.Info("External ServiceAccount RoleBinding reconciled", "name", rbName, "result", result)
			}
		}
	}

	return nil
}

func (r *WorkspaceReconciler) reconcileNetworkPolicy(ctx context.Context, workspace *omniav1alpha1.Workspace) error {
	log := logf.FromContext(ctx)
	namespaceName := workspace.Spec.Namespace.Name
	npName := fmt.Sprintf("workspace-%s-isolation", workspace.Name)

	// If network policy is not configured or isolation is disabled, delete existing NetworkPolicy
	if workspace.Spec.NetworkPolicy == nil || !workspace.Spec.NetworkPolicy.Isolate {
		np := &networkingv1.NetworkPolicy{}
		err := r.Get(ctx, client.ObjectKey{Name: npName, Namespace: namespaceName}, np)
		if err == nil {
			// NetworkPolicy exists, delete it
			if err := r.Delete(ctx, np); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete NetworkPolicy %s: %w", npName, err)
			}
			log.Info("Deleted NetworkPolicy (isolation disabled)", "name", npName)
		}
		// Clear status
		workspace.Status.NetworkPolicy = nil
		return nil
	}

	// Build the NetworkPolicy
	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      npName,
			Namespace: namespaceName,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, np, func() error {
		np.Labels = map[string]string{
			labelWorkspace:        workspace.Name,
			labelWorkspaceManaged: "true",
		}

		// Build ingress rules
		ingressRules := r.buildIngressRules(workspace)

		// Build egress rules
		egressRules := r.buildEgressRules(workspace)

		np.Spec = networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{}, // Select all pods in namespace
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: ingressRules,
			Egress:  egressRules,
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create/update NetworkPolicy %s: %w", npName, err)
	}

	if result != controllerutil.OperationResultNone {
		log.Info("NetworkPolicy reconciled", "name", npName, "result", result)
	}

	// Update status
	rulesCount := int32(len(np.Spec.Ingress) + len(np.Spec.Egress))
	workspace.Status.NetworkPolicy = &omniav1alpha1.NetworkPolicyStatus{
		Name:       npName,
		Enabled:    true,
		RulesCount: rulesCount,
	}

	return nil
}

// buildIngressRules builds the ingress rules for the NetworkPolicy
func (r *WorkspaceReconciler) buildIngressRules(workspace *omniav1alpha1.Workspace) []networkingv1.NetworkPolicyIngressRule {
	policy := workspace.Spec.NetworkPolicy
	// Pre-allocate: 1 for same namespace + 1 for shared (if enabled) + custom rules
	capacity := 1 + len(policy.AllowFrom)
	if policy.AllowSharedNamespaces == nil || *policy.AllowSharedNamespaces {
		capacity++
	}
	rules := make([]networkingv1.NetworkPolicyIngressRule, 0, capacity)

	// Allow from shared namespaces (default true)
	if policy.AllowSharedNamespaces == nil || *policy.AllowSharedNamespaces {
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							labelSharedNamespace: "true",
						},
					},
				},
			},
		})
	}

	// Allow from same namespace
	rules = append(rules, networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{
			{
				PodSelector: &metav1.LabelSelector{}, // All pods in same namespace
			},
		},
	})

	// Add custom allowFrom rules
	for _, rule := range policy.AllowFrom {
		ingressRule := networkingv1.NetworkPolicyIngressRule{
			From:  convertPeers(rule.Peers),
			Ports: convertPorts(rule.Ports),
		}
		rules = append(rules, ingressRule)
	}

	return rules
}

// buildEgressRules builds the egress rules for the NetworkPolicy
func (r *WorkspaceReconciler) buildEgressRules(workspace *omniav1alpha1.Workspace) []networkingv1.NetworkPolicyEgressRule {
	policy := workspace.Spec.NetworkPolicy
	// Pre-allocate: 1 for DNS + 1 for same namespace + 1 for shared (if enabled) + 1 for external (if enabled) + custom rules
	capacity := 2 + len(policy.AllowTo)
	if policy.AllowSharedNamespaces == nil || *policy.AllowSharedNamespaces {
		capacity++
	}
	if policy.AllowExternalAPIs == nil || *policy.AllowExternalAPIs {
		capacity++
	}
	rules := make([]networkingv1.NetworkPolicyEgressRule, 0, capacity)

	// Always allow DNS to kube-system
	dnsPort53 := intstr.FromInt32(53)
	protocolUDP := corev1.ProtocolUDP
	protocolTCP := corev1.ProtocolTCP
	rules = append(rules, networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": "kube-system",
					},
				},
			},
		},
		Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: &protocolUDP, Port: &dnsPort53},
			{Protocol: &protocolTCP, Port: &dnsPort53},
		},
	})

	// Allow to shared namespaces (default true)
	if policy.AllowSharedNamespaces == nil || *policy.AllowSharedNamespaces {
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							labelSharedNamespace: "true",
						},
					},
				},
			},
		})
	}

	// Allow to same namespace
	rules = append(rules, networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				PodSelector: &metav1.LabelSelector{}, // All pods in same namespace
			},
		},
	})

	// Allow external APIs (default true) - 0.0.0.0/0 excluding private IP ranges
	if policy.AllowExternalAPIs == nil || *policy.AllowExternalAPIs {
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: "0.0.0.0/0",
						Except: []string{
							"10.0.0.0/8",
							"172.16.0.0/12",
							"192.168.0.0/16",
						},
					},
				},
			},
		})
	}

	// Add custom allowTo rules
	for _, rule := range policy.AllowTo {
		egressRule := networkingv1.NetworkPolicyEgressRule{
			To:    convertPeers(rule.Peers),
			Ports: convertPorts(rule.Ports),
		}
		rules = append(rules, egressRule)
	}

	return rules
}

// convertPeers converts API peers to networking v1 peers
func convertPeers(peers []omniav1alpha1.NetworkPolicyPeer) []networkingv1.NetworkPolicyPeer {
	result := make([]networkingv1.NetworkPolicyPeer, 0, len(peers))
	for _, peer := range peers {
		npPeer := networkingv1.NetworkPolicyPeer{}

		if peer.NamespaceSelector != nil {
			npPeer.NamespaceSelector = &metav1.LabelSelector{
				MatchLabels: peer.NamespaceSelector.MatchLabels,
			}
		}

		if peer.PodSelector != nil {
			npPeer.PodSelector = &metav1.LabelSelector{
				MatchLabels: peer.PodSelector.MatchLabels,
			}
		}

		if peer.IPBlock != nil {
			npPeer.IPBlock = &networkingv1.IPBlock{
				CIDR:   peer.IPBlock.CIDR,
				Except: peer.IPBlock.Except,
			}
		}

		result = append(result, npPeer)
	}
	return result
}

// convertPorts converts API ports to networking v1 ports
func convertPorts(ports []omniav1alpha1.NetworkPolicyPort) []networkingv1.NetworkPolicyPort {
	result := make([]networkingv1.NetworkPolicyPort, 0, len(ports))
	for _, port := range ports {
		npPort := networkingv1.NetworkPolicyPort{}

		if port.Protocol != "" {
			protocol := corev1.Protocol(port.Protocol)
			npPort.Protocol = &protocol
		}

		if port.Port != 0 {
			portVal := intstr.FromInt32(port.Port)
			npPort.Port = &portVal
		}

		result = append(result, npPort)
	}
	return result
}

func (r *WorkspaceReconciler) getClusterRoleForRole(role omniav1alpha1.WorkspaceRole) string {
	switch role {
	case omniav1alpha1.WorkspaceRoleOwner:
		return clusterRoleOwner
	case omniav1alpha1.WorkspaceRoleEditor:
		return clusterRoleEditor
	case omniav1alpha1.WorkspaceRoleViewer:
		return clusterRoleViewer
	default:
		return clusterRoleViewer
	}
}

func (r *WorkspaceReconciler) updateMemberCount(workspace *omniav1alpha1.Workspace) {
	count := &omniav1alpha1.MemberCount{}

	for _, binding := range workspace.Spec.RoleBindings {
		groupCount := int32(len(binding.Groups))
		saCount := int32(len(binding.ServiceAccounts))
		total := groupCount + saCount

		switch binding.Role {
		case omniav1alpha1.WorkspaceRoleOwner:
			count.Owners += total
		case omniav1alpha1.WorkspaceRoleEditor:
			count.Editors += total
		case omniav1alpha1.WorkspaceRoleViewer:
			count.Viewers += total
		}
	}

	// Count direct grants
	for _, grant := range workspace.Spec.DirectGrants {
		switch grant.Role {
		case omniav1alpha1.WorkspaceRoleOwner:
			count.Owners++
		case omniav1alpha1.WorkspaceRoleEditor:
			count.Editors++
		case omniav1alpha1.WorkspaceRoleViewer:
			count.Viewers++
		}
	}

	workspace.Status.Members = count
}

func (r *WorkspaceReconciler) setCondition(
	workspace *omniav1alpha1.Workspace,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&workspace.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: workspace.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// sanitizeName converts a name to a valid Kubernetes name component
func sanitizeName(name string) string {
	// Simple sanitization - replace non-alphanumeric with dash
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c-'A'+'a') // lowercase
		} else {
			result = append(result, '-')
		}
	}
	// Trim leading/trailing dashes
	s := string(result)
	for len(s) > 0 && s[0] == '-' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '-' {
		s = s[:len(s)-1]
	}
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.Workspace{}).
		Named("workspace").
		Complete(r)
}
