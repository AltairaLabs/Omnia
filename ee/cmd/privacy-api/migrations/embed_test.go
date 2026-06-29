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

func TestOutboxMigrationEmbedded(t *testing.T) {
	data, err := MigrationsFS.ReadFile("000002_consent_outbox.up.sql")
	if err != nil {
		t.Fatalf("outbox up migration not embedded: %v", err)
	}
	if !strings.Contains(string(data), "consent_revocation_outbox") {
		t.Error("up migration must create consent_revocation_outbox")
	}
	if _, err := MigrationsFS.ReadFile("000002_consent_outbox.down.sql"); err != nil {
		t.Fatalf("outbox down migration not embedded: %v", err)
	}
}

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
