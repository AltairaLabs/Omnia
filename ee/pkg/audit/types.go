/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package audit

import "time"

// Event type constants for audit logging.
const (
	EventSessionCreated      = "session_created"
	EventSessionAccessed     = "session_accessed"
	EventSessionSearched     = "session_searched"
	EventSessionExported     = "session_exported"
	EventSessionDeleted      = "session_deleted"
	EventPIIRedacted         = "pii_redacted"
	EventDecryptionRequested = "decryption_requested"
)

// Entry represents a single audit log row in the database.
type Entry struct {
	ID          int64             `json:"id"`
	Timestamp   time.Time         `json:"timestamp"`
	EventType   string            `json:"eventType"`
	SessionID   string            `json:"sessionId,omitempty"`
	UserID      string            `json:"userId,omitempty"`
	Workspace   string            `json:"workspace,omitempty"`
	AgentName   string            `json:"agentName,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Query       string            `json:"query,omitempty"`
	ResultCount int               `json:"resultCount,omitempty"`
	IPAddress   string            `json:"ipAddress,omitempty"`
	UserAgent   string            `json:"userAgent,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// QueryOpts defines filters for querying audit log entries.
type QueryOpts struct {
	SessionID  string
	UserID     string
	Workspace  string
	EventTypes []string
	From       time.Time
	To         time.Time
	Limit      int
	Offset     int
}

// QueryResult is the result of an audit log query.
type QueryResult struct {
	Entries []*Entry `json:"entries"`
	Total   int64    `json:"total"`
	HasMore bool     `json:"hasMore"`
}
