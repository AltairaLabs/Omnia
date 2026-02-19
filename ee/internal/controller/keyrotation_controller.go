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
	"time"

	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/encryption"
)

const (
	// Annotation that triggers an immediate key rotation.
	rotateKeyAnnotation = "omnia.altairalabs.ai/rotate-key"

	// Condition types for KeyRotation.
	conditionTypeKeyRotationReady = "KeyRotationReady"

	// Event reasons.
	eventReasonKeyRotated          = "KeyRotated"
	eventReasonKeyRotationFailed   = "KeyRotationFailed"
	eventReasonReEncryptionStarted = "ReEncryptionStarted"
	eventReasonReEncryptionBatch   = "ReEncryptionBatch"

	// Default batch size for re-encryption.
	defaultBatchSize = 100

	// Re-encryption batch requeue delay.
	reEncryptionRequeueDelay = 5 * time.Second
)

// KeyRotationReconciler reconciles SessionPrivacyPolicy objects for key rotation.
type KeyRotationReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	Recorder        record.EventRecorder
	ProviderFactory func(cfg encryption.ProviderConfig) (encryption.Provider, error)
	StoreFactory    func() (encryption.ReEncryptionStore, error)
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionprivacypolicies,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionprivacypolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile handles key rotation for SessionPrivacyPolicy resources.
func (r *KeyRotationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling key rotation", "name", req.Name)

	policy, err := r.fetchPolicy(ctx, req)
	if err != nil || policy == nil {
		return ctrl.Result{}, err
	}

	if !r.isKeyRotationEnabled(policy) {
		return ctrl.Result{}, nil
	}

	// Handle in-progress re-encryption first.
	if r.isReEncryptionInProgress(policy) {
		return r.processReEncryptionBatch(ctx, policy)
	}

	// Handle annotation-triggered immediate rotation.
	if r.hasRotateAnnotation(policy) {
		return r.handleAnnotationRotation(ctx, policy)
	}

	// Handle scheduled rotation.
	return r.handleScheduledRotation(ctx, policy)
}

// fetchPolicy retrieves the SessionPrivacyPolicy, returning nil if not found.
func (r *KeyRotationReconciler) fetchPolicy(
	ctx context.Context, req ctrl.Request,
) (*omniav1alpha1.SessionPrivacyPolicy, error) {
	policy := &omniav1alpha1.SessionPrivacyPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return policy, nil
}

// isKeyRotationEnabled checks if key rotation is configured and enabled.
func (r *KeyRotationReconciler) isKeyRotationEnabled(policy *omniav1alpha1.SessionPrivacyPolicy) bool {
	return policy.Spec.Encryption != nil &&
		policy.Spec.Encryption.Enabled &&
		policy.Spec.Encryption.KeyRotation != nil &&
		policy.Spec.Encryption.KeyRotation.Enabled
}

// isReEncryptionInProgress checks if a re-encryption operation is currently running.
func (r *KeyRotationReconciler) isReEncryptionInProgress(policy *omniav1alpha1.SessionPrivacyPolicy) bool {
	return policy.Status.KeyRotation != nil &&
		policy.Status.KeyRotation.ReEncryptionProgress != nil &&
		policy.Status.KeyRotation.ReEncryptionProgress.Status == "InProgress"
}

// hasRotateAnnotation checks for the immediate rotation trigger annotation.
func (r *KeyRotationReconciler) hasRotateAnnotation(policy *omniav1alpha1.SessionPrivacyPolicy) bool {
	return policy.Annotations != nil && policy.Annotations[rotateKeyAnnotation] == "true"
}

// handleAnnotationRotation processes an annotation-triggered immediate rotation.
func (r *KeyRotationReconciler) handleAnnotationRotation(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("annotation-triggered key rotation")

	if err := r.executeRotation(ctx, policy); err != nil {
		return ctrl.Result{}, err
	}

	// Remove the annotation.
	delete(policy.Annotations, rotateKeyAnnotation)
	if err := r.Update(ctx, policy); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing rotate annotation: %w", err)
	}

	return r.maybeStartReEncryption(ctx, policy)
}

// handleScheduledRotation checks if a scheduled rotation is due and executes it.
func (r *KeyRotationReconciler) handleScheduledRotation(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) (ctrl.Result, error) {
	schedule := policy.Spec.Encryption.KeyRotation.Schedule
	if schedule == "" {
		return ctrl.Result{}, nil
	}

	nextRun, err := r.calculateNextRotation(policy)
	if err != nil {
		r.recordKeyRotationEvent(policy, corev1.EventTypeWarning, eventReasonKeyRotationFailed,
			fmt.Sprintf("invalid cron schedule: %v", err))
		return ctrl.Result{}, nil
	}

	now := time.Now()
	if now.Before(nextRun) {
		// Not due yet, requeue at next run time.
		return ctrl.Result{RequeueAfter: nextRun.Sub(now)}, nil
	}

	// Rotation is due.
	if err := r.executeRotation(ctx, policy); err != nil {
		return ctrl.Result{}, err
	}

	return r.maybeStartReEncryption(ctx, policy)
}

