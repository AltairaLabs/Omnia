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
	"net/http"
	"path/filepath"
	"regexp"
	"time"

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
	ProviderConditionTypeSecretFound          = "SecretFound"
	ProviderConditionTypeCredentialConfigured = "CredentialConfigured"
	ProviderConditionTypeAuthConfigured       = "AuthConfigured"
	ProviderConditionTypeEndpointReachable    = "EndpointReachable"
	// secretKeyAPIKey is the common secret key name for API keys.
	secretKeyAPIKey = "api-key"
	// Error message formats (go:S1192 — extracted to avoid duplication).
	errFmtSecretNotFound      = "secret %q not found in namespace %q"
	errFmtSecretMissingKey    = "secret %q does not contain key %q"
	errFmtSecretMissingAnyKey = "secret %q does not contain any expected API key (%v)"
	errFmtSecretGetFailed     = "failed to get secret %q: %v"
)

// healthCheckTimeout is how long we wait for a provider endpoint to respond.
const healthCheckTimeout = 5 * time.Second

// healthCheckRequeueInterval is how often we retry when a provider is unavailable.
const healthCheckRequeueInterval = 30 * time.Second

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
	Scheme     *runtime.Scheme
	Recorder   record.EventRecorder
	HTTPClient *http.Client // used for provider health checks; defaults to a 5s-timeout client
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
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}

	// Validate auth configuration for hyperscaler providers
	if err := r.validateAuthConfig(ctx, provider); err != nil {
		if statusErr := r.Status().Update(ctx, provider); statusErr != nil {
			log.Error(statusErr, logMsgFailedToUpdateStatus)
		}
		return ctrl.Result{}, err
	}

	// Health-check the provider endpoint if it has one
	if healthURL := r.resolveHealthURL(provider); healthURL != "" {
		if err := r.checkEndpointHealth(ctx, healthURL); err != nil {
			log.Info("provider endpoint unreachable",
				"name", req.Name, "url", healthURL, "error", err)
			provider.Status.Phase = omniav1alpha1.ProviderPhaseUnavailable
			meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{
				Type:               ProviderConditionTypeEndpointReachable,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: provider.Generation,
				Reason:             "EndpointUnreachable",
				Message:            fmt.Sprintf("health check failed: %v", err),
			})
			provider.Status.ObservedGeneration = provider.Generation
			if statusErr := r.Status().Update(ctx, provider); statusErr != nil {
				log.Error(statusErr, logMsgFailedToUpdateStatus)
			}
			return ctrl.Result{RequeueAfter: healthCheckRequeueInterval}, nil
		}
		meta.SetStatusCondition(&provider.Status.Conditions, metav1.Condition{
			Type:               ProviderConditionTypeEndpointReachable,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: provider.Generation,
			Reason:             "EndpointReachable",
			Message:            "health check passed",
		})
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
			SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretNotFound", err.Error())
			SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
				"SecretNotFound", err.Error())
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			return err
		}
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
			"SecretFound", "Referenced secret exists")
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
			"LegacySecretRef", "Credential configured via legacy secretRef")
		r.emitWarningEvent(provider, EventReasonLegacySecretRefUsed,
			"Provider uses deprecated top-level secretRef; migrate to spec.credential.secretRef")
		return nil
	}

	// No credentials specified
	if providerRequiresCredentials(provider) {
		msg := fmt.Sprintf("provider type %q requires credentials but none are configured", provider.Spec.Type)
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
			"NoSecretRequired", "Provider does not require credentials")
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"CredentialRequired", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
		"NoSecretRequired", "Provider does not require credentials")
	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
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
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"NoStrategySpecified", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	if count > 1 {
		msg := "credential block has multiple strategies set; exactly one of secretRef, envVar, or filePath must be specified"
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
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
			msg := fmt.Sprintf(errFmtSecretNotFound, key.Name, key.Namespace)
			SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretNotFound", msg)
			SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
				"SecretNotFound", msg)
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			return fmt.Errorf("%s", msg)
		}
		msg := fmt.Sprintf(errFmtSecretGetFailed, key.Name, err)
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
			"SecretNotFound", msg)
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"SecretNotFound", msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	// Check for expected key if specified
	if ref.Key != nil {
		expectedKey := *ref.Key
		if _, exists := secret.Data[expectedKey]; !exists {
			msg := fmt.Sprintf(errFmtSecretMissingKey, key.Name, expectedKey)
			SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretKeyMissing", msg)
			SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
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
			msg := fmt.Sprintf(errFmtSecretMissingAnyKey, key.Name, expectedKeys)
			SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionFalse,
				"SecretKeyMissing", msg)
			SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
				"SecretKeyMissing", msg)
			provider.Status.Phase = omniav1alpha1.ProviderPhaseError
			return fmt.Errorf("%s", msg)
		}
	}

	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
		"SecretFound", "Referenced secret exists")
	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
		"SecretFound", "Credential configured via secret reference")
	return nil
}

