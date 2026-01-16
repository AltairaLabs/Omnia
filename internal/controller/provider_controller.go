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

// Provider condition types
const (
	ProviderConditionTypeCredentialsValid = "CredentialsValid"
	ProviderConditionTypeSecretFound      = "SecretFound"
	// secretKeyAPIKey is the common secret key name for API keys.
	secretKeyAPIKey = "api-key"
)

// ProviderReconciler reconciles a Provider object
type ProviderReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProviderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconciling Provider", "name", req.Name, "namespace", req.Namespace)

	// Fetch the Provider instance
	provider := &omniav1alpha1.Provider{}
	if err := r.Get(ctx, req.NamespacedName, provider); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Provider resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Provider")
		return ctrl.Result{}, err
	}

	// Initialize status if needed
	if provider.Status.Phase == "" {
		provider.Status.Phase = omniav1alpha1.ProviderPhaseReady
	}

	// Validate the secret reference (if specified)
	if provider.Spec.SecretRef != nil {
		if err := r.validateSecretRef(ctx, provider); err != nil {
			r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretNotFound", err.Error())
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			if statusErr := r.Status().Update(ctx, provider); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{}, err
		}
		r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
			"SecretFound", "Referenced secret exists")
	} else {
		// No secret required for this provider
		r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
			"NoSecretRequired", "Provider does not require credentials")
	}

	// Validate credentials if enabled
	if provider.Spec.ValidateCredentials {
		if err := r.validateCredentials(ctx, provider); err != nil {
			r.setCondition(provider, ProviderConditionTypeCredentialsValid, metav1.ConditionFalse,
				"CredentialsInvalid", err.Error())
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			if statusErr := r.Status().Update(ctx, provider); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{}, nil // Don't requeue for invalid credentials
		}
		now := metav1.Now()
		provider.Status.LastValidatedAt = &now
		r.setCondition(provider, ProviderConditionTypeCredentialsValid, metav1.ConditionTrue,
			"CredentialsValid", "Credentials validated successfully")
	} else {
		// Clear validation condition if not enabled
		r.setCondition(provider, ProviderConditionTypeCredentialsValid, metav1.ConditionTrue,
			"ValidationDisabled", "Credential validation is disabled")
	}

	// Set phase to Ready if all validations pass
	provider.Status.Phase = omniav1alpha1.ProviderPhaseReady
	provider.Status.ObservedGeneration = provider.Generation

	if err := r.Status().Update(ctx, provider); err != nil {
		log.Error(err, "Failed to update Provider status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled Provider", "name", req.Name, "phase", provider.Status.Phase)
	return ctrl.Result{}, nil
}

// validateSecretRef validates that the referenced secret exists and has the expected key.
func (r *ProviderReconciler) validateSecretRef(ctx context.Context, provider *omniav1alpha1.Provider) error {
	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      provider.Spec.SecretRef.Name,
		Namespace: provider.Namespace,
	}

	if err := r.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("secret %q not found in namespace %q", key.Name, key.Namespace)
		}
		return fmt.Errorf("failed to get secret %q: %w", key.Name, err)
	}

	// Check for expected key if specified
	if provider.Spec.SecretRef.Key != nil {
		expectedKey := *provider.Spec.SecretRef.Key
		if _, exists := secret.Data[expectedKey]; !exists {
			return fmt.Errorf("secret %q does not contain key %q", key.Name, expectedKey)
		}
		return nil
	}

	// Check for provider-appropriate key
	expectedKeys := getExpectedKeysForProvider(provider.Spec.Type)
	for _, k := range expectedKeys {
		if _, exists := secret.Data[k]; exists {
			return nil
		}
	}

	return fmt.Errorf("secret %q does not contain any expected API key (%v)", key.Name, expectedKeys)
}

// getExpectedKeysForProvider returns the expected secret keys for a provider type.
func getExpectedKeysForProvider(providerType omniav1alpha1.ProviderType) []string {
	switch providerType {
	case omniav1alpha1.ProviderTypeClaude:
		return []string{"ANTHROPIC_API_KEY", "CLAUDE_API_KEY", secretKeyAPIKey}
	case omniav1alpha1.ProviderTypeOpenAI:
		return []string{"OPENAI_API_KEY", "OPENAI_TOKEN", secretKeyAPIKey}
	case omniav1alpha1.ProviderTypeGemini:
		return []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", secretKeyAPIKey}
	default:
		return []string{secretKeyAPIKey, "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY"}
	}
}

// validateCredentials validates the credentials by making a test API call.
// This is a placeholder for future implementation.
func (r *ProviderReconciler) validateCredentials(_ context.Context, _ *omniav1alpha1.Provider) error {
	// TODO: Implement credential validation by making a lightweight API call
	// For now, we just verify the secret exists (done in validateSecretRef)
	return nil
}

// setCondition sets a condition on the Provider status.
func (r *ProviderReconciler) setCondition(
	provider *omniav1alpha1.Provider,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: provider.Generation,
		Reason:             reason,
		Message:            message,
	})
}

// findProvidersForSecret maps a Secret to Providers that reference it.
func (r *ProviderReconciler) findProvidersForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret := obj.(*corev1.Secret)
	log := logf.FromContext(ctx)

	providerList := &omniav1alpha1.ProviderList{}
	if err := r.List(ctx, providerList, client.InNamespace(secret.Namespace)); err != nil {
		log.Error(err, "Failed to list Providers for Secret mapping")
		return nil
	}

	var requests []reconcile.Request
	for _, p := range providerList.Items {
		if p.Spec.SecretRef != nil && p.Spec.SecretRef.Name == secret.Name {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      p.Name,
					Namespace: p.Namespace,
				},
			})
		}
	}

	return requests
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&omniav1alpha1.Provider{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.findProvidersForSecret),
		).
		Named("provider").
		Complete(r)
}
