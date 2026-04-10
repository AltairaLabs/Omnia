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
	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
)

// effectiveSecretRef delegates to the shared pkg/k8s.EffectiveSecretRef.
//
// Provider CRD fields (type, model, baseURL, defaults, pricing) and the API
// key secret are NOT propagated to pods via env vars — the runtime reads the
// Provider CRD directly via the k8s client (see internal/runtime/config_crd.go).
// This helper is retained only for the deployment-builder's secret change
// detection hashing path.
func effectiveSecretRef(provider *omniav1alpha1.Provider) *omniav1alpha1.SecretKeyRef {
	return k8s.EffectiveSecretRef(provider)
}

// isPromptKit returns true if the AgentRuntime uses the PromptKit framework.
func isPromptKit(spec *omniav1alpha1.AgentRuntimeSpec) bool {
	if spec.Framework == nil {
		return true // default is PromptKit
	}
	return spec.Framework.Type == omniav1alpha1.FrameworkTypePromptKit
}

// hasEvalsEnabled returns true if the AgentRuntime has evals enabled.
func hasEvalsEnabled(spec *omniav1alpha1.AgentRuntimeSpec) bool {
	return spec.Evals != nil && spec.Evals.Enabled
}
