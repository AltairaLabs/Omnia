/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package consolidation

import (
	"context"
	"fmt"
	"time"

	coreconsol "github.com/altairalabs/omnia/internal/memory/consolidation"
)

// Store is the minimal interface the applier needs from the memory
// store. Real implementation lives in internal/memory/postgres; tests
// substitute a MockStore.
type Store interface {
	SaveSummary(ctx context.Context, w SummaryWrite) (string, error)
	Supersede(ctx context.Context, w SupersedeWrite) error
	Rescope(ctx context.Context, w RescopeWrite) error
	Invalidate(ctx context.Context, w InvalidateWrite) error
	MergeEntities(ctx context.Context, w MergeWrite) error
	Discard(ctx context.Context, w DiscardWrite) error
	Rescore(ctx context.Context, w RescoreWrite) error
}

// Auditor receives one entry per action attempt (applied or rejected).
type Auditor interface {
	LogConsolidation(ctx context.Context, entry AuditEntry) error
}

// AuditEntry is one consolidation-action audit row.
type AuditEntry struct {
	RunID       string
	WorkspaceID string
	PackRef     string
	ActionKind  ActionKind
	Outcome     string // "applied" / "rejected_validation" / "apply_failed"
	Reason      string // only set when Outcome != "applied"
	TargetIDs   []string
	Now         time.Time
}

// ApplyContext is the per-pass context the applier needs to populate
// lineage columns and audit rows on every write.
type ApplyContext struct {
	WorkspaceID string
	RunID       string
	PackRef     string
	Now         time.Time
}

// The *Write DTOs are defined in internal/memory/consolidation and
// re-exported here as type aliases so the Store interface, Applier, and
// their tests compile unchanged.

// SummaryWrite captures a CreateSummary apply.
type SummaryWrite = coreconsol.SummaryWrite

// SupersedeWrite captures a Supersede apply.
type SupersedeWrite = coreconsol.SupersedeWrite

// RescopeWrite captures a Rescope apply.
type RescopeWrite = coreconsol.RescopeWrite

// InvalidateWrite captures an Invalidate apply.
type InvalidateWrite = coreconsol.InvalidateWrite

// MergeWrite captures a MergeEntities apply.
type MergeWrite = coreconsol.MergeWrite

// DiscardWrite captures a Discard apply.
type DiscardWrite = coreconsol.DiscardWrite

// RescoreWrite captures a Rescore apply.
type RescoreWrite = coreconsol.RescoreWrite

// Outcome values for AuditEntry.Outcome.
const (
	OutcomeApplied            = "applied"
	OutcomeRejectedValidation = "rejected_validation"
	OutcomeApplyFailed        = "apply_failed"
)

// Applier translates accepted action Results into store writes,
// populating lineage columns and emitting audit entries.
type Applier struct {
	store   Store
	auditor Auditor
}

// NewApplier constructs an Applier without auditing (tests + legacy callers).
func NewApplier(store Store) *Applier {
	return &Applier{store: store}
}

// NewApplierWithAudit constructs an Applier with auditing.
func NewApplierWithAudit(store Store, auditor Auditor) *Applier {
	return &Applier{store: store, auditor: auditor}
}

// Apply iterates accepted actions and dispatches each to the matching
// store method. Rejected actions are skipped (but audit-logged).
// Returns the first error from any single write; the caller is
// responsible for the transactional boundary (worker holds the
// per-workspace Postgres advisory lock).
func (a *Applier) Apply(ctx context.Context, ac ApplyContext, results []Result) error {
	if ac.Now.IsZero() {
		ac.Now = time.Now()
	}
	for _, r := range results {
		entry := AuditEntry{
			RunID:       ac.RunID,
			WorkspaceID: ac.WorkspaceID,
			PackRef:     ac.PackRef,
			ActionKind:  r.Action.Kind(),
			TargetIDs:   modifyingTargets(r.Action),
			Now:         ac.Now,
		}
		if !r.Accepted {
			entry.Outcome = OutcomeRejectedValidation
			entry.Reason = r.Reason
			a.emitAudit(ctx, entry)
			continue
		}
		if err := a.applyOne(ctx, ac, r.Action); err != nil {
			entry.Outcome = OutcomeApplyFailed
			entry.Reason = err.Error()
			a.emitAudit(ctx, entry)
			return fmt.Errorf("apply %s: %w", r.Action.Kind(), err)
		}
		entry.Outcome = OutcomeApplied
		a.emitAudit(ctx, entry)
	}
	return nil
}

func (a *Applier) emitAudit(ctx context.Context, e AuditEntry) {
	if a.auditor == nil {
		return
	}
	_ = a.auditor.LogConsolidation(ctx, e)
}

func (a *Applier) applyOne(ctx context.Context, ac ApplyContext, act Action) error {
	switch x := act.(type) {
	case CreateSummaryAction:
		_, err := a.store.SaveSummary(ctx, SummaryWrite{
			WorkspaceID:    ac.WorkspaceID,
			Scope:          x.Scope,
			Content:        x.Content,
			Metadata:       x.Metadata,
			FromIDs:        x.FromIDs,
			PromotedByPack: ac.PackRef,
			PromotedAt:     ac.Now,
		})
		return err
	case SupersedeAction:
		return a.store.Supersede(ctx, SupersedeWrite{
			WorkspaceID:    ac.WorkspaceID,
			TargetIDs:      x.TargetIDs,
			WithID:         x.WithID,
			PromotedByPack: ac.PackRef,
			PromotedAt:     ac.Now,
		})
	case RescopeAction:
		return a.store.Rescope(ctx, RescopeWrite{
			WorkspaceID:    ac.WorkspaceID,
			TargetIDs:      x.TargetIDs,
			NewScope:       x.NewScope,
			Reason:         x.Reason,
			PromotedByPack: ac.PackRef,
			PromotedAt:     ac.Now,
		})
	case InvalidateAction:
		return a.store.Invalidate(ctx, InvalidateWrite{
			WorkspaceID:    ac.WorkspaceID,
			TargetIDs:      x.TargetIDs,
			ValidUntil:     x.ValidUntil,
			Reason:         x.Reason,
			PromotedByPack: ac.PackRef,
			PromotedAt:     ac.Now,
		})
	case MergeEntitiesAction:
		return a.store.MergeEntities(ctx, MergeWrite{
			WorkspaceID:    ac.WorkspaceID,
			CanonicalID:    x.CanonicalID,
			MergeIDs:       x.MergeIDs,
			PromotedByPack: ac.PackRef,
			PromotedAt:     ac.Now,
		})
	case DiscardAction:
		return a.store.Discard(ctx, DiscardWrite{
			WorkspaceID:    ac.WorkspaceID,
			TargetIDs:      x.TargetIDs,
			Reason:         x.Reason,
			PromotedByPack: ac.PackRef,
			PromotedAt:     ac.Now,
		})
	case RescoreAction:
		return a.store.Rescore(ctx, RescoreWrite{
			WorkspaceID:    ac.WorkspaceID,
			TargetID:       x.TargetID,
			Importance:     x.Importance,
			Confidence:     x.Confidence,
			PromotedByPack: ac.PackRef,
			PromotedAt:     ac.Now,
		})
	default:
		return fmt.Errorf("unknown action: %s", act.Kind())
	}
}
