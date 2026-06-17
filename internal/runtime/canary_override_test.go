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

package runtime

import (
	"os"
	"path/filepath"
	"testing"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// A pod with no mounted canary override (the common case: stable pods and
// non-rollout agents) must report ok=false so LoadFromCRD keeps today's
// live-spec behaviour.
func TestLoadCanaryOverride_AbsentIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override.json")
	ov, ok, err := loadCanaryOverride(path)
	if err != nil {
		t.Fatalf("absent override should not error, got %v", err)
	}
	if ok || ov != nil {
		t.Fatalf("absent override should be (nil,false), got ov=%v ok=%v", ov, ok)
	}
}

// A candidate pod has the override CM mounted; loadCanaryOverride parses the
// candidate's provider refs from it.
func TestLoadCanaryOverride_ParsesProviderRefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override.json")
	data := `{"providerRefs":[{"name":"default","providerRef":{"name":"p-candidate"}}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	ov, ok, err := loadCanaryOverride(path)
	if err != nil || !ok {
		t.Fatalf("present override: ok=%v err=%v", ok, err)
	}
	if len(ov.ProviderRefs) != 1 || ov.ProviderRefs[0].ProviderRef.Name != "p-candidate" {
		t.Fatalf("unexpected providerRefs: %+v", ov.ProviderRefs)
	}
}

// A malformed override file is a hard error, not a silent fallback — a
// corrupt CM must fail loudly rather than quietly run stable config.
func TestLoadCanaryOverride_MalformedIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "override.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := loadCanaryOverride(path); err == nil || ok {
		t.Fatalf("malformed override should error, got ok=%v err=%v", ok, err)
	}
}

// applyCanaryOverride substitutes the candidate's provider refs onto the
// in-memory AgentRuntime so the runtime's existing live resolution runs
// against the candidate providers (the #1468 fix), not stable's.
func TestApplyCanaryOverride_SubstitutesProviders(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{}
	ar.Spec.Providers = []v1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "p-stable"}},
	}
	ov := &CanaryOverride{ProviderRefs: []v1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "p-candidate"}},
	}}

	applyCanaryOverride(ar, ov)

	if ar.Spec.Providers[0].ProviderRef.Name != "p-candidate" {
		t.Fatalf("expected candidate provider, got %q", ar.Spec.Providers[0].ProviderRef.Name)
	}
}

// applyCanaryOverrideFromMount ties load + apply together against the mounted
// path: a missing mount is a no-op; a present one substitutes the providers.
func TestApplyCanaryOverrideFromMount(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{}
	ar.Spec.Providers = []v1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "p-stable"}},
	}

	// No override mounted (path doesn't exist) → no-op.
	t.Setenv(envCanaryOverridePath, filepath.Join(t.TempDir(), "missing.json"))
	if err := applyCanaryOverrideFromMount(ar); err != nil {
		t.Fatalf("missing mount should be a no-op, got %v", err)
	}
	if ar.Spec.Providers[0].ProviderRef.Name != "p-stable" {
		t.Fatalf("missing mount changed providers to %q", ar.Spec.Providers[0].ProviderRef.Name)
	}

	// Override mounted → applied.
	path := filepath.Join(t.TempDir(), "override.json")
	if err := os.WriteFile(path,
		[]byte(`{"providerRefs":[{"name":"default","providerRef":{"name":"p-candidate"}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envCanaryOverridePath, path)
	if err := applyCanaryOverrideFromMount(ar); err != nil {
		t.Fatalf("present mount: %v", err)
	}
	if ar.Spec.Providers[0].ProviderRef.Name != "p-candidate" {
		t.Fatalf("present mount did not apply override, got %q", ar.Spec.Providers[0].ProviderRef.Name)
	}

	// A malformed mount propagates the error.
	bad := filepath.Join(t.TempDir(), "override.json")
	if err := os.WriteFile(bad, []byte(`{bad`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envCanaryOverridePath, bad)
	if err := applyCanaryOverrideFromMount(ar); err == nil {
		t.Fatal("malformed mount should propagate an error")
	}
}

// An empty override must not wipe the stable providers — guards against a
// candidate that overrides nothing (or a malformed/empty CM) blanking config.
func TestApplyCanaryOverride_EmptyKeepsStable(t *testing.T) {
	ar := &v1alpha1.AgentRuntime{}
	ar.Spec.Providers = []v1alpha1.NamedProviderRef{
		{Name: "default", ProviderRef: v1alpha1.ProviderRef{Name: "p-stable"}},
	}
	applyCanaryOverride(ar, &CanaryOverride{})

	if len(ar.Spec.Providers) != 1 || ar.Spec.Providers[0].ProviderRef.Name != "p-stable" {
		t.Fatalf("empty override must keep stable providers, got %+v", ar.Spec.Providers)
	}
}
