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
	"testing"

	"github.com/altairalabs/omnia/internal/session"
)

func assistantMsg(content string) session.Message {
	return session.Message{Role: session.RoleAssistant, Content: content}
}

func userMsg(content string) session.Message {
	return session.Message{Role: session.RoleUser, Content: content}
}

func TestFilterAssistantMessages(t *testing.T) {
	msgs := []session.Message{
		userMsg("hello"),
		assistantMsg("hi there"),
		userMsg("bye"),
		assistantMsg("goodbye"),
	}
	result := filterAssistantMessages(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 assistant messages, got %d", len(result))
	}
}

func TestFilterAssistantMessages_Empty(t *testing.T) {
	result := filterAssistantMessages(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(result))
	}
}

func TestRunRuleEval_Contains_AllMatch(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("hello world"),
		assistantMsg("hello there"),
	}
	evalDef := EvalDefinition{
		ID:     "test-contains",
		Type:   EvalTypeContains,
		Params: map[string]any{"value": "hello"},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !item.Passed {
		t.Fatal("expected passed=true")
	}
	if item.Score == nil || *item.Score != 1.0 {
		t.Fatalf("expected score=1.0, got %v", item.Score)
	}
	if item.Source != "manual" {
		t.Fatalf("expected source=manual, got %s", item.Source)
	}
}

func TestRunRuleEval_Contains_PartialMatch(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("hello world"),
		assistantMsg("goodbye world"),
	}
	evalDef := EvalDefinition{
		ID:     "test-contains",
		Type:   EvalTypeContains,
		Params: map[string]any{"value": "hello"},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Passed {
		t.Fatal("expected passed=false")
	}
	if item.Score == nil || *item.Score != 0.5 {
		t.Fatalf("expected score=0.5, got %v", item.Score)
	}
}

