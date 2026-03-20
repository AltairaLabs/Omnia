/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

const (
	// ConfigMap namespace for effective policies.
	privacyPolicyNamespace = "omnia-system"

	// Condition types for SessionPrivacyPolicy.
	ConditionTypeReady                 = "Ready"
	ConditionTypeParentFound           = "ParentFound"
	ConditionTypeEffectivePolicyStored = "EffectivePolicyStored"

	// Event reasons for SessionPrivacyPolicy.
	EventReasonPolicyValidated         = "PolicyValidated"
	EventReasonEffectivePolicyComputed = "EffectivePolicyComputed"
	EventReasonParentNotFound          = "ParentNotFound"
	EventReasonInheritanceViolation    = "InheritanceViolation"
	EventReasonConfigMapSyncFailed     = "ConfigMapSyncFailed"
	EventReasonChildPoliciesRequeued   = "ChildPoliciesRequeued"
)

// SessionPrivacyPolicyReconciler reconciles a SessionPrivacyPolicy object.
type SessionPrivacyPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Metrics  *metrics.PrivacyPolicyMetrics
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionprivacypolicies,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionprivacypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile handles SessionPrivacyPolicy reconciliation.
func (r *SessionPrivacyPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling SessionPrivacyPolicy", "name", req.Name)

	// Fetch the SessionPrivacyPolicy
	policy := &omniav1alpha1.SessionPrivacyPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("SessionPrivacyPolicy not found, cleaning up")
			if cleanupErr := r.cleanupEffectivePolicy(ctx, req.Name); cleanupErr != nil {
				log.Error(cleanupErr, "failed to clean up ConfigMap on delete")
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Update observed generation
	policy.Status.ObservedGeneration = policy.Generation

	// Find parent policy
	parent, err := r.findParentPolicy(ctx, policy)
	if err != nil {
		log.Error(err, "failed to find parent policy")
		r.setErrorStatus(ctx, policy, "ParentLookupFailed", err.Error())
		r.recordMetricError(policy.Name, "parent_lookup")
		return ctrl.Result{}, err
	}

	// Handle orphaned child policies
	if policy.Spec.Level != omniav1alpha1.PolicyLevelGlobal && parent == nil {
		return r.handleOrphanedPolicy(ctx, policy)
	}

	// Set ParentFound condition
	r.setParentFoundCondition(policy, parent)

	// Build inheritance chain and compute effective policy
	chain := r.buildInheritanceChain(ctx, policy, parent)
	effective := privacy.ComputeEffectivePolicy(chain)
	r.recordComputationMetrics(policy.Name, len(chain))

	// Store effective policy in ConfigMap
	if err := r.storeEffectivePolicy(ctx, policy, effective, parent); err != nil {
		return r.handleStoreError(ctx, policy, err)
	}

	// Set success conditions and update status
	r.setSuccessStatus(policy)
	if err := r.Status().Update(ctx, policy); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	// Requeue child policies so they recompute their effective policies
	r.requeueChildren(ctx, policy)

	// Update active policy counts
	if r.Metrics != nil {
		r.updateActivePolicyCounts(ctx)
	}

	log.Info("successfully reconciled SessionPrivacyPolicy", "phase", policy.Status.Phase)
	return ctrl.Result{}, nil
}

// handleOrphanedPolicy handles the case where a child policy has no parent.
func (r *SessionPrivacyPolicyReconciler) handleOrphanedPolicy(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("no parent policy found", "level", policy.Spec.Level)

	msg := fmt.Sprintf("no parent policy found for %s-level policy", policy.Spec.Level)
	SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeParentFound, metav1.ConditionFalse, EventReasonParentNotFound, msg)
	SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeReady, metav1.ConditionFalse,
		EventReasonParentNotFound, "policy requires a parent but none was found")
	policy.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseError
	r.recordEvent(policy, corev1.EventTypeWarning, EventReasonParentNotFound,
		fmt.Sprintf("No parent policy found for %s-level policy", policy.Spec.Level))
	r.recordMetricError(policy.Name, "parent_not_found")

	if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
		log.Error(statusErr, "failed to update status")
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, nil
}

// setParentFoundCondition sets the ParentFound condition based on policy level.
func (r *SessionPrivacyPolicyReconciler) setParentFoundCondition(
	policy *omniav1alpha1.SessionPrivacyPolicy, parent *omniav1alpha1.SessionPrivacyPolicy,
) {
	if policy.Spec.Level == omniav1alpha1.PolicyLevelGlobal {
		SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeParentFound, metav1.ConditionTrue,
			"NotApplicable", "global policies have no parent")
	} else {
		SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeParentFound, metav1.ConditionTrue,
			EventReasonPolicyValidated, fmt.Sprintf("parent policy found: %s", parent.Name))
	}
}

