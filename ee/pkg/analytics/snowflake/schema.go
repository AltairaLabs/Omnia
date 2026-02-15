/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package snowflake

// Table name constants.
const (
	TableSessions    = "omnia_sessions"
	TableMessages    = "omnia_messages"
	TableEvalResults = "omnia_eval_results"
	TableWatermarks  = "_omnia_sync_watermarks"
)

// AllTables lists all destination tables managed by this provider.
var AllTables = []string{TableSessions, TableMessages, TableEvalResults}

// DDL statements for creating the Snowflake analytics tables.
const createSessionsTable = `CREATE TABLE IF NOT EXISTS omnia_sessions (
    session_id VARCHAR(36) PRIMARY KEY,
    workspace_id VARCHAR(255),
    agent_id VARCHAR(255),
    user_id VARCHAR(255),
    status VARCHAR(50),
    namespace VARCHAR(255),
    created_at TIMESTAMP_TZ,
    updated_at TIMESTAMP_TZ,
    message_count INT,
    total_input_tokens INT,
    total_output_tokens INT,
    tags ARRAY,
    metadata VARIANT
)`

const createMessagesTable = `CREATE TABLE IF NOT EXISTS omnia_messages (
    message_id VARCHAR(36),
    session_id VARCHAR(36),
    role VARCHAR(50),
    content TEXT,
    input_tokens INT,
    output_tokens INT,
    sequence_num INT,
    created_at TIMESTAMP_TZ
)`

const createEvalResultsTable = `CREATE TABLE IF NOT EXISTS omnia_eval_results (
    id VARCHAR(36) PRIMARY KEY,
    session_id VARCHAR(36),
    message_id VARCHAR(36),
    agent_name VARCHAR(255),
    namespace VARCHAR(255),
    promptpack_name VARCHAR(255),
    promptpack_version VARCHAR(100),
    eval_id VARCHAR(255),
    eval_type VARCHAR(100),
    trigger VARCHAR(100),
    passed BOOLEAN,
    score FLOAT,
    details VARIANT,
    duration_ms INT,
    judge_tokens INT,
    judge_cost_usd FLOAT,
    source VARCHAR(100),
    created_at TIMESTAMP_TZ
)`

const createWatermarksTable = `CREATE TABLE IF NOT EXISTS _omnia_sync_watermarks (
    table_name VARCHAR NOT NULL PRIMARY KEY,
    last_sync_at TIMESTAMP_TZ NOT NULL,
    last_sync_rows BIGINT DEFAULT 0,
    updated_at TIMESTAMP_TZ DEFAULT CURRENT_TIMESTAMP()
)`

// SchemaDDL returns all DDL statements needed to initialize the schema.
func SchemaDDL() []string {
	return []string{
		createSessionsTable,
		createMessagesTable,
		createEvalResultsTable,
		createWatermarksTable,
	}
}
