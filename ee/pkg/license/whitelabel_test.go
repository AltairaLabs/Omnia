/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package license

import "testing"

// TestWhiteLabelFeature verifies the whiteLabel entitlement is granted only to
// enterprise-tier licenses (dev/enterprise), never to open-core. This is what
// makes dashboard white-labeling a real entitlement rather than a free toggle.
func TestWhiteLabelFeature(t *testing.T) {
	if !DevLicense().Features.WhiteLabel {
		t.Error("expected DevLicense to grant WhiteLabel")
	}
	if OpenCoreLicense().Features.WhiteLabel {
		t.Error("expected OpenCoreLicense to NOT grant WhiteLabel")
	}
}
