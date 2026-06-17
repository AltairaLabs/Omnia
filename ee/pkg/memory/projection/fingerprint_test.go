/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package projection

import (
	"testing"
	"time"
)

func TestFingerprintStableAndSensitive(t *testing.T) {
	at := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	a := Fingerprint(100, at)
	if a != Fingerprint(100, at) {
		t.Fatal("same inputs must give same fingerprint")
	}
	if a == Fingerprint(101, at) {
		t.Error("count change must change fingerprint")
	}
	if a == Fingerprint(100, at.Add(time.Second)) {
		t.Error("max observedAt change must change fingerprint")
	}
}
