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
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// AgentPolicy condition types.
const (
	AgentPolicyConditionTypeValid   = "Valid"
	AgentPolicyConditionTypeApplied = "Applied"
)

// AgentPolicy event reasons.
const (
	EventReasonPolicyValid   = "PolicyValid"
	EventReasonPolicyInvalid = "PolicyInvalid"
	EventReasonPolicyApplied = "PolicyApplied"
)

// Claim header validation constants.
const (
	claimHeaderRequiredPrefix = "X-Omnia-Claim-"
)

// Istio AuthorizationPolicy constants.
const (
	istioSecurityAPIVersion = "security.istio.io/v1"
	istioAuthPolicyKind     = "AuthorizationPolicy"
	headerToolName          = "X-Omnia-Tool-Name"
	headerAgentName         = "X-Omnia-Agent-Name"
	istioActionAllow        = "ALLOW"
	istioActionDeny         = "DENY"
	istioActionAudit        = "AUDIT"
	managedByLabelValue     = "agentpolicy-controller"
	ownerPolicyLabel        = "omnia.altairalabs.ai/agentpolicy"
)

// AgentPolicyReconciler reconciles an AgentPolicy object.
type AgentPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch
// +kubebuilder:rbac:groups=security.istio.io,resources=authorizationpolicies,verbs=get;list;watch;create;update;patch;delete

// Reconcile reconciles the AgentPolicy resource.
func (r *AgentPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling AgentPolicy", "name", req.Name, "namespace", req.Namespace)

	// Fetch the AgentPolicy instance
	policy := &omniav1alpha1.AgentPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("AgentPolicy resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Validate the policy configuration
	if err := r.validatePolicy(policy); err != nil {
		return r.handleValidationError(ctx, policy, err)
	}

	// Reconcile Istio AuthorizationPolicies for toolAccess
	if err := r.reconcileAuthorizationPolicies(ctx, policy); err != nil {
		log.Error(err, "failed to reconcile Istio AuthorizationPolicies")
		return ctrl.Result{}, err
	}

	// Count matched agents
	matchedCount, err := r.countMatchedAgents(ctx, policy)
	if err != nil {
		log.Error(err, "failed to count matched agents")
		return ctrl.Result{}, err
	}

	// Update status
	r.setActiveStatus(policy, matchedCount)
	if err := r.Status().Update(ctx, policy); err != nil {
		log.Error(err, logMsgFailedToUpdateStatus)
		return ctrl.Result{}, err
	}

	logFields := []any{"name", req.Name, "matchedAgents", matchedCount}
	if policy.Spec.Mode == omniav1alpha1.AgentPolicyModePermissive {
		logFields = append(logFields, "mode", string(omniav1alpha1.AgentPolicyModePermissive))
	}
	log.Info("successfully reconciled AgentPolicy", logFields...)
	return ctrl.Result{}, nil
}

// handleValidationError sets error status and returns without retry.
func (r *AgentPolicyReconciler) handleValidationError(ctx context.Context, policy *omniav1alpha1.AgentPolicy, err error) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	r.setErrorStatus(policy, err)
	if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
		log.Error(statusErr, logMsgFailedToUpdateStatus)
	}
	return ctrl.Result{}, nil // Do not retry validation errors
}

// setActiveStatus updates the policy status to Active with matched agents count.
func (r *AgentPolicyReconciler) setActiveStatus(policy *omniav1alpha1.AgentPolicy, matchedCount int32) {
	policy.Status.Phase = omniav1alpha1.AgentPolicyPhaseActive
	policy.Status.MatchedAgents = matchedCount
	policy.Status.ObservedGeneration = policy.Generation

	SetCondition(&policy.Status.Conditions, policy.Generation,
		AgentPolicyConditionTypeValid, metav1.ConditionTrue,
		EventReasonPolicyValid, "Policy configuration is valid")

	appliedMsg := buildAppliedMessage(policy.Spec.Mode, matchedCount)
	SetCondition(&policy.Status.Conditions, policy.Generation,
		AgentPolicyConditionTypeApplied, metav1.ConditionTrue,
		EventReasonPolicyApplied, appliedMsg)
}

