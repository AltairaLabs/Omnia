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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/metrics"
)

// SessionRetentionPolicy condition types
const (
	RetentionConditionTypePolicyValid        = "PolicyValid"
	RetentionConditionTypeWorkspacesResolved = "WorkspacesResolved"
	RetentionConditionTypeReady              = "Ready"
)

// Event reason constants
const (
	RetentionEventReasonValidated          = "PolicyValidated"
	RetentionEventReasonValidationFailed   = "PolicyValidationFailed"
	RetentionEventReasonWorkspacesResolved = "WorkspacesResolved"
	RetentionEventReasonWorkspacesMissing  = "WorkspacesMissing"
	RetentionEventReasonConfigSynced       = "ConfigSynced"
	RetentionEventReasonConfigSyncFailed   = "ConfigSyncFailed"
	RetentionEventReasonActive             = "PolicyActive"
	RetentionEventReasonDeleting           = "PolicyDeleting"
)

// Finalizer for ConfigMap cleanup
const retentionPolicyFinalizer = "sessionretentionpolicy.omnia.altairalabs.ai/configmap-cleanup"

// ResolvedRetentionConfig is the format projected into the ConfigMap.
// Mirrors the flat CRD spec — workspaces opt in via
// Workspace.spec.services[].session.policyRef.
type ResolvedRetentionConfig struct {
	HotCache    *omniav1alpha1.HotCacheConfig    `json:"hotCache,omitempty"`
	WarmStore   *omniav1alpha1.WarmStoreConfig   `json:"warmStore,omitempty"`
	ColdArchive *omniav1alpha1.ColdArchiveConfig `json:"coldArchive,omitempty"`
}

