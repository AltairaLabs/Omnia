/*
Copyright 2026.

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

package v1alpha1

// ProjectLegacyA2AAuth folds the deprecated spec.a2a.authentication.secretRef
// into spec.externalAuth.sharedToken so the new validator chain can treat
// both shapes uniformly.
//
// Precedence rules:
//   - If spec.externalAuth.sharedToken is already set, do nothing — the
//     operator has explicitly chosen the new shape and we don't want to
//     surprise them by overwriting.
//   - If spec.a2a.authentication.secretRef is unset, do nothing — there's
//     no legacy state to project.
//   - Otherwise, ensure spec.externalAuth exists and copy the SecretRef
//     into spec.externalAuth.sharedToken.
//
// Mutates ar in place. Idempotent: subsequent calls with the same input
// are no-ops once the projection has happened.
//
// Why this lives in api/v1alpha1 rather than internal/controller: both
// the controller's Reconcile and cmd/agent's startup chain builder need
// to apply the same projection (otherwise legacy CRs that haven't been
// migrated see an empty data-plane chain at the facade and every request
// 401s). Keeping it on the type package means both callers share one
// definition without a reverse dependency.
//
// The function does not modify the legacy field. We leave it visible on
// the spec so operators see exactly what they configured; the controller
// emits a deprecation warning condition when the old field is still
// populated.
func ProjectLegacyA2AAuth(ar *AgentRuntime) {
	if ar == nil || ar.Spec.A2A == nil {
		return
	}
	// The whole point of this helper is to migrate the deprecated
	// authentication shape into the new one, so reading the old field is
	// the intended behaviour — silence staticcheck's deprecated check.
	legacy := ar.Spec.A2A.Authentication //nolint:staticcheck // SA1019: intentional read of deprecated field for projection
	if legacy == nil || legacy.SecretRef == nil {
		return
	}
	if ar.Spec.ExternalAuth == nil {
		ar.Spec.ExternalAuth = &AgentExternalAuth{}
	}
	if ar.Spec.ExternalAuth.SharedToken != nil {
		// Operator explicitly set the new shape — respect it.
		return
	}
	ar.Spec.ExternalAuth.SharedToken = &SharedTokenAuth{
		SecretRef: *legacy.SecretRef,
	}
}
