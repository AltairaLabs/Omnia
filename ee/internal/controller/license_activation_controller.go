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
	"time"

	corev1 "k8s.io/api/core/v1"
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

	"github.com/altairalabs/omnia/ee/pkg/license"
)

// Version is the current Omnia version, set at build time.
var Version = "dev"

// LicenseActivationReconciler reconciles license activation and heartbeats.
type LicenseActivationReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
	LicenseValidator *license.Validator
	ActivationClient *license.ActivationClient
	ClusterName      string
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile handles license activation and periodic heartbeats.
func (r *LicenseActivationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Only process the license secret
	if req.Name != license.LicenseSecretName || req.Namespace != license.LicenseSecretNamespace {
		return ctrl.Result{}, nil
	}

	// Get the license
	lic, err := r.LicenseValidator.GetLicense(ctx)
	if err != nil {
		log.V(1).Info("License not found or invalid, skipping activation", "error", err)
		return ctrl.Result{}, nil
	}

	// Open-core licenses don't need activation
	if lic.Tier == license.TierOpenCore {
		log.V(1).Info("Open-core license, skipping activation")
		return ctrl.Result{}, nil
	}

	// Check if already activated
	activationState, err := r.getActivationState(ctx)
	if err != nil && !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get activation state")
		return ctrl.Result{}, err
	}

	if activationState != nil {
		// Already activated - check if heartbeat is needed
		return r.handleHeartbeat(ctx, lic, activationState)
	}

	// Not activated - initiate activation
	return r.initiateActivation(ctx, lic)
}

// initiateActivation activates the license on the license server.
func (r *LicenseActivationReconciler) initiateActivation(ctx context.Context, lic *license.License) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Generate cluster fingerprint
	fingerprint, err := license.ClusterFingerprint(ctx, r.Client)
	if err != nil {
		log.Error(err, "Failed to generate cluster fingerprint")
		r.recordEvent(ctx, "Warning", "FingerprintFailed", fmt.Sprintf("Failed to generate cluster fingerprint: %v", err))
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	log.Info("Initiating license activation", "licenseID", lic.ID, "fingerprint", fingerprint)

	// Call activation API
	resp, err := r.ActivationClient.Activate(ctx, license.ActivationRequest{
		LicenseID:          lic.ID,
		ClusterFingerprint: fingerprint,
		ClusterName:        r.ClusterName,
		Version:            Version,
	})
	if err != nil {
		log.Error(err, "License activation failed")
		r.recordEvent(ctx, "Warning", "ActivationFailed", fmt.Sprintf("License activation failed: %v", err))
		// Requeue with exponential backoff
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if !resp.Activated {
		msg := fmt.Sprintf("License activation rejected: %s", resp.Message)
		if len(resp.ActiveClusters) > 0 {
			msg += fmt.Sprintf(" (Active clusters: %d/%d)", len(resp.ActiveClusters), resp.MaxActivations)
		}
		log.Info(msg)
		r.recordEvent(ctx, "Warning", "ActivationRejected", msg)
		// Don't requeue - operator action required
		return ctrl.Result{}, nil
	}

	// Store activation state
	activationState := &license.ActivationState{
		ActivationID:       resp.ActivationID,
		ClusterFingerprint: fingerprint,
		LicenseID:          lic.ID,
		ActivatedAt:        time.Now(),
		LastHeartbeat:      time.Now(),
	}

	if err := r.saveActivationState(ctx, activationState); err != nil {
		log.Error(err, "Failed to save activation state")
		return ctrl.Result{}, err
	}

	log.Info("License activated successfully", "activationID", resp.ActivationID)
	r.recordEvent(ctx, "Normal", "Activated", fmt.Sprintf("License activated successfully (ID: %s)", resp.ActivationID))

	// Requeue for heartbeat
	return ctrl.Result{RequeueAfter: license.DefaultHeartbeatInterval}, nil
}

// handleHeartbeat sends a heartbeat to the license server if needed.
func (r *LicenseActivationReconciler) handleHeartbeat(ctx context.Context, lic *license.License, state *license.ActivationState) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Check if heartbeat is needed
	if !state.NeedsHeartbeat(license.DefaultHeartbeatInterval) {
		// Calculate time until next heartbeat
		nextHeartbeat := time.Until(state.LastHeartbeat.Add(license.DefaultHeartbeatInterval))
		if nextHeartbeat < 0 {
			nextHeartbeat = time.Minute
		}
		return ctrl.Result{RequeueAfter: nextHeartbeat}, nil
	}

	log.V(1).Info("Sending license heartbeat", "licenseID", lic.ID)

	// Send heartbeat
	_, err := r.ActivationClient.Heartbeat(ctx, lic.ID, license.HeartbeatRequest{
		ClusterFingerprint: state.ClusterFingerprint,
		Version:            Version,
	})
	if err != nil {
		log.Error(err, "License heartbeat failed")
		state.HeartbeatFailures++

		// Check grace period
		if !state.IsInGracePeriod() {
			r.recordEvent(ctx, "Warning", "HeartbeatGracePeriodExpired",
				"License heartbeat grace period expired. Enterprise features may be disabled.")
		}

		// Save updated failure count
		if saveErr := r.saveActivationState(ctx, state); saveErr != nil {
			log.Error(saveErr, "Failed to save activation state after heartbeat failure")
		}

		// Retry after 1 hour on failure
		return ctrl.Result{RequeueAfter: time.Hour}, nil
	}

	// Update heartbeat time and reset failures
	state.LastHeartbeat = time.Now()
	state.HeartbeatFailures = 0

	if err := r.saveActivationState(ctx, state); err != nil {
		log.Error(err, "Failed to save activation state after successful heartbeat")
		return ctrl.Result{}, err
	}

	log.V(1).Info("License heartbeat successful")

	// Requeue for next heartbeat
	return ctrl.Result{RequeueAfter: license.DefaultHeartbeatInterval}, nil
}

