/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package consolidation

import "time"

// SummaryWrite captures a CreateSummary apply.
type SummaryWrite struct {
	WorkspaceID    string
	Scope          Scope
	Content        string
	Metadata       map[string]string
	FromIDs        []string
	PromotedByPack string
	PromotedAt     time.Time
}

// SupersedeWrite captures a Supersede apply.
type SupersedeWrite struct {
	WorkspaceID    string
	TargetIDs      []string
	WithID         string
	PromotedByPack string
	PromotedAt     time.Time
}

// RescopeWrite captures a Rescope apply.
type RescopeWrite struct {
	WorkspaceID    string
	TargetIDs      []string
	NewScope       Scope
	Reason         string
	PromotedByPack string
	PromotedAt     time.Time
}

// InvalidateWrite captures an Invalidate apply.
type InvalidateWrite struct {
	WorkspaceID    string
	TargetIDs      []string
	ValidUntil     time.Time
	Reason         string
	PromotedByPack string
	PromotedAt     time.Time
}

// MergeWrite captures a MergeEntities apply.
type MergeWrite struct {
	WorkspaceID    string
	CanonicalID    string
	MergeIDs       []string
	PromotedByPack string
	PromotedAt     time.Time
}

// DiscardWrite captures a Discard apply.
type DiscardWrite struct {
	WorkspaceID    string
	TargetIDs      []string
	Reason         string
	PromotedByPack string
	PromotedAt     time.Time
}

// RescoreWrite captures a Rescore apply. Lineage columns
// (PromotedByPack, PromotedAt) are populated alongside the scalar
// score fields so a forensic walker can answer "which pack rescored
// this row, when?".
type RescoreWrite struct {
	WorkspaceID    string
	TargetID       string
	Importance     float32
	Confidence     float32
	PromotedByPack string
	PromotedAt     time.Time
}
