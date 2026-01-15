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
	autoscalingv2 "k8s.io/api/autoscaling/v2"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Helper functions for creating pointers
func ptr[T any](v T) *T {
	return &v
}

func ptrSelectPolicy(p autoscalingv2.ScalingPolicySelect) *autoscalingv2.ScalingPolicySelect {
	return &p
}

func boolPtr(b bool) *bool {
	return &b
}

// providerKeyMapping maps provider types to their expected API key env var names.
// This is a package-level variable to avoid duplication across functions.
var providerKeyMapping = map[omniav1alpha1.ProviderType][]string{
	omniav1alpha1.ProviderTypeClaude: {"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"},
	omniav1alpha1.ProviderTypeOpenAI: {"OPENAI_API_KEY", "OPENAI_TOKEN"},
	omniav1alpha1.ProviderTypeGemini: {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
}
