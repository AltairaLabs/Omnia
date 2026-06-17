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
	"encoding/json"
	"fmt"
	"os"

	v1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// CanaryOverride is the per-revision config the operator freezes for a canary
// and mounts into the candidate pod. The runtime applies it to its in-memory
// AgentRuntime before resolving providers, so a canary genuinely runs its own
// providers instead of self-reading the shared stable spec (the #1468 fix).
//
// It carries provider *refs*, not resolved specs: the runtime still resolves
// the referenced Provider CRDs + secrets live via the k8s client, exactly as
// for stable. Only the choice of which refs to use travels with the pod.
type CanaryOverride struct {
	// providerRefs replaces AgentRuntime.spec.providers for this pod.
	ProviderRefs []v1alpha1.NamedProviderRef `json:"providerRefs,omitempty"`
}

// loadCanaryOverride reads the mounted canary override file. It returns
// (nil, false, nil) when the file is absent — the common case for stable pods
// and non-rollout agents, which must keep today's live-spec behaviour.
func loadCanaryOverride(path string) (*CanaryOverride, bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator-mounted, fixed location
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read canary override %q: %w", path, err)
	}
	var ov CanaryOverride
	if err := json.Unmarshal(data, &ov); err != nil {
		return nil, false, fmt.Errorf("parse canary override %q: %w", path, err)
	}
	return &ov, true, nil
}

// applyCanaryOverride substitutes the candidate's provider refs onto the
// in-memory AgentRuntime. An empty override is a no-op so a candidate that
// overrides nothing (or a malformed/empty CM) never blanks the stable config.
func applyCanaryOverride(ar *v1alpha1.AgentRuntime, ov *CanaryOverride) {
	if ov == nil || len(ov.ProviderRefs) == 0 {
		return
	}
	ar.Spec.Providers = ov.ProviderRefs
}

// applyCanaryOverrideFromMount loads the mounted canary override (if any) and
// applies it to ar. A no-op on stable / non-rollout pods, where no override is
// mounted. Kept as a single call so the AgentRuntime load path stays flat.
func applyCanaryOverrideFromMount(ar *v1alpha1.AgentRuntime) error {
	ov, ok, err := loadCanaryOverride(getEnvOrDefault(envCanaryOverridePath, defaultCanaryOverridePath))
	if err != nil {
		return err
	}
	if ok {
		applyCanaryOverride(ar, ov)
	}
	return nil
}
