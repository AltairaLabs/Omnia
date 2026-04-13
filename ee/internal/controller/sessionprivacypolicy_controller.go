/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/metrics"
)

const (
	// Condition types for SessionPrivacyPolicy.
	ConditionTypeReady = "Ready"

	// Event reasons for SessionPrivacyPolicy.
	EventReasonPolicyValidated = "PolicyValidated"
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
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile validates a SessionPrivacyPolicy and marks it Active.
func (r *SessionPrivacyPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling SessionPrivacyPolicy", "name", req.Name, "namespace", req.Namespace)

	policy := &omniav1alpha1.SessionPrivacyPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	policy.Status.ObservedGeneration = policy.Generation
	SetCondition(&policy.Status.Conditions, policy.Generation,
		ConditionTypeReady, metav1.ConditionTrue,
		EventReasonPolicyValidated, "policy is valid and active")
	policy.Status.Phase = omniav1alpha1.SessionPrivacyPolicyPhaseActive

	if err := r.Status().Update(ctx, policy); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	if r.Recorder != nil {
		r.Recorder.Event(policy, corev1.EventTypeNormal, EventReasonPolicyValidated, "Policy validated and active")
	}
	if r.Metrics != nil {
		r.Metrics.RecordEffectivePolicyComputation(policy.Name)
	}

	log.V(1).Info("SessionPrivacyPolicy active", "name", policy.Name, "namespace", policy.Namespace)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SessionPrivacyPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.SessionPrivacyPolicy{}).
		Named("sessionprivacypolicy").
		Complete(r)
}
