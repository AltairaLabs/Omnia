/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Package consolidation holds data types shared between the core memory
// postgres layer and the enterprise consolidation worker. Moving them here
// keeps internal/memory free of EE imports.
package consolidation

import "time"

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

// MutabilityMutable is the string value the validator + tests use to
// check a row is fair game for modification. Mirrors the value written
// to the Postgres `mutability` column.
const MutabilityMutable = "mutable"

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
