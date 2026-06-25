/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ProviderValidator emits advisory warnings when a Provider references a
// missing Secret or key, or when a role:embedding Provider's embedding
// dimension is changed. It never rejects — both cases are advisory so that
// GitOps ordering (secret applied after provider) does not break admission.
type ProviderValidator struct {
	Client client.Client
}

var providerLog = logf.Log.WithName("provider-webhook")

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-provider,mutating=false,failurePolicy=ignore,sideEffects=None,groups=omnia.altairalabs.ai,resources=providers,verbs=create;update,versions=v1alpha1,name=vprovider.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*corev1alpha1.Provider] = &ProviderValidator{}

// SetupProviderWebhookWithManager registers the webhook with the manager.
func SetupProviderWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.Provider{}).
		WithValidator(&ProviderValidator{Client: mgr.GetClient()}).
		Complete()
}

// ValidateCreate warns when a Provider references a missing Secret or key.
func (v *ProviderValidator) ValidateCreate(ctx context.Context, provider *corev1alpha1.Provider) (admission.Warnings, error) {
	return v.secretRefWarnings(ctx, provider), nil
}

// ValidateUpdate warns when an embedding Provider's declared dimension changes,
// and when a Provider references a missing Secret or key.
func (v *ProviderValidator) ValidateUpdate(ctx context.Context, oldObj, newObj *corev1alpha1.Provider) (admission.Warnings, error) {
	var warns admission.Warnings
	if newObj.EffectiveRole() == corev1alpha1.ProviderRoleEmbedding {
		oldDim := declaredEmbeddingDim(oldObj)
		newDim := declaredEmbeddingDim(newObj)
		// Only warn on an explicit change between two concrete dimensions — the
		// unambiguous destructive case. Leaving the dimension unset (model-driven)
		// or first declaring it produces no warning.
		if oldDim != 0 && newDim != 0 && oldDim != newDim {
			providerLog.Info("embedding dimension change detected",
				"name", newObj.Name, "namespace", newObj.Namespace,
				"fromDim", oldDim, "toDim", newDim)
			warns = append(warns, fmt.Sprintf(
				"changing the embedding dimension (%d→%d) discards all existing memory embeddings and "+
					"triggers a full re-embed on the next memory-api restart. It takes effect only after you "+
					"record one-shot consent (POST /admin/embedding-dimension-change {\"target_dim\":%d}); "+
					"without it memory-api refuses to start.", oldDim, newDim, newDim))
		}
	}
	return append(warns, v.secretRefWarnings(ctx, newObj)...), nil
}

// ValidateDelete permits all deletions.
func (v *ProviderValidator) ValidateDelete(_ context.Context, _ *corev1alpha1.Provider) (admission.Warnings, error) {
	return nil, nil
}

// secretRefWarnings checks credential.secretRef and auth.credentialsSecretRef
// against live Secrets and returns advisory warnings (never errors) so a
// Provider can still be applied before its Secret exists (GitOps ordering).
func (v *ProviderValidator) secretRefWarnings(ctx context.Context, p *corev1alpha1.Provider) admission.Warnings {
	var warns admission.Warnings
	if p.Spec.Credential != nil && p.Spec.Credential.SecretRef != nil {
		warns = append(warns, v.checkRef(ctx, p, p.Spec.Credential.SecretRef,
			corev1alpha1.ExpectedKeysForProvider(p.EffectiveRole(), p.Spec.Type))...)
	}
	if p.Spec.Auth != nil && p.Spec.Auth.CredentialsSecretRef != nil {
		warns = append(warns, v.checkRef(ctx, p, p.Spec.Auth.CredentialsSecretRef, nil)...)
	}
	return warns
}

// checkRef looks up a single SecretKeyRef in the cluster. Missing secret or
// missing key produces a warning; transient errors are silently swallowed so
// RBAC gaps or apiserver hiccups never block admission.
func (v *ProviderValidator) checkRef(ctx context.Context, p *corev1alpha1.Provider, ref *corev1alpha1.SecretKeyRef, defaultKeys []string) admission.Warnings {
	if v.Client == nil || ref.Name == "" {
		return nil
	}
	secret := &corev1.Secret{}
	err := v.Client.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: p.Namespace}, secret)
	if apierrors.IsNotFound(err) {
		return admission.Warnings{fmt.Sprintf(
			"referenced Secret %q not found in namespace %q; the Provider will report phase=Error until it exists",
			ref.Name, p.Namespace)}
	}
	if err != nil {
		return nil // transient/RBAC: stay advisory, don't block
	}
	if ref.Key != nil {
		if _, ok := secret.Data[*ref.Key]; !ok {
			return admission.Warnings{fmt.Sprintf("Secret %q has no key %q", ref.Name, *ref.Key)}
		}
		return nil
	}
	for _, k := range defaultKeys {
		if _, ok := secret.Data[k]; ok {
			return nil
		}
	}
	if len(defaultKeys) > 0 {
		return admission.Warnings{fmt.Sprintf("Secret %q has none of the expected keys %v", ref.Name, defaultKeys)}
	}
	return nil
}

// declaredEmbeddingDim returns the Provider's declared embedding dimension, or
// 0 when the embedding block or dimension is unset.
func declaredEmbeddingDim(p *corev1alpha1.Provider) int32 {
	if p == nil || p.Spec.Embedding == nil {
		return 0
	}
	return p.Spec.Embedding.Dimensions
}
