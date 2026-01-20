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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/license"
)

// ArenaConfig condition types
const (
	ArenaConfigConditionTypeReady          = "Ready"
	ArenaConfigConditionTypeSourceResolved = "SourceResolved"
	ArenaConfigConditionTypeProvidersValid = "ProvidersValid"
	ArenaConfigConditionTypeToolRegsValid  = "ToolRegistriesValid"
)

// Event reasons for ArenaConfig
const (
	ArenaConfigEventReasonValidationStarted   = "ValidationStarted"
	ArenaConfigEventReasonValidationSucceeded = "ValidationSucceeded"
	ArenaConfigEventReasonValidationFailed    = "ValidationFailed"
	ArenaConfigEventReasonSourceResolved      = "SourceResolved"
	ArenaConfigEventReasonSourceNotReady      = "SourceNotReady"
	ArenaConfigEventReasonProviderNotFound    = "ProviderNotFound"
	ArenaConfigEventReasonProviderNotReady    = "ProviderNotReady"
	ArenaConfigEventReasonToolRegNotFound     = "ToolRegistryNotFound"
)

// ArenaConfigReconciler reconciles an ArenaConfig object
type ArenaConfigReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	// LicenseValidator validates license for scenario counts (defense in depth)
	LicenseValidator *license.Validator
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenaconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenaconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenaconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=arenasources,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=toolregistries,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ArenaConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling ArenaConfig", "name", req.Name, "namespace", req.Namespace)

	// Fetch the ArenaConfig instance
	config := &omniav1alpha1.ArenaConfig{}
	if err := r.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ArenaConfig resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ArenaConfig")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if config.Status.Phase == "" {
		config.Status.Phase = omniav1alpha1.ArenaConfigPhasePending
	}

	// Update observed generation
	config.Status.ObservedGeneration = config.Generation

	// Check if suspended
	if config.Spec.Suspend {
		log.Info("ArenaConfig is suspended, skipping validation")
		r.setCondition(config, ArenaConfigConditionTypeReady, metav1.ConditionFalse,
			"Suspended", "ArenaConfig validation is suspended")
		if err := r.Status().Update(ctx, config); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if r.Recorder != nil {
		r.Recorder.Event(config, corev1.EventTypeNormal, ArenaConfigEventReasonValidationStarted, "Started validating configuration")
	}

	// Resolve ArenaSource
	source, err := r.resolveSource(ctx, config)
	if err != nil {
		log.Error(err, "Failed to resolve ArenaSource")
		r.handleValidationError(ctx, config, ArenaConfigConditionTypeSourceResolved, err)
		return ctrl.Result{}, nil
	}

	// Check if source is ready
	if source.Status.Phase != omniav1alpha1.ArenaSourcePhaseReady {
		log.Info("ArenaSource is not ready", "source", config.Spec.SourceRef.Name, "phase", source.Status.Phase)
		r.setCondition(config, ArenaConfigConditionTypeSourceResolved, metav1.ConditionFalse,
			"SourceNotReady", fmt.Sprintf("ArenaSource %s is not ready (phase: %s)", config.Spec.SourceRef.Name, source.Status.Phase))
		config.Status.Phase = omniav1alpha1.ArenaConfigPhasePending
		if r.Recorder != nil {
			r.Recorder.Event(config, corev1.EventTypeWarning, ArenaConfigEventReasonSourceNotReady,
				fmt.Sprintf("ArenaSource %s is not ready", config.Spec.SourceRef.Name))
		}
		if err := r.Status().Update(ctx, config); err != nil {
			log.Error(err, "Failed to update status")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Update resolved source info
	config.Status.ResolvedSource = &omniav1alpha1.ResolvedSource{
		Revision: source.Status.Artifact.Revision,
		URL:      source.Status.Artifact.URL,
	}
	r.setCondition(config, ArenaConfigConditionTypeSourceResolved, metav1.ConditionTrue,
		"SourceResolved", fmt.Sprintf("ArenaSource %s resolved at revision %s", config.Spec.SourceRef.Name, source.Status.Artifact.Revision))

	// Resolve Providers
	resolvedProviders, err := r.resolveProviders(ctx, config)
	if err != nil {
		log.Error(err, "Failed to resolve Providers")
		r.handleValidationError(ctx, config, ArenaConfigConditionTypeProvidersValid, err)
		return ctrl.Result{}, nil
	}
	config.Status.ResolvedProviders = resolvedProviders
	r.setCondition(config, ArenaConfigConditionTypeProvidersValid, metav1.ConditionTrue,
		"ProvidersValid", fmt.Sprintf("All %d providers validated", len(resolvedProviders)))

	// Resolve ToolRegistries (if specified)
	if len(config.Spec.ToolRegistries) > 0 {
		if err := r.resolveToolRegistries(ctx, config); err != nil {
			log.Error(err, "Failed to resolve ToolRegistries")
			r.handleValidationError(ctx, config, ArenaConfigConditionTypeToolRegsValid, err)
			return ctrl.Result{}, nil
		}
		r.setCondition(config, ArenaConfigConditionTypeToolRegsValid, metav1.ConditionTrue,
			"ToolRegistriesValid", fmt.Sprintf("All %d tool registries validated", len(config.Spec.ToolRegistries)))
	}

	// License check for scenario count (defense in depth)
	if r.LicenseValidator != nil && config.Status.ResolvedSource != nil {
		scenarioCount := int(config.Status.ResolvedSource.ScenarioCount)
		if scenarioCount > 0 {
			if err := r.LicenseValidator.ValidateScenarioCount(ctx, scenarioCount); err != nil {
				log.Info("Scenario count exceeds license limit", "count", scenarioCount, "error", err)
				config.Status.Phase = omniav1alpha1.ArenaConfigPhaseInvalid
				r.setCondition(config, ArenaConfigConditionTypeReady, metav1.ConditionFalse,
					"LicenseViolation", err.Error())
				if r.Recorder != nil {
					r.Recorder.Event(config, corev1.EventTypeWarning, "LicenseViolation",
						fmt.Sprintf("Scenario count %d exceeds license limit", scenarioCount))
				}
				if statusErr := r.Status().Update(ctx, config); statusErr != nil {
					log.Error(statusErr, "Failed to update status")
				}
				return ctrl.Result{}, nil
			}
		}
	}

	// All validations passed - set Ready
	config.Status.Phase = omniav1alpha1.ArenaConfigPhaseReady
	now := metav1.Now()
	config.Status.LastValidatedAt = &now
	r.setCondition(config, ArenaConfigConditionTypeReady, metav1.ConditionTrue,
		"Ready", "ArenaConfig is valid and ready for jobs")

	if r.Recorder != nil {
		r.Recorder.Event(config, corev1.EventTypeNormal, ArenaConfigEventReasonValidationSucceeded,
			fmt.Sprintf("Configuration validated with %d providers", len(resolvedProviders)))
	}

	if err := r.Status().Update(ctx, config); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled ArenaConfig", "providers", len(resolvedProviders))
	return ctrl.Result{}, nil
}

// resolveSource fetches and validates the referenced ArenaSource.
func (r *ArenaConfigReconciler) resolveSource(ctx context.Context, config *omniav1alpha1.ArenaConfig) (*omniav1alpha1.ArenaSource, error) {
	source := &omniav1alpha1.ArenaSource{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      config.Spec.SourceRef.Name,
		Namespace: config.Namespace,
	}, source); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("ArenaSource %s not found", config.Spec.SourceRef.Name)
		}
		return nil, fmt.Errorf("failed to get ArenaSource %s: %w", config.Spec.SourceRef.Name, err)
	}

	// Verify source has an artifact
	if source.Status.Artifact == nil {
		return nil, fmt.Errorf("ArenaSource %s has no artifact", config.Spec.SourceRef.Name)
	}

	return source, nil
}

