/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package evals

import (
	"context"
	"testing"

	runtimeevals "github.com/AltairaLabs/PromptKit/runtime/evals"
	sdkmetrics "github.com/AltairaLabs/PromptKit/runtime/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altairalabs/omnia/internal/session"
)

const (
	containsEvalType = "contains"
	helloToken       = "hello"
	paramPatterns    = "patterns"
)

// TestEvalMetric_CarriesVariantLabel is the wiring proof for #1188: it runs a
// real (no-LLM) `contains` eval through a collector that declares the same
// instance labels as the arena-eval-worker, then gathers the registry and
// asserts the emitted omnia_eval_<name> series actually carries variant.
//
// This fails if the collector is NOT constructed with "variant" in its
// InstanceLabels — which is the bug this test was written to catch: the
// per-call Bind map only supplies values for label NAMES declared at collector
// construction (see PromptKit collector.evalLabelValues).
func TestEvalMetric_CarriesVariantLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := sdkmetrics.NewEvalOnlyCollector(sdkmetrics.CollectorOpts{
		Registerer: reg,
		Namespace:  "omnia",
		// Must match ee/cmd/arena-eval-worker/main.go's declaration.
		InstanceLabels: []string{labelKeyAgent, labelKeyNamespace, labelKeyPromptPackName, labelKeyVariant},
	})
	runner := NewSDKRunner(WithEvalCollector(collector))

	packData := testPackData([]runtimeevals.EvalDef{
		{
			ID:      "e1",
			Type:    containsEvalType,
			Trigger: runtimeevals.TriggerEveryTurn,
			Params:  map[string]any{paramPatterns: []any{helloToken}},
			Metric:  &runtimeevals.MetricDef{Name: "test_contains", Type: runtimeevals.MetricBoolean},
		},
	})
	messages := []session.Message{
		{ID: "m1", Role: session.RoleUser, Content: "say " + helloToken},
		{ID: "m2", Role: session.RoleAssistant, Content: helloToken + " world"},
	}

	runner.RunTurnEvals(context.Background(), packData, messages, "sess-1", 1, nil,
		EvalLabels{Agent: "a", Namespace: "n", PromptPackName: "p", Variant: "candidate"})

	families, err := reg.Gather()
	require.NoError(t, err)

	variant, found := "", false
	for _, fam := range families {
		if fam.GetName() != "omnia_eval_test_contains" {
			continue
		}
		for _, m := range fam.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == labelKeyVariant {
					variant, found = l.GetValue(), true
				}
			}
		}
	}

	require.True(t, found, "omnia_eval_test_contains series should carry a variant label")
	assert.Equal(t, "candidate", variant)
}
