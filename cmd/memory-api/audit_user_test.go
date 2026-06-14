/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	"github.com/altairalabs/omnia/pkg/logging"
)

// SEC-6: the memory audit adapter must carry the subject user (hashed) so the
// audit trail can answer "who accessed/deleted user X's memories".
func TestAuditLoggerAdapter_CarriesHashedUser(t *testing.T) {
	rec := &recordingInner{}
	a := &auditLoggerAdapter{inner: rec}

	a.LogEvent(context.Background(), &memoryapi.MemoryAuditEntry{
		EventType:   "memory_accessed",
		WorkspaceID: "ws-1",
		UserID:      "user-abc",
	})

	if assert.Len(t, rec.got, 1) {
		assert.Equal(t, logging.HashID("user-abc"), rec.got[0].Metadata["userHash"],
			"audit entry must carry the hashed subject user")
		assert.NotEqual(t, "user-abc", rec.got[0].Metadata["userHash"], "must not store the raw id")
	}
}

// The privacy middleware emits enforcement entries with no MemoryID/Kind/UserID
// — only EventType, WorkspaceID, and a reason in Metadata. The adapter must
// forward these (event type + workspace + reason) intact so the audit_log row
// is queryable by event_type + workspace.
func TestAuditLoggerAdapter_ForwardsEnforcementEntries(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		reason    string
	}{
		{"opt-out block", "memory_write_blocked", "opt-out"},
		{"pii redaction", "pii_redacted", "memory:identity"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &recordingInner{}
			a := &auditLoggerAdapter{inner: rec}

			a.LogEvent(context.Background(), &memoryapi.MemoryAuditEntry{
				EventType:   tc.eventType,
				WorkspaceID: "ws-enf",
				Metadata:    map[string]string{"reason": tc.reason},
			})

			if assert.Len(t, rec.got, 1) {
				assert.Equal(t, tc.eventType, rec.got[0].EventType)
				assert.Equal(t, "ws-enf", rec.got[0].Workspace)
				assert.Equal(t, tc.reason, rec.got[0].Metadata["reason"])
				assert.NotContains(t, rec.got[0].Metadata, "userHash",
					"enforcement entries carry no subject user")
			}
		})
	}
}
