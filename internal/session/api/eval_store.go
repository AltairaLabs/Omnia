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
	"encoding/json"
	"time"
)

// EvalResult represents a single evaluation result stored in the eval_results table.
type EvalResult struct {
	ID                string          `json:"id"`
	SessionID         string          `json:"sessionId"`
	MessageID         string          `json:"messageId,omitempty"`
	AgentName         string          `json:"agentName"`
	Namespace         string          `json:"namespace"`
	PromptPackName    string          `json:"promptpackName"`
	PromptPackVersion string          `json:"promptpackVersion,omitempty"`
	EvalID            string          `json:"evalId"`
	EvalType          string          `json:"evalType"`
	Trigger           string          `json:"trigger"`
	Passed            bool            `json:"passed"`
	Score             *float64        `json:"score,omitempty"`
	Details           json.RawMessage `json:"details,omitempty"`
	DurationMs        *int            `json:"durationMs,omitempty"`
	JudgeTokens       *int            `json:"judgeTokens,omitempty"`
	JudgeCostUSD      *float64        `json:"judgeCostUsd,omitempty"`
	Source            string          `json:"source"`
	CreatedAt         time.Time       `json:"createdAt"`
}

// EvalResultSummary contains aggregate statistics for a group of eval results.
type EvalResultSummary struct {
	EvalID        string   `json:"evalId"`
	EvalType      string   `json:"evalType"`
	Total         int      `json:"total"`
	Passed        int      `json:"passed"`
	Failed        int      `json:"failed"`
	PassRate      float64  `json:"passRate"`
	AvgScore      *float64 `json:"avgScore,omitempty"`
	AvgDurationMs *float64 `json:"avgDurationMs,omitempty"`
}

// EvalResultListOpts configures queries for listing eval results.
type EvalResultListOpts struct {
	Limit         int
	Offset        int
	AgentName     string
	Namespace     string
	EvalID        string
	EvalType      string
	Passed        *bool
	CreatedAfter  time.Time
	CreatedBefore time.Time
}

// EvalResultSummaryOpts configures queries for eval result summaries.
type EvalResultSummaryOpts struct {
	AgentName     string
	Namespace     string
	EvalType      string
	CreatedAfter  time.Time
	CreatedBefore time.Time
}

// EvalStore defines the persistence interface for eval results.
type EvalStore interface {
	// InsertEvalResults persists one or more eval results.
	InsertEvalResults(ctx context.Context, results []*EvalResult) error

	// GetSessionEvalResults retrieves eval results for a specific session.
	GetSessionEvalResults(ctx context.Context, sessionID string) ([]*EvalResult, error)

	// ListEvalResults retrieves eval results matching the given filters.
	ListEvalResults(ctx context.Context, opts EvalResultListOpts) ([]*EvalResult, int64, error)

	// GetEvalResultSummary returns aggregate statistics grouped by eval_id and eval_type.
	GetEvalResultSummary(ctx context.Context, opts EvalResultSummaryOpts) ([]*EvalResultSummary, error)
}