// handleStoreError handles failures when storing the effective policy ConfigMap.
func (r *SessionPrivacyPolicyReconciler) handleStoreError(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy, err error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(err, "failed to store effective policy")

	SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeEffectivePolicyStored, metav1.ConditionFalse,
		EventReasonConfigMapSyncFailed, err.Error())
	SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeReady, metav1.ConditionFalse,
		EventReasonConfigMapSyncFailed, "failed to store effective policy in ConfigMap")
	policy.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseError
	r.recordEvent(policy, corev1.EventTypeWarning, EventReasonConfigMapSyncFailed, err.Error())
	r.recordMetricConfigMapError(policy.Name)

	if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
		log.Error(statusErr, "failed to update status")
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, err
}

// setSuccessStatus sets the success conditions on the policy.
func (r *SessionPrivacyPolicyReconciler) setSuccessStatus(policy *omniav1alpha1.SessionPrivacyPolicy) {
	SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeEffectivePolicyStored, metav1.ConditionTrue,
		EventReasonEffectivePolicyComputed, "effective policy stored in ConfigMap")
	SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeReady, metav1.ConditionTrue,
		EventReasonPolicyValidated, "policy is active and effective policy is stored")
	policy.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseActive
	r.recordEvent(policy, corev1.EventTypeNormal, EventReasonEffectivePolicyComputed,
		"Effective policy computed and stored in ConfigMap")
}

// requeueChildren triggers reconciliation for all child policies.
func (r *SessionPrivacyPolicyReconciler) requeueChildren(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) {
	log := logf.FromContext(ctx)
	childRequests := r.findChildPolicies(ctx, policy)
	if len(childRequests) == 0 {
		return
	}

	log.Info("requeuing child policies", "count", len(childRequests))
	r.recordEvent(policy, corev1.EventTypeNormal, EventReasonChildPoliciesRequeued,
		fmt.Sprintf("Requeued %d child policies", len(childRequests)))

	for _, childReq := range childRequests {
		child := &omniav1alpha1.SessionPrivacyPolicy{}
		if getErr := r.Get(ctx, childReq.NamespacedName, child); getErr != nil {
			log.V(1).Info("child policy not found during requeue", "name", childReq.Name)
			continue
		}
		if child.Annotations == nil {
			child.Annotations = map[string]string{}
		}
		child.Annotations["omnia.altairalabs.ai/parent-generation"] = fmt.Sprintf("%d", policy.Generation)
		if updateErr := r.Update(ctx, child); updateErr != nil {
			log.Error(updateErr, "failed to requeue child policy", "child", child.Name)
		}
	}
}

// recordEvent emits a Kubernetes event if the recorder is available.
func (r *SessionPrivacyPolicyReconciler) recordEvent(
	obj runtime.Object, eventType, reason, message string,
) {
	if r.Recorder != nil {
		r.Recorder.Event(obj, eventType, reason, message)
	}
}

// recordMetricError records a reconcile error metric if metrics are available.
func (r *SessionPrivacyPolicyReconciler) recordMetricError(policyName, errorType string) {
	if r.Metrics != nil {
		r.Metrics.RecordReconcileError(policyName, errorType)
	}
}

// recordMetricConfigMapError records a ConfigMap sync error metric if metrics are available.
func (r *SessionPrivacyPolicyReconciler) recordMetricConfigMapError(policyName string) {
	if r.Metrics != nil {
		r.Metrics.RecordConfigMapSyncError(policyName)
	}
}

// recordComputationMetrics records effective policy computation metrics.
func (r *SessionPrivacyPolicyReconciler) recordComputationMetrics(policyName string, chainDepth int) {
	if r.Metrics != nil {
		r.Metrics.RecordEffectivePolicyComputation(policyName)
		r.Metrics.SetInheritanceDepth(policyName, chainDepth)
	}
}

