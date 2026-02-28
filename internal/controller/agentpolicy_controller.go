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
		r.setErrorStatus(policy, err)
		if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, nil // Do not retry validation errors
	}

	// Count matched agents
	matchedCount, err := r.countMatchedAgents(ctx, policy)
	if err != nil {
		log.Error(err, "failed to count matched agents")
		return ctrl.Result{}, err
	}

	// Update status
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

// validatePolicy validates the AgentPolicy spec.
func (r *AgentPolicyReconciler) validatePolicy(policy *omniav1alpha1.AgentPolicy) error {
	if policy.Spec.ClaimMapping == nil {
		return nil
	}
	return validateClaimMappings(policy.Spec.ClaimMapping.ForwardClaims)
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