// validatePolicy validates the AgentPolicy spec.
func (r *AgentPolicyReconciler) validatePolicy(policy *omniav1alpha1.AgentPolicy) error {
	if policy.Spec.ClaimMapping != nil {
		if err := validateClaimMappings(policy.Spec.ClaimMapping.ForwardClaims); err != nil {
			return err
		}
	}
	if policy.Spec.ToolAccess != nil {
		return validateToolAccess(policy.Spec.ToolAccess)
	}
	return nil
}

// validateToolAccess validates the tool access configuration.
func validateToolAccess(cfg *omniav1alpha1.ToolAccessConfig) error {
	if len(cfg.Rules) == 0 {
		return fmt.Errorf("toolAccess rules must not be empty")
	}
	for _, rule := range cfg.Rules {
		if err := validateToolAccessRule(rule); err != nil {
			return err
		}
	}
	return nil
}

// validateToolAccessRule validates a single tool access rule.
func validateToolAccessRule(rule omniav1alpha1.ToolAccessRule) error {
	if rule.Registry == "" {
		return fmt.Errorf("toolAccess rule registry must not be empty")
	}
	if len(rule.Tools) == 0 {
		return fmt.Errorf("toolAccess rule tools must not be empty for registry %q", rule.Registry)
	}
	for _, tool := range rule.Tools {
		if tool == "" {
			return fmt.Errorf("tool name must not be empty in registry %q", rule.Registry)
		}
	}
	return nil
}

// validateClaimMappings validates each claim mapping entry.
func validateClaimMappings(entries []omniav1alpha1.ClaimMappingEntry) error {
	seen := make(map[string]bool)
	for _, entry := range entries {
		if err := validateClaimEntry(entry, seen); err != nil {
			return err
		}
	}
	return nil
}

// validateClaimEntry validates a single claim mapping entry for correctness.
func validateClaimEntry(entry omniav1alpha1.ClaimMappingEntry, seenHeaders map[string]bool) error {
	if entry.Claim == "" {
		return fmt.Errorf("claim name must not be empty")
	}
	if entry.Header == "" {
		return fmt.Errorf("header name must not be empty")
	}
	if !strings.HasPrefix(entry.Header, claimHeaderRequiredPrefix) {
		return fmt.Errorf("header %q must start with %q", entry.Header, claimHeaderRequiredPrefix)
	}
	headerLower := strings.ToLower(entry.Header)
	if seenHeaders[headerLower] {
		return fmt.Errorf("duplicate header %q in claim mappings", entry.Header)
	}
	seenHeaders[headerLower] = true
	return nil
}

// buildAppliedMessage creates the status message based on mode and matched agent count.
func buildAppliedMessage(mode omniav1alpha1.AgentPolicyMode, matchedCount int32) string {
	if mode == omniav1alpha1.AgentPolicyModePermissive {
		return fmt.Sprintf("Policy applied in permissive mode to %d agent(s) (audit only, not enforcing)", matchedCount)
	}
	return fmt.Sprintf("Policy applied to %d agent(s)", matchedCount)
}

// reconcileAuthorizationPolicies creates/updates/deletes Istio AuthorizationPolicies based on toolAccess config.
func (r *AgentPolicyReconciler) reconcileAuthorizationPolicies(ctx context.Context, policy *omniav1alpha1.AgentPolicy) error {
	desired := r.buildDesiredAuthPolicies(policy)
	return r.applyAuthPolicies(ctx, policy, desired)
}

// buildDesiredAuthPolicies builds the desired set of Istio AuthorizationPolicy resources.
func (r *AgentPolicyReconciler) buildDesiredAuthPolicies(policy *omniav1alpha1.AgentPolicy) []*unstructured.Unstructured {
	if policy.Spec.ToolAccess == nil {
		return nil
	}

	action := r.resolveIstioAction(policy)
	var policies []*unstructured.Unstructured

	switch policy.Spec.ToolAccess.Mode {
	case omniav1alpha1.ToolAccessModeAllowlist:
		policies = r.buildAllowlistPolicies(policy, action)
	case omniav1alpha1.ToolAccessModeDenylist:
		policies = r.buildDenylistPolicies(policy, action)
	}

	return policies
}

