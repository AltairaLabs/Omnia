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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	// Condition types for SessionAnalyticsSync.
	conditionTypeAnalyticsProviderConfigured = "ProviderConfigured"
	conditionTypeAnalyticsConnected          = "Connected"
	conditionTypeAnalyticsReady              = "Ready"

	// Event reasons for SessionAnalyticsSync.
	eventReasonAnalyticsConfigValidated       = "ConfigValidated"
	eventReasonAnalyticsProviderConfigured    = "ProviderConfigured"
	eventReasonAnalyticsProviderConfigInvalid = "ProviderConfigInvalid"
	eventReasonAnalyticsConnected             = "Connected"
	eventReasonAnalyticsConnectionFailed      = "ConnectionFailed"
	eventReasonAnalyticsSyncDisabled          = "SyncDisabled"
	eventReasonAnalyticsScheduleInvalid       = "ScheduleInvalid"
)

// AnalyticsProviderFactory creates an analytics provider for connectivity checks.
type AnalyticsProviderFactory interface {
	Ping(ctx context.Context, spec corev1alpha1.SessionAnalyticsSyncSpec) error
}

// SessionAnalyticsSyncReconciler reconciles a SessionAnalyticsSync object.
type SessionAnalyticsSyncReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	ProviderFactory AnalyticsProviderFactory
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionanalyticssyncs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionanalyticssyncs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile handles SessionAnalyticsSync reconciliation.
func (r *SessionAnalyticsSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling SessionAnalyticsSync", "name", req.Name)

	syncObj := &corev1alpha1.SessionAnalyticsSync{}
	if err := r.Get(ctx, req.NamespacedName, syncObj); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("SessionAnalyticsSync deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	syncObj.Status.ObservedGeneration = syncObj.Generation

	if !isSyncEnabled(syncObj) {
		return r.reconcileSyncDisabled(ctx, syncObj)
	}

	return r.reconcileSyncEnabled(ctx, syncObj)
}

// isSyncEnabled checks if the sync is enabled (default true when nil).
func isSyncEnabled(syncObj *corev1alpha1.SessionAnalyticsSync) bool {
	return syncObj.Spec.Enabled == nil || *syncObj.Spec.Enabled
}

// reconcileSyncDisabled handles reconciliation when sync is disabled.
func (r *SessionAnalyticsSyncReconciler) reconcileSyncDisabled(
	ctx context.Context, syncObj *corev1alpha1.SessionAnalyticsSync,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsProviderConfigured, metav1.ConditionTrue,
		eventReasonAnalyticsSyncDisabled, "analytics sync is disabled")
	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsConnected, metav1.ConditionFalse,
		eventReasonAnalyticsSyncDisabled, "analytics sync is disabled")
	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsReady, metav1.ConditionTrue,
		eventReasonAnalyticsSyncDisabled, "analytics sync is disabled")
	syncObj.Status.Phase = corev1alpha1.SessionAnalyticsSyncPhaseActive

	if err := r.Status().Update(ctx, syncObj); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("analytics sync is disabled", "name", syncObj.Name)
	return ctrl.Result{}, nil
}

// reconcileSyncEnabled handles reconciliation when sync is enabled.
func (r *SessionAnalyticsSyncReconciler) reconcileSyncEnabled(
	ctx context.Context, syncObj *corev1alpha1.SessionAnalyticsSync,
) (ctrl.Result, error) {
	if err := r.validateAnalyticsProviderConfig(syncObj); err != nil {
		return r.handleAnalyticsValidationError(ctx, syncObj, err)
	}

	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsProviderConfigured, metav1.ConditionTrue,
		eventReasonAnalyticsProviderConfigured,
		fmt.Sprintf("provider %s configuration is valid", syncObj.Spec.Provider))

	if err := r.checkAnalyticsConnectivity(ctx, syncObj); err != nil {
		return r.handleAnalyticsConnectionError(ctx, syncObj, err)
	}

	return r.setAnalyticsSuccessStatus(ctx, syncObj)
}

// validateAnalyticsProviderConfig checks that the correct provider config section exists.
func (r *SessionAnalyticsSyncReconciler) validateAnalyticsProviderConfig(
	syncObj *corev1alpha1.SessionAnalyticsSync,
) error {
	switch syncObj.Spec.Provider {
	case corev1alpha1.AnalyticsProviderSnowflake:
		if syncObj.Spec.Snowflake == nil {
			return fmt.Errorf("snowflake configuration is required when provider is snowflake")
		}
	case corev1alpha1.AnalyticsProviderBigQuery:
		if syncObj.Spec.BigQuery == nil {
			return fmt.Errorf("bigquery configuration is required when provider is bigquery")
		}
	case corev1alpha1.AnalyticsProviderClickHouse:
		if syncObj.Spec.ClickHouse == nil {
			return fmt.Errorf("clickhouse configuration is required when provider is clickhouse")
		}
	default:
		return fmt.Errorf("unsupported analytics provider: %s", syncObj.Spec.Provider)
	}
	return nil
}

