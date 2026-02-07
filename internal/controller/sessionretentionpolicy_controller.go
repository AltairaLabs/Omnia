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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// SessionRetentionPolicy condition types
const (
	RetentionConditionTypePolicyValid        = "PolicyValid"
	RetentionConditionTypeWorkspacesResolved = "WorkspacesResolved"
)

// SessionRetentionPolicyReconciler reconciles a SessionRetentionPolicy object
type SessionRetentionPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionretentionpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionretentionpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionretentionpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile validates the SessionRetentionPolicy spec, resolves workspace references,
// and sets status conditions accordingly.
func (r *SessionRetentionPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling SessionRetentionPolicy", "name", req.Name)

	// Fetch the SessionRetentionPolicy instance
	policy := &omniav1alpha1.SessionRetentionPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("SessionRetentionPolicy resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get SessionRetentionPolicy")
		return ctrl.Result{}, err
	}

	// Validate the policy spec
	if err := r.validatePolicy(policy); err != nil {
		r.setCondition(policy, RetentionConditionTypePolicyValid, metav1.ConditionFalse,
			"ValidationFailed", err.Error())
		policy.Status.Phase = omniav1alpha1.SessionRetentionPolicyPhaseError
		policy.Status.ObservedGeneration = policy.Generation
		if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
			log.Error(statusErr, errMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}
	r.setCondition(policy, RetentionConditionTypePolicyValid, metav1.ConditionTrue,
		"Valid", "Policy spec is valid")

	// Resolve workspace references
	resolvedCount, err := r.resolveWorkspaces(ctx, policy)
	if err != nil {
		r.setCondition(policy, RetentionConditionTypeWorkspacesResolved, metav1.ConditionFalse,
			"ResolutionFailed", err.Error())
		policy.Status.Phase = omniav1alpha1.SessionRetentionPolicyPhaseError
		policy.Status.ObservedGeneration = policy.Generation
		policy.Status.WorkspaceCount = resolvedCount
		if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
			log.Error(statusErr, errMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}

	if len(policy.Spec.PerWorkspace) == 0 {
		r.setCondition(policy, RetentionConditionTypeWorkspacesResolved, metav1.ConditionTrue,
			"NoOverrides", "No per-workspace overrides configured")
	} else {
		r.setCondition(policy, RetentionConditionTypeWorkspacesResolved, metav1.ConditionTrue,
			"AllResolved", fmt.Sprintf("All %d workspace references resolved", resolvedCount))
	}

	// Set final status
	policy.Status.Phase = omniav1alpha1.SessionRetentionPolicyPhaseActive
	policy.Status.ObservedGeneration = policy.Generation
	policy.Status.WorkspaceCount = resolvedCount

	if err := r.Status().Update(ctx, policy); err != nil {
		log.Error(err, "Failed to update SessionRetentionPolicy status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled SessionRetentionPolicy", "name", req.Name, "phase", policy.Status.Phase)
	return ctrl.Result{}, nil
}

// validatePolicy validates the SessionRetentionPolicy spec.
func (r *SessionRetentionPolicyReconciler) validatePolicy(policy *omniav1alpha1.SessionRetentionPolicy) error {
	// Validate hot cache TTL if configured
	if policy.Spec.Default.HotCache != nil && policy.Spec.Default.HotCache.TTLAfterInactive != "" {
		if _, err := time.ParseDuration(policy.Spec.Default.HotCache.TTLAfterInactive); err != nil {
			return fmt.Errorf("invalid hot cache TTL %q: %w", policy.Spec.Default.HotCache.TTLAfterInactive, err)
		}
	}

	// Validate cold archive: retentionDays is required when enabled
	if policy.Spec.Default.ColdArchive != nil && policy.Spec.Default.ColdArchive.Enabled {
		if policy.Spec.Default.ColdArchive.RetentionDays == nil || *policy.Spec.Default.ColdArchive.RetentionDays <= 0 {
			return fmt.Errorf("cold archive retentionDays is required when cold archive is enabled")
		}
	}

	// Validate per-workspace overrides
	for name, override := range policy.Spec.PerWorkspace {
		if override.ColdArchive != nil && override.ColdArchive.Enabled {
			if override.ColdArchive.RetentionDays == nil || *override.ColdArchive.RetentionDays <= 0 {
				return fmt.Errorf("cold archive retentionDays is required when cold archive is enabled for workspace %q", name)
			}
		}
	}

	return nil
}

// resolveWorkspaces verifies that all workspaces referenced in perWorkspace overrides exist.
func (r *SessionRetentionPolicyReconciler) resolveWorkspaces(ctx context.Context, policy *omniav1alpha1.SessionRetentionPolicy) (int32, error) {
	if len(policy.Spec.PerWorkspace) == 0 {
		return 0, nil
	}

	var resolved int32
	var missing []string

	for name := range policy.Spec.PerWorkspace {
		workspace := &omniav1alpha1.Workspace{}
		// Workspace is cluster-scoped, so no namespace needed
		if err := r.Get(ctx, types.NamespacedName{Name: name}, workspace); err != nil {
			if apierrors.IsNotFound(err) {
				missing = append(missing, name)
				continue
			}
			return resolved, fmt.Errorf("failed to get workspace %q: %w", name, err)
		}
		resolved++
	}

	if len(missing) > 0 {
		return resolved, fmt.Errorf("workspaces not found: %v", missing)
	}

	return resolved, nil
}

// setCondition sets a condition on the SessionRetentionPolicy status.
func (r *SessionRetentionPolicyReconciler) setCondition(
	policy *omniav1alpha1.SessionRetentionPolicy,
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
	})
}

// findPoliciesForWorkspace maps a Workspace to SessionRetentionPolicies that reference it.
func (r *SessionRetentionPolicyReconciler) findPoliciesForWorkspace(ctx context.Context, obj client.Object) []reconcile.Request {
	workspace := obj.(*omniav1alpha1.Workspace)
	log := logf.FromContext(ctx)

	policyList := &omniav1alpha1.SessionRetentionPolicyList{}
	if err := r.List(ctx, policyList); err != nil {
		log.Error(err, "Failed to list SessionRetentionPolicies for Workspace mapping")
		return nil
	}

	var requests []reconcile.Request
	for _, p := range policyList.Items {
		if _, exists := p.Spec.PerWorkspace[workspace.Name]; exists {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: p.Name,
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *SessionRetentionPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.SessionRetentionPolicy{}).
		Watches(
			&omniav1alpha1.Workspace{},
			handler.EnqueueRequestsFromMapFunc(r.findPoliciesForWorkspace),
		).
		Named("sessionretentionpolicy").
		Complete(r)
}