// resolveIstioAction maps policy mode to Istio action string.
func (r *AgentPolicyReconciler) resolveIstioAction(policy *omniav1alpha1.AgentPolicy) string {
	if policy.Spec.Mode == omniav1alpha1.AgentPolicyModePermissive {
		return istioActionAudit
	}
	if policy.Spec.ToolAccess.Mode == omniav1alpha1.ToolAccessModeAllowlist {
		return istioActionAllow
	}
	return istioActionDeny
}

// buildAllowlistPolicies creates ALLOW + DENY catch-all AuthorizationPolicies for allowlist mode.
func (r *AgentPolicyReconciler) buildAllowlistPolicies(policy *omniav1alpha1.AgentPolicy, action string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured

	// Create an ALLOW (or AUDIT) policy for the listed tools
	allowPolicy := r.newAuthPolicyBase(policy, policy.Name+"-allow")
	rules := buildRulesFromToolAccess(policy.Spec.ToolAccess.Rules, policy.Spec.Selector)
	setAuthPolicySpec(allowPolicy, action, rules)
	result = append(result, allowPolicy)

	// For enforce mode, add a DENY catch-all to block everything else
	if action == istioActionAllow {
		denyPolicy := r.newAuthPolicyBase(policy, policy.Name+"-deny-all")
		denyRules := buildCatchAllRules(policy.Spec.Selector)
		setAuthPolicySpec(denyPolicy, istioActionDeny, denyRules)
		result = append(result, denyPolicy)
	}

	return result
}

// buildDenylistPolicies creates DENY AuthorizationPolicies for denylist mode.
func (r *AgentPolicyReconciler) buildDenylistPolicies(policy *omniav1alpha1.AgentPolicy, action string) []*unstructured.Unstructured {
	denyPolicy := r.newAuthPolicyBase(policy, policy.Name+"-deny")
	rules := buildRulesFromToolAccess(policy.Spec.ToolAccess.Rules, policy.Spec.Selector)
	setAuthPolicySpec(denyPolicy, action, rules)
	return []*unstructured.Unstructured{denyPolicy}
}

// newAuthPolicyBase creates a base unstructured AuthorizationPolicy with metadata.
func (r *AgentPolicyReconciler) newAuthPolicyBase(policy *omniav1alpha1.AgentPolicy, name string) *unstructured.Unstructured {
	ap := &unstructured.Unstructured{}
	ap.SetAPIVersion(istioSecurityAPIVersion)
	ap.SetKind(istioAuthPolicyKind)
	ap.SetName(name)
	ap.SetNamespace(policy.Namespace)
	ap.SetLabels(map[string]string{
		labelAppManagedBy: managedByLabelValue,
		ownerPolicyLabel:  policy.Name,
	})
	ap.SetOwnerReferences([]metav1.OwnerReference{
		*metav1.NewControllerRef(policy, omniav1alpha1.GroupVersion.WithKind("AgentPolicy")),
	})
	return ap
}

// setAuthPolicySpec sets the spec on an unstructured AuthorizationPolicy.
func setAuthPolicySpec(ap *unstructured.Unstructured, action string, rules []interface{}) {
	spec := map[string]interface{}{
		"action": action,
		"rules":  rules,
	}
	if err := unstructured.SetNestedField(ap.Object, spec, "spec"); err != nil {
		// This should never fail for well-formed maps; log would be appropriate
		// but since this is a build step, we accept the silent failure.
		return
	}
}

