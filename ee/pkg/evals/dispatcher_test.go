/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	api "github.com/altairalabs/omnia/internal/session/api"
)

func TestNewEvalDispatcher_RuleType(t *testing.T) {
	dispatcher := NewEvalDispatcher()

	def := api.EvalDefinition{
		ID:      "e1",
		Type:    "contains",
		Trigger: "per_turn",
		Params:  map[string]any{"value": "hello"},
	}

	messages := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "hello world", Timestamp: time.Now()},
	}

	result, err := dispatcher(def, messages)
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Equal(t, "e1", result.EvalID)
}

func TestNewEvalDispatcher_ArenaAssertion(t *testing.T) {
	dispatcher := NewEvalDispatcher()

	def := api.EvalDefinition{
		ID:      "e2",
		Type:    EvalTypeArenaAssertion,
		Trigger: "on_session_complete",
		Params: map[string]any{
			"assertion_type": "content_includes_any",
			"assertion_params": map[string]any{
				"patterns": []any{"hello"},
			},
		},
	}

	messages := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "hello world", Timestamp: time.Now()},
	}

	result, err := dispatcher(def, messages)
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Equal(t, "e2", result.EvalID)
	assert.Equal(t, EvalTypeArenaAssertion, result.EvalType)
}

func TestNewEvalDispatcher_ArenaAssertionError(t *testing.T) {
	dispatcher := NewEvalDispatcher()

	def := api.EvalDefinition{
		ID:      "e3",
		Type:    EvalTypeArenaAssertion,
		Trigger: "on_session_complete",
		Params:  map[string]any{},
	}

	_, err := dispatcher(def, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "assertion_type")
}

func TestNewEvalDispatcher_UnknownTypeFallsThrough(t *testing.T) {
	dispatcher := NewEvalDispatcher()

	def := api.EvalDefinition{
		ID:      "e4",
		Type:    "max_length",
		Trigger: "per_turn",
		Params:  map[string]any{"maxLength": float64(1000)},
	}

	messages := []session.Message{
		{ID: "m1", Role: session.RoleAssistant, Content: "short", Timestamp: time.Now()},
	}

	result, err := dispatcher(def, messages)
	require.NoError(t, err)
	assert.True(t, result.Passed)
}

func TestIsDeterministicEval(t *testing.T) {
	tests := []struct {
		evalType string
		expected bool
	}{
		{"rule", true},
		{EvalTypeArenaAssertion, true},
		{"contains", true},
		{"max_length", true},
		{"similarity", true},
		{evalTypeLLMJudge, false},
	}

	for _, tc := range tests {
		t.Run(tc.evalType, func(t *testing.T) {
			assert.Equal(t, tc.expected, isDeterministicEval(tc.evalType))
		})
	}
}
