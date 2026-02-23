/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"github.com/altairalabs/omnia/internal/session"
	api "github.com/altairalabs/omnia/internal/session/api"
)

// NewEvalDispatcher returns an EvalRunner that dispatches to the correct
// eval implementation based on the definition's Type field.
// Arena assertions are routed to RunArenaAssertion; all other deterministic
// types (rule, contains, max_length, etc.) fall through to api.RunRuleEval.
func NewEvalDispatcher() EvalRunner {
	return func(def api.EvalDefinition, msgs []session.Message) (api.EvaluateResultItem, error) {
		if def.Type == EvalTypeArenaAssertion {
			return RunArenaAssertion(def, msgs)
		}
		return api.RunRuleEval(def, msgs)
	}
}

// isDeterministicEval returns true for eval types that are deterministic
// (not requiring an LLM call) and can be run synchronously in-process.
func isDeterministicEval(evalType string) bool {
	return evalType != evalTypeLLMJudge
}
