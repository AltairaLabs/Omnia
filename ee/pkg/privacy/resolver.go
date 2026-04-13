/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"encoding/json"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// facadePolicy is the subset of EffectivePolicy exposed to the facade via
// GET /api/v1/privacy-policy. Encryption config stays server-side.
type facadePolicy struct {
	Recording omniav1alpha1.RecordingConfig `json:"recording"`
}

// facadePolicyJSON marshals the facade-visible subset of an effective policy.
// Returns nil bytes and no error when p is nil.
func facadePolicyJSON(p *EffectivePolicy) (json.RawMessage, error) {
	if p == nil {
		return nil, nil
	}
	return json.Marshal(facadePolicy{Recording: p.Recording})
}

// ResolveEffectivePolicy adapts PolicyWatcher to the api.PolicyResolver
// interface defined in internal/session/api. Returns (jsonBytes, true) when
// a policy applies, or (nil, false) when none applies.
func (w *PolicyWatcher) ResolveEffectivePolicy(namespace, agentName string) (json.RawMessage, bool) {
	eff := w.GetEffectivePolicy(namespace, agentName)
	if eff == nil {
		return nil, false
	}
	raw, err := facadePolicyJSON(eff)
	if err != nil || len(raw) == 0 {
		return nil, false
	}
	return raw, true
}
