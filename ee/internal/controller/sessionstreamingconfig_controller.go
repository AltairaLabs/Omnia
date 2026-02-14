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
	"sync"

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
	// Condition types for SessionStreamingConfig.
	conditionTypeProviderConfigured = "ProviderConfigured"
	conditionTypeStreamingReady     = "Ready"

	// Event reasons for SessionStreamingConfig.
	eventReasonConfigValidated       = "ConfigValidated"
	eventReasonProviderConfigured    = "ProviderConfigured"
	eventReasonProviderConfigInvalid = "ProviderConfigInvalid"
	eventReasonPublisherCreated      = "PublisherCreated"
	eventReasonPublisherClosed       = "PublisherClosed"
	eventReasonPublisherError        = "PublisherError"
	eventReasonStreamingDisabled     = "StreamingDisabled"
)

// StreamingPublisher defines the interface for publishing session events to a streaming provider.
type StreamingPublisher interface {
	Close() error
}

// PublisherFactory creates a StreamingPublisher from a Kafka configuration.
type PublisherFactory func(cfg *corev1alpha1.KafkaConfig) (StreamingPublisher, error)

// SessionStreamingConfigReconciler reconciles a SessionStreamingConfig object.
type SessionStreamingConfigReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Recorder         record.EventRecorder
	PublisherFactory PublisherFactory

	mu        sync.Mutex
	publisher StreamingPublisher
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionstreamingconfigs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=sessionstreamingconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile handles SessionStreamingConfig reconciliation.
func (r *SessionStreamingConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling SessionStreamingConfig", "name", req.Name)

	config := &corev1alpha1.SessionStreamingConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("SessionStreamingConfig deleted, cleaning up publisher")
			r.closePublisher()
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	config.Status.ObservedGeneration = config.Generation

	if !config.Spec.Enabled {
		return r.reconcileDisabled(ctx, config)
	}

	return r.reconcileEnabled(ctx, config)
}

// reconcileDisabled handles reconciliation when streaming is disabled.
func (r *SessionStreamingConfigReconciler) reconcileDisabled(
	ctx context.Context, config *corev1alpha1.SessionStreamingConfig,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	r.closePublisher()

	r.setStreamingCondition(config, conditionTypeProviderConfigured, metav1.ConditionTrue,
		eventReasonStreamingDisabled, "streaming is disabled, no provider configured")
	r.setStreamingCondition(config, conditionTypeStreamingReady, metav1.ConditionTrue,
		eventReasonStreamingDisabled, "streaming is disabled")
	config.Status.Phase = corev1alpha1.SessionStreamingConfigPhaseActive
	config.Status.Connected = false

	if err := r.Status().Update(ctx, config); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("streaming is disabled", "name", config.Name)
	return ctrl.Result{}, nil
}

// reconcileEnabled handles reconciliation when streaming is enabled.
func (r *SessionStreamingConfigReconciler) reconcileEnabled(
	ctx context.Context, config *corev1alpha1.SessionStreamingConfig,
) (ctrl.Result, error) {
	if err := r.validateProviderConfig(config); err != nil {
		return r.handleValidationError(ctx, config, err)
	}

	r.setStreamingCondition(config, conditionTypeProviderConfigured, metav1.ConditionTrue,
		eventReasonProviderConfigured, fmt.Sprintf("provider %s configuration is valid", config.Spec.Provider))

	if err := r.ensurePublisher(config); err != nil {
		return r.handlePublisherError(ctx, config, err)
	}

	return r.setSuccessStreamingStatus(ctx, config)
}

// validateProviderConfig checks that the provider-specific configuration section exists.
func (r *SessionStreamingConfigReconciler) validateProviderConfig(
	config *corev1alpha1.SessionStreamingConfig,
) error {
	switch config.Spec.Provider {
	case corev1alpha1.StreamingProviderKafka:
		if config.Spec.Kafka == nil {
			return fmt.Errorf("kafka configuration is required when provider is kafka")
		}
	case corev1alpha1.StreamingProviderKinesis:
		if config.Spec.Kinesis == nil {
			return fmt.Errorf("kinesis configuration is required when provider is kinesis")
		}
	case corev1alpha1.StreamingProviderPulsar:
		if config.Spec.Pulsar == nil {
			return fmt.Errorf("pulsar configuration is required when provider is pulsar")
		}
	case corev1alpha1.StreamingProviderNATS:
		if config.Spec.NATS == nil {
			return fmt.Errorf("nats configuration is required when provider is nats")
		}
	default:
		return fmt.Errorf("unsupported streaming provider: %s", config.Spec.Provider)
	}
	return nil
}

