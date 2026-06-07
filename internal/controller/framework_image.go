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

import omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"

// frameworkTypeKey returns the framework type string for an AgentRuntime,
// defaulting to "promptkit" when unset (the historical default).
func frameworkTypeKey(ar *omniav1alpha1.AgentRuntime) string {
	if ar.Spec.Framework == nil || ar.Spec.Framework.Type == "" {
		return string(omniav1alpha1.FrameworkTypePromptKit)
	}
	return string(ar.Spec.Framework.Type)
}

// resolveFrameworkImage selects the runtime image for an AgentRuntime:
//  1. explicit spec.framework.image override
//  2. the configured FrameworkImages entry for the framework type
//  3. a built-in :latest default (promptkit/langchain only) — bare-dev last resort
//  4. otherwise ("", false) — the caller must fail loudly, NOT substitute another image
func (r *AgentRuntimeReconciler) resolveFrameworkImage(ar *omniav1alpha1.AgentRuntime) (string, bool) {
	if ar.Spec.Framework != nil && ar.Spec.Framework.Image != "" {
		return ar.Spec.Framework.Image, true
	}
	key := frameworkTypeKey(ar)
	if img, ok := r.FrameworkImages[key]; ok && img != "" {
		return img, true
	}
	if img := builtinDefaultImage(key); img != "" {
		return img, true
	}
	return "", false
}

// builtinDefaultImage returns the hardcoded :latest image for the framework
// type, or "" when none exists. ONLY promptkit and langchain have built-ins;
// autogen and unknown types return "" so they block instead of silently
// running PromptKit (issue #1206).
func builtinDefaultImage(frameworkType string) string {
	switch frameworkType {
	case string(omniav1alpha1.FrameworkTypePromptKit):
		return DefaultFrameworkImage
	case string(omniav1alpha1.FrameworkTypeLangChain):
		return DefaultLangChainImage
	default:
		return ""
	}
}