// calculateNextRotation determines when the next rotation should occur.
func (r *KeyRotationReconciler) calculateNextRotation(
	policy *omniav1alpha1.SessionPrivacyPolicy,
) (time.Time, error) {
	sched, err := cron.ParseStandard(policy.Spec.Encryption.KeyRotation.Schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing cron schedule: %w", err)
	}

	// If never rotated, rotate immediately.
	if policy.Status.KeyRotation == nil || policy.Status.KeyRotation.LastRotatedAt == nil {
		return time.Time{}, nil
	}

	return sched.Next(policy.Status.KeyRotation.LastRotatedAt.Time), nil
}

// executeRotation performs the actual key rotation via the KMS provider.
func (r *KeyRotationReconciler) executeRotation(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) error {
	log := logf.FromContext(ctx)

	providerCfg, err := r.buildProviderConfig(ctx, policy)
	if err != nil {
		r.setRotationError(ctx, policy, fmt.Sprintf("building provider config: %v", err))
		return err
	}

	provider, err := r.ProviderFactory(providerCfg)
	if err != nil {
		r.setRotationError(ctx, policy, fmt.Sprintf("creating provider: %v", err))
		return err
	}
	defer func() { _ = provider.Close() }()

	result, err := provider.RotateKey(ctx)
	if err != nil {
		r.setRotationError(ctx, policy, fmt.Sprintf("rotating key: %v", err))
		return err
	}

	log.Info("key rotated successfully",
		"previousVersion", result.PreviousKeyVersion,
		"newVersion", result.NewKeyVersion)

	r.updateRotationStatus(policy, result)

	if err := r.Status().Update(ctx, policy); err != nil {
		return fmt.Errorf("updating rotation status: %w", err)
	}

	r.recordKeyRotationEvent(policy, corev1.EventTypeNormal, eventReasonKeyRotated,
		fmt.Sprintf("Key rotated from version %s to %s", result.PreviousKeyVersion, result.NewKeyVersion))

	return nil
}

// updateRotationStatus updates the policy status after a successful rotation.
func (r *KeyRotationReconciler) updateRotationStatus(
	policy *omniav1alpha1.SessionPrivacyPolicy, result *encryption.KeyRotationResult,
) {
	now := metav1.Now()
	if policy.Status.KeyRotation == nil {
		policy.Status.KeyRotation = &omniav1alpha1.KeyRotationStatus{}
	}
	policy.Status.KeyRotation.LastRotatedAt = &now
	policy.Status.KeyRotation.CurrentKeyVersion = result.NewKeyVersion

	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               conditionTypeKeyRotationReady,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: policy.Generation,
		Reason:             eventReasonKeyRotated,
		Message:            fmt.Sprintf("Key rotated to version %s", result.NewKeyVersion),
		LastTransitionTime: now,
	})
}

// maybeStartReEncryption starts re-encryption if configured.
func (r *KeyRotationReconciler) maybeStartReEncryption(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) (ctrl.Result, error) {
	if !policy.Spec.Encryption.KeyRotation.ReEncryptExisting {
		return ctrl.Result{}, nil
	}

	return r.startReEncryption(ctx, policy)
}

// startReEncryption initiates the re-encryption process.
func (r *KeyRotationReconciler) startReEncryption(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) (ctrl.Result, error) {
	now := metav1.Now()
	if policy.Status.KeyRotation == nil {
		policy.Status.KeyRotation = &omniav1alpha1.KeyRotationStatus{}
	}
	policy.Status.KeyRotation.ReEncryptionProgress = &omniav1alpha1.ReEncryptionProgress{
		Status:    "InProgress",
		StartedAt: &now,
	}

	if err := r.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating re-encryption status: %w", err)
	}

	r.recordKeyRotationEvent(policy, corev1.EventTypeNormal, eventReasonReEncryptionStarted,
		"Re-encryption of existing data started")

	return ctrl.Result{RequeueAfter: reEncryptionRequeueDelay}, nil
}

// processReEncryptionBatch processes the next batch of messages for re-encryption.
func (r *KeyRotationReconciler) processReEncryptionBatch(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if r.StoreFactory == nil {
		return r.failReEncryption(ctx, policy, "store factory not configured")
	}

	store, err := r.StoreFactory()
	if err != nil {
		return r.failReEncryption(ctx, policy, fmt.Sprintf("creating store: %v", err))
	}

	providerCfg, err := r.buildProviderConfig(ctx, policy)
	if err != nil {
		return r.failReEncryption(ctx, policy, fmt.Sprintf("building provider config: %v", err))
	}

	provider, err := r.ProviderFactory(providerCfg)
	if err != nil {
		return r.failReEncryption(ctx, policy, fmt.Sprintf("creating provider: %v", err))
	}
	defer func() { _ = provider.Close() }()

	reEncryptor := encryption.NewMessageReEncryptor(provider, store)
	batchSize := r.getBatchSize(policy)

	lastID, hasMore, result, err := reEncryptor.ReEncryptBatch(ctx, encryption.ReEncryptionConfig{
		KeyID:         policy.Spec.Encryption.KeyID,
		NotKeyVersion: policy.Status.KeyRotation.CurrentKeyVersion,
		BatchSize:     batchSize,
	})
	if err != nil {
		return r.failReEncryption(ctx, policy, fmt.Sprintf("re-encryption batch failed: %v", err))
	}

	log.Info("re-encryption batch completed",
		"processed", result.MessagesProcessed,
		"errors", result.Errors,
		"hasMore", hasMore,
		"lastID", lastID)

	return r.updateReEncryptionProgress(ctx, policy, result, hasMore)
}