// buildRulesFromToolAccess converts tool access rules to Istio AuthorizationPolicy rule format.
func buildRulesFromToolAccess(rules []omniav1alpha1.ToolAccessRule, selector *omniav1alpha1.AgentPolicySelector) []interface{} {
	istioRules := make([]interface{}, 0, len(rules))
	for _, rule := range rules {
		istioRule := buildSingleToolRule(rule, selector)
		istioRules = append(istioRules, istioRule)
	}
	return istioRules
}

// buildSingleToolRule converts a single ToolAccessRule into an Istio rule map.
func buildSingleToolRule(rule omniav1alpha1.ToolAccessRule, selector *omniav1alpha1.AgentPolicySelector) map[string]interface{} {
	toolValues := buildToolHeaderValues(rule)
	whenConditions := []interface{}{
		map[string]interface{}{
			"key":    "request.headers[" + headerToolName + "]",
			"values": toolValues,
		},
	}
	if selector != nil && len(selector.Agents) > 0 {
		whenConditions = append(whenConditions, buildAgentCondition(selector.Agents))
	}
	return map[string]interface{}{
		"when": whenConditions,
	}
}

// buildToolHeaderValues creates the tool header values in "registry/tool" format.
func buildToolHeaderValues(rule omniav1alpha1.ToolAccessRule) []interface{} {
	values := make([]interface{}, 0, len(rule.Tools))
	for _, tool := range rule.Tools {
		values = append(values, rule.Registry+"/"+tool)
	}
	return values
}

// buildAgentCondition creates a "when" condition for agent name matching.
func buildAgentCondition(agents []string) map[string]interface{} {
	agentValues := make([]interface{}, 0, len(agents))
	for _, a := range agents {
		agentValues = append(agentValues, a)
	}
	return map[string]interface{}{
		"key":    "request.headers[" + headerAgentName + "]",
		"values": agentValues,
	}
}

// buildCatchAllRules creates a catch-all rule that matches all requests (optionally scoped to agents).
func buildCatchAllRules(selector *omniav1alpha1.AgentPolicySelector) []interface{} {
	rule := map[string]interface{}{}
	if selector != nil && len(selector.Agents) > 0 {
		rule["when"] = []interface{}{buildAgentCondition(selector.Agents)}
	}
	return []interface{}{rule}
}