func TestRunRuleEval_Contains_NoMessages(t *testing.T) {
	msgs := []session.Message{userMsg("hello")}
	evalDef := EvalDefinition{
		ID:     "test-contains",
		Type:   EvalTypeContains,
		Params: map[string]any{"value": "hello"},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Passed {
		t.Fatal("expected passed=false for no assistant messages")
	}
}

func TestRunRuleEval_Contains_MissingParam(t *testing.T) {
	evalDef := EvalDefinition{
		ID:     "test-contains",
		Type:   EvalTypeContains,
		Params: map[string]any{},
	}
	_, err := RunRuleEval(evalDef, []session.Message{assistantMsg("hi")})
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

func TestRunRuleEval_NotContains_Pass(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("hello world"),
		assistantMsg("hello there"),
	}
	evalDef := EvalDefinition{
		ID:     "test-not-contains",
		Type:   EvalTypeNotContains,
		Params: map[string]any{"value": "error"},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !item.Passed {
		t.Fatal("expected passed=true")
	}
}

func TestRunRuleEval_NotContains_Fail(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("hello error world"),
	}
	evalDef := EvalDefinition{
		ID:     "test-not-contains",
		Type:   EvalTypeNotContains,
		Params: map[string]any{"value": "error"},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Passed {
		t.Fatal("expected passed=false")
	}
}

func TestRunRuleEval_NotContains_NoMessages(t *testing.T) {
	evalDef := EvalDefinition{
		ID:     "test-not-contains",
		Type:   EvalTypeNotContains,
		Params: map[string]any{"value": "error"},
	}
	item, err := RunRuleEval(evalDef, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !item.Passed {
		t.Fatal("expected passed=true for empty messages")
	}
}

func TestRunRuleEval_MaxLength_Pass(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("short"),
		assistantMsg("also short"),
	}
	evalDef := EvalDefinition{
		ID:     "test-max-length",
		Type:   EvalTypeMaxLength,
		Params: map[string]any{"maxLength": float64(100)},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !item.Passed {
		t.Fatal("expected passed=true")
	}
}

func TestRunRuleEval_MaxLength_Fail(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("this is a longer message that exceeds our limit"),
	}
	evalDef := EvalDefinition{
		ID:     "test-max-length",
		Type:   EvalTypeMaxLength,
		Params: map[string]any{"maxLength": float64(10)},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Passed {
		t.Fatal("expected passed=false")
	}
}

func TestRunRuleEval_MaxLength_NoMessages(t *testing.T) {
	evalDef := EvalDefinition{
		ID:     "test-max-length",
		Type:   EvalTypeMaxLength,
		Params: map[string]any{"maxLength": float64(100)},
	}
	item, err := RunRuleEval(evalDef, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !item.Passed {
		t.Fatal("expected passed=true for empty messages")
	}
}

func TestRunRuleEval_MaxLength_MissingParam(t *testing.T) {
	evalDef := EvalDefinition{
		ID:   "test-max-length",
		Type: EvalTypeMaxLength,
	}
	_, err := RunRuleEval(evalDef, []session.Message{assistantMsg("hi")})
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

func TestRunRuleEval_MinLength_Pass(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("this is a sufficient message"),
	}
	evalDef := EvalDefinition{
		ID:     "test-min-length",
		Type:   EvalTypeMinLength,
		Params: map[string]any{"minLength": float64(5)},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !item.Passed {
		t.Fatal("expected passed=true")
	}
}

func TestRunRuleEval_MinLength_Fail(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("hi"),
	}
	evalDef := EvalDefinition{
		ID:     "test-min-length",
		Type:   EvalTypeMinLength,
		Params: map[string]any{"minLength": float64(50)},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Passed {
		t.Fatal("expected passed=false")
	}
}

func TestRunRuleEval_MinLength_NoMessages(t *testing.T) {
	evalDef := EvalDefinition{
		ID:     "test-min-length",
		Type:   EvalTypeMinLength,
		Params: map[string]any{"minLength": float64(5)},
	}
	item, err := RunRuleEval(evalDef, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Passed {
		t.Fatal("expected passed=false for empty messages")
	}
}

func TestRunRuleEval_RegexMatch_Pass(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("The answer is 42."),
	}
	evalDef := EvalDefinition{
		ID:     "test-regex",
		Type:   EvalTypeRegexMatch,
		Params: map[string]any{"pattern": `\d+`},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !item.Passed {
		t.Fatal("expected passed=true")
	}
}

func TestRunRuleEval_RegexMatch_Fail(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("no numbers here"),
	}
	evalDef := EvalDefinition{
		ID:     "test-regex",
		Type:   EvalTypeRegexMatch,
		Params: map[string]any{"pattern": `^\d+$`},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Passed {
		t.Fatal("expected passed=false")
	}
}

func TestRunRuleEval_RegexMatch_InvalidPattern(t *testing.T) {
	msgs := []session.Message{
		assistantMsg("test"),
	}
	evalDef := EvalDefinition{
		ID:     "test-regex",
		Type:   EvalTypeRegexMatch,
		Params: map[string]any{"pattern": `[invalid`},
	}
	_, err := RunRuleEval(evalDef, msgs)
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestRunRuleEval_RegexMatch_NoMessages(t *testing.T) {
	evalDef := EvalDefinition{
		ID:     "test-regex",
		Type:   EvalTypeRegexMatch,
		Params: map[string]any{"pattern": `\d+`},
	}
	item, err := RunRuleEval(evalDef, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Passed {
		t.Fatal("expected passed=false for empty messages")
	}
}

func TestRunRuleEval_UnsupportedType(t *testing.T) {
	evalDef := EvalDefinition{
		ID:   "test-unknown",
		Type: "unknown_type",
	}
	_, err := RunRuleEval(evalDef, []session.Message{assistantMsg("hi")})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestRunRuleEval_SetsMetadata(t *testing.T) {
	msgs := []session.Message{assistantMsg("hello")}
	evalDef := EvalDefinition{
		ID:      "meta-check",
		Type:    EvalTypeContains,
		Trigger: "per_turn",
		Params:  map[string]any{"value": "hello"},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.EvalID != "meta-check" {
		t.Fatalf("expected evalId=meta-check, got %s", item.EvalID)
	}
	if item.EvalType != EvalTypeContains {
		t.Fatalf("expected evalType=contains, got %s", item.EvalType)
	}
	if item.Trigger != "per_turn" {
		t.Fatalf("expected trigger=per_turn, got %s", item.Trigger)
	}
}

func TestGetStringParam_WrongType(t *testing.T) {
	_, err := getStringParam(map[string]any{"key": 123}, "key")
	if err == nil {
		t.Fatal("expected error for non-string param")
	}
}

func TestGetIntParam_WrongType(t *testing.T) {
	_, err := getIntParam(map[string]any{"key": "not-a-number"}, "key")
	if err == nil {
		t.Fatal("expected error for non-numeric param")
	}
}

func TestGetIntParam_IntType(t *testing.T) {
	v, err := getIntParam(map[string]any{"key": 42}, "key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
}

func TestGetIntParam_Missing(t *testing.T) {
	_, err := getIntParam(map[string]any{}, "key")
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

func TestGetStringParam_Missing(t *testing.T) {
	_, err := getStringParam(map[string]any{}, "key")
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

func TestRunRuleEval_NotContains_MissingParam(t *testing.T) {
	evalDef := EvalDefinition{
		ID:     "test-not-contains",
		Type:   EvalTypeNotContains,
		Params: map[string]any{},
	}
	_, err := RunRuleEval(evalDef, []session.Message{assistantMsg("hi")})
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

func TestRunRuleEval_MinLength_MissingParam(t *testing.T) {
	evalDef := EvalDefinition{
		ID:     "test-min-length",
		Type:   EvalTypeMinLength,
		Params: nil,
	}
	_, err := RunRuleEval(evalDef, []session.Message{assistantMsg("hi")})
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

func TestRunRuleEval_RegexMatch_MissingParam(t *testing.T) {
	evalDef := EvalDefinition{
		ID:     "test-regex",
		Type:   EvalTypeRegexMatch,
		Params: map[string]any{},
	}
	_, err := RunRuleEval(evalDef, []session.Message{assistantMsg("hi")})
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

func TestRunRuleEval_MaxLength_IntParam(t *testing.T) {
	msgs := []session.Message{assistantMsg("hi")}
	evalDef := EvalDefinition{
		ID:     "test-max-int",
		Type:   EvalTypeMaxLength,
		Params: map[string]any{"maxLength": 100},
	}
	item, err := RunRuleEval(evalDef, msgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !item.Passed {
		t.Fatal("expected passed=true")
	}
}

func TestBuildEvaluateResponse(t *testing.T) {
	results := []EvaluateResultItem{
		{EvalID: "e1", Passed: true},
		{EvalID: "e2", Passed: false},
		{EvalID: "e3", Passed: true},
	}
	resp := buildEvaluateResponse(results)
	if resp.Summary.Total != 3 {
		t.Fatalf("expected total=3, got %d", resp.Summary.Total)
	}
	if resp.Summary.Passed != 2 {
		t.Fatalf("expected passed=2, got %d", resp.Summary.Passed)
	}
	if resp.Summary.Failed != 1 {
		t.Fatalf("expected failed=1, got %d", resp.Summary.Failed)
	}
}

func TestBuildEvaluateResponse_Empty(t *testing.T) {
	resp := buildEvaluateResponse(nil)
	if resp.Summary.Total != 0 {
		t.Fatalf("expected total=0, got %d", resp.Summary.Total)
	}
}
