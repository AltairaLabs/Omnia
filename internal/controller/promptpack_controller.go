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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// PromptPack condition types
const (
	PromptPackConditionTypeSourceValid    = "SourceValid"
	PromptPackConditionTypeAgentsNotified = "AgentsNotified"
)

// PromptPackReconciler reconciles a PromptPack object
type PromptPackReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=promptpacks/finalizers,verbs=update
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *PromptPackReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling PromptPack", "name", req.Name, "namespace", req.Namespace)

	// Fetch the PromptPack instance
	promptPack := &omniav1alpha1.PromptPack{}
	if err := r.Get(ctx, req.NamespacedName, promptPack); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PromptPack resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get PromptPack")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if promptPack.Status.Phase == "" {
		promptPack.Status.Phase = omniav1alpha1.PromptPackPhasePending
	}

	// Validate the source configuration
	if err := r.validateSource(ctx, promptPack); err != nil {
		r.setCondition(promptPack, PromptPackConditionTypeSourceValid, metav1.ConditionFalse,
			"SourceValidationFailed", err.Error())
		promptPack.Status.Phase = omniav1alpha1.PromptPackPhaseFailed
		if statusErr := r.Status().Update(ctx, promptPack); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
	}
	r.setCondition(promptPack, PromptPackConditionTypeSourceValid, metav1.ConditionTrue,
		"SourceValid", "Source configuration is valid")

	// Find all AgentRuntimes referencing this PromptPack
	referencingRuntimes, err := r.findReferencingAgentRuntimes(ctx, promptPack)
	if err != nil {
		log.Error(err, "Failed to find referencing AgentRuntimes")
		return ctrl.Result{}, err
	}
	log.V(1).Info("Found referencing AgentRuntimes", "count", len(referencingRuntimes))

	// Update status based on rollout strategy
	r.updateRolloutStatus(promptPack, referencingRuntimes)

	// Set notification condition
	if len(referencingRuntimes) > 0 {
		r.setCondition(promptPack, PromptPackConditionTypeAgentsNotified, metav1.ConditionTrue,
			"AgentsNotified", fmt.Sprintf("Notified %d AgentRuntime(s)", len(referencingRuntimes)))
	} else {
		r.setCondition(promptPack, PromptPackConditionTypeAgentsNotified, metav1.ConditionTrue,
			"NoAgentsToNotify", "No AgentRuntimes reference this PromptPack")
	}

	// Update last updated timestamp
	now := metav1.Now()
	promptPack.Status.LastUpdated = &now

	if err := r.Status().Update(ctx, promptPack); err != nil {
		log.Error(err, "Failed to update PromptPack status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// validateSource validates the source configuration for the PromptPack.
func (r *PromptPackReconciler) validateSource(ctx context.Context, promptPack *omniav1alpha1.PromptPack) error {
	switch promptPack.Spec.Source.Type {
	case omniav1alpha1.PromptPackSourceTypeConfigMap:
		return r.validateConfigMapSource(ctx, promptPack)
	default:
		return fmt.Errorf("unsupported source type: %s", promptPack.Spec.Source.Type)
	}
}

// validateConfigMapSource validates that the referenced ConfigMap exists.
func (r *PromptPackReconciler) validateConfigMapSource(ctx context.Context, promptPack *omniav1alpha1.PromptPack) error {
	if promptPack.Spec.Source.ConfigMapRef == nil {
		return fmt.Errorf("configMapRef is required when source type is configmap")
	}

	configMap := &corev1.ConfigMap{}
	key := types.NamespacedName{
		Name:      promptPack.Spec.Source.ConfigMapRef.Name,
		Namespace: promptPack.Namespace,
	}

	if err := r.Get(ctx, key, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("ConfigMap %q not found in namespace %q", key.Name, key.Namespace)
		}
		return fmt.Errorf("failed to get ConfigMap %q: %w", key.Name, err)
	}

	// Validate that the ConfigMap has at least some data
	if len(configMap.Data) == 0 && len(configMap.BinaryData) == 0 {
		return fmt.Errorf("ConfigMap %q is empty", key.Name)
	}

	return nil
}

// findReferencingAgentRuntimes finds all AgentRuntimes that reference this PromptPack.
func (r *PromptPackReconciler) findReferencingAgentRuntimes(ctx context.Context, promptPack *omniav1alpha1.PromptPack) ([]omniav1alpha1.AgentRuntime, error) {
	// List all AgentRuntimes in the same namespace
	agentRuntimeList := &omniav1alpha1.AgentRuntimeList{}
	if err := r.List(ctx, agentRuntimeList, client.InNamespace(promptPack.Namespace)); err != nil {
		return nil, fmt.Errorf("failed to list AgentRuntimes: %w", err)
	}

	var referencingRuntimes []omniav1alpha1.AgentRuntime
	for _, ar := range agentRuntimeList.Items {
		if ar.Spec.PromptPackRef.Name == promptPack.Name {
			// Check version matching if specified
			if ar.Spec.PromptPackRef.Version != nil {
				if *ar.Spec.PromptPackRef.Version == promptPack.Spec.Version {
					referencingRuntimes = append(referencingRuntimes, ar)
				}
			} else {
				// No specific version, matches by name only
				referencingRuntimes = append(referencingRuntimes, ar)
			}
		}
	}

	return referencingRuntimes, nil
}

// updateRolloutStatus updates the status based on rollout strategy.
// The referencingRuntimes parameter is used to track how many agents will be affected.
func (r *PromptPackReconciler) updateRolloutStatus(promptPack *omniav1alpha1.PromptPack, referencingRuntimes []omniav1alpha1.AgentRuntime) {
	version := promptPack.Spec.Version
	_ = len(referencingRuntimes) // Track affected agents count for future metrics

	switch promptPack.Spec.Rollout.Type {
	case omniav1alpha1.RolloutStrategyImmediate:
		// Immediate rollout: set as active version
		promptPack.Status.Phase = omniav1alpha1.PromptPackPhaseActive
		promptPack.Status.ActiveVersion = &version
		promptPack.Status.CanaryVersion = nil
		promptPack.Status.CanaryWeight = nil

	case omniav1alpha1.RolloutStrategyCanary:
		// Canary rollout: track canary weight
		if promptPack.Spec.Rollout.Canary != nil {
			weight := promptPack.Spec.Rollout.Canary.Weight
			promptPack.Status.Phase = omniav1alpha1.PromptPackPhaseCanary
			promptPack.Status.CanaryVersion = &version
			promptPack.Status.CanaryWeight = &weight

			// If weight reaches 100%, promote to active
			if weight >= 100 {
				promptPack.Status.Phase = omniav1alpha1.PromptPackPhaseActive
				promptPack.Status.ActiveVersion = &version
				promptPack.Status.CanaryVersion = nil
				promptPack.Status.CanaryWeight = nil
			}
		}
	}
}

// setCondition sets a condition on the PromptPack status.
func (r *PromptPackReconciler) setCondition(
	promptPack *omniav1alpha1.PromptPack,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&promptPack.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: promptPack.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// findPromptPacksForConfigMap maps a ConfigMap to PromptPacks that reference it.
func (r *PromptPackReconciler) findPromptPacksForConfigMap(ctx context.Context, obj client.Object) []reconcile.Request {
	configMap := obj.(*corev1.ConfigMap)
	log := logf.FromContext(ctx)

	promptPackList := &omniav1alpha1.PromptPackList{}
	if err := r.List(ctx, promptPackList, client.InNamespace(configMap.Namespace)); err != nil {
		log.Error(err, "Failed to list PromptPacks for ConfigMap mapping")
		return nil
	}

	var requests []reconcile.Request
	for _, pp := range promptPackList.Items {
		if pp.Spec.Source.Type == omniav1alpha1.PromptPackSourceTypeConfigMap &&
			pp.Spec.Source.ConfigMapRef != nil &&
			pp.Spec.Source.ConfigMapRef.Name == configMap.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      pp.Name,
					Namespace: pp.Namespace,
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *PromptPackReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.PromptPack{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findPromptPacksForConfigMap),
		).
		Named("promptpack").
		Complete(r)
}
