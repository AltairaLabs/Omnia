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
	"path/filepath"
	"regexp"

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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Provider condition types
const (
	ProviderConditionTypeCredentialsValid     = "CredentialsValid"
	ProviderConditionTypeSecretFound          = "SecretFound"
	ProviderConditionTypeCredentialConfigured = "CredentialConfigured"
	// secretKeyAPIKey is the common secret key name for API keys.
	secretKeyAPIKey = "api-key"
)

// Event reason constants
const (
	EventReasonCredentialInvalid   = "CredentialInvalid"
	EventReasonMultipleCredentials = "MultipleCredentials"
	EventReasonLegacySecretRefUsed = "LegacySecretRefUsed"
)

// envVarNameRegex validates environment variable names.
var envVarNameRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// ProviderReconciler reconciles a Provider object
type ProviderReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=omnia.altairalabs.ai,resources=providers/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

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

	// Validate credential configuration
	if err := r.validateCredentialConfig(ctx, provider); err != nil {
		if statusErr := r.Status().Update(ctx, provider); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{}, err
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

// validateCredentialConfig dispatches to the appropriate credential validation strategy.
func (r *ProviderReconciler) validateCredentialConfig(ctx context.Context, provider *omniav1alpha1.Provider) error {
	if provider.Spec.Credential != nil {
		// New credential block takes precedence
		return r.validateCredentialBlock(ctx, provider)
	}

	if provider.Spec.SecretRef != nil {
		// Legacy secretRef path
		if err := r.validateSecretRef(ctx, provider); err != nil {
			r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretNotFound", err.Error())
			r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
				"SecretNotFound", err.Error())
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			return err
		}
		r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
			"SecretFound", "Referenced secret exists")
		r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
			"LegacySecretRef", "Credential configured via legacy secretRef")
		r.emitWarningEvent(provider, EventReasonLegacySecretRefUsed,
			"Provider uses deprecated top-level secretRef; migrate to spec.credential.secretRef")
		return nil
	}

	// No credentials specified
	if providerRequiresCredentials(provider.Spec.Type) {
		msg := fmt.Sprintf("provider type %q requires credentials but none are configured", provider.Spec.Type)
		r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
			"NoSecretRequired", "Provider does not require credentials")
		r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"CredentialRequired", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
		"NoSecretRequired", "Provider does not require credentials")
	r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
		"NoCredentialRequired", "Provider type does not require credentials")
	return nil
}

// validateCredentialBlock validates the credential block and dispatches to the correct strategy.
func (r *ProviderReconciler) validateCredentialBlock(ctx context.Context, provider *omniav1alpha1.Provider) error {
	cred := provider.Spec.Credential

	// Count how many strategies are set
	count := 0
	if cred.SecretRef != nil {
		count++
	}
	if cred.EnvVar != "" {
		count++
	}
	if cred.FilePath != "" {
		count++
	}

	if count == 0 {
		msg := "credential block is set but no strategy is specified (secretRef, envVar, or filePath)"
		r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"NoStrategySpecified", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	if count > 1 {
		msg := "credential block has multiple strategies set; exactly one of secretRef, envVar, or filePath must be specified"
		r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"MultipleStrategies", msg)
		r.emitWarningEvent(provider, EventReasonMultipleCredentials, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	// Dispatch to the single strategy
	switch {
	case cred.SecretRef != nil:
		return r.validateCredentialSecretRef(ctx, provider, cred.SecretRef)
	case cred.EnvVar != "":
		return r.validateCredentialEnvVar(provider, cred.EnvVar)
	default:
		return r.validateCredentialFilePath(provider, cred.FilePath)
	}
}

// validateCredentialSecretRef validates a SecretKeyRef from the credential block.
func (r *ProviderReconciler) validateCredentialSecretRef(ctx context.Context, provider *omniav1alpha1.Provider, ref *omniav1alpha1.SecretKeyRef) error {
	secret := &corev1.Secret{}
	key := types.NamespacedName{
		Name:      ref.Name,
		Namespace: provider.Namespace,
	}

	if err := r.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("secret %q not found in namespace %q", key.Name, key.Namespace)
			r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretNotFound", msg)
			r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
				"SecretNotFound", msg)
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			return fmt.Errorf("%s", msg)
		}
		msg := fmt.Sprintf("failed to get secret %q: %v", key.Name, err)
		r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
			"SecretNotFound", msg)
		r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"SecretNotFound", msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	// Check for expected key if specified
	if ref.Key != nil {
		expectedKey := *ref.Key
		if _, exists := secret.Data[expectedKey]; !exists {
			msg := fmt.Sprintf("secret %q does not contain key %q", key.Name, expectedKey)
			r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretKeyMissing", msg)
			r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
				"SecretKeyMissing", msg)
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			return fmt.Errorf("%s", msg)
		}
	} else {
		// Check for provider-appropriate key
		expectedKeys := getExpectedKeysForProvider(provider.Spec.Type)
		found := false
		for _, k := range expectedKeys {
			if _, exists := secret.Data[k]; exists {
				found = true
				break
			}
		}
		if !found {
			msg := fmt.Sprintf("secret %q does not contain any expected API key (%v)", key.Name, expectedKeys)
			r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretKeyMissing", msg)
			r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
				"SecretKeyMissing", msg)
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			return fmt.Errorf("%s", msg)
		}
	}

	r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
		"SecretFound", "Referenced secret exists")
	r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
		"SecretFound", "Credential configured via secret reference")
	return nil
}

