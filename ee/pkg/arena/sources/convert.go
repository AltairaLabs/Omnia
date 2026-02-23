/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package sources

import (
	"encoding/json"

	"github.com/AltairaLabs/PromptKit/tools/arena/assertions"
	"github.com/AltairaLabs/PromptKit/tools/arena/generate"
	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/api"
)

// convertEvalResults splits api.EvalResult into conversation-level and turn-level results.
// Results with an empty MessageID are treated as conversation-level assertions.
// Results with a MessageID are turn-level assertions, keyed by the message's index
// in the messages slice.
func convertEvalResults(
	results []*api.EvalResult,
	messages []session.Message,
) ([]assertions.ConversationValidationResult, map[int][]generate.TurnEvalResult) {
	if len(results) == 0 {
		return nil, nil
	}

	msgIdx := messageIndex(messages)
	var convResults []assertions.ConversationValidationResult
	turnResults := make(map[int][]generate.TurnEvalResult)

	for _, r := range results {
		if r.MessageID == "" {
			convResults = append(convResults, toConversationResult(r))
		} else {
			idx, ok := msgIdx[r.MessageID]
			if !ok {
				continue
			}
			turnResults[idx] = append(turnResults[idx], toTurnResult(r))
		}
	}

	if len(turnResults) == 0 {
		turnResults = nil
	}

	return convResults, turnResults
}

// toConversationResult converts an api.EvalResult to a ConversationValidationResult.
func toConversationResult(r *api.EvalResult) assertions.ConversationValidationResult {
	return assertions.ConversationValidationResult{
		Type:    r.EvalType,
		Passed:  r.Passed,
		Message: evalMessage(r),
		Details: unmarshalDetails(r.Details),
	}
}

// toTurnResult converts an api.EvalResult to a TurnEvalResult.
func toTurnResult(r *api.EvalResult) generate.TurnEvalResult {
	return generate.TurnEvalResult{
		Type:    r.EvalType,
		Passed:  r.Passed,
		Message: evalMessage(r),
		Params:  unmarshalDetails(r.Details),
	}
}

// evalMessage returns the EvalID as the human-readable label for a result.
func evalMessage(r *api.EvalResult) string {
	return r.EvalID
}

// unmarshalDetails parses the JSON details into a map.
// Returns nil if the input is empty or invalid.
func unmarshalDetails(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// messageIndex builds a MessageID → index lookup map.
func messageIndex(messages []session.Message) map[string]int {
	idx := make(map[string]int, len(messages))
	for i := range messages {
		if messages[i].ID != "" {
			idx[messages[i].ID] = i
		}
	}
	return idx
}