// findParentPolicy locates the applicable parent policy for inheritance.
func (r *SessionPrivacyPolicyReconciler) findParentPolicy(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) (*omniav1alpha1.SessionPrivacyPolicy, error) {
	if policy.Spec.Level == omniav1alpha1.PolicyLevelGlobal {
		return nil, nil
	}

	var list omniav1alpha1.SessionPrivacyPolicyList
	if err := r.List(ctx, &list); err != nil {
		return nil, err
	}

	switch policy.Spec.Level {
	case omniav1alpha1.PolicyLevelWorkspace:
		return findPrivacyPolicyByLevel(list.Items, omniav1alpha1.PolicyLevelGlobal, ""), nil

	case omniav1alpha1.PolicyLevelAgent:
		// Try workspace-level parent first
		wsName := ""
		if policy.Spec.AgentRef != nil {
			wsName = policy.Spec.AgentRef.Namespace
		}
		if ws := findPrivacyPolicyByLevel(list.Items, omniav1alpha1.PolicyLevelWorkspace, wsName); ws != nil {
			return ws, nil
		}
		// Fall back to global
		return findPrivacyPolicyByLevel(list.Items, omniav1alpha1.PolicyLevelGlobal, ""), nil
	}

	return nil, nil
}

// findPrivacyPolicyByLevel finds the first policy matching the given level.
func findPrivacyPolicyByLevel(
	policies []omniav1alpha1.SessionPrivacyPolicy,
	level omniav1alpha1.PolicyLevel,
	workspaceName string,
) *omniav1alpha1.SessionPrivacyPolicy {
	for i := range policies {
		p := &policies[i]
		if p.Spec.Level != level {
			continue
		}
		if level == omniav1alpha1.PolicyLevelWorkspace && workspaceName != "" {
			if p.Spec.WorkspaceRef == nil || p.Spec.WorkspaceRef.Name != workspaceName {
				continue
			}
		}
		return p
	}
	return nil
}

// findChildPolicies finds policies that depend on the given policy as a parent.
func (r *SessionPrivacyPolicyReconciler) findChildPolicies(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) []ctrl.Request {
	var list omniav1alpha1.SessionPrivacyPolicyList
	if err := r.List(ctx, &list); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for i := range list.Items {
		child := &list.Items[i]
		if child.Name == policy.Name {
			continue
		}

		if r.isChildOf(child, policy) {
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{Name: child.Name},
			})
		}
	}

	return requests
}

// isChildOf checks if the child policy depends on the parent policy.
func (r *SessionPrivacyPolicyReconciler) isChildOf(
	child, parent *omniav1alpha1.SessionPrivacyPolicy,
) bool {
	switch parent.Spec.Level {
	case omniav1alpha1.PolicyLevelGlobal:
		return child.Spec.Level == omniav1alpha1.PolicyLevelWorkspace ||
			child.Spec.Level == omniav1alpha1.PolicyLevelAgent
	case omniav1alpha1.PolicyLevelWorkspace:
		return child.Spec.Level == omniav1alpha1.PolicyLevelAgent &&
			child.Spec.AgentRef != nil &&
			parent.Spec.WorkspaceRef != nil &&
			child.Spec.AgentRef.Namespace == parent.Spec.WorkspaceRef.Name
	}
	return false
}

// buildInheritanceChain builds the full chain from global → workspace → agent.
func (r *SessionPrivacyPolicyReconciler) buildInheritanceChain(
	ctx context.Context,
	policy *omniav1alpha1.SessionPrivacyPolicy,
	parent *omniav1alpha1.SessionPrivacyPolicy,
) []*omniav1alpha1.SessionPrivacyPolicy {
	switch policy.Spec.Level {
	case omniav1alpha1.PolicyLevelGlobal:
		return []*omniav1alpha1.SessionPrivacyPolicy{policy}

	case omniav1alpha1.PolicyLevelWorkspace:
		if parent != nil {
			return []*omniav1alpha1.SessionPrivacyPolicy{parent, policy}
		}
		return []*omniav1alpha1.SessionPrivacyPolicy{policy}

	case omniav1alpha1.PolicyLevelAgent:
		return r.buildAgentChain(ctx, policy, parent)
	}

	return []*omniav1alpha1.SessionPrivacyPolicy{policy}
}

// buildAgentChain builds the inheritance chain for an agent-level policy.
func (r *SessionPrivacyPolicyReconciler) buildAgentChain(
	ctx context.Context,
	policy *omniav1alpha1.SessionPrivacyPolicy,
	parent *omniav1alpha1.SessionPrivacyPolicy,
) []*omniav1alpha1.SessionPrivacyPolicy {
	if parent == nil {
		return []*omniav1alpha1.SessionPrivacyPolicy{policy}
	}
	// If parent is workspace-level, also include its global parent
	if parent.Spec.Level == omniav1alpha1.PolicyLevelWorkspace {
		var list omniav1alpha1.SessionPrivacyPolicyList
		if err := r.List(ctx, &list); err == nil {
			if global := findPrivacyPolicyByLevel(list.Items, omniav1alpha1.PolicyLevelGlobal, ""); global != nil {
				return []*omniav1alpha1.SessionPrivacyPolicy{global, parent, policy}
			}
		}
		return []*omniav1alpha1.SessionPrivacyPolicy{parent, policy}
	}
	// Parent is global (fallback)
	return []*omniav1alpha1.SessionPrivacyPolicy{parent, policy}
}

