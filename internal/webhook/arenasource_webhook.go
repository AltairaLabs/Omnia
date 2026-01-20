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

package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/license"
)

// ArenaSourceValidator validates ArenaSource resources against the license.
type ArenaSourceValidator struct {
	LicenseValidator *license.Validator
}

// log is for logging in this package.
var arenasourcelog = logf.Log.WithName("arenasource-webhook")

// SetupArenaSourceWebhookWithManager registers the ArenaSource webhook with the manager.
func SetupArenaSourceWebhookWithManager(mgr ctrl.Manager, licenseValidator *license.Validator) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&omniav1alpha1.ArenaSource{}).
		WithValidator(&ArenaSourceValidator{LicenseValidator: licenseValidator}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-arenasource,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=arenasources,verbs=create;update,versions=v1alpha1,name=varenasource.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &ArenaSourceValidator{}

// ValidateCreate implements webhook.CustomValidator.
func (v *ArenaSourceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	source, ok := obj.(*omniav1alpha1.ArenaSource)
	if !ok {
		return nil, fmt.Errorf("expected ArenaSource but got %T", obj)
	}
	arenasourcelog.Info("validating create", "name", source.Name)

	return v.validateLicense(ctx, source)
}

// ValidateUpdate implements webhook.CustomValidator.
func (v *ArenaSourceValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	source, ok := newObj.(*omniav1alpha1.ArenaSource)
	if !ok {
		return nil, fmt.Errorf("expected ArenaSource but got %T", newObj)
	}
	arenasourcelog.Info("validating update", "name", source.Name)

	return v.validateLicense(ctx, source)
}

// ValidateDelete implements webhook.CustomValidator.
func (v *ArenaSourceValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No license validation needed for delete
	return nil, nil
}

// validateLicense checks if the source type is allowed by the license.
func (v *ArenaSourceValidator) validateLicense(ctx context.Context, source *omniav1alpha1.ArenaSource) (admission.Warnings, error) {
	if v.LicenseValidator == nil {
		// No license validator configured, allow all
		return nil, nil
	}

	sourceType := string(source.Spec.Type)
	if err := v.LicenseValidator.ValidateArenaSource(ctx, sourceType); err != nil {
		if licErr, ok := err.(*license.ValidationError); ok {
			arenasourcelog.Info("license validation failed",
				"name", source.Name,
				"sourceType", sourceType,
				"feature", licErr.Feature,
			)
			return admission.Warnings{licErr.UpgradeMessage()}, fmt.Errorf("%s", licErr.Error())
		}
		return nil, err
	}

	return nil, nil
}