// getActivationState retrieves the activation state from the ConfigMap.
func (r *LicenseActivationReconciler) getActivationState(ctx context.Context) (*license.ActivationState, error) {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, cm)
	if err != nil {
		return nil, err
	}

	data, ok := cm.Data["state"]
	if !ok {
		return nil, fmt.Errorf("activation state not found in ConfigMap")
	}

	state := &license.ActivationState{}
	if err := json.Unmarshal([]byte(data), state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal activation state: %w", err)
	}

	return state, nil
}

// saveActivationState saves the activation state to a ConfigMap.
func (r *LicenseActivationReconciler) saveActivationState(ctx context.Context, state *license.ActivationState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal activation state: %w", err)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "omnia",
				"app.kubernetes.io/component": "license-activation",
			},
		},
		Data: map[string]string{
			"state": string(data),
		},
	}

	// Try to get existing ConfigMap
	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      license.ActivationConfigMapName,
		Namespace: license.LicenseSecretNamespace,
	}, existing)

	if apierrors.IsNotFound(err) {
		// Create new ConfigMap
		return r.Create(ctx, cm)
	} else if err != nil {
		return err
	}

	// Update existing ConfigMap
	existing.Data = cm.Data
	return r.Update(ctx, existing)
}

// recordEvent records an event against the license secret.
func (r *LicenseActivationReconciler) recordEvent(ctx context.Context, eventType, reason, message string) {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      license.LicenseSecretName,
		Namespace: license.LicenseSecretNamespace,
	}, secret)
	if err != nil {
		return
	}
	r.Recorder.Event(secret, eventType, reason, message)
}

// Deactivate removes the activation from the license server.
// This should be called when the license is being removed or the cluster is being decommissioned.
func (r *LicenseActivationReconciler) Deactivate(ctx context.Context) error {
	log := logf.FromContext(ctx)

	state, err := r.getActivationState(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Not activated, nothing to do
		}
		return err
	}

	log.Info("Deactivating license", "licenseID", state.LicenseID, "activationID", state.ActivationID)

	// Call deactivation API
	resp, err := r.ActivationClient.Deactivate(ctx, state.LicenseID, state.ClusterFingerprint)
	if err != nil {
		log.Error(err, "Failed to deactivate license on server")
		// Continue to delete local state anyway
	} else if !resp.Deactivated {
		log.Info("Deactivation response", "message", resp.Message)
	}

	// Delete activation ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.ActivationConfigMapName,
			Namespace: license.LicenseSecretNamespace,
		},
	}
	if err := r.Delete(ctx, cm); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete activation ConfigMap: %w", err)
	}

	log.Info("License deactivated successfully")
	r.recordEvent(ctx, "Normal", "Deactivated", "License deactivated successfully")

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LicenseActivationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("license-activation").
		// Watch the license Secret
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				// Only watch the license secret
				if obj.GetName() == license.LicenseSecretName &&
					obj.GetNamespace() == license.LicenseSecretNamespace {
					return []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Name:      obj.GetName(),
								Namespace: obj.GetNamespace(),
							},
						},
					}
				}
				return nil
			}),
		).
		// Also watch the activation ConfigMap
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				// If activation ConfigMap is modified, check license
				if obj.GetName() == license.ActivationConfigMapName &&
					obj.GetNamespace() == license.LicenseSecretNamespace {
					return []reconcile.Request{
						{
							NamespacedName: types.NamespacedName{
								Name:      license.LicenseSecretName,
								Namespace: license.LicenseSecretNamespace,
							},
						},
					}
				}
				return nil
			}),
		).
		Complete(r)
}