// validateCredentialEnvVar validates an environment variable name.
func (r *ProviderReconciler) validateCredentialEnvVar(provider *omniav1alpha1.Provider, envVar string) error {
	if !envVarNameRegex.MatchString(envVar) {
		msg := fmt.Sprintf("invalid environment variable name %q: must match [a-zA-Z_][a-zA-Z0-9_]*", envVar)
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"InvalidEnvVar", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
		"NoSecretRequired", "Credential uses environment variable, no secret required")
	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
		"EnvVarConfigured", fmt.Sprintf("Credential configured via environment variable %q", envVar))
	return nil
}

// validateCredentialFilePath validates that a file path is absolute and clean.
func (r *ProviderReconciler) validateCredentialFilePath(provider *omniav1alpha1.Provider, path string) error {
	if !filepath.IsAbs(path) {
		msg := fmt.Sprintf("credential file path %q must be absolute", path)
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"InvalidFilePath", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	cleaned := filepath.Clean(path)
	if cleaned != path {
		msg := fmt.Sprintf("credential file path %q is not clean (resolved to %q)", path, cleaned)
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionFalse,
			"InvalidFilePath", msg)
		r.emitWarningEvent(provider, EventReasonCredentialInvalid, msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeSecretFound, metav1.ConditionTrue,
		"NoSecretRequired", "Credential uses file path, no secret required")
	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeCredentialConfigured, metav1.ConditionTrue,
		"FilePathConfigured", fmt.Sprintf("Credential configured via file path %q", path))
	return nil
}

// isPlatformHosted returns whether the provider is hosted on a hyperscaler platform.
// Platform hosting is signalled by spec.platform being set (CEL validation enforces
// that platform is only legal for claude/openai/gemini).
func isPlatformHosted(provider *omniav1alpha1.Provider) bool {
	return provider.Spec.Platform != nil
}

// validateAuthConfig validates the auth configuration for platform-hosted providers.
func (r *ProviderReconciler) validateAuthConfig(ctx context.Context, provider *omniav1alpha1.Provider) error {
	if !isPlatformHosted(provider) {
		// Non-platform-hosted providers don't use auth config. CEL validation
		// rejects spec.auth without spec.platform, so nothing to do here.
		return nil
	}

	if provider.Spec.Auth == nil {
		// CEL admission enforces spec.auth when spec.platform is set; treat any
		// slip-through as a configuration error rather than silently accepting.
		msg := "spec.auth is required when spec.platform is set"
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeAuthConfigured, metav1.ConditionFalse,
			"AuthRequired", msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	auth := provider.Spec.Auth

	// Workload identity - no secret needed
	if auth.Type == omniav1alpha1.AuthMethodWorkloadIdentity {
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeAuthConfigured, metav1.ConditionTrue,
			"WorkloadIdentityConfigured", "Workload identity authentication configured")
		return nil
	}

	// Non-workload-identity types require credentialsSecretRef
	if auth.CredentialsSecretRef == nil {
		msg := fmt.Sprintf("auth type %q requires credentialsSecretRef", auth.Type)
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeAuthConfigured, metav1.ConditionFalse,
			"CredentialsSecretRefRequired", msg)
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return fmt.Errorf("%s", msg)
	}

	// Validate the referenced secret exists and has the expected platform keys
	if err := r.validatePlatformCredentialsSecret(ctx, provider, auth); err != nil {
		SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeAuthConfigured, metav1.ConditionFalse,
			"CredentialsSecretNotFound", err.Error())
		provider.Status.Phase = omniav1alpha1.ProviderPhaseError
		return err
	}

	SetCondition(&provider.Status.Conditions, provider.Generation, ProviderConditionTypeAuthConfigured, metav1.ConditionTrue,
		"AuthConfigured", fmt.Sprintf("Auth type %q configured with credentials secret", auth.Type))
	return nil
}

// validatePlatformCredentialsSecret verifies the auth.credentialsSecretRef
// secret exists and contains the keys expected for the platform/auth combo.
func (r *ProviderReconciler) validatePlatformCredentialsSecret(
	ctx context.Context, provider *omniav1alpha1.Provider, auth *omniav1alpha1.AuthConfig,
) error {
	ref := auth.CredentialsSecretRef
	secret := &corev1.Secret{}
	key := types.NamespacedName{Name: ref.Name, Namespace: provider.Namespace}

	if err := r.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf(errFmtSecretNotFound, ref.Name, provider.Namespace)
		}
		return fmt.Errorf("failed to get secret %q: %w", ref.Name, err)
	}

	// When a specific key is named, that key alone must exist.
	if ref.Key != nil && *ref.Key != "" {
		if _, ok := secret.Data[*ref.Key]; !ok {
			return fmt.Errorf(errFmtSecretMissingKey, ref.Name, *ref.Key)
		}
		return nil
	}

	// Otherwise check for the platform+auth expected keys.
	expected := expectedPlatformSecretKeys(provider.Spec.Platform.Type, auth.Type)
	for _, k := range expected {
		if _, ok := secret.Data[k]; !ok {
			return fmt.Errorf("secret %q missing expected key %q for %s/%s", ref.Name, k, provider.Spec.Platform.Type, auth.Type)
		}
	}
	return nil
}