// resolveProviders fetches and validates all referenced Provider CRDs.
func (r *ArenaConfigReconciler) resolveProviders(ctx context.Context, config *omniav1alpha1.ArenaConfig) ([]string, error) {
	if len(config.Spec.Providers) == 0 {
		return nil, fmt.Errorf("at least one provider is required")
	}

	var resolvedProviders []string

	for _, provRef := range config.Spec.Providers {
		namespace := provRef.Namespace
		if namespace == "" {
			namespace = config.Namespace
		}

		provider := &omniav1alpha1.Provider{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      provRef.Name,
			Namespace: namespace,
		}, provider); err != nil {
			if apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("provider %s/%s not found", namespace, provRef.Name)
			}
			return nil, fmt.Errorf("failed to get provider %s/%s: %w", namespace, provRef.Name, err)
		}

		// Check if provider is ready
		if provider.Status.Phase != omniav1alpha1.ProviderPhaseReady && provider.Status.Phase != "" {
			return nil, fmt.Errorf("provider %s/%s is not ready (phase: %s)", namespace, provRef.Name, provider.Status.Phase)
		}

		// Add to resolved list with full reference
		if namespace == config.Namespace {
			resolvedProviders = append(resolvedProviders, provRef.Name)
		} else {
			resolvedProviders = append(resolvedProviders, fmt.Sprintf("%s/%s", namespace, provRef.Name))
		}
	}

	return resolvedProviders, nil
}

