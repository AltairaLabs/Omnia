/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package consolidation implements the LLM-driven memory consolidation
// worker. See docs/local-backlog/2026-05-22-memory-consolidation-design.md
// for the full design.
package consolidation

import (
	"encoding/json"
	"fmt"
	"time"
)

// PreFilterAxis identifies one of the three pre-filter SQL builders.
type PreFilterAxis string

// PreFilterAxis values match the MemoryPolicy CRD's functionRefs keys.
const (
	AxisStaleObservations         PreFilterAxis = "staleObservations"
	AxisCrossScopeCandidates      PreFilterAxis = "crossScopeCandidates"
	AxisEntityDuplicateCandidates PreFilterAxis = "entityDuplicateCandidates"
)

// String returns the axis as a string. Mirrors the JSON tag value.
func (a PreFilterAxis) String() string { return string(a) }

// Scope is the (workspace_id, agent_id?, user_id?) tuple a memory row
// (or a rescope action) targets. Fields with empty values are
// equivalent to NULL in the database.
type Scope struct {
	WorkspaceID string `json:"workspaceID"`
	AgentID     string `json:"agentID,omitempty"`
	UserID      string `json:"userID,omitempty"`
}

// ScopeShape labels a Scope by its nullability pattern. Maps 1:1 to
// the four operator-facing tier names.
type ScopeShape string

// ScopeShape values.
const (
	ScopeShapeInstitutional ScopeShape = "institutional"
	ScopeShapeAgentScoped   ScopeShape = "agent-scoped"
	ScopeShapeUserScoped    ScopeShape = "user-scoped"
	ScopeShapeUserForAgent  ScopeShape = "user-for-agent"
)

// Shape returns the ScopeShape this Scope represents.
func (s Scope) Shape() ScopeShape {
	hasAgent := s.AgentID != ""
	hasUser := s.UserID != ""
	switch {
	case hasAgent && hasUser:
		return ScopeShapeUserForAgent
	case hasUser:
		return ScopeShapeUserScoped
	case hasAgent:
		return ScopeShapeAgentScoped
	default:
		return ScopeShapeInstitutional
	}
}

// ActionKind labels each typed action. The pack emits actions as a
// JSON array; ActionKind is read from the "action" field.
type ActionKind string

// ActionKind values.
const (
	ActionCreateSummary ActionKind = "create_summary"
	ActionSupersede     ActionKind = "supersede"
	ActionRescope       ActionKind = "rescope"
	ActionInvalidate    ActionKind = "invalidate"
	ActionMergeEntities ActionKind = "merge_entities"
	ActionDiscard       ActionKind = "discard"
	ActionRescore       ActionKind = "rescore"
)

// Action is the common interface every typed action implements.
type Action interface {
	Kind() ActionKind
}

// CreateSummaryAction creates a new summary row from existing rows.
type CreateSummaryAction struct {
	FromIDs  []string          `json:"fromIDs"`
	Scope    Scope             `json:"scope"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Kind returns ActionCreateSummary.
func (CreateSummaryAction) Kind() ActionKind { return ActionCreateSummary }

// SupersedeAction marks target rows as superseded by another row.
type SupersedeAction struct {
	TargetIDs []string `json:"targetIDs"`
	WithID    string   `json:"withID"`
}

// Kind returns ActionSupersede.
func (SupersedeAction) Kind() ActionKind { return ActionSupersede }

// RescopeAction changes the scope tuple of target rows.
type RescopeAction struct {
	TargetIDs []string `json:"targetIDs"`
	NewScope  Scope    `json:"newScope"`
	Reason    string   `json:"reason,omitempty"`
}

// Kind returns ActionRescope.
func (RescopeAction) Kind() ActionKind { return ActionRescope }

// InvalidateAction sets valid_until on target rows.
type InvalidateAction struct {
	TargetIDs  []string  `json:"targetIDs"`
	ValidUntil time.Time `json:"validUntil"`
	Reason     string    `json:"reason,omitempty"`
}

// Kind returns ActionInvalidate.
func (InvalidateAction) Kind() ActionKind { return ActionInvalidate }

// MergeEntitiesAction collapses duplicate entities into a canonical one.
type MergeEntitiesAction struct {
	CanonicalID string   `json:"canonicalID"`
	MergeIDs    []string `json:"mergeIDs"`
}

// Kind returns ActionMergeEntities.
func (MergeEntitiesAction) Kind() ActionKind { return ActionMergeEntities }

// DiscardAction removes target rows (soft delete via valid_until).
type DiscardAction struct {
	TargetIDs []string `json:"targetIDs"`
	Reason    string   `json:"reason,omitempty"`
}

// Kind returns ActionDiscard.
func (DiscardAction) Kind() ActionKind { return ActionDiscard }

// RescoreAction adjusts importance/confidence on a single row.
type RescoreAction struct {
	TargetID   string  `json:"targetID"`
	Importance float32 `json:"importance,omitempty"`
	Confidence float32 `json:"confidence,omitempty"`
}

// Kind returns ActionRescore.
func (RescoreAction) Kind() ActionKind { return ActionRescore }

// UnmarshalActions decodes the pack's JSON action array into typed
// Action values. Unknown actions return an error rather than silent
// drop — the pack contract is strict.
func UnmarshalActions(data []byte) ([]Action, error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, fmt.Errorf("decode action array: %w", err)
	}
	out := make([]Action, 0, len(raws))
	for i, raw := range raws {
		var head struct {
			Action ActionKind `json:"action"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			return nil, fmt.Errorf("decode action[%d] header: %w", i, err)
		}
		act, err := decodeOne(head.Action, raw)
		if err != nil {
			return nil, fmt.Errorf("decode action[%d] (%s): %w", i, head.Action, err)
		}
		out = append(out, act)
	}
	return out, nil
}

