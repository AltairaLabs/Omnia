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

package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ToolProvider is optionally implemented by stores that register additional tools.
type ToolProvider interface {
	Tools() []ToolDef
}

// ToolDef describes a tool that can be registered.
type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
	Handler     func(ctx context.Context, params json.RawMessage) (string, error)
}

// Tool name constants.
const (
	toolNameRelated  = "memory__related"
	toolNameTimeline = "memory__timeline"
)

// Tools returns the list of graph and timeline tools registered by PostgresMemoryStore.
func (s *PostgresMemoryStore) Tools() []ToolDef {
	return []ToolDef{
		{
			Name:        toolNameRelated,
			Description: "Traverse the memory graph to find entities related to a given entity.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity_id":      map[string]any{"type": "string", "description": "UUID of the source entity"},
					"relation_types": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional filter on relation types"},
					"depth":          map[string]any{"type": "integer", "description": "Traversal depth (currently only 1 is supported)", "default": 1},
				},
				"required": []string{"entity_id"},
			},
			Handler: s.handleRelated,
		},
		{
			Name:        toolNameTimeline,
			Description: "Retrieve the temporal observation history for a given entity.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"entity_id": map[string]any{"type": "string", "description": "UUID of the entity"},
					"limit":     map[string]any{"type": "integer", "description": "Maximum number of observations to return", "default": 10},
				},
				"required": []string{"entity_id"},
			},
			Handler: s.handleTimeline,
		},
	}
}

// relatedParams is the input shape for memory__related.
type relatedParams struct {
	EntityID      string   `json:"entity_id"`
	RelationTypes []string `json:"relation_types"`
	Depth         int      `json:"depth"`
}

// relatedResult is a single row returned by memory__related.
type relatedResult struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`
	RelationType string  `json:"relation_type"`
	Weight       float32 `json:"weight"`
}

// handleRelated implements the memory__related tool.
func (s *PostgresMemoryStore) handleRelated(ctx context.Context, raw json.RawMessage) (string, error) {
	var p relatedParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("memory__related: invalid params: %w", err)
	}
	if p.EntityID == "" {
		return "", fmt.Errorf("memory__related: entity_id is required")
	}

	rows, err := s.queryRelated(ctx, p)
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("memory__related: marshal result: %w", err)
	}
	return string(out), nil
}

// queryRelated executes the graph traversal query.
func (s *PostgresMemoryStore) queryRelated(ctx context.Context, p relatedParams) ([]relatedResult, error) {
	sql, args := buildRelatedQuery(p)

	pgRows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("memory__related: query: %w", err)
	}
	defer pgRows.Close()

	var results []relatedResult
	for pgRows.Next() {
		var r relatedResult
		var weight *float32
		if err := pgRows.Scan(&r.ID, &r.Name, &r.Kind, &r.RelationType, &weight); err != nil {
			return nil, fmt.Errorf("memory__related: scan: %w", err)
		}
		if weight != nil {
			r.Weight = *weight
		}
		results = append(results, r)
	}
	if err := pgRows.Err(); err != nil {
		return nil, fmt.Errorf("memory__related: rows: %w", err)
	}
	if results == nil {
		results = []relatedResult{}
	}
	return results, nil
}

// buildRelatedQuery constructs the SQL and args for handleRelated.
func buildRelatedQuery(p relatedParams) (string, []any) {
	args := []any{p.EntityID}
	sql := `
		SELECT e.id, e.name, e.kind, r.relation_type, r.weight
		FROM memory_relations r
		JOIN memory_entities e ON e.id = r.target_entity_id
		WHERE r.source_entity_id = $1 AND e.forgotten = false`

	if len(p.RelationTypes) > 0 {
		args = append(args, p.RelationTypes)
		sql += fmt.Sprintf(" AND r.relation_type = ANY($%d)", len(args))
	}

	sql += " ORDER BY r.weight DESC LIMIT 20"
	return sql, args
}

// timelineParams is the input shape for memory__timeline.
type timelineParams struct {
	EntityID string `json:"entity_id"`
	Limit    int    `json:"limit"`
}

// timelineResult is a single row returned by memory__timeline.
type timelineResult struct {
	ID         string     `json:"id"`
	Content    string     `json:"content"`
	Confidence float32    `json:"confidence"`
	SourceType string     `json:"source_type"`
	ObservedAt time.Time  `json:"observed_at"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

// handleTimeline implements the memory__timeline tool.
func (s *PostgresMemoryStore) handleTimeline(ctx context.Context, raw json.RawMessage) (string, error) {
	var p timelineParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("memory__timeline: invalid params: %w", err)
	}
	if p.EntityID == "" {
		return "", fmt.Errorf("memory__timeline: entity_id is required")
	}
	if p.Limit <= 0 {
		p.Limit = 10
	}

	rows, err := s.queryTimeline(ctx, p)
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(rows)
	if err != nil {
		return "", fmt.Errorf("memory__timeline: marshal result: %w", err)
	}
	return string(out), nil
}

// queryTimeline executes the temporal history query.
func (s *PostgresMemoryStore) queryTimeline(ctx context.Context, p timelineParams) ([]timelineResult, error) {
	pgRows, err := s.pool.Query(ctx, `
		SELECT id, content, confidence, source_type, observed_at, valid_until
		FROM memory_observations
		WHERE entity_id = $1
		ORDER BY observed_at DESC
		LIMIT $2`,
		p.EntityID, p.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("memory__timeline: query: %w", err)
	}
	defer pgRows.Close()

	var results []timelineResult
	for pgRows.Next() {
		var r timelineResult
		if err := pgRows.Scan(&r.ID, &r.Content, &r.Confidence, &r.SourceType, &r.ObservedAt, &r.ValidUntil); err != nil {
			return nil, fmt.Errorf("memory__timeline: scan: %w", err)
		}
		results = append(results, r)
	}
	if err := pgRows.Err(); err != nil {
		return nil, fmt.Errorf("memory__timeline: rows: %w", err)
	}
	if results == nil {
		results = []timelineResult{}
	}
	return results, nil
}