// expectedPlatformSecretKeys returns the keys that must be present in the
// auth.credentialsSecretRef secret for each supported platform/auth combo.
// workloadIdentity combos do not use a secret and are not listed here.
func expectedPlatformSecretKeys(platform omniav1alpha1.PlatformType, auth omniav1alpha1.AuthMethod) []string {
	switch {
	case platform == omniav1alpha1.PlatformTypeBedrock && auth == omniav1alpha1.AuthMethodAccessKey:
		return []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}
	case platform == omniav1alpha1.PlatformTypeVertex && auth == omniav1alpha1.AuthMethodServiceAccount:
		return []string{"credentials.json"}
	case platform == omniav1alpha1.PlatformTypeAzure && auth == omniav1alpha1.AuthMethodServicePrincipal:
		return []string{"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET"}
	default:
		return nil
	}
}

// providerRequiresCredentials returns whether the given provider requires an
// API-key credential. Providers hosted on a platform (bedrock/vertex/azure) use
// the platform auth instead of an API key, so credentials are not required.
func providerRequiresCredentials(provider *omniav1alpha1.Provider) bool {
	if isPlatformHosted(provider) {
		return false
	}
	switch provider.Spec.Type {
	case omniav1alpha1.ProviderTypeMock, omniav1alpha1.ProviderTypeOllama, omniav1alpha1.ProviderTypeVLLM:
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
			return fmt.Errorf(errFmtSecretNotFound, key.Name, key.Namespace)
		}
		return fmt.Errorf("failed to get secret %q: %w", key.Name, err)
	}

	// Check for expected key if specified
	if provider.Spec.SecretRef.Key != nil {
		expectedKey := *provider.Spec.SecretRef.Key
		if _, exists := secret.Data[expectedKey]; !exists {
			return fmt.Errorf(errFmtSecretMissingKey, key.Name, expectedKey)
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

	return fmt.Errorf(errFmtSecretMissingAnyKey, key.Name, expectedKeys)
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
	case omniav1alpha1.ProviderTypeVoyageAI:
		return []string{"VOYAGE_API_KEY", secretKeyAPIKey}
	default:
		return []string{secretKeyAPIKey, "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY"}
	}
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

// defaultProviderEndpoints maps cloud provider types to their default API base URLs.
var defaultProviderEndpoints = map[omniav1alpha1.ProviderType]string{
	omniav1alpha1.ProviderTypeClaude: "https://api.anthropic.com",
	omniav1alpha1.ProviderTypeOpenAI: "https://api.openai.com",
	omniav1alpha1.ProviderTypeGemini: "https://generativelanguage.googleapis.com",
}

// resolveHealthURL returns the URL to health-check for this provider.
// Returns empty string if no health check is applicable (mock, platform-hosted
// providers that use SDK auth rather than a reachable HTTP URL, or providers
// without a known base URL like voyageai).
func (r *ProviderReconciler) resolveHealthURL(provider *omniav1alpha1.Provider) string {
	if provider.Spec.Type == omniav1alpha1.ProviderTypeMock {
		return ""
	}
	// Platform-hosted providers authenticate via cloud SDK, not a reachable HTTP URL.
	if isPlatformHosted(provider) {
		return ""
	}

	baseURL := provider.Spec.BaseURL
	if baseURL == "" {
		baseURL = defaultProviderEndpoints[provider.Spec.Type]
	}
	if baseURL == "" {
		return "" // no default endpoint (e.g. vllm without baseURL) — skip
	}

	// Provider-specific health paths
	switch provider.Spec.Type {
	case omniav1alpha1.ProviderTypeOllama:
		return baseURL + "/api/tags"
	default:
		// For OpenAI-compatible, Claude, Gemini: just check the base is reachable.
		// We don't send auth headers — a 401 still proves the endpoint is up.
		return baseURL
	}
}

// checkEndpointHealth does a lightweight HTTP check to verify the provider endpoint is reachable.
// A non-2xx response (e.g. 401 Unauthorized) is still considered "reachable" — it proves the
// server is running. Only connection failures (DNS, timeout, refused) are treated as unhealthy.
func (r *ProviderReconciler) checkEndpointHealth(_ context.Context, url string) error {
	httpClient := r.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: healthCheckTimeout}
	}
	resp, err := httpClient.Get(url) //nolint:gosec // URL is from trusted CRD spec, not user input
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	// Any HTTP response means the endpoint is reachable — even 401/403/404
	return nil
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
