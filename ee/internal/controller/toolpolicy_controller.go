/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/policy"
)

// ToolPolicy controller constants.
const (
	toolPolicyConditionReady     = "Ready"
	toolPolicyConditionCompiled  = "CELCompiled"
	toolPolicyEventCompiled      = "CELExpressionsCompiled"
	toolPolicyEventCompileError  = "CELCompilationError"
	toolPolicyEventValidated     = "PolicyValidated"
	toolPolicyReasonAllCompiled  = "AllExpressionsCompiled"
	toolPolicyReasonCompileError = "CompilationFailed"
)

// ToolPolicyReconciler reconciles a ToolPolicy object.
type ToolPolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolpolicies,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile validates ToolPolicy CEL expressions and updates status.
func (r *ToolPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling ToolPolicy", "name", req.Name, "namespace", req.Namespace)

	tp := &omniav1alpha1.ToolPolicy{}
	if err := r.Get(ctx, req.NamespacedName, tp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	tp.Status.ObservedGeneration = tp.Generation

	compileErr := r.validateCELExpressions(tp)

	r.updateStatus(tp, compileErr)

	if err := r.Status().Update(ctx, tp); err != nil {
		log.Error(err, "failed to update ToolPolicy status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// validateCELExpressions compiles all CEL expressions in the policy.
func (r *ToolPolicyReconciler) validateCELExpressions(tp *omniav1alpha1.ToolPolicy) error {
	evaluator, err := policy.NewEvaluator()
	if err != nil {
		return fmt.Errorf("creating evaluator: %w", err)
	}

	for _, rule := range tp.Spec.Rules {
		if err := evaluator.ValidateCEL(rule.Deny.CEL); err != nil {
			return fmt.Errorf("rule %q: %w", rule.Name, err)
		}
	}
	return nil
}

// updateStatus sets the status conditions based on compilation results.
func (r *ToolPolicyReconciler) updateStatus(tp *omniav1alpha1.ToolPolicy, compileErr error) {
	now := metav1.Now()

	if compileErr != nil {
		r.setErrorStatus(tp, compileErr, now)
		return
	}

	r.setActiveStatus(tp, now)
}

// setErrorStatus sets the status to error with appropriate conditions and events.
func (r *ToolPolicyReconciler) setErrorStatus(tp *omniav1alpha1.ToolPolicy, compileErr error, now metav1.Time) {
	tp.Status.Phase = omniav1alpha1.ToolPolicyPhaseError
	tp.Status.RuleCount = 0

	setCondition(&tp.Status.Conditions, metav1.Condition{
		Type:               toolPolicyConditionCompiled,
		Status:             metav1.ConditionFalse,
		Reason:             toolPolicyReasonCompileError,
		Message:            compileErr.Error(),
		ObservedGeneration: tp.Generation,
		LastTransitionTime: now,
	})

	setCondition(&tp.Status.Conditions, metav1.Condition{
		Type:               toolPolicyConditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             toolPolicyReasonCompileError,
		Message:            "CEL compilation failed",
		ObservedGeneration: tp.Generation,
		LastTransitionTime: now,
	})

	r.Recorder.Event(tp, "Warning", toolPolicyEventCompileError, compileErr.Error())
}

// setActiveStatus sets the status to active with appropriate conditions and events.
func (r *ToolPolicyReconciler) setActiveStatus(tp *omniav1alpha1.ToolPolicy, now metav1.Time) {
	tp.Status.Phase = omniav1alpha1.ToolPolicyPhaseActive
	tp.Status.RuleCount = int32(len(tp.Spec.Rules))

	setCondition(&tp.Status.Conditions, metav1.Condition{
		Type:               toolPolicyConditionCompiled,
		Status:             metav1.ConditionTrue,
		Reason:             toolPolicyReasonAllCompiled,
		Message:            fmt.Sprintf("%d rules compiled successfully", len(tp.Spec.Rules)),
		ObservedGeneration: tp.Generation,
		LastTransitionTime: now,
	})

	setCondition(&tp.Status.Conditions, metav1.Condition{
		Type:               toolPolicyConditionReady,
		Status:             metav1.ConditionTrue,
		Reason:             toolPolicyEventValidated,
		Message:            "Policy is active",
		ObservedGeneration: tp.Generation,
		LastTransitionTime: now,
	})

	r.Recorder.Event(tp, "Normal", toolPolicyEventCompiled,
		fmt.Sprintf("All %d CEL expressions compiled successfully", len(tp.Spec.Rules)))
}

// setCondition adds or updates a condition in the conditions slice.
func setCondition(conditions *[]metav1.Condition, condition metav1.Condition) {
	for i, c := range *conditions {
		if c.Type == condition.Type {
			(*conditions)[i] = condition
			return
		}
	}
	*conditions = append(*conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ToolPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ToolPolicy{}).
		Named("toolpolicy").
		Complete(r)
}
