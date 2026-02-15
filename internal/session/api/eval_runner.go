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
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/altairalabs/omnia/internal/session"
)

// Supported rule-based eval types.
const (
	EvalTypeContains    = "contains"
	EvalTypeNotContains = "not_contains"
	EvalTypeMaxLength   = "max_length"
	EvalTypeMinLength   = "min_length"
	EvalTypeRegexMatch  = "regex_match"
)

// EvalDefinition represents a single eval to run against session messages.
type EvalDefinition struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Trigger string         `json:"trigger"`
	Params  map[string]any `json:"params,omitempty"`
}

// EvaluateRequest is the JSON body for POST /api/v1/sessions/{id}/evaluate.
type EvaluateRequest struct {
	Evals []EvalDefinition `json:"evals"`
}

// EvaluateResultItem represents a single eval result in the response.
type EvaluateResultItem struct {
	EvalID     string   `json:"evalId"`
	EvalType   string   `json:"evalType"`
	Trigger    string   `json:"trigger"`
	Passed     bool     `json:"passed"`
	Score      *float64 `json:"score,omitempty"`
	DurationMs int      `json:"durationMs"`
	Source     string   `json:"source"`
}

// EvaluateResponseSummary contains aggregate counts for the evaluation run.
type EvaluateResponseSummary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

// EvaluateResponse is the JSON response for the evaluate endpoint.
type EvaluateResponse struct {
	Results []EvaluateResultItem    `json:"results"`
	Summary EvaluateResponseSummary `json:"summary"`
}

// RunRuleEval executes a single rule-based eval against the given messages.
// It returns the result item with timing information.
func RunRuleEval(evalDef EvalDefinition, messages []session.Message) (EvaluateResultItem, error) {
	start := time.Now()

	assistantMsgs := filterAssistantMessages(messages)

	passed, score, err := executeRule(evalDef.Type, evalDef.Params, assistantMsgs)
	if err != nil {
		return EvaluateResultItem{}, fmt.Errorf("eval %q: %w", evalDef.ID, err)
	}

	durationMs := int(time.Since(start).Milliseconds())

	item := EvaluateResultItem{
		EvalID:     evalDef.ID,
		EvalType:   evalDef.Type,
		Trigger:    evalDef.Trigger,
		Passed:     passed,
		DurationMs: durationMs,
		Source:     "manual",
	}
	if score != nil {
		item.Score = score
	}

	return item, nil
}

// filterAssistantMessages returns only messages with the assistant role.
func filterAssistantMessages(messages []session.Message) []session.Message {
	result := make([]session.Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == session.RoleAssistant {
			result = append(result, m)
		}
	}
	return result
}

// executeRule dispatches to the appropriate rule evaluator.
func executeRule(evalType string, params map[string]any, msgs []session.Message) (bool, *float64, error) {
	switch evalType {
	case EvalTypeContains:
		return evalContains(params, msgs)
	case EvalTypeNotContains:
		return evalNotContains(params, msgs)
	case EvalTypeMaxLength:
		return evalMaxLength(params, msgs)
	case EvalTypeMinLength:
		return evalMinLength(params, msgs)
	case EvalTypeRegexMatch:
		return evalRegexMatch(params, msgs)
	default:
		return false, nil, fmt.Errorf("unsupported eval type: %s", evalType)
	}
}

// evalContains checks if all assistant messages contain a given substring.
func evalContains(params map[string]any, msgs []session.Message) (bool, *float64, error) {
	value, err := getStringParam(params, "value")
	if err != nil {
		return false, nil, err
	}

	if len(msgs) == 0 {
		return false, nil, nil
	}

	matched := 0
	for _, m := range msgs {
		if strings.Contains(m.Content, value) {
			matched++
		}
	}

	score := float64(matched) / float64(len(msgs))
	passed := matched == len(msgs)
	return passed, &score, nil
}

// evalNotContains checks that no assistant messages contain a given substring.
func evalNotContains(params map[string]any, msgs []session.Message) (bool, *float64, error) {
	value, err := getStringParam(params, "value")
	if err != nil {
		return false, nil, err
	}

	if len(msgs) == 0 {
		return true, nil, nil
	}

	clean := 0
	for _, m := range msgs {
		if !strings.Contains(m.Content, value) {
			clean++
		}
	}

	score := float64(clean) / float64(len(msgs))
	passed := clean == len(msgs)
	return passed, &score, nil
}

// evalMaxLength checks that all assistant messages are within a max character length.
func evalMaxLength(params map[string]any, msgs []session.Message) (bool, *float64, error) {
	maxLen, err := getIntParam(params, "maxLength")
	if err != nil {
		return false, nil, err
	}

	if len(msgs) == 0 {
		return true, nil, nil
	}

	withinLimit := 0
	for _, m := range msgs {
		if len(m.Content) <= maxLen {
			withinLimit++
		}
	}

	score := float64(withinLimit) / float64(len(msgs))
	passed := withinLimit == len(msgs)
	return passed, &score, nil
}

// evalMinLength checks that all assistant messages meet a minimum character length.
func evalMinLength(params map[string]any, msgs []session.Message) (bool, *float64, error) {
	minLen, err := getIntParam(params, "minLength")
	if err != nil {
		return false, nil, err
	}

	if len(msgs) == 0 {
		return false, nil, nil
	}

	meetsMin := 0
	for _, m := range msgs {
		if len(m.Content) >= minLen {
			meetsMin++
		}
	}

	score := float64(meetsMin) / float64(len(msgs))
	passed := meetsMin == len(msgs)
	return passed, &score, nil
}

// evalRegexMatch checks that all assistant messages match a given regex pattern.
func evalRegexMatch(params map[string]any, msgs []session.Message) (bool, *float64, error) {
	pattern, err := getStringParam(params, "pattern")
	if err != nil {
		return false, nil, err
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, nil, fmt.Errorf("invalid regex pattern: %w", err)
	}

	if len(msgs) == 0 {
		return false, nil, nil
	}

	matched := 0
	for _, m := range msgs {
		if re.MatchString(m.Content) {
			matched++
		}
	}

	score := float64(matched) / float64(len(msgs))
	passed := matched == len(msgs)
	return passed, &score, nil
}

// getStringParam extracts a string parameter from the params map.
func getStringParam(params map[string]any, key string) (string, error) {
	v, ok := params[key]
	if !ok {
		return "", fmt.Errorf("missing required param %q", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("param %q must be a string", key)
	}
	return s, nil
}

// getIntParam extracts an integer parameter from the params map.
// Handles both int and float64 (JSON numbers decode as float64).
func getIntParam(params map[string]any, key string) (int, error) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("missing required param %q", key)
	}
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	default:
		return 0, fmt.Errorf("param %q must be a number", key)
	}
}