// validateCredentialEnvVar validates an environment variable name.
func (r *ProviderReconciler) validateCredentialEnvVar(provider *omniav1alpha1.Provider, envVar string) error {
	if !envVarNameRegex.MatchString(envVar) {
		msg := fmt.Sprintf("invalid environment variable name %q: must match [a-zA-Z_][a-zA-Z0-9_]*", envVar)
		r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"InvalidEnvVar", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
		"NoSecretRequired", "Credential uses environment variable, no secret required")
	r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
		"EnvVarConfigured", fmt.Sprintf("Credential configured via environment variable %q", envVar))
	return nil
}

// validateCredentialFilePath validates that a file path is absolute and clean.
func (r *ProviderReconciler) validateCredentialFilePath(provider *omniav1alpha1.Provider, path string) error {
	if !filepath.IsAbs(path) {
		msg := fmt.Sprintf("credential file path %q must be absolute", path)
		r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"InvalidFilePath", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	cleaned := filepath.Clean(path)
	if cleaned != path {
		msg := fmt.Sprintf("credential file path %q is not clean (resolved to %q)", path, cleaned)
		r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"InvalidFilePath", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	r.setCondition(provider, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
		"NoSecretRequired", "Credential uses file path, no secret required")
	r.setCondition(provider, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
		"FilePathConfigured", fmt.Sprintf("Credential configured via file path %q", path))
	return nil
}

// providerRequiresCredentials returns whether the given provider type requires credentials.
func providerRequiresCredentials(providerType omniav1alpha1.ProviderType) bool {
	switch providerType {
	case omniav1alpha1.ProviderTypeMock, omniav1alpha1.ProviderTypeOllama,
		omniav1alpha1.ProviderTypeBedrock, omniav1alpha1.ProviderTypeVertex,
		omniav1alpha1.ProviderTypeAzureAI:
		return false
	default:
		return true
	}
}

// emitWarningEvent emits a Kubernetes warning event if a Recorder is available.
func (r *ProviderReconciler) emitWarningEvent(provider *omniav1alpha1.Provider, reason, message string) {
	if r.Recorder != nil {
		r.Recorder.Event(provider, corev1.EventTypeWarning, reason, message)
	}
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
	case omniav1alpha1.ProviderTypeBedrock:
		return []string{"AWS_ACCESS_KEY_ID", secretKeyAPIKey}
	case omniav1alpha1.ProviderTypeVertex:
		return []string{"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_API_KEY", secretKeyAPIKey}
	case omniav1alpha1.ProviderTypeAzureAI:
		return []string{"AZURE_OPENAI_API_KEY", "AZURE_API_KEY", secretKeyAPIKey}
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
		matches := false
		if p.Spec.SecretRef != nil && p.Spec.SecretRef.Name == secret.Name {
			matches = true
		}
		if p.Spec.Credential != nil && p.Spec.Credential.SecretRef != nil && p.Spec.Credential.SecretRef.Name == secret.Name {
			matches = true
		}
		if matches {
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