// ensurePublisher creates or recreates the streaming publisher for Kafka.
func (r *SessionStreamingConfigReconciler) ensurePublisher(
	config *corev1alpha1.SessionStreamingConfig,
) error {
	if config.Spec.Provider != corev1alpha1.StreamingProviderKafka {
		return nil
	}
	if r.PublisherFactory == nil {
		return nil
	}

	r.closePublisher()

	publisher, err := r.PublisherFactory(config.Spec.Kafka)
	if err != nil {
		return fmt.Errorf("failed to create kafka publisher: %w", err)
	}

	r.mu.Lock()
	r.publisher = publisher
	r.mu.Unlock()

	return nil
}

// handleValidationError sets error status when provider config validation fails.
func (r *SessionStreamingConfigReconciler) handleValidationError(
	ctx context.Context, config *corev1alpha1.SessionStreamingConfig, err error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(err, "provider configuration validation failed")

	r.setStreamingCondition(config, conditionTypeProviderConfigured, metav1.ConditionFalse,
		eventReasonProviderConfigInvalid, err.Error())
	r.setStreamingCondition(config, conditionTypeStreamingReady, metav1.ConditionFalse,
		eventReasonProviderConfigInvalid, "provider configuration is invalid")
	config.Status.Phase = corev1alpha1.SessionStreamingConfigPhaseError
	config.Status.Connected = false
	r.recordStreamingEvent(config, "Warning", eventReasonProviderConfigInvalid, err.Error())

	if statusErr := r.Status().Update(ctx, config); statusErr != nil {
		log.Error(statusErr, "failed to update error status")
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, nil
}

// handlePublisherError sets error status when publisher creation fails.
func (r *SessionStreamingConfigReconciler) handlePublisherError(
	ctx context.Context, config *corev1alpha1.SessionStreamingConfig, err error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(err, "failed to create publisher")

	r.setStreamingCondition(config, conditionTypeStreamingReady, metav1.ConditionFalse,
		eventReasonPublisherError, err.Error())
	config.Status.Phase = corev1alpha1.SessionStreamingConfigPhaseError
	config.Status.Connected = false
	r.recordStreamingEvent(config, "Warning", eventReasonPublisherError, err.Error())

	if statusErr := r.Status().Update(ctx, config); statusErr != nil {
		log.Error(statusErr, "failed to update error status")
		return ctrl.Result{}, statusErr
	}
	return ctrl.Result{}, err
}

// setSuccessStreamingStatus sets the success status on the config.
func (r *SessionStreamingConfigReconciler) setSuccessStreamingStatus(
	ctx context.Context, config *corev1alpha1.SessionStreamingConfig,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	r.setStreamingCondition(config, conditionTypeStreamingReady, metav1.ConditionTrue,
		eventReasonConfigValidated, "streaming configuration is active")
	config.Status.Phase = corev1alpha1.SessionStreamingConfigPhaseActive
	config.Status.Connected = true
	r.recordStreamingEvent(config, "Normal", eventReasonPublisherCreated,
		fmt.Sprintf("Streaming publisher configured for provider %s", config.Spec.Provider))

	if err := r.Status().Update(ctx, config); err != nil {
		log.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("successfully reconciled SessionStreamingConfig",
		"name", config.Name, "provider", config.Spec.Provider, "phase", config.Status.Phase)
	return ctrl.Result{}, nil
}

// closePublisher gracefully shuts down the current publisher if one exists.
func (r *SessionStreamingConfigReconciler) closePublisher() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.publisher != nil {
		_ = r.publisher.Close()
		r.publisher = nil
	}
}

// setStreamingCondition sets a condition on the SessionStreamingConfig status.
func (r *SessionStreamingConfigReconciler) setStreamingCondition(
	config *corev1alpha1.SessionStreamingConfig,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: config.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// recordStreamingEvent emits a Kubernetes event if the recorder is available.
func (r *SessionStreamingConfigReconciler) recordStreamingEvent(
	obj runtime.Object, eventType, reason, message string,
) {
	if r.Recorder != nil {
		r.Recorder.Event(obj, eventType, reason, message)
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SessionStreamingConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1alpha1.SessionStreamingConfig{}).
		Named("sessionstreamingconfig").
		Complete(r)
}