func decodeOne(kind ActionKind, raw json.RawMessage) (Action, error) {
	switch kind {
	case ActionCreateSummary:
		var a CreateSummaryAction
		return a, json.Unmarshal(raw, &a)
	case ActionSupersede:
		var a SupersedeAction
		return a, json.Unmarshal(raw, &a)
	case ActionRescope:
		var a RescopeAction
		return a, json.Unmarshal(raw, &a)
	case ActionInvalidate:
		var a InvalidateAction
		return a, json.Unmarshal(raw, &a)
	case ActionMergeEntities:
		var a MergeEntitiesAction
		return a, json.Unmarshal(raw, &a)
	case ActionDiscard:
		var a DiscardAction
		return a, json.Unmarshal(raw, &a)
	case ActionRescore:
		var a RescoreAction
		return a, json.Unmarshal(raw, &a)
	default:
		return nil, fmt.Errorf("unknown action kind: %q", kind)
	}
}

// MutabilityMutable is the string value the validator + tests use to
// check a row is fair game for modification. Mirrors the value written
// to the Postgres `mutability` column.
const MutabilityMutable = "mutable"

// SafetyGateAgentScoped names the agent-scoped k-anonymity gate key in
// MemoryConsolidationSafetyGates.MinDistinctUserCount.
const SafetyGateAgentScoped = "agentScoped"

// SafetyGateUserScoped names the user-scoped k-anonymity gate key.
const SafetyGateUserScoped = "userScoped"

// Bucket is one pre-filter output bucket (e.g., a group of stale
// observations sharing kind+name).
type Bucket struct {
	Key     string         `json:"key"`
	Entries []BucketEntry  `json:"entries"`
	Stats   map[string]any `json:"stats,omitempty"`
}

// BucketEntry is one memory row inside a bucket. Mutability is surfaced
// so the pack can reason about which rows are off-limits.
type BucketEntry struct {
	ID         string            `json:"id"`
	Content    string            `json:"content"`
	Scope      Scope             `json:"scope"`
	Mutability string            `json:"mutability"`
	SourceType string            `json:"sourceType"`
	ObservedAt time.Time         `json:"observedAt,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// FunctionInput is the JSON body the worker POSTs to /functions/{name}.
type FunctionInput struct {
	Axis        PreFilterAxis `json:"axis"`
	WorkspaceID string        `json:"workspaceID"`
	Buckets     []Bucket      `json:"buckets"`
	// Gates surfaces the per-policy safety gates so the pack can
	// proactively respect them (e.g., not propose rescope below
	// minDistinctUserCount).
	Gates ResolvedGates `json:"gates"`
}

// ResolvedGates mirrors api/v1alpha1.MemoryConsolidationSafetyGates
// with defaults applied, in a JSON-safe shape.
type ResolvedGates struct {
	MinDistinctUserCount map[string]int32 `json:"minDistinctUserCount"`
	RequirePIIRedaction  bool             `json:"requirePIIRedaction"`
}
