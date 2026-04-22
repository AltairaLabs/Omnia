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

package controller

import (
	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// projectLegacyA2AAuth folds the deprecated spec.a2a.authentication.secretRef
// into spec.externalAuth.sharedToken so that the new validator chain can
// treat both shapes uniformly.
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
// are no-ops once the projection has happened. PR 2a ships this helper
// behind no callers — the chain runner in PR 2b is what invokes it.
//
// The function does not modify the legacy field. We leave it visible on
// the spec so operators see exactly what they configured; status emits
// a deprecation warning condition (added in PR 2b) when the old field is
// still populated.
func projectLegacyA2AAuth(ar *omniav1alpha1.AgentRuntime) {
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
		ar.Spec.ExternalAuth = &omniav1alpha1.AgentExternalAuth{}
	}
	if ar.Spec.ExternalAuth.SharedToken != nil {
		// Operator explicitly set the new shape — respect it.
		return
	}
	ar.Spec.ExternalAuth.SharedToken = &omniav1alpha1.SharedTokenAuth{
		SecretRef: *legacy.SecretRef,
	}
}
