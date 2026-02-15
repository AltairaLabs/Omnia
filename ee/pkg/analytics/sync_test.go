/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package analytics

import (
	"testing"
	"time"
)

func TestSyncOpts_Defaults(t *testing.T) {
	opts := SyncOpts{}
	if opts.BatchSize != 0 {
		t.Errorf("expected default BatchSize 0, got %d", opts.BatchSize)
	}
	if opts.DryRun {
		t.Error("expected default DryRun false")
	}
	if len(opts.Tables) != 0 {
		t.Errorf("expected empty Tables, got %v", opts.Tables)
	}
}

func TestSyncResult_Fields(t *testing.T) {
	now := time.Now()
	result := SyncResult{
		TotalRows:     100,
		Duration:      5 * time.Second,
		WatermarkFrom: now.Add(-1 * time.Hour),
		WatermarkTo:   now,
		Tables: []TableSyncResult{
			{Table: "sessions", RowsSynced: 60, WatermarkFrom: now.Add(-1 * time.Hour), WatermarkTo: now},
			{Table: "messages", RowsSynced: 40, WatermarkFrom: now.Add(-1 * time.Hour), WatermarkTo: now},
		},
	}
	if result.TotalRows != 100 {
		t.Errorf("expected TotalRows 100, got %d", result.TotalRows)
	}
	if len(result.Tables) != 2 {
		t.Errorf("expected 2 table results, got %d", len(result.Tables))
	}
}

func TestSessionRow_Fields(t *testing.T) {
	now := time.Now()
	row := SessionRow{
		SessionID:         "sess-1",
		WorkspaceID:       "ws-1",
		AgentID:           "agent-1",
		UserID:            "user-1",
		Status:            "active",
		Namespace:         "default",
		CreatedAt:         now,
		UpdatedAt:         now,
		MessageCount:      10,
		TotalInputTokens:  500,
		TotalOutputTokens: 300,
		Tags:              []string{"prod", "v2"},
		Metadata:          map[string]string{"env": "prod"},
	}
	if row.SessionID != "sess-1" {
		t.Errorf("expected SessionID 'sess-1', got %s", row.SessionID)
	}
	if len(row.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(row.Tags))
	}
}

func TestMessageRow_Fields(t *testing.T) {
	now := time.Now()
	row := MessageRow{
		MessageID:    "msg-1",
		SessionID:    "sess-1",
		Role:         "user",
		Content:      "hello",
		InputTokens:  50,
		OutputTokens: 0,
		SequenceNum:  1,
		CreatedAt:    now,
	}
	if row.MessageID != "msg-1" {
		t.Errorf("expected MessageID 'msg-1', got %s", row.MessageID)
	}
	if row.Role != "user" {
		t.Errorf("expected Role 'user', got %s", row.Role)
	}
}

func TestEvalResultRow_Fields(t *testing.T) {
	now := time.Now()
	score := 0.95
	dur := 150
	tokens := 200
	cost := 0.002
	row := EvalResultRow{
		ID:                "e1",
		SessionID:         "sess-1",
		MessageID:         "msg-1",
		AgentName:         "agent-a",
		Namespace:         "default",
		PromptPackName:    "pp-1",
		PromptPackVersion: "v1",
		EvalID:            "eval-a",
		EvalType:          "llm-judge",
		Trigger:           "on-message",
		Passed:            true,
		Score:             &score,
		Details:           `{"reason":"good"}`,
		DurationMs:        &dur,
		JudgeTokens:       &tokens,
		JudgeCostUSD:      &cost,
		Source:            "runtime",
		CreatedAt:         now,
	}
	if row.ID != "e1" {
		t.Errorf("expected ID 'e1', got %s", row.ID)
	}
	if row.SessionID != "sess-1" {
		t.Errorf("expected SessionID 'sess-1', got %s", row.SessionID)
	}
	if !row.Passed {
		t.Error("expected Passed true")
	}
	if row.Score == nil || *row.Score != 0.95 {
		t.Errorf("expected Score 0.95, got %v", row.Score)
	}
	if row.EvalType != "llm-judge" {
		t.Errorf("expected EvalType 'llm-judge', got %s", row.EvalType)
	}
}

func TestTableSyncResult_WithError(t *testing.T) {
	result := TableSyncResult{
		Table:      "sessions",
		RowsSynced: 0,
		Error:      ErrNotInitialized,
	}
	if result.Error == nil {
		t.Error("expected non-nil error")
	}
	if result.Error != ErrNotInitialized {
		t.Errorf("expected ErrNotInitialized, got %v", result.Error)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrNotInitialized", ErrNotInitialized, "sync provider not initialized"},
		{"ErrAlreadyClosed", ErrAlreadyClosed, "sync provider already closed"},
		{"ErrNoTables", ErrNoTables, "no tables configured for sync"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("expected %q, got %q", tt.msg, tt.err.Error())
			}
		})
	}
}
