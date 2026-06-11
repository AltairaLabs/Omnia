/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package webhook

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ProviderValidator emits an advisory warning when a role:embedding Provider's
// embedding dimension is changed. It never rejects — the memory store's
// embedding dimension is application-managed and a change is gated server-side
// by a one-shot consent marker (#1309). The warning just makes the operator
// aware that the change discards existing embeddings and forces a full
// re-embed, and that they must record consent for it to take effect.
type ProviderValidator struct{}

var providerLog = logf.Log.WithName("provider-webhook")

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-provider,mutating=false,failurePolicy=ignore,sideEffects=None,groups=omnia.altairalabs.ai,resources=providers,verbs=update,versions=v1alpha1,name=vprovider.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*corev1alpha1.Provider] = &ProviderValidator{}

// SetupProviderWebhookWithManager registers the webhook with the manager.
func SetupProviderWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.Provider{}).
		WithValidator(&ProviderValidator{}).
		Complete()
}

// ValidateCreate permits all creates (a create can't change a dimension).
func (v *ProviderValidator) ValidateCreate(_ context.Context, _ *corev1alpha1.Provider) (admission.Warnings, error) {
	return nil, nil
}

// ValidateUpdate warns when an embedding Provider's declared dimension changes.
func (v *ProviderValidator) ValidateUpdate(_ context.Context, oldObj, newObj *corev1alpha1.Provider) (admission.Warnings, error) {
	if newObj.EffectiveRole() != corev1alpha1.ProviderRoleEmbedding {
		return nil, nil
	}
	oldDim := declaredEmbeddingDim(oldObj)
	newDim := declaredEmbeddingDim(newObj)
	// Only warn on an explicit change between two concrete dimensions — the
	// unambiguous destructive case. Leaving the dimension unset (model-driven)
	// or first declaring it produces no warning.
	if oldDim != 0 && newDim != 0 && oldDim != newDim {
		providerLog.Info("embedding dimension change detected",
			"name", newObj.Name, "namespace", newObj.Namespace,
			"fromDim", oldDim, "toDim", newDim)
		return admission.Warnings{fmt.Sprintf(
			"changing the embedding dimension (%d→%d) discards all existing memory embeddings and "+
				"triggers a full re-embed on the next memory-api restart. It takes effect only after you "+
				"record one-shot consent (POST /admin/embedding-dimension-change {\"target_dim\":%d}); "+
				"without it memory-api refuses to start.", oldDim, newDim, newDim),
		}, nil
	}
	return nil, nil
}

// ValidateDelete permits all deletions.
func (v *ProviderValidator) ValidateDelete(_ context.Context, _ *corev1alpha1.Provider) (admission.Warnings, error) {
	return nil, nil
}

// declaredEmbeddingDim returns the Provider's declared embedding dimension, or
// 0 when the embedding block or dimension is unset.
func declaredEmbeddingDim(p *corev1alpha1.Provider) int32 {
	if p == nil || p.Spec.Embedding == nil {
		return 0
	}
	return p.Spec.Embedding.Dimensions
}
