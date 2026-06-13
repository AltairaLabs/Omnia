/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import (
	"time"

	memoryv1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// Validation reject reasons. Stable strings for audit and metrics.
// ReasonPIIBlocked is defined in pii_gate.go.
const (
	ReasonInstitutionalWriteBlocked = "institutional_write_blocked"
	ReasonMutabilityBlocked         = "mutability_blocked"
	ReasonAnonymityBelowThreshold   = "anonymity_below_threshold"
	ReasonScopeOutsideWorkspace     = "scope_outside_workspace"
	ReasonScopeWideningUnsupported  = "scope_widening_unsupported"
	ReasonTargetUnknown             = "target_unknown"
	ReasonShapeInvalid              = "shape_invalid"
)

// scopeWideningWorkspace is the only maxScopeWidening value supported in v1:
// rescope writes are confined to the originating workspace.
const scopeWideningWorkspace = "workspace"

// ValidatorOptions configures the validator's policy gates.
type ValidatorOptions struct {
	WorkspaceID string
	Gates       memoryv1.MemoryConsolidationSafetyGates
	// PIIRedactor is consulted by the PII gate when
	// Gates.RequirePIIRedaction is true. Nil disables the gate.
	PIIRedactor PIIRedactor
}

// ValidationContext is the per-pass context the validator needs: row
// metadata (mutability, scope) and bucket stats (cross-user counts)
// from the pre-filter.
type ValidationContext struct {
	// RowMutability maps target row ID → mutability string. Rows not
	// in the map are treated as unknown (reject with ReasonTargetUnknown).
	RowMutability map[string]string
	// RowScope maps target row ID → current scope.
	RowScope map[string]Scope
	// BucketDistinctUserCount is the cross-user count for the bucket
	// the actions came from (used by k-anonymity gate on rescope).
	BucketDistinctUserCount int
}

// Result reports one action's validation outcome.
type Result struct {
	Action   Action
	Accepted bool
	Reason   string
}

// Validator runs validation gates against a proposed action list.
type Validator struct {
	opts    ValidatorOptions
	piiGate *PIIGate
}

// NewValidator constructs a Validator.
func NewValidator(opts ValidatorOptions) *Validator {
	return &Validator{opts: opts, piiGate: NewPIIGate(opts.PIIRedactor)}
}

// Validate runs all gates against each action and returns one Result
// per input action.
func (v *Validator) Validate(actions []Action, ctx ValidationContext) []Result {
	out := make([]Result, 0, len(actions))
	for _, a := range actions {
		out = append(out, v.validateOne(a, ctx))
	}
	return out
}

func (v *Validator) validateOne(a Action, ctx ValidationContext) Result {
	if reason := v.checkShape(a); reason != "" {
		return Result{Action: a, Reason: reason}
	}
	if reason := v.checkMutability(a, ctx); reason != "" {
		return Result{Action: a, Reason: reason}
	}
	if reason := v.checkInstitutionalWrite(a); reason != "" {
		return Result{Action: a, Reason: reason}
	}
	if reason := v.checkAnonymity(a, ctx); reason != "" {
		return Result{Action: a, Reason: reason}
	}
	if reason := v.checkScope(a); reason != "" {
		return Result{Action: a, Reason: reason}
	}
	if reason := v.piiGate.Check(a, v.opts.Gates); reason != "" {
		return Result{Action: a, Reason: reason}
	}
	return Result{Action: a, Accepted: true}
}

// checkShape rejects malformed actions: empty FromIDs on CreateSummary,
// empty TargetIDs on mutating actions, etc.
func (v *Validator) checkShape(a Action) string {
	if shapeValid(a) {
		return ""
	}
	return ReasonShapeInvalid
}

// shapeValid returns true when the action's required fields are all
// present. One small predicate per action kind keeps cognitive
// complexity low (Sonar go:S3776).
func shapeValid(a Action) bool {
	switch x := a.(type) {
	case CreateSummaryAction:
		return len(x.FromIDs) > 0 && x.Content != ""
	case SupersedeAction:
		return len(x.TargetIDs) > 0 && x.WithID != ""
	case RescopeAction:
		return len(x.TargetIDs) > 0
	case InvalidateAction:
		return len(x.TargetIDs) > 0 && x.ValidUntil.After(time.Now())
	case MergeEntitiesAction:
		return x.CanonicalID != "" && len(x.MergeIDs) > 0
	case DiscardAction:
		return len(x.TargetIDs) > 0
	case RescoreAction:
		return x.TargetID != ""
	}
	return true
}

// checkMutability rejects actions whose target rows are not 'mutable'.
// CreateSummary may reference rows of any mutability via from_ids
// (read-only reference, not modification).
func (v *Validator) checkMutability(a Action, ctx ValidationContext) string {
	targets := modifyingTargets(a)
	if len(targets) == 0 {
		return ""
	}
	for _, id := range targets {
		m, ok := ctx.RowMutability[id]
		if !ok {
			return ReasonTargetUnknown
		}
		if m != MutabilityMutable {
			return ReasonMutabilityBlocked
		}
	}
	return ""
}

// modifyingTargets returns the IDs of rows the action mutates.
// CreateSummary returns nothing (it only references via from_ids).
// MergeEntities mutates only the merge_ids; the canonical is the
// destination, not a target.
func modifyingTargets(a Action) []string {
	switch act := a.(type) {
	case SupersedeAction:
		return act.TargetIDs
	case RescopeAction:
		return act.TargetIDs
	case InvalidateAction:
		return act.TargetIDs
	case MergeEntitiesAction:
		return act.MergeIDs
	case DiscardAction:
		return act.TargetIDs
	case RescoreAction:
		return []string{act.TargetID}
	default:
		return nil
	}
}

// checkInstitutionalWrite rejects rescope actions targeting
// (ws, null, null) — institutional promotion is deferred to the
// poisoning-defenses spec's proposal-queue flow.
func (v *Validator) checkInstitutionalWrite(a Action) string {
	r, ok := a.(RescopeAction)
	if !ok {
		return ""
	}
	if r.NewScope.Shape() == ScopeShapeInstitutional {
		return ReasonInstitutionalWriteBlocked
	}
	return ""
}

// checkAnonymity enforces minDistinctUserCount on rescope actions
// based on the action's destination shape.
func (v *Validator) checkAnonymity(a Action, ctx ValidationContext) string {
	r, ok := a.(RescopeAction)
	if !ok {
		return ""
	}
	var key string
	switch r.NewScope.Shape() {
	case ScopeShapeAgentScoped:
		key = SafetyGateAgentScoped
	case ScopeShapeUserScoped:
		key = SafetyGateUserScoped
	default:
		return ""
	}
	threshold := v.opts.Gates.MinDistinctUserCount[key]
	if threshold == 0 {
		return ""
	}
	if int32(ctx.BucketDistinctUserCount) < threshold {
		return ReasonAnonymityBelowThreshold
	}
	return ""
}

// checkScope ensures rescope actions write within the originating
// workspace.
func (v *Validator) checkScope(a Action) string {
	r, ok := a.(RescopeAction)
	if !ok {
		return ""
	}
	// maxScopeWidening caps how far a rescope may widen scope. Only
	// "workspace" (confine to the originating workspace) is supported in v1;
	// any other value fails closed rather than silently widening. Empty means
	// the resolved default ("workspace"). Previously this field was never
	// consulted (#1334) — the cross-workspace confinement below already
	// enforced the practical cap, but an unsupported value now rejects.
	if w := v.opts.Gates.MaxScopeWidening; w != "" && w != scopeWideningWorkspace {
		return ReasonScopeWideningUnsupported
	}
	if r.NewScope.WorkspaceID != v.opts.WorkspaceID {
		return ReasonScopeOutsideWorkspace
	}
	return ""
}