// storeEffectivePolicy creates or updates a ConfigMap with the computed effective policy.
func (r *SessionPrivacyPolicyReconciler) storeEffectivePolicy(
	ctx context.Context,
	policy *omniav1alpha1.SessionPrivacyPolicy,
	effective *omniav1alpha1.SessionPrivacyPolicySpec,
	parent *omniav1alpha1.SessionPrivacyPolicy,
) error {
	log := logf.FromContext(ctx)

	effectiveJSON, err := json.MarshalIndent(effective, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal effective policy: %w", err)
	}

	parentName := ""
	if parent != nil {
		parentName = parent.Name
	}

	cmName := fmt.Sprintf("omnia-privacy-policy-effective-%s", policy.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: privacyPolicyNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":            "omnia",
				"app.kubernetes.io/component":       "session-privacy-policy",
				"omnia.altairalabs.ai/policy-name":  policy.Name,
				"omnia.altairalabs.ai/policy-level": string(policy.Spec.Level),
				"app.kubernetes.io/managed-by":      "omnia-operator",
			},
		},
		Data: map[string]string{
			"effective-policy": string(effectiveJSON),
			"parent-policy":    parentName,
		},
	}

	// Try to get existing ConfigMap
	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: privacyPolicyNamespace}, existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			if createErr := r.Create(ctx, cm); createErr != nil {
				return fmt.Errorf("failed to create ConfigMap: %w", createErr)
			}
			log.Info("created effective policy ConfigMap", "name", cmName)
			return nil
		}
		return fmt.Errorf("failed to get existing ConfigMap: %w", err)
	}

	// Update existing ConfigMap
	existing.Data = cm.Data
	existing.Labels = cm.Labels
	if updateErr := r.Update(ctx, existing); updateErr != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", updateErr)
	}
	log.V(1).Info("updated effective policy ConfigMap", "name", cmName)
	return nil
}

// cleanupEffectivePolicy removes the ConfigMap for a deleted policy.
func (r *SessionPrivacyPolicyReconciler) cleanupEffectivePolicy(ctx context.Context, policyName string) error {
	log := logf.FromContext(ctx)

	cmName := fmt.Sprintf("omnia-privacy-policy-effective-%s", policyName)
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: privacyPolicyNamespace}, cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if err := r.Delete(ctx, cm); err != nil {
		return fmt.Errorf("failed to delete ConfigMap %s: %w", cmName, err)
	}
	log.Info("deleted effective policy ConfigMap", "name", cmName)
	return nil
}

// setErrorStatus sets an error status on the policy and persists it.
func (r *SessionPrivacyPolicyReconciler) setErrorStatus(
	ctx context.Context,
	policy *omniav1alpha1.SessionPrivacyPolicy,
	reason, message string,
) {
	log := logf.FromContext(ctx)
	SetCondition(&policy.Status.Conditions, policy.Generation, ConditionTypeReady, metav1.ConditionFalse, reason, message)
	policy.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseError
	r.recordEvent(policy, corev1.EventTypeWarning, reason, message)
	if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
		log.Error(statusErr, "failed to update error status")
	}
}

// updateActivePolicyCounts updates the active policy gauge metrics by listing all policies.
func (r *SessionPrivacyPolicyReconciler) updateActivePolicyCounts(ctx context.Context) {
	var list omniav1alpha1.SessionPrivacyPolicyList
	if err := r.List(ctx, &list); err != nil {
		return
	}

	counts := map[string]int{
		"global":    0,
		"workspace": 0,
		"agent":     0,
	}
	for i := range list.Items {
		if list.Items[i].Status.Phase == omniav1alpha1.SessionPrivacyPolicyPhaseActive {
			counts[string(list.Items[i].Spec.Level)]++
		}
	}

	for level, count := range counts {
		r.Metrics.SetActivePolicies(level, count)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SessionPrivacyPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.SessionPrivacyPolicy{}).
		Named("sessionprivacypolicy").
		Complete(r)
}
