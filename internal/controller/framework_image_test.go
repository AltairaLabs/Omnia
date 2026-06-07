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
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// promptkitImage builds a FrameworkImages map mapping the promptkit framework
// to img — the common test setup (agents default to promptkit).
func promptkitImage(img string) map[string]string {
	return map[string]string{string(omniav1alpha1.FrameworkTypePromptKit): img}
}

func arWithFramework(typ omniav1alpha1.FrameworkType, image string) *omniav1alpha1.AgentRuntime {
	ar := &omniav1alpha1.AgentRuntime{}
	if typ != "" || image != "" {
		ar.Spec.Framework = &omniav1alpha1.FrameworkConfig{Type: typ, Image: image}
	}
	return ar
}

func TestResolveFrameworkImage(t *testing.T) {
	r := &AgentRuntimeReconciler{FrameworkImages: map[string]string{
		"promptkit": "ghcr.io/altairalabs/omnia-runtime:v1",
		"langchain": "ghcr.io/altairalabs/omnia-langchain-runtime:v1",
	}}
	cases := []struct {
		name      string
		ar        *omniav1alpha1.AgentRuntime
		wantImage string
		wantOK    bool
	}{
		{"explicit override wins", arWithFramework(omniav1alpha1.FrameworkTypeLangChain, "custom:tag"), "custom:tag", true},
		{"langchain from map (the bug)", arWithFramework(omniav1alpha1.FrameworkTypeLangChain, ""), "ghcr.io/altairalabs/omnia-langchain-runtime:v1", true},
		{"promptkit from map", arWithFramework(omniav1alpha1.FrameworkTypePromptKit, ""), "ghcr.io/altairalabs/omnia-runtime:v1", true},
		{"nil framework -> promptkit", arWithFramework("", ""), "ghcr.io/altairalabs/omnia-runtime:v1", true},
		{"autogen -> blocked", arWithFramework(omniav1alpha1.FrameworkTypeAutoGen, ""), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, ok := r.resolveFrameworkImage(tc.ar)
			if img != tc.wantImage || ok != tc.wantOK {
				t.Fatalf("got (%q,%v) want (%q,%v)", img, ok, tc.wantImage, tc.wantOK)
			}
		})
	}
}

func TestResolveFrameworkImage_BareDevFallback(t *testing.T) {
	// No map configured (bare operator run) -> built-in :latest last resort.
	r := &AgentRuntimeReconciler{}
	img, ok := r.resolveFrameworkImage(arWithFramework(omniav1alpha1.FrameworkTypeLangChain, ""))
	if !ok || img != DefaultLangChainImage {
		t.Fatalf("bare-dev langchain: got (%q,%v) want (%q,true)", img, ok, DefaultLangChainImage)
	}
	// autogen has no built-in -> blocked even bare.
	if _, ok := r.resolveFrameworkImage(arWithFramework(omniav1alpha1.FrameworkTypeAutoGen, "")); ok {
		t.Fatal("autogen must block even with no map")
	}
}
