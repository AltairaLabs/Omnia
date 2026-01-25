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
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestDefaultImageForFramework(t *testing.T) {
	tests := []struct {
		name      string
		framework *omniav1alpha1.FrameworkConfig
		want      string
	}{
		{
			name:      "nil framework returns default PromptKit image",
			framework: nil,
			want:      DefaultFrameworkImage,
		},
		{
			name: "LangChain framework returns LangChain image",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: omniav1alpha1.FrameworkTypeLangChain,
			},
			want: DefaultLangChainImage,
		},
		{
			name: "PromptKit framework returns PromptKit image",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: omniav1alpha1.FrameworkTypePromptKit,
			},
			want: DefaultFrameworkImage,
		},
		{
			name: "AutoGen framework returns default image (fallback)",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: omniav1alpha1.FrameworkTypeAutoGen,
			},
			want: DefaultFrameworkImage,
		},
		{
			name: "Unknown framework type returns default image",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: "unknown",
			},
			want: DefaultFrameworkImage,
		},
		{
			name: "Empty framework type returns default image",
			framework: &omniav1alpha1.FrameworkConfig{
				Type: "",
			},
			want: DefaultFrameworkImage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultImageForFramework(tt.framework)
			if got != tt.want {
				t.Errorf("defaultImageForFramework() = %v, want %v", got, tt.want)
			}
		})
	}
}