// checkAnalyticsConnectivity verifies connectivity to the analytics backend.
func (r *SessionAnalyticsSyncReconciler) checkAnalyticsConnectivity(
	ctx context.Context, syncObj *corev1alpha1.SessionAnalyticsSync,
) error {
	if r.ProviderFactory == nil {
		return nil
	}
	return r.ProviderFactory.Ping(ctx, syncObj.Spec)
}

// handleAnalyticsValidationError sets error status when provider config validation fails.
func (r *SessionAnalyticsSyncReconciler) handleAnalyticsValidationError(
	ctx context.Context, syncObj *corev1alpha1.SessionAnalyticsSync, err error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(err, "analytics provider configuration validation failed")

	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsProviderConfigured, metav1.ConditionFalse,
		eventReasonAnalyticsProviderConfigInvalid, err.Error())
	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsReady, metav1.ConditionFalse,
		eventReasonAnalyticsProviderConfigInvalid, "provider configuration is invalid")
	syncObj.Status.Phase = corev1alpha1.SessionAnalyticsSyncPhaseError
	r.recordAnalyticsEvent(syncObj, "Warning", eventReasonAnalyticsProviderConfigInvalid, err.Error())

	if statusErr := r.Status().Update(ctx, syncObj); statusErr != nil {
		log.Error(statusErr, "failed to update error status")
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, nil
}

// handleAnalyticsConnectionError sets error status when connectivity check fails.
func (r *SessionAnalyticsSyncReconciler) handleAnalyticsConnectionError(
	ctx context.Context, syncObj *corev1alpha1.SessionAnalyticsSync, err error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(err, "analytics provider connectivity check failed")

	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsConnected, metav1.ConditionFalse,
		eventReasonAnalyticsConnectionFailed, err.Error())
	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsReady, metav1.ConditionFalse,
		eventReasonAnalyticsConnectionFailed, "provider connectivity check failed")
	syncObj.Status.Phase = corev1alpha1.SessionAnalyticsSyncPhaseError
	r.recordAnalyticsEvent(syncObj, "Warning", eventReasonAnalyticsConnectionFailed, err.Error())

	if statusErr := r.Status().Update(ctx, syncObj); statusErr != nil {
		log.Error(statusErr, "failed to update error status")
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, nil
}

// setAnalyticsSuccessStatus sets the success status on the analytics sync resource.
func (r *SessionAnalyticsSyncReconciler) setAnalyticsSuccessStatus(
	ctx context.Context, syncObj *corev1alpha1.SessionAnalyticsSync,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsConnected, metav1.ConditionTrue,
		eventReasonAnalyticsConnected, "analytics provider connectivity verified")
	r.setAnalyticsCondition(syncObj, conditionTypeAnalyticsReady, metav1.ConditionTrue,
		eventReasonAnalyticsConfigValidated, "analytics sync configuration is active")
	syncObj.Status.Phase = corev1alpha1.SessionAnalyticsSyncPhaseActive
	r.recordAnalyticsEvent(syncObj, "Normal", eventReasonAnalyticsConfigValidated,
		fmt.Sprintf("Analytics sync configured for provider %s", syncObj.Spec.Provider))

	if err := r.Status().Update(ctx, syncObj); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled SessionAnalyticsSync",
		"name", syncObj.Name, "provider", syncObj.Spec.Provider, "phase", syncObj.Status.Phase)
	return ctrl.Result{}, nil
}

// setAnalyticsCondition sets a condition on the SessionAnalyticsSync status.
func (r *SessionAnalyticsSyncReconciler) setAnalyticsCondition(
	syncObj *corev1alpha1.SessionAnalyticsSync,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&syncObj.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: syncObj.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// recordAnalyticsEvent emits a Kubernetes event if the recorder is available.
func (r *SessionAnalyticsSyncReconciler) recordAnalyticsEvent(
	obj runtime.Object, eventType, reason, message string,
) {
	if r.Recorder != nil {
		r.Recorder.Event(obj, eventType, reason, message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SessionAnalyticsSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.SessionAnalyticsSync{}).
		Named("sessionanalyticssync").
		Complete(r)
}
