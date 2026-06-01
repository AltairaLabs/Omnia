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
	"github.com/altairalabs/omnia/internal/schemautil"
)

// AgentRuntimeValidator rejects function-mode AgentRuntimes whose
// spec.inputSchema / spec.outputSchema are not valid JSON Schemas, using the
// same compiler the facade runs at startup so admission and runtime agree.
type AgentRuntimeValidator struct{}

var agentRuntimeLog = logf.Log.WithName("agentruntime-webhook")

// +kubebuilder:webhook:path=/validate-omnia-altairalabs-ai-v1alpha1-agentruntime,mutating=false,failurePolicy=fail,sideEffects=None,groups=omnia.altairalabs.ai,resources=agentruntimes,verbs=create;update,versions=v1alpha1,name=vagentruntime.kb.io,admissionReviewVersions=v1

var _ admission.Validator[*corev1alpha1.AgentRuntime] = &AgentRuntimeValidator{}

// SetupAgentRuntimeWebhookWithManager registers the webhook with the manager.
func SetupAgentRuntimeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &corev1alpha1.AgentRuntime{}).
		WithValidator(&AgentRuntimeValidator{}).
		Complete()
}

// ValidateCreate compiles the function-mode schemas.
func (v *AgentRuntimeValidator) ValidateCreate(_ context.Context, ar *corev1alpha1.AgentRuntime) (admission.Warnings, error) {
	agentRuntimeLog.Info("validating create", "name", ar.Name, "namespace", ar.Namespace)
	return nil, validateAgentRuntimeSchemas(ar)
}

// ValidateUpdate applies the same schema checks on updates.
func (v *AgentRuntimeValidator) ValidateUpdate(_ context.Context, _, ar *corev1alpha1.AgentRuntime) (admission.Warnings, error) {
	agentRuntimeLog.Info("validating update", "name", ar.Name, "namespace", ar.Namespace)
	return nil, validateAgentRuntimeSchemas(ar)
}

// ValidateDelete permits all deletions.
func (v *AgentRuntimeValidator) ValidateDelete(_ context.Context, _ *corev1alpha1.AgentRuntime) (admission.Warnings, error) {
	return nil, nil
}

// validateAgentRuntimeSchemas compiles inputSchema/outputSchema when the
// runtime is in function mode. Presence is enforced by CEL; this validator
// only checks validity, so it compiles whichever schema is present.
func validateAgentRuntimeSchemas(ar *corev1alpha1.AgentRuntime) error {
	if !ar.IsFunctionMode() {
		return nil
	}
	if ar.Spec.InputSchema != nil {
		if _, err := schemautil.CompileSchema(ar.Spec.InputSchema.Raw); err != nil {
			return fmt.Errorf("spec.inputSchema is not a valid JSON Schema: %w", err)
		}
	}
	if ar.Spec.OutputSchema != nil {
		if _, err := schemautil.CompileSchema(ar.Spec.OutputSchema.Raw); err != nil {
			return fmt.Errorf("spec.outputSchema is not a valid JSON Schema: %w", err)
		}
	}
	return nil
}