// resolveToolRegistries fetches and validates all referenced ToolRegistry CRDs.
func (r *ArenaConfigReconciler) resolveToolRegistries(ctx context.Context, config *omniav1alpha1.ArenaConfig) error {
	for _, regRef := range config.Spec.ToolRegistries {
		namespace := regRef.Namespace
		if namespace == "" {
			namespace = config.Namespace
		}

		registry := &omniav1alpha1.ToolRegistry{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      regRef.Name,
			Namespace: namespace,
		}, registry); err != nil {
			if apierrors.IsNotFound(err) {
				return fmt.Errorf("ToolRegistry %s/%s not found", namespace, regRef.Name)
			}
			return fmt.Errorf("failed to get ToolRegistry %s/%s: %w", namespace, regRef.Name, err)
		}

		// Check if registry is ready (has at least one tool or is in ready state)
		if registry.Status.Phase != omniav1alpha1.ToolRegistryPhaseReady && registry.Status.Phase != "" {
			return fmt.Errorf("ToolRegistry %s/%s is not ready (phase: %s)", namespace, regRef.Name, registry.Status.Phase)
		}
	}

	return nil
}

// handleValidationError handles errors during validation.
func (r *ArenaConfigReconciler) handleValidationError(ctx context.Context, config *omniav1alpha1.ArenaConfig, conditionType string, err error) {
	log := logf.FromContext(ctx)

	config.Status.Phase = omniav1alpha1.ArenaConfigPhaseInvalid
	r.setCondition(config, conditionType, metav1.ConditionFalse, "ValidationFailed", err.Error())
	r.setCondition(config, ArenaConfigConditionTypeReady, metav1.ConditionFalse,
		"ValidationFailed", err.Error())

	if r.Recorder != nil {
		r.Recorder.Event(config, corev1.EventTypeWarning, ArenaConfigEventReasonValidationFailed, err.Error())
	}

	if statusErr := r.Status().Update(ctx, config); statusErr != nil {
		log.Error(statusErr, "Failed to update status after validation error")
	}
}

// setCondition sets a condition on the ArenaConfig status.
func (r *ArenaConfigReconciler) setCondition(config *omniav1alpha1.ArenaConfig, conditionType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&config.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: config.Generation,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// findArenaConfigsForSource maps ArenaSource changes to ArenaConfig reconcile requests.
func (r *ArenaConfigReconciler) findArenaConfigsForSource(ctx context.Context, obj client.Object) []ctrl.Request {
	source, ok := obj.(*omniav1alpha1.ArenaSource)
	if !ok {
		return nil
	}

	// Find all ArenaConfigs in the same namespace that reference this source
	configList := &omniav1alpha1.ArenaConfigList{}
	if err := r.List(ctx, configList, client.InNamespace(source.Namespace)); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, config := range configList.Items {
		if config.Spec.SourceRef.Name == source.Name {
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      config.Name,
					Namespace: config.Namespace,
				},
			})
		}
	}

	return requests
}

// findArenaConfigsForProvider maps Provider changes to ArenaConfig reconcile requests.
func (r *ArenaConfigReconciler) findArenaConfigsForProvider(ctx context.Context, obj client.Object) []ctrl.Request {
	provider, ok := obj.(*omniav1alpha1.Provider)
	if !ok {
		return nil
	}

	// Find all ArenaConfigs that reference this provider
	configList := &omniav1alpha1.ArenaConfigList{}
	if err := r.List(ctx, configList); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for _, config := range configList.Items {
		for _, provRef := range config.Spec.Providers {
			namespace := provRef.Namespace
			if namespace == "" {
				namespace = config.Namespace
			}
			if provRef.Name == provider.Name && namespace == provider.Namespace {
				requests = append(requests, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      config.Name,
						Namespace: config.Namespace,
					},
				})
				break
			}
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *ArenaConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.ArenaConfig{}).
		Watches(
			&omniav1alpha1.ArenaSource{},
			handler.EnqueueRequestsFromMapFunc(r.findArenaConfigsForSource),
		).
		Watches(
			&omniav1alpha1.Provider{},
			handler.EnqueueRequestsFromMapFunc(r.findArenaConfigsForProvider),
		).
		Named("arenaconfig").
		Complete(r)
}
