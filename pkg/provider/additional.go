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

package provider

// HuggingFaceAdditionalConfig returns the PromptKit AdditionalConfig for a
// HuggingFace inference provider. A non-empty baseURL means a dedicated
// Inference Endpoint (dedicated=true); empty means the shared serverless
// Inference API (returns nil — no flags needed).
func HuggingFaceAdditionalConfig(baseURL string) map[string]any {
	if baseURL == "" {
		return nil
	}
	return map[string]any{"dedicated": true}
}
