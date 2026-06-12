/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import "testing"

func TestProviderRoleInferenceConstant(t *testing.T) {
	if ProviderRoleInference != "inference" {
		t.Fatalf("ProviderRoleInference = %q, want inference", ProviderRoleInference)
	}
	p := &Provider{Spec: ProviderSpec{Role: ProviderRoleInference}}
	if got := p.EffectiveRole(); got != ProviderRoleInference {
		t.Fatalf("EffectiveRole() = %q, want inference", got)
	}
}
