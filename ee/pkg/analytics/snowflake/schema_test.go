/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package snowflake

import (
	"strings"
	"testing"
)

func TestSchemaDDL_ReturnsThreeStatements(t *testing.T) {
	ddl := SchemaDDL()
	if len(ddl) != 3 {
		t.Fatalf("expected 3 DDL statements, got %d", len(ddl))
	}
}

func TestSchemaDDL_ContainsCreateTableIfNotExists(t *testing.T) {
	ddl := SchemaDDL()
	for i, stmt := range ddl {
		if !strings.Contains(stmt, "CREATE TABLE IF NOT EXISTS") {
			t.Errorf("DDL[%d] missing CREATE TABLE IF NOT EXISTS", i)
		}
	}
}

func TestSchemaDDL_SessionsTable(t *testing.T) {
	ddl := SchemaDDL()
	sessStmt := ddl[0]
	expectedCols := []string{
		"session_id", "workspace_id", "agent_id", "user_id",
		"status", "namespace", "created_at", "updated_at",
		"message_count", "total_input_tokens", "total_output_tokens",
		"tags", "metadata",
	}
	for _, col := range expectedCols {
		if !strings.Contains(sessStmt, col) {
			t.Errorf("sessions DDL missing column %q", col)
		}
	}
}

func TestSchemaDDL_MessagesTable(t *testing.T) {
	ddl := SchemaDDL()
	msgStmt := ddl[1]
	expectedCols := []string{
		"message_id", "session_id", "role", "content",
		"input_tokens", "output_tokens", "sequence_num", "created_at",
	}
	for _, col := range expectedCols {
		if !strings.Contains(msgStmt, col) {
			t.Errorf("messages DDL missing column %q", col)
		}
	}
}

func TestSchemaDDL_WatermarksTable(t *testing.T) {
	ddl := SchemaDDL()
	wmStmt := ddl[2]
	expectedCols := []string{
		"table_name", "last_sync_at", "last_sync_rows", "updated_at",
	}
	for _, col := range expectedCols {
		if !strings.Contains(wmStmt, col) {
			t.Errorf("watermarks DDL missing column %q", col)
		}
	}
}

func TestAllTables(t *testing.T) {
	if len(AllTables) != 2 {
		t.Fatalf("expected 2 tables in AllTables, got %d", len(AllTables))
	}
	if AllTables[0] != TableSessions {
		t.Errorf("expected AllTables[0] = %q, got %q", TableSessions, AllTables[0])
	}
	if AllTables[1] != TableMessages {
		t.Errorf("expected AllTables[1] = %q, got %q", TableMessages, AllTables[1])
	}
}

func TestTableConstants(t *testing.T) {
	if TableSessions != "omnia_sessions" {
		t.Errorf("unexpected TableSessions: %q", TableSessions)
	}
	if TableMessages != "omnia_messages" {
		t.Errorf("unexpected TableMessages: %q", TableMessages)
	}
	if TableWatermarks != "_omnia_sync_watermarks" {
		t.Errorf("unexpected TableWatermarks: %q", TableWatermarks)
	}
}
