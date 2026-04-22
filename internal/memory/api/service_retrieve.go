/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"context"

	"github.com/altairalabs/omnia/internal/memory"
)

// retrieveOperationTag is the metadata tag emitted on multi-tier retrieval
// audit events. Kept as a constant so tests and grep lookups share a single
// source of truth.
const retrieveOperationTag = "retrieve_multi_tier"

// RetrieveMultiTier runs a multi-tier retrieval and emits one memory_accessed
// audit event. The store result is returned unchanged so the handler can
// forward tier annotations and scores to the client. Audit runs only on
// success so failed queries are not recorded as accesses.
func (s *MemoryService) RetrieveMultiTier(ctx context.Context, req memory.MultiTierRequest) (*memory.MultiTierResult, error) {
	result, err := s.store.RetrieveMultiTier(ctx, req)
	if err != nil {
		return nil, err
	}
	s.emitAuditEvent(ctx, &MemoryAuditEntry{
		EventType:   auditEventMemoryAccessed,
		WorkspaceID: req.WorkspaceID,
		UserID:      req.UserID,
		Metadata: map[string]string{
			"operation": retrieveOperationTag,
		},
	})
	return result, nil
}