// SessionRetentionPolicyReconciler reconciles a SessionRetentionPolicy object
type SessionRetentionPolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	Namespace string
	Metrics   *metrics.RetentionMetrics
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionretentionpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionretentionpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionretentionpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=workspaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile validates the SessionRetentionPolicy spec, resolves workspace references,
// syncs a ConfigMap, and sets status conditions accordingly.
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

	// Handle deletion — clean up ConfigMap before removing finalizer
	if !policy.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(policy, retentionPolicyFinalizer) {
			r.emitEvent(policy, corev1.EventTypeNormal, RetentionEventReasonDeleting, "Cleaning up retention policy resources")
			if err := r.deleteRetentionConfigMap(ctx, policy); err != nil {
				log.Error(err, "Failed to delete retention ConfigMap during cleanup")
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(policy, retentionPolicyFinalizer)
			if err := r.Update(ctx, policy); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Ensure finalizer is present
	if !controllerutil.ContainsFinalizer(policy, retentionPolicyFinalizer) {
		controllerutil.AddFinalizer(policy, retentionPolicyFinalizer)
		if err := r.Update(ctx, policy); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate the policy spec
	if err := r.validatePolicy(policy); err != nil {
		SetCondition(&policy.Status.Conditions, policy.Generation, RetentionConditionTypePolicyValid, metav1.ConditionFalse,
			"ValidationFailed", err.Error())
		SetCondition(&policy.Status.Conditions, policy.Generation, RetentionConditionTypeReady, metav1.ConditionFalse,
			"ValidationFailed", "Policy validation failed")
		r.emitEvent(policy, corev1.EventTypeWarning, RetentionEventReasonValidationFailed, err.Error())
		policy.Status.Phase = omniav1alpha1.SessionRetentionPolicyPhaseError
		policy.Status.ObservedGeneration = policy.Generation
		if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		if r.Metrics != nil {
			r.Metrics.RecordReconcileError(policy.Name, "validation")
		}
		return ctrl.Result{}, err
	}
	SetCondition(&policy.Status.Conditions, policy.Generation, RetentionConditionTypePolicyValid, metav1.ConditionTrue,
		"Valid", "Policy spec is valid")
	r.emitEvent(policy, corev1.EventTypeNormal, RetentionEventReasonValidated, "Policy spec validated successfully")

	// Workspace binding moved to Workspace.spec.services[].session.policyRef
	// — the policy itself no longer tracks consumers. The condition stays
	// for backward observability, always reports True.
	SetCondition(&policy.Status.Conditions, policy.Generation, RetentionConditionTypeWorkspacesResolved, metav1.ConditionTrue,
		"NotApplicable", "Workspace binding is now via Workspace.spec.services[].session.policyRef")

	// Sync ConfigMap
	if r.Namespace != "" {
		if err := r.reconcileRetentionConfigMap(ctx, policy); err != nil {
			SetCondition(&policy.Status.Conditions, policy.Generation, RetentionConditionTypeReady, metav1.ConditionFalse,
				"ConfigSyncFailed", "Failed to sync retention ConfigMap")
			r.emitEvent(policy, corev1.EventTypeWarning, RetentionEventReasonConfigSyncFailed, err.Error())
			policy.Status.Phase = omniav1alpha1.SessionRetentionPolicyPhaseError
			policy.Status.ObservedGeneration = policy.Generation
			policy.Status.WorkspaceCount = 0
			if statusErr := r.Status().Update(ctx, policy); statusErr != nil {
				log.Error(statusErr, logMsgFailedToUpdateStatus)
			}
			if r.Metrics != nil {
				r.Metrics.RecordConfigMapSyncError(policy.Name)
				r.Metrics.RecordReconcileError(policy.Name, "configmap_sync")
			}
			return ctrl.Result{}, err
		}
		r.emitEvent(policy, corev1.EventTypeNormal, RetentionEventReasonConfigSynced, "Retention ConfigMap synced successfully")
	}

	// Set Ready condition — all sub-conditions passed
	SetCondition(&policy.Status.Conditions, policy.Generation, RetentionConditionTypeReady, metav1.ConditionTrue,
		"AllChecksPass", "Policy is valid and config synced")

	// Set final status
	policy.Status.Phase = omniav1alpha1.SessionRetentionPolicyPhaseActive
	policy.Status.ObservedGeneration = policy.Generation
	policy.Status.WorkspaceCount = 0

	if err := r.Status().Update(ctx, policy); err != nil {
		log.Error(err, "Failed to update SessionRetentionPolicy status")
		return ctrl.Result{}, err
	}

	r.emitEvent(policy, corev1.EventTypeNormal, RetentionEventReasonActive, "Policy is active")

	// Record metrics
	if r.Metrics != nil {
		r.Metrics.ActivePolicies.Inc()
		r.Metrics.SetWorkspaceOverrides(policy.Name, 0)
	}

	log.Info("Successfully reconciled SessionRetentionPolicy", "name", req.Name, "phase", policy.Status.Phase)
	return ctrl.Result{}, nil
}

// emitEvent emits a Kubernetes event if a Recorder is available.
func (r *SessionRetentionPolicyReconciler) emitEvent(obj runtime.Object, eventType, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(obj, eventType, reason, message)
	}
}

// retentionConfigMapName returns the ConfigMap name for a given policy.
func retentionConfigMapName(policyName string) string {
	return "retention-policy-" + policyName
}

// reconcileRetentionConfigMap creates or updates a ConfigMap with the resolved retention config.
func (r *SessionRetentionPolicyReconciler) reconcileRetentionConfigMap(ctx context.Context, policy *omniav1alpha1.SessionRetentionPolicy) error {
	log := logf.FromContext(ctx)

	resolved := r.buildResolvedConfig(policy)
	data, err := yaml.Marshal(resolved)
	if err != nil {
		return fmt.Errorf("failed to marshal retention config: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      retentionConfigMapName(policy.Name),
			Namespace: r.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		configMap.Labels = map[string]string{
			labelAppManagedBy: labelValueOmniaOperator,
			labelOmniaComp:    "retention-config",
		}
		configMap.Data = map[string]string{
			"retention.yaml": string(data),
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to reconcile retention ConfigMap: %w", err)
	}

	log.V(1).Info("Reconciled retention ConfigMap", "name", configMap.Name, "result", result)
	return nil
}

// buildResolvedConfig constructs the resolved retention config from the policy spec.
func (r *SessionRetentionPolicyReconciler) buildResolvedConfig(policy *omniav1alpha1.SessionRetentionPolicy) ResolvedRetentionConfig {
	return ResolvedRetentionConfig{
		HotCache:    policy.Spec.HotCache,
		WarmStore:   policy.Spec.WarmStore,
		ColdArchive: policy.Spec.ColdArchive,
	}
}

// deleteRetentionConfigMap deletes the ConfigMap associated with the policy.
func (r *SessionRetentionPolicyReconciler) deleteRetentionConfigMap(ctx context.Context, policy *omniav1alpha1.SessionRetentionPolicy) error {
	if r.Namespace == "" {
		return nil
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      retentionConfigMapName(policy.Name),
			Namespace: r.Namespace,
		},
	}

	if err := r.Delete(ctx, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete retention ConfigMap: %w", err)
	}

	return nil
}

// validatePolicy validates the SessionRetentionPolicy spec.
func (r *SessionRetentionPolicyReconciler) validatePolicy(policy *omniav1alpha1.SessionRetentionPolicy) error {
	// Validate hot cache TTL if configured
	if policy.Spec.HotCache != nil && policy.Spec.HotCache.TTLAfterInactive != "" {
		if _, err := time.ParseDuration(policy.Spec.HotCache.TTLAfterInactive); err != nil {
			return fmt.Errorf("invalid hot cache TTL %q: %w", policy.Spec.HotCache.TTLAfterInactive, err)
		}
	}

	// Validate cold archive: retentionDays is required when enabled
	if policy.Spec.ColdArchive != nil && policy.Spec.ColdArchive.Enabled {
		if policy.Spec.ColdArchive.RetentionDays == nil || *policy.Spec.ColdArchive.RetentionDays <= 0 {
			return fmt.Errorf("cold archive retentionDays is required when cold archive is enabled")
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SessionRetentionPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		For(&omniav1alpha1.SessionRetentionPolicy{}).
		Named("sessionretentionpolicy").
		Complete(r)
}