// applyAuthPolicies applies the desired AuthorizationPolicies and deletes stale ones.
func (r *AgentPolicyReconciler) applyAuthPolicies(ctx context.Context, policy *omniav1alpha1.AgentPolicy, desired []*unstructured.Unstructured) error {
	// Build set of desired names
	desiredNames := make(map[string]bool, len(desired))
	for _, d := range desired {
		desiredNames[d.GetName()] = true
	}

	// Delete stale AuthorizationPolicies owned by this AgentPolicy
	if err := r.deleteStaleAuthPolicies(ctx, policy, desiredNames); err != nil {
		return err
	}

	// Create or update desired AuthorizationPolicies
	for _, d := range desired {
		if err := r.createOrUpdateAuthPolicy(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// deleteStaleAuthPolicies removes AuthorizationPolicies owned by this policy that are no longer desired.
func (r *AgentPolicyReconciler) deleteStaleAuthPolicies(ctx context.Context, policy *omniav1alpha1.AgentPolicy, desiredNames map[string]bool) error {
	existing := &unstructured.UnstructuredList{}
	existing.SetAPIVersion(istioSecurityAPIVersion)
	existing.SetKind(istioAuthPolicyKind)
	if err := r.List(ctx, existing, client.InNamespace(policy.Namespace), client.MatchingLabels{
		ownerPolicyLabel: policy.Name,
	}); err != nil {
		if !apierrors.IsNotFound(err) && !isNoMatchError(err) {
			return fmt.Errorf("failed to list existing AuthorizationPolicies: %w", err)
		}
		return nil
	}
	for i := range existing.Items {
		if !desiredNames[existing.Items[i].GetName()] {
			if err := r.Delete(ctx, &existing.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete stale AuthorizationPolicy %q: %w", existing.Items[i].GetName(), err)
			}
		}
	}
	return nil
}

// isNoMatchError returns true if the error indicates the CRD is not installed.
func isNoMatchError(err error) bool {
	return strings.Contains(err.Error(), "no matches for kind")
}

// createOrUpdateAuthPolicy creates or updates a single Istio AuthorizationPolicy.
func (r *AgentPolicyReconciler) createOrUpdateAuthPolicy(ctx context.Context, desired *unstructured.Unstructured) error {
	existing := &unstructured.Unstructured{}
	existing.SetAPIVersion(istioSecurityAPIVersion)
	existing.SetKind(istioAuthPolicyKind)

	err := r.Get(ctx, types.NamespacedName{Name: desired.GetName(), Namespace: desired.GetNamespace()}, existing)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return fmt.Errorf("failed to get AuthorizationPolicy %q: %w", desired.GetName(), err)
	}

	// Update existing
	existing.Object["spec"] = desired.Object["spec"]
	existing.SetLabels(desired.GetLabels())
	existing.SetOwnerReferences(desired.GetOwnerReferences())
	return r.Update(ctx, existing)
}

// setErrorStatus sets the policy status to Error with appropriate conditions.
func (r *AgentPolicyReconciler) setErrorStatus(policy *omniav1alpha1.AgentPolicy, err error) {
	policy.Status.Phase = omniav1alpha1.AgentPolicyPhaseError
	policy.Status.ObservedGeneration = policy.Generation
	SetCondition(&policy.Status.Conditions, policy.Generation,
		AgentPolicyConditionTypeValid, metav1.ConditionFalse,
		EventReasonPolicyInvalid, err.Error())
}

// countMatchedAgents counts the number of AgentRuntime resources matched by the selector.
func (r *AgentPolicyReconciler) countMatchedAgents(ctx context.Context, policy *omniav1alpha1.AgentPolicy) (int32, error) {
	agentList := &omniav1alpha1.AgentRuntimeList{}
	if err := r.List(ctx, agentList, client.InNamespace(policy.Namespace)); err != nil {
		return 0, fmt.Errorf("failed to list AgentRuntimes: %w", err)
	}

	// If no selector or empty agents list, match all
	if policy.Spec.Selector == nil || len(policy.Spec.Selector.Agents) == 0 {
		return int32(len(agentList.Items)), nil
	}

	agentSet := make(map[string]bool, len(policy.Spec.Selector.Agents))
	for _, name := range policy.Spec.Selector.Agents {
		agentSet[name] = true
	}

	var count int32
	for i := range agentList.Items {
		if agentSet[agentList.Items[i].Name] {
			count++
		}
	}
	return count, nil
}

// findPoliciesForAgent maps an AgentRuntime to AgentPolicies that reference it.
func (r *AgentPolicyReconciler) findPoliciesForAgent(ctx context.Context, obj client.Object) []reconcile.Request {
	agent := obj.(*omniav1alpha1.AgentRuntime)
	log := logf.FromContext(ctx)

	policyList := &omniav1alpha1.AgentPolicyList{}
	if err := r.List(ctx, policyList, client.InNamespace(agent.Namespace)); err != nil {
		log.Error(err, "failed to list AgentPolicies for agent mapping")
		return nil
	}

	var requests []reconcile.Request
	for i := range policyList.Items {
		p := &policyList.Items[i]
		if r.policyMatchesAgent(p, agent.Name) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      p.Name,
					Namespace: p.Namespace,
				},
			})
		}
	}
	return requests
}

// policyMatchesAgent returns true if the policy applies to the given agent name.
func (r *AgentPolicyReconciler) policyMatchesAgent(policy *omniav1alpha1.AgentPolicy, agentName string) bool {
	if policy.Spec.Selector == nil || len(policy.Spec.Selector.Agents) == 0 {
		return true // No selector means match all
	}
	for _, name := range policy.Spec.Selector.Agents {
		if name == agentName {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *AgentPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.AgentPolicy{}).
		Watches(
			&omniav1alpha1.AgentRuntime{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForAgent),
		).
		Named("agentpolicy").
		Complete(r)
}
