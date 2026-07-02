/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package license

import "github.com/go-logr/logr"

// LicensingURL is where operators obtain or renew a license.
const LicensingURL = "https://altairalabs.ai/licensing"

// NagIfUnlicensed logs a single, prominent reminder when lic is not a valid
// enterprise license — i.e. open-core, absent, or expired. It is meant to be
// called once at startup (never per-request), and it never blocks: Omnia's
// enterprise features keep working regardless. It says nothing when a valid
// enterprise license is present.
//
// This is the one place the "you should be paying" wording lives, so every
// service — control plane (via the validator) and data plane (via Client) —
// nags identically.
func NagIfUnlicensed(lic *License, log logr.Logger) {
	if lic != nil && lic.IsValidEnterprise() {
		return
	}
	log.Info("========================================================================")
	log.Info("Omnia Enterprise features are enabled without a valid license.")
	log.Info("These are commercial features. Please obtain a license at " + LicensingURL)
	log.Info("========================================================================")
}
