/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package sources

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

func TestConvertEvalResults_ConversationLevel(t *testing.T) {
	results := []*api.EvalResult{
		{
			EvalID:   "tone-check",
			EvalType: "tone",
			Passed:   true,
			Details:  json.RawMessage(`{"score": 0.95}`),
		},
	}
	messages := []session.Message{{ID: "m1"}}

	convResults, turnResults := convertEvalResults(results, messages)

	require.Len(t, convResults, 1)
	assert.Equal(t, "tone", convResults[0].Type)
	assert.True(t, convResults[0].Passed)
	assert.Equal(t, "tone-check", convResults[0].Message)
	assert.Equal(t, 0.95, convResults[0].Details["score"])
	assert.Nil(t, turnResults)
}

func TestConvertEvalResults_TurnLevel(t *testing.T) {
	results := []*api.EvalResult{
		{
			EvalID:    "contains-check",
			EvalType:  "contains",
			Passed:    false,
			MessageID: "m2",
			Details:   json.RawMessage(`{"expected": "hello"}`),
		},
	}
	messages := []session.Message{
		{ID: "m1"},
		{ID: "m2"},
		{ID: "m3"},
	}

	convResults, turnResults := convertEvalResults(results, messages)

	assert.Nil(t, convResults)
	require.NotNil(t, turnResults)
	require.Len(t, turnResults[1], 1)
	assert.Equal(t, "contains", turnResults[1][0].Type)
	assert.False(t, turnResults[1][0].Passed)
	assert.Equal(t, "contains-check", turnResults[1][0].Message)
	assert.Equal(t, "hello", turnResults[1][0].Params["expected"])
}

func TestConvertEvalResults_Mixed(t *testing.T) {
	results := []*api.EvalResult{
		{
			EvalID:   "overall-tone",
			EvalType: "tone",
			Passed:   true,
		},
		{
			EvalID:    "turn-contains",
			EvalType:  "contains",
			Passed:    false,
			MessageID: "m1",
		},
		{
			EvalID:    "turn-length",
			EvalType:  "max_length",
			Passed:    true,
			MessageID: "m2",
		},
	}
	messages := []session.Message{
		{ID: "m1"},
		{ID: "m2"},
	}

	convResults, turnResults := convertEvalResults(results, messages)

	require.Len(t, convResults, 1)
	assert.Equal(t, "tone", convResults[0].Type)
	assert.True(t, convResults[0].Passed)

	require.NotNil(t, turnResults)
	require.Len(t, turnResults[0], 1)
	assert.Equal(t, "contains", turnResults[0][0].Type)
	require.Len(t, turnResults[1], 1)
	assert.Equal(t, "max_length", turnResults[1][0].Type)
}

func TestConvertEvalResults_Empty(t *testing.T) {
	convResults, turnResults := convertEvalResults(nil, nil)

	assert.Nil(t, convResults)
	assert.Nil(t, turnResults)
}

func TestConvertEvalResults_UnknownMessageID(t *testing.T) {
	results := []*api.EvalResult{
		{
			EvalID:    "check",
			EvalType:  "contains",
			Passed:    false,
			MessageID: "unknown",
		},
	}
	messages := []session.Message{{ID: "m1"}}

	convResults, turnResults := convertEvalResults(results, messages)

	assert.Nil(t, convResults)
	assert.Nil(t, turnResults)
}

func TestConvertEvalResults_InvalidDetails(t *testing.T) {
	results := []*api.EvalResult{
		{
			EvalID:   "check",
			EvalType: "tone",
			Passed:   true,
			Details:  json.RawMessage(`invalid json`),
		},
	}

	convResults, _ := convertEvalResults(results, nil)

	require.Len(t, convResults, 1)
	assert.Nil(t, convResults[0].Details)
}

func TestMessageIndex(t *testing.T) {
	messages := []session.Message{
		{ID: "m1"},
		{ID: "m2"},
		{ID: "m3"},
	}

	idx := messageIndex(messages)

	assert.Equal(t, 0, idx["m1"])
	assert.Equal(t, 1, idx["m2"])
	assert.Equal(t, 2, idx["m3"])
	assert.Len(t, idx, 3)
}

func TestMessageIndex_Empty(t *testing.T) {
	idx := messageIndex(nil)
	assert.Empty(t, idx)
}

func TestMessageIndex_SkipsEmptyIDs(t *testing.T) {
	messages := []session.Message{
		{ID: "m1"},
		{ID: ""},
		{ID: "m3"},
	}

	idx := messageIndex(messages)

	assert.Len(t, idx, 2)
	_, exists := idx[""]
	assert.False(t, exists)
}
