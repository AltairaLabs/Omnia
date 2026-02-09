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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
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
	effective := computeEffectivePolicy(chain)
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
	r.setCondition(policy, ConditionTypeParentFound, metav1.ConditionFalse, EventReasonParentNotFound, msg)
	r.setCondition(policy, ConditionTypeReady, metav1.ConditionFalse,
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
		r.setCondition(policy, ConditionTypeParentFound, metav1.ConditionTrue,
			"NotApplicable", "global policies have no parent")
	} else {
		r.setCondition(policy, ConditionTypeParentFound, metav1.ConditionTrue,
			EventReasonPolicyValidated, fmt.Sprintf("parent policy found: %s", parent.Name))
	}
}

// handleStoreError handles failures when storing the effective policy ConfigMap.
func (r *SessionPrivacyPolicyReconciler) handleStoreError(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy, err error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(err, "failed to store effective policy")

	r.setCondition(policy, ConditionTypeEffectivePolicyStored, metav1.ConditionFalse,
		EventReasonConfigMapSyncFailed, err.Error())
	r.setCondition(policy, ConditionTypeReady, metav1.ConditionFalse,
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
	r.setCondition(policy, ConditionTypeEffectivePolicyStored, metav1.ConditionTrue,
		EventReasonEffectivePolicyComputed, "effective policy stored in ConfigMap")
	r.setCondition(policy, ConditionTypeReady, metav1.ConditionTrue,
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

// computeEffectivePolicy merges the inheritance chain, applying stricter rules at each level.
func computeEffectivePolicy(chain []*omniav1alpha1.SessionPrivacyPolicy) *omniav1alpha1.SessionPrivacyPolicySpec {
	if len(chain) == 0 {
		return &omniav1alpha1.SessionPrivacyPolicySpec{}
	}

	effective := chain[0].Spec.DeepCopy()
	for i := 1; i < len(chain); i++ {
		effective = mergeStricter(effective, &chain[i].Spec)
	}
	return effective
}

// mergeStricter merges an override policy into a base, applying the stricter of each setting.
func mergeStricter(
	base, override *omniav1alpha1.SessionPrivacyPolicySpec,
) *omniav1alpha1.SessionPrivacyPolicySpec {
	result := base.DeepCopy()

	// recording.enabled: false wins (can't enable if parent disables)
	result.Recording.Enabled = base.Recording.Enabled && override.Recording.Enabled
	// recording.facadeData: false wins
	result.Recording.FacadeData = base.Recording.FacadeData && override.Recording.FacadeData
	// recording.richData: false wins
	result.Recording.RichData = base.Recording.RichData && override.Recording.RichData

	result.Recording.PII = mergePII(base.Recording.PII, override.Recording.PII)
	result.UserOptOut = mergeUserOptOut(base.UserOptOut, override.UserOptOut)
	result.Retention = mergeRetention(base.Retention, override.Retention)
	result.Encryption = mergeEncryption(base.Encryption, override.Encryption)
	result.AuditLog = mergeAuditLog(base.AuditLog, override.AuditLog)

	return result
}

// mergePII merges PII configs with the stricter rule.
func mergePII(base, override *omniav1alpha1.PIIConfig) *omniav1alpha1.PIIConfig {
	if base == nil && override == nil {
		return nil
	}

	result := &omniav1alpha1.PIIConfig{
		Redact:  boolFromEither(base, override, func(c *omniav1alpha1.PIIConfig) bool { return c.Redact }),
		Encrypt: boolFromEither(base, override, func(c *omniav1alpha1.PIIConfig) bool { return c.Encrypt }),
	}
	result.Patterns = mergePatterns(base, override)
	return result
}

// boolFromEither returns true if either config has the field set to true (true wins).
func boolFromEither[T any](a, b *T, getter func(*T) bool) bool {
	return (a != nil && getter(a)) || (b != nil && getter(b))
}

// mergePatterns returns the union of PII patterns from both configs.
func mergePatterns(base, override *omniav1alpha1.PIIConfig) []string {
	seen := map[string]bool{}
	var result []string
	for _, cfg := range []*omniav1alpha1.PIIConfig{base, override} {
		if cfg == nil {
			continue
		}
		for _, p := range cfg.Patterns {
			if !seen[p] {
				result = append(result, p)
				seen[p] = true
			}
		}
	}
	return result
}

// mergeUserOptOut merges user opt-out configs with the stricter rule.
func mergeUserOptOut(base, override *omniav1alpha1.UserOptOutConfig) *omniav1alpha1.UserOptOutConfig {
	if base == nil && override == nil {
		return nil
	}

	return &omniav1alpha1.UserOptOutConfig{
		Enabled:             boolFromEither(base, override, func(c *omniav1alpha1.UserOptOutConfig) bool { return c.Enabled }),
		HonorDeleteRequests: boolFromEither(base, override, func(c *omniav1alpha1.UserOptOutConfig) bool { return c.HonorDeleteRequests }),
		DeleteWithinDays: minInt32Ptr(
			getOptionalInt32(base, func(c *omniav1alpha1.UserOptOutConfig) *int32 { return c.DeleteWithinDays }),
			getOptionalInt32(override, func(c *omniav1alpha1.UserOptOutConfig) *int32 { return c.DeleteWithinDays }),
		),
	}
}

// mergeRetention merges retention configs taking the minimum of each field.
func mergeRetention(
	base, override *omniav1alpha1.PrivacyRetentionConfig,
) *omniav1alpha1.PrivacyRetentionConfig {
	if base == nil && override == nil {
		return nil
	}
	if base == nil {
		return override.DeepCopy()
	}
	if override == nil {
		return base.DeepCopy()
	}

	return &omniav1alpha1.PrivacyRetentionConfig{
		Facade:   mergeRetentionTier(base.Facade, override.Facade),
		RichData: mergeRetentionTier(base.RichData, override.RichData),
	}
}

// mergeRetentionTier merges a retention tier taking the minimum.
func mergeRetentionTier(
	base, override *omniav1alpha1.PrivacyRetentionTierConfig,
) *omniav1alpha1.PrivacyRetentionTierConfig {
	if base == nil && override == nil {
		return nil
	}
	if base == nil {
		return override.DeepCopy()
	}
	if override == nil {
		return base.DeepCopy()
	}

	return &omniav1alpha1.PrivacyRetentionTierConfig{
		WarmDays: minInt32Ptr(base.WarmDays, override.WarmDays),
		ColdDays: minInt32Ptr(base.ColdDays, override.ColdDays),
	}
}

// mergeEncryption merges encryption configs; true wins.
func mergeEncryption(base, override *omniav1alpha1.EncryptionConfig) *omniav1alpha1.EncryptionConfig {
	if base == nil && override == nil {
		return nil
	}

	result := &omniav1alpha1.EncryptionConfig{
		Enabled: boolFromEither(base, override, func(c *omniav1alpha1.EncryptionConfig) bool { return c.Enabled }),
	}

	// Use the most specific non-empty KMS provider (override takes precedence if set)
	if override != nil && override.KMSProvider != "" {
		result.KMSProvider = override.KMSProvider
		result.SecretRef = override.SecretRef
	} else if base != nil {
		result.KMSProvider = base.KMSProvider
		result.SecretRef = base.SecretRef
	}

	return result
}

// mergeAuditLog merges audit log configs; true wins.
func mergeAuditLog(base, override *omniav1alpha1.AuditLogConfig) *omniav1alpha1.AuditLogConfig {
	if base == nil && override == nil {
		return nil
	}

	return &omniav1alpha1.AuditLogConfig{
		Enabled: boolFromEither(base, override, func(c *omniav1alpha1.AuditLogConfig) bool { return c.Enabled }),
		RetentionDays: minInt32Ptr(
			getOptionalInt32(base, func(c *omniav1alpha1.AuditLogConfig) *int32 { return c.RetentionDays }),
			getOptionalInt32(override, func(c *omniav1alpha1.AuditLogConfig) *int32 { return c.RetentionDays }),
		),
	}
}

// getOptionalInt32 safely extracts an *int32 field from a possibly-nil struct.
func getOptionalInt32[T any](obj *T, getter func(*T) *int32) *int32 {
	if obj == nil {
		return nil
	}
	return getter(obj)
}

// minInt32Ptr returns the minimum of two *int32 values (nil means unset).
func minInt32Ptr(a, b *int32) *int32 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *a < *b {
		return a
	}
	return b
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

// setCondition sets a condition on the SessionPrivacyPolicy status.
func (r *SessionPrivacyPolicyReconciler) setCondition(
	policy *omniav1alpha1.SessionPrivacyPolicy,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: policy.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// setErrorStatus sets an error status on the policy and persists it.
func (r *SessionPrivacyPolicyReconciler) setErrorStatus(
	ctx context.Context,
	policy *omniav1alpha1.SessionPrivacyPolicy,
	reason, message string,
) {
	log := logf.FromContext(ctx)
	r.setCondition(policy, ConditionTypeReady, metav1.ConditionFalse, reason, message)
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
