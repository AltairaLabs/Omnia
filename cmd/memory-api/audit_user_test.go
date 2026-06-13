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
