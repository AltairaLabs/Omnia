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

// ProjectLegacyFacadeA2A copies the deprecated top-level spec.a2a into
// spec.facade.a2a when the new field is unset, so legacy CRs continue
// to work after the migration. Mirrors ProjectLegacyA2AAuth's pattern.
//
// Intended to be called by the AgentRuntime reconciler on each Get'd
// AgentRuntime AND by config_crd loaders in the agent (since the
// operator's projection is in-memory only — pods read the persisted
// spec). Wiring of those call sites is performed in subsequent tasks;
// the helper exists in isolation here.
//
// Uses DeepCopy on the source so the projected Facade.A2A does not
// share inner pointer fields with the deprecated Spec.A2A — protects
// future callers from accidental cross-field mutation.
func ProjectLegacyFacadeA2A(ar *AgentRuntime) {
	if ar == nil {
		return
	}
	if ar.Spec.A2A == nil { //nolint:staticcheck // SA1019: intentional read of deprecated field for projection
		return
	}
	if ar.Spec.Facade.A2A != nil {
		return // explicit new field wins
	}
	ar.Spec.Facade.A2A = ar.Spec.A2A.DeepCopy() //nolint:staticcheck // SA1019: intentional read of deprecated field for projection
}
