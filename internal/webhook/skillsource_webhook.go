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

// SkillSourceValidator enforces structural constraints beyond CEL.
type SkillSourceValidator struct{}

var skillSourceLog = logf.Log.WithName("skillsource-webhook")

// kubebuilder:webhook annotation intentionally omitted until the core
// operator wires an admission webhook server. The validator code below is
// reachable from tests and from a future cmd/main.go SetupWebhookWithManager
// call; reinstate the annotation when wiring lands.

var _ admission.Validator[*corev1alpha1.SkillSource] = &SkillSourceValidator{}

// SetupSkillSourceWebhookWithManager registers the webhook with the manager.
func SetupSkillSourceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.SkillSource{}).
		WithValidator(&SkillSourceValidator{}).
		Complete()
}

// ValidateCreate ensures the variant block matches the type.
func (v *SkillSourceValidator) ValidateCreate(_ context.Context, src *corev1alpha1.SkillSource) (admission.Warnings, error) {
	skillSourceLog.Info("validating create", "name", src.Name, "namespace", src.Namespace)
	return nil, validateSkillSourceVariant(src)
}

// ValidateUpdate applies the same variant check on updates.
func (v *SkillSourceValidator) ValidateUpdate(_ context.Context, _, src *corev1alpha1.SkillSource) (admission.Warnings, error) {
	skillSourceLog.Info("validating update", "name", src.Name, "namespace", src.Namespace)
	return nil, validateSkillSourceVariant(src)
}

// ValidateDelete permits all deletions.
func (v *SkillSourceValidator) ValidateDelete(_ context.Context, _ *corev1alpha1.SkillSource) (admission.Warnings, error) {
	return nil, nil
}

func validateSkillSourceVariant(src *corev1alpha1.SkillSource) error {
	switch src.Spec.Type {
	case corev1alpha1.SkillSourceTypeGit:
		if src.Spec.Git == nil {
			return fmt.Errorf("type=git requires spec.git")
		}
	case corev1alpha1.SkillSourceTypeOCI:
		if src.Spec.OCI == nil {
			return fmt.Errorf("type=oci requires spec.oci")
		}
	case corev1alpha1.SkillSourceTypeConfigMap:
		if src.Spec.ConfigMap == nil {
			return fmt.Errorf("type=configmap requires spec.configMap")
		}
	}
	return nil
}