// updateReEncryptionProgress updates the re-encryption progress in the status.
func (r *KeyRotationReconciler) updateReEncryptionProgress(
	ctx context.Context,
	policy *omniav1alpha1.SessionPrivacyPolicy,
	result *encryption.ReEncryptionResult,
	hasMore bool,
) (ctrl.Result, error) {
	progress := policy.Status.KeyRotation.ReEncryptionProgress
	progress.MessagesProcessed += int64(result.MessagesProcessed)

	if !hasMore {
		now := metav1.Now()
		progress.Status = "Completed"
		progress.CompletedAt = &now

		r.recordKeyRotationEvent(policy, corev1.EventTypeNormal, eventReasonReEncryptionBatch,
			fmt.Sprintf("Re-encryption completed: %d messages processed", progress.MessagesProcessed))
	}

	if err := r.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating re-encryption progress: %w", err)
	}

	if hasMore {
		return ctrl.Result{RequeueAfter: reEncryptionRequeueDelay}, nil
	}
	return ctrl.Result{}, nil
}

// failReEncryption marks the re-encryption as failed.
//
//nolint:unparam // returns zero Result for consistency with other reconciler helpers
func (r *KeyRotationReconciler) failReEncryption(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy, message string,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(fmt.Errorf("re-encryption failed: %s", message), "re-encryption failed")

	if policy.Status.KeyRotation != nil && policy.Status.KeyRotation.ReEncryptionProgress != nil {
		policy.Status.KeyRotation.ReEncryptionProgress.Status = "Failed"
	}

	_ = r.Status().Update(ctx, policy)
	r.recordKeyRotationEvent(policy, corev1.EventTypeWarning, eventReasonKeyRotationFailed, message)
	return ctrl.Result{}, nil
}

// buildProviderConfig constructs an encryption.ProviderConfig from the policy and its secret.
func (r *KeyRotationReconciler) buildProviderConfig(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy,
) (encryption.ProviderConfig, error) {
	enc := policy.Spec.Encryption

	cfg := encryption.ProviderConfig{
		ProviderType: encryption.ProviderType(enc.KMSProvider),
		KeyID:        enc.KeyID,
	}

	if enc.SecretRef != nil {
		creds, err := r.loadSecretCredentials(ctx, enc.SecretRef.Name)
		if err != nil {
			return cfg, err
		}
		cfg.Credentials = creds
		if v, ok := creds["vault-url"]; ok {
			cfg.VaultURL = v
		}
	}

	return cfg, nil
}

// loadSecretCredentials loads credential data from a Kubernetes Secret.
func (r *KeyRotationReconciler) loadSecretCredentials(
	ctx context.Context, secretName string,
) (map[string]string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      secretName,
		Namespace: privacyPolicyNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("loading secret %q: %w", secretName, err)
	}

	creds := make(map[string]string, len(secret.Data))
	for k, v := range secret.Data {
		creds[k] = string(v)
	}
	return creds, nil
}

// getBatchSize returns the configured batch size or the default.
func (r *KeyRotationReconciler) getBatchSize(policy *omniav1alpha1.SessionPrivacyPolicy) int {
	if policy.Spec.Encryption.KeyRotation.BatchSize != nil {
		return int(*policy.Spec.Encryption.KeyRotation.BatchSize)
	}
	return defaultBatchSize
}

// setRotationError records a rotation error in events and conditions.
func (r *KeyRotationReconciler) setRotationError(
	ctx context.Context, policy *omniav1alpha1.SessionPrivacyPolicy, message string,
) {
	log := logf.FromContext(ctx)
	log.Error(fmt.Errorf("key rotation failed: %s", message), "key rotation failed")

	meta.SetStatusCondition(&policy.Status.Conditions, metav1.Condition{
		Type:               conditionTypeKeyRotationReady,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: policy.Generation,
		Reason:             eventReasonKeyRotationFailed,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
	_ = r.Status().Update(ctx, policy)
	r.recordKeyRotationEvent(policy, corev1.EventTypeWarning, eventReasonKeyRotationFailed, message)
}

// recordKeyRotationEvent emits a Kubernetes event.
func (r *KeyRotationReconciler) recordKeyRotationEvent(
	obj runtime.Object, eventType, reason, message string,
) {
	if r.Recorder != nil {
		r.Recorder.Event(obj, eventType, reason, message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *KeyRotationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.SessionPrivacyPolicy{}).
		Named("keyrotation").
		Complete(r)
}
