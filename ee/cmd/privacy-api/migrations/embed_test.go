/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package migrations

import (
	"strings"
	"testing"
)

func TestMigrationsEmbedded(t *testing.T) {
	data, err := MigrationsFS.ReadFile("000001_initial.up.sql")
	if err != nil {
		t.Fatalf("up migration not embedded: %v", err)
	}
	if !strings.Contains(string(data), "consent_grants") {
		t.Error("up migration must create the consent_grants column (the #1642 fix)")
	}
	if _, err := MigrationsFS.ReadFile("000001_initial.down.sql"); err != nil {
		t.Fatalf("down migration not embedded: %v", err)
	}
}
