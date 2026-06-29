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
	"fmt"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/memory"
)

// PruneUserConsentCategory removes (or marks forgotten) all memory rows
// for the given user and consent category, respecting the workspace's
// MemoryPolicy consentRevocation.action:
//
//   - SoftDelete (default): sets forgotten=true + forgotten_at; rows are
//     hard-deleted after the policy grace window by the retention worker.
//   - HardDelete: removes rows immediately.
//   - Stop: no-op; existing rows are kept.
//
// Returns the number of rows affected (0 for Stop). The pruner must be
// wired via SetConsentEventPruner before calling this method.
func (s *MemoryService) PruneUserConsentCategory(
	ctx context.Context, workspaceID, userID, category string,
) (int64, error) {
	if s.consentEventPruner == nil {
		return 0, fmt.Errorf("memory: consent event pruner not configured")
	}

	action := s.resolveAction(ctx)

	var n int64
	var err error
	switch action {
	case omniav1alpha1.ConsentRevocationStop:
		return 0, nil
	case omniav1alpha1.ConsentRevocationHardDelete:
		n, err = s.consentEventPruner.HardDeleteUserConsentCategory(ctx, workspaceID, userID, category)
	default:
		n, err = s.consentEventPruner.SoftDeleteUserConsentCategory(ctx, workspaceID, userID, category)
	}
	if err != nil {
		return 0, err
	}

	if n > 0 {
		s.emitAuditEvent(ctx, &MemoryAuditEntry{
			EventType:   eventTypeConsentPrune,
			WorkspaceID: workspaceID,
			UserID:      userID,
			Metadata: map[string]string{
				metaKeyOperation: "consent_prune",
				"category":       category,
				"action":         string(action),
			},
		})
	}
	return n, nil
}

// resolveAction loads the MemoryPolicy (if a loader is wired) and
// returns the consentRevocation.action. Defaults to SoftDelete on load
// error or absent policy — fail-safe, never silently skips pruning.
func (s *MemoryService) resolveAction(ctx context.Context) omniav1alpha1.ConsentRevocationAction {
	if s.policyLoader == nil {
		return memory.ResolveConsentAction(nil)
	}
	policy, err := s.policyLoader.Load(ctx)
	if err != nil {
		s.log.Error(err, "consent event: policy load failed, using default action")
		return memory.ResolveConsentAction(nil)
	}
	if policy == nil {
		return memory.ResolveConsentAction(nil)
	}
	return memory.ResolveConsentAction(policy.Spec.ConsentRevocation)
}
