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

// ToolPolicy controller condition types and event reasons.
const (
	toolPolicyConditionReady    = "Ready"
	toolPolicyConditionCompiled = "Compiled"
	toolPolicyEventCompiled     = "PolicyCompiled"
	toolPolicyEventCompileError = "CompileError"
	toolPolicyEventValidated    = "PolicyValidated"
)

// ToolPolicyReconciler reconciles a ToolPolicy object.
type ToolPolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Evaluator *policy.Evaluator
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolpolicies,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile handles ToolPolicy reconciliation.
func (r *ToolPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling ToolPolicy", "name", req.Name, "namespace", req.Namespace)

	tp := &omniav1alpha1.ToolPolicy{}
	if err := r.Get(ctx, req.NamespacedName, tp); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ToolPolicy deleted, removing from evaluator")
			r.Evaluator.RemovePolicy(req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	tp.Status.ObservedGeneration = tp.Generation

	// Validate and compile all CEL expressions
	if err := r.validateAndCompile(ctx, tp); err != nil {
		return r.handleCompileError(ctx, tp, err)
	}

	// Set success status
	r.setSuccessStatus(tp)
	if err := r.Status().Update(ctx, tp); err != nil {
		log.Error(err, "failed to update ToolPolicy status")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled ToolPolicy",
		"name", tp.Name,
		"rules", len(tp.Spec.Rules),
		"phase", tp.Status.Phase)
	return ctrl.Result{}, nil
}

// validateAndCompile validates all CEL expressions and compiles the policy.
func (r *ToolPolicyReconciler) validateAndCompile(
	_ context.Context, tp *omniav1alpha1.ToolPolicy,
) error {
	for _, rule := range tp.Spec.Rules {
		if err := r.Evaluator.ValidateCEL(rule.Deny.CEL); err != nil {
			return fmt.Errorf("rule %q: %w", rule.Name, err)
		}
	}
	return r.Evaluator.CompilePolicy(tp)
}

// handleCompileError sets error status and returns the result.
func (r *ToolPolicyReconciler) handleCompileError(
	ctx context.Context, tp *omniav1alpha1.ToolPolicy, compileErr error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(compileErr, "failed to compile ToolPolicy")

	SetCondition(&tp.Status.Conditions, tp.Generation,
		toolPolicyConditionCompiled, metav1.ConditionFalse,
		toolPolicyEventCompileError, compileErr.Error())
	SetCondition(&tp.Status.Conditions, tp.Generation,
		toolPolicyConditionReady, metav1.ConditionFalse,
		toolPolicyEventCompileError, "policy has CEL compilation errors")
	tp.Status.Phase = omniav1alpha1.ToolPolicyPhaseError
	tp.Status.RuleCount = 0

	r.recordEvent(tp, "Warning", toolPolicyEventCompileError, compileErr.Error())

	if err := r.Status().Update(ctx, tp); err != nil {
		log.Error(err, "failed to update error status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// setSuccessStatus sets the success conditions on the policy.
func (r *ToolPolicyReconciler) setSuccessStatus(tp *omniav1alpha1.ToolPolicy) {
	ruleCount := int32(len(tp.Spec.Rules))
	msg := fmt.Sprintf("all %d rules compiled successfully", ruleCount)

	SetCondition(&tp.Status.Conditions, tp.Generation,
		toolPolicyConditionCompiled, metav1.ConditionTrue,
		toolPolicyEventCompiled, msg)
	SetCondition(&tp.Status.Conditions, tp.Generation,
		toolPolicyConditionReady, metav1.ConditionTrue,
		toolPolicyEventValidated, "policy is active")
	tp.Status.Phase = omniav1alpha1.ToolPolicyPhaseActive
	tp.Status.RuleCount = ruleCount

	r.recordEvent(tp, "Normal", toolPolicyEventCompiled, msg)
}

// recordEvent emits a Kubernetes event if the recorder is available.
func (r *ToolPolicyReconciler) recordEvent(obj runtime.Object, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(obj, eventType, reason, message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ToolPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ToolPolicy{}).
		Named("toolpolicy").
		Complete(r)
}
